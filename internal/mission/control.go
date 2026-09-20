package mission

import (
	"strconv"
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
	InsideHerdr        bool
}

// BuildState creates a complete mission-control state from the available
// sources. Phase 2 adds correlation and previous-state recovery.
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
			Herdr:      herdrIntegration(input.HerdrSnapshot, input.InsideHerdr),
			Sandcastle: sandcastleIntegration(input.SandcastleSnapshot),
			GitHub:     missingIntegration(), // GitHub not connected to this implementation
		},
		Warnings:  []domain.Warning{},
		UpdatedAt: input.Now,

		Correlations: domain.Correlations{
			WorktreeToPR:       make(map[string]string),
			WorktreeToIssue:    make(map[string]string),
			WorkflowToWorktree: make(map[string]string),
			AgentToWorkflow:    make(map[string]string),
			PaneToAgent:        make(map[string]string),
		},
	}

	state.WorkItems = correlateAll(input.Worktrees, input.Issues, input.PullRequests, wfs, ags, pns)

	// Preserve previous state WorkItems when Sandcastle is unavailable
	if input.SandcastleSnapshot == nil && input.PreviousState != nil {
		if len(input.PreviousState.WorkItems) > 0 {
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
		}
		if len(input.PreviousState.WorkflowRuns) > 0 {
			state.WorkflowRuns = append(state.WorkflowRuns, input.PreviousState.WorkflowRuns...)
		}
	}

	// Append warnings for unavailable integrations
	if input.SandcastleSnapshot == nil {
		msg := "Sandcastle unavailable"
		if input.PreviousState != nil {
			msg += "; previous state preserved"
		}
		state.Warnings = append(state.Warnings, domain.Warning{
			Message: msg,
			Level:   "warning",
		})
	}

	// Populate correlations from freshly correlated items
	branchToPath := make(map[string]string, len(input.Worktrees))
	for _, wt := range input.Worktrees {
		branchToPath[wt.Branch] = wt.Path
	}
	for i := range state.WorkItems {
		wi := &state.WorkItems[i]
		// Link issues to work items that have a matching PR number
		if wi.LinkedPR != nil {
			for _, issue := range input.Issues {
				if issue.Number == wi.LinkedPR.Number {
					wi.LinkedIssue = &issue
					break
				}
			}
		}
		path := branchToPath[wi.ID]
		if path == "" {
			path = wi.ID
		}
		if wi.LinkedPR != nil {
			state.Correlations.WorktreeToPR[path] = strconv.Itoa(wi.LinkedPR.Number)
		}
		if wi.LinkedIssue != nil {
			state.Correlations.WorktreeToIssue[path] = strconv.Itoa(wi.LinkedIssue.Number)
		}
	}

	// Mark stale pane IDs on work items
	markStalePaneIDs(&state, input.HerdrSnapshot)

	// Append warning for degraded Herdr inside Herdr
	if state.Integrations.Herdr.Mode == "degraded" {
		state.Warnings = append(state.Warnings, domain.Warning{
			Message: "Herdr unavailable while running inside Herdr; pane jumps disabled",
			Level:   "warning",
		})
	}

	// Determine top-level status when all integrations are unavailable
	if !state.Integrations.Herdr.Available && !state.Integrations.Sandcastle.Available {
		state.Status = domain.DegradedState
	}

	return state
}

// herdrIntegration evaluates Herdr integration status.
func herdrIntegration(snapshot *herdr.Snapshot, insideHerdr bool) domain.ExternalIntegration {
	if snapshot == nil {
		if insideHerdr {
			return domain.ExternalIntegration{
				Mode:      "degraded",
				Available: false,
			}
		}
		return missingIntegration()
	}

	integration := domain.ExternalIntegration{
		Mode:    snapshot.Integration.Mode,
		Enabled: snapshot.Integration.Enabled,
	}

	if integration.Mode == "" {
		integration.Available = true
		integration.Enabled = true
		integration.Mode = "missing"
	} else {
		integration.Available = true
		integration.Enabled = true
	}

	return integration
}

// sandcastleIntegration evaluates Sandcastle integration status.
// Returns a new Integration struct to avoid mutating caller's snapshot data.
func sandcastleIntegration(snapshot *sandcastle.Snapshot) domain.ExternalIntegration {
	if snapshot == nil {
		return missingIntegration()
	}

	integration := domain.ExternalIntegration{
		Mode:    snapshot.Integration.Mode,
		Enabled: snapshot.Integration.Enabled,
	}

	if integration.Mode == "" {
		integration.Available = true
		integration.Enabled = true
		integration.Mode = "missing"
	} else {
		integration.Available = true
		integration.Enabled = true
	}

	return integration
}

func missingIntegration() domain.ExternalIntegration {
	return domain.ExternalIntegration{Mode: integrationModeMissing}
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

// markStalePaneIDs marks work items that reference pane IDs not present in
// the current Herdr snapshot. Phase 3 stub: fully functional once WorkItem
// PaneRef linkage is established upstream.
func markStalePaneIDs(state *domain.MissionControlState, herdrSnapshot *herdr.Snapshot) {
	if herdrSnapshot == nil {
		return
	}
	currentPanes := make(map[string]struct{}, len(herdrSnapshot.Panes))
	for _, p := range herdrSnapshot.Panes {
		currentPanes[p.PaneID] = struct{}{}
	}
	for i := range state.WorkItems {
		if state.WorkItems[i].PaneRef == nil {
			continue
		}
		if _, ok := currentPanes[state.WorkItems[i].PaneRef.PaneID]; !ok {
			state.WorkItems[i].Degraded = true
			state.WorkItems[i].DegradedReason = "stale pane ID"
		}
	}
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
		cloned[i].WorkspaceID = clonePointer(items[i].WorkspaceID)
		cloned[i].TabID = clonePointer(items[i].TabID)
		cloned[i].PaneID = clonePointer(items[i].PaneID)
		cloned[i].WorkflowRunID = clonePointer(items[i].WorkflowRunID)
		cloned[i].DegradedReason = clonePointer(items[i].DegradedReason)
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
		if pr, ok := exactBranchMatch(worktree.Branch, prs); ok {
			item := domain.WorkItem{
				ID:        worktree.Branch,
				CreatedAt: time.Now().Unix(),
				UpdatedAt: time.Now().Unix(),
				LinkedPR:  &pr,
			}
			workItems = append(workItems, item)
			continue
		}

		if pr, ok := containmentMatch(worktree.Branch, prs); ok {
			item := domain.WorkItem{
				ID:        worktree.Branch,
				CreatedAt: time.Now().Unix(),
				UpdatedAt: time.Now().Unix(),
				LinkedPR:  &pr,
			}
			workItems = append(workItems, item)
			continue
		}

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
					ID:        worktree.Branch,
					CreatedAt: time.Now().Unix(),
					UpdatedAt: time.Now().Unix(),
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
