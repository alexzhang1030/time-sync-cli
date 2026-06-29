package status

import "strings"

// ParsePMCFields extracts key/value pairs from pmc management output.
// Lines look like "        portState               SLAVE".
func ParsePMCFields(output string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "sending:") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := parts[0]
		value := strings.Join(parts[1:], " ")
		fields[key] = value
	}
	return fields
}

// PTPMetrics holds parsed PTP fields from pmc queries.
type PTPMetrics struct {
	PortState        string
	MasterOffset     string
	OffsetFromMaster string
	PathDelay        string
	StepsRemoved     string
	GMIdentity       string
}

// ParsePTPMetrics merges fields from PORT_DATA_SET, TIME_STATUS_NP, and CURRENT_DATA_SET.
func ParsePTPMetrics(portDataSet, timeStatusNP, currentDataSet string) PTPMetrics {
	port := ParsePMCFields(portDataSet)
	timeNP := ParsePMCFields(timeStatusNP)
	current := ParsePMCFields(currentDataSet)
	return PTPMetrics{
		PortState:        port["portState"],
		MasterOffset:     timeNP["master_offset"],
		OffsetFromMaster: current["offsetFromMaster"],
		PathDelay:        current["meanPathDelay"],
		StepsRemoved:     current["stepsRemoved"],
		GMIdentity:       timeNP["gmIdentity"],
	}
}

// PTPOffset returns the best available offset string with units, or empty.
func (m PTPMetrics) PTPOffset() string {
	if m.MasterOffset != "" {
		return m.MasterOffset + " ns"
	}
	if m.OffsetFromMaster != "" {
		return m.OffsetFromMaster + " ns"
	}
	return ""
}
