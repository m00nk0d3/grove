package modal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/markdown"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// LabInspectorRun is one of an entry's Sandcastle runs, with the Herdr pane
// its agent runs in when it has one.
type LabInspectorRun struct {
	Workflow domain.WorkflowRunRef
	PaneID   string
}

// Live reports whether the run has not finished.
func (r LabInspectorRun) Live() bool {
	switch strings.ToLower(r.Workflow.Status) {
	case domain.WorkflowQueued, domain.WorkflowRunning, domain.WorkflowBlocked:
		return true
	}
	return false
}

// LabInspectorState is what the inspector shows for an entry. Grove derives
// the status and stage from the entry and its runs, the same way the Lab list
// does, so both always agree.
type LabInspectorState struct {
	Entry domain.LabEntry
	// Badge is the entry's status as the list shows it, such as
	// "WAITING ON YOU".
	Badge string
	// Attention is set when the entry needs the user.
	Attention bool
	// StageLabel names the step reached, such as "Interview 1/4"; empty for a
	// draft.
	StageLabel string
	// Runs are the entry's runs, oldest first.
	Runs []LabInspectorRun
}

// latestRun returns the entry's most recent run.
func (s LabInspectorState) latestRun() (LabInspectorRun, bool) {
	if len(s.Runs) == 0 {
		return LabInspectorRun{}, false
	}
	return s.Runs[len(s.Runs)-1], true
}

// LabArtifactsRequestedMsg asks Grove to read an entry's drafted artifacts.
type LabArtifactsRequestedMsg struct {
	EntryID string
}

// LabArtifactsLoadedMsg carries an entry's drafted artifacts.
type LabArtifactsLoadedMsg struct {
	EntryID   string
	Artifacts []domain.LabArtifact
	Err       error
}

// LabOpenPaneMsg asks Grove to focus the Herdr pane of a run's agent.
type LabOpenPaneMsg struct {
	RunID string
}

type labInspectorTab int

const (
	labInspectorOverview labInspectorTab = iota
	labInspectorSteps
	labInspectorArtifacts
	labInspectorCapture
	labInspectorTabCount
)

var labInspectorTabLabels = [labInspectorTabCount]string{"Overview", "Steps", "Artifacts", "Capture"}

// Rows the inspector draws around a scrolling document: the fullscreen box
// border, margin and title; the header; the tab row; the artifact selector;
// and the footer.
const labInspectorChromeRows = 5 + 9 + 3 + 3 + 2

// LabInspectorModal is the fullscreen inspector for a Lab entry.
type LabInspectorModal struct {
	state     LabInspectorState
	activeTab labInspectorTab

	artifacts       []domain.LabArtifact
	artifactsLoaded bool
	artifactsErr    error
	selected        int
	scroll          int
	height          int

	inspectorChrome
}

// NewLabInspectorModal opens the inspector on state's entry.
func NewLabInspectorModal(state LabInspectorState) *LabInspectorModal {
	return &LabInspectorModal{state: state}
}

// EntryID returns the ID of the inspected entry.
func (m *LabInspectorModal) EntryID() string { return m.state.Entry.ID }

// SetState replaces what the inspector shows for its entry.
func (m *LabInspectorModal) SetState(state LabInspectorState) { m.state = state }

// SetArtifacts records the entry's artifacts, keeping the selected artifact
// when it still exists.
func (m *LabInspectorModal) SetArtifacts(msg LabArtifactsLoadedMsg) {
	if msg.EntryID != m.EntryID() {
		return
	}
	var selectedPath string
	if m.selected < len(m.artifacts) {
		selectedPath = m.artifacts[m.selected].Path
	}
	m.artifacts, m.artifactsErr, m.artifactsLoaded = msg.Artifacts, msg.Err, true
	m.selected = 0
	for i, a := range m.artifacts {
		if a.Path == selectedPath {
			m.selected = i
		}
	}
	m.clampScroll()
}

// Init asks for the entry's artifacts.
func (m *LabInspectorModal) Init() tea.Cmd { return m.requestArtifacts() }

