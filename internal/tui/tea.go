package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexzhang1030/time-sync-cli/internal/apply"
	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/planner"
	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

type screen int

const (
	screenMain screen = iota
	screenDoctor
	screenStatus
	screenApply
	screenDone
)

type applyStep int

const (
	applyStepRole applyStep = iota
	applyStepIface
	applyStepIfaceText
	applyStepNTPPool
	applyStepPTPAuto
	applyStepCIDR
	applyStepPTPMaster
	applyStepSource
	applyStepPTPClient
	applyStepPlan
	applyStepAction
	applyStepConfirm
)

type doctorResultMsg struct {
	text string
	err  error
}

type statusResultMsg struct {
	report *status.Report
	err    error
}

type detectResultMsg struct {
	report *detect.Report
	err    error
}

type appModel struct {
	screen screen
	width  int
	height int

	menuCursor int
	menuItems  []string

	viewport viewport.Model
	content  string
	loading  bool
	status   *status.Report

	applyStep     applyStep
	applyCursor   int
	applyItems    []string
	role          model.Role
	opts          model.ApplyOptions
	report        *detect.Report
	plan          *model.Plan
	textInput     textinput.Model
	textLabel     string
	yesNoQuestion string
	actionItems   []string
	conflicts     []string

	finalMessage string
	finalErr     error
	quitting     bool
}

func newAppModel() appModel {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 40

	vp := viewport.New(0, 0)

	return appModel{
		screen: screenMain,
		menuItems: []string{
			"Doctor — detect OS, tools, interfaces, PTP",
			"Status — sync health report",
			"Apply — configure and apply role",
			"Quit",
		},
		actionItems: []string{
			"Dry-run (preview only)",
			"Apply (write configs)",
			"Cancel",
		},
		textInput: ti,
		viewport:  vp,
	}
}

func runTea() error {
	m := newAppModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	app, ok := finalModel.(appModel)
	if !ok {
		return nil
	}
	return app.finalErr
}

func (m appModel) Init() tea.Cmd {
	return nil
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 2
		m.viewport.Height = msg.Height - 6
		if m.status != nil {
			m.content = m.status.FancySummary(status.RenderOptions{Color: true, Width: m.viewport.Width})
		}
		if m.content != "" {
			m.viewport.SetContent(m.content)
		}
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}
		switch m.screen {
		case screenMain:
			return m.updateMain(msg)
		case screenDoctor, screenStatus, screenDone:
			return m.handleBack(msg)
		case screenApply:
			return m.updateApply(msg)
		}

	case doctorResultMsg:
		m.loading = false
		m.content = msg.text
		if msg.err != nil {
			m.finalErr = msg.err
		}
		m.viewport.SetContent(m.content)
		return m, nil

	case statusResultMsg:
		m.loading = false
		if msg.err != nil {
			m.finalErr = msg.err
			m.content = fmt.Sprintf("Status failed: %v\n", msg.err)
		} else {
			m.status = msg.report
			m.content = msg.report.FancySummary(status.RenderOptions{Color: true, Width: m.viewport.Width})
		}
		m.viewport.SetContent(m.content)
		return m, nil

	case detectResultMsg:
		if msg.err != nil {
			m.finalErr = msg.err
			m.screen = screenDone
			m.content = fmt.Sprintf("Detection failed: %v\n", msg.err)
			m.viewport.SetContent(m.content)
			return m, nil
		}
		m.report = msg.report
		m.applyStep = applyStepIface
		m.applyItems = interfaceChoices(msg.report.Interfaces)
		m.applyCursor = 0
		return m, nil
	}

	return m, nil
}

func (m appModel) View() string {
	if m.loading {
		return titleStyle.Render("timesync") + "\n\n" + helpStyle.Render("Loading...")
	}

	switch m.screen {
	case screenMain:
		return renderMenu("timesync — interactive setup", m.menuItems, m.menuCursor, "↑/↓ navigate · enter select · q quit")
	case screenDoctor:
		return renderViewport("Doctor", m.viewport, "esc back · q quit")
	case screenStatus:
		return renderViewport("Status", m.viewport, "esc back · q quit")
	case screenApply:
		return m.viewApply()
	case screenDone:
		return renderViewport("Done", m.viewport, "esc back · q quit")
	}
	return ""
}

