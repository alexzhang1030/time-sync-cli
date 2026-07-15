package status

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	maxHealthyNTPCorrectionSec  = 0.1
	maxDegradedNTPCorrectionSec = 1.0
	maxHealthyPTPOffsetNs       = 10_000_000
	maxSafePTPOffsetNs          = 1_000_000_000
	maxHealthyPHCResidualNs     = 10_000_000
	maxDegradedPHCResidualNs    = 1_000_000_000
	maxHealthyRTCSkewSec        = 3600
)

func populateDerivedStatus(r *Report) {
	r.Health.NTP = inferNTPStatus(r.Chrony)
	r.Health.PTPLink = inferPTPLinkStatus(r.PTP, r.ConfiguredRole)
	r.Health.PTPAccuracy = inferPTPAccuracyStatus(r.PTP, r.ConfiguredRole)
	if r.ConfiguredPTP && strings.EqualFold(r.ConfiguredRole, "client") {
		r.Health.NTP = HealthDisabled
	}
	if r.ConfiguredRole != "" && !r.ConfiguredPTP {
		r.Health.PTPLink = HealthDisabled
		r.Health.PTPAccuracy = HealthDisabled
	}
	r.Health.Clock = inferClockStatus(r.Clock)
	r.Health.Discipline = inferDisciplineStatus(r)
	r.Health.Guard = inferGuardStatus(r)
	r.Health.Overall = inferOverallStatus(r)

	r.NTPHealth = r.Health.NTP == HealthHealthy
	r.PTPHealth = inferPTPHealth(r.PTP, r.ConfiguredRole)
	r.ClockHealth = legacyHealth(r.Health.Clock)
	r.Healthy = r.Health.Overall == HealthHealthy
	r.Role, r.Source, r.Offset = inferSyncState(r)
	r.SystemClockSource = r.Source
	r.ClockFlow = inferClockFlow(r)
	r.Warnings = inferWarnings(r)
}

func inferNTPStatus(s ChronyStatus) HealthState {
	if s.UnitState == "query-error" {
		return HealthUnknown
	}
	if !s.Active {
		return HealthInactive
	}
	if !s.Available {
		return HealthUnknown
	}
	if s.Holdover {
		return HealthDegraded
	}
	if !s.Synchronized {
		return HealthUnhealthy
	}
	correction, err := strconv.ParseFloat(strings.TrimSpace(s.Offset), 64)
	if err != nil {
		return HealthUnknown
	}
	switch absolute := math.Abs(correction); {
	case absolute <= maxHealthyNTPCorrectionSec:
		return HealthHealthy
	case absolute <= maxDegradedNTPCorrectionSec:
		return HealthDegraded
	default:
		return HealthUnhealthy
	}
}

func inferPTPLinkStatus(s PTPStatus, configuredRole string) HealthState {
	if s.PTP4LState == "query-error" {
		return HealthUnknown
	}
	if !s.PTP4LActive && !s.PHC2SysActive {
		return HealthInactive
	}
	if !s.PTP4LActive {
		return HealthUnhealthy
	}
	if !s.Available {
		return HealthUnknown
	}

	state := strings.ToUpper(strings.TrimSpace(s.PortState))
	switch strings.ToLower(strings.TrimSpace(configuredRole)) {
	case "client", "auto":
		if state == "SLAVE" {
			return HealthHealthy
		}
		if ptpPortIsConverging(state) {
			return HealthDegraded
		}
		return HealthUnhealthy
	case "master":
		if state == "MASTER" {
			return HealthHealthy
		}
		if ptpPortIsConverging(state) {
			return HealthDegraded
		}
		return HealthUnhealthy
	default:
		if state == "MASTER" || state == "SLAVE" {
			return HealthHealthy
		}
		if state == "" {
			return HealthUnknown
		}
		if ptpPortIsConverging(state) {
			return HealthDegraded
		}
		return HealthUnhealthy
	}
}

func ptpPortIsConverging(state string) bool {
	switch state {
	case "INITIALIZING", "LISTENING", "PRE_MASTER", "PASSIVE", "UNCALIBRATED":
		return true
	default:
		return false
	}
}

func inferPTPAccuracyStatus(s PTPStatus, configuredRole string) HealthState {
	if s.PTP4LState == "query-error" {
		return HealthUnknown
	}
	if !s.PTP4LActive {
		return HealthInactive
	}
	if !s.Available {
		return HealthUnknown
	}
	if strings.EqualFold(configuredRole, "master") && strings.EqualFold(s.PortState, "MASTER") {
		return HealthNotApplicable
	}
	offset, known := ptpOffsetNanoseconds(s)
	if !known {
		return HealthUnknown
	}
	switch absolute := math.Abs(offset); {
	case absolute <= maxHealthyPTPOffsetNs:
		return HealthHealthy
	case absolute <= maxSafePTPOffsetNs:
		return HealthDegraded
	default:
		return HealthUnhealthy
	}
}

