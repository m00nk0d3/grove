package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRCheckoutModal_Title(t *testing.T) {
	pr := domain.PullRequest{Number: 42, Title: "Add feature", Branch: "feat/issue-42-add-feature"}
	m := NewPRCheckoutModal(pr, "/worktrees/feat-issue-42-add-feature")
	assert.Equal(t, "Checkout PR", m.Title())
}

func TestPRCheckoutModal_View(t *testing.T) {
	pr := domain.PullRequest{Number: 7, Title: "Fix bug", Branch: "fix/issue-7-crash"}
	path := "/worktrees/fix-issue-7-crash"
	m := NewPRCheckoutModal(pr, path)
	view := m.View()
	assert.Contains(t, view, "PR #7")
	assert.Contains(t, view, "Fix bug")
	assert.Contains(t, view, "fix/issue-7-crash")
	assert.Contains(t, view, path)
	assert.Contains(t, view, "[y] confirm")
	assert.Contains(t, view, "[n / Esc] cancel")
}

func TestPRCheckoutModal_Init(t *testing.T) {
	pr := domain.PullRequest{Number: 1, Branch: "feat/issue-1-init"}
	m := NewPRCheckoutModal(pr, "/worktrees/feat-issue-1-init")
	cmd := m.Init()
	assert.Nil(t, cmd)
}

func TestPRCheckoutModal_Update(t *testing.T) {
	pr := domain.PullRequest{Number: 99, Title: "My PR", Branch: "feat/issue-99-my-pr"}
	path := "/worktrees/feat-issue-99-my-pr"

	tests := []struct {
		name        string
		key         string
		wantMsgType interface{}
	}{
		{
			name:        "y confirms checkout",
			key:         "y",
			wantMsgType: PRWorktreeCreateConfirmedMsg{},
		},
		{
			name:        "Y also confirms checkout",
			key:         "Y",
			wantMsgType: PRWorktreeCreateConfirmedMsg{},
		},
		{
			name:        "n cancels",
			key:         "n",
			wantMsgType: ModalCancelledMsg{},
		},
		{
			name:        "N also cancels",
			key:         "N",
			wantMsgType: ModalCancelledMsg{},
		},
		{
			name:        "esc cancels",
			key:         "esc",
			wantMsgType: ModalCancelledMsg{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewPRCheckoutModal(pr, path)
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			if tt.key == "esc" {
				_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			}
			require.NotNil(t, cmd)
			result := cmd()
			switch tt.wantMsgType.(type) {
			case PRWorktreeCreateConfirmedMsg:
				msg, ok := result.(PRWorktreeCreateConfirmedMsg)
				require.True(t, ok, "expected PRWorktreeCreateConfirmedMsg, got %T", result)
				assert.Equal(t, pr.Branch, msg.Branch)
				assert.Equal(t, path, msg.Path)
			case ModalCancelledMsg:
				_, ok := result.(ModalCancelledMsg)
				require.True(t, ok, "expected ModalCancelledMsg, got %T", result)
			}
		})
	}
}

func TestPRCheckoutModal_Update_NonKeyMsg(t *testing.T) {
	pr := domain.PullRequest{Number: 1, Branch: "feat/issue-1"}
	m := NewPRCheckoutModal(pr, "/worktrees/feat-issue-1")
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Nil(t, cmd)
}

func TestPRCheckoutModal_Update_OtherKeys(t *testing.T) {
	pr := domain.PullRequest{Number: 1, Branch: "feat/issue-1"}
	m := NewPRCheckoutModal(pr, "/worktrees/feat-issue-1")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	assert.Nil(t, cmd)
}
