package mission

import (
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ================================================================
// Issue #170: Worktree-to-PR correlation — Rules 3 & 4 + tiebreaker
// ================================================================

func TestWorktreePRLinkedMetadataMatch(t *testing.T) {
	// AC-6 Rule 3: Worktree.LinkedPR metadata → PR match
	// When no branch match exists but the worktree has LinkedPR set,
	// the builder should match by PR number.
	pr := domain.PullRequest{
		Number: 99,
		Title:  "Fix bug",
		Branch: "fix-something-else",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:       "/repo/grove",
		Branch:     "feature-xyz",
		CommitSHA:  "abc123",
		IsClean:    true,
		LinkedPR:   &domain.PullRequest{Number: 99},
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{{Number: 42}},
		PullRequests:      []domain.PullRequest{pr},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "feature-xyz", state.WorkItems[0].ID)
	require.NotNil(t, state.WorkItems[0].LinkedPR)
	assert.Equal(t, 99, state.WorkItems[0].LinkedPR.Number)
}

func TestWorktreePRPathFallbackMatch(t *testing.T) {
	// AC-6 Rule 4: Best-effort path/name fallback
	// When no exact or containment match, check if worktree path
	// contains a PR branch name substring.
	pr := domain.PullRequest{
		Number: 55,
		Title:  "Add feature",
		Branch: "add-feature-xy",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove/worktrees/add-feature-xy-abc123",
		Branch:    "some-unique-branch",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{},
		PullRequests:      []domain.PullRequest{pr},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedPR)
	assert.Equal(t, 55, state.WorkItems[0].LinkedPR.Number)
}

func TestWorktreePRTiebreakerHighestNumberedWins(t *testing.T) {
	// AC-13: Two PRs share the same branch name; highest-numbered should win.
	pr1 := domain.PullRequest{
		Number: 10,
		Title:  "Old PR",
		Branch: "shared-branch",
		State:  "MERGED",
	}
	pr2 := domain.PullRequest{
		Number: 25,
		Title:  "New PR",
		Branch: "shared-branch",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "shared-branch",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{},
		PullRequests:      []domain.PullRequest{pr1, pr2},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedPR)
	assert.Equal(t, 25, state.WorkItems[0].LinkedPR.Number, "highest-numbered PR should win")
}

// ================================================================
// Issue #170: Worktree-to-Issue correlation — 4 rules
// ================================================================

func TestWorktreeIssueByLinkedMetadata(t *testing.T) {
	// AC-7 Rule 1: worktree.LinkedIssue.Number matches issue
	issue42 := domain.Issue{
		Number: 42,
		Title:  "Fix the bug",
	}
	worktree := domain.Worktree{
		Path:        "/repo/grove",
		Branch:      "feature-xyz",
		CommitSHA:   "abc123",
		IsClean:     true,
		LinkedIssue: &domain.Issue{Number: 42},
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{issue42},
		PullRequests:      []domain.PullRequest{},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue)
	assert.Equal(t, 42, state.WorkItems[0].LinkedIssue.Number)
}

func TestWorktreeIssueByBranchPatternIssuePrefix(t *testing.T) {
	// AC-7 Rule 2: branch "issue-42" → issue #42
	issue42 := domain.Issue{
		Number: 42,
		Title:  "Fix the bug",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "issue-42",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{issue42},
		PullRequests:      []domain.PullRequest{},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue)
	assert.Equal(t, 42, state.WorkItems[0].LinkedIssue.Number)
}

func TestWorktreeIssueByBranchPatternFeatPrefix(t *testing.T) {
	// AC-7 Rule 2: branch "feat-42-fix-the-bug" → issue #42
	issue42 := domain.Issue{
		Number: 42,
		Title:  "Fix the bug",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "feat-42-fix-the-bug",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{issue42},
		PullRequests:      []domain.PullRequest{},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue)
	assert.Equal(t, 42, state.WorkItems[0].LinkedIssue.Number)
}

func TestWorktreeIssueByBranchPatternBareNumber(t *testing.T) {
	// AC-7 Rule 2: branch "42-fix-something" → issue #42
	issue42 := domain.Issue{
		Number: 42,
		Title:  "Fix the bug",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "42-fix-something",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{issue42},
		PullRequests:      []domain.PullRequest{},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue)
	assert.Equal(t, 42, state.WorkItems[0].LinkedIssue.Number)
}

func TestWorktreeIssueByLinkedPRBody(t *testing.T) {
	// AC-7 Rule 3: PR body contains "closes #42" → issue #42 linked to worktree
	issue42 := domain.Issue{
		Number: 42,
		Title:  "Fix the bug",
	}
	pr := domain.PullRequest{
		Number: 100,
		Title:  "Fix bug via PR",
		Branch: "fix-bug",
		Body:   "This PR closes #42",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "fix-bug",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{issue42},
		PullRequests:      []domain.PullRequest{pr},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedPR, "should have linked PR")
	require.NotNil(t, state.WorkItems[0].LinkedIssue, "should have linked issue from PR body")
	assert.Equal(t, 42, state.WorkItems[0].LinkedIssue.Number)
}

func TestWorktreeIssueByWorkflowMetadata(t *testing.T) {
	// AC-7 Rule 4: workflow IssueNumber → matches issue
	issue42 := domain.Issue{
		Number: 42,
		Title:  "Fix the bug",
	}
	num42 := 42
	workflow := domain.WorkflowRunRef{
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		WorktreePath: "/repo/grove",
		Branch:       "feature-xyz",
		Status:       "running",
		IssueNumber:  &num42,
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:        []domain.Issue{issue42},
		PullRequests:  []domain.PullRequest{},
		Sessions:      []domain.Session{},
		HerdrSnapshot: nil,
		SandcastleSnapshot: &sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{workflow}},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue, "workflow IssueNumber should link issue")
	assert.Equal(t, 42, state.WorkItems[0].LinkedIssue.Number)
}

