package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

const (
	DefaultConfigDir = "/etc/timesync-cli"
	DefaultBackupDir = "/etc/timesync-cli/backups"
)

// Applier writes planned changes and reloads systemd services.
type Applier struct {
	ConfigDir string
	BackupDir string
}

// State records the last applied role configuration.
type State struct {
	Role      model.Role        `json:"role"`
	Iface     string            `json:"iface,omitempty"`
	PTP       bool              `json:"ptp"`
	AppliedAt string            `json:"applied_at"`
	Backups   map[string]string `json:"backups,omitempty"`
	Created   []string          `json:"created,omitempty"`
}

// LoadState reads the last applied configuration from disk.
func LoadState(configDir string) (*State, error) {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	data, err := os.ReadFile(filepath.Join(configDir, "state.json"))
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Apply executes a plan with backup, config writes, and systemd reload.
func Apply(plan *model.Plan) error {
	return DefaultApplier().Apply(plan)
}

// DefaultApplier returns an applier using system paths.
func DefaultApplier() *Applier {
	return &Applier{
		ConfigDir: DefaultConfigDir,
		BackupDir: DefaultBackupDir,
	}
}

// Apply writes files, saves state, and restarts affected units.
func (a *Applier) Apply(plan *model.Plan) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("apply requires root; re-run with sudo")
	}
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}

	if err := os.MkdirAll(a.ConfigDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.MkdirAll(a.BackupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	backups := make(map[string]string)
	var created []string

	for _, change := range plan.Changes {
		if change.Path == "" || change.Content == "" {
			continue
		}
		backupPath, wasCreated, err := writeWithBackup(change.Path, change.Content, a.BackupDir)
		if err != nil {
			return fmt.Errorf("write %s: %w", change.Path, err)
		}
		if backupPath != "" {
			backups[change.Path] = backupPath
		}
		if wasCreated {
			created = append(created, change.Path)
		}
	}

	if err := a.writeState(plan, backups, created); err != nil {
		return err
	}

	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}

	for _, unit := range UnitsForPlan(plan) {
		if err := runSystemctl("enable", unit); err != nil {
			return fmt.Errorf("systemctl enable %s: %w", unit, err)
		}
		if err := runSystemctl("restart", unit); err != nil {
			return fmt.Errorf("systemctl restart %s: %w", unit, err)
		}
	}

	return nil
}

func (a *Applier) writeState(plan *model.Plan, backups map[string]string, created []string) error {
	statePath := filepath.Join(a.ConfigDir, "state.json")

	state := State{
		Role:      plan.Role,
		Iface:     plan.Iface,
		PTP:       plan.PTP,
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Backups:   backups,
		Created:   created,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	backupPath, wasCreated, err := writeWithBackup(statePath, string(data)+"\n", a.BackupDir)
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if backupPath != "" {
		state.Backups[statePath] = backupPath
	}
	if wasCreated {
		state.Created = append(state.Created, statePath)
	}

	if len(state.Backups) != len(backups) || (wasCreated && !contains(created, statePath)) {
		data, err = json.MarshalIndent(state, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal state: %w", err)
		}
		if err := os.WriteFile(statePath, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("update state metadata: %w", err)
		}
	}
	return nil
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

// UnitsForPlan returns systemd units that should be enabled and restarted.
func UnitsForPlan(plan *model.Plan) []string {
	seen := make(map[string]bool)
	var units []string
	for _, change := range plan.Changes {
		unit := unitFromChangePath(change.Path)
		if unit == "" || seen[unit] {
			continue
		}
		seen[unit] = true
		units = append(units, unit)
	}
	return units
}

func unitFromChangePath(path string) string {
	switch {
	case strings.Contains(path, "chronyd.service.d"):
		return "chronyd"
	case strings.Contains(path, "ptp4l.service.d"):
		return "ptp4l"
	case strings.Contains(path, "phc2sys.service.d"):
		return "phc2sys"
	default:
		return ""
	}
}

func writeWithBackup(path, content, backupDir string) (backupPath string, created bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			return "", false, err
		}
		name := fmt.Sprintf("%s.%s.bak", filepath.Base(path), time.Now().Format("20060102-150405"))
		backupPath = filepath.Join(backupDir, name)
		if err := os.WriteFile(backupPath, existing, 0o644); err != nil {
			return "", false, fmt.Errorf("backup %s: %w", path, err)
		}
	} else if os.IsNotExist(err) {
		created = true
	} else {
		return "", false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return backupPath, created, err
	}
	return backupPath, created, nil
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
