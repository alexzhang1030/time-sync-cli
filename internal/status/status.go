package status

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
)

const (
	maxHealthyPTPOffsetNs = 1_000_000_000
	minTrustedClockEpoch  = 946684800
	maxHealthyRTCSkewSec  = 3600
	maxHealthyPHCSkewSec  = 120
)

// Report holds read-only status from system time services.
type Report struct {
	Chrony         ChronyStatus  `json:"chrony" yaml:"chrony"`
	PTP            PTPStatus     `json:"ptp" yaml:"ptp"`
	Clock          ClockStatus   `json:"clock" yaml:"clock"`
	Systemd        SystemdStatus `json:"systemd" yaml:"systemd"`
	Role           string        `json:"role" yaml:"role"`
	ConfiguredRole string        `json:"configured_role" yaml:"configured_role"`
	ConfiguredPTP  bool          `json:"configured_ptp" yaml:"configured_ptp"`
	Source         string        `json:"source" yaml:"source"`
	Offset         string        `json:"offset" yaml:"offset"`
	Healthy        bool          `json:"healthy" yaml:"healthy"`
	NTPHealth      bool          `json:"ntp_health" yaml:"ntp_health"`
	PTPHealth      string        `json:"ptp_health" yaml:"ptp_health"`
	ClockHealth    string        `json:"clock_health" yaml:"clock_health"`
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

// ClockStatus reports wall-clock, RTC, and PHC consistency.
type ClockStatus struct {
	SystemUnix    int64  `json:"system_unix" yaml:"system_unix"`
	RTCUnix       int64  `json:"rtc_unix" yaml:"rtc_unix"`
	PHCUnix       int64  `json:"phc_unix" yaml:"phc_unix"`
	Iface         string `json:"iface" yaml:"iface"`
	RTCSystemSkew string `json:"rtc_system_skew" yaml:"rtc_system_skew"`
	PHCSystemSkew string `json:"phc_system_skew" yaml:"phc_system_skew"`
	Detail        string `json:"detail,omitempty" yaml:"detail,omitempty"`
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
	r.ConfiguredRole, r.ConfiguredPTP = configuredState()
	r.Clock = collectClock()
	r.NTPHealth = r.Chrony.Active
	r.PTPHealth = inferPTPHealth(r.PTP, r.ConfiguredRole)
	r.ClockHealth = inferClockHealth(r.Clock)
	r.Healthy = inferOverallHealth(r)
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

func collectClock() ClockStatus {
	s := ClockStatus{SystemUnix: time.Now().Unix()}
	rtc, err := readEpochFile("/sys/class/rtc/rtc0/since_epoch")
	if err != nil {
		s.Detail = "unable to read RTC: " + err.Error()
	} else {
		s.RTCUnix = rtc
		s.RTCSystemSkew = formatSeconds(s.SystemUnix - s.RTCUnix)
	}

	state, err := apply.LoadState("")
	if err == nil {
		s.Iface = strings.TrimSpace(state.Iface)
	}
	if s.Iface != "" {
		out, err := exec.Command("phc_ctl", s.Iface, "get").CombinedOutput()
		if err != nil {
			detail := "unable to read PHC: " + strings.TrimSpace(string(out))
			if os.Geteuid() != 0 {
				detail += "; try sudo timesync status"
			}
			appendClockDetail(&s, detail)
		} else if phc, err := parsePHCTime(string(out)); err != nil {
			appendClockDetail(&s, "unable to parse PHC: "+err.Error())
		} else {
			s.PHCUnix = phc
			s.PHCSystemSkew = formatSeconds(s.SystemUnix - s.PHCUnix)
		}
	}
	return s
}

func appendClockDetail(s *ClockStatus, detail string) {
	if s.Detail == "" {
		s.Detail = detail
		return
	}
	s.Detail += "; " + detail
}

func readEpochFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

func parsePHCTime(output string) (int64, error) {
	const marker = "clock time is "
	idx := strings.Index(output, marker)
	if idx < 0 {
		return 0, fmt.Errorf("missing %q", marker)
	}
	rest := output[idx+len(marker):]
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, fmt.Errorf("missing clock value")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return int64(value), nil
}

func pmcQuery(dataSet string) (string, error) {
	var errs []string
	for _, serverSocket := range pmcServerSocketCandidates() {
		out, err := pmcQueryWithServer(dataSet, serverSocket)
		if err == nil {
			return out, nil
		}
		if serverSocket == "" {
			errs = append(errs, "default socket: "+err.Error())
		} else {
			errs = append(errs, serverSocket+": "+err.Error())
		}
	}
	if hint := pmcPermissionHint(); hint != "" {
		errs = append(errs, hint)
	}
	return "", errors.New(strings.Join(errs, "; "))
}

func pmcQueryWithServer(dataSet, serverSocket string) (string, error) {
	args, clientSocket := pmcQueryArgsWithServer(dataSet, serverSocket)
	defer os.Remove(clientSocket)
	out, err := exec.Command("pmc", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	text := string(out)
	if !strings.Contains(text, "RESPONSE MANAGEMENT") {
		return "", fmt.Errorf("no management response from ptp4l: %s", strings.TrimSpace(text))
	}
	return text, nil
}

func pmcQueryArgs(dataSet string) ([]string, string) {
	return pmcQueryArgsWithServer(dataSet, "")
}

func pmcQueryArgsWithServer(dataSet, serverSocket string) ([]string, string) {
	clientSocket := filepath.Join(os.TempDir(), fmt.Sprintf("timesync-pmc-%d-%s.sock", os.Getpid(), strings.ToLower(dataSet)))
	args := []string{"-u"}
	if serverSocket != "" {
		args = append(args, "-s", serverSocket)
	}
	args = append(args, "-i", clientSocket, "-b", "0", "GET "+dataSet)
	return args, clientSocket
}

func pmcServerSocketCandidates() []string {
	candidates := []string{""}
	for _, socket := range []string{
		"/var/run/ptp4l",
		"/var/run/ptp/ptp4l",
		"/var/run/ptp4lro",
		"/var/run/ptp/ptp4lro",
	} {
		if _, err := os.Stat(socket); err == nil {
			candidates = append(candidates, socket)
		}
	}
	return candidates
}

func pmcPermissionHint() string {
	if os.Geteuid() == 0 {
		return ""
	}
	for _, socket := range []string{"/var/run/ptp4l", "/var/run/ptp/ptp4l"} {
		info, err := os.Stat(socket)
		if err != nil {
			continue
		}
		if info.Mode().Perm()&0o007 == 0 {
			return "ptp4l management socket may require root; try sudo timesync status"
		}
	}
	return ""
}

func configuredState() (string, bool) {
	state, err := apply.LoadState("")
	if err != nil {
		return "", false
	}
	return string(state.Role), state.PTP
}

func inferSyncState(r *Report) (role, source, offset string) {
	if r.Chrony.Active {
		role = "ntp"
		source = r.Chrony.Source
		offset = withUnit(r.Chrony.Offset, "s")
	}
	if r.PTPHealth == "true" && r.PTP.PHC2SysActive {
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

func inferPTPHealth(s PTPStatus, configuredRole string) string {
	if !s.PTP4LActive && !s.PHC2SysActive {
		return "false"
	}
	if !s.Available {
		return "unknown"
	}
	switch strings.ToUpper(s.PortState) {
	case "MASTER", "SLAVE":
		if strings.EqualFold(configuredRole, "client") && strings.EqualFold(s.PortState, "MASTER") {
			return "false"
		}
		if strings.EqualFold(configuredRole, "master") {
			if strings.EqualFold(s.PortState, "MASTER") {
				return "true"
			}
			return "false"
		}
		offsetOK, offsetKnown := ptpOffsetWithinThreshold(s)
		if !offsetKnown {
			return "unknown"
		}
		if !offsetOK {
			return "false"
		}
		return "true"
	case "":
		return "unknown"
	default:
		return "false"
	}
}

func inferClockHealth(s ClockStatus) string {
	if s.SystemUnix > 0 && s.SystemUnix < minTrustedClockEpoch {
		return "false"
	}
	if s.RTCUnix > 0 {
		if s.RTCUnix < minTrustedClockEpoch {
			return "false"
		}
		if s.SystemUnix > 0 && absInt64(s.SystemUnix-s.RTCUnix) > maxHealthyRTCSkewSec {
			return "false"
		}
	}
	if s.PHCUnix > 0 && s.SystemUnix > 0 && absInt64(s.SystemUnix-s.PHCUnix) > maxHealthyPHCSkewSec {
		return "false"
	}
	if s.Detail != "" {
		return "unknown"
	}
	return "true"
}

func inferOverallHealth(r *Report) bool {
	if r.ClockHealth != "true" {
		return false
	}
	if r.ConfiguredPTP && (strings.EqualFold(r.ConfiguredRole, "client") || strings.EqualFold(r.ConfiguredRole, "master")) {
		return r.PTPHealth == "true" && r.PTP.PHC2SysActive
	}
	if r.NTPHealth {
		return true
	}
	if r.PTPHealth == "true" {
		return r.PTP.PHC2SysActive
	}
	return false
}

func ptpOffsetWithinThreshold(s PTPStatus) (ok bool, known bool) {
	for _, value := range []string{s.MasterOffset, s.OffsetFromMaster} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		offset, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		return math.Abs(offset) <= maxHealthyPTPOffsetNs, true
	}
	return false, false
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func formatSeconds(value int64) string {
	return fmt.Sprintf("%d s", value)
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
	if r.ClockHealth != "" {
		fmt.Fprintf(&b, "Clock health: %s\n", r.ClockHealth)
	}
	fmt.Fprintf(&b, "Overall health: %v\n", r.Healthy)
	if r.ConfiguredRole != "" {
		fmt.Fprintf(&b, "Configured role: %s\n", r.ConfiguredRole)
		fmt.Fprintf(&b, "Configured PTP: %v\n", r.ConfiguredPTP)
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
	if r.Chrony.Offset != "" {
		fmt.Fprintf(&b, "  ntp offset: %s\n", withUnit(r.Chrony.Offset, "s"))
	}
	if r.Chrony.Tracking != "" {
		fmt.Fprintf(&b, "  tracking: %s\n", r.Chrony.Tracking)
	}
	fmt.Fprintf(&b, "\nClocks:\n")
	if r.Clock.SystemUnix > 0 {
		fmt.Fprintf(&b, "  system unix: %d\n", r.Clock.SystemUnix)
	}
	if r.Clock.RTCUnix > 0 {
		fmt.Fprintf(&b, "  rtc unix:    %d\n", r.Clock.RTCUnix)
	}
	if r.Clock.Iface != "" {
		fmt.Fprintf(&b, "  phc iface:   %s\n", r.Clock.Iface)
	}
	if r.Clock.PHCUnix > 0 {
		fmt.Fprintf(&b, "  phc unix:    %d\n", r.Clock.PHCUnix)
	}
	if r.Clock.RTCSystemSkew != "" {
		fmt.Fprintf(&b, "  rtc-system skew: %s\n", r.Clock.RTCSystemSkew)
	}
	if r.Clock.PHCSystemSkew != "" {
		fmt.Fprintf(&b, "  phc-system skew: %s\n", r.Clock.PHCSystemSkew)
	}
	if r.Clock.Detail != "" {
		fmt.Fprintf(&b, "  (%s)\n", r.Clock.Detail)
	}
	fmt.Fprintf(&b, "\nPTP:\n")
	fmt.Fprintf(&b, "  ptp4l:   %v\n", r.PTP.PTP4LActive)
	fmt.Fprintf(&b, "  phc2sys: %v\n", r.PTP.PHC2SysActive)
	if r.PTP.Available {
		if r.PTP.PortState != "" {
			fmt.Fprintf(&b, "  port state: %s\n", r.PTP.PortState)
		}
		metrics := PTPMetrics{
			MasterOffset:     r.PTP.MasterOffset,
			OffsetFromMaster: r.PTP.OffsetFromMaster,
		}
		if ptpOffset := metrics.PTPOffset(); ptpOffset != "" {
			fmt.Fprintf(&b, "  ptp offset: %s\n", ptpOffset)
		}
		if r.PTP.MasterOffset != "" {
			fmt.Fprintf(&b, "  master offset: %s\n", formatPTPNanoseconds(r.PTP.MasterOffset))
		} else if r.PTP.OffsetFromMaster != "" {
			fmt.Fprintf(&b, "  offset from master: %s\n", formatPTPNanoseconds(r.PTP.OffsetFromMaster))
		}
		if r.PTP.PathDelay != "" {
			fmt.Fprintf(&b, "  path delay: %s\n", formatPTPNanoseconds(r.PTP.PathDelay))
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
