package planner

import (
	"fmt"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

const configDir = "/etc/timesync-cli"

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
			Path:        "/etc/systemd/system/chronyd.service.d/timesync-cli.conf",
			Description: "systemd drop-in to use timesync-cli chrony config",
			Content:     renderChronyDropIn(),
		},
	)

	if opts.PTP {
		plan.Changes = append(plan.Changes, ptpClientChanges(iface, "")...)
	}

	plan.Warnings = append(plan.Warnings, "auto mode will NOT enable master/grandmaster serving without explicit apply master")
}

func planChangesMaster(plan *model.Plan, opts model.ApplyOptions) {
	cidr := opts.NTPServeCIDR
	if cidr == "" {
		cidr = "192.168.0.0/24"
		plan.Warnings = append(plan.Warnings, "no --ntp-serve-cidr specified; defaulting to 192.168.0.0/24")
	}

	plan.Changes = append(plan.Changes,
		model.PlannedChange{
			Kind:        "config",
			Path:        configDir + "/chrony.conf",
			Description: "NTP server config (chrony) for local serving",
			Content:     renderChronyMaster(cidr),
		},
		model.PlannedChange{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/chronyd.service.d/timesync-cli.conf",
			Description: "systemd drop-in for chrony master",
			Content:     renderChronyDropIn(),
		},
	)

	if opts.PTP {
		plan.Changes = append(plan.Changes, ptpMasterChanges(opts.Iface)...)
	}
}

func planChangesClient(plan *model.Plan, opts model.ApplyOptions) {
	if opts.PTP {
		plan.Changes = append(plan.Changes, ptpClientChanges(opts.Iface, opts.Source)...)
	} else {
		plan.Changes = append(plan.Changes,
			model.PlannedChange{
				Kind:        "config",
				Path:        configDir + "/chrony.conf",
				Description: "NTP client config pointing to upstream source",
				Content:     renderChronyClientUpstream(opts.Source),
			},
			model.PlannedChange{
				Kind:        "systemd",
				Path:        "/etc/systemd/system/chronyd.service.d/timesync-cli.conf",
				Description: "systemd drop-in for chrony client",
				Content:     renderChronyDropIn(),
			},
		)
	}
}

func ptpClientChanges(iface, source string) []model.PlannedChange {
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
			Path:        "/etc/systemd/system/ptp4l.service.d/timesync-cli.conf",
			Description: "systemd drop-in for ptp4l",
			Content:     renderPTP4LDropIn(),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/phc2sys.service.d/timesync-cli.conf",
			Description: "systemd drop-in for phc2sys",
			Content:     renderPHC2SysDropIn(iface),
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
			Content:     renderPHC2Sys(iface),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/ptp4l.service.d/timesync-cli.conf",
			Description: "systemd drop-in for ptp4l grandmaster",
			Content:     renderPTP4LDropIn(),
		},
		{
			Kind:        "systemd",
			Path:        "/etc/systemd/system/phc2sys.service.d/timesync-cli.conf",
			Description: "systemd drop-in for phc2sys",
			Content:     renderPHC2SysDropIn(iface),
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

func renderChronyMaster(cidr string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
local stratum 8
allow %s
rtcsync
`, cidr)) + "\n"
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
clock_servo           linreg
summary_interval      0
time_stamping         hardware
tx_timestamp_timeout  10
logAnnounceInterval   0
logSyncInterval       -3
logMinDelayReqInterval -3
unicast_listen        1

[unicast_master_table]
address               %s
logAnnounceInterval   0
logSyncInterval       -3
logMinDelayReqInterval -3

[%s]
network_transport     UDPv4
`, source, iface)) + "\n"
	}
	return strings.TrimSpace(fmt.Sprintf(`
[global]
clock_servo           linreg
summary_interval      0
time_stamping         hardware
tx_timestamp_timeout  10
logAnnounceInterval   0
logSyncInterval       -3
logMinDelayReqInterval -3

[%s]
network_transport     UDPv4
`, iface)) + "\n"
}

func renderPTP4LMaster(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
[global]
clockClass            6
clockAccuracy         0x21
priority1             128
priority2             128
domainNumber          0
time_stamping         hardware

[%s]
network_transport     UDPv4
`, iface)) + "\n"
}

func renderPHC2Sys(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
# Generated by timesync-cli
-f /etc/timesync-cli/ptp4l.conf -s %s -w
`, iface)) + "\n"
}

func renderPHC2SysDropIn(iface string) string {
	return strings.TrimSpace(fmt.Sprintf(`
[Service]
ExecStart=
ExecStart=/usr/sbin/phc2sys -f /etc/timesync-cli/ptp4l.conf -s %s -w
`, iface)) + "\n"
}

func renderPTP4LDropIn() string {
	return strings.TrimSpace(`
[Service]
ExecStart=
ExecStart=/usr/sbin/ptp4l -f /etc/timesync-cli/ptp4l.conf
`) + "\n"
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
