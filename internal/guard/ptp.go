package guard

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

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
	Runner    Runner
	RTCWriter RTCWriter
	Collect   func() (*status.Report, error)
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
