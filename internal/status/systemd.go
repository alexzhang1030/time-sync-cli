package status

import (
	"os/exec"
	"strings"
)

func collectSystemd() SystemdStatus {
	chrony := queryUnitStatus("chrony.service")
	chronyd := queryUnitStatus("chronyd.service")
	ntpDaemon := chooseNTPDaemon(chrony, chronyd)
	ptp4l := queryUnitStatus("ptp4l.service")
	phc2sys := queryUnitStatus("phc2sys.service")
	guard := queryUnitStatus("timesync-ptp-guard.timer")

	return SystemdStatus{
		Chronyd:     legacyActiveState(ntpDaemon),
		PTP4L:       legacyActiveState(ptp4l),
		PHC2Sys:     legacyActiveState(phc2sys),
		NTPDaemon:   ntpDaemon,
		PTP4LUnit:   ptp4l,
		PHC2SysUnit: phc2sys,
		GuardTimer:  guard,
	}
}

func queryUnitStatus(unit string) UnitStatus {
	status := UnitStatus{Unit: unit}
	out, err := exec.Command(
		"systemctl", "show", unit,
		"--property=Id,LoadState,ActiveState,UnitFileState",
	).CombinedOutput()
	values := parseSystemctlShow(string(out))
	if values["Id"] != "" {
		status.Unit = values["Id"]
	}
	status.LoadState = values["LoadState"]
	status.ActiveState = values["ActiveState"]
	status.UnitFileState = values["UnitFileState"]

	if status.LoadState == "not-found" {
		if status.ActiveState == "" {
			status.ActiveState = "inactive"
		}
		if status.UnitFileState == "" {
			status.UnitFileState = "not-found"
		}
		return status
	}
	if err != nil || status.ActiveState == "" {
		status.Detail = commandFailureDetail("systemctl show "+unit, out, errOrUnknown(err))
		status.LoadState = valueOr(status.LoadState, "unknown")
		status.ActiveState = "query-error"
		status.UnitFileState = valueOr(status.UnitFileState, "unknown")
	}
	return status
}

func parseSystemctlShow(output string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
}

func chooseNTPDaemon(units ...UnitStatus) UnitStatus {
	for _, unit := range units {
		if unit.ActiveState == "active" {
			return unit
		}
	}
	for _, unit := range units {
		if unit.LoadState == "loaded" {
			return unit
		}
	}
	for _, unit := range units {
		if unit.ActiveState == "query-error" {
			return unit
		}
	}
	if len(units) > 0 {
		return units[0]
	}
	return UnitStatus{Unit: "chrony.service", LoadState: "unknown", ActiveState: "query-error", UnitFileState: "unknown"}
}

func legacyActiveState(unit UnitStatus) string {
	if unit.ActiveState == "" {
		return "query-error"
	}
	return unit.ActiveState
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

type staticError string

func (e staticError) Error() string { return string(e) }

func errOrUnknown(err error) error {
	if err != nil {
		return err
	}
	return staticError("missing systemctl properties")
}
