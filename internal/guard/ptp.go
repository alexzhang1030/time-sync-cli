package guard

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/gm"
	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

const (
	maxTrustedPHCSystemSkewSec = 120
	maxTrustedPHCResidualNS    = int64(time.Second)
	minRTCWriteSkewSec         = 5
)

// Runner executes system commands.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

// RTCWriter writes a trusted system time into the hardware RTC.
type RTCWriter interface {
	WriteSystemTime(t time.Time) error
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Options controls the PTP runtime guard.
type Options struct {
	Runner     Runner
	RTCWriter  RTCWriter
	Collect    func() (*status.Report, error)
	PublishGM  func(iface string) error
	ConfigPath string // passed through to WaitForPHCAlignment / Publish; empty falls back to Default
}

// Result records what the runtime guard observed and changed.
type Result struct {
	PTPHealth      string
	ClockHealth    string
	ConfiguredRole string
	ConfiguredPTP  bool
	PortState      string
	MasterOffset   string
	PHC2SysActive  bool
	Action         string
	Reason         string
}

// PTPOnce keeps PHC-to-system sync aligned with current health.
func PTPOnce(opts Options) (*Result, error) {
	collect := opts.Collect
	if collect == nil {
		collect = status.Collect
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	rtcWriter := opts.RTCWriter
	if rtcWriter == nil {
		rtcWriter = linuxRTCWriter{}
	}

	report, err := collect()
	if err != nil {
		return nil, err
	}
	result := &Result{
		PTPHealth:      report.PTPHealth,
		ClockHealth:    report.ClockHealth,
		ConfiguredRole: report.ConfiguredRole,
		ConfiguredPTP:  report.ConfiguredPTP,
		PortState:      report.PTP.PortState,
		MasterOffset:   report.PTP.MasterOffset,
		PHC2SysActive:  report.PTP.PHC2SysActive,
		Action:         "none",
	}
	result.Reason = fmt.Sprintf("ptp=%s clock=%s configured_role=%s configured_ptp=%v port_state=%s master_offset=%s",
		report.PTPHealth,
		report.ClockHealth,
		report.ConfiguredRole,
		report.ConfiguredPTP,
		report.PTP.PortState,
		report.PTP.MasterOffset,
	)

	if shouldPublishGMTimeProperties(report) {
		iface := report.Clock.Iface

		// For masters: phc2sys is THE component that disciplines PHC from
		// CLOCK_REALTIME to TAI. The guard's job is to make sure phc2sys
		// is running and has aligned, then publish the valid bit.
		// This provides the "开机就自动" recovery: even after epoch, bad
		// boots, or system time jumps, the periodic guard will fix it
		// without user intervention.
		var startedPHC2Sys, restartedPHC2Sys bool
		// Clean up any stale failed state from a 1970-era boot.
		if err := run(runner, "systemctl", "reset-failed", "phc2sys", "ptp4l"); err != nil {
			return result, fmt.Errorf("reset failed systemd units: %w", err)
		}

		// Ensure chrony is providing trusted system time (masters rely on it).
		if err := run(runner, "systemctl", "start", "chrony"); err != nil {
			return result, fmt.Errorf("start chrony for trusted system time: %w", err)
		}

		if !report.PTP.PHC2SysActive {
			if err := run(runner, "systemctl", "start", "phc2sys"); err != nil {
				return result, err
			}
			startedPHC2Sys = true
		} else if !report.PTP.CurrentUTCOffsetValid {
			// phc2sys is running but valid bit is not set. This often
			// means it aligned to an old system time. Force a restart
			// so it re-evaluates current system and steps the PHC
			// (thanks to -S 1.0).
			if err := run(runner, "systemctl", "restart", "phc2sys"); err != nil {
				return result, err
			}
			restartedPHC2Sys = true
		}

		// Wait (up to ~20s) for phc2sys to make PHC match system + offset.
		// This is the key "兜底" that makes boot automatic.
		if err := gm.WaitForPHCAlignment(runner, opts.ConfigPath, iface, 20*time.Second); err != nil {
			return result, fmt.Errorf("wait for PHC alignment: %w", err)
		}

		publishGM := opts.PublishGM
		if publishGM == nil {
			publishGM = func(iface string) error {
				_, err := gm.Publish(gm.Options{Iface: iface, ConfigPath: opts.ConfigPath})
				return err
			}
		}
		if err := publishGM(iface); err != nil {
			// Propagate publish error; do not swallow (fixes guard contract).
			if startedPHC2Sys {
				result.Action = "start phc2sys + publish gm time properties failed"
			} else if restartedPHC2Sys {
				result.Action = "restart phc2sys + publish gm time properties failed"
			} else {
				result.Action = "publish gm time properties failed"
			}
			return result, err
		}
		if startedPHC2Sys {
			result.Action = "start phc2sys + publish gm time properties"
		} else if restartedPHC2Sys {
			result.Action = "restart phc2sys + publish gm time properties"
		} else {
			result.Action = "publish gm time properties"
		}
		// The master recovery path is self-contained: it already started/
		// restarted phc2sys, waited for alignment, and published. Returning
		// here prevents the general guard switch below from overwriting the
		// action (e.g. with a redundant "start phc2sys" when PHC2SysActive was
		// false before we started it).
		return result, nil
	}

	if !guardApplies(report) {
		if report.PTP.PHC2SysActive {
			if err := run(runner, "systemctl", "stop", "phc2sys"); err != nil {
				return result, err
			}
			result.Action = "stop phc2sys outside configured ptp role"
			return result, nil
		}
		result.Action = "hold phc2sys stopped outside configured ptp role"
		return result, nil
	}

	ptpHealthy := report.PTPHealth == "true"
	switch {
	case ptpHealthy && !report.PTP.PHC2SysActive:
		if err := run(runner, "systemctl", "start", "phc2sys"); err != nil {
			return result, err
		}
		result.Action = "start phc2sys"
		return result, nil
	case ptpHealthy:
		if rtcWriteNeeded(report) {
			if err := rtcWriter.WriteSystemTime(time.Unix(report.Clock.SystemUnix, 0).UTC()); err != nil {
				return result, err
			}
			result.Action = "sync rtc"
		}
		return result, nil
	case report.PTP.PHC2SysActive:
		if err := run(runner, "systemctl", "stop", "phc2sys"); err != nil {
			return result, err
		}
		result.Action = "stop phc2sys"
		return result, nil
	default:
		result.Action = "hold phc2sys stopped"
		return result, nil
	}
}

func shouldPublishGMTimeProperties(report *status.Report) bool {
	return report != nil &&
		report.ConfiguredPTP &&
		strings.EqualFold(report.ConfiguredRole, "master") &&
		report.PTP.PTP4LActive &&
		report.PTP.TimePropertiesAvailable &&
		!report.PTP.CurrentUTCOffsetValid
}

func guardApplies(report *status.Report) bool {
	if report == nil || !report.ConfiguredPTP {
		return false
	}
	return strings.EqualFold(report.ConfiguredRole, "client") || strings.EqualFold(report.ConfiguredRole, "master")
}

func rtcWriteNeeded(report *status.Report) bool {
	if !report.PTP.PHC2SysActive {
		return false
	}
	if report.Clock.SystemUnix <= 0 || report.Clock.PHCUnix <= 0 || report.Clock.RTCUnix <= 0 {
		return false
	}
	if report.Clock.PHCResidualNS != nil {
		if absInt64(*report.Clock.PHCResidualNS) > maxTrustedPHCResidualNS {
			return false
		}
	} else if absInt64(report.Clock.SystemUnix-report.Clock.PHCUnix) > maxTrustedPHCSystemSkewSec {
		return false
	}
	return absInt64(report.Clock.SystemUnix-report.Clock.RTCUnix) > minRTCWriteSkewSec
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func run(runner Runner, name string, args ...string) error {
	out, err := runner.Run(name, args...)
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(append([]string{name}, args...), " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
