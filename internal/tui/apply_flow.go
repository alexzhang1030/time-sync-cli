package tui

import (
	"fmt"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
)

func defaultApplyOpts(role model.Role) model.ApplyOptions {
	opts := model.ApplyOptions{Role: role}
	switch role {
	case model.RoleAuto:
		opts.NTPPool = "pool.ntp.org"
	case model.RoleMaster:
		opts.NTPServeCIDR = "192.168.0.0/24"
	}
	return opts
}

func buildPlan(opts model.ApplyOptions) (*model.Plan, error) {
	return planner.Plan(opts)
}

func executeApplyAction(opts model.ApplyOptions, plan *model.Plan, action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "dry-run", "d", "":
		return "(dry-run: no changes applied)", nil
	case "apply", "a", "y", "yes":
		if !opts.Yes {
			conflicts, err := apply.UnmanagedConflicts(plan)
			if err != nil {
				return "", err
			}
			if len(conflicts) > 0 {
				return "", fmt.Errorf("refusing to overwrite %d unmanaged file(s) without confirmation", len(conflicts))
			}
		}
		if err := apply.ValidatePTPHardware(opts); err != nil {
			return "", err
		}
		if err := apply.Apply(plan); err != nil {
			return "", err
		}
		return "Configuration applied successfully.", nil
	case "cancel", "c", "n", "no":
		return "Cancelled.", nil
	default:
		return "", fmt.Errorf("unknown action: %q", action)
	}
}

func isApplyAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "apply", "a", "y", "yes":
		return true
	default:
		return false
	}
}

func formatConflictSummary(conflicts []string) string {
	var b strings.Builder
	b.WriteString("These existing files are NOT managed by timesync and will be overwritten:\n")
	for _, p := range conflicts {
		fmt.Fprintf(&b, "  - %s\n", p)
	}
	b.WriteString("Existing contents are backed up under ")
	b.WriteString(apply.DefaultBackupDir)
	b.WriteString(" before overwrite.")
	return b.String()
}

func ifaceRequired(role model.Role) bool {
	return role == model.RoleMaster || role == model.RoleClient
}

func interfaceLabel(ifaces []detect.InterfaceInfo, required bool) string {
	label := "Interface number or name"
	if !required {
		label += " (optional, Enter for eth0)"
	}
	if len(ifaces) == 0 {
		if required {
			label = "Network interface"
		} else {
			label = "Network interface (optional)"
		}
	}
	return label
}

func resolveInterfaceInput(choice string, ifaces []detect.InterfaceInfo, required bool) (string, error) {
	if !required && strings.TrimSpace(choice) == "" {
		return "eth0", nil
	}
	if len(ifaces) == 0 {
		name := strings.TrimSpace(choice)
		if name == "" {
			return "", fmt.Errorf("network interface is required")
		}
		return name, nil
	}
	return ResolveInterfaceChoice(choice, ifaces)
}
