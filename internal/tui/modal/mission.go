package modal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

type missionTab int

const (
	missionOverview missionTab = iota
	missionSteps
	missionMetrics
	missionImplementation
	missionReports
	missionTabCount
)

var missionTabLabels = [missionTabCount]string{"Overview", "Steps", "Metrics", "Implementation", "Reports"}

const (
	missionTabsRow       = 9
	missionStepFirstRow  = missionTabsRow + 3
	missionModalMargin   = 4
	missionHeaderSpacing = 3
)

// MissionModal presents live, structured Sandcastle workflow telemetry.
type MissionModal struct {
	workflow      domain.WorkflowRunRef
	agents        []domain.AgentRef
	activeTab     missionTab
	selectedStep  int
	scrollOffset  int
	width         int
	height        int
	theme         *styles.Theme
	lastStepClick int
	lastClickAt   time.Time

	reports          []domain.WorkflowReport
	reportsSupported bool
	reportsErr       error
	reportsLoaded    bool
	reportsLoading   bool
	selectedReport   int
	rendered         renderedReport
}

func NewMissionModal(workflow domain.WorkflowRunRef, agents []domain.AgentRef) *MissionModal {
	return &MissionModal{
		workflow:      workflow,
		agents:        append([]domain.AgentRef(nil), agents...),
		lastStepClick: -1,
	}
}

func (m *MissionModal) Init() tea.Cmd { return nil }

func (m *MissionModal) Title() string { return "MISSION INSPECTOR" }

func (m *MissionModal) Fullscreen() bool { return true }

func (m *MissionModal) RunID() string {
	return firstNonEmpty(m.workflow.RunID, m.workflow.WorkflowID)
}

func (m *MissionModal) SetWidth(width int) { m.width = width }

func (m *MissionModal) SetHeight(height int) { m.height = height }

func (m *MissionModal) SetTheme(theme styles.Theme) { m.theme = &theme }

func (m *MissionModal) SetWorkflow(workflow domain.WorkflowRunRef, agents []domain.AgentRef) {
	m.workflow = workflow
	m.agents = append(m.agents[:0], agents...)
	if len(workflow.Steps) == 0 {
		m.selectedStep = 0
	} else if m.selectedStep >= len(workflow.Steps) {
		m.selectedStep = len(workflow.Steps) - 1
	}
}

func (m *MissionModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case tea.KeyTab, tea.KeyRight:
		return m, m.setTab((m.activeTab + 1) % missionTabCount)
	case tea.KeyShiftTab, tea.KeyLeft:
		return m, m.setTab((m.activeTab + missionTabCount - 1) % missionTabCount)
	case tea.KeyEnter:
		return m, func() tea.Msg { return MissionJumpMsg{RunID: m.RunID()} }
	case tea.KeyUp:
		m.moveUp()
		return m, nil
	case tea.KeyDown:
		m.moveDown()
		return m, nil
	case tea.KeyPgDown, tea.KeyCtrlD:
		m.scrollReport(max(m.reportRows()-2, 1))
		return m, nil
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.scrollReport(-max(m.reportRows()-2, 1))
		return m, nil
	case tea.KeyHome:
		m.scrollOffset = 0
		return m, nil
	case tea.KeyEnd:
		m.scrollReport(1 << 20)
		return m, nil
	case tea.KeyRunes:
		switch key.String() {
		case "q":
			return m, func() tea.Msg { return ModalCancelledMsg{} }
		case "h":
			return m, m.setTab((m.activeTab + missionTabCount - 1) % missionTabCount)
		case "l":
			return m, m.setTab((m.activeTab + 1) % missionTabCount)
		case "j":
			m.moveDown()
		case "k":
			m.moveUp()
		case "g":
			m.scrollOffset = 0
		case "G":
			m.scrollReport(1 << 20)
		case "[":
			m.selectReport(-1)
		case "]":
			m.selectReport(1)
		case "r", "R":
			refresh := func() tea.Msg { return MissionRefreshMsg{} }
			if m.activeTab == missionReports {
				return m, tea.Batch(refresh, m.requestReports())
			}
			return m, refresh
		case "t", "T":
			if strings.EqualFold(m.workflow.Status, domain.WorkflowFailed) {
				return m, func() tea.Msg { return MissionRetryRequestedMsg{RunID: m.RunID()} }
			}
		case "x", "X":
			return m, func() tea.Msg { return MissionRemoveRequestedMsg{RunID: m.RunID()} }
		default:
			if s := key.String(); len(s) == 1 && s[0] >= '1' && s[0] < '1'+byte(missionTabCount) {
				return m, m.setTab(missionTab(s[0] - '1'))
			}
		}
	}
	return m, nil
}

