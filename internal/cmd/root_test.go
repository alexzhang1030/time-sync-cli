package cmd

import (
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestPTPReadyForPHC2SysAcceptsPTPHealthyClockUnhealthy(t *testing.T) {
	if !ptpReadyForPHC2Sys(&status.Report{
		PTPHealth:   "true",
		ClockHealth: "false",
	}) {
		t.Fatal("PTP-ready report rejected")
	}
}

func TestPTPReadyForPHC2SysRejectsPTPUnhealthy(t *testing.T) {
	if ptpReadyForPHC2Sys(&status.Report{
		PTPHealth:   "false",
		ClockHealth: "true",
	}) {
		t.Fatal("PTP-unhealthy report accepted")
	}
}
