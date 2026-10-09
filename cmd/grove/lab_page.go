package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// labPageFocus is the part of the entry page that has the keyboard.
type labPageFocus int

const (
	labFocusPanel labPageFocus = iota
	labFocusLog
)

// labPage is the entry page: one entry's steps, what it needs from the user
// now, and the decisions made so far. It replaces the Lab list until Esc and
// owns the keyboard while it is open. See docs/LAB_DESIGN.md, "Entry page".
type labPage struct {
	entryID string
	focus   labPageFocus
	// card is the question card being answered, or reopened for revision.
	card     *modal.LabQuestionModal
	cardNum  int
	revising bool
	// reply is the editor for answering a turn that ended without a card.
	reply   textarea.Model
	replyOn bool
	// awaiting is set once a reply or permission decision is sent, until the
	// session moves on, so the same request is not offered twice.
	awaiting  bool
	logCursor int
}

// Rows of the agent's output shown on the page.
const labPageOutputRows = 12

func newLabReplyEditor() textarea.Model {
	editor := textarea.New()
	editor.ShowLineNumbers = false
	editor.CharLimit = 0
	editor.Prompt = "│ "
	editor.Placeholder = "Your reply to the agent"
	// Grove delivers only key events, so a blinking cursor would never blink.
	editor.Cursor.SetMode(cursor.CursorStatic)
	editor.SetHeight(4)
	editor.Focus()
	return editor
}

// openLabPage shows e's entry page.
func (m *Model) openLabPage(e domain.LabEntry) {
	m.lab.selectID(e.ID)
	m.lab.page = &labPage{entryID: e.ID}
	m.focused = panelList
	m.ctxScrollOffset = 0
	m.contextActionIdx = 0
	m.syncLabPage()
}

// closeLabPage returns to the list.
func (m *Model) closeLabPage() {
	m.lab.page = nil
	m.focused = panelList
}

// syncLabPage brings the page in line with its entry's session: the card
// shown follows the first waiting question, and the reply editor appears
// while the agent waits for one.
func (m *Model) syncLabPage() {
	p := m.lab.page
	if p == nil {
		return
	}
	e, ok := m.lab.entry(p.entryID)
	if !ok {
		m.lab.page = nil
		return
	}
	if n := len(labDecisions(m.lab.talks[e.ID])); p.logCursor >= n {
		p.logCursor = max(0, n-1)
	}
	if p.revising && p.card != nil {
		return
	}
	ask, t := m.lab.ask(e)
	if ask != labAskReply && ask != labAskPermission {
		p.awaiting = false
	}
	switch ask {
	case labAskQuestion:
		p.replyOn = false
		pending := domain.LabPendingQuestions(t.questions)
		if p.card == nil || p.cardNum != pending[0].Number {
			position := ""
			if len(pending) > 1 {
				position = fmt.Sprintf("1 of %d", len(pending))
			}
			p.card = modal.NewLabQuestionModal(e.ID, pending[0].Number, *pending[0].Question, position)
			p.cardNum = pending[0].Number
			if p.focus == labFocusLog {
				p.card.Blur()
			}
		}
	case labAskReply:
		p.card = nil
		if !p.replyOn && !p.awaiting {
			p.reply = newLabReplyEditor()
			p.replyOn = true
		}
	default:
		p.card = nil
		p.replyOn = false
	}
}

// labDecisions are the questions answered so far, most recent first.
func labDecisions(t labTalk) []domain.LabQuestionRecord {
	var out []domain.LabQuestionRecord
	for _, r := range t.questions {
		if r.Question != nil && (r.Answer != nil || r.Receipt != nil) {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number > out[j].Number })
	return out
}

// labAnswerText is an answer as the decisions log shows it.
func labAnswerText(r domain.LabQuestionRecord) string {
	switch {
	case r.Answer == nil && r.Receipt != nil && r.Receipt.Via == domain.LabReceiptSkipped:
		return "not answered (interview ended)"
	case r.Answer == nil:
		return "answered in the agent's pane"
	}
	var parts []string
	for _, i := range r.Answer.Choices {
		if i >= 0 && i < len(r.Question.Options) {
			parts = append(parts, r.Question.Options[i])
		}
	}
	text := strings.Join(parts, ", ")
	if note := strings.TrimSpace(r.Answer.Text); note != "" {
		if text == "" {
			text = note
		} else {
			text += " — " + note
		}
	}
	if len(r.Answer.Revisions) > 0 {
		text += " (changed)"
	}
	return text
}

