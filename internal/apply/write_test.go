package apply

import (
	"os"
	"path/filepath"
	"testing"
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