// ================================================================
// Issue #170: Sandcastle Workflow-to-Worktree correlation
// ================================================================

func TestWorkflowMatchesWorktreeByPath(t *testing.T) {
	// AC-8 Rule 1: Workflow WorktreePath matches worktree path
	workflow := domain.WorkflowRunRef{
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		WorktreePath: "/repo/grove",
		Branch:       "feature-xyz",
		Status:       "running",
	}

	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:    []domain.Issue{},
		PullRequests: []domain.PullRequest{
			{Number: 1, Title: "PR", Branch: "feature-xyz", State: "OPEN"},
		},
		Sessions:          []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{workflow}},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.Len(t, state.WorkItems[0].LinkedWorkflows, 1)
	assert.Equal(t, "run-1", state.WorkItems[0].LinkedWorkflows[0].RunID)
}

func TestWorkflowMatchesWorktreeByRepoAndBranch(t *testing.T) {
	// AC-8 Rule 2: repo path + branch match
	workflow := domain.WorkflowRunRef{
		WorkflowID: "wf-2",
		RunID:      "run-2",
		Branch:     "feature-xyz",
		Status:     "completed",
	}

	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:    []domain.Issue{},
		PullRequests: []domain.PullRequest{
			{Number: 1, Title: "PR", Branch: "feature-xyz", State: "OPEN"},
		},
		Sessions:          []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{workflow}},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.Len(t, state.WorkItems[0].LinkedWorkflows, 1)
	assert.Equal(t, "run-2", state.WorkItems[0].LinkedWorkflows[0].RunID)
}

func TestWorkflowMatchesWorktreeByIssueNumber(t *testing.T) {
	// AC-8 Rule 3: Workflow IssueNumber matches worktree's linked issue
	issue42 := domain.Issue{Number: 42, Title: "Fix bug"}
	num42 := 42
	workflow := domain.WorkflowRunRef{
		WorkflowID:  "wf-3",
		RunID:       "run-3",
		Status:      "running",
		IssueNumber: &num42,
	}

	input := BuildInput{
		RepoPath: "/repo/grove",
		Worktrees: []domain.Worktree{
			{Path: "/repo/grove", Branch: "issue-42", CommitSHA: "abc123", IsClean: true},
		},
		Issues:            []domain.Issue{issue42},
		PullRequests:      []domain.PullRequest{},
		Sessions:          []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{workflow}},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue, "issue should be linked via branch pattern")
	require.Len(t, state.WorkItems[0].LinkedWorkflows, 1)
	assert.Equal(t, "run-3", state.WorkItems[0].LinkedWorkflows[0].RunID)
}

// ================================================================
// Issue #170: Agent-to-Workflow matching
// ================================================================

