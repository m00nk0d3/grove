package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// LabAnswerSubmittedMsg carries the user's answer to a question card.
type LabAnswerSubmittedMsg struct {
	EntryID string
	Number  int
	Choices []int
	Text    string
	// Revise replaces an answer already given instead of answering anew.
	Revise bool
}

// LabRequestSubmittedMsg carries a message for a session's agent: a reply, a
// change request, or a permission decision.
type LabRequestSubmittedMsg struct {
	EntryID string
	Kind    domain.LabRequestKind
	Text    string
	Allow   bool
}

// LabQuestionModal shows one question card and takes its answer. Options are
// chosen with the arrow keys or their number; Tab moves to the note, which
// takes every key as text until Esc; Enter submits from either.
type LabQuestionModal struct {
	entryID  string
	question domain.LabQuestion
	number   int
	// position is shown in the title when several cards wait, such as "1 of 3".
	position string
	cursor   int
	picked   map[int]bool
	note     textinput.Model
	inNote   bool
	// revise marks a card reopened to change an answer already given.
	revise bool
	// blurred hides the card's cursor while another part of the page has
	// the keyboard.
	blurred bool
	err     string
	width   int
	theme   *styles.Theme
}

// NewLabRevisionCard reopens question n to change its answer, starting from
// the answer given.
func NewLabRevisionCard(entryID string, n int, q domain.LabQuestion, a domain.LabAnswer) *LabQuestionModal {
	m := NewLabQuestionModal(entryID, n, q, "")
	m.revise = true
	m.picked = map[int]bool{}
	for _, i := range a.Choices {
		m.picked[i] = true
	}
	if len(a.Choices) > 0 {
		m.cursor = a.Choices[0]
	}
	m.note.SetValue(a.Text)
	m.note.CursorEnd()
	return m
}

// InNote reports whether the note has the keyboard.
func (m *LabQuestionModal) InNote() bool { return m.inNote }

// HasOptions reports whether the card is answered by choosing options.
func (m *LabQuestionModal) HasOptions() bool { return len(m.question.Options) > 0 }

// FocusOptions gives the keyboard to the options, or to the answer of a text
// card.
func (m *LabQuestionModal) FocusOptions() {
	m.blurred = false
	if m.HasOptions() {
		m.leaveNote()
	} else {
		m.focusNote()
	}
}

// FocusNote gives the keyboard to the note.
func (m *LabQuestionModal) FocusNote() {
	m.blurred = false
	m.focusNote()
}

// Blur takes the keyboard away from the card.
func (m *LabQuestionModal) Blur() {
	m.blurred = true
	m.leaveNote()
}

// NewLabQuestionModal opens the card of question n. The recommended answer is
// preselected, so Enter accepts it.
func NewLabQuestionModal(entryID string, n int, q domain.LabQuestion, position string) *LabQuestionModal {
	note := textinput.New()
	note.Prompt = "Note: "
	note.CharLimit = 0
	note.Cursor.SetMode(cursor.CursorStatic)
	m := &LabQuestionModal{entryID: entryID, question: q, number: n, position: position, picked: map[int]bool{}, note: note, width: 72}
	switch q.Kind {
	case domain.LabQuestionText:
		m.note.Prompt = "Answer: "
		m.note.SetValue(q.RecommendedText)
		m.note.CursorEnd()
		m.focusNote()
	case domain.LabQuestionMulti:
		for _, i := range q.Recommended {
			m.picked[i] = true
		}
		if len(q.Recommended) > 0 {
			m.cursor = q.Recommended[0]
		}
	default:
		if len(q.Recommended) > 0 {
			m.cursor = q.Recommended[0]
		}
	}
	return m
}

// EntryID returns the entry the question belongs to.
func (m *LabQuestionModal) EntryID() string { return m.entryID }

// Number returns the question's number.
func (m *LabQuestionModal) Number() int { return m.number }

// Init satisfies tea.Model.
func (m *LabQuestionModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabQuestionModal) Title() string {
	title := fmt.Sprintf("QUESTION %d", m.number)
	if m.revise {
		title = fmt.Sprintf("CHANGE ANSWER %d", m.number)
	}
	if m.position != "" {
		title += " · " + m.position
	}
	return title
}

// SetWidth sizes the card to the terminal width.
func (m *LabQuestionModal) SetWidth(w int) {
	m.width = max(40, min(96, w-12))
	m.note.Width = m.width - len(m.note.Prompt) - 2
}

// SetTheme applies the active theme.
func (m *LabQuestionModal) SetTheme(t styles.Theme) { m.theme = &t }

func (m *LabQuestionModal) focusNote() {
	m.inNote = true
	m.note.Focus()
}

func (m *LabQuestionModal) leaveNote() {
	m.inNote = false
	m.note.Blur()
}

