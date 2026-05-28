package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

const changelogMaxLines = 15

// UpdateModal shows the user that a new version is available and offers to self-update.
type UpdateModal struct {
	current        string
	latest         string
	changelog      string
	htmlURL        string
	activeSessions int
	theme          *styles.Theme
}

// NewUpdateModal creates a new UpdateModal with the given version strings, release
// body (changelog), release URL, and active session count.
func NewUpdateModal(current, latest, changelog, htmlURL string, activeSessions int) *UpdateModal {
	return &UpdateModal{
		current:        current,
		latest:         latest,
		changelog:      changelog,
		htmlURL:        htmlURL,
		activeSessions: activeSessions,
	}
}

// Init satisfies tea.Model.
func (m *UpdateModal) Init() tea.Cmd { return nil }

// Title returns the modal title for themed overlay rendering.
func (m *UpdateModal) Title() string { return "✨  Update Available" }

// Update handles y/n/Esc input and emits UpdateConfirmedMsg or ModalCancelledMsg.
func (m *UpdateModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "y", "Y":
		return m, func() tea.Msg { return UpdateConfirmedMsg{} }
	case "n", "N", "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}
	return m, nil
}

// View renders the update notification content.
func (m *UpdateModal) View() string {
	var b strings.Builder

	// Accent style for separators and key hints; warning style for active-sessions alert.
	var accentSt, warnSt lipgloss.Style
	if m.theme != nil {
		accentSt = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent()))
		warnSt = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Warning()))
	}

	styled := func(st lipgloss.Style, s string) string {
		if m.theme == nil {
			return s
		}
		return st.Render(s)
	}

	b.WriteString("🚀 A shiny new version of grove just dropped!\n\n")
	b.WriteString(fmt.Sprintf("  Running:   %s\n", m.current))
	b.WriteString(fmt.Sprintf("  Available: %s\n", m.latest))

	if m.changelog != "" {
		b.WriteString("\n" + styled(accentSt, "── WHAT'S NEW ──────────────────────────────────────") + "\n")
		lines := strings.Split(strings.TrimSpace(m.changelog), "\n")
		truncated := false
		if len(lines) > changelogMaxLines {
			lines = lines[:changelogMaxLines]
			truncated = true
		}
		for _, line := range lines {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		if truncated && m.htmlURL != "" {
			b.WriteString(fmt.Sprintf("  ↓ full changelog at %s\n", m.htmlURL))
		}
		b.WriteString(styled(accentSt, "─────────────────────────────────────────────────────") + "\n")
	}

	if m.activeSessions > 0 {
		b.WriteString(fmt.Sprintf(
			"\n%s  %d active session(s) vibing right now — they'll be fine,\n   but give grove a restart after the update.\n",
			styled(warnSt, "⚠"), m.activeSessions,
		))
	}

	b.WriteString("\n  " + styled(accentSt, "[y] Hell yeah, update!    [n] Nah, I fear change") + "\n")
	return b.String()
}

// SetTheme injects the current visual theme for styled rendering.
func (m *UpdateModal) SetTheme(t styles.Theme) { m.theme = &t }
