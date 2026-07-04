package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UninstallAction describes one cleanup action for timesync-managed config.
type UninstallAction struct {
	Kind   string
	Target string
	Detail string
}

// UninstallPlan describes the NTP/PTP role cleanup performed by uninstall.
type UninstallPlan struct {
	Actions []UninstallAction
}

// PlanUninstall builds a cleanup plan for timesync-managed NTP/PTP role config.
func PlanUninstall() (*UninstallPlan, error) {
	return DefaultApplier().PlanUninstall()
}

// PlanUninstall builds a cleanup plan for this applier.
func (a *Applier) PlanUninstall() (*UninstallPlan, error) {
	if a.ConfigDir == "" {
		a.ConfigDir = DefaultConfigDir
	}
	if a.SystemdDir == "" {
		a.SystemdDir = DefaultSystemdDir
	}

	plan := &UninstallPlan{}
	for _, action := range []UninstallAction{
		{Kind: "systemd", Target: "timesync-ptp-guard.timer", Detail: "stop"},
		{Kind: "systemd", Target: "phc2sys.service", Detail: "stop"},
		{Kind: "systemd", Target: "ptp4l.service", Detail: "stop"},
		{Kind: "systemd", Target: "timesync-ptp-guard.timer", Detail: "disable"},
		{Kind: "systemd", Target: "phc2sys.service", Detail: "disable"},
		{Kind: "systemd", Target: "ptp4l.service", Detail: "disable"},
	} {
		plan.Actions = append(plan.Actions, action)
	}

	for _, path := range a.timesyncDropInPaths() {
		if exists(path) {
			plan.Actions = append(plan.Actions, UninstallAction{Kind: "remove", Target: path, Detail: "timesync systemd drop-in"})
		}
	}
	for _, path := range a.timesyncUnitPaths() {
		managed, err := fileContains(path, "timesync-cli")
		if err != nil {
			return nil, err
		}
		if managed {
			plan.Actions = append(plan.Actions, UninstallAction{Kind: "remove", Target: path, Detail: "timesync systemd unit"})
		}
	}
	if exists(a.ConfigDir) {
		plan.Actions = append(plan.Actions, UninstallAction{Kind: "remove", Target: a.ConfigDir, Detail: "timesync config state"})
	}

	for _, action := range []UninstallAction{
		{Kind: "systemd", Target: "daemon-reload", Detail: "reload systemd"},
		{Kind: "systemd", Target: "chrony.service chronyd.service ptp4l.service phc2sys.service timesync-ptp-guard.service timesync-ptp-guard.timer", Detail: "reset-failed"},
		{Kind: "systemd", Target: "chrony.service", Detail: "try-restart"},
		{Kind: "systemd", Target: "chronyd.service", Detail: "try-restart"},
	} {
		plan.Actions = append(plan.Actions, action)
	}
	return plan, nil
}

// Uninstall removes timesync-managed role configuration and leaves the CLI installed.
func Uninstall() error {
	return DefaultApplier().Uninstall()
}

// Uninstall removes timesync-managed role configuration and restarts affected services.
func (a *Applier) Uninstall() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("uninstall requires root; re-run with sudo")
	}
	if a.ConfigDir == "" {
		a.ConfigDir = DefaultConfigDir
	}
	if a.SystemdDir == "" {
		a.SystemdDir = DefaultSystemdDir
	}

	_ = runSystemctl("stop", "timesync-ptp-guard.timer")
	_ = runSystemctl("stop", "phc2sys.service")
	_ = runSystemctl("stop", "ptp4l.service")
	_ = runSystemctl("disable", "timesync-ptp-guard.timer")
	_ = runSystemctl("disable", "phc2sys.service")
	_ = runSystemctl("disable", "ptp4l.service")

	for _, path := range a.timesyncDropInPaths() {
		if err := removeIfExists(path); err != nil {
			return err
		}
		removeEmptyParent(path, a.SystemdDir)
	}
	for _, path := range a.timesyncUnitPaths() {
		managed, err := fileContains(path, "timesync-cli")
		if err != nil {
			return err
		}
		if managed {
			if err := removeIfExists(path); err != nil {
				return err
			}
		}
	}
	if err := os.RemoveAll(a.ConfigDir); err != nil {
		return fmt.Errorf("remove %s: %w", a.ConfigDir, err)
	}

	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	_ = runSystemctl("reset-failed", "chrony.service", "chronyd.service", "ptp4l.service", "phc2sys.service", "timesync-ptp-guard.service", "timesync-ptp-guard.timer")
	_ = runSystemctl("try-restart", "chrony.service")
	_ = runSystemctl("try-restart", "chronyd.service")
	return nil
}

// FormatUninstallPlan renders an uninstall plan for CLI dry-runs.
func FormatUninstallPlan(plan *UninstallPlan) string {
	var b strings.Builder
	b.WriteString("Planned uninstall actions:\n")
	for i, action := range plan.Actions {
		fmt.Fprintf(&b, "%d. [%s] %s", i+1, action.Kind, action.Target)
		if action.Detail != "" {
			fmt.Fprintf(&b, " (%s)", action.Detail)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (a *Applier) timesyncDropInPaths() []string {
	return []string{
		filepath.Join(a.SystemdDir, "chrony.service.d", "timesync-cli.conf"),
		filepath.Join(a.SystemdDir, "chronyd.service.d", "timesync-cli.conf"),
		filepath.Join(a.SystemdDir, "ptp4l.service.d", "timesync-cli.conf"),
		filepath.Join(a.SystemdDir, "phc2sys.service.d", "timesync-cli.conf"),
	}
}

func (a *Applier) timesyncUnitPaths() []string {
	return []string{
		filepath.Join(a.SystemdDir, "ptp4l.service"),
		filepath.Join(a.SystemdDir, "phc2sys.service"),
		filepath.Join(a.SystemdDir, "timesync-ptp-guard.service"),
		filepath.Join(a.SystemdDir, "timesync-ptp-guard.timer"),
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func fileContains(path, marker string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return strings.Contains(string(data), marker), nil
}

func removeIfExists(path string) error {
	if !exists(path) {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func removeEmptyParent(path, keep string) {
	parent := filepath.Dir(path)
	if parent == keep || parent == "/" || parent == "." {
		return
	}
	if empty, _ := dirEmpty(parent); empty {
		_ = os.Remove(parent)
	}
}
