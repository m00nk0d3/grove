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
		Worktrees:    cloneWorktrees(input.Worktrees),
		Issues:       cloneIssues(input.Issues),
		PullRequests: clonePullRequests(input.PullRequests),
		Sessions:     cloneSessions(input.Sessions),
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
	cloned := make([]T, len(items))
	copy(cloned, items)
	return cloned
}

func cloneWorktrees(items []domain.Worktree) []domain.Worktree {
	cloned := cloneSlice(items)
	for i := range cloned {
		if items[i].LinkedPR != nil {
			linkedPR := clonePullRequest(*items[i].LinkedPR)
			cloned[i].LinkedPR = &linkedPR
		}
	}
	return cloned
}

func cloneIssues(items []domain.Issue) []domain.Issue {
	cloned := cloneSlice(items)
	for i := range cloned {
		cloned[i].Labels = append([]string(nil), items[i].Labels...)
		cloned[i].Assignees = append([]string(nil), items[i].Assignees...)
		cloned[i].ParentNumber = clonePointer(items[i].ParentNumber)
		cloned[i].SubIssueNumbers = append([]int(nil), items[i].SubIssueNumbers...)
	}
	return cloned
}

func clonePullRequests(items []domain.PullRequest) []domain.PullRequest {
	cloned := cloneSlice(items)
	for i := range cloned {
		cloned[i] = clonePullRequest(items[i])
	}
	return cloned
}

func clonePullRequest(item domain.PullRequest) domain.PullRequest {
	item.Labels = append([]string(nil), item.Labels...)
	item.Assignees = append([]string(nil), item.Assignees...)
	return item
}

func cloneSessions(items []domain.Session) []domain.Session {
	cloned := cloneSlice(items)
	for i := range cloned {
		cloned[i].ShellPID = clonePointer(items[i].ShellPID)
		cloned[i].AgentName = clonePointer(items[i].AgentName)
		cloned[i].Prompt = clonePointer(items[i].Prompt)
	}
	return cloned
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
