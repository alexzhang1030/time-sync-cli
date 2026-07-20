package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/gm"
	"github.com/alexzhang1030/time-sync-cli/internal/guard"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
	"github.com/alexzhang1030/time-sync-cli/internal/repair"
	"github.com/alexzhang1030/time-sync-cli/internal/status"
	"github.com/alexzhang1030/time-sync-cli/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "timesync",
	Short: "Linux time sync CLI for NTP/PTP management",
	Long:  "Hide NTP/PTP complexity behind simple configuration flows for robot, industrial PC, and embedded Linux deployments.",
}

// SetVersion exposes the release version through Cobra's --version flag.
func SetVersion(version string) {
	rootCmd.Version = version
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
	rootCmd.AddCommand(bootGuardCmd())
	rootCmd.AddCommand(publishGMTimePropertiesCmd())
	rootCmd.AddCommand(guardPTPCmd())
	rootCmd.AddCommand(waitPTPCmd())
	rootCmd.AddCommand(repairClockCmd())
	rootCmd.AddCommand(rollbackCmd())
	rootCmd.AddCommand(uninstallCmd())
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
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report role-aware clock health, source flow, NTP/PTP accuracy, and runtime guards",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := status.Collect()
			if err != nil {
				return err
			}
			writer := cmd.OutOrStdout()
			output, err := renderStatusOutput(
				report,
				outputFormat,
				writerSupportsFancy(writer),
				writerSupportsColor(writer),
				statusOutputWidth(),
			)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(writer, output)
			return err
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "auto", "Output format: auto, fancy, plain, or json")
	return cmd
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
	var iface, ntpServeCIDR, ntpPool string
	var ptp, dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "master",
		Short: "Serve local time to downstream devices (NTP and optional PTP grandmaster)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(model.ApplyOptions{
				Role:         model.RoleMaster,
				Iface:        iface,
				NTPPool:      ntpPool,
				NTPServeCIDR: ntpServeCIDR,
				PTP:          ptp,
				DryRun:       dryRun,
				Yes:          yes,
			})
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "network interface (required)")
	cmd.Flags().StringVar(&ntpPool, "ntp-pool", "pool.ntp.org", "upstream NTP pool server")
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