// inferPTPHealth preserves the v1 guard-readiness contract. The dashboard uses
// separate link and accuracy states, while the guard keeps phc2sys available
// through bounded convergence up to the one-second safety limit.
func inferPTPHealth(s PTPStatus, configuredRole string) string {
	link := inferPTPLinkStatus(s, configuredRole)
	if link == HealthInactive || link == HealthUnhealthy || link == HealthDegraded {
		return "false"
	}
	if link == HealthUnknown {
		return "unknown"
	}
	if strings.EqualFold(configuredRole, "master") {
		return "true"
	}
	offset, known := ptpOffsetNanoseconds(s)
	if !known {
		return "unknown"
	}
	if math.Abs(offset) <= maxSafePTPOffsetNs {
		return "true"
	}
	return "false"
}

func ptpOffsetNanoseconds(s PTPStatus) (float64, bool) {
	for _, value := range []string{s.MasterOffset, s.OffsetFromMaster} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		offset, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return offset, true
		}
	}
	return 0, false
}

func populatePHCResidual(s *ClockStatus, ptp PTPStatus, phcUnixNS int64) {
	s.PHCTimeScale = "unknown"
	if !ptp.TimePropertiesAvailable {
		return
	}
	if ptp.PTPTimescale {
		s.PHCTimeScale = "TAI"
		s.TAIUTCOffset = ptp.CurrentUTCOffset
		s.TAIUTCOffsetValid = ptp.CurrentUTCOffsetValid
		if !ptp.CurrentUTCOffsetValid {
			return
		}
		phcUnixNS -= int64(ptp.CurrentUTCOffset) * int64(time.Second)
	} else {
		s.PHCTimeScale = "UTC"
	}
	residual := s.phcSampleSystemUnixNS - phcUnixNS
	s.PHCResidualNS = &residual
	s.PHCResidual = formatClockResidual(residual)
}

func inferClockStatus(s ClockStatus) HealthState {
	if s.SystemUnix > 0 && s.SystemUnix < minTrustedClockEpoch {
		return HealthUnhealthy
	}
	if s.RTCUnix > 0 {
		if s.RTCUnix < minTrustedClockEpoch {
			return HealthUnhealthy
		}
		if s.SystemUnix > 0 && absInt64(s.SystemUnix-s.RTCUnix) > maxHealthyRTCSkewSec {
			return HealthUnhealthy
		}
	}
	if s.PHCUnix > 0 {
		if s.PHCResidualNS == nil {
			return HealthUnknown
		}
		switch absolute := absInt64(*s.PHCResidualNS); {
		case absolute <= maxHealthyPHCResidualNs:
		case absolute <= maxDegradedPHCResidualNs:
			return HealthDegraded
		default:
			return HealthUnhealthy
		}
	}
	if s.Detail != "" {
		return HealthUnknown
	}
	return HealthHealthy
}

func inferClockHealth(s ClockStatus) string {
	return legacyHealth(inferClockStatus(s))
}

func inferDisciplineStatus(r *Report) HealthState {
	if r.ConfiguredPTP && (strings.EqualFold(r.ConfiguredRole, "client") || strings.EqualFold(r.ConfiguredRole, "master")) {
		switch r.Systemd.PHC2SysUnit.ActiveState {
		case "active":
			return HealthHealthy
		case "query-error", "":
			return HealthUnknown
		default:
			return HealthUnhealthy
		}
	}
	if r.Chrony.Active {
		return inferNTPStatus(r.Chrony)
	}
	if r.PTP.PHC2SysActive && strings.EqualFold(r.PTP.PortState, "SLAVE") {
		return HealthHealthy
	}
	return HealthUnknown
}

func inferGuardStatus(r *Report) HealthState {
	guardExpected := r.ConfiguredPTP && (strings.EqualFold(r.ConfiguredRole, "client") || strings.EqualFold(r.ConfiguredRole, "master"))
	if !guardExpected {
		return HealthDisabled
	}
	guard := r.Systemd.GuardTimer
	if guard.ActiveState == "query-error" || guard.ActiveState == "" {
		return HealthUnknown
	}
	if guard.ActiveState == "active" && guard.UnitFileState == "enabled" {
		return HealthHealthy
	}
	if guard.ActiveState == "active" || guard.UnitFileState == "enabled" {
		return HealthDegraded
	}
	return HealthUnhealthy
}

