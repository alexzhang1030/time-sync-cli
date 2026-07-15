package status

import (
	"testing"
	"time"
)

func TestInferSyncStateAddsNTPUnit(t *testing.T) {
	r := &Report{
		Chrony: ChronyStatus{
			Active: true,
			Source: "192.168.1.1",
			Offset: "-0.000092",
		},
	}
	role, source, offset := inferSyncState(r)
	if role != "ntp" || source != "192.168.1.1" || offset != "-0.000092 s" {
		t.Fatalf("sync state = role=%q source=%q offset=%q", role, source, offset)
	}
}

func TestInferSyncStateUsesGrandmasterAsPTPSource(t *testing.T) {
	r := &Report{
		ConfiguredRole: "client",
		ConfiguredPTP:  true,
		PTP: PTPStatus{
			PHC2SysActive: true,
			PortState:     "SLAVE",
			GMIdentity:    "aabbcc.fffe.112233",
			MasterOffset:  "42",
		},
	}
	role, source, offset := inferSyncState(r)
	if role != "ptp" || source != "aabbcc.fffe.112233" || offset != "0.000042 ms" {
		t.Fatalf("sync state = role=%q source=%q offset=%q", role, source, offset)
	}
}

func TestInferSyncStateKeepsNTPAsMasterSystemSource(t *testing.T) {
	r := &Report{
		ConfiguredRole: "master",
		ConfiguredPTP:  true,
		Chrony: ChronyStatus{
			Active: true,
			Source: "192.168.1.1",
			Offset: "0.000010",
		},
		PTP: PTPStatus{
			PHC2SysActive: true,
			PortState:     "MASTER",
		},
	}
	role, source, _ := inferSyncState(r)
	if role != "ntp" || source != "192.168.1.1" {
		t.Fatalf("sync state = role=%q source=%q", role, source)
	}
}

func TestInferNTPStatusRequiresTrackingEvidence(t *testing.T) {
	if got := inferNTPStatus(ChronyStatus{UnitState: "query-error"}); got != HealthUnknown {
		t.Fatalf("query status = %q, want unknown", got)
	}
	if got := inferNTPStatus(ChronyStatus{Active: true}); got != HealthUnknown {
		t.Fatalf("status = %q, want unknown", got)
	}
	if got := inferNTPStatus(ChronyStatus{Active: true, Available: true, LeapStatus: "Not synchronised", Offset: "0"}); got != HealthUnhealthy {
		t.Fatalf("status = %q, want unhealthy", got)
	}
	if got := inferNTPStatus(ChronyStatus{Active: true, Available: true, Synchronized: true, Offset: "0.001"}); got != HealthHealthy {
		t.Fatalf("status = %q, want healthy", got)
	}
}

func TestInferNTPStatusMarksHoldoverAndLargeCorrection(t *testing.T) {
	if got := inferNTPStatus(ChronyStatus{Active: true, Available: true, Holdover: true, Offset: "0"}); got != HealthDegraded {
		t.Fatalf("holdover status = %q", got)
	}
	if got := inferNTPStatus(ChronyStatus{Active: true, Available: true, Synchronized: true, Offset: "0.5"}); got != HealthDegraded {
		t.Fatalf("correction status = %q", got)
	}
	if got := inferNTPStatus(ChronyStatus{Active: true, Available: true, Synchronized: true, Offset: "2"}); got != HealthUnhealthy {
		t.Fatalf("correction status = %q", got)
	}
}

func TestInferPTPHealthRejectsEpochSizedMasterOffset(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PTP4LState:    "active",
		PHC2SysActive: true,
		Available:     true,
		PortState:     "SLAVE",
		MasterOffset:  "-1783161116251737041",
	}, "client")
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferPTPAccuracySeparatesTargetFromGuardSafetyLimit(t *testing.T) {
	s := PTPStatus{
		PTP4LActive:   true,
		PTP4LState:    "active",
		PHC2SysActive: true,
		Available:     true,
		PortState:     "SLAVE",
		MasterOffset:  "500000000",
	}
	if got := inferPTPAccuracyStatus(s, "client"); got != HealthDegraded {
		t.Fatalf("accuracy = %q, want degraded", got)
	}
	if got := inferPTPHealth(s, "client"); got != "true" {
		t.Fatalf("guard readiness = %q, want true", got)
	}
}

func TestInferPTPAccuracyAcceptsTenMillisecondTarget(t *testing.T) {
	s := PTPStatus{PTP4LActive: true, PTP4LState: "active", Available: true, PortState: "SLAVE", MasterOffset: "9999999"}
	if got := inferPTPAccuracyStatus(s, "client"); got != HealthHealthy {
		t.Fatalf("accuracy = %q, want healthy", got)
	}
}

