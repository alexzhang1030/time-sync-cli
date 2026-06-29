package cmd

import (
	"fmt"
	"os"

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
	var ptp, dryRun bool

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
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface for PTP")
	cmd.Flags().StringVar(&ntpPool, "ntp-pool", "pool.ntp.org", "NTP pool server")
	cmd.Flags().BoolVar(&ptp, "ptp", false, "enable PTP when hardware supports it")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned changes without applying")
	return cmd
}

func applyMasterCmd() *cobra.Command {
	var iface, ntpServeCIDR string
	var ptp, dryRun bool

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
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (required)")
	cmd.Flags().StringVar(&ntpServeCIDR, "ntp-serve-cidr", "", "CIDR to allow NTP serving")
	cmd.Flags().BoolVar(&ptp, "ptp", false, "enable PTP grandmaster mode")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned changes without applying")
	_ = cmd.MarkFlagRequired("iface")
	return cmd
}

func applyClientCmd() *cobra.Command {
	var iface, source string
	var ptp, dryRun bool

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
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (required)")
	cmd.Flags().StringVar(&source, "source", "", "upstream host or IP (required)")
	cmd.Flags().BoolVar(&ptp, "ptp", false, "use PTP slave mode")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned changes without applying")
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
	if err := apply.Apply(plan); err != nil {
		return err
	}
	fmt.Println("\nConfiguration applied successfully.")
	return nil
}

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive terminal UI for role/source/interface selection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run()
		},
	}
}