// Title returns the modal title.
func (m *LabInspectorModal) Title() string { return "LAB INSPECTOR" }

// Fullscreen makes the inspector fill the terminal.
func (m *LabInspectorModal) Fullscreen() bool { return true }

// SetWidth records the terminal width.
func (m *LabInspectorModal) SetWidth(width int) { m.width = width }

// SetHeight records the terminal height.
func (m *LabInspectorModal) SetHeight(height int) { m.height = height }

// SetTheme applies the active theme.
func (m *LabInspectorModal) SetTheme(theme styles.Theme) { m.theme = &theme }

func (m *LabInspectorModal) requestArtifacts() tea.Cmd {
	id := m.EntryID()
	return func() tea.Msg { return LabArtifactsRequestedMsg{EntryID: id} }
}

// Update handles navigation, scrolling, opening the pane, and closing.
func (m *LabInspectorModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case tea.KeyTab, tea.KeyRight:
		return m, m.setTab(m.activeTab + 1)
	case tea.KeyShiftTab, tea.KeyLeft:
		return m, m.setTab(m.activeTab + labInspectorTabCount - 1)
	case tea.KeyEnter:
		if run, ok := m.state.latestRun(); ok && run.Live() && run.PaneID != "" {
			runID := firstNonEmpty(run.Workflow.RunID, run.Workflow.WorkflowID)
			return m, func() tea.Msg { return LabOpenPaneMsg{RunID: runID} }
		}
		return m, nil
	case tea.KeyDown:
		m.scrollBy(1)
	case tea.KeyUp:
		m.scrollBy(-1)
	case tea.KeyPgDown, tea.KeyCtrlD:
		m.scrollBy(max(m.documentRows()-2, 1))
	case tea.KeyPgUp, tea.KeyCtrlU:
		m.scrollBy(-max(m.documentRows()-2, 1))
	case tea.KeyHome:
		m.scroll = 0
	case tea.KeyEnd:
		m.scrollBy(1 << 20)
	case tea.KeyRunes:
		switch s := key.String(); s {
		case "q":
			return m, func() tea.Msg { return ModalCancelledMsg{} }
		case "h":
			return m, m.setTab(m.activeTab + labInspectorTabCount - 1)
		case "l":
			return m, m.setTab(m.activeTab + 1)
		case "j":
			m.scrollBy(1)
		case "k":
			m.scrollBy(-1)
		case "g":
			m.scroll = 0
		case "G":
			m.scrollBy(1 << 20)
		case "[":
			m.selectArtifact(-1)
		case "]":
			m.selectArtifact(1)
		case "r", "R":
			return m, tea.Batch(func() tea.Msg { return MissionRefreshMsg{} }, m.requestArtifacts())
		default:
			if len(s) == 1 && s[0] >= '1' && s[0] < '1'+byte(labInspectorTabCount) {
				return m, m.setTab(labInspectorTab(s[0] - '1'))
			}
		}
	}
	return m, nil
}

func (m *LabInspectorModal) setTab(tab labInspectorTab) tea.Cmd {
	m.activeTab = tab % labInspectorTabCount
	m.scroll = 0
	if m.activeTab == labInspectorArtifacts {
		return m.requestArtifacts()
	}
	return nil
}

func (m *LabInspectorModal) selectArtifact(delta int) {
	if m.activeTab != labInspectorArtifacts || len(m.artifacts) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(m.artifacts)) % len(m.artifacts)
	m.scroll = 0
}

// documentRows is how many lines of a scrolling document fit on screen.
func (m *LabInspectorModal) documentRows() int {
	if m.height < 20 {
		return 20
	}
	return max(m.height-labInspectorChromeRows, 3)
}

