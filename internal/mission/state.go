package mission

import (
	"strings"
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
	state := domain.MissionControlState{
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
			Herdr:      herdrIntegration(input.HerdrSnapshot, input.PreviousState),
			Sandcastle: sandcastleIntegration(input.SandcastleSnapshot),
			GitHub:     missingIntegration(false), // GitHub not connected to this implementation
		},
		Warnings:  []domain.Warning{},
		UpdatedAt: input.Now,
	}

	// Perform correlation to create work items from branch name matching
	state.WorkItems = correlateWorktreesWithIssuesAndPRs(
		input.Worktrees,
		input.Issues,
		input.PullRequests,
	)

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

// correlateWorktreesWithIssuesAndPRs links worktrees to issues/PRs based on branch names.
func correlateWorktreesWithIssuesAndPRs(
	worktrees []domain.Worktree,
	issues []domain.Issue,
	prs []domain.PullRequest,
) []domain.WorkItem {
	workItems := make([]domain.WorkItem, 0) // Always non-nil

	for _, worktree := range worktrees {
		// Rule 1: Exact branch match with PR
		if pr, ok := exactBranchMatch(worktree.Branch, prs); ok {
			item := domain.WorkItem{
				ID:              worktree.Branch,
				CreatedAt:       time.Now().Unix(),
				UpdatedAt:       time.Now().Unix(),
				LinkedPR:        &pr,
			}
			workItems = append(workItems, item)
			continue
		}

		// Rule 2: Branch containment (worktree branch contains PR branch as substring)
		if pr, ok := containmentMatch(worktree.Branch, prs); ok {
			item := domain.WorkItem{
				ID:              worktree.Branch,
				CreatedAt:       time.Now().Unix(),
				UpdatedAt:       time.Now().Unix(),
				LinkedPR:        &pr,
			}
			workItems = append(workItems, item)
			continue
		}

		// Rule 3: Orphan worktrees appear as standalone workitems ONLY IF there's at least one PR or Issue
		// If there are no PRs/issues, orphan worktrees should not create work items
		if len(prs) > 0 || len(issues) > 0 {
			existing := false
			for _, wi := range workItems {
				if wi.ID == worktree.Branch {
					existing = true
					break
				}
			}
			if !existing {
				item := domain.WorkItem{
					ID:              worktree.Branch,
					CreatedAt:       time.Now().Unix(),
					UpdatedAt:       time.Now().Unix(),
					LinkedPR:        nil,
					LinkedIssue:     nil,
				}
				workItems = append(workItems, item)
			}
		}
	}

	return workItems
}

// exactBranchMatch returns a PR that has an exact branch name match with the worktree.
func exactBranchMatch(worktreeBranch string, prs []domain.PullRequest) (domain.PullRequest, bool) {
	for _, pr := range prs {
		if pr.Branch == worktreeBranch {
			return pr, true
		}
	}
	return domain.PullRequest{}, false
}

// containmentMatch returns a PR where the worktree branch contains the PR branch as substring.
func containmentMatch(worktreeBranch string, prs []domain.PullRequest) (domain.PullRequest, bool) {
	for _, pr := range prs {
		if strings.Contains(worktreeBranch, pr.Branch) {
			return pr, true
		}
	}
	return domain.PullRequest{}, false
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
	copy(cloned, items)
	return cloned
}

func cloneIssues(items []domain.Issue) []domain.Issue {
	cloned := make([]domain.Issue, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].Labels = append([]string(nil), item.Labels...)
		cloned[i].Assignees = append([]string(nil), item.Assignees...)
	}
	return cloned
}

func clonePullRequests(items []domain.PullRequest) []domain.PullRequest {
	cloned := make([]domain.PullRequest, len(items))
	for i, item := range items {
		cloned[i] = item
		cloned[i].Labels = append([]string(nil), item.Labels...)
		cloned[i].Assignees = append([]string(nil), item.Assignees...)
	}
	return cloned
}

func cloneSessions(items []domain.Session) []domain.Session {
	cloned := make([]domain.Session, len(items))
	copy(cloned, items)
	return cloned
}