func (m appModel) updateMain(msg tea.KeyMsg) (appModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(m.menuItems)-1 {
			m.menuCursor++
		}
	case "enter":
		switch m.menuCursor {
		case 0:
			m.screen = screenDoctor
			m.loading = true
			m.content = ""
			return m, runDoctorCmd
		case 1:
			m.screen = screenStatus
			m.loading = true
			m.content = ""
			m.status = nil
			return m, runStatusCmd
		case 2:
			m.screen = screenApply
			m.applyStep = applyStepRole
			m.applyItems = []string{
				"auto — internet time sync",
				"master — serve local time",
				"client — follow upstream source",
			}
			m.applyCursor = 0
			m.opts = model.ApplyOptions{}
			m.plan = nil
			m.report = nil
			return m, nil
		case 3:
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m appModel) handleBack(msg tea.KeyMsg) (appModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc", "enter":
		if m.screen == screenDone {
			m.screen = screenMain
			m.content = ""
			m.finalErr = nil
			return m, nil
		}
		m.screen = screenMain
		m.content = ""
		m.status = nil
		return m, nil
	}
	return m, nil
}

func (m appModel) updateApply(msg tea.KeyMsg) (appModel, tea.Cmd) {
	switch m.applyStep {
	case applyStepRole, applyStepIface, applyStepPTPAuto, applyStepPTPMaster, applyStepPTPClient, applyStepAction, applyStepConfirm:
		return m.updateApplyMenu(msg)
	case applyStepIfaceText, applyStepNTPPool, applyStepCIDR, applyStepSource:
		return m.updateApplyText(msg)
	case applyStepPlan:
		if msg.String() == "enter" {
			m.applyStep = applyStepAction
			m.applyCursor = 0
			return m, nil
		}
		return m.handleBack(msg)
	}
	return m, nil
}

func (m appModel) updateApplyMenu(msg tea.KeyMsg) (appModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		if m.applyStep == applyStepRole {
			m.screen = screenMain
			return m, nil
		}
		m.applyBack()
		return m, nil
	case "up", "k":
		if m.applyCursor > 0 {
			m.applyCursor--
		}
	case "down", "j":
		if m.applyCursor < len(m.applyItems)-1 {
			m.applyCursor++
		}
	case "enter":
		return m.applySelect()
	}
	return m, nil
}

