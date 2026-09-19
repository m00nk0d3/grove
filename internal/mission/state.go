package mission

import (
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
)

// BuildInput contains the source state used to build a mission-control view.
type BuildInput struct {
	RepoPath           string
	Worktrees          []domain.Worktree
	Issues             []domain.Issue
	PullRequests       []domain.PullRequest
	Sessions           []domain.Session
	HerdrSnapshot      *herdr.Snapshot  // Internal use only
	SandcastleSnapshot *sandcastle.Snapshot // Internal use only
	PreviousState      *domain.MissionControlState
	Now                time.Time
}

// BuildState creates a complete mission-control state from the available sources.
func BuildState(input BuildInput) domain.MissionControlState {
	wfs := snapshotWorkflows(input.SandcastleSnapshot)
	ags := snapshotAgents(input.SandcastleSnapshot)
	pns := snapshotPanes(input.HerdrSnapshot)

	state := domain.MissionControlState{
		Status:       domain.UnknownState,
		Details:      make(map[string]interface{}),
		RepoPath:     input.RepoPath,
		WorkItems:    []domain.WorkItem{},
		Worktrees:    cloneWorktrees(input.Worktrees),
		Issues:       cloneIssues(input.Issues),
		PullRequests: clonePullRequests(input.PullRequests),
		Sessions:     cloneSessions(input.Sessions),
		WorkflowRuns: wfs,
		Agents:       ags,
		Panes:        pns,
		Integrations: domain.IntegrationStatus{
			Herdr:      herdrIntegration(input.HerdrSnapshot, input.PreviousState),
			Sandcastle: sandcastleIntegration(input.SandcastleSnapshot),
			GitHub:     missingIntegration(false),
		},
		Warnings:  []domain.Warning{},
		UpdatedAt: input.Now,
	}

	state.WorkItems = correlateAll(
		input.Worktrees,
		input.Issues,
		input.PullRequests,
		wfs,
		ags,
		pns,
	)

	// Set timestamps on work items that don't have them.
	now := input.Now.Unix()
	for i := range state.WorkItems {
		if state.WorkItems[i].CreatedAt == 0 {
			state.WorkItems[i].CreatedAt = now
		}
		if state.WorkItems[i].UpdatedAt == 0 {
			state.WorkItems[i].UpdatedAt = now
		}
	}

	// Preserve previous state WorkItems when Sandcastle is unavailable
	if input.SandcastleSnapshot == nil && input.PreviousState != nil && len(input.PreviousState.WorkItems) > 0 {
		for _, pw := range input.PreviousState.WorkItems {
			existing := false
			for _, wi := range state.WorkItems {
				if wi.ID == pw.ID {
					existing = true
					break
				}
			}
			if !existing {
				state.WorkItems = append(state.WorkItems, pw)
			}
		}
		// Mark as degraded only when BOTH integrations are truly unavailable (Available=false)
		sc := sandcastleIntegration(input.SandcastleSnapshot)
		hr := herdrIntegration(input.HerdrSnapshot, input.PreviousState)
		if sc.Available == false && hr.Available == false {
			state.Status = domain.DegradedState
		}
	}

	return state
}

// snapshotWorkflows extracts workflow data from a Sandcastle snapshot, returning
// an empty slice for nil snapshots.
func snapshotWorkflows(snap *sandcastle.Snapshot) []domain.WorkflowRunRef {
	if snap == nil {
		return []domain.WorkflowRunRef{}
	}
	return snap.Workflows
}

// snapshotAgents extracts agent data from a Sandcastle snapshot.
func snapshotAgents(snap *sandcastle.Snapshot) []domain.AgentRef {
	if snap == nil {
		return []domain.AgentRef{}
	}
	return snap.Agents
}

// snapshotPanes extracts pane data from a Herdr snapshot.
func snapshotPanes(snap *herdr.Snapshot) []domain.PaneRef {
	if snap == nil {
		return []domain.PaneRef{}
	}
	return snap.Panes
}

