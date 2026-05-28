package modal

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

// PRCheckoutModal is a single-step confirmation modal for checking out a PR into a new worktree.
type PRCheckoutModal struct {
	pr   domain.PullRequest
	path string
}

// NewPRCheckoutModal creates a new PRCheckoutModal for the given PR and target worktree path.
func NewPRCheckoutModal(pr domain.PullRequest, path string) *PRCheckoutModal {
	return &PRCheckoutModal{pr: pr, path: path}
}

// Init satisfies tea.Model.
func (m *PRCheckoutModal) Init() tea.Cmd { return nil }

// Title returns the modal title for themed overlay rendering.
func (m *PRCheckoutModal) Title() string { return "Checkout PR" }

// Update handles y/n/Esc input and emits the appropriate confirmation or cancel message.
func (m *PRCheckoutModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "y", "Y":
		branch, path := m.pr.Branch, m.path
		return m, func() tea.Msg { return PRWorktreeCreateConfirmedMsg{Branch: branch, Path: path} }
	case "n", "N", "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	return m, nil
}

// View renders the PR checkout confirmation dialog.
func (m *PRCheckoutModal) View() string {
	return fmt.Sprintf(
		"Create worktree for PR #%d?\n  Title:  %s\n  Branch: %s\n  Path:   %s\n\n[y] confirm  [n / Esc] cancel",
		m.pr.Number,
		m.pr.Title,
		m.pr.Branch,
		m.path,
	)
}