// Update handles choosing, typing, submitting, and cancelling.
func (m *LabQuestionModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.err = ""
	options := len(m.question.Options)
	switch key.String() {
	case "enter":
		return m, m.submit()
	case "esc":
		if m.inNote && options > 0 {
			m.leaveNote()
			return m, nil
		}
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case "tab", "shift+tab":
		if options == 0 {
			return m, nil
		}
		if m.inNote {
			m.leaveNote()
		} else {
			m.focusNote()
		}
		return m, nil
	}
	if m.inNote {
		var cmd tea.Cmd
		m.note, cmd = m.note.Update(msg)
		return m, cmd
	}
	switch s := key.String(); s {
	case "up", "k":
		m.cursor = (m.cursor - 1 + options) % options
	case "down", "j":
		m.cursor = (m.cursor + 1) % options
	case " ", "x":
		if m.question.Kind == domain.LabQuestionMulti {
			m.picked[m.cursor] = !m.picked[m.cursor]
		}
	default:
		if len(s) == 1 && s[0] >= '1' && int(s[0]-'1') < options {
			m.cursor = int(s[0] - '1')
			if m.question.Kind == domain.LabQuestionMulti {
				m.picked[m.cursor] = !m.picked[m.cursor]
			}
		}
	}
	return m, nil
}

// submit returns the answer, or nil with an error shown when it is empty.
func (m *LabQuestionModal) submit() tea.Cmd {
	answer := LabAnswerSubmittedMsg{EntryID: m.entryID, Number: m.number, Text: strings.TrimSpace(m.note.Value()), Revise: m.revise}
	switch m.question.Kind {
	case domain.LabQuestionText:
		if answer.Text == "" {
			m.err = "Type an answer first."
			return nil
		}
	case domain.LabQuestionMulti:
		for i := range m.question.Options {
			if m.picked[i] {
				answer.Choices = append(answer.Choices, i)
			}
		}
		if len(answer.Choices) == 0 && answer.Text == "" {
			m.err = "Pick at least one option, or write a note."
			return nil
		}
	default:
		answer.Choices = []int{m.cursor}
	}
	return func() tea.Msg { return answer }
}

func (m *LabQuestionModal) styles() (accent, muted, warning lipgloss.Style) {
	accent, muted, warning = lipgloss.NewStyle().Bold(true), lipgloss.NewStyle(), lipgloss.NewStyle()
	if m.theme != nil {
		accent = accent.Foreground(lipgloss.Color(m.theme.Accent()))
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
		warning = warning.Foreground(lipgloss.Color(m.theme.Warning()))
	}
	return accent, muted, warning
}

