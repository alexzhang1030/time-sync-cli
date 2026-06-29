package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
)

// Run starts the interactive configuration flow.
func Run() error {
	return RunWithIO(nil, nil)
}

// RunWithIO runs the TUI with custom input/output (for tests).
func RunWithIO(in io.Reader, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	reader := bufio.NewReader(in)

	fmt.Fprintln(out, "timesync — interactive setup")
	fmt.Fprintln(out, "")

	role, err := promptRole(reader, out)
	if err != nil {
		return err
	}

	report, err := detect.Run()
	if err != nil {
		return err
	}

	opts := model.ApplyOptions{Role: role}

	switch role {
	case model.RoleAuto:
		iface, err := promptInterface(reader, out, report.Interfaces, false)
		if err != nil {
			return err
		}
		opts.Iface = iface
		pool, err := promptLine(reader, out, "NTP pool", "pool.ntp.org")
		if err != nil {
			return err
		}
		opts.NTPPool = pool
		opts.PTP, err = promptYesNo(reader, out, "Enable PTP when hardware supports it?", false)
		if err != nil {
			return err
		}
	case model.RoleMaster:
		iface, err := promptInterface(reader, out, report.Interfaces, true)
		if err != nil {
			return err
		}
		opts.Iface = iface
		cidr, err := promptLine(reader, out, "NTP serve CIDR", "192.168.0.0/24")
		if err != nil {
			return err
		}
		opts.NTPServeCIDR = cidr
		opts.PTP, err = promptYesNo(reader, out, "Enable PTP grandmaster?", false)
		if err != nil {
			return err
		}
	case model.RoleClient:
		iface, err := promptInterface(reader, out, report.Interfaces, true)
		if err != nil {
			return err
		}
		opts.Iface = iface
		source, err := promptLine(reader, out, "Upstream host or IP", "")
		if err != nil {
			return err
		}
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("upstream source is required")
		}
		opts.Source = source
		opts.PTP, err = promptYesNo(reader, out, "Use PTP slave mode?", false)
		if err != nil {
			return err
		}
	}

	plan, err := planner.Plan(opts)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "")
	fmt.Fprint(out, planner.FormatPlan(plan))
	fmt.Fprintln(out, "")

	action, err := promptLine(reader, out, "Action (dry-run/apply/cancel)", "dry-run")
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "dry-run", "d", "":
		fmt.Fprintln(out, "(dry-run: no changes applied)")
		return nil
	case "apply", "a", "y", "yes":
		if err := apply.Apply(plan); err != nil {
			return err
		}
		fmt.Fprintln(out, "Configuration applied successfully.")
		return nil
	case "cancel", "c", "n", "no":
		fmt.Fprintln(out, "Cancelled.")
		return nil
	default:
		return fmt.Errorf("unknown action: %q", action)
	}
}

func promptRole(reader *bufio.Reader, out io.Writer) (model.Role, error) {
	fmt.Fprintln(out, "Select role:")
	fmt.Fprintln(out, "  1) auto   — internet time sync")
	fmt.Fprintln(out, "  2) master — serve local time")
	fmt.Fprintln(out, "  3) client — follow upstream source")
	choice, err := promptLine(reader, out, "Choice", "1")
	if err != nil {
		return "", err
	}
	return ParseRoleChoice(choice)
}

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

func promptInterface(reader *bufio.Reader, out io.Writer, ifaces []detect.InterfaceInfo, required bool) (string, error) {
	if len(ifaces) == 0 {
		if required {
			return "", fmt.Errorf("no network interfaces detected")
		}
		return promptLine(reader, out, "Network interface (optional)", "eth0")
	}

	fmt.Fprintln(out, "Available interfaces:")
	for i, iface := range ifaces {
		state := "down"
		if iface.Up {
			state = "up"
		}
		fmt.Fprintf(out, "  %d) %s (%s)\n", i+1, iface.Name, state)
	}
	label := "Interface number or name"
	if !required {
		label += " (optional, Enter for eth0)"
	}
	choice, err := promptLine(reader, out, label, "")
	if err != nil {
		return "", err
	}
	if !required && strings.TrimSpace(choice) == "" {
		return "eth0", nil
	}
	return ResolveInterfaceChoice(choice, ifaces)
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

func promptYesNo(reader *bufio.Reader, out io.Writer, question string, defaultNo bool) (bool, error) {
	def := "n"
	if !defaultNo {
		def = "y"
	}
	answer, err := promptLine(reader, out, question+" [y/N]", def)
	if err != nil {
		return false, err
	}
	return ParseYesNo(answer, defaultNo), nil
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

func promptLine(reader *bufio.Reader, out io.Writer, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultVal)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}
