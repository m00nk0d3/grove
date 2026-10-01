package modal

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// LabCaptureSubmittedMsg carries a captured or edited Lab entry. ID is empty
// for a new entry.
type LabCaptureSubmittedMsg struct {
	ID   string
	Kind domain.LabKind
	Text string
}

// Layout of the capture modal around its editor, in rows and columns.
const (
	labCaptureChromeRows = 9 // box border, title, kind row, spacing, error, hints
	labCaptureMinRows    = 3
	labCaptureMaxRows    = 16
	labCaptureBoxColumns = 6 // box border and padding, and the editor prompt
)

// LabCaptureModal captures a new Lab entry or edits a draft. The first line of
// the text is the entry's title.
type LabCaptureModal struct {
	id     string
	kind   domain.LabKind
	editor textarea.Model
	err    string
	theme  *styles.Theme
}

// NewLabCaptureModal opens the editor for a new entry of the given kind.
func NewLabCaptureModal(kind domain.LabKind) *LabCaptureModal {
	if !kind.Valid() {
		kind = domain.LabKindIdea
	}
	return newLabCaptureModal("", kind, "")
}

// NewLabEditModal opens the editor on an existing entry.
func NewLabEditModal(e domain.LabEntry) *LabCaptureModal {
	return newLabCaptureModal(e.ID, e.Kind, e.Text)
}

func newLabCaptureModal(id string, kind domain.LabKind, text string) *LabCaptureModal {
	editor := textarea.New()
	editor.Placeholder = "First line is the title. Add details below."
	editor.ShowLineNumbers = false
	editor.CharLimit = 0
	editor.Prompt = "│ "
	// Grove delivers only key events to a modal, so a blinking cursor would
	// never receive its blink messages.
	editor.Cursor.SetMode(cursor.CursorStatic)
	editor.SetWidth(60)
	editor.SetHeight(8)
	editor.SetValue(text)
	editor.Focus()
	return &LabCaptureModal{id: id, kind: kind, editor: editor}
}

// Init satisfies tea.Model.
func (m *LabCaptureModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabCaptureModal) Title() string {
	if m.id != "" {
		return "EDIT CAPTURE"
	}
	return "NEW LAB ITEM"
}

// SetWidth sizes the editor to the terminal width.
func (m *LabCaptureModal) SetWidth(w int) {
	m.editor.SetWidth(max(20, w-labCaptureBoxColumns-4))
}

// SetHeight sizes the editor to the terminal height.
func (m *LabCaptureModal) SetHeight(h int) {
	m.editor.SetHeight(min(labCaptureMaxRows, max(labCaptureMinRows, h-labCaptureChromeRows-4)))
}

// SetTheme applies the active theme.
func (m *LabCaptureModal) SetTheme(t styles.Theme) { m.theme = &t }

// Kind returns the selected kind.
func (m *LabCaptureModal) Kind() domain.LabKind { return m.kind }

// Value returns the editor's current text.
func (m *LabCaptureModal) Value() string { return m.editor.Value() }

// Update handles editing, kind toggling, saving, and cancelling.
func (m *LabCaptureModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case "tab", "shift+tab":
		if m.kind == domain.LabKindIdea {
			m.kind = domain.LabKindBug
		} else {
			m.kind = domain.LabKindIdea
		}
		return m, nil
	case "ctrl+s":
		text := domain.NormalizeLabText(m.editor.Value())
		if text == "" {
			m.err = "Write something first — the first line becomes the title."
			return m, nil
		}
		submitted := LabCaptureSubmittedMsg{ID: m.id, Kind: m.kind, Text: text}
		return m, func() tea.Msg { return submitted }
	}
	m.err = ""
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

// View renders the kind selector, the editor, and the key hints.
func (m *LabCaptureModal) View() string {
	accent := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle()
	warning := lipgloss.NewStyle()
	if m.theme != nil {
		accent = accent.Foreground(lipgloss.Color(m.theme.Accent()))
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
		warning = warning.Foreground(lipgloss.Color(m.theme.Warning()))
	}
	option := func(kind domain.LabKind) string {
		label := "IDEA  →  GRILL"
		if kind == domain.LabKindBug {
			label = "BUG  →  REPORT"
		}
		if kind == m.kind {
			return accent.Render("● " + label)
		}
		return muted.Render("○ " + label)
	}

	var b strings.Builder
	b.WriteString("What are you bringing to the Lab?\n")
	b.WriteString(option(domain.LabKindIdea) + "     " + option(domain.LabKindBug))
	b.WriteString("\n\n")
	b.WriteString(m.editor.View())
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(warning.Render(m.err))
	}
	b.WriteString("\n")
	b.WriteString(muted.Render("Tab choose  ·  Ctrl+S save to Inbox  ·  Enter new line  ·  Esc cancel"))
	return b.String()
}
