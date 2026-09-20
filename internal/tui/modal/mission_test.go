package modal

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func missionTestWorkflow() domain.WorkflowRunRef {
	started := time.Now().Add(-5 * time.Minute)
	return domain.WorkflowRunRef{
		WorkflowID:   "run-42",
		RunID:        "run-42",
		Title:        "Implement issue #42",
		Status:       domain.WorkflowRunning,
		CurrentStep:  "implementation",
		DefaultAgent: "opencode",
		Branch:       "issue-42",
		WorktreePath: "/repos/grove-42",
		Progress:     domain.WorkflowProgress{Completed: 1, Total: 3, Percent: 33},
		StartedAt:    started,
		UpdatedAt:    time.Now(),
		Steps: []domain.WorkflowStep{
			{ID: "plan", Title: "Plan implementation", Status: domain.WorkflowSucceeded, DurationMillis: 1200},
			{ID: "implement", Title: "Implement changes", Status: domain.WorkflowRunning, StartedAt: started},
			{ID: "verify", Title: "Verify changes", Status: domain.WorkflowQueued},
		},
	}
}

func TestMissionModal_RendersWorkflowTelemetry(t *testing.T) {
	workflow := missionTestWorkflow()
	m := NewMissionModal(workflow, []domain.AgentRef{
		{Name: "implementer", Kind: "opencode", Status: domain.AgentWorking, Summary: "Editing renderer", PaneID: "w1:p2"},
	})
	m.SetWidth(100)
	m.SetHeight(30)

	overview := m.View()
	assert.Contains(t, overview, "SANDCASTLE // MISSION INTELLIGENCE")
	assert.Contains(t, overview, "Implement issue #42")
	assert.Contains(t, overview, "implementation")
	assert.Contains(t, overview, "1/3 steps")
	assert.Contains(t, overview, "ELAPSED")
	assert.Contains(t, overview, "MISSION SIGNAL")
	assert.Contains(t, overview, "EXECUTION CONTEXT")

	m.activeTab = missionSteps
	assert.Contains(t, m.View(), "Plan implementation")
	assert.Contains(t, m.View(), "Implement changes")

	m.activeTab = missionMetrics
	assert.Contains(t, m.View(), "STEP HEALTH")
	assert.Contains(t, m.View(), "33%")

	m.activeTab = missionImplementation
	assert.Contains(t, m.View(), "implementer")
	assert.Contains(t, m.View(), "Editing renderer")
}

func TestMissionModal_NavigatesTabsAndSteps(t *testing.T) {
	m := NewMissionModal(missionTestWorkflow(), nil)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	require.Nil(t, cmd)
	model := updated.(*MissionModal)
	assert.Equal(t, missionSteps, model.activeTab)

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Nil(t, cmd)
	assert.Equal(t, 1, updated.(*MissionModal).selectedStep)
}

func TestMissionModal_EnterRequestsWorkflowJump(t *testing.T) {
	m := NewMissionModal(missionTestWorkflow(), nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg, ok := cmd().(MissionJumpMsg)
	require.True(t, ok)
	assert.Equal(t, "run-42", msg.RunID)
}

func TestMissionModal_XRequestsWorkflowRemoval(t *testing.T) {
	m := NewMissionModal(missionTestWorkflow(), nil)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(MissionRemoveRequestedMsg)
	require.True(t, ok)
	assert.Equal(t, "run-42", msg.RunID)
}

func TestMissionModal_SetWorkflowRefreshesTelemetry(t *testing.T) {
	m := NewMissionModal(missionTestWorkflow(), nil)
	updated := missionTestWorkflow()
	updated.Progress = domain.WorkflowProgress{Completed: 3, Total: 3, Percent: 100}
	updated.Status = domain.WorkflowSucceeded

	m.SetWorkflow(updated, []domain.AgentRef{{AgentID: "agent-1"}})

	assert.Equal(t, 100, m.workflow.Progress.Percent)
	assert.Len(t, m.agents, 1)
}

func TestMissionModal_MouseSelectsTabsAndSteps(t *testing.T) {
	m := NewMissionModal(missionTestWorkflow(), nil)
	m.SetWidth(120)
	m.SetHeight(40)

	updated, cmd := m.HandleMouse(tea.MouseMsg{
		X:      23,
		Y:      3 + missionTabsRow,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}, 120, 40)
	require.Nil(t, cmd)
	model := updated.(*MissionModal)
	assert.Equal(t, missionSteps, model.activeTab)

	updated, cmd = model.HandleMouse(tea.MouseMsg{
		X:      8,
		Y:      3 + missionStepFirstRow + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}, 120, 40)
	require.Nil(t, cmd)
	assert.Equal(t, 1, updated.(*MissionModal).selectedStep)
}
