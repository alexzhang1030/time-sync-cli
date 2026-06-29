package detect

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Report holds system detection results for doctor command.
type Report struct {
	OS              string            `json:"os" yaml:"os"`
	Arch            string            `json:"arch" yaml:"arch"`
	InitSystem      string            `json:"init_system" yaml:"init_system"`
	Binaries        map[string]bool   `json:"binaries" yaml:"binaries"`
	Interfaces      []InterfaceInfo   `json:"interfaces" yaml:"interfaces"`
	PTPCapabilities map[string]PTPInfo `json:"ptp_capabilities" yaml:"ptp_capabilities"`
}

// InterfaceInfo describes a network interface candidate.
type InterfaceInfo struct {
	Name string `json:"name" yaml:"name"`
	Up   bool   `json:"up" yaml:"up"`
}

// PTPInfo describes PTP timestamping capability for an interface.
type PTPInfo struct {
	Interface       string `json:"interface" yaml:"interface"`
	HardwareTS      bool   `json:"hardware_timestamping" yaml:"hardware_timestamping"`
	SoftwareTS      bool   `json:"software_timestamping" yaml:"software_timestamping"`
	Raw             string `json:"raw,omitempty" yaml:"raw,omitempty"`
}

var requiredBinaries = []string{
	"chronyc", "chronyd", "ptp4l", "phc2sys", "ethtool", "systemctl",
}

// Run executes full system detection.
func Run() (*Report, error) {
	r := &Report{
		OS:              detectOS(),
		Arch:            runtime.GOARCH,
		InitSystem:      detectInitSystem(),
		Binaries:        detectBinaries(requiredBinaries),
		Interfaces:      detectInterfaces(),
		PTPCapabilities: make(map[string]PTPInfo),
	}
	for _, iface := range r.Interfaces {
		if info, err := detectPTPCapability(iface.Name); err == nil {
			r.PTPCapabilities[iface.Name] = info
		}
	}
	return r, nil
}

func detectOS() string {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
	}
	return runtime.GOOS
}

func detectInitSystem() string {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return "systemd"
	}
	if _, err := os.Stat("/sbin/init"); err == nil {
		return "unknown"
	}
	return "unknown"
}

func detectBinaries(names []string) map[string]bool {
	found := make(map[string]bool, len(names))
	for _, name := range names {
		path, err := exec.LookPath(name)
		found[name] = err == nil && path != ""
	}
	return found
}

func detectInterfaces() []InterfaceInfo {
	var out []InterfaceInfo
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		up := false
		if data, err := os.ReadFile(filepath.Join("/sys/class/net", name, "operstate")); err == nil {
			up = strings.TrimSpace(string(data)) == "up"
		}
		out = append(out, InterfaceInfo{Name: name, Up: up})
	}
	return out
}

func detectPTPCapability(iface string) (PTPInfo, error) {
	info := PTPInfo{Interface: iface}
	cmd := exec.Command("ethtool", "-T", iface)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return info, err
	}
	raw := stdout.String()
	info.Raw = raw
	info.HardwareTS, info.SoftwareTS = parseEthtoolTimestamping(raw)
	return info, nil
}

// ParseEthtoolTimestamping parses ethtool -T output for timestamping modes.
func ParseEthtoolTimestamping(output string) (hardware, software bool) {
	return parseEthtoolTimestamping(output)
}

func isEnabledValue(line string) bool {
	_, val, ok := strings.Cut(strings.ToLower(line), ":")
	if !ok {
		return false
	}
	val = strings.TrimSpace(val)
	return val == "on" || val == "yes"
}

func parseEthtoolTimestamping(output string) (hardware, software bool) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)
		if (strings.Contains(lower, "hardware transmit") || strings.Contains(lower, "hardware receive")) && isEnabledValue(line) {
			hardware = true
		}
		if (strings.Contains(lower, "software transmit") || strings.Contains(lower, "software receive")) && isEnabledValue(line) {
			software = true
		}
		if strings.Contains(lower, "ptp hardware clock") {
			_, val, ok := strings.Cut(lower, ":")
			if ok && strings.TrimSpace(val) != "none" && strings.TrimSpace(val) != "0" {
				hardware = true
			}
		}
	}
	return hardware, software
}

// MissingBinaries returns names of required binaries not found.
func (r *Report) MissingBinaries() []string {
	var missing []string
	for name, ok := range r.Binaries {
		if !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

// Summary returns a human-readable doctor summary.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "OS: %s (%s)\n", r.OS, r.Arch)
	fmt.Fprintf(&b, "Init: %s\n", r.InitSystem)
	fmt.Fprintf(&b, "Binaries:\n")
	for _, name := range requiredBinaries {
		status := "missing"
		if r.Binaries[name] {
			status = "ok"
		}
		fmt.Fprintf(&b, "  %s: %s\n", name, status)
	}
	fmt.Fprintf(&b, "Interfaces:\n")
	for _, iface := range r.Interfaces {
		state := "down"
		if iface.Up {
			state = "up"
		}
		fmt.Fprintf(&b, "  %s: %s\n", iface.Name, state)
	}
	fmt.Fprintf(&b, "PTP capabilities:\n")
	for name, ptp := range r.PTPCapabilities {
		fmt.Fprintf(&b, "  %s: hw=%v sw=%v\n", name, ptp.HardwareTS, ptp.SoftwareTS)
	}
	return b.String()
}
