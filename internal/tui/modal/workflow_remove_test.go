package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRemoveModal_ConfirmsActiveStop(t *testing.T) {
	m := NewWorkflowRemoveModal(domain.WorkflowRunRef{
		RunID:  "run-active",
		Title:  "Implement issue #42",
		Status: domain.WorkflowRunning,
	})

	assert.Equal(t, "STOP WORKFLOW", m.Title())
	assert.Contains(t, m.View(), "worktree and Git branch will be preserved")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(WorkflowRemoveConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, "run-active", msg.RunID)
	assert.True(t, msg.Stop)
}

func TestWorkflowRemoveModal_RemovesCompletedHistoryWithoutStop(t *testing.T) {
	m := NewWorkflowRemoveModal(domain.WorkflowRunRef{
		RunID:  "run-complete",
		Title:  "Review pull request #17",
		Status: domain.WorkflowSucceeded,
	})

	assert.Equal(t, "REMOVE WORKFLOW", m.Title())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(WorkflowRemoveConfirmedMsg)
	require.True(t, ok)
	assert.False(t, msg.Stop)
}
