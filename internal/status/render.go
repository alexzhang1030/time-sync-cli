package status

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const defaultStatusWidth = 76

// RenderOptions controls the terminal status presentation.
type RenderOptions struct {
	Color bool
	Width int
}

type statusRenderer struct {
	width   int
	box     lipgloss.Style
	title   lipgloss.Style
	section lipgloss.Style
	label   lipgloss.Style
	value   lipgloss.Style
	muted   lipgloss.Style
	good    lipgloss.Style
	warn    lipgloss.Style
	bad     lipgloss.Style
}

// FancySummary renders a high-signal terminal dashboard from the report.
func (r *Report) FancySummary(options RenderOptions) string {
	renderer := newStatusRenderer(options)
	return renderer.render(r)
}

func newStatusRenderer(options RenderOptions) statusRenderer {
	width := options.Width
	if width == 0 {
		width = defaultStatusWidth
	}
	if width < 56 {
		width = 56
	}
	if width > 96 {
		width = 96
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width - 4)
	title := lipgloss.NewStyle().Bold(true)
	section := lipgloss.NewStyle().Bold(true)
	label := lipgloss.NewStyle().Width(18)
	value := lipgloss.NewStyle()
	muted := lipgloss.NewStyle()
	good := lipgloss.NewStyle().Bold(true)
	warn := lipgloss.NewStyle().Bold(true)
	bad := lipgloss.NewStyle().Bold(true)

	if options.Color {
		box = box.BorderForeground(lipgloss.Color("#475569"))
		title = title.Foreground(lipgloss.Color("#7DD3FC"))
		section = section.Foreground(lipgloss.Color("#38BDF8"))
		label = label.Foreground(lipgloss.Color("#94A3B8"))
		muted = muted.Foreground(lipgloss.Color("#94A3B8"))
		good = good.Foreground(lipgloss.Color("#4ADE80"))
		warn = warn.Foreground(lipgloss.Color("#FACC15"))
		bad = bad.Foreground(lipgloss.Color("#FB7185"))
	}

	return statusRenderer{
		width:   width,
		box:     box,
		title:   title,
		section: section,
		label:   label,
		value:   value,
		muted:   muted,
		good:    good,
		warn:    warn,
		bad:     bad,
	}
}

