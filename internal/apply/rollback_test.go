package apply

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteWithBackup_CreatedFlag(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new.conf")
	backupDir := filepath.Join(dir, "backups")

	backupPath, created, err := writeWithBackup(target, "v1\n", backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("expected created=true for new file")
	}
	if backupPath != "" {
		t.Error("expected no backup for new file")
	}
}

func TestRestoreFromBackup(t *testing.T) {
	dir := t.TempDir()
	backup := filepath.Join(dir, "chrony.conf.bak")
	target := filepath.Join(dir, "chrony.conf")
	original := "pool ntp.org iburst\n"

	if err := os.WriteFile(backup, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreFromBackup(backup, target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("restored = %q, want %q", data, original)
	}
}

func TestRollback_RequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}
	a := DefaultApplier()
	if err := a.Rollback(); err == nil {
		t.Fatal("expected root error")
	}
}