func repairClockCmd() *cobra.Command {
	var iface, rtcPath string
	cmd := &cobra.Command{
		Use:          "repair-clock",
		Short:        "Recover system time and PHC from RTC after an epoch clock reset",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := repair.Clock(repair.Options{
				Iface:   iface,
				RTCPath: rtcPath,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Clock repair completed using RTC epoch %d and interface %s.\n", result.RTCEpoch, result.Iface)
			for _, step := range result.Steps {
				fmt.Printf("  - %s\n", step.Command)
			}
			report, err := status.Collect()
			if err != nil {
				return err
			}
			fmt.Println()
			fmt.Print(report.Summary())
			return nil
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "PTP interface; defaults to the last applied timesync interface")
	cmd.Flags().StringVar(&rtcPath, "rtc-path", repair.DefaultRTCPath, "RTC since_epoch path")
	return cmd
}

func bootGuardCmd() *cobra.Command {
	var iface, rtcPath string
	var repairSystemClock, requireTrustedSystemClock bool
	cmd := &cobra.Command{
		Use:          "boot-guard",
		Short:        "Prime system time and PHC before ptp4l starts",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := repair.BootGuard(repair.Options{
				Iface:                     iface,
				RTCPath:                   rtcPath,
				RepairSystemClock:         repairSystemClock,
				RequireTrustedSystemClock: requireTrustedSystemClock,
			})
			return err
		},
	}
	cmd.Flags().StringVar(&iface, "iface", "", "PTP interface; defaults to the last applied timesync interface")
	cmd.Flags().StringVar(&rtcPath, "rtc-path", repair.DefaultRTCPath, "RTC since_epoch path")
	cmd.Flags().BoolVar(&repairSystemClock, "repair-system-clock", false, "repair system clock from RTC when system time is untrusted")
	cmd.Flags().BoolVar(&requireTrustedSystemClock, "require-trusted-system-clock", false, "fail when system time is not trusted against RTC")
	return cmd
}

func waitPTPCmd() *cobra.Command {
	var timeout, interval time.Duration
	cmd := &cobra.Command{
		Use:          "wait-ptp",
		Short:        "Wait until PTP is healthy before phc2sys starts",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if interval <= 0 {
				interval = time.Second
			}
			deadline := time.Now().Add(timeout)
			var last *status.Report
			var lastErr error
			for {
				report, err := status.Collect()
				if err == nil {
					last = report
					if ptpReadyForPHC2Sys(report) {
						return nil
					}
				} else {
					lastErr = err
				}
				if timeout <= 0 || time.Now().After(deadline) {
					break
				}
				time.Sleep(interval)
			}
			if last != nil {
				return fmt.Errorf("PTP not healthy before phc2sys start: ptp=%s clock=%s configured_role=%s port_state=%s master_offset=%s",
					last.PTPHealth,
					last.ClockHealth,
					last.ConfiguredRole,
					last.PTP.PortState,
					last.PTP.MasterOffset,
				)
			}
			if lastErr != nil {
				return fmt.Errorf("PTP health check failed before phc2sys start: %w", lastErr)
			}
			return fmt.Errorf("PTP health check did not run before phc2sys start")
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum time to wait for healthy PTP before phc2sys starts")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "PTP health polling interval")
	return cmd
}

func publishGMTimePropertiesCmd() *cobra.Command {
	var configPath string
	var timeout, interval time.Duration
	cmd := &cobra.Command{
		Use:          "publish-gm-time-properties",
		Short:        "Publish a valid UTC offset from a PTP grandmaster",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := apply.LoadState("")
			if err != nil {
				return fmt.Errorf("load applied role before publishing grandmaster properties: %w", err)
			}
			if state.Role != model.RoleMaster || !state.PTP {
				return fmt.Errorf("grandmaster time properties require an applied master --ptp role")
			}

			// Ensure phc2sys (the master PHC writer) is running so the PHC can
			// converge to TAI before we set currentUtcOffsetValid.
			//
			// Critical: this command is also invoked from phc2sys ExecStartPost.
			// Calling a blocking `systemctl start phc2sys` while the unit is
			// already "activating" deadlocks against the start job and times out.
			// Only start when the unit is fully stopped; if it is already active
			// or mid-start, just wait for PHC alignment.
			if err := ensurePHC2SysRunningForPublish(); err != nil {
				return err
			}
			// Wait for phc2sys to bring PHC into the required state.
			// Respect the --config flag for the utc_offset and the --timeout
			// flag for the maximum wait. Fail early if we cannot achieve
			// alignment so the caller gets a clear error instead of a
			// confusing residual message from Publish.
			if waitErr := gm.WaitForPHCAlignment(nil, configPath, state.Iface, timeout); waitErr != nil {
				return fmt.Errorf("PHC alignment failed (using config %s): %w", configPath, waitErr)
			}

			result, err := gm.Publish(gm.Options{
				ConfigPath: configPath,
				Iface:      state.Iface,
				Timeout:    timeout,
				Interval:   interval,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Published PTP grandmaster UTC offset %+d s.\n", result.UTCOffset)
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", gm.DefaultConfigPath, "generated ptp4l configuration")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum time to wait for PHC alignment and the ptp4l management socket")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "retry interval")
	return cmd
}

// ensurePHC2SysRunningForPublish starts phc2sys only when it is fully stopped.
// When the unit is already active or activating (including ExecStartPost of
// phc2sys itself), a blocking systemctl start would deadlock the start job.
func ensurePHC2SysRunningForPublish() error {
	out, err := exec.Command("systemctl", "is-active", "phc2sys").CombinedOutput()
	state := strings.TrimSpace(string(out))
	switch state {
	case "active", "activating", "reloading":
		return nil
	}
	// is-active returns exit status 3 for inactive/failed; still try to start.
	if err := exec.Command("systemctl", "start", "phc2sys").Run(); err != nil {
		return fmt.Errorf("failed to start phc2sys for GM alignment (was %q): %w", valueOr(state, "unknown"), err)
	}
	_ = err // is-active non-zero is expected when inactive
	return nil
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func ptpReadyForPHC2Sys(report *status.Report) bool {
	return report != nil && report.PTPHealth == "true"
}

func guardPTPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "guard-ptp",
		Short:        "Keep phc2sys aligned with current PTP and clock health",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := guard.PTPOnce(guard.Options{})
			if result != nil && result.Action != "none" {
				fmt.Printf("PTP runtime guard action: %s (%s)\n", result.Action, result.Reason)
			}
			return err
		},
	}
	return cmd
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

func uninstallCmd() *cobra.Command {
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove timesync-managed NTP/PTP role configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := apply.PlanUninstall()
			if err != nil {
				return err
			}
			fmt.Print(apply.FormatUninstallPlan(plan))
			if dryRun {
				fmt.Println("\n(dry-run: no changes applied)")
				return nil
			}
			if !yes {
				proceed, err := confirmUninstall()
				if err != nil {
					return err
				}
				if !proceed {
					fmt.Println("\nAborted; no changes applied.")
					return nil
				}
			}
			if err := apply.Uninstall(); err != nil {
				return err
			}
			fmt.Println("\nTimesync-managed NTP/PTP configuration removed successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "render planned uninstall actions without applying")
	cmd.Flags().BoolVar(&yes, "yes", false, "assume yes; remove timesync-managed NTP/PTP config without prompting")
	return cmd
}

func confirmUninstall() (bool, error) {
	if !stdinIsInteractive() {
		return false, fmt.Errorf("refusing to remove timesync-managed config in a non-interactive session; re-run with --yes to confirm")
	}
	fmt.Print("Remove timesync-managed NTP/PTP configuration? [y/N]: ")
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
