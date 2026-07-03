package apply

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

func TestWriteWithBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested", "chrony.conf")
	backupDir := filepath.Join(dir, "backups")

	if _, _, err := writeWithBackup(target, "v1\n", backupDir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeWithBackup(target, "v2\n", backupDir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2\n" {
		t.Fatalf("content = %q", data)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}
}

func TestRemoveLegacyPTPDropIns(t *testing.T) {
	dir := t.TempDir()
	systemdDir := filepath.Join(dir, "systemd")
	legacyPHC := filepath.Join(systemdDir, "phc2sys.service.d", "timesync-cli.conf")
	legacyPTP := filepath.Join(systemdDir, "ptp4l.service.d", "timesync-cli.conf")
	otherDropIn := filepath.Join(systemdDir, "phc2sys.service.d", "local.conf")
	for _, path := range []string{legacyPHC, legacyPTP, otherDropIn} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := &Applier{SystemdDir: systemdDir}
	err := a.removeLegacyPTPDropIns(&model.Plan{
		Changes: []model.PlannedChange{{Path: filepath.Join(systemdDir, "phc2sys.service")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{legacyPHC, legacyPTP} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists", path)
		}
	}
	if _, err := os.Stat(otherDropIn); err != nil {
		t.Fatalf("other drop-in removed: %v", err)
	}
}
