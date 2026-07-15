package status_test

import (
	"strings"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestFancySummaryShowsOperationalStatus(t *testing.T) {
	report := &status.Report{
		Healthy:        true,
		NTPHealth:      false,
		PTPHealth:      "true",
		ClockHealth:    "true",
		ConfiguredRole: "client",
		ConfiguredPTP:  true,
		Role:           "ptp",
		Source:         "SLAVE",
		Offset:         "0.000042 ms",
		Systemd: status.SystemdStatus{
			Chronyd: "inactive",
			PTP4L:   "active",
			PHC2Sys: "active",
		},
		Clock: status.ClockStatus{
			SystemUnix:    1784102400,
			RTCUnix:       1784102399,
			PHCUnix:       1784102400,
			Iface:         "eth0",
			RTCSystemSkew: "-1 s",
			PHCSystemSkew: "0 s",
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
		"HEALTHY",
		"inactive",
		"client · PTP enabled",
		"Active role",
		"SLAVE",
		"SERVICES",
		"phc2sys",
		"PTP offset",
		"0.000042 ms",
		"CLOCKS",
		"PHC (eth0)",
		"2026-07-15 08:00:00 UTC",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("FancySummary missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("color-disabled FancySummary contains ANSI escapes:\n%q", output)
	}
}

func TestFancySummaryShowsDisabledPTPForNTPConfig(t *testing.T) {
	report := &status.Report{
		Healthy:        true,
		NTPHealth:      true,
		PTPHealth:      "false",
		ClockHealth:    "true",
		ConfiguredRole: "auto",
		Role:           "ntp",
	}

	output := report.FancySummary(status.RenderOptions{Width: 76})
	if !strings.Contains(output, "disabled") {
		t.Fatalf("FancySummary missing disabled PTP state:\n%s", output)
	}
}

func TestFancySummaryTreatsMissingHealthAsUnknown(t *testing.T) {
	output := (&status.Report{}).FancySummary(status.RenderOptions{Width: 76})
	if !strings.Contains(output, "UNKNOWN") {
		t.Fatalf("FancySummary missing unknown health state:\n%s", output)
	}
}

func TestFancySummarySurfacesUnknownPTPDetail(t *testing.T) {
	report := &status.Report{
		Healthy:     false,
		NTPHealth:   true,
		PTPHealth:   "unknown",
		ClockHealth: "false",
		Role:        "ntp",
		PTP: status.PTPStatus{
			Detail: "unable to query ptp4l via pmc",
		},
	}

	output := report.FancySummary(status.RenderOptions{Width: 76})
	for _, expected := range []string{
		"NEEDS ATTENTION",
		"UNKNOWN",
		"DIAGNOSTICS",
		"unable to query ptp4l via pmc",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("FancySummary missing %q:\n%s", expected, output)
		}
	}
}
