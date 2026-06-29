package tui

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func runFallback(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	fmt.Fprintln(out, "timesync — interactive setup")
	fmt.Fprintln(out, "")

	for {
		action, err := promptMainAction(reader, out)
		if err != nil {
			return err
		}
		switch action {
		case "doctor":
			if err := runDoctorFallback(out); err != nil {
				return err
			}
			if err := waitForContinue(reader, out); err != nil {
				return err
			}
		case "status":
			if err := runStatusFallback(out); err != nil {
				return err
			}
			if err := waitForContinue(reader, out); err != nil {
				return err
			}
		case "apply":
			return runApplyFallback(reader, out)
		case "quit":
			fmt.Fprintln(out, "Goodbye.")
			return nil
		}
		fmt.Fprintln(out, "")
	}
}

func promptMainAction(reader *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "Select action:")
	fmt.Fprintln(out, "  1) doctor — detect OS, tools, interfaces, PTP")
	fmt.Fprintln(out, "  2) status  — sync health report")
	fmt.Fprintln(out, "  3) apply   — configure and apply role")
	fmt.Fprintln(out, "  4) quit")
	choice, err := promptLine(reader, out, "Choice", "1")
	if err != nil {
		return "", err
	}
	return ParseMainAction(choice)
}

func runDoctorFallback(out io.Writer) error {
	report, err := detect.Run()
	fmt.Fprint(out, formatDoctorSummary(report, err))
	return err
}

func runStatusFallback(out io.Writer) error {
	report, err := status.Collect()
	if err != nil {
		return err
	}
	fmt.Fprint(out, report.Summary())
	return nil
}

func waitForContinue(reader *bufio.Reader, out io.Writer) error {
	_, err := promptLine(reader, out, "Press Enter to continue", "")
	return err
}

func runApplyFallback(reader *bufio.Reader, out io.Writer) error {
	role, err := promptRole(reader, out)
	if err != nil {
		return err
	}

	report, err := detect.Run()
	if err != nil {
		return err
	}

	opts := defaultApplyOpts(role)
	required := ifaceRequired(role)

	iface, err := promptInterface(reader, out, report.Interfaces, required)
	if err != nil {
		return err
	}
	opts.Iface = iface

	switch role {
	case model.RoleAuto:
		pool, err := promptLine(reader, out, "NTP pool", opts.NTPPool)
		if err != nil {
			return err
		}
		opts.NTPPool = pool
		opts.PTP, err = promptYesNo(reader, out, "Enable PTP when hardware supports it?", false)
		if err != nil {
			return err
		}
	case model.RoleMaster:
		cidr, err := promptLine(reader, out, "NTP serve CIDR", opts.NTPServeCIDR)
		if err != nil {
			return err
		}
		opts.NTPServeCIDR = cidr
		opts.PTP, err = promptYesNo(reader, out, "Enable PTP grandmaster?", false)
		if err != nil {
			return err
		}
	case model.RoleClient:
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

	plan, err := buildPlan(opts)
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

	if isApplyAction(action) && !opts.Yes {
		conflicts, err := apply.UnmanagedConflicts(plan)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, formatConflictSummary(conflicts))
			confirmed, err := promptYesNo(reader, out, "Overwrite these files?", true)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(out, "Aborted; no changes applied.")
				return nil
			}
			opts.Yes = true
		}
	}

	msg, err := executeApplyAction(opts, plan, action)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, msg)
	return nil
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

func promptInterface(reader *bufio.Reader, out io.Writer, ifaces []detect.InterfaceInfo, required bool) (string, error) {
	if len(ifaces) > 0 {
		fmt.Fprintln(out, "Available interfaces:")
		for i, iface := range ifaces {
			state := "down"
			if iface.Up {
				state = "up"
			}
			fmt.Fprintf(out, "  %d) %s (%s)\n", i+1, iface.Name, state)
		}
	}
	choice, err := promptLine(reader, out, interfaceLabel(ifaces, required), "")
	if err != nil {
		return "", err
	}
	return resolveInterfaceInput(choice, ifaces, required)
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
