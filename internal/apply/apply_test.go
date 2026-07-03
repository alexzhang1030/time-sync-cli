package apply_test

import (
	"os"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

func TestUnitsForPlan_ChronyOnly(t *testing.T) {
	plan := &model.Plan{
		Changes: []model.PlannedChange{
			{Path: "/etc/timesync-cli/chrony.conf"},
			{Path: "/etc/systemd/system/chrony.service.d/timesync-cli.conf"},
		},
	}
	units := apply.UnitsForPlan(plan)
	if len(units) != 1 || units[0] != "chrony" {
		t.Fatalf("units = %v, want [chrony]", units)
	}
}

func TestUnitsForPlan_ChronydDropInCompatibility(t *testing.T) {
	plan := &model.Plan{
		Changes: []model.PlannedChange{
			{Path: "/etc/systemd/system/chronyd.service.d/timesync-cli.conf"},
		},
	}
	units := apply.UnitsForPlan(plan)
	if len(units) != 1 || units[0] != "chrony" {
		t.Fatalf("units = %v, want [chrony]", units)
	}
}

func TestUnitsForPlan_PTP(t *testing.T) {
	plan := &model.Plan{
		Changes: []model.PlannedChange{
			{Path: "/etc/systemd/system/ptp4l.service"},
			{Path: "/etc/systemd/system/phc2sys.service"},
		},
	}
	units := apply.UnitsForPlan(plan)
	if len(units) != 2 {
		t.Fatalf("units = %v, want ptp4l and phc2sys", units)
	}
}

func TestUnitsForPlan_PTPDropInCompatibility(t *testing.T) {
	plan := &model.Plan{
		Changes: []model.PlannedChange{
			{Path: "/etc/systemd/system/ptp4l.service.d/timesync-cli.conf"},
			{Path: "/etc/systemd/system/phc2sys.service.d/timesync-cli.conf"},
		},
	}
	units := apply.UnitsForPlan(plan)
	if len(units) != 2 {
		t.Fatalf("units = %v, want ptp4l and phc2sys", units)
	}
}

func TestApplier_RequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	a := apply.DefaultApplier()
	err := a.Apply(&model.Plan{Role: model.RoleAuto})
	if err == nil {
		t.Fatal("expected root error")
	}
}