func TestInferPTPHealthEnforcesConfiguredPortRole(t *testing.T) {
	s := PTPStatus{PTP4LActive: true, PTP4LState: "active", Available: true, PortState: "MASTER", MasterOffset: "42"}
	if got := inferPTPHealth(s, "client"); got != "false" {
		t.Fatalf("client health = %q", got)
	}
	s.PortState = "SLAVE"
	if got := inferPTPHealth(s, "master"); got != "false" {
		t.Fatalf("master health = %q", got)
	}
}

func TestInferPTPHealthAcceptsMasterWithoutOffset(t *testing.T) {
	s := PTPStatus{PTP4LActive: true, PTP4LState: "active", Available: true, PortState: "MASTER"}
	if got := inferPTPHealth(s, "master"); got != "true" {
		t.Fatalf("health = %q, want true", got)
	}
	if got := inferPTPAccuracyStatus(s, "master"); got != HealthNotApplicable {
		t.Fatalf("accuracy = %q, want not_applicable", got)
	}
}

func TestInferPTPHealthUnknownWithoutMetrics(t *testing.T) {
	s := PTPStatus{PTP4LActive: true, PTP4LState: "active", Available: true, PortState: "SLAVE"}
	if got := inferPTPHealth(s, "client"); got != "unknown" {
		t.Fatalf("health = %q, want unknown", got)
	}
}

func TestInferPTPQueryFailureIsUnknown(t *testing.T) {
	s := PTPStatus{PTP4LState: "query-error"}
	if got := inferPTPLinkStatus(s, "client"); got != HealthUnknown {
		t.Fatalf("link = %q, want unknown", got)
	}
	if got := inferPTPAccuracyStatus(s, "client"); got != HealthUnknown {
		t.Fatalf("accuracy = %q, want unknown", got)
	}
}

func TestPopulatePHCResidualNormalizesTAITime(t *testing.T) {
	system := int64(1783162152)
	clock := ClockStatus{phcSampleSystemUnixNS: system * int64(time.Second)}
	ptp := PTPStatus{
		TimePropertiesAvailable: true,
		CurrentUTCOffset:        37,
		CurrentUTCOffsetValid:   true,
		PTPTimescale:            true,
	}
	populatePHCResidual(&clock, ptp, (system+37)*int64(time.Second))
	if clock.PHCResidualNS == nil || *clock.PHCResidualNS != 0 {
		t.Fatalf("residual = %v", clock.PHCResidualNS)
	}
	if clock.PHCTimeScale != "TAI" || clock.TAIUTCOffset != 37 || !clock.TAIUTCOffsetValid {
		t.Fatalf("clock = %+v", clock)
	}
}

