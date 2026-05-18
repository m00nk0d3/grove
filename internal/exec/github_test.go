package exec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIssueCommand(t *testing.T) {
	cmd := NewIssueCommand("/repo")
	assert.NotNil(t, cmd)
	assert.Equal(t, "/repo", cmd.repoPath)
	assert.NotNil(t, cmd.runner)
}

func TestListOpenIssues_ValidOutput_ReturnsIssues(t *testing.T) {
	raw := `[{"number":5,"title":"[Phase 1] Implement - Create/delete modals","body":"","labels":[{"name":"phase-1"}]},{"number":6,"title":"[Phase 1] Implement - Switch worktree","body":"Some details","labels":[{"name":"phase-1"},{"name":"enhancement"}]}]`

	runner := func(_ string, _ ...string) (string, error) {
		return raw, nil
	}

	cmd := NewIssueCommandWithRunner("/repo", runner)
	issues, err := cmd.ListOpenIssues()

	require.NoError(t, err)
	require.Len(t, issues, 2)

	assert.Equal(t, 5, issues[0].Number)
	assert.Equal(t, "[Phase 1] Implement - Create/delete modals", issues[0].Title)
	assert.Equal(t, []string{"phase-1"}, issues[0].Labels)

	assert.Equal(t, 6, issues[1].Number)
	assert.Equal(t, []string{"phase-1", "enhancement"}, issues[1].Labels)
}

func TestListOpenIssues_EmptyList_ReturnsEmptySlice(t *testing.T) {
	runner := func(_ string, _ ...string) (string, error) {
		return "[]", nil
	}

	cmd := NewIssueCommandWithRunner("/repo", runner)
	issues, err := cmd.ListOpenIssues()

	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestListOpenIssues_NoLabels_ReturnsIssueWithEmptyLabels(t *testing.T) {
	raw := `[{"number":7,"title":"Some issue","labels":[]}]`

	runner := func(_ string, _ ...string) (string, error) {
		return raw, nil
	}

	cmd := NewIssueCommandWithRunner("/repo", runner)
	issues, err := cmd.ListOpenIssues()

	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Empty(t, issues[0].Labels)
}

func TestListOpenIssues_RunnerError_PropagatesError(t *testing.T) {
	runner := func(_ string, _ ...string) (string, error) {
		return "", errors.New("gh: not logged in")
	}

	cmd := NewIssueCommandWithRunner("/repo", runner)
	_, err := cmd.ListOpenIssues()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "list open issues")
	assert.Contains(t, err.Error(), "gh: not logged in")
}

func TestListOpenIssues_InvalidJSON_ReturnsError(t *testing.T) {
	runner := func(_ string, _ ...string) (string, error) {
		return "not valid json", nil
	}

	cmd := NewIssueCommandWithRunner("/repo", runner)
	_, err := cmd.ListOpenIssues()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse issue list")
}

func TestListOpenIssues_PassesCorrectArgs(t *testing.T) {
	var capturedArgs []string

	runner := func(_ string, args ...string) (string, error) {
		capturedArgs = args
		return "[]", nil
	}

	cmd := NewIssueCommandWithRunner("/repo", runner)
	_, err := cmd.ListOpenIssues()

	require.NoError(t, err)
	assert.Equal(t, []string{"issue", "list", "--json", "number,title,body,labels,assignees", "--state", "open", "--limit", "100"}, capturedArgs)
}

func TestListOpenIssues_MapsDomainsCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected []domain.Issue
	}{
		{
			name: "single issue with multiple labels",
			raw:  `[{"number":42,"title":"Fix the thing","body":"Details here","labels":[{"name":"bug"},{"name":"priority-high"}]}]`,
			expected: []domain.Issue{
				{Number: 42, Title: "Fix the thing", Body: "Details here", Labels: []string{"bug", "priority-high"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(_ string, _ ...string) (string, error) { return tt.raw, nil }
			cmd := NewIssueCommandWithRunner("/repo", runner)

			issues, err := cmd.ListOpenIssues()

			require.NoError(t, err)
			assert.Equal(t, tt.expected, issues)
		})
	}
}

func TestListOpenIssues_MapsAssigneesCorrectly(t *testing.T) {
	tests := []struct {
		name              string
		raw               string
		wantAssignees     []string
	}{
		{
			name:          "single assignee is mapped",
			raw:           `[{"number":1,"title":"T","labels":[],"assignees":[{"login":"alice"}]}]`,
			wantAssignees: []string{"alice"},
		},
		{
			name:          "multiple assignees are mapped",
			raw:           `[{"number":1,"title":"T","labels":[],"assignees":[{"login":"alice"},{"login":"bob"}]}]`,
			wantAssignees: []string{"alice", "bob"},
		},
		{
			name:          "no assignees returns nil",
			raw:           `[{"number":1,"title":"T","labels":[],"assignees":[]}]`,
			wantAssignees: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(_ string, _ ...string) (string, error) { return tt.raw, nil }
			cmd := NewIssueCommandWithRunner("/repo", runner)

			issues, err := cmd.ListOpenIssues()

			require.NoError(t, err)
			require.Len(t, issues, 1)
			assert.Equal(t, tt.wantAssignees, issues[0].Assignees)
		})
	}
}

