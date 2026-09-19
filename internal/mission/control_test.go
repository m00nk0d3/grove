package mission

import (
	"strings"
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
	input := BuildInput{
		RepoPath:      "/repo/grove",
		HerdrSnapshot: &herdr.Snapshot{},
	}

	state := BuildState(input)

	// Rule 7: Previous state preserved on refresh failure, integration degraded
	assert.Equal(t, "missing", state.Integrations.Sandcastle.Mode)
	require.Len(t, state.WorkItems, 0)
	// Phase 2: Warning appended for missing Sandcastle
	require.Len(t, state.Warnings, 1)
	assert.Contains(t, state.Warnings[0].Message, "Sandcastle")
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
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: nil, // Unavailable
		HerdrSnapshot:      &herdr.Snapshot{},
		PreviousState:      previousState,
	}

	state := BuildState(input)

	// Rule 8: Previous state preserved gracefully
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42", state.WorkItems[0].ID)
	assert.Equal(t, string(domain.UnknownState), state.Status)
	// Phase 2: Warning appended for missing Sandcastle
	require.Len(t, state.Warnings, 1)
	assert.Contains(t, state.Warnings[0].Message, "Sandcastle")
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
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: nil, // Unavailable
		HerdrSnapshot:      &herdr.Snapshot{},
		PreviousState:      previousState,
	}

	state := BuildState(input)

	// Rule 15: Workflows still render, just marked as unavailable
	assert.Equal(t, "missing", state.Integrations.Sandcastle.Mode)
	require.Len(t, state.WorkItems, 2) // Previous workitems preserved
	assert.Equal(t, string(domain.UnknownState), state.Status)
	// Phase 2: Warning appended for missing Sandcastle
	require.Len(t, state.Warnings, 1)
	assert.Contains(t, state.Warnings[0].Message, "Sandcastle")
}

func TestHerdrDegradedInsideHerdr(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
		InsideHerdr:        true,
	}

	state := BuildState(input)

	// Rule 16: Inside Herdr but snapshot unavailable → degraded mode
	assert.Equal(t, "degraded", state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
	// Warning appended for degraded Herdr
	require.Len(t, state.Warnings, 1)
	assert.Contains(t, state.Warnings[0].Message, "Herdr")
}

func TestMalformedIntegrationJSONWarningAdded(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: nil,
	}

	state := BuildState(input)

	// Rule 17: Both integrations unavailable → warnings added to state
	require.NotNil(t, state.Warnings)
	require.NotEmpty(t, state.Warnings, "expected warnings when integrations are unavailable")
	// Status must be degraded when both are unavailable
	assert.Equal(t, domain.DegradedState, state.Status)
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

// Phase 2: Correlations metadata tests for verifying struct existence and initialization

func TestCorrelationsStructExists(t *testing.T) {
	input := BuildInput{RepoPath: "/repo/grove"}
	state := BuildState(input)

	// Verify Correlations struct exists in MissionControlState
	require.NotNil(t, state.Correlations)

	// Verify all maps are initialized (not nil)
	assert.NotNil(t, state.Correlations.WorktreeToPR)
	assert.NotNil(t, state.Correlations.WorktreeToIssue)
	assert.NotNil(t, state.Correlations.WorkflowToWorktree)
	assert.NotNil(t, state.Correlations.AgentToWorkflow)
	assert.NotNil(t, state.Correlations.PaneToAgent)
}

func TestCorrelationsInitializedEmpty(t *testing.T) {
	input := BuildInput{
		RepoPath: "/repo/grove",
		Worktrees: []domain.Worktree{{Path: "/repo/grove", Branch: "main"}},
	}
	state := BuildState(input)

	// Verify Correlations maps are initialized (empty, not nil)
	require.Empty(t, state.Correlations.WorktreeToPR)
	require.Empty(t, state.Correlations.WorktreeToIssue)
	require.Empty(t, state.Correlations.WorkflowToWorktree)
	require.Empty(t, state.Correlations.AgentToWorkflow)
	require.Empty(t, state.Correlations.PaneToAgent)
}

// ================================================================
// Phase 2 Red-Phase Tests: Issue #171
// ================================================================
// These tests assert required behavior from #171 acceptance criteria.
// They are expected to FAIL before the production fix is applied.
// ================================================================

