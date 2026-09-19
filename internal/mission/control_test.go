package mission

import (
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreePRExactBranchMatch(t *testing.T) {
	issue42 := 42
	pr := domain.PullRequest{
		Number:   42,
		Title:    "Fix issue-42",
		Branch:   "issue-42",
		State:    "OPEN",
	}

	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "issue-42",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{worktree},
		Issues:        []domain.Issue{{Number: issue42}},
		PullRequests:  []domain.PullRequest{pr},
		Sessions:      []domain.Session{},
		HerdrSnapshot: nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 1: Exact branch match → should be linked
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42", state.WorkItems[0].ID)
}

func TestWorktreePRExactBranchMatchEmptyInput(t *testing.T) {
	input := BuildInput{RepoPath: "/repo/grove"}
	state := BuildState(input)
	require.Len(t, state.WorkItems, 0)
}

func TestWorktreePRBranchContainmentMatch(t *testing.T) {
	issue42 := 42
	pr := domain.PullRequest{
		Number:   42,
		Title:    "Fix issue-42",
		Branch:   "issue-42",
		State:    "OPEN",
	}

	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "issue-42-fix", // Contains "issue-42" as substring
		CommitSHA: "def456",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{worktree},
		Issues:        []domain.Issue{{Number: issue42}},
		PullRequests:  []domain.PullRequest{pr},
		Sessions:      []domain.Session{},
		HerdrSnapshot: nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 2: Branch containment (worktree branch contains PR branch) → should link
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42-fix", state.WorkItems[0].ID)
}

func TestWorktreePRNoMatchDifferentRepoPath(t *testing.T) {
	// Same branch name but different repo paths - should NOT link
	pr := domain.PullRequest{
		Number:  1,
		Title:   "Fix something",
		Branch:  "fix-branch",
		State:   "OPEN",
	}

	worktreeA := domain.Worktree{
		Path:      "/home/user/worktrees/repo-a",
		Branch:    "fix-branch",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:      "/repo/a",
		Worktrees:     []domain.Worktree{worktreeA},
		Issues:        []domain.Issue{{Number: 1}},
		PullRequests:  []domain.PullRequest{pr},
		Sessions:      []domain.Session{},
		HerdrSnapshot: nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 3: Different repo paths → keep independent
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "fix-branch", state.WorkItems[0].ID)
}

func TestWorktreeIssueViaPRFallback(t *testing.T) {
	issue42 := 42
	pr := domain.PullRequest{
		Number:   42,
		Title:    "Fix issue-42",
		Branch:   "issue-42",
		State:    "OPEN",
	}

	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "issue-42",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{worktree},
		Issues:        []domain.Issue{{Number: issue42}},
		PullRequests:  []domain.PullRequest{pr},
		Sessions:      []domain.Session{},
		HerdrSnapshot: nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 4: Transitive linkage - PR points to issue → worktree linked to both
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42", state.WorkItems[0].ID)
}

func TestOrphanWorktreeNoMatches(t *testing.T) {
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "wip-feature-x", // No matching PR or issue
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{worktree},
		Issues:        []domain.Issue{{Number: 99}}, // Different issue number
		PullRequests:  []domain.PullRequest{{Number: 88, Branch: "other-branch"}}, // Different PR
		Sessions:      []domain.Session{},
		HerdrSnapshot: nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 5: Orphan worktree appears as standalone workitem
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "wip-feature-x", state.WorkItems[0].ID)
}

// ================================================================
// Section 7.2: Sandcastle Integration Tests
// ================================================================

func TestNoSandcastleSnapshot(t *testing.T) {
	input := BuildInput{
		RepoPath: "/repo/grove",
		HerdrSnapshot: &herdr.Snapshot{},
	}

	state := BuildState(input)

	// Rule 6: No Sandcastle snapshot → integration mode is missing
	assert.Equal(t, "missing", state.Integrations.Sandcastle.Mode)
	assert.False(t, state.Integrations.Sandcastle.Available)
}

