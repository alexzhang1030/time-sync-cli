package status

import (
	"fmt"
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
	label := lipgloss.NewStyle().Width(14)
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
	if diagnostics := s.renderDiagnostics(r); diagnostics != "" {
		blocks = append(blocks, diagnostics)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

func (s statusRenderer) renderHeader(r *Report) string {
	title := s.title.Render("TIMESYNC STATUS")
	badge := s.overallBadge(r.Healthy)
	contentWidth := s.width - 6
	gap := contentWidth - lipgloss.Width(title) - lipgloss.Width(badge)
	if gap < 1 {
		gap = 1
	}
	line := title + strings.Repeat(" ", gap) + badge

	configured := "unconfigured"
	if r.ConfiguredRole != "" {
		configured = r.ConfiguredRole
		if r.ConfiguredPTP {
			configured += " · PTP enabled"
		}
	}

	body := []string{
		line,
		s.muted.Render(configured),
		s.row("Active role", displayValue(r.Role)),
		s.row("Source", displayValue(r.Source)),
	}
	if r.Offset != "" {
		body = append(body, s.row("Offset", r.Offset))
	}
	return s.box.Render(strings.Join(body, "\n"))
}

func (s statusRenderer) renderHealth(r *Report) string {
	ntp := s.healthRow("NTP", boolHealth(r.NTPHealth))
	if r.ConfiguredPTP && !r.NTPHealth && (strings.EqualFold(r.ConfiguredRole, "client") || strings.EqualFold(r.ConfiguredRole, "master")) {
		ntp = s.neutralRow("NTP", "inactive")
	}
	ptp := s.healthRow("PTP", r.PTPHealth)
	if r.ConfiguredRole != "" && !r.ConfiguredPTP && strings.EqualFold(r.PTPHealth, "false") {
		ptp = s.neutralRow("PTP", "disabled")
	}
	rows := []string{
		ntp,
		ptp,
		s.healthRow("Clock", r.ClockHealth),
	}
	return s.block("HEALTH", rows)
}

func (s statusRenderer) renderServices(r *Report) string {
	rows := []string{
		s.serviceRow("chronyd", r.Systemd.Chronyd),
		s.serviceRow("ptp4l", r.Systemd.PTP4L),
		s.serviceRow("phc2sys", r.Systemd.PHC2Sys),
	}
	return s.block("SERVICES", rows)
}

func (s statusRenderer) renderTiming(r *Report) string {
	var rows []string
	if r.Chrony.Source != "" {
		rows = append(rows, s.row("NTP source", r.Chrony.Source))
	}
	if r.Chrony.Offset != "" {
		rows = append(rows, s.row("NTP offset", withUnit(r.Chrony.Offset, "s")))
	}
	if r.PTP.PortState != "" {
		rows = append(rows, s.row("PTP port", r.PTP.PortState))
	}
	metrics := PTPMetrics{
		MasterOffset:     r.PTP.MasterOffset,
		OffsetFromMaster: r.PTP.OffsetFromMaster,
	}
	if offset := metrics.PTPOffset(); offset != "" {
		rows = append(rows, s.row("PTP offset", offset))
	}
	if r.PTP.PathDelay != "" {
		rows = append(rows, s.row("Path delay", formatPTPNanoseconds(r.PTP.PathDelay)))
	}
	if r.PTP.StepsRemoved != "" {
		rows = append(rows, s.row("Steps removed", r.PTP.StepsRemoved))
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
		rows = append(rows, s.row("System", formatUnixTime(r.Clock.SystemUnix)))
	}
	if r.Clock.RTCUnix > 0 {
		rows = append(rows, s.row("RTC", formatUnixTime(r.Clock.RTCUnix)))
	}
	if r.Clock.PHCUnix > 0 {
		label := "PHC"
		if r.Clock.Iface != "" {
			label += " (" + r.Clock.Iface + ")"
		}
		rows = append(rows, s.row(label, formatUnixTime(r.Clock.PHCUnix)))
	}
	if r.Clock.RTCSystemSkew != "" {
		rows = append(rows, s.row("RTC skew", r.Clock.RTCSystemSkew))
	}
	if r.Clock.PHCSystemSkew != "" {
		rows = append(rows, s.row("PHC skew", r.Clock.PHCSystemSkew))
	}
	if len(rows) == 0 {
		return ""
	}
	return s.block("CLOCKS", rows)
}

func (s statusRenderer) renderDiagnostics(r *Report) string {
	var rows []string
	if r.PTP.Detail != "" {
		rows = append(rows, s.row("PTP", r.PTP.Detail))
	}
	if r.Clock.Detail != "" {
		rows = append(rows, s.row("Clock", r.Clock.Detail))
	}
	if r.Chrony.Tracking != "" {
		rows = append(rows, s.row("Chrony", r.Chrony.Tracking))
	}
	if len(rows) == 0 {
		return ""
	}
	return s.block("DIAGNOSTICS", rows)
}

func (s statusRenderer) block(title string, rows []string) string {
	return s.section.Render(title) + "\n" + strings.Join(rows, "\n")
}

func (s statusRenderer) row(label, value string) string {
	return "  " + s.label.Render(label) + s.value.Render(value)
}

func (s statusRenderer) healthRow(label, health string) string {
	normalized := strings.ToLower(strings.TrimSpace(health))
	switch normalized {
	case "true", "healthy":
		return "  " + s.label.Render(label) + s.good.Render("● HEALTHY")
	case "unknown", "":
		return "  " + s.label.Render(label) + s.warn.Render("◆ UNKNOWN")
	default:
		return "  " + s.label.Render(label) + s.bad.Render("● UNHEALTHY")
	}
}

func (s statusRenderer) serviceRow(label, state string) string {
	if strings.EqualFold(strings.TrimSpace(state), "active") {
		return "  " + s.label.Render(label) + s.good.Render("● active")
	}
	return "  " + s.label.Render(label) + s.muted.Render("○ "+displayValue(state))
}

func (s statusRenderer) neutralRow(label, state string) string {
	return "  " + s.label.Render(label) + s.muted.Render("○ "+state)
}

func (s statusRenderer) overallBadge(healthy bool) string {
	if healthy {
		return s.good.Render("● HEALTHY")
	}
	return s.bad.Render("● NEEDS ATTENTION")
}

func boolHealth(healthy bool) string {
	if healthy {
		return "true"
	}
	return "false"
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