func TestAgentMatchesWorkflowByRunID(t *testing.T) {
	// AC-9: Agent WorkflowRunID matches workflow RunID
	workflow := domain.WorkflowRunRef{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Branch:     "feature-xyz",
		Status:     "running",
	}
	agent := domain.AgentRef{
		AgentID:       "agent-1",
		Name:          "coder",
		WorkflowRunID: "run-1",
		Status:        "running",
	}

	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:    []domain.Issue{},
		PullRequests: []domain.PullRequest{
			{Number: 1, Title: "PR", Branch: "feature-xyz", State: "OPEN"},
		},
		Sessions: []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{workflow},
			Agents:    []domain.AgentRef{agent},
		},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.Len(t, state.WorkItems[0].LinkedAgents, 1)
	assert.Equal(t, "agent-1", state.WorkItems[0].LinkedAgents[0].AgentID)
	assert.Equal(t, "run-1", state.WorkItems[0].LinkedAgents[0].WorkflowRunID)
}

// ================================================================
// Issue #170: Pane-to-Agent/Worktree matching
// ================================================================

func TestPaneMatchesAgentByID(t *testing.T) {
	// AC-10 Rule 1: Pane AgentID matches agent
	workflow := domain.WorkflowRunRef{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Branch:     "feature-xyz",
		Status:     "running",
	}
	agent := domain.AgentRef{
		AgentID:       "agent-1",
		Name:          "coder",
		WorkflowRunID: "run-1",
		Status:        "running",
	}
	pane := domain.PaneRef{
		PaneID:  "pane-1",
		AgentID: "agent-1",
		CWD:     "/repo/grove",
		Status:  "active",
	}

	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:    []domain.Issue{},
		PullRequests: []domain.PullRequest{
			{Number: 1, Title: "PR", Branch: "feature-xyz", State: "OPEN"},
		},
		Sessions: []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{workflow},
			Agents:    []domain.AgentRef{agent},
		},
		HerdrSnapshot: &herdr.Snapshot{
			Panes: []domain.PaneRef{pane},
		},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.Len(t, state.WorkItems[0].LinkedPanes, 1)
	assert.Equal(t, "pane-1", state.WorkItems[0].LinkedPanes[0].PaneID)
}

func TestPaneMatchesWorktreeByCWD(t *testing.T) {
	// AC-10 Rule 2: Pane CWD matches worktree path
	workflow := domain.WorkflowRunRef{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Branch:     "feature-xyz",
		Status:     "running",
	}
	agent := domain.AgentRef{
		AgentID:       "agent-1",
		Name:          "coder",
		WorkflowRunID: "run-1",
		Status:        "running",
	}
	pane := domain.PaneRef{
		PaneID: "pane-1",
		CWD:    "/repo/grove",
		Status: "active",
	}

	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:    []domain.Issue{},
		PullRequests: []domain.PullRequest{
			{Number: 1, Title: "PR", Branch: "feature-xyz", State: "OPEN"},
		},
		Sessions: []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{workflow},
			Agents:    []domain.AgentRef{agent},
		},
		HerdrSnapshot: &herdr.Snapshot{
			Panes: []domain.PaneRef{pane},
		},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.Len(t, state.WorkItems[0].LinkedPanes, 1)
	assert.Equal(t, "pane-1", state.WorkItems[0].LinkedPanes[0].PaneID)
}

// ================================================================
// Issue #170: Edge cases — nil/partial/orphan
// ================================================================

func TestNilSnapshotsNoPanic(t *testing.T) {
	// AC-11: Nil Herdr and nil Sandcastle snapshots must not panic
	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "main", CommitSHA: "abc123"}},
		Issues:    []domain.Issue{{Number: 1}},
	}

	state := BuildState(input)

	assert.Equal(t, domain.UnknownState, state.Status)
	require.NotNil(t, state.WorkItems)
	require.NotNil(t, state.WorkflowRuns)
	require.NotNil(t, state.Agents)
	require.NotNil(t, state.Panes)
}

func TestPartialDataWorkflowsOnlyNoPanic(t *testing.T) {
	// AC-12: Partial data (only workflows, no agents/panes) → degraded, not error
	workflow := domain.WorkflowRunRef{
		WorkflowID: "wf-1",
		RunID:      "run-1",
		Branch:     "feature-xyz",
		Status:     "running",
	}

	input := BuildInput{
		RepoPath:  "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "feature-xyz", CommitSHA: "abc123", IsClean: true}},
		Issues:    []domain.Issue{},
		PullRequests: []domain.PullRequest{
			{Number: 1, Title: "PR", Branch: "feature-xyz", State: "OPEN"},
		},
		Sessions:          []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{workflow}},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.Len(t, state.WorkItems[0].LinkedWorkflows, 1, "workflow should be linked")
	require.Empty(t, state.WorkItems[0].LinkedAgents)
	require.Empty(t, state.WorkItems[0].LinkedPanes)
}

