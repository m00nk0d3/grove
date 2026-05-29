package modal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupModal_ConfirmCmd_IncludesWorktreeBranch(t *testing.T) {
	candidates := []CleanupCandidate{
		{Kind: CandidateWorktree, Path: "/wt/feat-issue-5", Branch: "feat/issue-5"},
		{Kind: CandidateBranch, Branch: "feat/old-feature"},
	}
	m := NewCleanupModal(candidates)

	// Select all entries.
	for i := range m.entries {
		m.entries[i].selected = true
	}

	cmd := m.confirmCmd()
	require.NotNil(t, cmd)

	msg := cmd()
	confirmed, ok := msg.(CleanupConfirmedMsg)
	require.True(t, ok, "expected CleanupConfirmedMsg")

	assert.Equal(t, []string{"/wt/feat-issue-5"}, confirmed.Worktrees)
	assert.ElementsMatch(t, []string{"feat/issue-5", "feat/old-feature"}, confirmed.Branches,
		"worktree branch should be included in Branches so it gets deleted too")
}

func TestCleanupModal_ConfirmCmd_NothingSelected_Cancels(t *testing.T) {
	candidates := []CleanupCandidate{
		{Kind: CandidateWorktree, Path: "/wt/feat-issue-5", Branch: "feat/issue-5"},
	}
	m := NewCleanupModal(candidates)
	// Nothing selected.

	cmd := m.confirmCmd()
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok, "nothing selected should return ModalCancelledMsg")
}

func TestCleanupModal_ConfirmCmd_OnlyBranchSelected(t *testing.T) {
	candidates := []CleanupCandidate{
		{Kind: CandidateWorktree, Path: "/wt/feat-issue-5", Branch: "feat/issue-5"},
		{Kind: CandidateBranch, Branch: "feat/old"},
	}
	m := NewCleanupModal(candidates)
	m.entries[1].selected = true // only branch entry selected

	cmd := m.confirmCmd()
	require.NotNil(t, cmd)

	msg := cmd()
	confirmed, ok := msg.(CleanupConfirmedMsg)
	require.True(t, ok)

	assert.Empty(t, confirmed.Worktrees)
	assert.Equal(t, []string{"feat/old"}, confirmed.Branches)
}
