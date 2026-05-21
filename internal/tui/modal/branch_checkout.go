package modal

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// BranchCheckoutModal is a single-step confirmation modal for creating a worktree
// from an existing local or remote branch selected via the fuzzy finder.
type BranchCheckoutModal struct {
	branch string
	path   string
}

// NewBranchCheckoutModal creates a new BranchCheckoutModal for the given branch and
// target worktree path.
func NewBranchCheckoutModal(branch, path string) *BranchCheckoutModal {
	return &BranchCheckoutModal{branch: branch, path: path}
}

// Init satisfies tea.Model.
func (m *BranchCheckoutModal) Init() tea.Cmd { return nil }

// Title returns the modal title for themed overlay rendering.
func (m *BranchCheckoutModal) Title() string { return "Checkout Branch" }

// Update handles y/n/Esc input and emits the appropriate confirmation or cancel message.
func (m *BranchCheckoutModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "y", "Y":
		branch, path := m.branch, m.path
		return m, func() tea.Msg { return PRWorktreeCreateConfirmedMsg{Branch: branch, Path: path} }
	case "n", "N", "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	return m, nil
}

// View renders the branch checkout confirmation dialog.
func (m *BranchCheckoutModal) View() string {
	return fmt.Sprintf(
		"Create worktree for branch %q?\n  Path: %s\n\n[y] confirm  [n / Esc] cancel",
		m.branch,
		m.path,
	)
}
