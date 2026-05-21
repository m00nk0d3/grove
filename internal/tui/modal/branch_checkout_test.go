package modal_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBranchCheckoutModal_Title(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")
	assert.Equal(t, "Checkout Branch", m.Title())
}

func TestBranchCheckoutModal_View(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")
	view := m.View()
	assert.Contains(t, view, "feat/login")
	assert.Contains(t, view, "/worktrees/feat-login")
	assert.Contains(t, view, "[y]")
	assert.Contains(t, view, "[n")
}

func TestBranchCheckoutModal_ConfirmY(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, next)
	require.NotNil(t, cmd)

	msg := cmd()
	confirmed, ok := msg.(modal.PRWorktreeCreateConfirmedMsg)
	require.True(t, ok, "expected PRWorktreeCreateConfirmedMsg, got %T", msg)
	assert.Equal(t, "feat/login", confirmed.Branch)
	assert.Equal(t, "/worktrees/feat-login", confirmed.Path)
}

func TestBranchCheckoutModal_ConfirmUpperY(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(modal.PRWorktreeCreateConfirmedMsg)
	assert.True(t, ok)
}

func TestBranchCheckoutModal_CancelN(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(modal.ModalCancelledMsg)
	assert.True(t, ok)
}

func TestBranchCheckoutModal_CancelEsc(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(modal.ModalCancelledMsg)
	assert.True(t, ok)
}

func TestBranchCheckoutModal_OtherKeyIsNoop(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	assert.Nil(t, cmd)
	assert.Equal(t, m, next)
}

func TestBranchCheckoutModal_NonKeyMsgIsNoop(t *testing.T) {
	m := modal.NewBranchCheckoutModal("feat/login", "/worktrees/feat-login")

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Nil(t, cmd)
	assert.Equal(t, m, next)
}