// labCanRevise reports whether answers can still be changed: only while the
// session asks questions. Later, a changed decision is a change request.
func labCanRevise(v labView, e domain.LabEntry) bool {
	run, ok := v.latestRun(e)
	t := v.talks[e.ID]
	return ok && run.live() && t.hasSession && (t.session.Stage == "interview" || t.session.Stage == "shape")
}

// handleLabPageKey handles a key while the entry page has the keyboard.
func (m *Model) handleLabPageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.lab.page
	e, ok := m.lab.entry(p.entryID)
	if !ok {
		m.closeLabPage()
		return m, nil
	}
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	typing := p.focus == labFocusPanel && ((p.card != nil && p.card.InNote()) || p.replyOn)
	if !typing {
		switch key {
		case "?":
			m.activeModal = modal.NewHelpModal()
			return m, nil
		case ".":
			m.focused = panelCtx
			m.contextActionIdx = 0
			return m, nil
		}
	}
	if p.focus == labFocusLog {
		return m.handleLabLogKey(e, key)
	}
	switch {
	case p.card != nil:
		return m.handleLabCardKey(msg)
	case p.replyOn:
		switch key {
		case "esc":
			m.closeLabPage()
			return m, nil
		case "ctrl+s":
			text := strings.TrimSpace(p.reply.Value())
			if text == "" {
				m.statusErr = "Write a reply first"
				return m, clearErrorCmd()
			}
			p.replyOn, p.awaiting = false, true
			return m.handleLabRequestSubmitted(modal.LabRequestSubmittedMsg{EntryID: e.ID, Kind: domain.LabRequestReply, Text: text})
		}
		var cmd tea.Cmd
		p.reply, cmd = p.reply.Update(msg)
		return m, cmd
	}
	if ask, _ := m.lab.ask(e); ask == labAskPermission && !p.awaiting {
		switch key {
		case "y", "Y", "n", "N":
			p.awaiting = true
			allow := key == "y" || key == "Y"
			return m.handleLabRequestSubmitted(modal.LabRequestSubmittedMsg{EntryID: e.ID, Kind: domain.LabRequestPermission, Allow: allow})
		}
	}
	switch key {
	case "enter":
		if ask, _ := m.lab.ask(e); ask == labAskPermission {
			return m, nil
		}
		return m.labPrimaryAction(e)
	case "esc":
		m.closeLabPage()
	case "tab", "down", "j":
		if len(labDecisions(m.lab.talks[e.ID])) > 0 {
			p.focus = labFocusLog
		}
	}
	return m, nil
}

// handleLabCardKey handles a key while a question card has the keyboard.
// Tab moves from the options to the note to the decisions log.
func (m *Model) handleLabCardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.lab.page
	switch msg.String() {
	case "tab", "shift+tab":
		switch {
		case p.card.HasOptions() && !p.card.InNote():
			p.card.FocusNote()
		case len(labDecisions(m.lab.talks[p.entryID])) > 0:
			p.card.Blur()
			p.focus = labFocusLog
		default:
			p.card.FocusOptions()
		}
		return m, nil
	case "esc":
		switch {
		case p.card.InNote() && p.card.HasOptions():
			p.card.FocusOptions()
		case p.revising:
			p.revising, p.card = false, nil
			m.syncLabPage()
		default:
			m.closeLabPage()
		}
		return m, nil
	}
	_, cmd := p.card.Update(msg)
	return m, cmd
}

// handleLabLogKey handles a key while the decisions log has the keyboard.
func (m *Model) handleLabLogKey(e domain.LabEntry, key string) (tea.Model, tea.Cmd) {
	p := m.lab.page
	decisions := labDecisions(m.lab.talks[e.ID])
	back := func() {
		p.focus = labFocusPanel
		if p.card != nil {
			p.card.FocusOptions()
		}
	}
	switch key {
	case "up", "k":
		if p.logCursor == 0 {
			back()
		} else {
			p.logCursor--
		}
	case "down", "j":
		if p.logCursor < len(decisions)-1 {
			p.logCursor++
		}
	case "tab", "shift+tab", "esc":
		back()
	case "enter":
		if p.logCursor >= len(decisions) {
			return m, nil
		}
		r := decisions[p.logCursor]
		if !labCanRevise(m.lab, e) || r.Answer == nil {
			m.statusMsg = "Answers can be changed only while the agent is asking"
			return m, clearMsgCmd()
		}
		p.card = modal.NewLabRevisionCard(e.ID, r.Number, *r.Question, *r.Answer)
		p.cardNum, p.revising, p.focus = r.Number, true, labFocusPanel
	}
	return m, nil
}

