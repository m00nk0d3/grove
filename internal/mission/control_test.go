package mission

import (
	"testing"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildStateEmptyInput(t *testing.T) {
	now := time.Date(2026, time.September, 18, 23, 0, 0, 0, time.UTC)

	state := BuildState(BuildInput{RepoPath: "/repo/grove", Now: now})

	assert.Equal(t, "/repo/grove", state.RepoPath)
	assert.Equal(t, now, state.UpdatedAt)
	assert.Equal(t, domain.UnknownState, state.Status)
	require.NotNil(t, state.Details)
	require.NotNil(t, state.WorkItems)
	require.NotNil(t, state.Worktrees)
	require.NotNil(t, state.Issues)
	require.NotNil(t, state.PullRequests)
	require.NotNil(t, state.Sessions)
	require.NotNil(t, state.WorkflowRuns)
	require.NotNil(t, state.Agents)
	require.NotNil(t, state.Panes)
	require.NotNil(t, state.Warnings)
	assert.Equal(t, integrationModeMissing, state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
	assert.Equal(t, integrationModeMissing, state.Integrations.Sandcastle.Mode)
	assert.False(t, state.Integrations.Sandcastle.Available)
}

func TestBuildStateExplicitlyEmptyCollections(t *testing.T) {
	state := BuildState(BuildInput{
		Worktrees:    []domain.Worktree{},
		Issues:       []domain.Issue{},
		PullRequests: []domain.PullRequest{},
		Sessions:     []domain.Session{},
	})

	require.NotNil(t, state.Worktrees)
	require.NotNil(t, state.Issues)
	require.NotNil(t, state.PullRequests)
	require.NotNil(t, state.Sessions)
}

func TestBuildStateWithoutHerdrSnapshot(t *testing.T) {
	state := BuildState(BuildInput{
		SandcastleSnapshot: &sandcastle.Snapshot{},
	})

	assert.Equal(t, integrationModeMissing, state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
	assert.True(t, state.Integrations.Sandcastle.Available)
	assert.True(t, state.Integrations.Sandcastle.Enabled)
}

func TestBuildStateWithoutSandcastleSnapshot(t *testing.T) {
	state := BuildState(BuildInput{
		HerdrSnapshot: &herdr.Snapshot{},
	})

	assert.True(t, state.Integrations.Herdr.Available)
	assert.True(t, state.Integrations.Herdr.Enabled)
	assert.Equal(t, integrationModeMissing, state.Integrations.Sandcastle.Mode)
	assert.False(t, state.Integrations.Sandcastle.Available)
}

func TestBuildStatePreservesProvidedIntegrationStatus(t *testing.T) {
	state := BuildState(BuildInput{
		HerdrSnapshot: &herdr.Snapshot{
			Integration: domain.ExternalIntegration{
				Enabled: true,
				Mode:    "standalone",
			},
		},
	})

	assert.False(t, state.Integrations.Herdr.Available)
	assert.True(t, state.Integrations.Herdr.Enabled)
	assert.Equal(t, "standalone", state.Integrations.Herdr.Mode)
}

func TestBuildStateCopiesSourceSlices(t *testing.T) {
	parentNumber := 41
	agentName := "pi"
	worktrees := []domain.Worktree{{
		Path:     "/repo/grove",
		LinkedPR: &domain.PullRequest{Labels: []string{"linked"}},
	}}
	issues := []domain.Issue{{
		Labels:          []string{"bug"},
		Assignees:       []string{"octocat"},
		ParentNumber:    &parentNumber,
		SubIssueNumbers: []int{43},
	}}
	pullRequests := []domain.PullRequest{{
		Labels:    []string{"ready"},
		Assignees: []string{"hubot"},
	}}
	sessions := []domain.Session{{AgentName: &agentName}}

	state := BuildState(BuildInput{
		Worktrees:    worktrees,
		Issues:       issues,
		PullRequests: pullRequests,
		Sessions:     sessions,
	})
	worktrees[0].Path = "/changed"
	worktrees[0].LinkedPR.Labels[0] = "changed"
	issues[0].Labels[0] = "changed"
	issues[0].Assignees[0] = "changed"
	*issues[0].ParentNumber = 99
	issues[0].SubIssueNumbers[0] = 99
	pullRequests[0].Labels[0] = "changed"
	pullRequests[0].Assignees[0] = "changed"
	*sessions[0].AgentName = "changed"

	assert.Equal(t, "/repo/grove", state.Worktrees[0].Path)
	assert.Equal(t, "linked", state.Worktrees[0].LinkedPR.Labels[0])
	assert.Equal(t, "bug", state.Issues[0].Labels[0])
	assert.Equal(t, "octocat", state.Issues[0].Assignees[0])
	assert.Equal(t, 41, *state.Issues[0].ParentNumber)
	assert.Equal(t, 43, state.Issues[0].SubIssueNumbers[0])
	assert.Equal(t, "ready", state.PullRequests[0].Labels[0])
	assert.Equal(t, "hubot", state.PullRequests[0].Assignees[0])
	assert.Equal(t, "pi", *state.Sessions[0].AgentName)
}
