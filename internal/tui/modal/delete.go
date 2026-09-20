package modal

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

// DeleteModal is a single-step confirmation modal for removing a worktree.
type DeleteModal struct {
	worktree domain.Worktree
}

// NewDeleteModal creates a new DeleteModal for the given worktree.
func NewDeleteModal(wt domain.Worktree) *DeleteModal {
	return &DeleteModal{worktree: wt}
}

// Init satisfies tea.Model.
func (m *DeleteModal) Init() tea.Cmd { return nil }

// Update handles y/n/Esc input and emits the appropriate confirmation or cancel message.
func (m *DeleteModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "y", "Y":
		return m, func() tea.Msg {
			return WorktreeDeleteConfirmedMsg{
				Path:   m.worktree.Path,
				Branch: m.worktree.Branch,
			}
		}
	case "n", "N", "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	return m, nil
}

// Title returns the modal title for themed overlay rendering.
func (m *DeleteModal) Title() string { return "Delete Worktree" }

// View renders the delete confirmation dialog.
func (m *DeleteModal) View() string {
	name := filepath.Base(m.worktree.Path)
	return fmt.Sprintf(
		"Delete worktree %q and local branch %q?\n\nUncommitted work and the local branch will be permanently removed.\nThe remote branch will be preserved.\n\n[y] delete both  [n / Esc] cancel",
		name,
		m.worktree.Branch,
	)
}
