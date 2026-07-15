package planner

import (
	"fmt"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

const configDir = "/etc/timesync-cli"
const timesyncBin = "/usr/bin/timesync"

// Plan generates a dry-run plan for the given apply options.
func Plan(opts model.ApplyOptions) (*model.Plan, error) {
	if err := validate(opts); err != nil {
		return nil, err
	}

	plan := &model.Plan{
		Role:  opts.Role,
		Iface: opts.Iface,
		PTP:   opts.PTP,
	}

	switch opts.Role {
	case model.RoleAuto:
		planChangesAuto(plan, opts)
	case model.RoleMaster:
		planChangesMaster(plan, opts)
	case model.RoleClient:
		planChangesClient(plan, opts)
	default:
		return nil, fmt.Errorf("unknown role: %s", opts.Role)
	}

	return plan, nil
}

func validate(opts model.ApplyOptions) error {
	if opts.Iface == "" && opts.Role != model.RoleAuto {
		return fmt.Errorf("--iface is required for role %s", opts.Role)
	}
	if opts.Role == model.RoleClient && opts.Source == "" {
		return fmt.Errorf("--source is required for client role")
	}
	return nil
}

func planChangesAuto(plan *model.Plan, opts model.ApplyOptions) {
	iface := opts.Iface
	if iface == "" {
		iface = "eth0"
		plan.Warnings = append(plan.Warnings, "no --iface specified; defaulting to eth0 in plan")
	}
	pool := opts.NTPPool
	if pool == "" {
		pool = "pool.ntp.org"
	}

	plan.Changes = append(plan.Changes,
		model.PlannedChange{
			Kind:        "config",
			Path:        configDir + "/chrony.conf",
			Description: "NTP client config for internet sync (chrony)",
			Content:     renderChronyClient(pool),
		},
		model.PlannedChange{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/chrony.service.d/timesync-cli.conf",
			Description: "systemd drop-in to use timesync-cli chrony config",
			Content:     renderChronyDropIn(),
		},
	)

	if opts.PTP {
		plan.Changes = append(plan.Changes, ptpMonitorChanges(iface)...)
		disablePTPSystemClockWriters(plan)
		plan.Warnings = append(plan.Warnings, "auto PTP mode keeps chrony as the only system clock discipline source and runs ptp4l for PTP monitoring")
	} else {
		disablePTPUnits(plan)
	}

	plan.Warnings = append(plan.Warnings, "auto mode will NOT enable master/grandmaster serving without explicit apply master")
}

func planChangesMaster(plan *model.Plan, opts model.ApplyOptions) {
	cidr := opts.NTPServeCIDR
	if cidr == "" {
		cidr = "192.168.0.0/24"
		plan.Warnings = append(plan.Warnings, "no --ntp-serve-cidr specified; defaulting to 192.168.0.0/24")
	}
	pool := opts.NTPPool
	if pool == "" {
		pool = "pool.ntp.org"
	}

	plan.Changes = append(plan.Changes,
		model.PlannedChange{
			Kind:        "config",
			Path:        configDir + "/chrony.conf",
			Description: "NTP server config (chrony) for local serving",
			Content:     renderChronyMaster(pool, cidr),
		},
		model.PlannedChange{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/chrony.service.d/timesync-cli.conf",
			Description: "systemd drop-in for chrony master",
			Content:     renderChronyDropIn(),
		},
	)

	if opts.PTP {
		plan.Changes = append(plan.Changes, ptpMasterChanges(opts.Iface)...)
	}
	if !opts.PTP {
		disablePTPUnits(plan)
	}
}

func planChangesClient(plan *model.Plan, opts model.ApplyOptions) {
	if opts.PTP {
		plan.Changes = append(plan.Changes, ptpClientChanges(opts.Iface, opts.Source, bootGuardRepairSystem)...)
		disableChronyUnits(plan)
		plan.Warnings = append(plan.Warnings, "PTP client mode disables chrony so phc2sys is the only system clock discipline source")
	} else {
		disablePTPUnits(plan)
		plan.Changes = append(plan.Changes,
			model.PlannedChange{
				Kind:        "config",
				Path:        configDir + "/chrony.conf",
				Description: "NTP client config pointing to upstream source",
				Content:     renderChronyClientUpstream(opts.Source),
			},
			model.PlannedChange{
				Kind:        "systemd",
				Path:        "/etc/systemd/system/chrony.service.d/timesync-cli.conf",
				Description: "systemd drop-in for chrony client",
				Content:     renderChronyDropIn(),
			},
		)
	}
}

func disablePTPUnits(plan *model.Plan) {
	plan.DisableUnits = appendUnique(plan.DisableUnits, "timesync-ptp-guard.timer", "phc2sys", "ptp4l")
}

func disablePTPSystemClockWriters(plan *model.Plan) {
	plan.DisableUnits = appendUnique(plan.DisableUnits, "timesync-ptp-guard.timer", "phc2sys")
}

func disableChronyUnits(plan *model.Plan) {
	plan.DisableUnits = appendUnique(plan.DisableUnits, "chrony", "chronyd")
}

func appendUnique(list []string, values ...string) []string {
	seen := make(map[string]bool, len(list)+len(values))
	for _, value := range list {
		seen[value] = true
	}
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		list = append(list, value)
	}
	return list
}