// ---------------------------------------------------------------------------
// EnrichHierarchyFromBodies
// ---------------------------------------------------------------------------

func TestEnrichHierarchyFromBodies_PartOfPattern(t *testing.T) {
	issues := []domain.Issue{
		{Number: 61, Title: "Parent", Body: ""},
		{Number: 65, Title: "Child", Body: "## Parent Issue\nPart of #61 - some title"},
	}
	EnrichHierarchyFromBodies(issues)

	require.NotNil(t, issues[1].ParentNumber, "sub-issue should have ParentNumber set")
	assert.Equal(t, 61, *issues[1].ParentNumber)
	assert.Contains(t, issues[0].SubIssueNumbers, 65, "parent should list sub-issue number")
}

func TestEnrichHierarchyFromBodies_TrackedByPattern(t *testing.T) {
	issues := []domain.Issue{
		{Number: 10, Title: "Epic", Body: ""},
		{Number: 11, Title: "Task", Body: "Tracked by #10"},
	}
	EnrichHierarchyFromBodies(issues)

	require.NotNil(t, issues[1].ParentNumber)
	assert.Equal(t, 10, *issues[1].ParentNumber)
}

func TestEnrichHierarchyFromBodies_MultipleChildren(t *testing.T) {
	issues := []domain.Issue{
		{Number: 61, Title: "Parent", Body: ""},
		{Number: 62, Title: "Child A", Body: "Part of #61"},
		{Number: 63, Title: "Child B", Body: "Part of #61"},
		{Number: 64, Title: "Child C", Body: "Part of #61"},
	}
	EnrichHierarchyFromBodies(issues)

	assert.Equal(t, 3, len(issues[0].SubIssueNumbers))
	for _, child := range issues[1:] {
		require.NotNil(t, child.ParentNumber)
		assert.Equal(t, 61, *child.ParentNumber)
	}
}

func TestEnrichHierarchyFromBodies_NoPattern_NoChange(t *testing.T) {
	issues := []domain.Issue{
		{Number: 1, Title: "Standalone", Body: "No hierarchy info here"},
	}
	EnrichHierarchyFromBodies(issues)

	assert.Nil(t, issues[0].ParentNumber)
	assert.Empty(t, issues[0].SubIssueNumbers)
}

func TestEnrichHierarchyFromBodies_DoesNotOverwriteExistingParent(t *testing.T) {
	existing := 99
	issues := []domain.Issue{
		{Number: 61, Title: "Parent", Body: ""},
		{Number: 65, Title: "Child", Body: "Part of #61", ParentNumber: &existing},
	}
	EnrichHierarchyFromBodies(issues)

	assert.Equal(t, 99, *issues[1].ParentNumber, "should not overwrite GraphQL-set ParentNumber")
}

func TestEnrichHierarchyFromBodies_ParentNotInSlice_ChildStillGetsParentNumber(t *testing.T) {
	issues := []domain.Issue{
		{Number: 65, Title: "Child", Body: "Part of #61"},
		// issue 61 is NOT in this slice (e.g., it's closed)
	}
	EnrichHierarchyFromBodies(issues)

	require.NotNil(t, issues[0].ParentNumber)
	assert.Equal(t, 61, *issues[0].ParentNumber)
}

// ---------------------------------------------------------------------------
// FetchIssueHierarchy
// ---------------------------------------------------------------------------

