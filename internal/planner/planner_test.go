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
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "chrony.service.d") {
			return
		}
	}
	t.Error("missing chrony.service drop-in")
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
	hasPTP4L, hasPHC2Sys := false, false
	for _, c := range plan.Changes {
		if c.Path == "/etc/systemd/system/ptp4l.service" {
			hasPTP4L = true
			if !strings.Contains(c.Content, "[Install]") {
				t.Error("expected install section in ptp4l service")
			}
		}
		if c.Path == "/etc/systemd/system/phc2sys.service" {
			hasPHC2Sys = true
			if !strings.Contains(c.Content, "Requires=ptp4l.service") {
				t.Error("expected phc2sys service to require ptp4l")
			}
		}
	}
	if !hasPTP4L || !hasPHC2Sys {
		t.Errorf("expected ptp4l and phc2sys service units, ptp4l=%v phc2sys=%v", hasPTP4L, hasPHC2Sys)
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
		}
	}
	hasChronyConfig, hasChronyDropIn := false, false
	for _, c := range plan.Changes {
		if c.Path == "/etc/timesync-cli/chrony.conf" {
			hasChronyConfig = true
		}
		if c.Path == "/etc/systemd/system/chrony.service.d/timesync-cli.conf" {
			hasChronyDropIn = true
		}
	}
	if !hasChronyConfig {
		t.Error("missing chrony master config")
	}
	if !hasChronyDropIn {
		t.Error("missing chrony.service drop-in")
	}
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
			if !strings.Contains(c.Content, "unicast_listen") {
				t.Error("expected PTP master to listen for unicast clients")
			}
		}
	}
	if !found {
		t.Error("expected PTP grandmaster config")
	}
}

func TestPlanMaster_PTPPhc2sysSyncsPHCFromSystemClock(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:  model.RoleMaster,
		Iface: "eth2",
		PTP:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]string{
		"/etc/timesync-cli/phc2sys.conf":      "-s CLOCK_REALTIME -c eth2 -w",
		"/etc/systemd/system/phc2sys.service": "ExecStart=/usr/sbin/phc2sys -s CLOCK_REALTIME -c eth2 -w",
	}
	for path, want := range required {
		found := false
		for _, c := range plan.Changes {
			if c.Path == path {
				found = true
				if !strings.Contains(c.Content, want) {
					t.Errorf("%s = %q, want %q", path, c.Content, want)
				}
				if strings.Contains(c.Content, "-f /etc/timesync-cli/ptp4l.conf -s eth2 -w") {
					t.Errorf("%s uses client phc2sys direction", path)
				}
			}
		}
		if !found {
			t.Errorf("missing %s", path)
		}
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
	ptp4lCount := 0
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "ptp4l.conf") {
			ptp4lCount++
			if !strings.Contains(c.Content, "unicast_listen") {
				t.Error("expected unicast_listen in PTP client config")
			}
			if !strings.Contains(c.Content, "192.168.1.1") {
				t.Error("expected source in unicast_master_table")
			}
			if !strings.Contains(c.Content, "table_id              1") {
				t.Error("expected unicast master table_id")
			}
			if !strings.Contains(c.Content, "UDPv4                 192.168.1.1") {
				t.Error("expected linuxptp UDPv4 unicast master entry")
			}
			if !strings.Contains(c.Content, "unicast_master_table  1") {
				t.Error("expected interface to reference unicast master table")
			}
			if strings.Contains(c.Content, "\naddress") {
				t.Error("unexpected legacy address line in unicast master table")
			}
		}
	}
	if ptp4lCount != 1 {
		t.Errorf("expected exactly one ptp4l.conf change, got %d", ptp4lCount)
	}
	if len(plan.DisableUnits) != 1 || plan.DisableUnits[0] != "chrony" {
		t.Fatalf("DisableUnits = %v, want [chrony]", plan.DisableUnits)
	}
	formatted := planner.FormatPlan(plan)
	if !strings.Contains(formatted, "Systemd units to disable:") || !strings.Contains(formatted, "- chrony") {
		t.Fatalf("formatted plan missing chrony disable action:\n%s", formatted)
	}
	if !strings.Contains(formatted, "phc2sys is the only system clock discipline source") {
		t.Fatalf("formatted plan missing PTP client warning:\n%s", formatted)
	}
}

func TestPlanClient_PTPSystemdUnitsAreEnableable(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:   model.RoleClient,
		Iface:  "eth0",
		Source: "192.168.71.51",
		PTP:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]string{
		"/etc/systemd/system/ptp4l.service":   "ExecStart=/usr/sbin/ptp4l -f /etc/timesync-cli/ptp4l.conf",
		"/etc/systemd/system/phc2sys.service": "ExecStart=/usr/sbin/phc2sys -f /etc/timesync-cli/ptp4l.conf -s eth0 -w",
	}
	for path, execStart := range required {
		found := false
		for _, c := range plan.Changes {
			if c.Path == path {
				found = true
				if !strings.Contains(c.Content, "[Unit]") {
					t.Errorf("%s missing unit section", path)
				}
				if !strings.Contains(c.Content, execStart) {
					t.Errorf("%s missing ExecStart", path)
				}
				if !strings.Contains(c.Content, "[Install]") {
					t.Errorf("%s missing install section", path)
				}
			}
		}
		if !found {
			t.Errorf("missing %s", path)
		}
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
