package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// CloseIssueReason is one selectable reason for closing an issue. Values match
// the reasons `gh issue close --reason` accepts.
type CloseIssueReason struct {
	Label string
	Value string
}

// CloseIssueReasons lists the close reasons in the order shown by the modal.
var CloseIssueReasons = []CloseIssueReason{
	{Label: "Completed", Value: "completed"},
	{Label: "Not planned", Value: "not planned"},
	{Label: "Duplicate", Value: "duplicate"},
}

// IssueCloseModal is a confirmation dialog for closing a GitHub issue. It
// offers a reason list (the gh-supported close reasons) and an optional
// closing comment. The caller decides whether closure is allowed: an already
// closed issue or a viewer without collaborator permission renders a message
// with confirmation disabled.
type IssueCloseModal struct {
	issue     domain.Issue
	canClose  bool
	reasonIdx int
	comment   textinput.Model
	inComment bool
	err       string
	theme     *styles.Theme
}

// NewIssueCloseModal opens the dialog for issue. canClose is false when the
// viewer is not a collaborator; closure of an already closed issue is
// disabled from the issue's own state.
func NewIssueCloseModal(issue domain.Issue, canClose bool) *IssueCloseModal {
	ti := textinput.New()
	ti.Prompt = "Comment: "
	ti.Placeholder = "optional closing comment"
	ti.CharLimit = 500
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Width = 60
	return &IssueCloseModal{issue: issue, canClose: canClose, comment: ti}
}

// Init satisfies tea.Model.
func (m *IssueCloseModal) Init() tea.Cmd { return nil }

// Title returns the modal title for themed overlay rendering.
func (m *IssueCloseModal) Title() string { return "Close Issue" }

// Issue returns the issue the dialog was opened for.
func (m *IssueCloseModal) Issue() domain.Issue { return m.issue }

// SelectedReason returns the gh close-reason value currently selected.
func (m *IssueCloseModal) SelectedReason() string { return CloseIssueReasons[m.reasonIdx].Value }

// Comment returns the trimmed closing comment.
func (m *IssueCloseModal) Comment() string { return strings.TrimSpace(m.comment.Value()) }

// CanConfirm reports whether confirmation is allowed: the viewer may close
// and the issue is still open.
func (m *IssueCloseModal) CanConfirm() bool { return m.canClose && !m.issue.IsClosed() }

// SetError shows an inline error, such as a rate-limit or permission failure
// from a close attempt.
func (m *IssueCloseModal) SetError(err string) { m.err = err }

// SetWidth sizes the comment input to the terminal width.
func (m *IssueCloseModal) SetWidth(w int) {
	m.comment.Width = max(20, min(72, w-16))
}

// SetTheme applies the active theme.
func (m *IssueCloseModal) SetTheme(t styles.Theme) { m.theme = &t }

// Update handles reason navigation, comment editing, confirming, and
// cancelling.
func (m *IssueCloseModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		if m.inComment {
			var cmd tea.Cmd
			m.comment, cmd = m.comment.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Esc always leaves the comment first, then cancels.
	if keyMsg.Type == tea.KeyEsc {
		if m.inComment {
			m.inComment = false
			m.comment.Blur()
			return m, nil
		}
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	if keyMsg.String() == "tab" || keyMsg.String() == "shift+tab" {
		if m.CanConfirm() {
			if m.inComment {
				m.inComment = false
				m.comment.Blur()
			} else {
				m.inComment = true
				_ = m.comment.Focus()
			}
		}
		return m, nil
	}

	if m.inComment {
		if keyMsg.Type == tea.KeyEnter {
			return m.confirm()
		}
		var cmd tea.Cmd
		m.comment, cmd = m.comment.Update(msg)
		return m, cmd
	}

	switch keyMsg.String() {
	case "up", "k":
		if m.reasonIdx > 0 {
			m.reasonIdx--
		}
	case "down", "j":
		if m.reasonIdx < len(CloseIssueReasons)-1 {
			m.reasonIdx++
		}
	case "y", "Y", "enter":
		return m.confirm()
	case "n", "N":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}
	return m, nil
}

func (m *IssueCloseModal) confirm() (tea.Model, tea.Cmd) {
	if !m.CanConfirm() {
		return m, nil
	}
	msg := IssueCloseConfirmedMsg{Number: m.issue.Number, Reason: m.SelectedReason(), Comment: m.Comment()}
	return m, func() tea.Msg { return msg }
}

func (m *IssueCloseModal) styles() (accent, muted, warning lipgloss.Style) {
	accent, muted, warning = lipgloss.NewStyle().Bold(true), lipgloss.NewStyle(), lipgloss.NewStyle()
	if m.theme != nil {
		accent = accent.Foreground(lipgloss.Color(m.theme.Accent()))
		muted = muted.Foreground(lipgloss.Color(m.theme.Muted()))
		warning = warning.Foreground(lipgloss.Color(m.theme.Warning()))
	}
	return accent, muted, warning
}

// View renders the issue, the reason list, the comment input, and the key
// hints. Disabled states explain why confirmation is unavailable.
func (m *IssueCloseModal) View() string {
	accent, muted, warning := m.styles()
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Close issue #%d %s\n\n", m.issue.Number, m.issue.Title))
	switch {
	case m.issue.IsClosed():
		b.WriteString("This issue is already closed.\n")
	case !m.canClose:
		b.WriteString("You do not have permission to close issues in this repository.\n")
	default:
		b.WriteString("Reason:\n")
		for i, r := range CloseIssueReasons {
			marker := "  "
			if i == m.reasonIdx {
				marker = "> "
				if m.inComment {
					marker = "● "
				}
			}
			line := fmt.Sprintf("%s%s\n", marker, r.Label)
			if i == m.reasonIdx && !m.inComment {
				line = accent.Render(line)
			}
			b.WriteString(line)
		}
		b.WriteString("\n")
		b.WriteString(m.comment.View())
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString(warning.Render(m.err))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(muted.Render(m.hints()))
	return b.String()
}

func (m *IssueCloseModal) hints() string {
	if !m.CanConfirm() {
		return "Esc close"
	}
	if m.inComment {
		return "Enter close  ·  Tab back to reasons  ·  Esc back"
	}
	return "↑↓/j/k reason  ·  Tab comment  ·  y/Enter close  ·  n/Esc cancel"
}