func herdrIntegration(snapshot *herdr.Snapshot, previousState *domain.MissionControlState) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration(false) // Integration not available at all
	}
	// Snapshot exists - integration is available
	// Check if mode is set, otherwise it's "missing" but still available (just haven't queried yet)
	if snapshot.Integration.Mode == "" {
		snapshot.Integration.Available = true
		snapshot.Integration.Enabled = true
		snapshot.Integration.Mode = "missing" // Mode not determined yet - will be filled by adapter
	} else {
		// Use existing mode from snapshot (e.g., "available", "degraded")
		snapshot.Integration.Available = true
		snapshot.Integration.Enabled = true
	}
	return snapshot.Integration
}

func sandcastleIntegration(snapshot *sandcastle.Snapshot) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration(false) // Integration not available at all
	}
	// Snapshot exists - integration is available
	if snapshot.Integration.Mode == "" {
		snapshot.Integration.Available = true
		snapshot.Integration.Enabled = true
		snapshot.Integration.Mode = "missing" // Mode not determined yet - will be filled by adapter
	} else {
		// Use existing mode from snapshot (e.g., "available", "degraded")
		snapshot.Integration.Available = true
		snapshot.Integration.Enabled = true
	}
	return snapshot.Integration
}

func missingIntegration(available bool) domain.ExternalIntegration {
	if available {
		return domain.ExternalIntegration{
			Mode:     "missing",
			Available: true,
			Enabled:  true,
		}
	}
	return domain.ExternalIntegration{
		Mode:     "missing",
		Available: false,
		Enabled:  false,
	}
}

// Clone functions to avoid mutations
func cloneWorktrees(items []domain.Worktree) []domain.Worktree {
	cloned := make([]domain.Worktree, len(items))
	for i, item := range items {
		cloned[i] = item
		if item.LinkedPR != nil {
			pr := clonePullRequest(*item.LinkedPR)
			cloned[i].LinkedPR = &pr
		}
		if item.LinkedIssue != nil {
			issue := cloneIssue(*item.LinkedIssue)
			cloned[i].LinkedIssue = &issue
		}
	}
	return cloned
}

func cloneIssues(items []domain.Issue) []domain.Issue {
	cloned := make([]domain.Issue, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].Labels = append([]string(nil), item.Labels...)
		cloned[i].Assignees = append([]string(nil), item.Assignees...)
		if item.ParentNumber != nil {
			pn := *item.ParentNumber
			cloned[i].ParentNumber = &pn
		}
		cloned[i].SubIssueNumbers = append([]int(nil), item.SubIssueNumbers...)
	}
	return cloned
}

func clonePullRequests(items []domain.PullRequest) []domain.PullRequest {
	cloned := make([]domain.PullRequest, len(items))
	for i, item := range items {
		cloned[i] = clonePullRequest(item)
	}
	return cloned
}

func clonePullRequest(item domain.PullRequest) domain.PullRequest {
	item.Labels = append([]string(nil), item.Labels...)
	item.Assignees = append([]string(nil), item.Assignees...)
	return item
}

func cloneIssue(item domain.Issue) domain.Issue {
	item.Labels = append([]string(nil), item.Labels...)
	item.Assignees = append([]string(nil), item.Assignees...)
	if item.ParentNumber != nil {
		pn := *item.ParentNumber
		item.ParentNumber = &pn
	}
	item.SubIssueNumbers = append([]int(nil), item.SubIssueNumbers...)
	return item
}

func cloneSessions(items []domain.Session) []domain.Session {
	cloned := make([]domain.Session, len(items))
	for i, item := range items {
		cloned[i] = item
		if item.ShellPID != nil {
			v := *item.ShellPID
			cloned[i].ShellPID = &v
		}
		if item.AgentName != nil {
			v := *item.AgentName
			cloned[i].AgentName = &v
		}
		if item.Prompt != nil {
			v := *item.Prompt
			cloned[i].Prompt = &v
		}
	}
	return cloned
}
