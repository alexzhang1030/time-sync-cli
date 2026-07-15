package status

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
)

const (
	statusSchemaVersion  = "1.2"
	minTrustedClockEpoch = 946684800
)

// HealthState is the machine-readable state used by the v1.2 status model.
type HealthState string

const (
	HealthHealthy       HealthState = "healthy"
	HealthDegraded      HealthState = "degraded"
	HealthUnhealthy     HealthState = "unhealthy"
	HealthUnknown       HealthState = "unknown"
	HealthDisabled      HealthState = "disabled"
	HealthInactive      HealthState = "inactive"
	HealthNotApplicable HealthState = "not_applicable"
	HealthUnmanaged     HealthState = "unmanaged"
)

// HealthSummary separates service/link state from accuracy and clock discipline.
type HealthSummary struct {
	Overall     HealthState `json:"overall" yaml:"overall"`
	NTP         HealthState `json:"ntp" yaml:"ntp"`
	PTPLink     HealthState `json:"ptp_link" yaml:"ptp_link"`
	PTPAccuracy HealthState `json:"ptp_accuracy" yaml:"ptp_accuracy"`
	Clock       HealthState `json:"clock" yaml:"clock"`
	Discipline  HealthState `json:"discipline" yaml:"discipline"`
	Guard       HealthState `json:"guard" yaml:"guard"`
}