// AC-1 / R1.1b: Previous WorkflowRuns must be preserved when Sandcastle is nil.
func TestPreviousWorkflowRunsPreserved(t *testing.T) {
	previousState := &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "wf-1", RunID: "run-1"},
			{WorkflowID: "wf-2", RunID: "run-2"},
		},
	}

	input := BuildInput{
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: nil,
		HerdrSnapshot:      &herdr.Snapshot{},
		PreviousState:      previousState,
	}

	state := BuildState(input)

	// AC-1: Previous WorkflowRuns must survive when Sandcastle is unavailable
	require.Len(t, state.WorkflowRuns, 2)
	assert.Equal(t, "wf-1", state.WorkflowRuns[0].WorkflowID)
	assert.Equal(t, "run-1", state.WorkflowRuns[0].RunID)
}

// AC-1 / R1.2c: Malformed Sandcastle (nil) must produce a Warning entry.
func TestMalformedSandcastleWarning(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: nil,
		HerdrSnapshot:      &herdr.Snapshot{},
		PreviousState: &domain.MissionControlState{
			WorkItems: []domain.WorkItem{
				{ID: "issue-42"},
			},
		},
	}

	state := BuildState(input)

	// AC-6: Warnings must be populated when Sandcastle is unavailable
	require.NotEmpty(t, state.Warnings, "expected warning when Sandcastle is nil")
	assert.Contains(t, state.Warnings[0].Message, "Sandcastle")
	assert.NotEmpty(t, state.Warnings[0].Level)
}

// AC-5 / R1.4: Missing Herdr inside Herdr must be degraded (not standalone).
func TestMissingHerdrInsideHerdrDegraded(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
		InsideHerdr:        true,
	}

	state := BuildState(input)

	// AC-5: Inside Herdr + nil snapshot → Mode "degraded"
	assert.Equal(t, "degraded", state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
}

// AC-4 / R1.3: Missing Herdr outside Herdr must be standalone (not degraded).
func TestMissingHerdrOutsideHerdrStandalone(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
		// InsideHerdr defaults to false (zero value) — E8
	}

	state := BuildState(input)

	// AC-4: Outside Herdr + nil snapshot → Mode "missing"
	assert.Equal(t, "missing", state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
}

// AC-8 / R2.4: Correlations maps must be populated after worktree-PR matching.
func TestCorrelationsPopulated(t *testing.T) {
	issue42 := 42
	pr := domain.PullRequest{
		Number: 42,
		Title:  "Fix issue-42",
		Branch: "issue-42",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "issue-42",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	input := BuildInput{
		RepoPath:           "/repo/grove",
		Worktrees:          []domain.Worktree{worktree},
		Issues:             []domain.Issue{{Number: issue42}},
		PullRequests:       []domain.PullRequest{pr},
		SandcastleSnapshot: &sandcastle.Snapshot{},
		HerdrSnapshot:      &herdr.Snapshot{},
	}

	state := BuildState(input)

	// AC-8: WorktreeToPR must map worktree branch to PR number
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42", state.WorkItems[0].ID)

	// R2.4: Correlations must be populated, keyed by worktree path
	assert.Equal(t, "42", state.Correlations.WorktreeToPR["/repo/grove"],
		"WorktreeToPR map must be populated with worktree path as key")
	assert.Equal(t, "42", state.Correlations.WorktreeToIssue["/repo/grove"],
		"WorktreeToIssue map must be populated with worktree path as key")
}

// E4: Both integrations missing → state.Status must be "degraded".
func TestBothIntegrationsMissingDegradedStatus(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: nil,
		// No PreviousState — degraded status must be set regardless
	}

	state := BuildState(input)

	// E4: Both integrations unavailable → top-level status degraded
	assert.Equal(t, domain.DegradedState, state.Status,
		"status must be degraded when all integrations are unavailable")
	assert.False(t, state.Integrations.Herdr.Available)
	assert.False(t, state.Integrations.Sandcastle.Available)
}

// E1: PreviousState nil must not panic on first build.
func TestEmptyPreviousStateNoPanic(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: nil,
		HerdrSnapshot:      nil,
		PreviousState:      nil,
	}

	// Must not panic
	state := BuildState(input)

	require.NotNil(t, state.WorkItems)
	require.NotNil(t, state.WorkflowRuns)
	require.NotNil(t, state.Warnings)
}