func TestFetchIssueHierarchy_BulkQuery_ReturnsParentChildMap(t *testing.T) {
	raw := `{"data":{"repository":{"issues":{"nodes":[
		{"number":61,"subIssues":{"nodes":[{"number":62},{"number":63}]}},
		{"number":62,"subIssues":{"nodes":[]}},
		{"number":63,"subIssues":{"nodes":[]}}
	]}}}}`

	runner := func(_ string, args ...string) (string, error) { return raw, nil }
	cmd := NewIssueCommandWithRunner("/repo", runner)

	hier, err := cmd.FetchIssueHierarchy([]int{61, 62, 63}, "owner", "repo")

	require.NoError(t, err)
	require.NotNil(t, hier)
	assert.Equal(t, []int{62, 63}, hier[61])
	assert.Nil(t, hier[62])
}

func TestFetchIssueHierarchy_RunnerError_ReturnsNilNil(t *testing.T) {
	runner := func(_ string, _ ...string) (string, error) {
		return "", errors.New("gh api failed")
	}
	cmd := NewIssueCommandWithRunner("/repo", runner)

	hier, err := cmd.FetchIssueHierarchy([]int{1}, "owner", "repo")

	assert.NoError(t, err)
	assert.Nil(t, hier)
}

func TestFetchIssueHierarchy_InvalidJSON_ReturnsNilNil(t *testing.T) {
	runner := func(_ string, _ ...string) (string, error) { return "not-json", nil }
	cmd := NewIssueCommandWithRunner("/repo", runner)

	hier, err := cmd.FetchIssueHierarchy([]int{1}, "owner", "repo")

	assert.NoError(t, err)
	assert.Nil(t, hier)
}

func TestFetchIssueHierarchy_EmptyNumbers_StillQueriesAPI(t *testing.T) {
	raw := `{"data":{"repository":{"issues":{"nodes":[]}}}}`
	runner := func(_ string, _ ...string) (string, error) { return raw, nil }
	cmd := NewIssueCommandWithRunner("/repo", runner)

	// numbers slice is now ignored — func always does a bulk query
	hier, err := cmd.FetchIssueHierarchy(nil, "owner", "repo")

	assert.NoError(t, err)
	assert.Empty(t, hier)
}

// ---------------------------------------------------------------------------
// PRCommand — PullRequest tests
// ---------------------------------------------------------------------------

func TestNewPRCommand(t *testing.T) {
	client := NewPRCommand("/repo")
	assert.NotNil(t, client)
	assert.Equal(t, "/repo", client.repoPath)
	assert.NotNil(t, client.runner)
}

func TestListOpenPRs(t *testing.T) {
	twoPRsJSON := `[
		{"number":1,"title":"Add login","body":"Adds login flow","headRefName":"feat/login","author":{"login":"alice"},"state":"OPEN","labels":[{"name":"enhancement"}],"isDraft":false},
		{"number":2,"title":"WIP: refactor","body":"","headRefName":"chore/refactor","author":{"login":"bob"},"state":"OPEN","labels":[{"name":"wip"},{"name":"refactor"}],"isDraft":true}
	]`

	tests := []struct {
		name        string
		runnerOut   string
		runnerErr   error
		wantErr     bool
		errContains string
		wantPRs     []domain.PullRequest
		checkArgs   bool
	}{
		{
			name:      "valid JSON with 2 PRs maps correctly",
			runnerOut: twoPRsJSON,
			wantPRs: []domain.PullRequest{
				{Number: 1, Title: "Add login", Body: "Adds login flow", Branch: "feat/login", Author: "alice", State: "OPEN", Labels: []string{"enhancement"}, IsDraft: false},
				{Number: 2, Title: "WIP: refactor", Body: "", Branch: "chore/refactor", Author: "bob", State: "OPEN", Labels: []string{"wip", "refactor"}, IsDraft: true},
			},
		},
		{
			name:      "empty list returns empty slice",
			runnerOut: "[]",
			wantPRs:   []domain.PullRequest{},
		},
		{
			name:        "runner error wraps with list open prs",
			runnerErr:   errors.New("gh: auth required"),
			wantErr:     true,
			errContains: "list open prs",
		},
		{
			name:        "invalid JSON returns parse pr list error",
			runnerOut:   "not json",
			wantErr:     true,
			errContains: "parse pr list",
		},
		{
			name:      "PR with no labels has empty (non-nil) Labels slice",
			runnerOut: `[{"number":3,"title":"Hotfix","headRefName":"fix/hotfix","author":{"login":"carol"},"state":"OPEN","labels":[],"isDraft":false}]`,
			wantPRs: []domain.PullRequest{
				{Number: 3, Title: "Hotfix", Branch: "fix/hotfix", Author: "carol", State: "OPEN", Labels: []string{}, IsDraft: false},
			},
		},
		{
			name:      "isDraft false maps correctly",
			runnerOut: `[{"number":4,"title":"Stable PR","headRefName":"feat/stable","author":{"login":"dave"},"state":"OPEN","labels":[],"isDraft":false}]`,
			wantPRs: []domain.PullRequest{
				{Number: 4, Title: "Stable PR", Branch: "feat/stable", Author: "dave", State: "OPEN", Labels: []string{}, IsDraft: false},
			},
		},
		{
			name:      "isDraft true maps correctly",
			runnerOut: `[{"number":5,"title":"Draft PR","headRefName":"feat/draft","author":{"login":"eve"},"state":"OPEN","labels":[],"isDraft":true}]`,
			wantPRs: []domain.PullRequest{
				{Number: 5, Title: "Draft PR", Branch: "feat/draft", Author: "eve", State: "OPEN", Labels: []string{}, IsDraft: true},
			},
		},
		{
			name:      "correct gh args passed",
			runnerOut: "[]",
			checkArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedArgs []string
			runner := func(_ string, args ...string) (string, error) {
				capturedArgs = args
				return tt.runnerOut, tt.runnerErr
			}

			client := NewPRCommandWithRunner("/repo", runner)
			prs, err := client.ListOpenPRs()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)

			if tt.checkArgs {
				assert.Equal(t,
					[]string{"pr", "list", "--json", prFields, "--state", "open", "--limit", "100"},
					capturedArgs,
				)
				return
			}

			assert.Equal(t, tt.wantPRs, prs)
		})
	}
}

