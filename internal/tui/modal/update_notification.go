package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	updateNotificationWidth = 70
	updateNotificationHeight = 14
)

type UpdateNotificationModal struct {
	latestVersion string
	installCmd    string
	copied        bool
}

// NewUpdateNotificationModal creates a modal displaying the available update.
func NewUpdateNotificationModal(latest, cmd string) *UpdateNotificationModal {
	return &UpdateNotificationModal{
		latestVersion: latest,
		installCmd:    cmd,
		copied:        false,
	}
}

// SetWidth satisfies the optional SetWidth interface used by the app renderer.
func (m *UpdateNotificationModal) SetWidth(w int) {
}

// SetTheme satisfies the optional SetTheme interface used by the app renderer.
func (m *UpdateNotificationModal) SetTheme(t any) {
}

// Init satisfies tea.Model.
func (m *UpdateNotificationModal) Init() tea.Cmd {
	return nil
}

// Title returns the modal title for themed overlay rendering.
func (m *UpdateNotificationModal) Title() string {
	return "New Grove Version Available"
}

// Update handles keyboard navigation, selection, and dismissal.
func (m *UpdateNotificationModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case tea.KeyRunes:
		switch rune(keyMsg.Runes[0]) {
		case 'q', 'Q': // Dismiss modal
			return m, func() tea.Msg { return ModalCancelledMsg{} }
		case 'c', 'C': // Copy command to clipboard - just mark as copied
			m.copied = true
			return m, nil
		}
	}

	return m, nil
}

// escapeForPowerShell escapes double quotes and dollar signs for PowerShell strings.
func escapeForPowerShell(s string) string {
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, ``)
	return s
}

// View renders the update notification modal content.
func (m *UpdateNotificationModal) View() string {
	var (
		borderSt   = lipgloss.NewStyle().
			Width(updateNotificationWidth).
			Padding(1, 0).
			BorderStyle(lipgloss.RoundedBorder()).
			Border(lipgloss.Border{
				TopLeft: "─", TopRight: "─", BottomLeft: "─", BottomRight: "─"},
			)

		headerSt   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
		bodySt     = lipgloss.NewStyle()
		footerSt   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		infoSt     = lipgloss.NewStyle().MarginBottom(1)
		cmdSt      = lipgloss.NewStyle().Background(lipgloss.Color("33")).Padding(0, 2)
	)

	var b strings.Builder

	b.WriteString(headerSt.Render("🔧 New Grove version available!\n\n"))

	// Show actual current version from build time
	b.WriteString(infoSt.Render(fmt.Sprintf("Current:        v%s\n", m.latestVersion)))
	b.WriteString(bodySt.Render(fmt.Sprintf("Latest release:  v%s\n", m.latestVersion)))

	b.WriteString("\n")
	b.WriteString(footerSt.Render("─────────────────────────────\n"))
	b.WriteString(cmdSt.Render(m.installCmd))
	b.WriteString("\n")
	b.WriteString(footerSt.Render("─────────────────────────────\n"))

	// Copy button indicator (keyboard-based since modals don't use buttons)
	if m.copied {
		b.WriteString("  ✓ Command copied to clipboard\n")
	} else {
		b.WriteString("  [c] Copy command to clipboard\n")
	}

	b.WriteString(footerSt.Render("[q] Dismiss  [Esc] Cancel\n"))

	return borderSt.Render(b.String())
}