type bootGuardMode string

const (
	bootGuardRequireTrustedSystem bootGuardMode = "require-trusted-system"
	bootGuardRepairSystem         bootGuardMode = "repair-system"
)

func ptpClientChanges(iface, source string, mode bootGuardMode) []model.PlannedChange {
	desc := "PTP slave/ordinary clock config (ptp4l)"
	if source != "" {
		desc = "PTP unicast slave config targeting " + source + " (ptp4l)"
	}
	return []model.PlannedChange{
		{
			Kind:        "config",
			Path:        configDir + "/ptp4l.conf",
			Description: desc,
			Content:     renderPTP4LClient(iface, source),
		},
		{
			Kind:        "config",
			Path:        configDir + "/phc2sys.conf",
			Description: "PHC to system clock sync (phc2sys)",
			Content:     renderPHC2Sys(iface),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/ptp4l.service",
			Description: "systemd unit for ptp4l",
			Content:     renderPTP4LService(iface, mode),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/phc2sys.service",
			Description: "systemd unit for phc2sys",
			Content:     renderPHC2SysService(iface),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/timesync-ptp-guard.service",
			Description: "systemd guard that stops phc2sys when PTP or clock health fails",
			Content:     renderPTPGuardService(),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/timesync-ptp-guard.timer",
			Description: "systemd timer for PTP runtime guard",
			Content:     renderPTPGuardTimer(),
		},
	}
}

func ptpMonitorChanges(iface string) []model.PlannedChange {
	return []model.PlannedChange{
		{
			Kind:        "config",
			Path:        configDir + "/ptp4l.conf",
			Description: "PTP ordinary clock monitor config (ptp4l)",
			Content:     renderPTP4LClient(iface, ""),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/ptp4l.service",
			Description: "systemd unit for ptp4l monitor",
			Content:     renderPTP4LService(iface, bootGuardRequireTrustedSystem),
		},
	}
}

func ptpMasterChanges(iface string) []model.PlannedChange {
	return []model.PlannedChange{
		{
			Kind:        "config",
			Path:        configDir + "/ptp4l.conf",
			Description: "PTP grandmaster config (ptp4l)",
			Content:     renderPTP4LMaster(iface),
		},
		{
			Kind:        "config",
			Path:        configDir + "/phc2sys.conf",
			Description: "PHC to system clock sync for grandmaster",
			Content:     renderPHC2SysMaster(iface),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/ptp4l.service",
			Description: "systemd unit for ptp4l grandmaster",
			Content:     renderPTP4LMasterService(iface),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/phc2sys.service",
			Description: "systemd unit for phc2sys",
			Content:     renderPHC2SysMasterService(iface),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/timesync-ptp-guard.service",
			Description: "systemd guard that keeps PTP and RTC state healthy",
			Content:     renderPTPGuardService(),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/timesync-ptp-guard.timer",
			Description: "systemd timer for PTP runtime guard",
			Content:     renderPTPGuardTimer(),
		},
	}
}

func renderChronyClient(pool string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
pool %s iburst
makestep 1.0 3
rtcsync
`, pool)) + "\n"
}

func renderChronyClientUpstream(source string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
server %s iburst
makestep 1.0 3
rtcsync
`, source)) + "\n"
}

func renderChronyMaster(pool, cidr string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
pool %s iburst
makestep 1.0 3
local stratum 8
allow %s
rtcsync
`, pool, cidr)) + "\n"
}

func renderChronyDropIn() string {
	return strings.TrimSpace(`
[Service]
EnvironmentFile=-/etc/timesync-cli/chrony.env
ExecStart=
ExecStart=/usr/sbin/chronyd -f /etc/timesync-cli/chrony.conf
`) + "\n"
}

func renderPTP4LClient(iface, source string) string {
	if source != "" {
		return strings.TrimSpace(fmt.Sprintf(`
[global]
summary_interval      0
time_stamping         hardware
tx_timestamp_timeout  10
logAnnounceInterval   0
logSyncInterval       -3
logMinDelayReqInterval -3
first_step_threshold  1.0
step_threshold        1.0
clientOnly            1
unicast_listen        1

[unicast_master_table]
table_id              1
logQueryInterval      0
UDPv4                 %s

[%s]
network_transport     UDPv4
unicast_master_table  1
`, source, iface)) + "\n"
	}
	return strings.TrimSpace(fmt.Sprintf(`
[global]
summary_interval      0
time_stamping         hardware
tx_timestamp_timeout  10
logAnnounceInterval   0
logSyncInterval       -3
logMinDelayReqInterval -3
first_step_threshold  1.0
step_threshold        1.0
clientOnly            1

[%s]
network_transport     UDPv4
`, iface)) + "\n"
}

func renderPTP4LMaster(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
[global]
clockClass            6
clockAccuracy         0xFE
offsetScaledLogVariance 0xFFFF
utc_offset            37
timeSource            0x50
priority1             128
priority2             128
domainNumber          0
time_stamping         hardware
network_transport     UDPv4
delay_mechanism       E2E
unicast_listen        1

[%s]
network_transport     UDPv4
`, iface)) + "\n"
}

