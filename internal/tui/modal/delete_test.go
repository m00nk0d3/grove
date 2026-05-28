package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testWorktree = domain.Worktree{
	Path:   "/home/user/worktrees/feat-issue-5-create-delete-modals",
	Branch: "feat/issue-5-create-delete-modals",
}

func TestNewDeleteModal_StoresWorktree(t *testing.T) {
	m := NewDeleteModal(testWorktree)

	require.NotNil(t, m)
	assert.Equal(t, testWorktree, m.worktree)
}

func TestDeleteModal_View_ShowsWorktreeNameAndBranch(t *testing.T) {
	m := NewDeleteModal(testWorktree)
	view := m.View()

	assert.Contains(t, view, "feat-issue-5-create-delete-modals")
	assert.Contains(t, view, "feat/issue-5-create-delete-modals")
}

func TestDeleteModal_View_ShowsConfirmOptions(t *testing.T) {
	m := NewDeleteModal(testWorktree)
	view := m.View()

	assert.Contains(t, view, "y")
	assert.Contains(t, view, "n")
}

func TestDeleteModal_Y_EmitsDeleteConfirmedMsg(t *testing.T) {
	m := NewDeleteModal(testWorktree)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	require.NotNil(t, cmd)

	msg := cmd()
	confirmed, ok := msg.(WorktreeDeleteConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, testWorktree.Path, confirmed.Path)
}

func TestDeleteModal_N_EmitsCancelMsg(t *testing.T) {
	m := NewDeleteModal(testWorktree)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok)
}

func TestDeleteModal_Esc_EmitsCancelMsg(t *testing.T) {
	m := NewDeleteModal(testWorktree)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok)
}

func TestDeleteModal_OtherKeys_DoNothing(t *testing.T) {
	m := NewDeleteModal(testWorktree)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
}
