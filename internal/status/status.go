package status

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
)

// Report holds read-only status from system time services.
type Report struct {
	Chrony         ChronyStatus  `json:"chrony" yaml:"chrony"`
	PTP            PTPStatus     `json:"ptp" yaml:"ptp"`
	Systemd        SystemdStatus `json:"systemd" yaml:"systemd"`
	Role           string        `json:"role" yaml:"role"`
	ConfiguredRole string        `json:"configured_role" yaml:"configured_role"`
	Source         string        `json:"source" yaml:"source"`
	Offset         string        `json:"offset" yaml:"offset"`
	Healthy        bool          `json:"healthy" yaml:"healthy"`
	NTPHealth      bool          `json:"ntp_health" yaml:"ntp_health"`
	PTPHealth      string        `json:"ptp_health" yaml:"ptp_health"`
}

// ChronyStatus from chronyc tracking/sources.
type ChronyStatus struct {
	Active   bool   `json:"active" yaml:"active"`
	Tracking string `json:"tracking" yaml:"tracking"`
	Source   string `json:"source" yaml:"source"`
	Offset   string `json:"offset" yaml:"offset"`
}

// PTPStatus from ptp4l/phc2sys via pmc queries.
type PTPStatus struct {
	PTP4LActive      bool   `json:"ptp4l_active" yaml:"ptp4l_active"`
	PHC2SysActive    bool   `json:"phc2sys_active" yaml:"phc2sys_active"`
	Available        bool   `json:"available" yaml:"available"`
	Detail           string `json:"detail,omitempty" yaml:"detail,omitempty"`
	PortState        string `json:"port_state" yaml:"port_state"`
	MasterOffset     string `json:"master_offset" yaml:"master_offset"`
	OffsetFromMaster string `json:"offset_from_master" yaml:"offset_from_master"`
	PathDelay        string `json:"path_delay" yaml:"path_delay"`
	StepsRemoved     string `json:"steps_removed" yaml:"steps_removed"`
	GMIdentity       string `json:"gm_identity" yaml:"gm_identity"`
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
	r.NTPHealth = r.Chrony.Active
	r.PTPHealth = inferPTPHealth(r.PTP)
	r.Healthy = r.NTPHealth || r.PTPHealth == "true"
	r.Role, r.Source, r.Offset = inferSyncState(r)
	return r, nil
}

func collectSystemd() SystemdStatus {
	return SystemdStatus{
		Chronyd: unitActiveAny("chrony", "chronyd"),
		PTP4L:   unitActive("ptp4l"),
		PHC2Sys: unitActive("phc2sys"),
	}
}

func unitActiveAny(units ...string) string {
	last := "inactive"
	for _, unit := range units {
		state := unitActive(unit)
		if state == "active" {
			return state
		}
		last = state
	}
	return last
}

func unitActive(unit string) string {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func collectChrony() ChronyStatus {
	s := ChronyStatus{Active: unitActiveAny("chrony", "chronyd") == "active"}
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
	s := PTPStatus{
		PTP4LActive:   unitActive("ptp4l") == "active",
		PHC2SysActive: unitActive("phc2sys") == "active",
	}
	if !s.PTP4LActive {
		s.Detail = "ptp4l not running"
		return s
	}
	portOut, err := pmcQuery("PORT_DATA_SET")
	if err != nil {
		s.Detail = "unable to query ptp4l via pmc: " + err.Error()
		return s
	}
	timeOut, _ := pmcQuery("TIME_STATUS_NP")
	currentOut, _ := pmcQuery("CURRENT_DATA_SET")
	metrics := ParsePTPMetrics(portOut, timeOut, currentOut)

	s.Available = true
	s.PortState = metrics.PortState
	s.MasterOffset = metrics.MasterOffset
	s.OffsetFromMaster = metrics.OffsetFromMaster
	s.PathDelay = metrics.PathDelay
	s.StepsRemoved = metrics.StepsRemoved
	s.GMIdentity = metrics.GMIdentity
	return s
}

func pmcQuery(dataSet string) (string, error) {
	args, clientSocket := pmcQueryArgs(dataSet)
	defer os.Remove(clientSocket)
	out, err := exec.Command("pmc", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func pmcQueryArgs(dataSet string) ([]string, string) {
	clientSocket := filepath.Join(os.TempDir(), fmt.Sprintf("timesync-pmc-%d-%s.sock", os.Getpid(), strings.ToLower(dataSet)))
	return []string{"-u", "-i", clientSocket, "-b", "0", "GET", dataSet}, clientSocket
}

func configuredRole() string {
	state, err := apply.LoadState("")
	if err != nil {
		return ""
	}
	return string(state.Role)
}

func inferSyncState(r *Report) (role, source, offset string) {
	if r.Chrony.Active {
		role = "ntp"
		source = r.Chrony.Source
		offset = withUnit(r.Chrony.Offset, "s")
	}
	if r.PTPHealth == "true" {
		role = "ptp"
		switch {
		case r.PTP.PortState != "":
			source = r.PTP.PortState
		case r.PTP.GMIdentity != "":
			source = r.PTP.GMIdentity
		default:
			source = "ptp4l"
		}
		metrics := PTPMetrics{
			MasterOffset:     r.PTP.MasterOffset,
			OffsetFromMaster: r.PTP.OffsetFromMaster,
		}
		offset = metrics.PTPOffset()
	}
	if role == "" {
		role = "unknown"
	}
	return role, source, offset
}

func inferPTPHealth(s PTPStatus) string {
	if !s.PTP4LActive && !s.PHC2SysActive {
		return "false"
	}
	if !s.Available {
		return "unknown"
	}
	switch strings.ToUpper(s.PortState) {
	case "MASTER", "SLAVE":
		return "true"
	case "":
		return "unknown"
	default:
		return "false"
	}
}

func withUnit(value, unit string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, " ") {
		return value
	}
	return value + " " + unit
}

// Summary returns human-readable status output.
func (r *Report) Summary() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "NTP health: %v\n", r.NTPHealth)
	if r.PTPHealth != "" {
		fmt.Fprintf(&b, "PTP health: %s\n", r.PTPHealth)
	}
	fmt.Fprintf(&b, "Overall health: %v\n", r.Healthy)
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
	if r.PTP.Available {
		if r.PTP.PortState != "" {
			fmt.Fprintf(&b, "  port state: %s\n", r.PTP.PortState)
		}
		if r.PTP.MasterOffset != "" {
			fmt.Fprintf(&b, "  master offset: %s ns\n", r.PTP.MasterOffset)
		} else if r.PTP.OffsetFromMaster != "" {
			fmt.Fprintf(&b, "  offset from master: %s ns\n", r.PTP.OffsetFromMaster)
		}
		if r.PTP.PathDelay != "" {
			fmt.Fprintf(&b, "  path delay: %s ns\n", r.PTP.PathDelay)
		}
		if r.PTP.StepsRemoved != "" {
			fmt.Fprintf(&b, "  steps removed: %s\n", r.PTP.StepsRemoved)
		}
		if r.PTP.GMIdentity != "" {
			fmt.Fprintf(&b, "  grandmaster: %s\n", r.PTP.GMIdentity)
		}
	} else if r.PTP.Detail != "" {
		fmt.Fprintf(&b, "  (%s)\n", r.PTP.Detail)
	}
	return b.String()
}
