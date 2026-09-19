package mission

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/m00nk0d3/grove/internal/domain"
)

// branchIssuePatternRe extracts an issue number from branch names like
// "issue-42", "feat-42-title", "42-something", "fix-42", "hotfix-123-title".
// Captures the leading digits as group 1.
var branchIssuePatternRe = regexp.MustCompile(`(?i)^(?:issue[-_]|fix[-_]|feat[-_]|feature[-_]|chore[-_]|hotfix[-_])?(\d+)(?:[-_].*)?$`)

// prBodyIssueRefRe extracts issue numbers from PR bodies via keywords
// "closes", "fixes", "resolves".
var prBodyIssueRefRe = regexp.MustCompile(`(?i)(?:closes|fixes|resolves)\s+#(\d+)`)

// correlateAll links worktrees to PRs, issues, workflows, agents, and panes,
// then creates standalone work items for unmatched entities.
func correlateAll(
	worktrees []domain.Worktree,
	issues []domain.Issue,
	prs []domain.PullRequest,
	workflows []domain.WorkflowRunRef,
	agents []domain.AgentRef,
	panes []domain.PaneRef,
) []domain.WorkItem {
	itemsByWorktree := make(map[string]*domain.WorkItem)

	// Phase 1: Match worktrees to PRs and issues.
	// Track per-branch issue links so Phase 2 can use them without mutating input.
	worktreeIssueLinks := make(map[string]*domain.Issue) // branch → linked issue
	for i := range worktrees {
		wt := &worktrees[i]
		item := &domain.WorkItem{
			ID: wt.Branch,
		}

		// --- PR matching (priority 1→4) ---
		item.LinkedPR = matchPRForWorktree(wt, prs)

		// --- Issue matching (priority 1→4) ---
		item.LinkedIssue = matchIssueForWorktree(wt, item.LinkedPR, issues, workflows)

		if item.LinkedIssue != nil {
			worktreeIssueLinks[wt.Branch] = item.LinkedIssue
		}

		itemsByWorktree[wt.Branch] = item
	}

	// Phase 2: Match workflows to worktrees.
	for _, wf := range workflows {
		wt := matchWorkflowToWorktree(wf, worktrees, issues, prs, worktreeIssueLinks)
		if wt == nil {
			continue
		}
		item, ok := itemsByWorktree[wt.Branch]
		if !ok {
			continue
		}
		item.LinkedWorkflows = append(item.LinkedWorkflows, wf)
	}

	// Phase 3: Match agents to workflows → attach to work items.
	workflowToWorktree := make(map[string]string) // RunID → worktree Branch
	for _, wf := range workflows {
		wt := matchWorkflowToWorktree(wf, worktrees, issues, prs, worktreeIssueLinks)
		if wt != nil {
			workflowToWorktree[wf.RunID] = wt.Branch
		}
	}
	for _, agent := range agents {
		branch, ok := workflowToWorktree[agent.WorkflowRunID]
		if !ok {
			continue
		}
		item, ok := itemsByWorktree[branch]
		if !ok {
			continue
		}
		item.LinkedAgents = append(item.LinkedAgents, agent)
	}

	// Phase 4: Match panes to agents or worktrees → attach to work items.
	agentToWorktree := make(map[string]string) // AgentID → worktree Branch
	for branch, item := range itemsByWorktree {
		for _, agent := range item.LinkedAgents {
			agentToWorktree[agent.AgentID] = branch
		}
	}
	for _, pane := range panes {
		branch := matchPaneToWorktree(pane, agentToWorktree, worktrees)
		if branch == "" {
			continue
		}
		item, ok := itemsByWorktree[branch]
		if !ok {
			continue
		}
		item.LinkedPanes = append(item.LinkedPanes, pane)
	}

	// Collect work items from the map, preserving stable ordering by worktree branch.
	hasOtherSources := len(prs) > 0 || len(issues) > 0
	result := make([]domain.WorkItem, 0, len(itemsByWorktree))
	for _, wt := range worktrees {
		if item, ok := itemsByWorktree[wt.Branch]; ok {
			// Orphan worktrees appear as standalone work items only when there
			// are other data sources (PRs or issues) to anchor them.
			if item.LinkedPR == nil && item.LinkedIssue == nil && !hasOtherSources {
				continue
			}
			result = append(result, *item)
		}
	}

	// Phase 5: Standalone work items for unmatched issues/PRs, only when
	// there are no worktrees to anchor them.
	if len(worktrees) == 0 {
		for _, issue := range issues {
			issue := issue
			result = append(result, domain.WorkItem{
				ID:          fmt.Sprintf("issue-%d", issue.Number),
				LinkedIssue: &issue,
			})
		}
		for _, pr := range prs {
			pr := pr
			result = append(result, domain.WorkItem{
				ID:       fmt.Sprintf("pr-%d", pr.Number),
				LinkedPR: &pr,
			})
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// PR matching: priorities 1→4
// ---------------------------------------------------------------------------

// matchPRForWorktree applies the four priority rules to find a PR for a worktree.
func matchPRForWorktree(wt *domain.Worktree, prs []domain.PullRequest) *domain.PullRequest {
	// Rule 1: Exact branch match (highest-numbered wins on ties).
	if pr := exactBranchMatchBest(wt.Branch, prs); pr != nil {
		return pr
	}

	// Rule 2: Containment match (worktree branch contains PR branch as substring).
	if pr := containmentMatchBest(wt.Branch, prs); pr != nil {
		return pr
	}

	// Rule 3: Existing metadata (worktree.LinkedPR).
	if pr := linkedMetadataMatch(wt, prs); pr != nil {
		return pr
	}

	// Rule 4: Path/name fallback.
	if pr := pathFallbackMatch(wt, prs); pr != nil {
		return pr
	}

	return nil
}

// exactBranchMatchBest returns the highest-numbered PR whose branch equals worktreeBranch.
func exactBranchMatchBest(worktreeBranch string, prs []domain.PullRequest) *domain.PullRequest {
	var best *domain.PullRequest
	for i := range prs {
		if prs[i].Branch == worktreeBranch {
			if best == nil || prs[i].Number > best.Number {
				best = &prs[i]
			}
		}
	}
	return best
}

// containmentMatchBest returns the highest-numbered PR whose branch is a substring
// of worktreeBranch. An exact match is excluded (rule 1 handles that).
func containmentMatchBest(worktreeBranch string, prs []domain.PullRequest) *domain.PullRequest {
	var best *domain.PullRequest
	for i := range prs {
		if prs[i].Branch == "" || prs[i].Branch == worktreeBranch {
			continue
		}
		if strings.Contains(worktreeBranch, prs[i].Branch) {
			if best == nil || prs[i].Number > best.Number {
				best = &prs[i]
			}
		}
	}
	return best
}

// linkedMetadataMatch uses worktree.LinkedPR to match by PR number.
func linkedMetadataMatch(wt *domain.Worktree, prs []domain.PullRequest) *domain.PullRequest {
	if wt.LinkedPR == nil {
		return nil
	}
	var best *domain.PullRequest
	for i := range prs {
		if prs[i].Number == wt.LinkedPR.Number {
			if best == nil || prs[i].Number > best.Number {
				best = &prs[i]
			}
		}
	}
	return best
}

// pathFallbackMatch checks if the worktree path contains a PR branch name.
func pathFallbackMatch(wt *domain.Worktree, prs []domain.PullRequest) *domain.PullRequest {
	lowerPath := strings.ToLower(wt.Path)
	var best *domain.PullRequest
	for i := range prs {
		if strings.Contains(lowerPath, strings.ToLower(prs[i].Branch)) {
			if best == nil || prs[i].Number > best.Number {
				best = &prs[i]
			}
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Issue matching: priorities 1→4
// ---------------------------------------------------------------------------

// matchIssueForWorktree applies the four priority rules to find an issue for a worktree.
func matchIssueForWorktree(wt *domain.Worktree, matchedPR *domain.PullRequest, issues []domain.Issue, workflows []domain.WorkflowRunRef) *domain.Issue {
	// Rule 1: Existing metadata (worktree.LinkedIssue).
	if issue := issueByMetadata(wt, issues); issue != nil {
		return issue
	}

	// Rule 2: Branch pattern matching.
	if issue := issueByBranchPattern(wt, issues); issue != nil {
		return issue
	}

	// Rule 3: PR linked to issue (PR body text).
	if issue := issueByLinkedPR(matchedPR, issues); issue != nil {
		return issue
	}

	// Rule 4: Workflow metadata references an issue.
	if issue := issueByWorkflowMetadata(wt, workflows, issues); issue != nil {
		return issue
	}

	return nil
}

// issueByMetadata checks worktree.LinkedIssue against the issues list.
func issueByMetadata(wt *domain.Worktree, issues []domain.Issue) *domain.Issue {
	if wt.LinkedIssue == nil {
		return nil
	}
	for i := range issues {
		if issues[i].Number == wt.LinkedIssue.Number {
			return &issues[i]
		}
	}
	return nil
}

// issueByBranchPattern extracts an issue number from the branch name.
func issueByBranchPattern(wt *domain.Worktree, issues []domain.Issue) *domain.Issue {
	if wt.Branch == "" {
		return nil
	}
	match := branchIssuePatternRe.FindStringSubmatch(wt.Branch)
	if match == nil {
		return nil
	}
	var num int
	fmt.Sscanf(match[1], "%d", &num)
	for i := range issues {
		if issues[i].Number == num {
			return &issues[i]
		}
	}
	return nil
}

// issueByLinkedPR extracts issue references from a PR body (closes #N / fixes #N / resolves #N).
func issueByLinkedPR(pr *domain.PullRequest, issues []domain.Issue) *domain.Issue {
	if pr == nil || pr.Body == "" {
		return nil
	}
	match := prBodyIssueRefRe.FindStringSubmatch(pr.Body)
	if match == nil {
		return nil
	}
	var num int
	fmt.Sscanf(match[1], "%d", &num)
	for i := range issues {
		if issues[i].Number == num {
			return &issues[i]
		}
	}
	return nil
}

// issueByWorkflowMetadata checks if any workflow referencing this worktree carries an issue number.
func issueByWorkflowMetadata(wt *domain.Worktree, workflows []domain.WorkflowRunRef, issues []domain.Issue) *domain.Issue {
	for _, wf := range workflows {
		if wf.IssueNumber == nil {
			continue
		}
		// Match workflow to worktree by path or branch.
		if wf.WorktreePath != "" && wf.WorktreePath == wt.Path {
			return findIssueByNumber(issues, *wf.IssueNumber)
		}
		if wf.Branch != "" && wf.Branch == wt.Branch {
			return findIssueByNumber(issues, *wf.IssueNumber)
		}
	}
	return nil
}

func findIssueByNumber(issues []domain.Issue, num int) *domain.Issue {
	for i := range issues {
		if issues[i].Number == num {
			return &issues[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Workflow → Worktree matching
// ---------------------------------------------------------------------------

// matchWorkflowToWorktree returns the worktree best matching a workflow, or nil.
func matchWorkflowToWorktree(
	wf domain.WorkflowRunRef,
	worktrees []domain.Worktree,
	issues []domain.Issue,
	prs []domain.PullRequest,
	worktreeIssueLinks map[string]*domain.Issue,
) *domain.Worktree {
	// Rule 1: Exact worktree path.
	if wf.WorktreePath != "" {
		for i := range worktrees {
			if worktrees[i].Path == wf.WorktreePath {
				return &worktrees[i]
			}
		}
	}

	// Rule 2: Repo path + branch match.
	if wf.Branch != "" {
		for i := range worktrees {
			if worktrees[i].Branch == wf.Branch {
				return &worktrees[i]
			}
		}
	}

	// Rule 3: Workflow's issue/PR number matches a worktree's linked issue/PR.
	if wf.IssueNumber != nil {
		for i := range worktrees {
			linkedIssue := worktreeIssueLinks[worktrees[i].Branch]
			if linkedIssue != nil && linkedIssue.Number == *wf.IssueNumber {
				return &worktrees[i]
			}
		}
	}
	if wf.PRNumber != nil {
		for i := range worktrees {
			if worktrees[i].LinkedPR != nil && worktrees[i].LinkedPR.Number == *wf.PRNumber {
				return &worktrees[i]
			}
		}
	}

	// Rule 4: Label overlap (best-effort).
	if len(wf.Labels) > 0 {
		for i := range worktrees {
			if branchContainsAnyLabel(worktrees[i].Branch, wf.Labels) {
				return &worktrees[i]
			}
		}
	}

	return nil
}

func branchContainsAnyLabel(branch string, labels []string) bool {
	lowerBranch := strings.ToLower(branch)
	for _, l := range labels {
		if strings.Contains(lowerBranch, strings.ToLower(l)) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Pane → Worktree matching
// ---------------------------------------------------------------------------

// matchPaneToWorktree returns the worktree branch a pane belongs to, or "".
func matchPaneToWorktree(
	pane domain.PaneRef,
	agentToWorktree map[string]string,
	worktrees []domain.Worktree,
) string {
	// Rule 1: Pane's agent is linked to a worktree.
	if pane.AgentID != "" {
		if branch, ok := agentToWorktree[pane.AgentID]; ok {
			return branch
		}
	}

	// Rule 2: Pane CWD matches a worktree path.
	if pane.CWD != "" {
		for _, wt := range worktrees {
			if wt.Path == pane.CWD {
				return wt.Branch
			}
		}
	}

	return ""
}
