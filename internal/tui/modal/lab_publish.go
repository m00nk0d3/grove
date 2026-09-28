package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/tui/markdown"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// LabPublishBoard is a project board an issue can be placed on.
type LabPublishBoard struct {
	// Ref is the board as "owner/number".
	Ref   string
	Title string
}

// LabPublishPlan is everything publishing an entry will create, shown for
// confirmation before anything is sent.
type LabPublishPlan struct {
	EntryID string
	Repo    string
	Title   string
	Body    string
	Labels  []string
	// Boards are the boards the issue can go to. With more than one, the user
	// picks; the last choice is "no board".
	Boards []LabPublishBoard
	// Board is the chosen index into Boards, or -1 for no board.
	Board int
	// Tickets are an epic's tickets, published as its sub-issues in this order.
	Tickets []LabPublishTicket
	// Notes are further consequences of publishing, each shown on its own line.
	Notes []string
	// Resume describes what an earlier, interrupted publication already
	// created, which is not created again; empty for a first attempt.
	Resume string
}

// LabPublishTicket is a ticket as the preview lists it.
type LabPublishTicket struct {
	Key       string
	Title     string
	BlockedBy []string
}

// LabPublishConfirmedMsg confirms publishing with the chosen board.
type LabPublishConfirmedMsg struct {
	EntryID string
	// BoardRef is the chosen board, or empty for none.
	BoardRef string
	// Chosen is set when the user picked the board among several, so Grove
	// remembers the choice.
	Chosen bool
}

// LabPublishModal previews an entry's publication and asks for confirmation.
type LabPublishModal struct {
	plan   LabPublishPlan
	scroll int
	inspectorChrome
	height int
}

// NewLabPublishModal opens the preview for plan.
func NewLabPublishModal(plan LabPublishPlan) *LabPublishModal {
	if plan.Board >= len(plan.Boards) {
		plan.Board = -1
	}
	return &LabPublishModal{plan: plan}
}

// Init satisfies tea.Model.
func (m *LabPublishModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabPublishModal) Title() string { return "PUBLISH TO GITHUB" }

// Fullscreen makes the preview fill the terminal.
func (m *LabPublishModal) Fullscreen() bool { return true }

// SetWidth records the terminal width.
func (m *LabPublishModal) SetWidth(width int) { m.width = width }

// SetHeight records the terminal height.
func (m *LabPublishModal) SetHeight(height int) { m.height = height }

// SetTheme applies the active theme.
func (m *LabPublishModal) SetTheme(theme styles.Theme) { m.theme = &theme }

func (m *LabPublishModal) choosable() bool { return len(m.plan.Boards) > 1 }

// Update handles confirming, cancelling, choosing a board, and scrolling.
func (m *LabPublishModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		confirmed := LabPublishConfirmedMsg{EntryID: m.plan.EntryID, Chosen: m.choosable() && m.plan.Board >= 0}
		if m.plan.Board >= 0 {
			confirmed.BoardRef = m.plan.Boards[m.plan.Board].Ref
		}
		return m, func() tea.Msg { return confirmed }
	case "n", "N", "esc", "q":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case "left", "h":
		if m.choosable() {
			m.plan.Board = m.cycleBoard(-1)
		}
	case "right", "l", "tab":
		if m.choosable() {
			m.plan.Board = m.cycleBoard(1)
		}
	case "down", "j":
		m.scroll++
	case "up", "k":
		m.scroll = max(m.scroll-1, 0)
	case "pgdown":
		m.scroll += m.bodyRows()
	case "pgup":
		m.scroll = max(m.scroll-m.bodyRows(), 0)
	}
	return m, nil
}

// cycleBoard moves through the boards and "no board", which comes last.
func (m *LabPublishModal) cycleBoard(delta int) int {
	n := len(m.plan.Boards) + 1
	pos := m.plan.Board
	if pos < 0 {
		pos = n - 1
	}
	pos = (pos + delta + n) % n
	if pos == n-1 {
		return -1
	}
	return pos
}

func (m *LabPublishModal) bodyRows() int {
	if m.height < 20 {
		return 12
	}
	return max(m.height-20, 4)
}

// View renders what will be created and the confirmation keys.
func (m *LabPublishModal) View() string {
	p := m.plan
	var b strings.Builder
	b.WriteString(m.accentStyle().Bold(true).Render("◈ LAB // PUBLISH"))
	b.WriteString("\n")
	if p.Resume != "" {
		b.WriteString(m.statusStyle("blocked").Render(p.Resume))
	} else {
		b.WriteString(m.mutedStyle().Render("Nothing is sent to GitHub until you confirm."))
	}
	b.WriteString("\n\n")

	b.WriteString(m.field("Repository", p.Repo))
	labels := "—"
	if len(p.Labels) > 0 {
		labels = strings.Join(p.Labels, ", ")
	}
	b.WriteString(m.field("Labels", labels))
	b.WriteString(m.field("Board", m.boardLabel()))
	b.WriteString(m.field("Title", lipgloss.NewStyle().Bold(true).Render(m.truncateTo(p.Title, m.contentWidth()-20))))
	for _, note := range p.Notes {
		b.WriteString(m.field("Note", note))
	}
	if len(p.Tickets) > 0 {
		b.WriteString("\n")
		b.WriteString(m.heading(fmt.Sprintf("TICKETS  %d sub-issues, labelled ready-for-agent, in this order", len(p.Tickets))))
		b.WriteString("\n")
		for _, t := range p.Tickets {
			blockers := "can start immediately"
			if len(t.BlockedBy) > 0 {
				blockers = "blocked by " + strings.Join(t.BlockedBy, ", ")
			}
			b.WriteString(fmt.Sprintf("  %s  %s", t.Key, m.truncateTo(t.Title, max(20, m.contentWidth()-34))))
			b.WriteString(m.mutedStyle().Render("   " + blockers))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.mutedStyle().Render(strings.Repeat("─", m.contentWidth())))
	b.WriteString("\n")

	lines := strings.Split(markdown.Render(p.Body, m.contentWidth()-2, m.currentTheme()), "\n")
	rows := m.bodyRows()
	m.scroll = min(m.scroll, max(len(lines)-rows, 0))
	end := min(m.scroll+rows, len(lines))
	b.WriteString(strings.Join(lines[m.scroll:end], "\n"))
	if len(lines) > rows {
		b.WriteString("\n" + m.mutedStyle().Render(fmt.Sprintf("  lines %d–%d of %d", m.scroll+1, end, len(lines))))
	}
	b.WriteString("\n\n")

	hints := []string{"y", "publish", "n/Esc", "cancel", "j/k", "scroll"}
	if m.choosable() {
		hints = append(hints, "←/→", "board")
	}
	b.WriteString(m.keyHints(hints...))
	return b.String()
}

func (m *LabPublishModal) boardLabel() string {
	p := m.plan
	if p.Board < 0 {
		if len(p.Boards) == 0 {
			return m.mutedStyle().Render("none — the repository has no linked project board")
		}
		return m.mutedStyle().Render("none")
	}
	board := p.Boards[p.Board]
	label := fmt.Sprintf("%s (%s), status Backlog", board.Title, board.Ref)
	if m.choosable() {
		label += m.mutedStyle().Render(fmt.Sprintf("   %d of %d — ←/→ to change; your choice is remembered", p.Board+1, len(p.Boards)))
	}
	return label
}
