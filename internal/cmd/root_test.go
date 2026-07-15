package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestSetVersion(t *testing.T) {
	previous := rootCmd.Version
	t.Cleanup(func() { rootCmd.Version = previous })
	SetVersion("v1.2.0")
	if rootCmd.Version != "v1.2.0" {
		t.Fatalf("version = %q", rootCmd.Version)
	}
}

func TestPTPReadyForPHC2SysAcceptsPTPHealthyClockUnhealthy(t *testing.T) {
	if !ptpReadyForPHC2Sys(&status.Report{
		PTPHealth:   "true",
		ClockHealth: "false",
	}) {
		t.Fatal("PTP-ready report rejected")
	}
}

func TestRenderStatusOutputAutoPreservesPlainPipes(t *testing.T) {
	report := &status.Report{Role: "ntp", Source: "pool.ntp.org", NTPHealth: true, Healthy: true}
	output, err := renderStatusOutput(report, "auto", false, false, 76)
	if err != nil {
		t.Fatal(err)
	}
	if output != report.Summary() {
		t.Fatalf("auto pipe output changed plain contract:\n%s", output)
	}
}

func TestRenderStatusOutputAutoUsesFancyTTY(t *testing.T) {
	report := &status.Report{Role: "ntp", Source: "pool.ntp.org", NTPHealth: true, Healthy: true}
	output, err := renderStatusOutput(report, "auto", true, false, 76)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "TIMESYNC STATUS") {
		t.Fatalf("auto TTY output missing dashboard:\n%s", output)
	}
}

func TestRenderStatusOutputExplicitFormatsOverrideTerminal(t *testing.T) {
	report := &status.Report{Role: "ntp", Source: "pool.ntp.org", NTPHealth: true, Healthy: true}
	plain, err := renderStatusOutput(report, "plain", true, true, 76)
	if err != nil {
		t.Fatal(err)
	}
	if plain != report.Summary() {
		t.Fatalf("explicit plain output changed contract:\n%s", plain)
	}

	fancy, err := renderStatusOutput(report, "fancy", false, false, 76)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fancy, "TIMESYNC STATUS") {
		t.Fatalf("explicit fancy output missing dashboard:\n%s", fancy)
	}
}

func TestRenderStatusOutputJSON(t *testing.T) {
	report := &status.Report{
		SchemaVersion:     "1.2",
		Role:              "ptp",
		Source:            "aabbcc.fffe.112233",
		Healthy:           true,
		SystemClockSource: "aabbcc.fffe.112233",
		Clock:             status.ClockStatus{PHCUnix: 1784102437, PHCUTCUnix: 1784102400},
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthDisabled,
			PTPLink:     status.HealthHealthy,
			PTPAccuracy: status.HealthHealthy,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthHealthy,
		},
	}
	output, err := renderStatusOutput(report, "json", false, false, 76)
	if err != nil {
		t.Fatal(err)
	}
	var decoded status.Report
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("JSON output decode: %v\n%s", err, output)
	}
	if decoded.Role != "ptp" || decoded.Source != "aabbcc.fffe.112233" || !decoded.Healthy {
		t.Fatalf("legacy JSON fields = %+v", decoded)
	}
	if decoded.SchemaVersion != "1.2" || decoded.Health.Overall != status.HealthHealthy || decoded.Health.PTPAccuracy != status.HealthHealthy {
		t.Fatalf("decoded report = %+v", decoded)
	}
	if decoded.SystemClockSource != "aabbcc.fffe.112233" {
		t.Fatalf("system clock source = %q", decoded.SystemClockSource)
	}
	if decoded.Clock.PHCUnix != 1784102437 || decoded.Clock.PHCUTCUnix != 1784102400 {
		t.Fatalf("clock JSON = %+v", decoded.Clock)
	}
}

func TestRenderStatusOutputRejectsUnknownFormat(t *testing.T) {
	_, err := renderStatusOutput(&status.Report{}, "yaml", false, false, 76)
	if err == nil || !strings.Contains(err.Error(), "auto, fancy, plain, or json") {
		t.Fatalf("unexpected error: %v", err)
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
