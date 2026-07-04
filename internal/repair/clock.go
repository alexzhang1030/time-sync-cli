package repair

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
)

const (
	DefaultRTCPath      = "/sys/class/rtc/rtc0/since_epoch"
	minTrustedRTCEpoch  = 946684800 // 2000-01-01T00:00:00Z
	maxSystemRTCSkewSec = 3600
)

var getEUID = os.Geteuid
var now = time.Now

// Runner executes system commands.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Options controls clock repair.
type Options struct {
	Iface                     string
	ConfigDir                 string
	RTCPath                   string
	RepairSystemClock         bool
	RequireTrustedSystemClock bool
	Runner                    Runner
}

// Result describes the repair steps that ran.
type Result struct {
	Iface    string
	RTCEpoch int64
	Steps    []Step
}

// Step records one executed repair command.
type Step struct {
	Command string
}

// Clock restores the system clock from RTC, copies it into the interface PHC,
// then restarts linuxptp services in dependency order.
func Clock(opts Options) (*Result, error) {
	if getEUID() != 0 {
		return nil, fmt.Errorf("repair-clock requires root; re-run with sudo")
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	rtcPath := opts.RTCPath
	if rtcPath == "" {
		rtcPath = DefaultRTCPath
	}
	iface, err := resolveIface(opts)
	if err != nil {
		return nil, err
	}
	rtcEpoch, err := readRTCEpoch(rtcPath)
	if err != nil {
		return nil, err
	}

	result := &Result{Iface: iface, RTCEpoch: rtcEpoch}
	if err := runStep(runner, result, "systemctl", "stop", "phc2sys", "ptp4l"); err != nil {
		return nil, err
	}
	if err := runStep(runner, result, "date", "-u", "-s", "@"+strconv.FormatInt(rtcEpoch, 10)); err != nil {
		return nil, err
	}
	if err := runStep(runner, result, "phc_ctl", iface, "set"); err != nil {
		return nil, err
	}
	if err := runStep(runner, result, "systemctl", "start", "ptp4l"); err != nil {
		return nil, err
	}
	if err := runStep(runner, result, "systemctl", "start", "phc2sys"); err != nil {
		return nil, err
	}
	return result, nil
}

// BootGuard primes clocks before ptp4l starts. It corrects epoch system time
// from RTC, then initializes the interface PHC from the system clock.
func BootGuard(opts Options) (*Result, error) {
	if getEUID() != 0 {
		return nil, fmt.Errorf("boot-guard requires root; re-run with sudo")
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	rtcPath := opts.RTCPath
	if rtcPath == "" {
		rtcPath = DefaultRTCPath
	}
	iface, err := resolveIface(opts)
	if err != nil {
		return nil, err
	}

	result := &Result{Iface: iface}
	if opts.RepairSystemClock || opts.RequireTrustedSystemClock {
		rtcEpoch, err := readRTCEpoch(rtcPath)
		if err != nil {
			return nil, err
		}
		result.RTCEpoch = rtcEpoch
		systemEpoch := now().Unix()
		needsRTC := systemEpoch < minTrustedRTCEpoch
		if opts.RepairSystemClock && needsRTC {
			if err := runStep(runner, result, "date", "-u", "-s", "@"+strconv.FormatInt(rtcEpoch, 10)); err != nil {
				return nil, err
			}
		}
		if opts.RequireTrustedSystemClock && needsRTC {
			return nil, fmt.Errorf("system clock is not trusted enough to start PTP service; run sudo timesync repair-clock or restore a trusted time source")
		}
		if systemEpoch >= minTrustedRTCEpoch && absInt64(systemEpoch-rtcEpoch) > maxSystemRTCSkewSec {
			return nil, fmt.Errorf("system clock and RTC differ by more than %d seconds; restore a trusted time source before starting PTP", maxSystemRTCSkewSec)
		}
	}
	if err := runStep(runner, result, "phc_ctl", iface, "set"); err != nil {
		return nil, err
	}
	return result, nil
}

func resolveIface(opts Options) (string, error) {
	if strings.TrimSpace(opts.Iface) != "" {
		return strings.TrimSpace(opts.Iface), nil
	}
	state, err := apply.LoadState(opts.ConfigDir)
	if err != nil {
		return "", fmt.Errorf("load timesync state: %w; pass --iface explicitly", err)
	}
	if strings.TrimSpace(state.Iface) == "" {
		return "", fmt.Errorf("timesync state has no interface; pass --iface explicitly")
	}
	return strings.TrimSpace(state.Iface), nil
}

func readRTCEpoch(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read RTC epoch %s: %w", path, err)
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse RTC epoch %s: %w", path, err)
	}
	if epoch < minTrustedRTCEpoch {
		return 0, fmt.Errorf("RTC epoch %d is before 2000-01-01; set a trusted time source first", epoch)
	}
	return epoch, nil
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func runStep(runner Runner, result *Result, name string, args ...string) error {
	out, err := runner.Run(name, args...)
	command := strings.Join(append([]string{name}, args...), " ")
	if err != nil {
		return fmt.Errorf("%s: %w: %s", command, err, strings.TrimSpace(string(out)))
	}
	result.Steps = append(result.Steps, Step{Command: command})
	return nil
}
