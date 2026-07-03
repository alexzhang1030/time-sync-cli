package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanUninstallIncludesManagedRoleConfig(t *testing.T) {
	dir := t.TempDir()
	systemdDir := filepath.Join(dir, "systemd")
	configDir := filepath.Join(dir, "timesync-cli")

	mustWrite(t, filepath.Join(systemdDir, "chrony.service.d", "timesync-cli.conf"), "[Service]\n")
	mustWrite(t, filepath.Join(systemdDir, "ptp4l.service"), "Description=Precision Time Protocol configured by timesync-cli\n")
	mustWrite(t, filepath.Join(systemdDir, "phc2sys.service"), "Description=PHC to system clock sync configured by timesync-cli\n")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := &Applier{ConfigDir: configDir, SystemdDir: systemdDir}
	plan, err := a.PlanUninstall()
	if err != nil {
		t.Fatal(err)
	}
	out := FormatUninstallPlan(plan)

	for _, want := range []string{
		"phc2sys.service (stop)",
		"ptp4l.service (disable)",
		filepath.Join(systemdDir, "chrony.service.d", "timesync-cli.conf"),
		filepath.Join(systemdDir, "ptp4l.service"),
		filepath.Join(systemdDir, "phc2sys.service"),
		configDir,
		"chrony.service (try-restart)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("plan missing %q:\n%s", want, out)
		}
	}
}

func TestPlanUninstallSkipsUnmanagedPTPUnits(t *testing.T) {
	dir := t.TempDir()
	systemdDir := filepath.Join(dir, "systemd")
	mustWrite(t, filepath.Join(systemdDir, "ptp4l.service"), "Description=Distribution PTP service\n")

	a := &Applier{ConfigDir: filepath.Join(dir, "timesync-cli"), SystemdDir: systemdDir}
	plan, err := a.PlanUninstall()
	if err != nil {
		t.Fatal(err)
	}
	out := FormatUninstallPlan(plan)
	if strings.Contains(out, filepath.Join(systemdDir, "ptp4l.service")+" ") {
		t.Fatalf("plan removed unmanaged unit:\n%s", out)
	}
}

func TestUninstallRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	a := &Applier{ConfigDir: t.TempDir(), SystemdDir: t.TempDir()}
	if err := a.Uninstall(); err == nil {
		t.Fatal("expected root error")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