// setTab switches to tab. Entering the reports tab asks for the reports to be
// read again, so a report written since the last visit shows up.
func (m *MissionModal) setTab(tab missionTab) tea.Cmd {
	m.activeTab = tab
	m.scrollOffset = 0
	if tab == missionReports {
		return m.requestReports()
	}
	return nil
}

func (m *MissionModal) requestReports() tea.Cmd {
	if !m.reportsLoaded {
		m.reportsLoading = true
	}
	runID, workflow := m.RunID(), m.workflow
	return func() tea.Msg { return MissionReportsRequestedMsg{RunID: runID, Workflow: workflow} }
}

// SetReports stores the reports Grove found for the inspected run. The report
// on screen stays selected when it is still among them.
func (m *MissionModal) SetReports(msg MissionReportsLoadedMsg) {
	var selectedPath string
	if m.selectedReport >= 0 && m.selectedReport < len(m.reports) {
		selectedPath = m.reports[m.selectedReport].Path
	}
	m.reports = msg.Reports
	m.reportsSupported = msg.Supported
	m.reportsErr = msg.Err
	m.reportsLoaded = true
	m.reportsLoading = false
	m.rendered = renderedReport{}
	m.selectedReport = 0
	for i, report := range m.reports {
		if report.Path == selectedPath {
			m.selectedReport = i
		}
	}
	m.scrollOffset = min(m.scrollOffset, m.maxReportScroll())
}

func (m *MissionModal) selectReport(delta int) {
	if m.activeTab != missionReports || len(m.reports) < 2 {
		return
	}
	n := len(m.reports)
	m.selectedReport = (m.selectedReport + delta + n) % n
	m.scrollOffset = 0
}

// scrollReport moves the report view by delta lines, stopping at either end.
func (m *MissionModal) scrollReport(delta int) {
	if m.activeTab != missionReports {
		return
	}
	m.scrollOffset = max(0, min(m.scrollOffset+delta, m.maxReportScroll()))
}

// HandleMouse maps clicks and wheel events inside the centered inspector.
func (m *MissionModal) HandleMouse(msg tea.MouseMsg, _ int, screenHeight int) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveUp()
		return m, nil
	case tea.MouseButtonWheelDown:
		m.moveDown()
		return m, nil
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
	default:
		return m, nil
	}

	contentTop := 3
	if screenHeight < 20 {
		contentHeight := lipgloss.Height(m.View())
		contentTop = max(0, (screenHeight-(contentHeight+3))/2) + 2
	}
	contentLeft := missionModalMargin

	if msg.Y == contentTop+missionTabsRow {
		x := contentLeft
		for i, label := range missionTabLabels {
			width := len(fmt.Sprintf(" %d %s ", i+1, label))
			if msg.X >= x && msg.X < x+width {
				return m, m.setTab(missionTab(i))
			}
			x += width + 6
		}
	}

	if m.activeTab != missionSteps {
		return m, nil
	}
	row := msg.Y - contentTop - missionStepFirstRow
	if row < 0 {
		return m, nil
	}
	maxRows := m.height - 21
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > len(m.workflow.Steps) {
		maxRows = len(m.workflow.Steps)
	}
	start := 0
	if m.selectedStep >= maxRows {
		start = m.selectedStep - maxRows + 1
	}
	idx := start + row
	if idx < 0 || idx >= len(m.workflow.Steps) {
		return m, nil
	}
	doubleClick := idx == m.lastStepClick && !m.lastClickAt.IsZero() &&
		time.Since(m.lastClickAt) <= 450*time.Millisecond
	m.selectedStep = idx
	m.lastStepClick = idx
	m.lastClickAt = time.Now()
	if doubleClick {
		return m, func() tea.Msg { return MissionJumpMsg{RunID: m.RunID()} }
	}
	return m, nil
}

func (m *MissionModal) moveDown() {
	if m.activeTab == missionReports {
		m.scrollReport(1)
		return
	}
	if m.activeTab == missionSteps && m.selectedStep < len(m.workflow.Steps)-1 {
		m.selectedStep++
		return
	}
	m.scrollOffset++
}

func (m *MissionModal) moveUp() {
	if m.activeTab == missionReports {
		m.scrollReport(-1)
		return
	}
	if m.activeTab == missionSteps && m.selectedStep > 0 {
		m.selectedStep--
		return
	}
	if m.scrollOffset > 0 {
		m.scrollOffset--
	}
}

