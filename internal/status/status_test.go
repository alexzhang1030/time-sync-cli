package status_test

import (
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestParseChronyTracking(t *testing.T) {
	line := "^,192.168.1.1,1.2.3.4,1,0.000123,0.000456,0.001"
	source, offset := status.ParseChronyTracking(line)
	if source != "1.2.3.4" {
		t.Errorf("source = %q, want 1.2.3.4", source)
	}
	if offset != "0.000456" {
		t.Errorf("offset = %q, want 0.000456", offset)
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
	if m.PTPOffset() != "42 ns" {
		t.Errorf("PTPOffset() = %q, want 42 ns", m.PTPOffset())
	}
}

func TestParsePTPMetrics_FallbackOffset(t *testing.T) {
	m := status.ParsePTPMetrics("", "", "offsetFromMaster        -15.0")
	if m.PTPOffset() != "-15.0 ns" {
		t.Errorf("PTPOffset() = %q, want -15.0 ns", m.PTPOffset())
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
		Healthy:   true,
		NTPHealth: false,
		PTPHealth: "true",
		Role:      "ptp",
		Source:    "SLAVE",
		Offset:    "42 ns",
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
		"NTP health: false",
		"PTP health: true",
		"Overall health: true",
		"port state: SLAVE",
		"master offset: 42 ns",
		"path delay: 2500.0 ns",
		"steps removed: 1",
		"grandmaster: aabbcc.fffe.112233",
	} {
		if !contains(out, want) {
			t.Errorf("Summary missing %q:\n%s", want, out)
		}
	}
}

func TestReportSummary_SplitsNTPAndUnknownPTPHealth(t *testing.T) {
	r := &status.Report{
		Healthy:   true,
		NTPHealth: true,
		PTPHealth: "unknown",
		Role:      "ntp",
		Source:    "3",
		Offset:    "-1.462872982",
		Chrony: status.ChronyStatus{
			Active: true,
		},
		PTP: status.PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Detail:        "unable to query ptp4l via pmc",
		},
	}
	out := r.Summary()
	for _, want := range []string{
		"NTP health: true",
		"PTP health: unknown",
		"Overall health: true",
		"Active role: ntp",
		"(unable to query ptp4l via pmc)",
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