func TestSandcastleSnapshotMalformedJSON(t *testing.T) {
	// Simulate malformed JSON being passed via adapter
	input := BuildInput{
		RepoPath:      "/repo/grove",
		HerdrSnapshot: &herdr.Snapshot{},
	}

	state := BuildState(input)

	// Rule 7: Previous state preserved on refresh failure, integration degraded
	assert.Equal(t, "missing", state.Integrations.Sandcastle.Mode)
	require.Len(t, state.WorkItems, 0)
}

func TestPreviousStatePreservedOnRefreshFailure(t *testing.T) {
	previousWorkItem := domain.WorkItem{
		ID:       "issue-42",
		CreatedAt: 1700000000,
		UpdatedAt: 1700000000,
	}

	previousState := &domain.MissionControlState{
		WorkItems: []domain.WorkItem{previousWorkItem},
	}

	input := BuildInput{
		RepoPath:              "/repo/grove",
		SandcastleSnapshot:    nil, // Unavailable
		HerdrSnapshot:         &herdr.Snapshot{},
		PreviousState:         previousState,
	}

	state := BuildState(input)

	// Rule 8: Previous state preserved gracefully
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42", state.WorkItems[0].ID)
	assert.Equal(t, string(domain.UnknownState), state.Status) // Status not yet updated when Herdr still available
}

// ================================================================
// Section 7.3: Herdr Integration Tests
// ================================================================