func renderPHC2Sys(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
-f /etc/timesync-cli/ptp4l.conf -s %s -w -S 1.0
`, iface)) + "\n"
}

func renderPHC2SysMaster(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
-s CLOCK_REALTIME -c %s -w -S 1.0
`, iface)) + "\n"
}

func renderPHC2SysService(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=PHC to system clock sync configured by timesync-cli
After=ptp4l.service
Requires=ptp4l.service

[Service]
ExecStartPre=/usr/bin/timesync wait-ptp --timeout 30s
ExecStart=/usr/sbin/phc2sys -f /etc/timesync-cli/ptp4l.conf -s %s -w -S 1.0
Restart=on-failure

[Install]
WantedBy=multi-user.target
`, iface)) + "\n"
}

func renderPTPGuardService() string {
	return strings.TrimSpace(`
# Generated by timesync-cli
[Unit]
Description=Runtime PTP and clock health guard configured by timesync-cli
After=ptp4l.service

[Service]
Type=oneshot
ExecStart=/usr/bin/timesync guard-ptp
`) + "\n"
}

func renderPTPGuardTimer() string {
	return strings.TrimSpace(`
# Generated by timesync-cli
[Unit]
Description=Run timesync PTP health guard periodically

[Timer]
OnBootSec=10s
OnUnitActiveSec=5s
AccuracySec=1s
Unit=timesync-ptp-guard.service

[Install]
WantedBy=timers.target
`) + "\n"
}

func renderPHC2SysMasterService(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=Sync system clock to PTP hardware clock configured by timesync-cli
After=ptp4l.service chrony.service
Requires=ptp4l.service

[Service]
ExecStart=/usr/sbin/phc2sys -s CLOCK_REALTIME -c %s -w -S 1.0
ExecStartPost=%s publish-gm-time-properties --timeout 30s
Restart=on-failure

[Install]
WantedBy=multi-user.target
`, iface, timesyncBin)) + "\n"
}

func renderPTP4LMasterService(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=Precision Time Protocol configured by timesync-cli
After=network-online.target chrony.service
Wants=network-online.target chrony.service
StartLimitIntervalSec=0

[Service]
ExecStartPre=%s boot-guard --iface %s --repair-system-clock
ExecStart=/usr/sbin/ptp4l -f /etc/timesync-cli/ptp4l.conf
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, timesyncBin, iface)) + "\n"
}

func renderPTP4LService(iface string, mode bootGuardMode) string {
	bootGuard := fmt.Sprintf("ExecStartPre=%s boot-guard --iface %s", timesyncBin, iface)
	switch mode {
	case bootGuardRepairSystem:
		bootGuard += " --repair-system-clock"
	case bootGuardRequireTrustedSystem:
		bootGuard += " --require-trusted-system-clock"
	}
	return strings.TrimSpace(fmt.Sprintf(`
[Unit]
Description=Precision Time Protocol configured by timesync-cli
After=network-online.target
Wants=network-online.target

[Service]
%s
ExecStart=/usr/sbin/ptp4l -f /etc/timesync-cli/ptp4l.conf
Restart=on-failure

[Install]
WantedBy=multi-user.target
`, bootGuard)) + "\n"
}

// FormatPlan renders a plan for CLI output.
func FormatPlan(plan *model.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Role: %s\n", plan.Role)
	fmt.Fprintf(&b, "Interface: %s\n", plan.Iface)
	fmt.Fprintf(&b, "PTP: %v\n", plan.PTP)
	if len(plan.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range plan.Warnings {
			fmt.Fprintf(&b, "  - %s\n", w)
		}
	}
	if len(plan.DisableUnits) > 0 {
		b.WriteString("\nSystemd units to disable:\n")
		for _, unit := range plan.DisableUnits {
			fmt.Fprintf(&b, "  - %s\n", unit)
		}
	}
	b.WriteString("\nPlanned changes:\n")
	for i, c := range plan.Changes {
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, c.Kind, c.Description)
		if c.Path != "" {
			fmt.Fprintf(&b, "   path: %s\n", c.Path)
		}
		if c.Content != "" {
			fmt.Fprintf(&b, "   ---\n")
			for _, line := range strings.Split(strings.TrimRight(c.Content, "\n"), "\n") {
				fmt.Fprintf(&b, "   %s\n", line)
			}
			fmt.Fprintf(&b, "   ---\n")
		}
	}
	return b.String()
}