// documentLines returns the scrolling document of the active tab: the
// selected artifact or the captured text.
func (m *LabInspectorModal) documentLines() []string {
	width := m.contentWidth() - 2
	theme := m.currentTheme()
	switch m.activeTab {
	case labInspectorArtifacts:
		if m.selected >= len(m.artifacts) {
			return nil
		}
		a := m.artifacts[m.selected]
		body := a.Body
		if !strings.HasSuffix(a.Path, ".md") {
			body = "```\n" + strings.TrimRight(body, "\n") + "\n```"
		}
		return strings.Split(markdown.Render(body, width, theme), "\n")
	case labInspectorCapture:
		return strings.Split(markdown.Render(m.state.Entry.Text, width, theme), "\n")
	}
	return nil
}

func (m *LabInspectorModal) scrollBy(delta int) {
	m.scroll += delta
	m.clampScroll()
}

func (m *LabInspectorModal) clampScroll() {
	m.scroll = min(m.scroll, max(len(m.documentLines())-m.documentRows(), 0))
	m.scroll = max(m.scroll, 0)
}

// View renders the header, the tabs, the active tab, and the key hints.
func (m *LabInspectorModal) View() string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderTabs())
	b.WriteString("\n\n")
	switch m.activeTab {
	case labInspectorOverview:
		b.WriteString(m.renderOverview())
	case labInspectorSteps:
		b.WriteString(m.renderSteps())
	case labInspectorArtifacts:
		b.WriteString(m.renderArtifacts())
	case labInspectorCapture:
		b.WriteString(m.renderDocument(m.documentLines()))
	}
	b.WriteString("\n\n")
	hints := []string{"Tab/h/l", "sections"}
	switch m.activeTab {
	case labInspectorArtifacts:
		hints = append(hints, "j/k PgUp/PgDn", "scroll")
		if len(m.artifacts) > 1 {
			hints = append(hints, "[ ]", "artifact")
		}
	case labInspectorCapture:
		hints = append(hints, "j/k PgUp/PgDn", "scroll")
	}
	if run, ok := m.state.latestRun(); ok && run.Live() && run.PaneID != "" {
		hints = append(hints, "Enter", "open pane")
	}
	hints = append(hints, "r", "refresh", "Esc", "close")
	b.WriteString(m.keyHints(hints...))
	return b.String()
}