func TestListOpenPRs_MapsAssigneesCorrectly(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantAssignees []string
	}{
		{
			name:          "single assignee is mapped",
			raw:           `[{"number":1,"title":"T","labels":[],"assignees":[{"login":"alice"}]}]`,
			wantAssignees: []string{"alice"},
		},
		{
			name:          "multiple assignees are mapped",
			raw:           `[{"number":1,"title":"T","labels":[],"assignees":[{"login":"alice"},{"login":"bob"}]}]`,
			wantAssignees: []string{"alice", "bob"},
		},
		{
			name:          "no assignees returns nil",
			raw:           `[{"number":1,"title":"T","labels":[],"assignees":[]}]`,
			wantAssignees: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(_ string, args ...string) (string, error) {
				return tt.raw, nil
			}
			client := NewPRCommandWithRunner("/repo", runner)
			prs, err := client.ListOpenPRs()
			require.NoError(t, err)
			require.Len(t, prs, 1)
			assert.Equal(t, tt.wantAssignees, prs[0].Assignees)
		})
	}
}

func TestGetPR(t *testing.T) {
	validPRJSON := `{"number":42,"title":"Implement feature","body":"This implements the feature.","headRefName":"feat/feature","author":{"login":"frank"},"state":"MERGED","labels":[{"name":"feature"}],"isDraft":false}`

	tests := []struct {
		name        string
		prNumber    int
		runnerOut   string
		runnerErr   error
		wantErr     bool
		errContains string
		wantPR      *domain.PullRequest
		checkArgs   bool
	}{
		{
			name:      "valid JSON single PR maps correctly",
			prNumber:  42,
			runnerOut: validPRJSON,
			wantPR: &domain.PullRequest{
				Number:  42,
				Title:   "Implement feature",
				Body:    "This implements the feature.",
				Branch:  "feat/feature",
				Author:  "frank",
				State:   "MERGED",
				Labels:  []string{"feature"},
				IsDraft: false,
			},
		},
		{
			name:        "runner error wraps with get pr",
			prNumber:    42,
			runnerErr:   errors.New("gh: not found"),
			wantErr:     true,
			errContains: "get pr",
		},
		{
			name:        "invalid JSON returns parse pr error",
			prNumber:    42,
			runnerOut:   "bad json",
			wantErr:     true,
			errContains: "parse pr",
		},
		{
			name:      "correct gh args passed for pr 42",
			prNumber:  42,
			runnerOut: validPRJSON,
			checkArgs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedArgs []string
			runner := func(_ string, args ...string) (string, error) {
				capturedArgs = args
				return tt.runnerOut, tt.runnerErr
			}

			client := NewPRCommandWithRunner("/repo", runner)
			pr, err := client.GetPR(tt.prNumber)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)

			if tt.checkArgs {
				assert.Equal(t,
					[]string{"pr", "view", fmt.Sprintf("%d", tt.prNumber), "--json", prFields},
					capturedArgs,
				)
				return
			}

			assert.Equal(t, tt.wantPR, pr)
		})
	}
}
