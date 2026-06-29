package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
	"github.com/alexzhang1030/time-sync-cli/internal/status"
	"github.com/alexzhang1030/time-sync-cli/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "timesync",
	Short: "Linux time sync CLI for NTP/PTP management",
	Long:  "Hide NTP/PTP complexity behind simple configuration flows for robot, industrial PC, and embedded Linux deployments.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(applyCmd())
	rootCmd.AddCommand(rollbackCmd())
	rootCmd.AddCommand(tuiCmd())
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Detect OS, init system, binaries, interfaces, and PTP capability",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := detect.Run()
			if err != nil {
				return err
			}
			fmt.Print(report.Summary())
			if missing := report.MissingBinaries(); len(missing) > 0 {
				fmt.Fprintf(os.Stderr, "\nWarning: missing binaries: %v\n", missing)
			}
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report sync health, role, NTP/PTP offset, port state, and service state",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := status.Collect()
			if err != nil {
				return err
			}
			fmt.Print(report.Summary())
			return nil
		},
	}
}

func applyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a time sync role configuration",
	}
	cmd.AddCommand(applyAutoCmd())
	cmd.AddCommand(applyMasterCmd())
	cmd.AddCommand(applyClientCmd())
	return cmd
}

func applyAutoCmd() *cobra.Command {
	var iface, ntpPool string
	var ptp, dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Configure internet time sync (NTP baseline, optional PTP)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(model.ApplyOptions{
				Role:    model.RoleAuto,
				Iface:   iface,
				NTPPool: ntpPool,
				PTP:     ptp,
				DryRun:  dryRun,
				Yes:     yes,
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface for PTP")
	cmd.Flags().StringVar(&ntpPool, "ntp-pool", "pool.ntp.org", "NTP pool server")
	cmd.Flags().BoolVar(&ptp, "ptp", false, "enable PTP when hardware supports it")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned changes without applying")
	cmd.Flags().BoolVar(&yes, "yes", false, "assume yes; overwrite non-timesync config files without prompting")
	return cmd
}

func applyMasterCmd() *cobra.Command {
	var iface, ntpServeCIDR string
	var ptp, dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "master",
		Short: "Serve local time to downstream devices (NTP and optional PTP grandmaster)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(model.ApplyOptions{
				Role:         model.RoleMaster,
				Iface:        iface,
				NTPServeCIDR: ntpServeCIDR,
				PTP:          ptp,
				DryRun:       dryRun,
				Yes:          yes,
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (required)")
	cmd.Flags().StringVar(&ntpServeCIDR, "ntp-serve-cidr", "", "CIDR to allow NTP serving")
	cmd.Flags().BoolVar(&ptp, "ptp", false, "enable PTP grandmaster mode")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned changes without applying")
	cmd.Flags().BoolVar(&yes, "yes", false, "assume yes; overwrite non-timesync config files without prompting")
	_ = cmd.MarkFlagRequired("iface")
	return cmd
}

func applyClientCmd() *cobra.Command {
	var iface, source string
	var ptp, dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Follow an upstream time source (NTP or PTP slave)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(model.ApplyOptions{
				Role:   model.RoleClient,
				Iface:  iface,
				Source: source,
				PTP:    ptp,
				DryRun: dryRun,
				Yes:    yes,
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (required)")
	cmd.Flags().StringVar(&source, "source", "", "upstream host or IP (required)")
	cmd.Flags().BoolVar(&ptp, "ptp", false, "use PTP slave mode")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned changes without applying")
	cmd.Flags().BoolVar(&yes, "yes", false, "assume yes; overwrite non-timesync config files without prompting")
	_ = cmd.MarkFlagRequired("iface")
	_ = cmd.MarkFlagRequired("source")
	return cmd
}

func runApply(opts model.ApplyOptions) error {
	plan, err := planner.Plan(opts)
	if err != nil {
		return err
	}
	fmt.Print(planner.FormatPlan(plan))
	if opts.DryRun {
		fmt.Println("\n(dry-run: no changes applied)")
		return nil
	}

	conflicts, err := apply.UnmanagedConflicts(plan)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		proceed, err := confirmOverwrite(conflicts, opts.Yes)
		if err != nil {
			return err
		}
		if !proceed {
			fmt.Println("\nAborted; no changes applied.")
			return nil
		}
	}

	if err := apply.ValidatePTPHardware(opts); err != nil {
		return err
	}
	if err := apply.Apply(plan); err != nil {
		return err
	}
	fmt.Println("\nConfiguration applied successfully.")
	return nil
}

// confirmOverwrite shows the unmanaged files that would be overwritten and
// decides whether to proceed: --yes always proceeds, an interactive TTY asks
// y/n, and a non-interactive session refuses unless --yes was given.
func confirmOverwrite(conflicts []string, assumeYes bool) (bool, error) {
	fmt.Println("\nThe following existing files are NOT managed by timesync and would be overwritten:")
	for _, p := range conflicts {
		fmt.Printf("  - %s\n", p)
	}
	fmt.Println("Existing contents are backed up under", apply.DefaultBackupDir, "before overwrite.")

	if assumeYes {
		fmt.Println("Proceeding (--yes).")
		return true, nil
	}
	if !stdinIsInteractive() {
		return false, fmt.Errorf("refusing to overwrite %d unmanaged file(s) in a non-interactive session; re-run with --yes to confirm", len(conflicts))
	}

	fmt.Print("Overwrite these files? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func stdinIsInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive terminal UI with arrow-key menus for doctor, status, and apply",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run()
		},
	}
}

func rollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback",
		Short: "Restore configuration files from the last apply backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := apply.Rollback(); err != nil {
				return err
			}
			fmt.Println("Rollback completed successfully.")
			return nil
		},
	}
}