func (m *MissionModal) View() string {
	var b strings.Builder
	b.WriteString(m.renderCommandHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderTabs())
	b.WriteString("\n\n")
	switch m.activeTab {
	case missionOverview:
		b.WriteString(m.renderOverview())
	case missionSteps:
		b.WriteString(m.renderSteps())
	case missionMetrics:
		b.WriteString(m.renderMetrics())
	case missionImplementation:
		b.WriteString(m.renderImplementation())
	case missionReports:
		b.WriteString(m.renderReports())
	}
	b.WriteString("\n\n")
	b.WriteString(m.keyStyle().Render("Tab/h/l"))
	b.WriteString(m.mutedStyle().Render(" sections  ·  "))
	if m.activeTab == missionReports {
		b.WriteString(m.keyStyle().Render("j/k PgUp/PgDn"))
		b.WriteString(m.mutedStyle().Render(" scroll  ·  "))
		if len(m.reports) > 1 {
			b.WriteString(m.keyStyle().Render("[ ]"))
			b.WriteString(m.mutedStyle().Render(" report  ·  "))
		}
	} else {
		b.WriteString(m.keyStyle().Render("j/k"))
		b.WriteString(m.mutedStyle().Render(" navigate  ·  "))
	}
	b.WriteString(m.keyStyle().Render("Enter"))
	b.WriteString(m.mutedStyle().Render(" jump  ·  "))
	b.WriteString(m.keyStyle().Render("r"))
	b.WriteString(m.mutedStyle().Render(" refresh  ·  "))
	if strings.EqualFold(m.workflow.Status, domain.WorkflowFailed) {
		b.WriteString(m.keyStyle().Render("t"))
		b.WriteString(m.mutedStyle().Render(" retry  ·  "))
	}
	b.WriteString(m.keyStyle().Render("x"))
	b.WriteString(m.mutedStyle().Render(" remove  ·  "))
	b.WriteString(m.keyStyle().Render("Esc"))
	b.WriteString(m.mutedStyle().Render(" close"))
	return b.String()
}

