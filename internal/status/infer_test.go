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
