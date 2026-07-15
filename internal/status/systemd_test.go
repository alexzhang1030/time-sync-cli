package status

import "testing"

func TestParseSystemctlShow(t *testing.T) {
	values := parseSystemctlShow(`Id=chrony.service
LoadState=loaded
ActiveState=active
UnitFileState=enabled
`)
	if values["Id"] != "chrony.service" || values["LoadState"] != "loaded" || values["ActiveState"] != "active" || values["UnitFileState"] != "enabled" {
		t.Fatalf("values = %#v", values)
	}
}

func TestChooseNTPDaemonPrefersActiveThenLoaded(t *testing.T) {
	chrony := UnitStatus{Unit: "chrony.service", LoadState: "loaded", ActiveState: "inactive"}
	chronyd := UnitStatus{Unit: "chronyd.service", LoadState: "loaded", ActiveState: "active"}
	if got := chooseNTPDaemon(chrony, chronyd); got.Unit != "chronyd.service" {
		t.Fatalf("unit = %q", got.Unit)
	}
	chronyd.ActiveState = "inactive"
	if got := chooseNTPDaemon(chrony, chronyd); got.Unit != "chrony.service" {
		t.Fatalf("unit = %q", got.Unit)
	}
}
