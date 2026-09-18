package mission

import (
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
)

const integrationModeMissing = "missing"

// BuildInput contains the source state used to build a mission-control view.
type BuildInput struct {
	RepoPath           string
	Worktrees          []domain.Worktree
	Issues             []domain.Issue
	PullRequests       []domain.PullRequest
	Sessions           []domain.Session
	HerdrSnapshot      *herdr.Snapshot
	SandcastleSnapshot *sandcastle.Snapshot
	PreviousState      *domain.MissionControlState
	Now                time.Time
}

// BuildState creates a complete mission-control state from the available
// sources. Correlation and previous-state recovery are added in later phases.
func BuildState(input BuildInput) domain.MissionControlState {
	return domain.MissionControlState{
		Status:       domain.UnknownState,
		Details:      make(map[string]interface{}),
		RepoPath:     input.RepoPath,
		WorkItems:    []domain.WorkItem{},
		Worktrees:    cloneSlice(input.Worktrees),
		Issues:       cloneSlice(input.Issues),
		PullRequests: cloneSlice(input.PullRequests),
		Sessions:     cloneSlice(input.Sessions),
		WorkflowRuns: []domain.WorkflowRunRef{},
		Agents:       []domain.AgentRef{},
		Panes:        []domain.PaneRef{},
		Integrations: domain.IntegrationStatus{
			Herdr:      herdrIntegration(input.HerdrSnapshot),
			Sandcastle: sandcastleIntegration(input.SandcastleSnapshot),
			GitHub:     missingIntegration(),
		},
		Warnings:  []domain.Warning{},
		UpdatedAt: input.Now,
	}
}

func herdrIntegration(snapshot *herdr.Snapshot) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration()
	}
	return presentIntegration(snapshot.Integration)
}

func sandcastleIntegration(snapshot *sandcastle.Snapshot) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration()
	}
	return presentIntegration(snapshot.Integration)
}

func missingIntegration() domain.ExternalIntegration {
	return domain.ExternalIntegration{Mode: integrationModeMissing}
}

func presentIntegration(integration domain.ExternalIntegration) domain.ExternalIntegration {
	if integration == (domain.ExternalIntegration{}) {
		integration.Available = true
		integration.Enabled = true
	}
	return integration
}

func cloneSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return append([]T(nil), items...)
}
