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

// LabQuestionCard shows one question card on the entry page and takes its
// answer. Options are
// chosen with the arrow keys or their number; Tab moves to the note, which
// takes every key as text until Esc; Enter submits from either.
type LabQuestionCard struct {
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
func NewLabRevisionCard(entryID string, n int, q domain.LabQuestion, a domain.LabAnswer) *LabQuestionCard {
	m := NewLabQuestionCard(entryID, n, q, "")
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
func (m *LabQuestionCard) InNote() bool { return m.inNote }

// HasOptions reports whether the card is answered by choosing options.
func (m *LabQuestionCard) HasOptions() bool { return len(m.question.Options) > 0 }

// FocusOptions gives the keyboard to the options, or to the answer of a text
// card.
func (m *LabQuestionCard) FocusOptions() {
	m.blurred = false
	if m.HasOptions() {
		m.leaveNote()
	} else {
		m.focusNote()
	}
}

// FocusNote gives the keyboard to the note.
func (m *LabQuestionCard) FocusNote() {
	m.blurred = false
	m.focusNote()
}

// Blur takes the keyboard away from the card.
func (m *LabQuestionCard) Blur() {
	m.blurred = true
	m.leaveNote()
}

// NewLabQuestionCard opens the card of question n. The recommended answer is
// preselected, so Enter accepts it.
func NewLabQuestionCard(entryID string, n int, q domain.LabQuestion, position string) *LabQuestionCard {
	note := textinput.New()
	note.Prompt = "Note: "
	note.CharLimit = 0
	note.Cursor.SetMode(cursor.CursorStatic)
	m := &LabQuestionCard{entryID: entryID, question: q, number: n, position: position, picked: map[int]bool{}, note: note, width: 72}
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
func (m *LabQuestionCard) EntryID() string { return m.entryID }

// Number returns the question's number.
func (m *LabQuestionCard) Number() int { return m.number }

// Init satisfies tea.Model.
func (m *LabQuestionCard) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabQuestionCard) Title() string {
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
func (m *LabQuestionCard) SetWidth(w int) {
	m.width = max(40, min(96, w-12))
	m.note.Width = m.width - len(m.note.Prompt) - 2
}

// SetTheme applies the active theme.
func (m *LabQuestionCard) SetTheme(t styles.Theme) { m.theme = &t }

func (m *LabQuestionCard) focusNote() {
	m.inNote = true
	m.note.Focus()
}

func (m *LabQuestionCard) leaveNote() {
	m.inNote = false
	m.note.Blur()
}

// Update handles choosing, typing, submitting, and cancelling.
func (m *LabQuestionCard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
func (m *LabQuestionCard) submit() tea.Cmd {
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

func (m *LabQuestionCard) styles() (accent, muted, warning lipgloss.Style) {
	accent, muted, warning = lipgloss.NewStyle().Bold(true), lipgloss.NewStyle(), lipgloss.NewStyle()
	if m.theme != nil {
		accent = accent.Foreground(lipgloss.Color(m.theme.Accent()))
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
		warning = warning.Foreground(lipgloss.Color(m.theme.Warning()))
	}
	return accent, muted, warning
}

// View renders the card.
func (m *LabQuestionCard) View() string {
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

func (m *LabQuestionCard) hints() string {
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

// LabChangeModal writes a change request on a draft for the session's
// agent, which revises the draft in place.
type LabChangeModal struct {
	entryID string
	intro   string
	editor  textarea.Model
	err     string
	width   int
	theme   *styles.Theme
}

// NewLabChangeModal asks the agent for changes to the draft at path.
func NewLabChangeModal(entryID, path string) *LabChangeModal {
	editor := textarea.New()
	editor.ShowLineNumbers = false
	editor.CharLimit = 0
	editor.Prompt = "│ "
	// Grove delivers only key events to a modal, so a blinking cursor would
	// never receive its blink messages.
	editor.Cursor.SetMode(cursor.CursorStatic)
	editor.SetWidth(60)
	editor.SetHeight(5)
	editor.SetValue(path + ": ")
	editor.Focus()
	return &LabChangeModal{
		entryID: entryID,
		intro:   fmt.Sprintf("What should the agent change? It revises the drafts in place and %s returns to review.", path),
		editor:  editor,
		width:   72,
	}
}

// AppendText adds text to the request, such as the ticket it is about.
func (m *LabChangeModal) AppendText(text string) {
	m.editor.SetValue(m.editor.Value() + text)
	m.editor.CursorEnd()
}

// Init satisfies tea.Model.
func (m *LabChangeModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabChangeModal) Title() string { return "REQUEST CHANGES" }

// SetWidth sizes the modal to the terminal width.
func (m *LabChangeModal) SetWidth(w int) {
	m.width = max(40, min(96, w-12))
	m.editor.SetWidth(m.width - 2)
}

// SetTheme applies the active theme.
func (m *LabChangeModal) SetTheme(t styles.Theme) { m.theme = &t }

// Update handles typing, sending, and cancelling.
func (m *LabChangeModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.err = "Write what to change first."
			return m, nil
		}
		sent := LabRequestSubmittedMsg{EntryID: m.entryID, Kind: domain.LabRequestChange, Text: text}
		return m, func() tea.Msg { return sent }
	}
	m.err = ""
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// View renders the request editor and the key hints.
func (m *LabChangeModal) View() string {
	muted, warning := lipgloss.NewStyle(), lipgloss.NewStyle()
	if m.theme != nil {
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
		warning = warning.Foreground(lipgloss.Color(m.theme.Warning()))
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Width(m.width).Render(m.intro))
	b.WriteString("\n\n")
	b.WriteString(m.editor.View())
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(warning.Render(m.err))
	}
	b.WriteString("\n")
	b.WriteString(muted.Render("Ctrl+S send  ·  Enter new line  ·  Esc cancel"))
	return b.String()
}
