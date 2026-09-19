package mission

import (
	"strings"
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
// sources. Phase 2 adds correlation and previous-state recovery.
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
			GitHub:     missingIntegration(), // GitHub not connected to this implementation
		},
		Warnings:  []domain.Warning{},
		UpdatedAt: input.Now,

		Correlations: domain.Correlations{
			WorktreeToPR:      make(map[string]string),
			WorktreeToIssue:   make(map[string]string),
			WorkflowToWorktree: make(map[string]string),
			AgentToWorkflow:   make(map[string]string),
			PaneToAgent:       make(map[string]string),
		},
	}

	state.WorkItems = correlateWorktreesWithIssuesAndPRs(input.Worktrees, input.Issues, input.PullRequests)

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
		sc := sandcastleIntegration(input.SandcastleSnapshot)
		hr := herdrIntegration(input.HerdrSnapshot, input.PreviousState)
		if sc.Available == false && hr.Available == false {
			state.Status = domain.DegradedState
		}
	}

	return state
}

// herdrIntegration evaluates Herdr integration status.
//
// Phase 2: previousState parameter is unused (reserved for Phase 3 correlation recovery)
// Returns a new Integration struct to avoid mutating caller's snapshot data.
func herdrIntegration(snapshot *herdr.Snapshot, previousState *domain.MissionControlState) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration() // Integration not available at all
	}

	// Read original values before applying any modifications to avoid side effects
	integration := domain.ExternalIntegration{
		Mode:    snapshot.Integration.Mode,
		Enabled: snapshot.Integration.Enabled,
	}

	// Snapshot exists - integration is available
	if integration.Mode == "" {
		integration.Available = true
		integration.Enabled = true
		integration.Mode = "missing" // Mode not determined yet - will be filled by adapter
	} else {
		// Use existing mode from snapshot (e.g., "available", "degraded")
		integration.Available = true
		integration.Enabled = true
	}

	return integration
}

// sandcastleIntegration evaluates Sandcastle integration status.
// Returns a new Integration struct to avoid mutating caller's snapshot data.
func sandcastleIntegration(snapshot *sandcastle.Snapshot) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration() // Integration not available at all
	}

	// Read original values before applying any modifications to avoid side effects
	integration := domain.ExternalIntegration{
		Mode:    snapshot.Integration.Mode,
		Enabled: snapshot.Integration.Enabled,
	}

	// Snapshot exists - integration is available
	if integration.Mode == "" {
		integration.Available = true
		integration.Enabled = true
		integration.Mode = "missing" // Mode not determined yet - will be filled by adapter
	} else {
		// Use existing mode from snapshot (e.g., "available", "degraded")
		integration.Available = true
		integration.Enabled = true
	}

	return integration
}

func missingIntegration() domain.ExternalIntegration {
	return domain.ExternalIntegration{Mode: integrationModeMissing}
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

// correlateWorktreesWithIssuesAndPRs links worktrees to issues/PRs based on branch names.
func correlateWorktreesWithIssuesAndPRs(
	worktrees []domain.Worktree,
	issues []domain.Issue,
	prs []domain.PullRequest,
) []domain.WorkItem {
	workItems := make([]domain.WorkItem, 0) // Always non-nil

	for _, worktree := range worktrees {
		// Rule 1: Exact branch match with PR (highest precedence)
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
