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
	wantDisabled := []string{"timesync-ptp-guard.timer", "phc2sys", "ptp4l"}
	if strings.Join(plan.DisableUnits, ",") != strings.Join(wantDisabled, ",") {
		t.Fatalf("DisableUnits = %v, want %v", plan.DisableUnits, wantDisabled)
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
	hasPTP4L, hasPHC2Sys, hasGuard := false, false, false
	for _, c := range plan.Changes {
		if c.Path == "/etc/systemd/system/ptp4l.service" {
			hasPTP4L = true
			if !strings.Contains(c.Content, "[Install]") {
				t.Error("expected install section in ptp4l service")
			}
			if !strings.Contains(c.Content, "ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --require-trusted-system-clock") {
				t.Error("expected PTP boot guard before ptp4l starts")
			}
			if strings.Contains(c.Content, "--repair-system-clock") {
				t.Error("auto PTP boot guard should keep chrony as system clock authority")
			}
		}
		if c.Path == "/etc/systemd/system/phc2sys.service" {
			hasPHC2Sys = true
		}
		if strings.Contains(c.Path, "timesync-ptp-guard") {
			hasGuard = true
		}
	}
	if !hasPTP4L || hasPHC2Sys || hasGuard {
		t.Errorf("expected only ptp4l PTP unit, ptp4l=%v phc2sys=%v guard=%v", hasPTP4L, hasPHC2Sys, hasGuard)
	}
	wantDisabled := []string{"timesync-ptp-guard.timer", "phc2sys"}
	if strings.Join(plan.DisableUnits, ",") != strings.Join(wantDisabled, ",") {
		t.Fatalf("DisableUnits = %v, want %v", plan.DisableUnits, wantDisabled)
	}
	formatted := planner.FormatPlan(plan)
	if !strings.Contains(formatted, "chrony as the only system clock discipline source") {
		t.Fatalf("formatted plan missing auto PTP single-writer warning:\n%s", formatted)
	}
}

func TestPlanMaster_NTP(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:         model.RoleMaster,
		Iface:        "eth0",
		NTPPool:      "cn.pool.ntp.org",
		NTPServeCIDR: "10.0.0.0/8",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "chrony.conf") {
			if !strings.Contains(c.Content, "pool cn.pool.ntp.org iburst") {
				t.Error("master chrony config missing upstream pool")
			}
			if !strings.Contains(c.Content, "makestep 1.0 3") {
				t.Error("master chrony config missing makestep")
			}
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
	wantDisabled := []string{"timesync-ptp-guard.timer", "phc2sys", "ptp4l"}
	if strings.Join(plan.DisableUnits, ",") != strings.Join(wantDisabled, ",") {
		t.Fatalf("DisableUnits = %v, want %v", plan.DisableUnits, wantDisabled)
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
			for _, want := range []string{
				"unicast_listen",
				"summary_interval      0",
				"logAnnounceInterval   0",
				"logSyncInterval       -3",
				"logMinDelayReqInterval -3",
				"tx_timestamp_timeout  100",
				"utc_offset            37",
				"clockAccuracy         0xFE",
				"offsetScaledLogVariance 0xFFFF",
				"timeSource            0x50",
			} {
				if !strings.Contains(c.Content, want) {
					t.Errorf("PTP master config missing %q", want)
				}
			}
		}
		if c.Path == "/etc/systemd/system/ptp4l.service" {
			if !strings.Contains(c.Content, "After=network-online.target chrony.service") {
				t.Error("expected PTP master to start after chrony")
			}
			if !strings.Contains(c.Content, "StartLimitIntervalSec=0") {
				t.Error("expected PTP master to keep retrying until chrony provides trusted time")
			}
			if !strings.Contains(c.Content, "ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --repair-system-clock") {
				t.Error("expected PTP master boot guard to seed PHC from trusted system time")
			}
			if strings.Contains(c.Content, "--require-trusted-system-clock") {
				t.Error("PTP master boot guard should use chrony-gated system time")
			}
		}
	}
	if !found {
		t.Error("expected PTP grandmaster config")
	}
	if len(plan.DisableUnits) != 0 {
		t.Fatalf("DisableUnits = %v, want none", plan.DisableUnits)
	}
}

