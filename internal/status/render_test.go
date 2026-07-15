package status_test

import (
	"strings"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestFancySummaryShowsRoleAwareOperationalStatus(t *testing.T) {
	residual := int64(-42000)
	report := &status.Report{
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
			NTP:         status.HealthInactive,
			PTPLink:     status.HealthHealthy,
			PTPAccuracy: status.HealthHealthy,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthHealthy,
		},
		Systemd: status.SystemdStatus{
			NTPDaemon: status.UnitStatus{Unit: "chrony.service", ActiveState: "inactive", UnitFileState: "enabled"},
			PTP4LUnit: status.UnitStatus{Unit: "ptp4l.service", ActiveState: "active", UnitFileState: "enabled"},
			PHC2SysUnit: status.UnitStatus{
				Unit: "phc2sys.service", ActiveState: "active", UnitFileState: "enabled",
			},
			GuardTimer: status.UnitStatus{
				Unit: "timesync-ptp-guard.timer", ActiveState: "active", UnitFileState: "enabled",
			},
		},
		Clock: status.ClockStatus{
			SystemUnix:        1784102400,
			RTCUnix:           1784102399,
			PHCUnix:           1784102437,
			PHCUTCUnix:        1784102400,
			Iface:             "eth0",
			RTCSystemSkew:     "-1 s",
			PHCSystemSkew:     "-37 s",
			PHCResidualNS:     &residual,
			PHCResidual:       "-42.000 µs",
			PHCTimeScale:      "TAI",
			TAIUTCOffset:      37,
			TAIUTCOffsetValid: true,
		},
		PTP: status.PTPStatus{
			PTP4LActive:   true,
			PHC2SysActive: true,
			Available:     true,
			PortState:     "SLAVE",
			MasterOffset:  "42",
			PathDelay:     "2500",
			StepsRemoved:  "1",
			GMIdentity:    "aabbcc.fffe.112233",
		},
	}

	output := report.FancySummary(status.RenderOptions{Width: 76})
	for _, expected := range []string{
		"TIMESYNC STATUS",
		"● HEALTHY",
		"managed · client · PTP enabled",
		"System source",
		"aabbcc.fffe.112233",
		"Clock flow",
		"PTP grandmaster → PHC → System",
		"PTP link",
		"PTP accuracy",
		"Runtime guard",
		"timesync-ptp-guard.timer",
		"Offset from GM",
		"Mean path delay",
		"GM hops",
		"TAI–UTC offset",
		"+37 s",
		"PHC raw (eth0)",
		"2026-07-15 08:00:37 TAI",
		"PHC as UTC",
		"2026-07-15 08:00:00 UTC",
		"PHC residual",
		"-42.000 µs · System − PHC(UTC)",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("FancySummary missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "PHC skew") {
		t.Fatalf("FancySummary exposes the raw cross-timescale PHC skew:\n%s", output)
	}
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("color-disabled FancySummary contains ANSI escapes:\n%q", output)
	}
}

func TestFancySummaryShowsDisabledPTPForNTPConfig(t *testing.T) {
	report := &status.Report{
		ManagementState: "managed",
		ConfiguredRole:  "auto",
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthHealthy,
			PTPLink:     status.HealthDisabled,
			PTPAccuracy: status.HealthDisabled,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthDisabled,
		},
		Role: "ntp",
	}

	output := report.FancySummary(status.RenderOptions{Width: 76})
	if !strings.Contains(output, "PTP link          ○ disabled") {
		t.Fatalf("FancySummary missing disabled PTP state:\n%s", output)
	}
}

func TestFancySummaryShowsUnmanagedHostAndNextSteps(t *testing.T) {
	output := (&status.Report{ManagementState: "unmanaged"}).FancySummary(status.RenderOptions{Width: 76})
	for _, expected := range []string{"UNMANAGED", "observed state", "NEXT STEP", "timesync doctor", "sudo timesync tui"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("FancySummary missing %q:\n%s", expected, output)
		}
	}
}

func TestFancySummaryKeepsOptionalPTPWarningSeparateFromOverallHealth(t *testing.T) {
	report := &status.Report{
		ManagementState: "managed",
		ConfiguredRole:  "auto",
		ConfiguredPTP:   true,
		Warnings:        []string{"optional PTP monitor is unknown"},
		Health: status.HealthSummary{
			Overall:     status.HealthHealthy,
			NTP:         status.HealthHealthy,
			PTPLink:     status.HealthUnknown,
			PTPAccuracy: status.HealthUnknown,
			Clock:       status.HealthHealthy,
			Discipline:  status.HealthHealthy,
			Guard:       status.HealthDisabled,
		},
	}

	output := report.FancySummary(status.RenderOptions{Width: 76})
	for _, expected := range []string{"HEALTHY · 1 WARN", "PTP link          ◆ UNKNOWN", "WARNINGS", "optional PTP monitor is unknown"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("FancySummary missing %q:\n%s", expected, output)
		}
	}
}

func TestFancySummarySurfacesManagementQueryDetail(t *testing.T) {
	report := &status.Report{
		ManagementState:  "error",
		ManagementDetail: "permission denied",
		Health:           status.HealthSummary{Overall: status.HealthUnknown},
	}

	output := report.FancySummary(status.RenderOptions{Width: 76})
	for _, expected := range []string{"UNKNOWN", "management state error", "DIAGNOSTICS", "permission denied"} {
		if !strings.Contains(output, expected) {
			t.Errorf("FancySummary missing %q:\n%s", expected, output)
		}
	}
}