func TestInferClockHealthRejectsEpochSystemClock(t *testing.T) {
	health := inferClockHealth(ClockStatus{SystemUnix: 1038, RTCUnix: 1783162152})
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferClockHealthRejectsBadRTC(t *testing.T) {
	health := inferClockHealth(ClockStatus{SystemUnix: 1783162152, RTCUnix: 1038})
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferClockHealthUsesNormalizedPHCResidual(t *testing.T) {
	zero := int64(0)
	health := inferClockHealth(ClockStatus{
		SystemUnix:    1783162152,
		RTCUnix:       1783162153,
		PHCUnix:       1783162189,
		PHCResidualNS: &zero,
	})
	if health != "true" {
		t.Fatalf("health = %q, want true", health)
	}
}

func TestInferClockStatusMarksResidualDegraded(t *testing.T) {
	residual := int64(20 * time.Millisecond)
	status := inferClockStatus(ClockStatus{
		SystemUnix:    1783162152,
		RTCUnix:       1783162152,
		PHCUnix:       1783162189,
		PHCResidualNS: &residual,
	})
	if status != HealthDegraded {
		t.Fatalf("status = %q, want degraded", status)
	}
}

func TestInferClockHealthUnknownWithoutPHCTimeScale(t *testing.T) {
	health := inferClockHealth(ClockStatus{SystemUnix: 1783162152, RTCUnix: 1783162152, PHCUnix: 1783162189})
	if health != "unknown" {
		t.Fatalf("health = %q, want unknown", health)
	}
}

func TestInferClockHealthUnknownWhenClockDetailPresent(t *testing.T) {
	health := inferClockHealth(ClockStatus{SystemUnix: 1783162152, RTCUnix: 1783162152, Detail: "unable to read PHC"})
	if health != "unknown" {
		t.Fatalf("health = %q, want unknown", health)
	}
}

func TestInferOverallStatusRequiresClientAccuracyDisciplineAndGuard(t *testing.T) {
	r := &Report{
		ConfiguredRole: "client",
		ConfiguredPTP:  true,
		Health: HealthSummary{
			Clock:       HealthHealthy,
			PTPLink:     HealthHealthy,
			PTPAccuracy: HealthHealthy,
			Discipline:  HealthHealthy,
			Guard:       HealthHealthy,
		},
	}
	if got := inferOverallStatus(r); got != HealthHealthy {
		t.Fatalf("overall = %q, want healthy", got)
	}
	r.Health.PTPAccuracy = HealthDegraded
	if got := inferOverallStatus(r); got != HealthDegraded {
		t.Fatalf("overall = %q, want degraded", got)
	}
	r.Health.Guard = HealthUnhealthy
	if got := inferOverallStatus(r); got != HealthUnhealthy {
		t.Fatalf("overall = %q, want unhealthy", got)
	}
}

func TestInferOverallStatusRequiresMasterNTP(t *testing.T) {
	r := &Report{
		ConfiguredRole: "master",
		ConfiguredPTP:  true,
		Health: HealthSummary{
			NTP:        HealthUnhealthy,
			Clock:      HealthHealthy,
			PTPLink:    HealthHealthy,
			Discipline: HealthHealthy,
			Guard:      HealthHealthy,
		},
	}
	if got := inferOverallStatus(r); got != HealthUnhealthy {
		t.Fatalf("overall = %q, want unhealthy", got)
	}
	r.Health.NTP = HealthDegraded
	if got := inferOverallStatus(r); got != HealthDegraded {
		t.Fatalf("overall = %q, want degraded", got)
	}
}

func TestInferOverallStatusKeepsAutoPTPMonitorOptional(t *testing.T) {
	r := &Report{
		ConfiguredRole: "auto",
		ConfiguredPTP:  true,
		Health: HealthSummary{
			NTP:     HealthHealthy,
			Clock:   HealthHealthy,
			PTPLink: HealthUnknown,
		},
	}
	if got := inferOverallStatus(r); got != HealthHealthy {
		t.Fatalf("overall = %q, want healthy", got)
	}
}

func TestPopulateDerivedStatusUsesNeutralStatesForDisabledSources(t *testing.T) {
	client := &Report{
		ManagementState: "managed",
		ConfiguredRole:  "client",
		ConfiguredPTP:   true,
		PTP: PTPStatus{
			PTP4LActive:   true,
			PTP4LState:    "active",
			PHC2SysActive: true,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
		},
		Clock: ClockStatus{SystemUnix: 1783162152, RTCUnix: 1783162152},
		Systemd: SystemdStatus{
			PHC2SysUnit: UnitStatus{ActiveState: "active"},
			GuardTimer:  UnitStatus{ActiveState: "active", UnitFileState: "enabled"},
		},
	}
	populateDerivedStatus(client)
	if client.Health.NTP != HealthDisabled {
		t.Fatalf("client NTP state = %q, want disabled", client.Health.NTP)
	}

	auto := &Report{
		ManagementState: "managed",
		ConfiguredRole:  "auto",
		Chrony: ChronyStatus{
			Active:       true,
			Available:    true,
			Synchronized: true,
			Offset:       "0.001",
		},
		Clock: ClockStatus{SystemUnix: 1783162152, RTCUnix: 1783162152},
	}
	populateDerivedStatus(auto)
	if auto.Health.PTPLink != HealthDisabled || auto.Health.PTPAccuracy != HealthDisabled {
		t.Fatalf("auto PTP states = link=%q accuracy=%q", auto.Health.PTPLink, auto.Health.PTPAccuracy)
	}
}

func TestInferOverallStatusMarksMissingStateUnmanaged(t *testing.T) {
	r := &Report{Health: HealthSummary{NTP: HealthHealthy, Clock: HealthHealthy}}
	if got := inferOverallStatus(r); got != HealthUnmanaged {
		t.Fatalf("overall = %q, want unmanaged", got)
	}
	if inferOverallHealth(r) {
		t.Fatal("unmanaged report returned legacy healthy=true")
	}
}

func TestInferOverallStatusMarksStateReadFailureUnknown(t *testing.T) {
	r := &Report{ManagementState: "error", Health: HealthSummary{NTP: HealthHealthy, Clock: HealthHealthy}}
	if got := inferOverallStatus(r); got != HealthUnknown {
		t.Fatalf("overall = %q, want unknown", got)
	}
}

func TestParsePHCTime(t *testing.T) {
	got, err := parsePHCTime("phc_ctl[1223.646]: clock time is 1783162369.726269290 or Sat Jul  4 18:52:49 2026")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1783162369 {
		t.Fatalf("got = %d, want 1783162369", got)
	}
	nanoseconds, err := parsePHCTimeNS("phc_ctl[1223.646]: clock time is 1783162369.726269290 or Sat Jul  4 18:52:49 2026")
	if err != nil {
		t.Fatal(err)
	}
	if nanoseconds != 1783162369726269290 {
		t.Fatalf("nanoseconds = %d", nanoseconds)
	}
}