// labPageStep is one step of the entry page's stepper.
type labPageStep struct {
	title  string
	status string
	detail string
}

// labPageSteps are the steps of e's session: its run's when it has one,
// otherwise the steps its kind goes through.
func labPageSteps(v labView, e domain.LabEntry) []labPageStep {
	var steps []labPageStep
	if run, ok := v.latestRun(e); ok && len(run.workflow.Steps) > 0 {
		for _, s := range run.workflow.Steps {
			steps = append(steps, labPageStep{title: s.Title, status: strings.ToLower(s.Status)})
		}
	} else {
		titles := []string{"Scout", "Interview", "Spec", "Tickets", "Publish"}
		if e.Mode == domain.LabModeShape || (e.Mode == "" && e.Kind == domain.LabKindBug) {
			titles = []string{"Shape", "Review", "Publish"}
		}
		status := "queued"
		if e.Status == domain.LabStatusPublished {
			status = "succeeded"
		}
		for _, title := range titles {
			steps = append(steps, labPageStep{title: title, status: status})
		}
	}
	if t := v.talks[e.ID]; t.hasCoverage {
		for i := range steps {
			if steps[i].title == "Interview" {
				steps[i].detail = fmt.Sprintf("%d/%d", t.coverage.Settled, len(domain.LabCoverageTopics))
			}
		}
	}
	return steps
}

func renderLabStepper(steps []labPageStep, theme styles.Theme, width int) string {
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	style := func(color string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(color)) }
	parts := make([]string, len(steps))
	for i, s := range steps {
		label := s.title
		if s.detail != "" {
			label += " " + s.detail
		}
		switch s.status {
		case "succeeded":
			parts[i] = style(theme.Success()).Render(label + " ✓")
		case "running":
			parts[i] = style(theme.Accent()).Bold(true).Render("● " + label)
		case "blocked":
			parts[i] = style(theme.Warning()).Bold(true).Render("◆ " + label)
		case "failed":
			parts[i] = style(theme.Warning()).Render("✗ " + label)
		default:
			parts[i] = muted.Render(label)
		}
	}
	line := strings.Join(parts, muted.Render(" ─ "))
	if lipgloss.Width(line) > width {
		return truncateStr(line, width)
	}
	return line
}

