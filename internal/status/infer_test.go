package status

import "testing"

func TestInferSyncStateAddsNTPUnit(t *testing.T) {
	r := &Report{
		NTPHealth: true,
		PTPHealth: "false",
		Chrony: ChronyStatus{
			Active: true,
			Source: "3",
			Offset: "-1.462872982",
		},
	}
	role, source, offset := inferSyncState(r)
	if role != "ntp" {
		t.Fatalf("role = %q, want ntp", role)
	}
	if source != "3" {
		t.Fatalf("source = %q, want 3", source)
	}
	if offset != "-1.462872982 s" {
		t.Fatalf("offset = %q, want seconds unit", offset)
	}
}

func TestInferSyncStatePrefersHealthyPTP(t *testing.T) {
	r := &Report{
		NTPHealth: true,
		PTPHealth: "true",
		Chrony: ChronyStatus{
			Active: true,
			Source: "3",
			Offset: "-1.462872982",
		},
		PTP: PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
		},
	}
	role, source, offset := inferSyncState(r)
	if role != "ptp" {
		t.Fatalf("role = %q, want ptp", role)
	}
	if source != "SLAVE" {
		t.Fatalf("source = %q, want SLAVE", source)
	}
	if offset != "0.000042 ms" {
		t.Fatalf("offset = %q, want PTP offset", offset)
	}
}

func TestInferSyncStateKeepsNTPWhenPTPDoesNotDisciplineSystem(t *testing.T) {
	r := &Report{
		NTPHealth: true,
		PTPHealth: "true",
		Chrony: ChronyStatus{
			Active: true,
			Source: "3",
			Offset: "-1.462872982",
		},
		PTP: PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: false,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
		},
	}
	role, source, offset := inferSyncState(r)
	if role != "ntp" {
		t.Fatalf("role = %q, want ntp", role)
	}
	if source != "3" {
		t.Fatalf("source = %q, want 3", source)
	}
	if offset != "-1.462872982 s" {
		t.Fatalf("offset = %q, want NTP offset", offset)
	}
}

func TestInferPTPHealthRejectsLargeMasterOffset(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PHC2SysActive: true,
		Available:     true,
		PortState:     "SLAVE",
		MasterOffset:  "-1783161116251737041",
	}, "")
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferPTPHealthAcceptsSmallMasterOffset(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PHC2SysActive: true,
		Available:     true,
		PortState:     "SLAVE",
		MasterOffset:  "-1075814",
	}, "")
	if health != "true" {
		t.Fatalf("health = %q, want true", health)
	}
}

func TestInferPTPHealthRejectsClientMasterState(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PHC2SysActive: true,
		Available:     true,
		PortState:     "MASTER",
		MasterOffset:  "42",
	}, "client")
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferPTPHealthAcceptsConfiguredMasterStateWithoutOffset(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PHC2SysActive: true,
		Available:     true,
		PortState:     "MASTER",
	}, "master")
	if health != "true" {
		t.Fatalf("health = %q, want true", health)
	}
}

func TestInferPTPHealthRejectsConfiguredMasterSlaveState(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PHC2SysActive: true,
		Available:     true,
		PortState:     "SLAVE",
		MasterOffset:  "42",
	}, "master")
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferPTPHealthUnknownWithoutOffset(t *testing.T) {
	health := inferPTPHealth(PTPStatus{
		PTP4LActive:   true,
		PHC2SysActive: true,
		Available:     true,
		PortState:     "SLAVE",
	}, "")
	if health != "unknown" {
		t.Fatalf("health = %q, want unknown", health)
	}
}

func TestInferClockHealthRejectsEpochSystemClock(t *testing.T) {
	health := inferClockHealth(ClockStatus{
		SystemUnix: 1038,
		RTCUnix:    1783162152,
	})
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferClockHealthRejectsBadRTC(t *testing.T) {
	health := inferClockHealth(ClockStatus{
		SystemUnix: 1783162152,
		RTCUnix:    1038,
	})
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferClockHealthRejectsLargePHCSkew(t *testing.T) {
	health := inferClockHealth(ClockStatus{
		SystemUnix: 1783162152,
		RTCUnix:    1783162152,
		PHCUnix:    1038,
	})
	if health != "false" {
		t.Fatalf("health = %q, want false", health)
	}
}

func TestInferClockHealthAllowsTaiOffsetPHCSkew(t *testing.T) {
	health := inferClockHealth(ClockStatus{
		SystemUnix: 1783162152,
		RTCUnix:    1783162153,
		PHCUnix:    1783162189,
	})
	if health != "true" {
		t.Fatalf("health = %q, want true", health)
	}
}

func TestInferClockHealthUnknownWhenClockDetailPresent(t *testing.T) {
	health := inferClockHealth(ClockStatus{
		SystemUnix: 1783162152,
		RTCUnix:    1783162152,
		Detail:     "unable to read PHC",
	})
	if health != "unknown" {
		t.Fatalf("health = %q, want unknown", health)
	}
}

func TestInferOverallHealthRejectsPTPWithoutPHC2Sys(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		PTPHealth:   "true",
		ClockHealth: "true",
		PTP: PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: false,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
		},
	})
	if healthy {
		t.Fatal("healthy = true, want false")
	}
}

func TestInferOverallHealthAcceptsPTPWithPHC2Sys(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		PTPHealth:   "true",
		ClockHealth: "true",
		PTP: PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
		},
	})
	if !healthy {
		t.Fatal("healthy = false, want true")
	}
}

func TestInferOverallHealthAcceptsNTPWithoutPHC2Sys(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		NTPHealth:   true,
		PTPHealth:   "false",
		ClockHealth: "true",
		PTP: PTPStatus{
			PHC2SysActive: false,
		},
	})
	if !healthy {
		t.Fatal("healthy = false, want true")
	}
}

func TestInferOverallHealthRejectsConfiguredPTPClientNTPMasking(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		ConfiguredRole: "client",
		ConfiguredPTP:  true,
		NTPHealth:      true,
		PTPHealth:      "false",
		ClockHealth:    "true",
		PTP: PTPStatus{
			PHC2SysActive: false,
		},
	})
	if healthy {
		t.Fatal("healthy = true, want false")
	}
}

func TestInferOverallHealthRejectsConfiguredPTPMasterWithoutPHC2Sys(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		ConfiguredRole: "master",
		ConfiguredPTP:  true,
		NTPHealth:      true,
		PTPHealth:      "true",
		ClockHealth:    "true",
		PTP: PTPStatus{
			PHC2SysActive: false,
		},
	})
	if healthy {
		t.Fatal("healthy = true, want false")
	}
}

func TestInferOverallHealthAcceptsConfiguredPTPMasterWithPHC2Sys(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		ConfiguredRole: "master",
		ConfiguredPTP:  true,
		NTPHealth:      true,
		PTPHealth:      "true",
		ClockHealth:    "true",
		PTP: PTPStatus{
			PHC2SysActive: true,
			PortState:     "MASTER",
		},
	})
	if !healthy {
		t.Fatal("healthy = false, want true")
	}
}

func TestInferOverallHealthAcceptsAutoPTPWithNTPDiscipline(t *testing.T) {
	healthy := inferOverallHealth(&Report{
		ConfiguredRole: "auto",
		ConfiguredPTP:  true,
		NTPHealth:      true,
		PTPHealth:      "unknown",
		ClockHealth:    "true",
	})
	if !healthy {
		t.Fatal("healthy = false, want true")
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
}