func TestPlanMaster_PTPTimerGuard(t *testing.T) {
	plan, err := planner.Plan(model.ApplyOptions{
		Role:  model.RoleMaster,
		Iface: "eth0",
		PTP:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]string{
		"/etc/systemd/system/timesync-ptp-guard.service": "ExecStart=/usr/bin/timesync guard-ptp",
		"/etc/systemd/system/timesync-ptp-guard.timer":   "WantedBy=timers.target",
	}
	for path, want := range required {
		found := false
		for _, c := range plan.Changes {
			if c.Path == path {
				found = true
				if !strings.Contains(c.Content, want) {
					t.Errorf("%s missing %q", path, want)
				}
			}
		}
		if !found {
			t.Errorf("missing %s", path)
		}
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
		"/etc/timesync-cli/phc2sys.conf":      "-s CLOCK_REALTIME -c eth2 -w -S 1.0",
		"/etc/systemd/system/phc2sys.service": "ExecStart=/usr/sbin/phc2sys -s CLOCK_REALTIME -c eth2 -w -S 1.0",
	}
	for path, want := range required {
		found := false
		for _, c := range plan.Changes {
			if c.Path == path {
				found = true
				if !strings.Contains(c.Content, want) {
					t.Errorf("%s = %q, want %q", path, c.Content, want)
				}
				if path == "/etc/systemd/system/phc2sys.service" && !strings.Contains(c.Content, "ExecStartPost=/usr/bin/timesync publish-gm-time-properties --timeout 30s") {
					t.Errorf("%s missing verified GM time properties publication", path)
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
	foundChrony := false
	for _, c := range plan.Changes {
		if strings.Contains(c.Path, "chrony.conf") {
			foundChrony = true
			if !strings.Contains(c.Content, "server 192.168.1.1") {
				t.Error("client chrony missing server directive")
			}
			break
		}
	}
	if !foundChrony {
		t.Error("missing chrony client config")
	}
	wantDisabled := []string{"timesync-ptp-guard.timer", "phc2sys", "ptp4l"}
	if strings.Join(plan.DisableUnits, ",") != strings.Join(wantDisabled, ",") {
		t.Fatalf("DisableUnits = %v, want %v", plan.DisableUnits, wantDisabled)
	}
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
			if !strings.Contains(c.Content, "first_step_threshold  1.0") {
				t.Error("expected initial PTP step threshold")
			}
			if !strings.Contains(c.Content, "step_threshold        1.0") {
				t.Error("expected PTP step threshold")
			}
			if !strings.Contains(c.Content, "clientOnly            1") {
				t.Error("expected clientOnly in PTP client config")
			}
			if strings.Contains(c.Content, "\naddress") {
				t.Error("unexpected legacy address line in unicast master table")
			}
			if strings.Contains(c.Content, "clock_servo") {
				t.Error("PTP client config should use the linuxptp default servo")
			}
		}
	}
	if ptp4lCount != 1 {
		t.Errorf("expected exactly one ptp4l.conf change, got %d", ptp4lCount)
	}
	wantDisabled := []string{"chrony", "chronyd"}
	if strings.Join(plan.DisableUnits, ",") != strings.Join(wantDisabled, ",") {
		t.Fatalf("DisableUnits = %v, want %v", plan.DisableUnits, wantDisabled)
	}
	formatted := planner.FormatPlan(plan)
	if !strings.Contains(formatted, "Systemd units to disable:") || !strings.Contains(formatted, "- chrony") || !strings.Contains(formatted, "- chronyd") {
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
		"/etc/systemd/system/ptp4l.service":              "ExecStart=/usr/sbin/ptp4l -f /etc/timesync-cli/ptp4l.conf",
		"/etc/systemd/system/phc2sys.service":            "ExecStart=/usr/sbin/phc2sys -f /etc/timesync-cli/ptp4l.conf -s eth0 -w -R 8 -N 5 -S 1.0",
		"/etc/systemd/system/timesync-ptp-guard.service": "ExecStart=/usr/bin/timesync guard-ptp",
		"/etc/systemd/system/timesync-ptp-guard.timer":   "OnUnitActiveSec=5s",
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
				if path == "/etc/systemd/system/ptp4l.service" && !strings.Contains(c.Content, "ExecStartPre=/usr/bin/timesync boot-guard --iface eth0 --repair-system-clock") {
					t.Errorf("%s missing boot guard", path)
				}
				if path == "/etc/systemd/system/phc2sys.service" && !strings.Contains(c.Content, "ExecStartPre=/usr/bin/timesync wait-ptp --timeout 30s") {
					t.Errorf("%s missing PTP health wait", path)
				}
				if path == "/etc/systemd/system/timesync-ptp-guard.timer" && !strings.Contains(c.Content, "WantedBy=timers.target") {
					t.Errorf("%s missing timer install target", path)
				}
				if path != "/etc/systemd/system/timesync-ptp-guard.service" && !strings.Contains(c.Content, "[Install]") {
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