func (m *MissionModal) renderCommandHeader() string {
	wf := m.workflow
	title := firstNonEmpty(wf.Title, wf.WorkflowID, wf.RunID, "Untitled mission")
	status := strings.ToUpper(firstNonEmpty(wf.Status, domain.UnknownState))
	var b strings.Builder
	b.WriteString(m.accentStyle().Bold(true).Render("◈ SANDCASTLE // MISSION INTELLIGENCE"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.truncateTo(title, m.contentWidth()-2)))
	b.WriteString("\n")
	b.WriteString(m.mutedStyle().Render("RUN " + firstNonEmpty(wf.RunID, wf.WorkflowID, "—") + "  //  "))
	b.WriteString(m.statusStyle(wf.Status).Bold(true).Render(status))
	b.WriteString(m.mutedStyle().Render("  //  LIVE TELEMETRY"))
	b.WriteString("\n\n")

	cardWidth := max(12, (m.contentWidth()-missionHeaderSpacing*3)/4)
	cards := []string{
		m.metricCard("PROGRESS", fmt.Sprintf("%d%%", clampPercent(wf.Progress.Percent)), cardWidth),
		m.metricCard("ELAPSED", workflowElapsed(wf), cardWidth),
		m.metricCard("STEPS", fmt.Sprintf("%d / %d", wf.Progress.Completed, wf.Progress.Total), cardWidth),
		m.metricCard("AGENTS", fmt.Sprintf("%02d", len(m.agents)), cardWidth),
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	return b.String()
}

func (m *MissionModal) renderTabs() string {
	var b strings.Builder
	for i, label := range missionTabLabels {
		style := m.mutedStyle()
		if missionTab(i) == m.activeTab {
			style = m.selectedStyle()
		}
		b.WriteString(style.Render(fmt.Sprintf(" %d %s ", i+1, strings.ToUpper(label))))
		if i < len(missionTabLabels)-1 {
			b.WriteString(m.mutedStyle().Render("  ──  "))
		}
	}
	return b.String()
}

func (m *MissionModal) renderOverview() string {
	wf := m.workflow
	columnWidth := max(24, (m.contentWidth()-3)/2)
	var mission strings.Builder
	mission.WriteString(m.field("Status", strings.ToUpper(firstNonEmpty(wf.Status, domain.UnknownState))))
	mission.WriteString(m.field("Current", firstNonEmpty(wf.CurrentStep, "Waiting for telemetry")))
	mission.WriteString(m.field("Agent", firstNonEmpty(wf.DefaultAgent, "opencode")))
	mission.WriteString(m.field("Progress", progressLabel(wf.Progress)))
	mission.WriteString("\n  " + progressBar(wf.Progress.Percent, max(10, columnWidth-6)))

	var context strings.Builder
	context.WriteString(m.field("Branch", firstNonEmpty(wf.Branch, "—")))
	context.WriteString(m.field("Worktree", m.truncateTo(firstNonEmpty(wf.WorktreePath, "—"), columnWidth-18)))
	context.WriteString(m.field("Repository", m.truncateTo(firstNonEmpty(wf.Repo, "—"), columnWidth-18)))
	if wf.IssueNumber != nil {
		context.WriteString(m.field("Issue", fmt.Sprintf("#%d", *wf.IssueNumber)))
	}
	if wf.PRNumber != nil {
		context.WriteString(m.field("Pull request", fmt.Sprintf("#%d", *wf.PRNumber)))
	}
	context.WriteString(m.field("Updated", formatTimestamp(wf.UpdatedAt)))

	left := m.sectionPanel("MISSION SIGNAL", strings.TrimRight(mission.String(), "\n"), columnWidth)
	right := m.sectionPanel("EXECUTION CONTEXT", strings.TrimRight(context.String(), "\n"), columnWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m *MissionModal) renderSteps() string {
	steps := m.workflow.Steps
	if len(steps) == 0 {
		return m.mutedStyle().Render("No structured step telemetry is available yet.")
	}

	maxRows := m.height - 21
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > len(steps) {
		maxRows = len(steps)
	}
	start := 0
	if m.selectedStep >= maxRows {
		start = m.selectedStep - maxRows + 1
	}
	end := min(start+maxRows, len(steps))

	var b strings.Builder
	b.WriteString(m.heading(fmt.Sprintf("PIPELINE  %d/%d", m.workflow.Progress.Completed, len(steps))))
	b.WriteString("\n")
	for i := start; i < end; i++ {
		step := steps[i]
		cursor := "  "
		style := m.statusStyle(step.Status)
		if i == m.selectedStep {
			cursor = "> "
			style = m.selectedStyle()
		}
		duration := stepDuration(step)
		row := fmt.Sprintf("%s%s  %-*s  %s", cursor, stepIcon(step.Status), max(12, m.contentWidth()-28), m.truncateTo(step.Title, max(12, m.contentWidth()-28)), duration)
		b.WriteString(style.Render(row))
		b.WriteString("\n")
	}
	selected := steps[m.selectedStep]
	if selected.Summary != "" {
		b.WriteString("\n")
		b.WriteString(m.mutedStyle().Render(m.truncate(selected.Summary)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *MissionModal) renderMetrics() string {
	wf := m.workflow
	counts := make(map[string]int)
	for _, step := range wf.Steps {
		counts[strings.ToLower(step.Status)]++
	}
	total := max(1, len(wf.Steps))
	var b strings.Builder
	b.WriteString(m.heading("PROGRESS"))
	b.WriteString("\n")
	b.WriteString("  " + progressBar(wf.Progress.Percent, max(20, m.contentWidth()-12)) + fmt.Sprintf("  %d%%\n", clampPercent(wf.Progress.Percent)))
	b.WriteString(m.field("Completed", fmt.Sprintf("%d of %d steps", wf.Progress.Completed, wf.Progress.Total)))
	b.WriteString(m.field("Elapsed", workflowElapsed(wf)))
	b.WriteString(m.field("Agents", fmt.Sprintf("%d", len(m.agents))))
	b.WriteString("\n")
	b.WriteString(m.heading("STEP HEALTH"))
	b.WriteString("\n")
	for _, status := range []string{"succeeded", "running", "failed", "queued"} {
		count := counts[status]
		barWidth := 24
		filled := count * barWidth / total
		graph := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		b.WriteString(fmt.Sprintf("  %-10s %s  %d\n", strings.ToUpper(status), m.statusStyle(status).Render(graph), count))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *MissionModal) renderImplementation() string {
	var b strings.Builder
	b.WriteString(m.heading("CURRENT ACTIVITY"))
	b.WriteString("\n")
	b.WriteString("  " + firstNonEmpty(m.workflow.CurrentStep, "No current activity reported") + "\n")
	if m.workflow.Error != "" {
		b.WriteString("\n")
		b.WriteString(m.statusStyle("failed").Render("  ERROR  " + m.truncate(m.workflow.Error)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.heading(fmt.Sprintf("AGENTS  %d", len(m.agents))))
	b.WriteString("\n")
	if len(m.agents) == 0 {
		b.WriteString(m.mutedStyle().Render("  No agent activity reported."))
		return b.String()
	}
	for _, agent := range m.agents {
		name := firstNonEmpty(agent.Name, agent.AgentID, "agent")
		meta := strings.ToUpper(firstNonEmpty(agent.Status, domain.UnknownState))
		if agent.Kind != "" {
			meta = agent.Kind + " · " + meta
		}
		if agent.PaneID != "" {
			meta += " · " + agent.PaneID
		}
		b.WriteString(fmt.Sprintf("  %s  %s\n", m.statusStyle(agent.Status).Render("●"), m.accentStyle().Render(name)))
		b.WriteString(m.mutedStyle().Render("     " + meta))
		b.WriteString("\n")
		if agent.Summary != "" {
			b.WriteString("     " + m.truncate(agent.Summary))
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *MissionModal) field(label, value string) string {
	return fmt.Sprintf("  %s %s\n", m.mutedStyle().Render(fmt.Sprintf("%-14s", label)), value)
}

func (m *MissionModal) heading(value string) string {
	return m.accentStyle().Bold(true).Render("◆ " + value)
}

func (m *MissionModal) metricCard(label, value string, width int) string {
	return lipgloss.NewStyle().
		Width(max(8, width-2)).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.accentStyle().GetForeground()).
		Padding(0, 1).
		Render(m.mutedStyle().Render(label) + "\n" + m.accentStyle().Bold(true).Render(value))
}

func (m *MissionModal) sectionPanel(title, content string, width int) string {
	return lipgloss.NewStyle().
		Width(max(20, width-4)).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(m.mutedStyle().GetForeground()).
		Padding(0, 1).
		Render(m.accentStyle().Bold(true).Render(title) + "\n\n" + content)
}

func (m *MissionModal) contentWidth() int {
	width := m.width - 12
	if width < 48 {
		return 48
	}
	if width > 132 {
		return 132
	}
	return width
}

func (m *MissionModal) truncate(value string) string {
	return m.truncateTo(value, m.contentWidth()-18)
}

func (m *MissionModal) truncateTo(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(value)
	if len(runes) >= width {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

func (m *MissionModal) accentStyle() lipgloss.Style {
	if m.theme == nil {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent()))
}

func (m *MissionModal) mutedStyle() lipgloss.Style {
	if m.theme == nil {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted()))
}

func (m *MissionModal) keyStyle() lipgloss.Style { return m.accentStyle().Bold(true) }

func (m *MissionModal) selectedStyle() lipgloss.Style {
	if m.theme == nil {
		return lipgloss.NewStyle().Bold(true)
	}
	return m.theme.GetStyle("selected-row")
}

func (m *MissionModal) statusStyle(status string) lipgloss.Style {
	if m.theme == nil {
		return lipgloss.NewStyle()
	}
	switch strings.ToLower(status) {
	case "running", "working":
		return m.accentStyle()
	case "succeeded", "done":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Success()))
	case "failed", "blocked":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Warning()))
	default:
		return m.mutedStyle()
	}
}

func progressBar(percent, width int) string {
	percent = clampPercent(percent)
	if width < 10 {
		width = 10
	}
	filled := percent * width / 100
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func progressLabel(progress domain.WorkflowProgress) string {
	if progress.Total > 0 {
		return fmt.Sprintf("%d/%d steps", progress.Completed, progress.Total)
	}
	return fmt.Sprintf("%d%%", clampPercent(progress.Percent))
}

func clampPercent(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func stepIcon(status string) string {
	switch strings.ToLower(status) {
	case "succeeded", "done":
		return "✓"
	case "running", "working":
		return "◆"
	case "failed", "blocked":
		return "!"
	default:
		return "○"
	}
}

func stepDuration(step domain.WorkflowStep) string {
	if step.DurationMillis > 0 {
		return formatDuration(time.Duration(step.DurationMillis) * time.Millisecond)
	}
	if !step.StartedAt.IsZero() {
		end := step.CompletedAt
		if end.IsZero() {
			end = time.Now()
		}
		return formatDuration(end.Sub(step.StartedAt))
	}
	return "—"
}

func workflowElapsed(workflow domain.WorkflowRunRef) string {
	if workflow.StartedAt.IsZero() {
		return "—"
	}
	end := workflow.UpdatedAt
	if workflow.Status == domain.WorkflowRunning || end.IsZero() {
		end = time.Now()
	}
	return formatDuration(end.Sub(workflow.StartedAt))
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		return "—"
	}
	duration = duration.Round(time.Second)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(duration.Minutes()), int(duration.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(duration.Hours()), int(duration.Minutes())%60)
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
