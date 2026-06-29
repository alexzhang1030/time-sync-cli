package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

// ParseRoleChoice maps user input to a role.
func ParseRoleChoice(choice string) (model.Role, error) {
	switch strings.TrimSpace(choice) {
	case "1", "auto":
		return model.RoleAuto, nil
	case "2", "master":
		return model.RoleMaster, nil
	case "3", "client":
		return model.RoleClient, nil
	default:
		return "", fmt.Errorf("invalid role choice: %q", choice)
	}
}

// ResolveInterfaceChoice maps user input to an interface name.
func ResolveInterfaceChoice(choice string, ifaces []detect.InterfaceInfo) (string, error) {
	choice = strings.TrimSpace(choice)
	for _, iface := range ifaces {
		if iface.Name == choice {
			return iface.Name, nil
		}
	}
	var idx int
	if _, err := fmt.Sscanf(choice, "%d", &idx); err == nil && idx >= 1 && idx <= len(ifaces) {
		return ifaces[idx-1].Name, nil
	}
	return "", fmt.Errorf("invalid interface choice: %q", choice)
}

// ParseYesNo interprets y/n input.
func ParseYesNo(answer string, defaultVal bool) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultVal
	}
}

// ParseMainAction maps user input to a main-menu action.
func ParseMainAction(choice string) (string, error) {
	switch strings.TrimSpace(choice) {
	case "1", "doctor", "d":
		return "doctor", nil
	case "2", "status", "s":
		return "status", nil
	case "3", "apply", "a":
		return "apply", nil
	case "4", "quit", "q", "exit":
		return "quit", nil
	default:
		return "", fmt.Errorf("invalid action: %q", choice)
	}
}

func isInteractiveTTY(in io.Reader) bool {
	if in != os.Stdin {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func formatDoctorSummary(report *detect.Report, err error) string {
	if err != nil {
		return fmt.Sprintf("Doctor failed: %v\n", err)
	}
	text := report.Summary()
	if missing := report.MissingBinaries(); len(missing) > 0 {
		text += fmt.Sprintf("\nWarning: missing binaries: %v\n", missing)
	}
	return text
}
