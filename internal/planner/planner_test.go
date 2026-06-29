package planner_test

import (
	"strings"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
)

func TestPlanAuto_DryRun(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:    model.RoleAuto,
		Iface:   "eth0",
		NTPPool: "pool.ntp.org",
		DryRun:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Role != model.RoleAuto {
		t.Errorf("role = %s, want auto", plan.Role)
	}
	if len(plan.Changes) == 0 {
		t.Fatal("expected planned changes")
	}
	found := false
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "chrony.conf") {
			found = true
			if !strings.Contains(c.Content, "pool.ntp.org") {
				t.Error("chrony config missing pool")
			}
		}
	}
	if !found {
		t.Error("missing chrony config change")
	}
}

func TestPlanAuto_WithPTP(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:  model.RoleAuto,
		Iface: "eth0",
		PTP:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.PTP {
		t.Error("expected PTP enabled in plan")
	}
	ptpFound := false
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "ptp4l.conf") {
			ptpFound = true
		}
	}
	if !ptpFound {
		t.Error("expected ptp4l config in auto+ptp plan")
	}
}

func TestPlanMaster_NTP(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:         model.RoleMaster,
		Iface:        "eth0",
		NTPServeCIDR: "10.0.0.0/8",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "chrony.conf") {
			if !strings.Contains(c.Content, "allow 10.0.0.0/8") {
				t.Error("master chrony config missing allow directive")
			}
			return
		}
	}
	t.Error("missing chrony master config")
}

func TestPlanMaster_PTP(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:  model.RoleMaster,
		Iface: "eth0",
		PTP:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range plan.Changes {
		if strings.Contains(c.Content, "clockClass") {
			found = true
		}
	}
	if !found {
		t.Error("expected PTP grandmaster config")
	}
}

func TestPlanClient_NTP(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:   model.RoleClient,
		Iface:  "eth0",
		Source: "192.168.1.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "chrony.conf") {
			if !strings.Contains(c.Content, "server 192.168.1.1") {
				t.Error("client chrony missing server directive")
			}
			return
		}
	}
	t.Error("missing chrony client config")
}

func TestPlanClient_PTP(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:   model.RoleClient,
		Iface:  "eth0",
		Source: "192.168.1.1",
		PTP:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "ptp4l.conf") {
			found = true
		}
	}
	if !found {
		t.Error("expected ptp4l client config")
	}
}

func TestPlanClient_MissingSource(t *testing.T) {
	_, err := planner.Plan(model.ApplyOptions{
		Role:  model.RoleClient,
		Iface: "eth0",
	})
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestPlanMaster_MissingIface(t *testing.T) {
	_, err := planner.Plan(model.ApplyOptions{
		Role: model.RoleMaster,
	})
	if err == nil {
		t.Error("expected error for missing iface")
	}
}
