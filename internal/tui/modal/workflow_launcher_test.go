package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueWorkflowLauncherStartsImp(t *testing.T) {
	m := NewIssueWorkflowLauncherModal(domain.Issue{Number: 42})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg, ok := cmd().(WorkflowLaunchMsg)
	require.True(t, ok)
	assert.Equal(t, WorkflowKindImplement, msg.Kind)
	require.NotNil(t, msg.IssueNumber)
	assert.Equal(t, 42, *msg.IssueNumber)
}

func TestPRWorkflowLauncherSelectsCI(t *testing.T) {
	m := NewPRWorkflowLauncherModal(domain.PullRequest{Number: 17})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	require.NotNil(t, cmd)

	msg, ok := cmd().(WorkflowLaunchMsg)
	require.True(t, ok)
	assert.Equal(t, WorkflowKindCI, msg.Kind)
	require.NotNil(t, msg.PRNumber)
	assert.Equal(t, 17, *msg.PRNumber)
}

func TestMaintenanceWorkflowLauncherStartsClean(t *testing.T) {
	m := NewMaintenanceWorkflowLauncherModal()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg, ok := cmd().(WorkflowLaunchMsg)
	require.True(t, ok)
	assert.Equal(t, WorkflowKindClean, msg.Kind)
	assert.Nil(t, msg.IssueNumber)
	assert.Nil(t, msg.PRNumber)
}
