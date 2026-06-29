package status

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
)

// Report holds read-only status from system time services.
type Report struct {
	Chrony   ChronyStatus   `json:"chrony" yaml:"chrony"`
	PTP      PTPStatus      `json:"ptp" yaml:"ptp"`
	Systemd  SystemdStatus  `json:"systemd" yaml:"systemd"`
	Role     string         `json:"role" yaml:"role"`
	ConfiguredRole string   `json:"configured_role" yaml:"configured_role"`
	Source   string         `json:"source" yaml:"source"`
	Offset   string         `json:"offset" yaml:"offset"`
	Healthy  bool           `json:"healthy" yaml:"healthy"`
}

// ChronyStatus from chronyc tracking/sources.
type ChronyStatus struct {
	Active   bool   `json:"active" yaml:"active"`
	Tracking string `json:"tracking" yaml:"tracking"`
	Source   string `json:"source" yaml:"source"`
	Offset   string `json:"offset" yaml:"offset"`
}

// PTPStatus from ptp4l/phc2sys.
type PTPStatus struct {
	PTP4LActive   bool   `json:"ptp4l_active" yaml:"ptp4l_active"`
	PHC2SysActive bool   `json:"phc2sys_active" yaml:"phc2sys_active"`
	State         string `json:"state" yaml:"state"`
}

// SystemdStatus for relevant units.
type SystemdStatus struct {
	Chronyd string `json:"chronyd" yaml:"chronyd"`
	PTP4L   string `json:"ptp4l" yaml:"ptp4l"`
	PHC2Sys string `json:"phc2sys" yaml:"phc2sys"`
}

// Collect gathers status without mutating the system.
func Collect() (*Report, error) {
	r := &Report{}
	r.Systemd = collectSystemd()
	r.Chrony = collectChrony()
	r.PTP = collectPTP()
	r.ConfiguredRole = configuredRole()
	r.Role, r.Source, r.Offset = inferSyncState(r)
	r.Healthy = r.Chrony.Active || r.PTP.PTP4LActive
	return r, nil
}

func collectSystemd() SystemdStatus {
	return SystemdStatus{
		Chronyd: unitActive("chronyd"),
		PTP4L:   unitActive("ptp4l"),
		PHC2Sys: unitActive("phc2sys"),
	}
}

func unitActive(unit string) string {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func collectChrony() ChronyStatus {
	s := ChronyStatus{Active: unitActive("chronyd") == "active"}
	out, err := exec.Command("chronyc", "-c", "tracking").Output()
	if err != nil {
		return s
	}
	s.Tracking = strings.TrimSpace(string(out))
	s.Source, s.Offset = ParseChronyTracking(string(out))
	return s
}

// ParseChronyTracking extracts reference ID and offset from chronyc -c tracking output.
func ParseChronyTracking(line string) (source, offset string) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) >= 6 {
		source = fields[2]
		offset = fields[5]
	}
	return source, offset
}

func collectPTP() PTPStatus {
	return PTPStatus{
		PTP4LActive:   unitActive("ptp4l") == "active",
		PHC2SysActive: unitActive("phc2sys") == "active",
		State:         ptp4lState(),
	}
}

func ptp4lState() string {
	out, err := exec.Command("pmc", "-u", "-b", "0", "GET", "PORT_DATA_SET").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func configuredRole() string {
	state, err := apply.LoadState("")
	if err != nil {
		return ""
	}
	return string(state.Role)
}

func inferSyncState(r *Report) (role, source, offset string) {
	if r.PTP.PTP4LActive {
		role = "ptp"
		source = r.PTP.State
	}
	if r.Chrony.Active {
		role = "ntp"
		source = r.Chrony.Source
		offset = r.Chrony.Offset
	}
	if role == "" {
		role = "unknown"
	}
	return role, source, offset
}

// Summary returns human-readable status output.
func (r *Report) Summary() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Sync health: %v\n", r.Healthy)
	if r.ConfiguredRole != "" {
		fmt.Fprintf(&b, "Configured role: %s\n", r.ConfiguredRole)
	}
	fmt.Fprintf(&b, "Active role: %s\n", r.Role)
	fmt.Fprintf(&b, "Source: %s\n", r.Source)
	if r.Offset != "" {
		fmt.Fprintf(&b, "Offset: %s\n", r.Offset)
	}
	fmt.Fprintf(&b, "\nSystemd:\n")
	fmt.Fprintf(&b, "  chronyd: %s\n", r.Systemd.Chronyd)
	fmt.Fprintf(&b, "  ptp4l:   %s\n", r.Systemd.PTP4L)
	fmt.Fprintf(&b, "  phc2sys: %s\n", r.Systemd.PHC2Sys)
	fmt.Fprintf(&b, "\nChrony:\n")
	fmt.Fprintf(&b, "  active: %v\n", r.Chrony.Active)
	if r.Chrony.Tracking != "" {
		fmt.Fprintf(&b, "  tracking: %s\n", r.Chrony.Tracking)
	}
	fmt.Fprintf(&b, "\nPTP:\n")
	fmt.Fprintf(&b, "  ptp4l:   %v\n", r.PTP.PTP4LActive)
	fmt.Fprintf(&b, "  phc2sys: %v\n", r.PTP.PHC2SysActive)
	return b.String()
}