func (s statusRenderer) render(r *Report) string {
	blocks := []string{s.renderHeader(r), s.renderHealth(r), s.renderServices(r)}
	if timing := s.renderTiming(r); timing != "" {
		blocks = append(blocks, timing)
	}
	if clocks := s.renderClocks(r); clocks != "" {
		blocks = append(blocks, clocks)
	}
	if warnings := s.renderWarnings(r); warnings != "" {
		blocks = append(blocks, warnings)
	}
	if diagnostics := s.renderDiagnostics(r); diagnostics != "" {
		blocks = append(blocks, diagnostics)
	}
	if next := s.renderNextStep(r); next != "" {
		blocks = append(blocks, next)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func (s statusRenderer) renderHeader(r *Report) string {
	title := s.title.Render("TIMESYNC STATUS")
	badge := s.overallBadge(reportOverallState(r), len(r.Warnings))
	contentWidth := s.width - 6
	gap := contentWidth - lipgloss.Width(title) - lipgloss.Width(badge)
	if gap < 1 {
		gap = 1
	}
	line := title + strings.Repeat(" ", gap) + badge

	management := "unmanaged · observed state"
	if r.ManagementState == "error" {
		management = "management state error"
	} else if r.ConfiguredRole != "" {
		management = "managed · " + r.ConfiguredRole
		if r.ConfiguredPTP {
			management += " · PTP enabled"
		}
	}

	body := []string{
		line,
		s.muted.Render(management),
		s.row("System source", displayValue(r.SystemClockSource)),
		s.row("Clock flow", displayValue(r.ClockFlow)),
	}
	if r.Offset != "" {
		body = append(body, s.row("Current offset", r.Offset))
	}
	return s.box.Render(strings.Join(body, "\n"))
}

func (s statusRenderer) renderHealth(r *Report) string {
	ntp := s.healthRow("NTP sync", r.Health.NTP)
	if r.ConfiguredPTP && strings.EqualFold(r.ConfiguredRole, "client") {
		ntp = s.neutralRow("NTP sync", "disabled · PTP client")
	}

	ptpLink := s.healthRow("PTP link", r.Health.PTPLink)
	ptpAccuracy := s.healthRow("PTP accuracy", r.Health.PTPAccuracy)
	if r.ConfiguredRole != "" && !r.ConfiguredPTP {
		ptpLink = s.neutralRow("PTP link", "disabled")
		ptpAccuracy = s.neutralRow("PTP accuracy", "disabled")
	}
	if r.Health.PTPAccuracy == HealthNotApplicable {
		ptpAccuracy = s.neutralRow("PTP accuracy", "n/a · grandmaster")
	}

	rows := []string{
		ntp,
		ptpLink,
		ptpAccuracy,
		s.healthRow("Clock", r.Health.Clock),
		s.healthRow("Clock discipline", r.Health.Discipline),
		s.healthRow("Runtime guard", r.Health.Guard),
	}
	return s.block("HEALTH", rows)
}

func (s statusRenderer) renderServices(r *Report) string {
	rows := []string{
		s.unitRow("NTP daemon", r.Systemd.NTPDaemon, r.Systemd.Chronyd),
		s.unitRow("ptp4l", r.Systemd.PTP4LUnit, r.Systemd.PTP4L),
		s.unitRow("phc2sys", r.Systemd.PHC2SysUnit, r.Systemd.PHC2Sys),
		s.unitRow("PTP guard", r.Systemd.GuardTimer, "unknown"),
	}
	return s.block("SERVICES", rows)
}

func (s statusRenderer) renderTiming(r *Report) string {
	var rows []string
	if r.Chrony.Source != "" {
		rows = append(rows, s.row("NTP source", r.Chrony.Source))
	}
	if r.Chrony.Offset != "" {
		rows = append(rows, s.row("NTP correction", withUnit(r.Chrony.Offset, "s")))
	}
	if r.Chrony.LeapStatus != "" {
		rows = append(rows, s.row("NTP leap status", r.Chrony.LeapStatus))
	}
	if r.Chrony.Stratum > 0 {
		rows = append(rows, s.row("NTP stratum", strconv.Itoa(r.Chrony.Stratum)))
	}
	if r.PTP.PortState != "" {
		rows = append(rows, s.row("PTP port state", r.PTP.PortState))
	}
	metrics := PTPMetrics{MasterOffset: r.PTP.MasterOffset, OffsetFromMaster: r.PTP.OffsetFromMaster}
	if offset := metrics.PTPOffset(); offset != "" && !strings.EqualFold(r.PTP.PortState, "MASTER") {
		rows = append(rows, s.row("Offset from GM", offset))
	}
	if r.PTP.PathDelay != "" {
		rows = append(rows, s.row("Mean path delay", formatPTPNanoseconds(r.PTP.PathDelay)))
	}
	if r.PTP.StepsRemoved != "" {
		rows = append(rows, s.row("GM hops", r.PTP.StepsRemoved))
	}
	if r.PTP.GMIdentity != "" {
		rows = append(rows, s.row("Grandmaster", r.PTP.GMIdentity))
	}
	if len(rows) == 0 {
		return ""
	}
	return s.block("TIMING", rows)
}

func (s statusRenderer) renderClocks(r *Report) string {
	var rows []string
	if r.Clock.SystemUnix > 0 {
		rows = append(rows, s.row("System (UTC)", formatUnixTime(r.Clock.SystemUnix)))
	}
	if r.Clock.RTCUnix > 0 {
		rows = append(rows, s.row("RTC (UTC)", formatUnixTime(r.Clock.RTCUnix)))
	}
	if r.Clock.PHCUnix > 0 {
		iface := ""
		if r.Clock.Iface != "" {
			iface = " (" + r.Clock.Iface + ")"
		}
		switch {
		case r.Clock.PHCTimeScale == "TAI":
			rows = append(rows, s.row("PHC raw"+iface, formatUnixTimeScale(r.Clock.PHCUnix, "TAI")))
			if r.Clock.PHCUTCUnix > 0 {
				rows = append(rows, s.row("PHC as UTC", formatUnixTime(r.Clock.PHCUTCUnix)))
			}
		case r.Clock.PHCTimeScale == "UTC":
			rows = append(rows, s.row("PHC (UTC)"+iface, formatUnixTime(r.Clock.PHCUnix)))
		default:
			rows = append(rows, s.row("PHC raw"+iface, formatUnixTimeScale(r.Clock.PHCUnix, "unknown scale")))
		}
	}
	if r.Clock.RTCSystemSkew != "" {
		rows = append(rows, s.row("RTC residual", r.Clock.RTCSystemSkew+" · System − RTC"))
	}
	if r.Clock.PHCTimeScale != "" {
		rows = append(rows, s.row("PHC time scale", r.Clock.PHCTimeScale))
	}
	// Single coherent decision for TAI–UTC offset validity to avoid any
	// possibility of duplicate or conflicting "TAI–UTC offset valid" rows.
	offsetValid := ""
	offsetDesc := ""
	if r.Clock.TAIUTCOffsetValid {
		offsetValid = "yes"
		offsetDesc = fmt.Sprintf("%+d s · from ptp4l", r.Clock.TAIUTCOffset)
	} else if r.Clock.PHCTimeScale == "TAI" || r.Clock.PHCTimeScale == "UTC" {
		if strings.EqualFold(r.ConfiguredRole, "master") {
			offsetValid = "no"
		} else {
			offsetValid = "no (from GM)"
		}
	} else if r.Clock.PHCTimeScale != "" && r.Clock.PHCTimeScale != "unknown" {
		offsetValid = "no"
	}
	if offsetValid != "" {
		rows = append(rows, s.row("TAI–UTC offset valid", offsetValid))
		if offsetDesc != "" {
			rows = append(rows, s.row("TAI–UTC offset", offsetDesc))
		}
	}
	if r.Clock.PHCResidual != "" {
		rows = append(rows, s.row("PHC residual", r.Clock.PHCResidual+" · System − PHC(UTC)"))
	} else if r.Clock.PHCUnix > 0 {
		rows = append(rows, s.row("PHC residual", "unknown · time scale unavailable"))
	}
	if len(rows) == 0 {
		return ""
	}
	return s.block("CLOCKS · RTC SNAPSHOT 1 s", rows)
}

func (s statusRenderer) renderWarnings(r *Report) string {
	if len(r.Warnings) == 0 {
		return ""
	}
	rows := make([]string, 0, len(r.Warnings))
	for _, warning := range r.Warnings {
		rows = append(rows, "  "+s.warn.Render("▲ "+warning))
	}
	return s.block("WARNINGS", rows)
}

func (s statusRenderer) renderDiagnostics(r *Report) string {
	var rows []string
	if r.Chrony.Detail != "" && (!r.ConfiguredPTP || !strings.EqualFold(r.ConfiguredRole, "client")) {
		rows = append(rows, s.row("NTP", r.Chrony.Detail))
	}
	if r.PTP.Detail != "" && (r.ConfiguredPTP || r.ConfiguredRole == "") {
		rows = append(rows, s.row("PTP", r.PTP.Detail))
	}
	if r.PTP.TimePropertiesDetail != "" {
		rows = append(rows, s.row("PTP time scale", r.PTP.TimePropertiesDetail))
	}
	if r.Clock.Detail != "" {
		rows = append(rows, s.row("Clock", r.Clock.Detail))
	}
	if r.ManagementDetail != "" {
		rows = append(rows, s.row("Management", r.ManagementDetail))
	}
	for _, unit := range []UnitStatus{r.Systemd.NTPDaemon, r.Systemd.PTP4LUnit, r.Systemd.PHC2SysUnit, r.Systemd.GuardTimer} {
		if unit.Detail != "" {
			rows = append(rows, s.row(unitLabel(unit), unit.Detail))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return s.block("DIAGNOSTICS", rows)
}

func (s statusRenderer) renderNextStep(r *Report) string {
	if reportOverallState(r) != HealthUnmanaged {
		return ""
	}
	return s.block("NEXT STEP", []string{
		s.row("Inspect host", "timesync doctor"),
		s.row("Configure", "sudo timesync tui"),
	})
}

func (s statusRenderer) block(title string, rows []string) string {
	return s.section.Render(title) + "\n" + strings.Join(rows, "\n")
}

func (s statusRenderer) row(label, value string) string {
	return "  " + s.label.Render(label) + s.value.Render(value)
}

func (s statusRenderer) healthRow(label string, state HealthState) string {
	switch state {
	case HealthHealthy:
		return "  " + s.label.Render(label) + s.good.Render("● HEALTHY")
	case HealthDegraded:
		return "  " + s.label.Render(label) + s.warn.Render("▲ DEGRADED")
	case HealthUnhealthy:
		return "  " + s.label.Render(label) + s.bad.Render("● UNHEALTHY")
	case HealthUnknown, "":
		return "  " + s.label.Render(label) + s.warn.Render("◆ UNKNOWN")
	case HealthInactive:
		return s.neutralRow(label, "inactive")
	case HealthDisabled:
		return s.neutralRow(label, "disabled")
	case HealthNotApplicable:
		return s.neutralRow(label, "n/a")
	case HealthUnmanaged:
		return s.neutralRow(label, "unmanaged")
	default:
		return s.neutralRow(label, string(state))
	}
}

func (s statusRenderer) unitRow(label string, unit UnitStatus, fallback string) string {
	state := valueOr(unit.ActiveState, fallback)
	name := unitLabel(unit)
	value := name + " · " + state
	if unit.UnitFileState != "" && unit.UnitFileState != "unknown" && unit.UnitFileState != "not-found" {
		value += " · " + unit.UnitFileState
	}
	switch state {
	case "active":
		return "  " + s.label.Render(label) + s.good.Render("● "+value)
	case "query-error":
		return "  " + s.label.Render(label) + s.warn.Render("◆ "+value)
	default:
		return "  " + s.label.Render(label) + s.muted.Render("○ "+value)
	}
}

func (s statusRenderer) neutralRow(label, state string) string {
	return "  " + s.label.Render(label) + s.muted.Render("○ "+state)
}

func (s statusRenderer) overallBadge(state HealthState, warnings int) string {
	switch state {
	case HealthHealthy:
		if warnings > 0 {
			return s.good.Render(fmt.Sprintf("● HEALTHY · %d WARN", warnings))
		}
		return s.good.Render("● HEALTHY")
	case HealthDegraded:
		return s.warn.Render("▲ DEGRADED")
	case HealthUnknown:
		return s.warn.Render("◆ UNKNOWN")
	case HealthUnmanaged:
		return s.muted.Render("◇ UNMANAGED")
	default:
		return s.bad.Render("● NEEDS ATTENTION")
	}
}

func reportOverallState(r *Report) HealthState {
	if r.Health.Overall != "" {
		return r.Health.Overall
	}
	if r.ManagementState == "error" {
		return HealthUnknown
	}
	if r.ConfiguredRole == "" {
		return HealthUnmanaged
	}
	if r.Healthy {
		return HealthHealthy
	}
	return HealthUnhealthy
}

func unitLabel(unit UnitStatus) string {
	name := strings.TrimSpace(unit.Unit)
	if name == "" {
		return "unknown unit"
	}
	return name
}

func displayValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

func formatUnixTime(value int64) string {
	return fmt.Sprintf("%s  (%d)", time.Unix(value, 0).UTC().Format("2006-01-02 15:04:05 UTC"), value)
}

func formatUnixTimeScale(value int64, scale string) string {
	formatted := time.Unix(value, 0).UTC().Format("2006-01-02 15:04:05")
	return fmt.Sprintf("%s %s  (%d)", formatted, scale, value)
}