func (m appModel) updateApplyText(msg tea.KeyMsg) (appModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.applyBack()
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.textInput.Value())
		if value == "" {
			value = m.textInput.Placeholder
		}
		if (m.applyStep == applyStepSource || m.applyStep == applyStepIfaceText) && value == "" {
			return m, nil
		}
		switch m.applyStep {
		case applyStepIfaceText:
			m.opts.Iface = value
			m.textInput.Blur()
			return m.advanceAfterIface()
		case applyStepNTPPool:
			m.opts.NTPPool = value
			m.applyStep = applyStepPTPAuto
			m.applyItems = yesNoChoices()
			m.yesNoQuestion = "Enable PTP when hardware supports it?"
		case applyStepCIDR:
			m.opts.NTPServeCIDR = value
			m.applyStep = applyStepPTPMaster
			m.applyItems = yesNoChoices()
			m.yesNoQuestion = "Enable PTP grandmaster?"
		case applyStepSource:
			m.opts.Source = value
			m.applyStep = applyStepPTPClient
			m.applyItems = yesNoChoices()
			m.yesNoQuestion = "Use PTP slave mode?"
		}
		m.applyCursor = 0
		m.textInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m appModel) applySelect() (appModel, tea.Cmd) {
	switch m.applyStep {
	case applyStepRole:
		roles := []model.Role{model.RoleAuto, model.RoleMaster, model.RoleClient}
		m.role = roles[m.applyCursor]
		m.opts = defaultApplyOpts(m.role)
		return m, runDetectCmd
	case applyStepIface:
		required := ifaceRequired(m.role)
		if len(m.report.Interfaces) == 0 {
			switch m.applyCursor {
			case 0:
				if !required {
					m.opts.Iface = "eth0"
					break
				}
				fallthrough
			case 1:
				m.applyStep = applyStepIfaceText
				m.textLabel = "Network interface"
				m.textInput.SetValue("")
				m.textInput.Placeholder = "eth0"
				m.textInput.Focus()
				return m, textinput.Blink
			}
		} else {
			if !required && m.applyCursor == len(m.report.Interfaces) {
				m.opts.Iface = "eth0"
			} else {
				m.opts.Iface = m.report.Interfaces[m.applyCursor].Name
			}
		}
		return m.advanceAfterIface()
	case applyStepPTPAuto, applyStepPTPMaster, applyStepPTPClient:
		m.opts.PTP = m.applyCursor == 0
		return m.showPlan()
	case applyStepAction:
		switch m.applyCursor {
		case 0:
			m.content = "(dry-run: no changes applied)\n\n" + planner.FormatPlan(m.plan)
			m.viewport.SetContent(m.content)
			m.screen = screenDone
			return m, nil
		case 1:
			conflicts, err := apply.UnmanagedConflicts(m.plan)
			if err != nil {
				m.finalErr = err
				m.content = fmt.Sprintf("Conflict check failed: %v\n", err)
				m.viewport.SetContent(m.content)
				m.screen = screenDone
				return m, nil
			}
			if len(conflicts) > 0 && !m.opts.Yes {
				m.conflicts = conflicts
				m.applyStep = applyStepConfirm
				m.applyItems = []string{"No, cancel", "Yes, overwrite"}
				m.applyCursor = 0
				return m, nil
			}
			return m.doApply()
		case 2:
			m.content = "Cancelled."
			m.viewport.SetContent(m.content)
			m.screen = screenDone
			return m, nil
		}
	case applyStepConfirm:
		if m.applyCursor == 1 {
			m.opts.Yes = true
			return m.doApply()
		}
		m.content = "Cancelled; no changes applied."
		m.viewport.SetContent(m.content)
		m.screen = screenDone
		return m, nil
	}
	return m, nil
}

func (m appModel) doApply() (appModel, tea.Cmd) {
	msg, err := executeApplyAction(m.opts, m.plan, "apply")
	if err != nil {
		m.finalErr = err
		m.content = fmt.Sprintf("Apply failed: %v\n", err)
	} else {
		m.finalMessage = msg
		m.content = msg + "\n\n" + planner.FormatPlan(m.plan)
	}
	m.viewport.SetContent(m.content)
	m.screen = screenDone
	return m, nil
}