// Report holds read-only status from system time services.
type Report struct {
	SchemaVersion     string        `json:"schema_version" yaml:"schema_version"`
	Health            HealthSummary `json:"health" yaml:"health"`
	Chrony            ChronyStatus  `json:"chrony" yaml:"chrony"`
	PTP               PTPStatus     `json:"ptp" yaml:"ptp"`
	Clock             ClockStatus   `json:"clock" yaml:"clock"`
	Systemd           SystemdStatus `json:"systemd" yaml:"systemd"`
	Role              string        `json:"role" yaml:"role"`
	ConfiguredRole    string        `json:"configured_role" yaml:"configured_role"`
	ConfiguredPTP     bool          `json:"configured_ptp" yaml:"configured_ptp"`
	Source            string        `json:"source" yaml:"source"`
	Offset            string        `json:"offset" yaml:"offset"`
	Healthy           bool          `json:"healthy" yaml:"healthy"`
	NTPHealth         bool          `json:"ntp_health" yaml:"ntp_health"`
	PTPHealth         string        `json:"ptp_health" yaml:"ptp_health"`
	ClockHealth       string        `json:"clock_health" yaml:"clock_health"`
	SystemClockSource string        `json:"system_clock_source" yaml:"system_clock_source"`
	ClockFlow         string        `json:"clock_flow" yaml:"clock_flow"`
	Warnings          []string      `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// ChronyStatus from chronyc tracking/sources.
type ChronyStatus struct {
	Active       bool    `json:"active" yaml:"active"`
	UnitState    string  `json:"unit_state" yaml:"unit_state"`
	Available    bool    `json:"available" yaml:"available"`
	Synchronized bool    `json:"synchronized" yaml:"synchronized"`
	Holdover     bool    `json:"holdover" yaml:"holdover"`
	Tracking     string  `json:"tracking" yaml:"tracking"`
	ReferenceID  string  `json:"reference_id" yaml:"reference_id"`
	Source       string  `json:"source" yaml:"source"`
	Stratum      int     `json:"stratum" yaml:"stratum"`
	RefTimeUnix  float64 `json:"ref_time_unix" yaml:"ref_time_unix"`
	Offset       string  `json:"offset" yaml:"offset"`
	LastOffset   string  `json:"last_offset" yaml:"last_offset"`
	RMSOffset    string  `json:"rms_offset" yaml:"rms_offset"`
	SkewPPM      string  `json:"skew_ppm" yaml:"skew_ppm"`
	LeapStatus   string  `json:"leap_status" yaml:"leap_status"`
	Detail       string  `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// PTPStatus from ptp4l/phc2sys via pmc queries.
type PTPStatus struct {
	PTP4LActive             bool   `json:"ptp4l_active" yaml:"ptp4l_active"`
	PHC2SysActive           bool   `json:"phc2sys_active" yaml:"phc2sys_active"`
	PTP4LState              string `json:"ptp4l_state" yaml:"ptp4l_state"`
	PHC2SysState            string `json:"phc2sys_state" yaml:"phc2sys_state"`
	Available               bool   `json:"available" yaml:"available"`
	Detail                  string `json:"detail,omitempty" yaml:"detail,omitempty"`
	PortState               string `json:"port_state" yaml:"port_state"`
	MasterOffset            string `json:"master_offset" yaml:"master_offset"`
	OffsetFromMaster        string `json:"offset_from_master" yaml:"offset_from_master"`
	PathDelay               string `json:"path_delay" yaml:"path_delay"`
	StepsRemoved            string `json:"steps_removed" yaml:"steps_removed"`
	GMIdentity              string `json:"gm_identity" yaml:"gm_identity"`
	TimePropertiesAvailable bool   `json:"time_properties_available" yaml:"time_properties_available"`
	CurrentUTCOffset        int    `json:"current_utc_offset" yaml:"current_utc_offset"`
	CurrentUTCOffsetValid   bool   `json:"current_utc_offset_valid" yaml:"current_utc_offset_valid"`
	PTPTimescale            bool   `json:"ptp_timescale" yaml:"ptp_timescale"`
	TimeTraceable           bool   `json:"time_traceable" yaml:"time_traceable"`
	FrequencyTraceable      bool   `json:"frequency_traceable" yaml:"frequency_traceable"`
	TimePropertiesDetail    string `json:"time_properties_detail,omitempty" yaml:"time_properties_detail,omitempty"`
}

// ClockStatus reports wall-clock, RTC, and PHC consistency.
type ClockStatus struct {
	SystemUnix            int64  `json:"system_unix" yaml:"system_unix"`
	RTCUnix               int64  `json:"rtc_unix" yaml:"rtc_unix"`
	PHCUnix               int64  `json:"phc_unix" yaml:"phc_unix"`
	Iface                 string `json:"iface" yaml:"iface"`
	RTCSystemSkew         string `json:"rtc_system_skew" yaml:"rtc_system_skew"`
	PHCSystemSkew         string `json:"phc_system_skew" yaml:"phc_system_skew"`
	PHCResidualNS         *int64 `json:"phc_residual_ns,omitempty" yaml:"phc_residual_ns,omitempty"`
	PHCResidual           string `json:"phc_residual" yaml:"phc_residual"`
	PHCTimeScale          string `json:"phc_time_scale" yaml:"phc_time_scale"`
	TAIUTCOffset          int    `json:"tai_utc_offset" yaml:"tai_utc_offset"`
	TAIUTCOffsetValid     bool   `json:"tai_utc_offset_valid" yaml:"tai_utc_offset_valid"`
	Resolution            string `json:"resolution" yaml:"resolution"`
	Detail                string `json:"detail,omitempty" yaml:"detail,omitempty"`
	phcSampleSystemUnixNS int64
}

// UnitStatus preserves the actual unit name and query outcome.
type UnitStatus struct {
	Unit          string `json:"unit" yaml:"unit"`
	LoadState     string `json:"load_state" yaml:"load_state"`
	ActiveState   string `json:"active_state" yaml:"active_state"`
	UnitFileState string `json:"unit_file_state" yaml:"unit_file_state"`
	Detail        string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// SystemdStatus for relevant units.
type SystemdStatus struct {
	Chronyd     string     `json:"chronyd" yaml:"chronyd"`
	PTP4L       string     `json:"ptp4l" yaml:"ptp4l"`
	PHC2Sys     string     `json:"phc2sys" yaml:"phc2sys"`
	NTPDaemon   UnitStatus `json:"ntp_daemon" yaml:"ntp_daemon"`
	PTP4LUnit   UnitStatus `json:"ptp4l_unit" yaml:"ptp4l_unit"`
	PHC2SysUnit UnitStatus `json:"phc2sys_unit" yaml:"phc2sys_unit"`
	GuardTimer  UnitStatus `json:"guard_timer" yaml:"guard_timer"`
}

// Collect gathers status without mutating the system.
func Collect() (*Report, error) {
	r := &Report{SchemaVersion: statusSchemaVersion}
	r.Systemd = collectSystemd()
	r.Chrony = collectChrony(r.Systemd.NTPDaemon)
	r.PTP = collectPTP(r.Systemd)
	r.ConfiguredRole, r.ConfiguredPTP = configuredState()
	r.Clock = collectClock(r.PTP)
	populateDerivedStatus(r)
	return r, nil
}

func collectPTP(systemd SystemdStatus) PTPStatus {
	s := PTPStatus{
		PTP4LActive:   systemd.PTP4LUnit.ActiveState == "active",
		PHC2SysActive: systemd.PHC2SysUnit.ActiveState == "active",
		PTP4LState:    systemd.PTP4LUnit.ActiveState,
		PHC2SysState:  systemd.PHC2SysUnit.ActiveState,
	}
	if !s.PTP4LActive {
		if s.PTP4LState == "query-error" {
			s.Detail = valueOr(systemd.PTP4LUnit.Detail, "unable to query ptp4l service")
		} else {
			s.Detail = "ptp4l not running"
		}
		return s
	}
	portOut, err := pmcQuery("PORT_DATA_SET")
	if err != nil {
		s.Detail = "unable to query ptp4l via pmc: " + err.Error()
		return s
	}
	timeOut, timeErr := pmcQuery("TIME_STATUS_NP")
	currentOut, currentErr := pmcQuery("CURRENT_DATA_SET")
	if timeErr != nil && currentErr != nil {
		s.Detail = "PTP offset metrics unavailable: " + timeErr.Error()
	}
	metrics := ParsePTPMetrics(portOut, timeOut, currentOut)

	s.Available = true
	s.PortState = metrics.PortState
	s.MasterOffset = metrics.MasterOffset
	s.OffsetFromMaster = metrics.OffsetFromMaster
	s.PathDelay = metrics.PathDelay
	s.StepsRemoved = metrics.StepsRemoved
	s.GMIdentity = metrics.GMIdentity

	propertiesOut, err := pmcQuery("TIME_PROPERTIES_DATA_SET")
	if err != nil {
		s.TimePropertiesDetail = "unable to query PTP time properties: " + err.Error()
		return s
	}
	properties, err := ParsePTPTimeProperties(propertiesOut)
	if err != nil {
		s.TimePropertiesDetail = "unable to parse PTP time properties: " + err.Error()
		return s
	}
	s.TimePropertiesAvailable = true
	s.CurrentUTCOffset = properties.CurrentUTCOffset
	s.CurrentUTCOffsetValid = properties.CurrentUTCOffsetValid
	s.PTPTimescale = properties.PTPTimescale
	s.TimeTraceable = properties.TimeTraceable
	s.FrequencyTraceable = properties.FrequencyTraceable
	return s
}

func collectClock(ptp PTPStatus) ClockStatus {
	now := time.Now()
	s := ClockStatus{
		SystemUnix: now.Unix(),
		Resolution: "PHC residual uses nanosecond samples; RTC snapshot has 1 s resolution",
	}
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
		before := time.Now()
		out, err := exec.Command("phc_ctl", s.Iface, "get").CombinedOutput()
		after := time.Now()
		if err != nil {
			detail := "unable to read PHC: " + strings.TrimSpace(string(out))
			if os.Geteuid() != 0 {
				detail += "; try sudo timesync status"
			}
			appendClockDetail(&s, detail)
		} else if phcNS, err := parsePHCTimeNS(string(out)); err != nil {
			appendClockDetail(&s, "unable to parse PHC: "+err.Error())
		} else {
			s.PHCUnix = phcNS / int64(time.Second)
			s.phcSampleSystemUnixNS = before.UnixNano() + after.Sub(before).Nanoseconds()/2
			rawSkewNS := s.phcSampleSystemUnixNS - phcNS
			s.PHCSystemSkew = formatSeconds(int64(time.Duration(rawSkewNS).Round(time.Second) / time.Second))
			populatePHCResidual(&s, ptp, phcNS)
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
	value, err := parsePHCTimeNS(output)
	return value / int64(time.Second), err
}

func parsePHCTimeNS(output string) (int64, error) {
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
	parts := strings.SplitN(fields[0], ".", 2)
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 9 {
		fraction = fraction[:9]
	}
	fraction += strings.Repeat("0", 9-len(fraction))
	nanoseconds, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, err
	}
	return seconds*int64(time.Second) + nanoseconds, nil
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