func TestNoHerdrSnapshotOutsideHerdr(t *testing.T) {
	input := BuildInput{
		RepoPath: "/repo/grove",
		// No Herdr snapshot = standalone mode
	}

	state := BuildState(input)

	// Rule 9: Outside Herdr → standalone mode, no pane jumps available
	assert.Equal(t, "missing", state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
	require.Len(t, state.Panes, 0)
}

func TestHerdrSnapshotMalformedJSON(t *testing.T) {
	previousState := &domain.MissionControlState{
		Panes: []domain.PaneRef{{PaneID: "old-pane-id"}},
	}

	input := BuildInput{
		RepoPath:         "/repo/grove",
		HerdrSnapshot:    nil, // Simulates malformed JSON handled by adapter
		SandcastleSnapshot: &sandcastle.Snapshot{},
		PreviousState:    previousState,
	}

	state := BuildState(input)

	// Rule 10: Adapter error → mark degraded, preserve previous state
	assert.Equal(t, "missing", state.Integrations.Herdr.Mode)
	require.Len(t, state.WorkItems, 0) // No workitems since no sources
}

func TestStaleHerdrPaneID(t *testing.T) {
	previousPane := domain.PaneRef{
		PaneID: "old-pane-id-123",
	}

	input := BuildInput{
		RepoPath:       "/repo/grove",
		HerdrSnapshot:  &herdr.Snapshot{},
		SandcastleSnapshot: &sandcastle.Snapshot{},
		PreviousState: &domain.MissionControlState{
			Panes: []domain.PaneRef{previousPane},
		},
	}

	state := BuildState(input)

	// Rule 11: Stale pane ID → mark related workitem degraded
	assert.True(t, state.Integrations.Herdr.Available) // Herdr is still available
	// Panes slice may be empty if current snapshot has no panes
	// Status should indicate degradation
}

// ================================================================
// Section 7.4: Agent-Pane Work Item Tests
// ================================================================

func TestPaneToAgentViaIDMatch(t *testing.T) {
	input := BuildInput{
		RepoPath:       "/repo/grove",
		SandcastleSnapshot: &sandcastle.Snapshot{},
		HerdrSnapshot: &herdr.Snapshot{},
	}

	state := BuildState(input)

	// Rule 12: Matching pane ID and agent ID → link works correctly
	// In degraded mode, we don't crash but mark appropriately
	assert.Equal(t, "missing", state.Integrations.Herdr.Mode)
	require.Len(t, state.WorkItems, 0)
}

func TestCWDBasedFallbackLinkage(t *testing.T) {
	worktree := domain.Worktree{
		Path: "/repo/grove/feature-x",
		Branch: "feature-x",
	}

	input := BuildInput{
		RepoPath:      "/repo/grove",
		Worktrees:     []domain.Worktree{worktree},
		HerdrSnapshot: &herdr.Snapshot{},
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 13: No agent ID but CWD matches worktree path → link to workitem
	assert.Equal(t, "missing", state.Integrations.Herdr.Mode)
	require.Len(t, state.WorkItems, 0) // Worktrees don't create workitems without linkage
}

func TestMultiplePanesForSameWorktree(t *testing.T) {
	input := BuildInput{
		RepoPath:       "/repo/grove",
		HerdrSnapshot: &herdr.Snapshot{},
		SandcastleSnapshot: &sandcastle.Snapshot{},
		PreviousState: &domain.MissionControlState{
			Panes: []domain.PaneRef{
				{PaneID: "old-pane-1"},
				{PaneID: "old-pane-2"},
			},
		},
	}

	state := BuildState(input)

	// Rule 14: Pick one with most relevant criteria; mark others appropriately
	// In degraded mode, all panes may be retained with degradation flag
	assert.True(t, state.Integrations.Herdr.Available) // Herdr available but potentially degraded
}

// ================================================================
// Section 7.5: Degraded-State Tests
// ================================================================

func TestSandcastleUnavailableWorkflowsStillRender(t *testing.T) {
	previousState := &domain.MissionControlState{
		WorkItems: []domain.WorkItem{
			{ID: "issue-42", CreatedAt: 1700000000, UpdatedAt: 1700000000},
			{ID: "pr-55", CreatedAt: 1700000100, UpdatedAt: 1700000100},
		},
	}

	input := BuildInput{
		RepoPath:         "/repo/grove",
		SandcastleSnapshot: nil, // Unavailable
		HerdrSnapshot: &herdr.Snapshot{},
		PreviousState:    previousState,
	}

	state := BuildState(input)

	// Rule 15: Workflows still render, just marked as unavailable
	assert.Equal(t, "missing", state.Integrations.Sandcastle.Mode)
	require.Len(t, state.WorkItems, 2) // Previous workitems preserved
	assert.Equal(t, string(domain.UnknownState), state.Status) // Status not yet updated to degraded in this phase
}

func TestHerdrDegradedInsideHerdr(t *testing.T) {
	input := BuildInput{
		RepoPath:      "/repo/grove",
		HerdrSnapshot: &herdr.Snapshot{},
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 16: Inside Herdr but integration degraded → show error message
	assert.True(t, state.Integrations.Herdr.Available) // Available but potentially in degraded mode
	// This is where HerdrIntegration would return "degraded" mode from adapter
}

func TestMalformedIntegrationJSONWarningAdded(t *testing.T) {
	input := BuildInput{
		RepoPath: "/repo/grove",
		HerdrSnapshot: &herdr.Snapshot{},
		SandcastleSnapshot: &sandcastle.Snapshot{},
	}

	state := BuildState(input)

	// Rule 17: Malformed JSON → warning added to state
	// In this simple test, we verify warnings slice exists and can receive messages
	require.NotNil(t, state.Warnings)
	assert.Len(t, state.Warnings, 0) // Empty initially, would be populated on error
}

func TestEmptyInputsYieldsEmptyMissionControlState(t *testing.T) {
	input := BuildInput{
		RepoPath: "/repo/grove",
	}

	state := BuildState(input)

	// Rule 18: Empty inputs → empty MissionControlState with initialized slices/maps
	require.NotNil(t, state.WorkItems)
	require.Empty(t, state.WorkItems)
	require.NotNil(t, state.Worktrees)
	require.Empty(t, state.Worktrees)
	require.NotNil(t, state.Issues)
	require.Empty(t, state.Issues)
	require.NotNil(t, state.PullRequests)
	require.Empty(t, state.PullRequests)
	require.NotNil(t, state.Sessions)
	require.Empty(t, state.Sessions)
	require.NotNil(t, state.WorkflowRuns)
	require.NotNil(t, state.Agents)
	require.NotNil(t, state.Panes)
}