func (m appModel) advanceAfterIface() (appModel, tea.Cmd) {
	switch m.role {
	case model.RoleAuto:
		m.applyStep = applyStepNTPPool
		m.textLabel = "NTP pool"
		m.textInput.SetValue(m.opts.NTPPool)
		m.textInput.Placeholder = m.opts.NTPPool
		m.textInput.Focus()
		return m, textinput.Blink
	case model.RoleMaster:
		m.applyStep = applyStepCIDR
		m.textLabel = "NTP serve CIDR"
		m.textInput.SetValue(m.opts.NTPServeCIDR)
		m.textInput.Placeholder = m.opts.NTPServeCIDR
		m.textInput.Focus()
		return m, textinput.Blink
	case model.RoleClient:
		m.applyStep = applyStepSource
		m.textLabel = "Upstream host or IP"
		m.textInput.SetValue("")
		m.textInput.Placeholder = ""
		m.textInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m appModel) applyBack() {
	switch m.applyStep {
	case applyStepIface:
		m.applyStep = applyStepRole
		m.applyItems = roleChoices()
	case applyStepIfaceText:
		m.applyStep = applyStepIface
		m.applyItems = interfaceChoices(m.report.Interfaces)
	case applyStepNTPPool:
		m.applyStep = applyStepIface
		m.applyItems = interfaceChoices(m.report.Interfaces)
	case applyStepPTPAuto:
		m.applyStep = applyStepNTPPool
		m.textLabel = "NTP pool"
	case applyStepCIDR:
		m.applyStep = applyStepIface
		m.applyItems = interfaceChoices(m.report.Interfaces)
	case applyStepPTPMaster:
		m.applyStep = applyStepCIDR
		m.textLabel = "NTP serve CIDR"
	case applyStepSource:
		m.applyStep = applyStepIface
		m.applyItems = interfaceChoices(m.report.Interfaces)
	case applyStepPTPClient:
		m.applyStep = applyStepSource
		m.textLabel = "Upstream host or IP"
	case applyStepAction:
		m.applyStep = applyStepPlan
	case applyStepConfirm:
		m.applyStep = applyStepAction
		m.applyItems = nil
	}
	m.applyCursor = 0
}

func (m appModel) showPlan() (appModel, tea.Cmd) {
	plan, err := buildPlan(m.opts)
	if err != nil {
		m.finalErr = err
		m.content = fmt.Sprintf("Planning failed: %v\n", err)
		m.viewport.SetContent(m.content)
		m.screen = screenDone
		return m, nil
	}
	m.plan = plan
	m.applyStep = applyStepPlan
	m.content = planner.FormatPlan(plan)
	m.viewport.SetContent(m.content)
	return m, nil
}

func (m appModel) viewApply() string {
	switch m.applyStep {
	case applyStepRole:
		return renderMenu("Apply — select role", m.applyItems, m.applyCursor, "↑/↓ navigate · enter select · esc back · q quit")
	case applyStepIface:
		return renderMenu("Apply — select interface", m.applyItems, m.applyCursor, "↑/↓ navigate · enter select · esc back · q quit")
	case applyStepIfaceText, applyStepNTPPool, applyStepCIDR, applyStepSource:
		var b strings.Builder
		b.WriteString(titleStyle.Render("Apply — " + m.textLabel))
		b.WriteString("\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter confirm · esc back · q quit"))
		return b.String()
	case applyStepPTPAuto, applyStepPTPMaster, applyStepPTPClient:
		return renderMenu("Apply — "+m.yesNoQuestion, m.applyItems, m.applyCursor, "↑/↓ navigate · enter select · esc back · q quit")
	case applyStepPlan:
		return renderViewport("Apply — planned changes", m.viewport, "enter continue · esc back · q quit")
	case applyStepAction:
		return renderMenu("Apply — choose action", m.actionItems, m.applyCursor, "↑/↓ navigate · enter select · esc back · q quit")
	case applyStepConfirm:
		title := "Apply — confirm overwrite\n\n" + formatConflictSummary(m.conflicts)
		return renderMenu(title, m.applyItems, m.applyCursor, "↑/↓ navigate · enter select · esc back · q quit")
	}
	return ""
}

func renderMenu(title string, items []string, cursor int, help string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	for i, item := range items {
		if i == cursor {
			b.WriteString(selectedStyle.Render("▸ " + item))
		} else {
			b.WriteString("  " + item)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(help))
	return b.String()
}

func renderViewport(title string, vp viewport.Model, help string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(vp.View())
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(help))
	return b.String()
}

func roleChoices() []string {
	return []string{
		"auto — internet time sync",
		"master — serve local time",
		"client — follow upstream source",
	}
}

func interfaceChoices(ifaces []detect.InterfaceInfo) []string {
	if len(ifaces) == 0 {
		return []string{"eth0 (default)", "Enter name manually..."}
	}
	choices := make([]string, len(ifaces))
	for i, iface := range ifaces {
		state := "down"
		if iface.Up {
			state = "up"
		}
		choices[i] = fmt.Sprintf("%s (%s)", iface.Name, state)
	}
	choices = append(choices, "eth0 (default, optional)")
	return choices
}

func yesNoChoices() []string {
	return []string{"Yes", "No"}
}

func runDoctorCmd() tea.Msg {
	report, err := detect.Run()
	return doctorResultMsg{text: formatDoctorSummary(report, err), err: err}
}

func runStatusCmd() tea.Msg {
	report, err := status.Collect()
	if err != nil {
		return statusResultMsg{err: err}
	}
	return statusResultMsg{report: report}
}

func runDetectCmd() tea.Msg {
	report, err := detect.Run()
	return detectResultMsg{report: report, err: err}
}