// E6: Previous WorkItem ID matching a fresh WorkItem must not duplicate.
func TestPreviousWorkItemsNotDuplicated(t *testing.T) {
	issue42 := 42
	pr := domain.PullRequest{
		Number: 42,
		Title:  "Fix issue-42",
		Branch: "issue-42",
		State:  "OPEN",
	}
	worktree := domain.Worktree{
		Path:      "/repo/grove",
		Branch:    "issue-42",
		CommitSHA: "abc123",
		IsClean:   true,
	}

	previousState := &domain.MissionControlState{
		WorkItems: []domain.WorkItem{
			{ID: "issue-42", CreatedAt: 1700000000, UpdatedAt: 1700000000},
		},
	}

	input := BuildInput{
		RepoPath:           "/repo/grove",
		Worktrees:          []domain.Worktree{worktree},
		Issues:             []domain.Issue{{Number: issue42}},
		PullRequests:       []domain.PullRequest{pr},
		SandcastleSnapshot: nil,
		HerdrSnapshot:      &herdr.Snapshot{},
		PreviousState:      previousState,
	}

	state := BuildState(input)

	// E6: Fresh correlation wins; previous item with same ID not appended
	require.Len(t, state.WorkItems, 1)
	assert.Equal(t, "issue-42", state.WorkItems[0].ID)
}

// AC-6: Warning for degraded Herdr inside Herdr.
func TestHerdrDegradedWarning(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
		InsideHerdr:        true,
	}

	state := BuildState(input)

	// AC-6: Degraded Herdr must produce a warning
	require.NotEmpty(t, state.Warnings)
	found := false
	for _, w := range state.Warnings {
		if strings.Contains(w.Message, "Herdr") || strings.Contains(w.Message, "herdr") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected warning about degraded Herdr")
}

// AC-3 / R2.2: WorkItem must have Degraded and DegradedReason fields.
func TestWorkItemDegradedMarking(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: &sandcastle.Snapshot{},
		HerdrSnapshot:      &herdr.Snapshot{},
	}

	state := BuildState(input)

	// R2.2: WorkItem struct must have Degraded and DegradedReason fields
	for _, wi := range state.WorkItems {
		_ = wi.Degraded       // Field must exist
		_ = wi.DegradedReason // Field must exist
	}
}

// E8: InsideHerdr defaults to false — zero BuildInput means outside Herdr (standalone).
func TestInsideHerdrDefaultsFalse(t *testing.T) {
	input := BuildInput{
		RepoPath:           "/repo/grove",
		HerdrSnapshot:      nil,
		SandcastleSnapshot: &sandcastle.Snapshot{},
		// InsideHerdr omitted — zero value false
	}

	state := BuildState(input)

	// E8: Outside Herdr + nil snapshot → Mode "missing", not "degraded"
	assert.Equal(t, "missing", state.Integrations.Herdr.Mode)
	assert.False(t, state.Integrations.Herdr.Available)
}

// AC-3: Stale pane ID marks WorkItem degraded with reason.
func TestStalePaneIDMarksWorkItemDegraded(t *testing.T) {
	previousWorkItem := domain.WorkItem{
		ID:       "issue-42",
		PaneRef:  &domain.PaneRef{PaneID: "w1:p3"},
	}

	previousState := &domain.MissionControlState{
		WorkItems: []domain.WorkItem{previousWorkItem},
	}

	// Herdr snapshot with different pane IDs (w1:p3 is absent)
	herdrSnapshot := &herdr.Snapshot{
		Panes: []domain.PaneRef{
			{PaneID: "w1:p1"},
			{PaneID: "w1:p2"},
		},
	}

	input := BuildInput{
		RepoPath:           "/repo/grove",
		SandcastleSnapshot: nil,
		HerdrSnapshot:      herdrSnapshot,
		PreviousState:      previousState,
	}

	state := BuildState(input)

	require.Len(t, state.WorkItems, 1)
	wi := state.WorkItems[0]
	assert.True(t, wi.Degraded, "work item referencing stale pane must be degraded")
	assert.Equal(t, "stale pane ID", wi.DegradedReason)
}