func TestOneWorktreeTwoPRsSameBranchOneWorkItem(t *testing.T) {
	// AC-13: One worktree matched by two PRs with same branch → one WorkItem
	pr1 := domain.PullRequest{
		Number: 10,
		Title:  "Old PR",
		Branch: "feature-xyz",
		State:  "MERGED",
	}
	pr2 := domain.PullRequest{
		Number: 25,
		Title:  "New PR",
		Branch: "feature-xyz",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "feature-xyz",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{worktree},
		Issues:            []domain.Issue{},
		PullRequests:      []domain.PullRequest{pr1, pr2},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1, "should produce exactly one work item for one worktree")
	require.NotNil(t, state.WorkItems[0].LinkedPR)
	assert.Equal(t, 25, state.WorkItems[0].LinkedPR.Number, "highest-numbered PR should win")
}

func TestUnmatchedIssueStandaloneWorkItem(t *testing.T) {
	// AC-14: Unmatched issue appears as standalone WorkItem
	issue99 := domain.Issue{
		Number: 99,
		Title:  "Orphan issue",
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{},
		Issues:            []domain.Issue{issue99},
		PullRequests:      []domain.PullRequest{},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedIssue)
	assert.Equal(t, 99, state.WorkItems[0].LinkedIssue.Number)
}

func TestUnmatchedPRStandaloneWorkItem(t *testing.T) {
	// AC-14: Unmatched PR appears as standalone WorkItem
	pr := domain.PullRequest{
		Number: 55,
		Title:  "Orphan PR",
		Branch: "orphan-branch",
		State:  "OPEN",
	}

	input := BuildInput{
		RepoPath:          "/repo/grove",
		Worktrees:         []domain.Worktree{},
		Issues:            []domain.Issue{},
		PullRequests:      []domain.PullRequest{pr},
		Sessions:          []domain.Session{},
		HerdrSnapshot:     nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	require.NotNil(t, state.WorkItems[0].LinkedPR)
	assert.Equal(t, 55, state.WorkItems[0].LinkedPR.Number)
}

// ================================================================
// Issue #170: Full integration test
// ================================================================

func TestFullIntegrationWorktreePRIssueWorkflowAgentPane(t *testing.T) {
	// AC-1: All data sources → single coherent WorkItem
	issue42 := domain.Issue{Number: 42, Title: "Fix the bug"}
	pr := domain.PullRequest{
		Number: 100,
		Title:  "Fix bug",
		Branch: "fix-bug",
		Body:   "This PR closes #42",
		State:  "OPEN",
	}
	workflow := domain.WorkflowRunRef{
		WorkflowID:   "wf-1",
		RunID:        "run-1",
		WorktreePath: "/repo/grove",
		Branch:       "fix-bug",
		Status:       "running",
	}
	agent := domain.AgentRef{
		AgentID:       "agent-1",
		Name:          "coder",
		WorkflowRunID: "run-1",
		Status:        "running",
	}
	pane := domain.PaneRef{
		PaneID:  "pane-1",
		AgentID: "agent-1",
		CWD:     "/repo/grove",
		Status:  "active",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "fix-bug",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{worktree},
		Issues:        []domain.Issue{issue42},
		PullRequests:  []domain.PullRequest{pr},
		Sessions:      []domain.Session{},
		SandcastleSnapshot: &sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{workflow},
			Agents:    []domain.AgentRef{agent},
		},
		HerdrSnapshot: &herdr.Snapshot{
			Panes: []domain.PaneRef{pane},
		},
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1, "should produce exactly one coherent work item")
	wi := state.WorkItems[0]

	assert.Equal(t, "fix-bug", wi.ID)
	require.NotNil(t, wi.LinkedPR, "PR should be linked by exact branch")
	assert.Equal(t, 100, wi.LinkedPR.Number)
	require.NotNil(t, wi.LinkedIssue, "issue should be linked via PR body or branch pattern")
	assert.Equal(t, 42, wi.LinkedIssue.Number)
	require.Len(t, wi.LinkedWorkflows, 1, "workflow should be linked by path/branch")
	assert.Equal(t, "run-1", wi.LinkedWorkflows[0].RunID)
	require.Len(t, wi.LinkedAgents, 1, "agent should be linked to workflow")
	assert.Equal(t, "agent-1", wi.LinkedAgents[0].AgentID)
	require.Len(t, wi.LinkedPanes, 1, "pane should be linked to agent/worktree")
	assert.Equal(t, "pane-1", wi.LinkedPanes[0].PaneID)
}
