package gm

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

const DefaultConfigPath = "/etc/timesync-cli/ptp4l.conf"

const maxPHCTAIResidual = time.Second

// Runner executes linuxptp management commands.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Options controls publication of the grandmaster time properties data set.
type Options struct {
	Runner     Runner
	ConfigPath string
	Iface      string
	Timeout    time.Duration
	Interval   time.Duration
}

// Result records the verified UTC offset advertised by the grandmaster.
type Result struct {
	UTCOffset int
}

// Publish marks the generated grandmaster UTC offset as valid through ptp4l's
// management socket. Existing clock quality and traceability fields are kept.
func Publish(opts Options) (*Result, error) {
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = DefaultConfigPath
	}
	offset, err := utcOffsetFromConfig(configPath)
	if err != nil {
		return nil, err
	}
	runner := opts.Runner
	if runner == nil {
		runner = execRunner{}
	}
	iface := strings.TrimSpace(opts.Iface)
	if iface == "" {
		return nil, fmt.Errorf("PTP interface is required to verify the PHC time scale")
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = time.Second
	}
	deadline := time.Now().Add(opts.Timeout)

	var lastErr error
	for {
		if err := publishOnce(runner, iface, offset); err == nil {
			return &Result{UTCOffset: offset}, nil
		} else {
			lastErr = err
		}
		if opts.Timeout <= 0 || !time.Now().Before(deadline) {
			return nil, fmt.Errorf("publish PTP grandmaster time properties: %w", lastErr)
		}
		time.Sleep(interval)
	}
}

func publishOnce(runner Runner, iface string, offset int) error {
	if err := verifyPHCTimescale(runner, iface, offset); err != nil {
		return err
	}
	current, err := runPMC(runner, "GET GRANDMASTER_SETTINGS_NP")
	if err != nil {
		return err
	}
	setCommand, err := buildSetCommand(status.ParsePMCFields(string(current)), offset)
	if err != nil {
		return err
	}
	if _, err := runPMC(runner, setCommand); err != nil {
		return err
	}

	verified, err := runPMC(runner, "GET GRANDMASTER_SETTINGS_NP")
	if err != nil {
		return err
	}
	fields := status.ParsePMCFields(string(verified))
	if fields["currentUtcOffsetValid"] != "1" {
		return fmt.Errorf("verification returned currentUtcOffsetValid=%s", valueOr(fields["currentUtcOffsetValid"], "missing"))
	}
	gotOffset, err := strconv.Atoi(fields["currentUtcOffset"])
	if err != nil {
		return fmt.Errorf("verification returned currentUtcOffset=%q", fields["currentUtcOffset"])
	}
	if gotOffset != offset {
		return fmt.Errorf("verification returned currentUtcOffset=%d, want %d", gotOffset, offset)
	}
	return nil
}

func verifyPHCTimescale(runner Runner, iface string, offset int) error {
	residual, err := samplePHCResidual(runner, iface, offset)
	if err != nil {
		return err
	}
	if absInt64(residual) > int64(maxPHCTAIResidual) {
		return fmt.Errorf("PHC TAI residual %s exceeds %s while phc2sys converges", time.Duration(residual), maxPHCTAIResidual)
	}
	return nil
}

// samplePHCResidual returns (PHC time - (system time + offset)) in nanoseconds.
// Positive means PHC is ahead of expected TAI.
func samplePHCResidual(runner Runner, iface string, offset int) (int64, error) {
	before := time.Now()
	out, err := runner.Run("phc_ctl", iface, "get")
	after := time.Now()
	if err != nil {
		return 0, fmt.Errorf("phc_ctl %s get: %w: %s", iface, err, strings.TrimSpace(string(out)))
	}
	phcNS, err := status.ParsePHCTimeNS(string(out))
	if err != nil {
		return 0, fmt.Errorf("parse PHC time: %w", err)
	}
	systemMidpointNS := before.UnixNano() + after.Sub(before).Nanoseconds()/2
	expectedPHCNS := systemMidpointNS + int64(offset)*int64(time.Second)
	return phcNS - expectedPHCNS, nil
}

func buildSetCommand(fields map[string]string, offset int) (string, error) {
	keys := []string{
		"clockClass",
		"clockAccuracy",
		"offsetScaledLogVariance",
		"currentUtcOffset",
		"leap61",
		"leap59",
		"currentUtcOffsetValid",
		"ptpTimescale",
		"timeTraceable",
		"frequencyTraceable",
		"timeSource",
	}
	for _, key := range keys {
		if strings.TrimSpace(fields[key]) == "" {
			return "", fmt.Errorf("GRANDMASTER_SETTINGS_NP missing %s", key)
		}
	}
	fields["currentUtcOffset"] = strconv.Itoa(offset)
	fields["currentUtcOffsetValid"] = "1"

	var parts []string
	for _, key := range keys {
		parts = append(parts, key, fields[key])
	}
	return "SET GRANDMASTER_SETTINGS_NP " + strings.Join(parts, " "), nil
}

func runPMC(runner Runner, command string) ([]byte, error) {
	out, err := runner.Run("pmc", "-u", "-b", "0", command)
	if err != nil {
		return nil, fmt.Errorf("pmc %q: %w: %s", command, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func utcOffsetFromConfig(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("read PTP config %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "utc_offset" {
			continue
		}
		offset, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("parse utc_offset %q in %s: %w", fields[1], path, err)
		}
		return offset, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read PTP config %s: %w", path, err)
	}
	return 0, fmt.Errorf("PTP config %s is missing utc_offset; re-apply the master role", path)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

// ReadUTCOffset reads the utc_offset from the ptp4l configuration file.
// It is intended for use by repair-clock and other components that need
// to seed the PHC in the correct timescale for a PTP master.
func ReadUTCOffset(path string) (int, error) {
	if path == "" {
		path = DefaultConfigPath
	}
	return utcOffsetFromConfig(path)
}

// WaitForPHCAlignment polls until the PHC on iface is within the 1s tolerance
// of (current system time + utc_offset from config). This is the proper
// precondition for a master to publish valid TAI time properties.
//
// The caller is responsible for ensuring phc2sys is running (the component
// that actually disciplines PHC from CLOCK_REALTIME on a master). This
// function only observes and waits.
//
// If configPath is empty, DefaultConfigPath is used.
func WaitForPHCAlignment(runner Runner, configPath string, iface string, timeout time.Duration) error {
	if runner == nil {
		runner = execRunner{}
	}
	offset, err := ReadUTCOffset(configPath)
	if err != nil {
		return fmt.Errorf("read utc_offset for alignment wait: %w", err)
	}
	deadline := time.Now().Add(timeout)
	interval := time.Second
	consecutiveErrors := 0
	var lastErr error
	for {
		residual, sampleErr := samplePHCResidual(runner, iface, offset)
		if sampleErr == nil && absInt64(residual) <= int64(maxPHCTAIResidual) {
			return nil
		}
		if sampleErr != nil {
			lastErr = sampleErr
		} else {
			lastErr = fmt.Errorf("residual %s exceeds 1s", time.Duration(residual))
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out waiting for PHC alignment: %w", lastErr)
		}
		if sampleErr != nil {
			consecutiveErrors++
			if consecutiveErrors >= 5 {
				// Early exit on persistent phc_ctl / sampling errors instead of
				// polling the full timeout.
				return fmt.Errorf("persistent failure sampling PHC for alignment: %w", sampleErr)
			}
		} else {
			consecutiveErrors = 0
		}
		sleep := interval
		if consecutiveErrors > 2 {
			// modest backoff on errors
			sleep = 3 * time.Second
		}
		time.Sleep(sleep)
	}
}