func (m *LabInspectorModal) renderHeader() string {
	e := m.state.Entry
	badgeStyle := m.accentStyle()
	if m.state.Attention {
		badgeStyle = m.statusStyle("blocked")
	}
	var b strings.Builder
	b.WriteString(m.accentStyle().Bold(true).Render("◈ LAB // ENTRY INSPECTOR"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.truncateTo(firstNonEmpty(e.Title(), "Untitled entry"), m.contentWidth()-2)))
	b.WriteString("\n")
	b.WriteString(m.mutedStyle().Render(strings.ToUpper(string(e.Kind)) + "  //  "))
	b.WriteString(badgeStyle.Bold(true).Render(m.state.Badge))
	if m.state.StageLabel != "" {
		b.WriteString(m.mutedStyle().Render("  //  " + m.state.StageLabel))
	}
	b.WriteString("\n\n")

	stages := e.LabStages()
	stage := "—"
	if n := e.Stage(); n > 0 {
		stage = fmt.Sprintf("%d / %d", n, len(stages))
	}
	cardWidth := max(12, (m.contentWidth()-9)/4)
	cards := []string{
		m.metricCard("STAGE", stage, cardWidth),
		m.metricCard("ELAPSED", m.elapsed(time.Now()), cardWidth),
		m.metricCard("ARTIFACTS", m.artifactCount(), cardWidth),
		m.metricCard("ISSUES", m.issueCount(), cardWidth),
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	return b.String()
}

// elapsed is how long the entry has been in the build chain: since its first
// run started, or since it was captured when it has no run.
func (m *LabInspectorModal) elapsed(now time.Time) string {
	since := m.state.Entry.Created
	if len(m.state.Runs) > 0 && !m.state.Runs[0].Workflow.StartedAt.IsZero() {
		since = m.state.Runs[0].Workflow.StartedAt
	}
	if since.IsZero() {
		return "—"
	}
	until := now
	if run, ok := m.state.latestRun(); ok && !run.Live() && !run.Workflow.UpdatedAt.IsZero() {
		until = run.Workflow.UpdatedAt
	}
	return formatDuration(until.Sub(since))
}

func (m *LabInspectorModal) artifactCount() string {
	if !m.artifactsLoaded {
		return "…"
	}
	return fmt.Sprintf("%02d", len(m.artifacts))
}

func (m *LabInspectorModal) issueCount() string {
	issues := m.state.Entry.Issues
	n := len(issues.Tickets)
	if issues.Epic != nil {
		n++
	}
	if issues.Issue != nil {
		n++
	}
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%02d", n)
}

func (m *LabInspectorModal) renderTabs() string {
	var b strings.Builder
	for i, label := range labInspectorTabLabels {
		style := m.mutedStyle()
		if labInspectorTab(i) == m.activeTab {
			style = m.selectedStyle()
		}
		b.WriteString(style.Render(fmt.Sprintf(" %d %s ", i+1, strings.ToUpper(label))))
		if i < len(labInspectorTabLabels)-1 {
			b.WriteString(m.mutedStyle().Render("  ──  "))
		}
	}
	return b.String()
}

func (m *LabInspectorModal) renderOverview() string {
	e := m.state.Entry
	columnWidth := max(24, (m.contentWidth()-3)/2)

	var entry strings.Builder
	entry.WriteString(m.field("Kind", string(e.Kind)))
	entry.WriteString(m.field("Status", m.state.Badge))
	if e.Mode != "" {
		entry.WriteString(m.field("Mode", string(e.Mode)))
	}
	if e.Archived {
		entry.WriteString(m.field("Archived", "yes — restore to continue"))
	}
	entry.WriteString(m.field("Captured", formatTimestamp(e.Created)))
	entry.WriteString(m.field("Updated", formatTimestamp(e.Updated)))
	entry.WriteString(m.field("Issues", m.issueList()))

	var session strings.Builder
	if run, ok := m.state.latestRun(); ok {
		wf := run.Workflow
		status := strings.ToUpper(firstNonEmpty(wf.Status, domain.UnknownState))
		if m.state.Attention {
			status = "WAITING ON YOU"
		}
		session.WriteString(m.field("Run", firstNonEmpty(wf.RunID, wf.WorkflowID)))
		session.WriteString(m.field("Status", m.statusStyle(wf.Status).Render(status)))
		session.WriteString(m.field("Current", firstNonEmpty(wf.CurrentStep, "—")))
		session.WriteString(m.field("Agent", firstNonEmpty(wf.DefaultAgent, "—")))
		pane := "—"
		if run.PaneID != "" {
			pane = "Herdr " + run.PaneID
			if run.Live() {
				pane += "  (Enter to open)"
			}
		}
		session.WriteString(m.field("Pane", pane))
		session.WriteString(m.field("Started", formatTimestamp(wf.StartedAt)))
		if wf.Error != "" {
			session.WriteString(m.field("Error", m.truncateTo(wf.Error, columnWidth-20)))
		}
	} else {
		session.WriteString(m.mutedStyle().Render("  No session yet.\n"))
		next := "Grill"
		if e.Kind == domain.LabKindBug {
			next = "Shape"
		}
		session.WriteString(m.mutedStyle().Render(fmt.Sprintf("  %s the entry to start one.", next)))
	}

	left := m.sectionPanel("ENTRY", strings.TrimRight(entry.String(), "\n"), columnWidth)
	right := m.sectionPanel("SESSION", strings.TrimRight(session.String(), "\n"), columnWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m *LabInspectorModal) issueList() string {
	issues := m.state.Entry.Issues
	var parts []string
	if issues.Epic != nil {
		parts = append(parts, fmt.Sprintf("epic #%d", *issues.Epic))
	}
	for _, n := range issues.Tickets {
		parts = append(parts, fmt.Sprintf("#%d", n))
	}
	if issues.Issue != nil {
		parts = append(parts, fmt.Sprintf("#%d", *issues.Issue))
	}
	if len(parts) == 0 {
		return "—"
	}
	return m.truncateTo(strings.Join(parts, ", "), m.contentWidth()/2-22)
}

// renderSteps shows the build chain the entry takes and the runs it has had.
func (m *LabInspectorModal) renderSteps() string {
	e := m.state.Entry
	stages := e.LabStages()
	reached := e.Stage()
	done := e.Status == domain.LabStatusPublished

	var b strings.Builder
	b.WriteString(m.heading(fmt.Sprintf("BUILD CHAIN  %d/%d", reached, len(stages))))
	b.WriteString("\n")
	for i, stage := range stages {
		icon, style := "○", m.mutedStyle()
		switch {
		case done || i+1 < reached:
			icon, style = "✓", m.statusStyle("succeeded")
		case i+1 == reached && m.state.Attention:
			icon, style = "◆", m.statusStyle("blocked")
		case i+1 == reached:
			icon, style = "●", m.statusStyle("running")
		}
		b.WriteString(style.Render(fmt.Sprintf("  %s  %s", icon, stage)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.heading(fmt.Sprintf("RUNS  %d", len(m.state.Runs))))
	b.WriteString("\n")
	if len(m.state.Runs) == 0 {
		b.WriteString(m.mutedStyle().Render("  No runs yet."))
		return b.String()
	}
	for _, run := range m.state.Runs {
		wf := run.Workflow
		b.WriteString(m.statusStyle(wf.Status).Render(fmt.Sprintf("  %s  %-8s %s", stepIcon(wf.Status), firstNonEmpty(wf.Kind, "run"), strings.ToUpper(firstNonEmpty(wf.Status, domain.UnknownState)))))
		b.WriteString(m.mutedStyle().Render(fmt.Sprintf("   %s  •  started %s", firstNonEmpty(wf.RunID, wf.WorkflowID), formatTimestamp(wf.StartedAt))))
		b.WriteString("\n")
		for _, step := range wf.Steps {
			b.WriteString(m.statusStyle(step.Status).Render(fmt.Sprintf("       %s  %s", stepIcon(step.Status), step.Title)))
			if d := stepDuration(step); d != "" {
				b.WriteString(m.mutedStyle().Render("  " + d))
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *LabInspectorModal) renderArtifacts() string {
	switch {
	case !m.artifactsLoaded:
		return m.mutedStyle().Render("  Loading artifacts…")
	case m.artifactsErr != nil:
		return m.statusStyle("failed").Render("  Could not read artifacts: "+m.truncateTo(m.artifactsErr.Error(), m.contentWidth()-30)) +
			"\n" + m.mutedStyle().Render("  Press r to try again.")
	case len(m.artifacts) == 0:
		return m.mutedStyle().Render("  Nothing drafted yet. The agent's CONTEXT.md, decision records, spec,\n  and tickets appear here as it writes them.")
	}
	var b strings.Builder
	for i, a := range m.artifacts {
		style := m.mutedStyle()
		if i == m.selected {
			style = m.selectedStyle()
		}
		b.WriteString(style.Render(" " + a.Path + " "))
		b.WriteString(" ")
	}
	b.WriteString("\n")
	a := m.artifacts[m.selected]
	b.WriteString(m.mutedStyle().Render(fmt.Sprintf("  written %s  •  %d lines", formatTimestamp(a.ModTime), strings.Count(a.Body, "\n")+1)))
	b.WriteString("\n")
	b.WriteString(m.mutedStyle().Render(strings.Repeat("─", m.contentWidth())))
	b.WriteString("\n")
	b.WriteString(m.renderDocument(m.documentLines()))
	return b.String()
}

// renderDocument draws the visible window of lines.
func (m *LabInspectorModal) renderDocument(lines []string) string {
	m.clampScroll()
	end := min(m.scroll+m.documentRows(), len(lines))
	window := lines[min(m.scroll, end):end]
	out := strings.Join(window, "\n")
	if len(lines) > m.documentRows() {
		out += "\n" + m.mutedStyle().Render(fmt.Sprintf("  lines %d–%d of %d", m.scroll+1, end, len(lines)))
	}
	return out
}
