package status

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const chronyTrackingCSVFields = 14

func collectChrony(unit UnitStatus) ChronyStatus {
	s := ChronyStatus{Active: unit.ActiveState == "active", UnitState: unit.ActiveState}
	out, err := exec.Command("chronyc", "-c", "tracking").CombinedOutput()
	if err != nil {
		s.Detail = commandFailureDetail("chronyc -c tracking", out, err)
		return s
	}

	parsed, err := ParseChronyTrackingStatus(string(out))
	if err != nil {
		s.Detail = err.Error()
		return s
	}
	parsed.Active = s.Active
	parsed.UnitState = s.UnitState
	return parsed
}

// ParseChronyTrackingStatus parses the 14 CSV fields emitted by chronyc tracking.
// Field order source: https://gitlab.com/chrony/chrony/-/blob/master/client.c#L2330-2355
func ParseChronyTrackingStatus(line string) (ChronyStatus, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return ChronyStatus{}, fmt.Errorf("empty chronyc tracking response")
	}
	fields := strings.Split(line, ",")
	if len(fields) < chronyTrackingCSVFields {
		return ChronyStatus{}, fmt.Errorf("chronyc tracking returned %d fields; expected at least %d", len(fields), chronyTrackingCSVFields)
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}

	stratum, err := strconv.Atoi(fields[2])
	if err != nil {
		return ChronyStatus{}, fmt.Errorf("parse chrony stratum %q: %w", fields[2], err)
	}
	refTime, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return ChronyStatus{}, fmt.Errorf("parse chrony reference time %q: %w", fields[3], err)
	}

	referenceID := strings.ToUpper(fields[0])
	source := fields[1]
	leapStatus := fields[13]
	holdover := referenceID == "7F7F0101" || strings.EqualFold(source, "LOCAL")
	hasReference := referenceID != "" && referenceID != "00000000" && source != "" && source != "0.0.0.0" && source != "::"
	synchronized := hasReference && !holdover && chronyLeapIsSynchronized(leapStatus)

	return ChronyStatus{
		Available:    true,
		Synchronized: synchronized,
		Holdover:     holdover,
		Tracking:     line,
		ReferenceID:  referenceID,
		Source:       source,
		Stratum:      stratum,
		RefTimeUnix:  refTime,
		Offset:       fields[4],
		LastOffset:   fields[5],
		RMSOffset:    fields[6],
		SkewPPM:      fields[9],
		LeapStatus:   leapStatus,
	}, nil
}

// ParseChronyTracking preserves the original helper signature while returning
// the actual reference source and current system-time correction.
func ParseChronyTracking(line string) (source, offset string) {
	status, err := ParseChronyTrackingStatus(line)
	if err != nil {
		return "", ""
	}
	return status.Source, status.Offset
}

func chronyLeapIsSynchronized(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal", "insert second", "delete second":
		return true
	default:
		return false
	}
}

func commandFailureDetail(command string, output []byte, err error) string {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = err.Error()
	}
	return command + " failed: " + detail
}
