package modal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

type cleanupEntry struct {
	candidate CleanupCandidate
	selected  bool
}

// CleanupModal presents a two-section multi-select list of stale worktrees and
// merged branches. Space toggles selection; Enter confirms; Esc cancels.
type CleanupModal struct {
	entries []cleanupEntry
	cursor  int
	width   int
	theme   *styles.Theme
}

// NewCleanupModal creates a CleanupModal populated with the given candidates.
// Worktree candidates are shown first, branch-only candidates second.
func NewCleanupModal(candidates []CleanupCandidate) *CleanupModal {
	entries := make([]cleanupEntry, len(candidates))
	for i, c := range candidates {
		entries[i] = cleanupEntry{candidate: c}
	}
	return &CleanupModal{entries: entries}
}

// SetWidth satisfies the optional SetWidth interface used by the app renderer.
func (m *CleanupModal) SetWidth(w int) { m.width = w }

// SetTheme satisfies the optional SetTheme interface used by the app renderer.
func (m *CleanupModal) SetTheme(t styles.Theme) { m.theme = &t }

// Init satisfies tea.Model.
func (m *CleanupModal) Init() tea.Cmd { return nil }

// Title returns the modal title for themed overlay rendering.
func (m *CleanupModal) Title() string { return "Cleanup Stale Worktrees & Branches" }

// Update handles keyboard navigation, selection, and confirmation.
func (m *CleanupModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }

	case tea.KeyEnter:
		return m, m.confirmCmd()

	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil

	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil

	case tea.KeySpace:
		m.toggleSelected()
		return m, nil
	}

	switch keyMsg.String() {
	case "j":
		m.moveCursor(1)
	case "k":
		m.moveCursor(-1)
	case " ":
		m.toggleSelected()
	case "a", "A":
		for i := range m.entries {
			m.entries[i].selected = true
		}
	case "n", "N":
		for i := range m.entries {
			m.entries[i].selected = false
		}
	}

	return m, nil
}

// View renders the cleanup modal content.
func (m *CleanupModal) View() string {
	if len(m.entries) == 0 {
		return "No stale worktrees or merged branches found.\n\n[Esc] close"
	}

	var (
		accentSt  = lipgloss.NewStyle()
		mutedSt   = lipgloss.NewStyle()
		selectedSt = lipgloss.NewStyle()
		cursorSt  = lipgloss.NewStyle().Bold(true)
	)

	if m.theme != nil {
		accentSt = accentSt.Foreground(lipgloss.Color(m.theme.Accent()))
		mutedSt = mutedSt.Foreground(lipgloss.Color(m.theme.Muted()))
		selectedSt = selectedSt.Foreground(lipgloss.Color(m.theme.Success()))
		cursorSt = cursorSt.Foreground(lipgloss.Color(m.theme.Accent()))
	}

	var b strings.Builder

	worktreeHeader := false
	branchHeader := false

	for i, e := range m.entries {
		// Section headers — printed once per section.
		if e.candidate.Kind == CandidateWorktree && !worktreeHeader {
			b.WriteString(accentSt.Render("─── STALE WORKTREES ───"))
			b.WriteString("\n")
			worktreeHeader = true
		}
		if e.candidate.Kind == CandidateBranch && !branchHeader {
			if worktreeHeader {
				b.WriteString("\n")
			}
			b.WriteString(accentSt.Render("─── MERGED BRANCHES ───"))
			b.WriteString("\n")
			branchHeader = true
		}

		// Checkbox.
		check := "[ ]"
		if e.selected {
			check = selectedSt.Render("[✓]")
		}

		// Cursor indicator.
		cursor := "  "
		if i == m.cursor {
			cursor = cursorSt.Render("▶ ")
		}

		// Item label.
		var label string
		if e.candidate.Kind == CandidateWorktree {
			label = fmt.Sprintf("%-32s  %s",
				e.candidate.Branch,
				mutedSt.Render(filepath.Base(e.candidate.Path)),
			)
		} else {
			label = e.candidate.Branch
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, label))
	}

	// Footer.
	b.WriteString("\n")
	b.WriteString(mutedSt.Render("[↑/↓] navigate  [Space] toggle  [a] select all  [n] deselect all"))
	b.WriteString("\n")
	b.WriteString(mutedSt.Render("[Enter] delete selected  [Esc] cancel"))

	return b.String()
}

func (m *CleanupModal) moveCursor(delta int) {
	if len(m.entries) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.entries) {
		m.cursor = len(m.entries) - 1
	}
}

func (m *CleanupModal) toggleSelected() {
	if m.cursor < len(m.entries) {
		m.entries[m.cursor].selected = !m.entries[m.cursor].selected
	}
}

func (m *CleanupModal) confirmCmd() tea.Cmd {
	var worktrees, branches []string
	for _, e := range m.entries {
		if !e.selected {
			continue
		}
		switch e.candidate.Kind {
		case CandidateWorktree:
			worktrees = append(worktrees, e.candidate.Path)
		case CandidateBranch:
			branches = append(branches, e.candidate.Branch)
		}
	}
	if len(worktrees) == 0 && len(branches) == 0 {
		// Nothing selected — cancel silently.
		return func() tea.Msg { return ModalCancelledMsg{} }
	}
	return func() tea.Msg {
		return CleanupConfirmedMsg{Worktrees: worktrees, Branches: branches}
	}
}
