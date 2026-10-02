package modal

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	updateNotificationWidth  = 70
	updateNotificationHeight = 14
)

type UpdateNotificationModal struct {
	currentVersion string
	latestVersion  string
	installCmd     string
	copied         bool
	copyErr        error
	copyCommand    func(string) error
}

// NewUpdateNotificationModal creates a modal displaying the available update.
func NewUpdateNotificationModal(current, latest, cmd string) *UpdateNotificationModal {
	return &UpdateNotificationModal{
		currentVersion: current,
		latestVersion:  latest,
		installCmd:     cmd,
		copyCommand:    clipboard.WriteAll,
	}
}

type updateCopyResultMsg struct{ err error }

func (updateCopyResultMsg) modalOwned() {}

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
	if result, ok := msg.(updateCopyResultMsg); ok {
		m.copied = result.err == nil
		m.copyErr = result.err
		return m, nil
	}

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
		case 'c', 'C':
			m.copied = false
			m.copyErr = nil
			return m, func() tea.Msg {
				return updateCopyResultMsg{err: m.copyCommand(m.installCmd)}
			}
		}
	}

	return m, nil
}

func displayVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// View renders the update notification modal content.
func (m *UpdateNotificationModal) View() string {
	var (
		borderSt = lipgloss.NewStyle().
				Width(updateNotificationWidth).
				Padding(1, 0).
				BorderStyle(lipgloss.RoundedBorder()).
				Border(lipgloss.Border{
				TopLeft: "─", TopRight: "─", BottomLeft: "─", BottomRight: "─"},
			)

		headerSt = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
		bodySt   = lipgloss.NewStyle()
		footerSt = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		infoSt   = lipgloss.NewStyle().MarginBottom(1)
		cmdSt    = lipgloss.NewStyle().Background(lipgloss.Color("33")).Padding(0, 2)
	)

	var b strings.Builder

	b.WriteString(headerSt.Render("New Grove version available!\n\n"))

	b.WriteString(infoSt.Render(fmt.Sprintf("Current:         %s\n", displayVersion(m.currentVersion))))
	b.WriteString(bodySt.Render(fmt.Sprintf("Latest release:  %s\n", displayVersion(m.latestVersion))))

	b.WriteString("\n")
	b.WriteString(footerSt.Render("─────────────────────────────\n"))
	b.WriteString(cmdSt.Render(m.installCmd))
	b.WriteString("\n")
	b.WriteString(footerSt.Render("─────────────────────────────\n"))

	// Copy button indicator (keyboard-based since modals don't use buttons)
	if m.copied {
		b.WriteString("  Command copied to clipboard\n")
	} else if m.copyErr != nil {
		b.WriteString(fmt.Sprintf("  Copy failed: %v\n", m.copyErr))
	} else {
		b.WriteString("  [c] Copy command to clipboard\n")
	}

	if strings.Contains(m.installCmd, "install.ps1") {
		b.WriteString(footerSt.Render("Exit Grove before running this command.\n"))
	}
	b.WriteString(footerSt.Render("[q] Dismiss  [Esc] Cancel\n"))

	return borderSt.Render(b.String())
}