// renderLabPage draws the entry page: the stepper, the panel for what the
// entry needs now, the decisions log, and the key hints.
func renderLabPage(v labView, theme styles.Theme, width, height int, focused bool) string {
	p := v.page
	e, ok := v.entry(p.entryID)
	if !ok {
		return ""
	}
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent())).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warning()))
	rule := muted.Render(strings.Repeat("─", max(1, width)))
	t := v.talks[e.ID]
	ask, _ := v.ask(e)

	var b strings.Builder
	b.WriteString(accent.Render("⌁ LAB › ") + truncateStr(e.Title(), max(8, width-12)) + "\n")
	b.WriteString(renderLabStepper(labPageSteps(v, e), theme, width) + "\n")
	b.WriteString(rule + "\n")

	var hints string
	switch {
	case p.card != nil:
		if p.card.Title() != "" {
			line := p.card.Title()
			if ask == labAskQuestion && t.hasCoverage && !p.revising {
				line += muted.Render(fmt.Sprintf("   coverage %d/%d", t.coverage.Settled, len(domain.LabCoverageTopics)))
				if len(t.coverage.Open) > 0 {
					line += muted.Render(" · open: " + strings.Join(t.coverage.Open, ", "))
				}
			}
			b.WriteString(truncateStr(accent.Render(line), width) + "\n\n")
		}
		p.card.SetWidth(width + 12)
		p.card.SetTheme(theme)
		b.WriteString(p.card.View() + "\n")
		if p.focus == labFocusPanel {
			hints = "Tab note or decisions  ·  . actions"
		}
	case p.replyOn:
		b.WriteString(accent.Render("REPLY TO THE AGENT") + "\n")
		b.WriteString("The agent stopped without asking through a question card. Its last output:\n\n")
		b.WriteString(muted.Render(labTail(t.session.Output, labPageOutputRows, width)) + "\n\n")
		p.reply.SetWidth(max(20, width-2))
		b.WriteString(p.reply.View() + "\n")
		hints = "Ctrl+S send  ·  Enter new line  ·  Esc back to the list"
	case p.awaiting:
		b.WriteString(muted.Render("Sent. Waiting for the agent to continue…") + "\n")
		hints = "Esc back to the list  ·  . actions"
	case ask == labAskPermission:
		b.WriteString(warning.Render("PERMISSION REQUEST") + "\n")
		b.WriteString("The agent is waiting for a permission decision:\n\n")
		b.WriteString(muted.Render(labTail(t.session.Output, labPageOutputRows, width)) + "\n\n")
		hints = "y allow  ·  n deny  ·  Esc back to the list  ·  . actions"
	default:
		b.WriteString(accent.Render("NEXT") + "  " + labNextAction(v, e) + "\n")
		if ask == labAskReview && len(t.session.Problems) > 0 {
			b.WriteString("\n" + warning.Render("The agent could not fix these problems in the draft:") + "\n")
			for _, problem := range t.session.Problems {
				b.WriteString(wrapText("• "+problem, width) + "\n")
			}
		}
		if run, ok := v.latestRun(e); ok && run.live() && strings.EqualFold(run.workflow.Status, domain.WorkflowRunning) {
			b.WriteString(muted.Render(firstNonEmptyString(labSessionSummary(run), "The agent is working.")) + "\n")
		}
		if body := e.Body(); body != "" && len(t.questions) == 0 {
			b.WriteString("\n" + muted.Render(wrapText(body, width)) + "\n")
		}
		hints = "↵ " + labNextAction(v, e) + "  ·  Tab decisions  ·  . actions  ·  Esc back to the list"
	}

	decisions := labDecisions(t)
	if len(decisions) > 0 {
		b.WriteString(rule + "\n")
		b.WriteString(accent.Render(fmt.Sprintf("DECISIONS (%d)", len(decisions))) + "\n")
		for i, r := range decisions {
			cursor := "  "
			if p.focus == labFocusLog && i == p.logCursor && focused {
				cursor = "▸ "
			}
			line := fmt.Sprintf("%sQ%d  %s  → %s", cursor, r.Number, r.Question.Question, labAnswerText(r))
			line = truncateStr(line, width)
			if p.focus == labFocusLog && i == p.logCursor && focused {
				line = accent.Render(line)
			}
			b.WriteString(line + "\n")
		}
		if p.focus == labFocusLog {
			hints = "↑↓ choose  ·  ↵ change the answer  ·  Tab back  ·  . actions  ·  Esc back"
		}
	}

	content := strings.TrimRight(b.String(), "\n")
	if hints != "" {
		// The hints stay at the bottom; the log scrolls off first.
		lines := strings.Split(content, "\n")
		if height > 0 && len(lines) > height-2 {
			lines = lines[:max(1, height-2)]
		}
		content = strings.Join(lines, "\n") + "\n\n" + muted.Render(truncateStr(hints, width))
	}
	return content
}

// labSessionSummary is the run's own description of what it is doing.
func labSessionSummary(run labRun) string {
	for _, s := range run.workflow.Steps {
		if strings.EqualFold(s.Title, run.workflow.CurrentStep) && s.Summary != "" {
			return s.Summary
		}
	}
	return ""
}

// labTail is the last rows of text, each cut to width.
func labTail(text string, rows, width int) string {
	lines := strings.Split(strings.TrimRight(text, "\n "), "\n")
	if len(lines) > rows {
		lines = lines[len(lines)-rows:]
	}
	for i, line := range lines {
		lines[i] = truncateStr(line, width)
	}
	out := strings.Join(lines, "\n")
	if strings.TrimSpace(out) == "" {
		return "(no output captured)"
	}
	return out
}
