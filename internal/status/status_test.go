package status_test

import (
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestParseChronyTracking(t *testing.T) {
	line := "C0A80101,192.168.1.1,3,1784102400.123456789,0.000123,-0.000456,0.001,3.225,-0.000,0.129,0.013639022,0.001100737,64.2,Normal"
	source, offset := status.ParseChronyTracking(line)
	if source != "192.168.1.1" {
		t.Errorf("source = %q, want 192.168.1.1", source)
	}
	if offset != "0.000123" {
		t.Errorf("offset = %q, want current correction 0.000123", offset)
	}
}

func TestParseChronyTrackingStatus(t *testing.T) {
	line := "C0A80101,192.168.1.1,3,1784102400.123456789,0.000123,-0.000456,0.001,3.225,-0.000,0.129,0.013639022,0.001100737,64.2,Normal"
	got, err := status.ParseChronyTrackingStatus(line)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Available || !got.Synchronized || got.Holdover {
		t.Fatalf("tracking state = %+v", got)
	}
	if got.ReferenceID != "C0A80101" || got.Stratum != 3 || got.LeapStatus != "Normal" {
		t.Fatalf("tracking fields = %+v", got)
	}
}

func TestParseChronyTrackingStatusDetectsHoldover(t *testing.T) {
	line := "7F7F0101,LOCAL,8,1784102400.123456789,0.000123,-0.000456,0.001,3.225,-0.000,0.129,0.013639022,0.001100737,64.2,Normal"
	got, err := status.ParseChronyTrackingStatus(line)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Holdover || got.Synchronized {
		t.Fatalf("tracking state = %+v", got)
	}
}

func TestParseChronyTrackingStatusDetectsUnsynchronized(t *testing.T) {
	line := "00000000,0.0.0.0,0,0.000000000,0.000000,0.000000,0.000,0.000,0.000,0.000,0.000000000,0.000000000,0.0,Not synchronised"
	got, err := status.ParseChronyTrackingStatus(line)
	if err != nil {
		t.Fatal(err)
	}
	if got.Synchronized {
		t.Fatalf("tracking state = %+v", got)
	}
}

func TestParseChronyTracking_Empty(t *testing.T) {
	source, offset := status.ParseChronyTracking("")
	if source != "" || offset != "" {
		t.Error("expected empty parse for empty input")
	}
}

func TestParsePMCFields(t *testing.T) {
	input := `sending: GET PORT_DATA_SET
	bbbbbeff.fffe.ffff-0 seq 0 RESPONSE MANAGEMENT PORT_DATA_SET
		portIdentity            bbbbbeff.fffe.ffff-1
		portState               SLAVE
		logMinDelayReqInterval  -5
`
	fields := status.ParsePMCFields(input)
	if fields["portState"] != "SLAVE" {
		t.Errorf("portState = %q, want SLAVE", fields["portState"])
	}
	if fields["portIdentity"] != "bbbbbeff.fffe.ffff-1" {
		t.Errorf("portIdentity = %q", fields["portIdentity"])
	}
}

func TestParsePTPMetrics(t *testing.T) {
	port := `portState               MASTER`
	timeNP := `master_offset           42
gmIdentity              aabbcc.fffe.112233`
	current := `offsetFromMaster        100.0
meanPathDelay           2500.0
stepsRemoved            0`

	m := status.ParsePTPMetrics(port, timeNP, current)
	if m.PortState != "MASTER" {
		t.Errorf("PortState = %q, want MASTER", m.PortState)
	}
	if m.MasterOffset != "42" {
		t.Errorf("MasterOffset = %q, want 42", m.MasterOffset)
	}
	if m.PathDelay != "2500.0" {
		t.Errorf("PathDelay = %q, want 2500.0", m.PathDelay)
	}
	if m.GMIdentity != "aabbcc.fffe.112233" {
		t.Errorf("GMIdentity = %q", m.GMIdentity)
	}
	if m.PTPOffset() != "0.000042 ms" {
		t.Errorf("PTPOffset() = %q, want 0.000042 ms", m.PTPOffset())
	}
}

func TestParsePTPMetrics_FallbackOffset(t *testing.T) {
	m := status.ParsePTPMetrics("", "", "offsetFromMaster        -15.0")
	if m.PTPOffset() != "-0.000015 ms" {
		t.Errorf("PTPOffset() = %q, want -0.000015 ms", m.PTPOffset())
	}
}

func TestParsePTPTimeProperties(t *testing.T) {
	properties, err := status.ParsePTPTimeProperties(`
currentUtcOffset       37
currentUtcOffsetValid  1
ptpTimescale           1
timeTraceable          1
frequencyTraceable     0
`)
	if err != nil {
		t.Fatal(err)
	}
	if properties.CurrentUTCOffset != 37 || !properties.CurrentUTCOffsetValid || !properties.PTPTimescale || !properties.TimeTraceable || properties.FrequencyTraceable {
		t.Fatalf("properties = %+v", properties)
	}
}

func TestReportSummary_PTPNotRunning(t *testing.T) {
	r := &status.Report{
		Healthy:   false,
		NTPHealth: false,
		PTPHealth: "false",
		Role:      "unknown",
		PTP: status.PTPStatus{
			Detail: "ptp4l not running",
		},
	}
	out := r.Summary()
	if !contains(out, "(ptp4l not running)") {
		t.Errorf("Summary missing not-running detail:\n%s", out)
	}
}

func TestReportSummary_PTPDetails(t *testing.T) {
	r := &status.Report{
		SchemaVersion:     "1.2",
		ManagementState:   "managed",
		ConfiguredRole:    "client",
		ConfiguredPTP:     true,
		Role:              "ptp",
		SystemClockSource: "aabbcc.fffe.112233",
		ClockFlow:         "PTP grandmaster → PHC → System",
		Offset:            "0.000042 ms",
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthDisabled,
			PTPLink:     status.HealthHealthy,
			PTPAccuracy: status.HealthHealthy,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthHealthy,
		},
		PTP: status.PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
			PathDelay:     "2500.0",
			StepsRemoved:  "1",
			GMIdentity:    "aabbcc.fffe.112233",
		},
	}
	out := r.Summary()
	for _, want := range []string{
		"Overall status: healthy",
		"Management: managed by timesync",
		"NTP sync status: disabled",
		"PTP link status: healthy",
		"PTP accuracy status: healthy",
		"System clock source: aabbcc.fffe.112233",
		"port state: SLAVE",
		"grandmaster offset: 0.000042 ms",
		"master offset: 0.000042 ms",
		"mean path delay: 0.002500 ms",
		"grandmaster hops: 1",
		"grandmaster: aabbcc.fffe.112233",
	} {
		if !contains(out, want) {
			t.Errorf("Summary missing %q:\n%s", want, out)
		}
	}
}

