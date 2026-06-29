package apply

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Rollback restores files from the last apply recorded in state.json.
func Rollback() error {
	return DefaultApplier().Rollback()
}

// Rollback restores backed-up files and removes paths created by the last apply.
func (a *Applier) Rollback() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("rollback requires root; re-run with sudo")
	}

	state, err := LoadState(a.ConfigDir)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if len(state.Backups) == 0 && len(state.Created) == 0 {
		return fmt.Errorf("no rollback data in state; nothing to restore")
	}

	for target, backup := range state.Backups {
		if err := restoreFromBackup(backup, target); err != nil {
			return fmt.Errorf("restore %s: %w", target, err)
		}
	}

	for _, path := range state.Created {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove created file %s: %w", path, err)
		}
		parent := filepath.Dir(path)
		if empty, _ := dirEmpty(parent); empty && parent != a.ConfigDir && parent != "/" {
			_ = os.Remove(parent)
		}
	}

	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	for _, unit := range unitsFromPaths(append(mapKeys(state.Backups), state.Created...)) {
		_ = runSystemctl("try-restart", unit)
	}

	return nil
}

func restoreFromBackup(backupPath, targetPath string) error {
	src, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func unitsFromPaths(paths []string) []string {
	seen := make(map[string]bool)
	var units []string
	for _, path := range paths {
		unit := unitFromChangePath(path)
		if unit == "" || seen[unit] {
			continue
		}
		seen[unit] = true
		units = append(units, unit)
	}
	return units
}