func inferOverallStatus(r *Report) HealthState {
	if r.ManagementState == "error" {
		return HealthUnknown
	}
	if strings.TrimSpace(r.ConfiguredRole) == "" {
		return HealthUnmanaged
	}
	if r.ConfiguredPTP && strings.EqualFold(r.ConfiguredRole, "client") {
		return reduceRequiredHealth(
			r.Health.Clock,
			r.Health.PTPLink,
			r.Health.PTPAccuracy,
			r.Health.Discipline,
			r.Health.Guard,
		)
	}
	if r.ConfiguredPTP && strings.EqualFold(r.ConfiguredRole, "master") {
		return reduceRequiredHealth(
			r.Health.NTP,
			r.Health.Clock,
			r.Health.PTPLink,
			r.Health.Discipline,
			r.Health.Guard,
		)
	}
	return reduceRequiredHealth(r.Health.NTP, r.Health.Clock)
}

func inferOverallHealth(r *Report) bool {
	return inferOverallStatus(r) == HealthHealthy
}

func reduceRequiredHealth(states ...HealthState) HealthState {
	result := HealthHealthy
	for _, state := range states {
		switch state {
		case HealthUnhealthy, HealthInactive:
			return HealthUnhealthy
		case HealthUnknown, "":
			if result == HealthHealthy || result == HealthDegraded {
				result = HealthUnknown
			}
		case HealthDegraded:
			if result == HealthHealthy {
				result = HealthDegraded
			}
		}
	}
	return result
}

func inferSyncState(r *Report) (role, source, offset string) {
	ptpDisciplinesSystem := r.PTP.PHC2SysActive && strings.EqualFold(r.PTP.PortState, "SLAVE")
	configuredPTPClient := r.ConfiguredPTP && strings.EqualFold(r.ConfiguredRole, "client")
	if (configuredPTPClient || strings.TrimSpace(r.ConfiguredRole) == "") && ptpDisciplinesSystem {
		role = "ptp"
		source = valueOr(r.PTP.GMIdentity, "PTP grandmaster")
		offset = (PTPMetrics{MasterOffset: r.PTP.MasterOffset, OffsetFromMaster: r.PTP.OffsetFromMaster}).PTPOffset()
		return role, source, offset
	}
	if r.Chrony.Active {
		role = "ntp"
		if r.Chrony.Holdover {
			source = "local holdover"
		} else {
			source = valueOr(r.Chrony.Source, "chrony")
		}
		offset = withUnit(r.Chrony.Offset, "s")
		return role, source, offset
	}
	return "unknown", "", ""
}

func inferClockFlow(r *Report) string {
	if r.ConfiguredPTP {
		switch strings.ToLower(strings.TrimSpace(r.ConfiguredRole)) {
		case "client":
			return "PTP grandmaster → PHC → System"
		case "master":
			return "NTP → System → PHC → PTP clients"
		case "auto":
			return "NTP → System · PTP monitor"
		}
	}
	if r.ConfiguredRole != "" {
		return "NTP → System → RTC"
	}
	if r.Role == "ptp" {
		return "PTP grandmaster → PHC → System (observed)"
	}
	if r.Role == "ntp" {
		return "NTP → System (observed)"
	}
	return "unknown"
}

func inferWarnings(r *Report) []string {
	var warnings []string
	if r.Health.Overall == HealthUnmanaged {
		warnings = append(warnings, "timesync state is absent; observed services may be managed externally")
	}
	if r.ManagementState == "error" {
		warnings = append(warnings, "timesync state could not be read; managed role is unknown")
	}
	if r.Chrony.Holdover {
		warnings = append(warnings, "chrony is serving local holdover time")
	}
	if r.PTP.PTP4LActive && !r.PTP.TimePropertiesAvailable {
		warnings = append(warnings, "PTP time properties are unavailable; PHC residual is unknown")
	}
	if r.ConfiguredPTP && strings.EqualFold(r.ConfiguredRole, "auto") && r.Health.PTPLink != HealthHealthy {
		warnings = append(warnings, fmt.Sprintf("optional PTP monitor is %s", r.Health.PTPLink))
	}
	guardExpected := r.ConfiguredPTP && (strings.EqualFold(r.ConfiguredRole, "client") || strings.EqualFold(r.ConfiguredRole, "master"))
	if guardExpected && r.Health.Guard != HealthHealthy {
		warnings = append(warnings, fmt.Sprintf("PTP runtime guard is %s", r.Health.Guard))
	}
	return warnings
}

func legacyHealth(state HealthState) string {
	switch state {
	case HealthHealthy:
		return "true"
	case HealthUnknown, HealthUnmanaged, "":
		return "unknown"
	default:
		return "false"
	}
}

func formatClockResidual(value int64) string {
	absolute := absInt64(value)
	switch {
	case absolute < 1_000:
		return fmt.Sprintf("%+d ns", value)
	case absolute < 1_000_000:
		return fmt.Sprintf("%+.3f µs", float64(value)/1_000)
	case absolute < 1_000_000_000:
		return fmt.Sprintf("%+.3f ms", float64(value)/1_000_000)
	default:
		return fmt.Sprintf("%+.6f s", float64(value)/1_000_000_000)
	}
}