func TestReportSummary_SplitsRequiredNTPAndOptionalPTPHealth(t *testing.T) {
	r := &status.Report{
		SchemaVersion:     "1.2",
		ManagementState:   "managed",
		ConfiguredRole:    "auto",
		ConfiguredPTP:     true,
		Role:              "ntp",
		SystemClockSource: "time.example.net",
		ClockFlow:         "NTP → System · PTP monitor",
		Offset:            "-1.462872982 s",
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthHealthy,
			PTPLink:     status.HealthUnknown,
			PTPAccuracy: status.HealthUnknown,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthDisabled,
		},
		Chrony: status.ChronyStatus{
			Active:       true,
			Synchronized: true,
			Source:       "time.example.net",
		},
		PTP: status.PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Detail:        "unable to query ptp4l via pmc",
		},
	}
	out := r.Summary()
	for _, want := range []string{
		"Overall status: healthy",
		"NTP sync status: healthy",
		"PTP link status: unknown",
		"Observed discipline: ntp",
		"System clock source: time.example.net",
		"Current offset: -1.462872982 s",
		"(unable to query ptp4l via pmc)",
	} {
		if !contains(out, want) {
			t.Errorf("Summary missing %q:\n%s", want, out)
		}
	}
}

func TestReportSummary_PTPClientShowsGrandmasterAsSystemSource(t *testing.T) {
	r := &status.Report{
		SchemaVersion:     "1.2",
		ManagementState:   "managed",
		ConfiguredRole:    "client",
		ConfiguredPTP:     true,
		Role:              "ptp",
		SystemClockSource: "aabbcc.fffe.112233",
		ClockFlow:         "PTP grandmaster → PHC → System",
		Offset:            "0.000042 ms",
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthDisabled,
			PTPLink:     status.HealthHealthy,
			PTPAccuracy: status.HealthHealthy,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthHealthy,
		},
		Chrony: status.ChronyStatus{
			Active: true,
			Offset: "-1.462872982",
		},
		PTP: status.PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
			GMIdentity:    "aabbcc.fffe.112233",
		},
	}
	out := r.Summary()
	for _, want := range []string{
		"NTP sync status: disabled",
		"PTP link status: healthy",
		"Observed discipline: ptp",
		"System clock source: aabbcc.fffe.112233",
		"Current offset: 0.000042 ms",
		"current correction: -1.462872982 s",
		"grandmaster offset: 0.000042 ms",
	} {
		if !contains(out, want) {
			t.Errorf("Summary missing %q:\n%s", want, out)
		}
	}
}

func TestReportSummary_MasterShowsPTPAndNTPOffsets(t *testing.T) {
	r := &status.Report{
		SchemaVersion:     "1.2",
		ManagementState:   "managed",
		ConfiguredRole:    "master",
		ConfiguredPTP:     true,
		Role:              "ntp",
		SystemClockSource: "time.example.net",
		ClockFlow:         "NTP → System → PHC → PTP clients",
		Offset:            "0.000091882 s",
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthHealthy,
			PTPLink:     status.HealthHealthy,
			PTPAccuracy: status.HealthNotApplicable,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthHealthy,
		},
		Chrony: status.ChronyStatus{
			Active:       true,
			Synchronized: true,
			Source:       "time.example.net",
			Offset:       "0.000091882",
		},
		PTP: status.PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Available:     true,
			PortState:     "MASTER",
			MasterOffset:  "15",
		},
	}
	out := r.Summary()
	for _, want := range []string{
		"Configured role: master",
		"Observed discipline: ntp",
		"System clock source: time.example.net",
		"Current offset: 0.000091882 s",
		"current correction: 0.000091882 s",
		"port state: MASTER",
		"grandmaster offset: 0.000015 ms",
	} {
		if !contains(out, want) {
			t.Errorf("Summary missing %q:\n%s", want, out)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