// View renders the card.
func (m *LabQuestionModal) View() string {
	accent, muted, warning := m.styles()
	wrap := lipgloss.NewStyle().Width(m.width)
	var b strings.Builder
	b.WriteString(wrap.Render(accent.Render(m.question.Question)))
	b.WriteString("\n")
	b.WriteString(wrap.Render(muted.Render(m.question.Context)))
	b.WriteString("\n\n")
	recommended := map[int]bool{}
	for _, i := range m.question.Recommended {
		recommended[i] = true
	}
	for i, option := range m.question.Options {
		marker := "  "
		if i == m.cursor && !m.inNote && !m.blurred {
			marker = "▸ "
		}
		box := ""
		if m.question.Kind == domain.LabQuestionMulti {
			box = "[ ] "
			if m.picked[i] {
				box = "[x] "
			}
		}
		line := fmt.Sprintf("%s%s%d. %s", marker, box, i+1, option)
		if recommended[i] {
			line += muted.Render("  (recommended)")
		}
		if i == m.cursor && !m.inNote && !m.blurred {
			line = accent.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if len(m.question.Options) > 0 {
		b.WriteString("\n")
	}
	b.WriteString(wrap.Render(muted.Render("Why: " + m.question.Why)))
	b.WriteString("\n\n")
	b.WriteString(m.note.View())
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(warning.Render(m.err))
	}
	b.WriteString("\n")
	b.WriteString(muted.Render(m.hints()))
	return b.String()
}

func (m *LabQuestionModal) hints() string {
	switch {
	case m.question.Kind == domain.LabQuestionText:
		return "Enter send  ·  Esc later"
	case m.inNote:
		return "Enter send  ·  Esc back to the options"
	case m.question.Kind == domain.LabQuestionMulti:
		return "↑↓ move  ·  Space or 1–4 toggle  ·  Tab add a note  ·  Enter send  ·  Esc later"
	default:
		return "↑↓ or 1–4 choose  ·  Tab add a note  ·  Enter send  ·  Esc later"
	}
}

// LabMessageModal writes a free-text message to a session's agent: a reply to
// a turn that ended without a question card, or a change request on a draft.
// The agent's recent output, when there is any, is shown above the editor.
type LabMessageModal struct {
	entryID string
	kind    domain.LabRequestKind
	title   string
	intro   string
	output  string
	editor  textarea.Model
	err     string
	width   int
	theme   *styles.Theme
}

// Rows the agent's output may take in the message modal.
const labMessageOutputRows = 14

// NewLabReplyModal replies to an agent that ended its turn without a question
// card; output is what it last wrote.
func NewLabReplyModal(entryID, output string) *LabMessageModal {
	return newLabMessageModal(entryID, domain.LabRequestReply, "REPLY TO THE AGENT",
		"The agent stopped without asking through a question card. Its last output:", output, "")
}

// NewLabChangeModal asks the agent for changes to a draft at path.
func NewLabChangeModal(entryID, path string) *LabMessageModal {
	return newLabMessageModal(entryID, domain.LabRequestChange, "REQUEST CHANGES",
		fmt.Sprintf("What should the agent change? The agent revises the drafts in place and %s returns to review.", path), "", path+": ")
}

func newLabMessageModal(entryID string, kind domain.LabRequestKind, title, intro, output, text string) *LabMessageModal {
	editor := textarea.New()
	editor.ShowLineNumbers = false
	editor.CharLimit = 0
	editor.Prompt = "│ "
	editor.Cursor.SetMode(cursor.CursorStatic)
	editor.SetWidth(60)
	editor.SetHeight(5)
	editor.SetValue(text)
	editor.Focus()
	return &LabMessageModal{entryID: entryID, kind: kind, title: title, intro: intro, output: output, editor: editor, width: 72}
}

// Init satisfies tea.Model.
func (m *LabMessageModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabMessageModal) Title() string { return m.title }

// SetWidth sizes the modal to the terminal width.
func (m *LabMessageModal) SetWidth(w int) {
	m.width = max(40, min(96, w-12))
	m.editor.SetWidth(m.width - 2)
}

// SetTheme applies the active theme.
func (m *LabMessageModal) SetTheme(t styles.Theme) { m.theme = &t }

// Update handles typing, sending, and cancelling.
func (m *LabMessageModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case "ctrl+s":
		text := strings.TrimSpace(m.editor.Value())
		if text == "" {
			m.err = "Write a message first."
			return m, nil
		}
		sent := LabRequestSubmittedMsg{EntryID: m.entryID, Kind: m.kind, Text: text}
		return m, func() tea.Msg { return sent }
	}
	m.err = ""
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// View renders the output, the editor, and the key hints.
func (m *LabMessageModal) View() string {
	muted, warning := lipgloss.NewStyle(), lipgloss.NewStyle()
	if m.theme != nil {
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
		warning = warning.Foreground(lipgloss.Color(m.theme.Warning()))
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Width(m.width).Render(m.intro))
	b.WriteString("\n")
	if out := tailLines(m.output, labMessageOutputRows); out != "" {
		b.WriteString("\n")
		b.WriteString(muted.Render(truncateLines(out, m.width)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.editor.View())
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(warning.Render(m.err))
	}
	b.WriteString("\n")
	b.WriteString(muted.Render("Ctrl+S send  ·  Enter new line  ·  Esc cancel"))
	return b.String()
}

// LabPermissionModal answers a permission prompt the agent is waiting on.
type LabPermissionModal struct {
	entryID string
	output  string
	width   int
	theme   *styles.Theme
}

// NewLabPermissionModal shows what the agent is asking permission for.
func NewLabPermissionModal(entryID, output string) *LabPermissionModal {
	return &LabPermissionModal{entryID: entryID, output: output, width: 72}
}

// Init satisfies tea.Model.
func (m *LabPermissionModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabPermissionModal) Title() string { return "PERMISSION REQUEST" }

// SetWidth sizes the modal to the terminal width.
func (m *LabPermissionModal) SetWidth(w int) { m.width = max(40, min(96, w-12)) }

// SetTheme applies the active theme.
func (m *LabPermissionModal) SetTheme(t styles.Theme) { m.theme = &t }

// Update handles allow, deny, and cancel.
func (m *LabPermissionModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	decide := func(allow bool) tea.Cmd {
		sent := LabRequestSubmittedMsg{EntryID: m.entryID, Kind: domain.LabRequestPermission, Allow: allow}
		return func() tea.Msg { return sent }
	}
	switch key.String() {
	case "y", "Y":
		return m, decide(true)
	case "n", "N":
		return m, decide(false)
	case "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}
	return m, nil
}

// View renders the agent's output and the choices.
func (m *LabPermissionModal) View() string {
	muted := lipgloss.NewStyle()
	if m.theme != nil {
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
	}
	var b strings.Builder
	b.WriteString("The agent is waiting for a permission decision:\n\n")
	if out := tailLines(m.output, labMessageOutputRows); out != "" {
		b.WriteString(muted.Render(truncateLines(out, m.width)))
	} else {
		b.WriteString(muted.Render("(no output captured)"))
	}
	b.WriteString("\n\n[y] allow  [n] deny  [Esc] decide later")
	return b.String()
}

// tailLines returns the last n non-trailing-blank lines of s.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n "), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n ")
}

// truncateLines cuts every line of s to width display columns.
func truncateLines(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			runes := []rune(line)
			for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
				runes = runes[:len(runes)-1]
			}
			lines[i] = string(runes) + "…"
		}
	}
	return strings.Join(lines, "\n")
}
