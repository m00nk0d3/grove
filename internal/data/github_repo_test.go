package data

import (
	"errors"
	"testing"

	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/m00nk0d3/nexus/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDB returns an in-memory SQLite DB with the full schema applied.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// PR tests
// ---------------------------------------------------------------------------

func TestGitHubRepository_GetPRs_EmptyDB(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	prs, err := repo.GetPRs()

	require.NoError(t, err)
	assert.Empty(t, prs)
}

func TestGitHubRepository_UpsertAndGetPRs(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	input := []domain.PullRequest{
		{Number: 1, Title: "Add login", Branch: "feat/login", Author: "alice", State: "OPEN", Labels: []string{"enhancement"}, IsDraft: false},
		{Number: 2, Title: "WIP: refactor", Branch: "chore/refactor", Author: "bob", State: "OPEN", Labels: []string{"wip", "refactor"}, IsDraft: true},
	}

	err := repo.UpsertPRs(input)
	require.NoError(t, err)

	prs, err := repo.GetPRs()
	require.NoError(t, err)
	require.Len(t, prs, 2)

	// Sort by number for deterministic comparison.
	byNumber := func(slice []domain.PullRequest) map[int]domain.PullRequest {
		m := make(map[int]domain.PullRequest)
		for _, p := range slice {
			m[p.Number] = p
		}
		return m
	}

	got := byNumber(prs)
	assert.Equal(t, input[0], got[1])
	assert.Equal(t, input[1], got[2])
}

func TestGitHubRepository_UpsertPRs_UpdatesExisting(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	original := []domain.PullRequest{
		{Number: 1, Title: "Original title", Branch: "feat/original", Author: "alice", State: "OPEN", Labels: []string{}, IsDraft: false},
	}
	require.NoError(t, repo.UpsertPRs(original))

	updated := []domain.PullRequest{
		{Number: 1, Title: "Updated title", Branch: "feat/original", Author: "alice", State: "MERGED", Labels: []string{"done"}, IsDraft: false},
	}
	require.NoError(t, repo.UpsertPRs(updated))

	prs, err := repo.GetPRs()
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, "Updated title", prs[0].Title)
	assert.Equal(t, "MERGED", prs[0].State)
	assert.Equal(t, []string{"done"}, prs[0].Labels)
}

// ---------------------------------------------------------------------------
// Issue tests
// ---------------------------------------------------------------------------

func TestGitHubRepository_GetIssues_EmptyDB(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	issues, err := repo.GetIssues()

	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestGitHubRepository_UpsertAndGetIssues(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	input := []domain.Issue{
		{Number: 10, Title: "Fix the bug", Labels: []string{"bug"}},
		{Number: 11, Title: "Add a feature", Labels: []string{"enhancement", "phase-1"}},
	}

	require.NoError(t, repo.UpsertIssues(input))

	issues, err := repo.GetIssues()
	require.NoError(t, err)
	require.Len(t, issues, 2)

	byNumber := func(slice []domain.Issue) map[int]domain.Issue {
		m := make(map[int]domain.Issue)
		for _, i := range slice {
			m[i.Number] = i
		}
		return m
	}

	got := byNumber(issues)
	assert.Equal(t, input[0], got[10])
	assert.Equal(t, input[1], got[11])
}

func TestGitHubRepository_UpsertAndGetIssues_WithHierarchy(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	parentNum := 10
	input := []domain.Issue{
		{Number: 10, Title: "Parent issue", Labels: []string{"epic"}, SubIssueNumbers: []int{11, 12}},
		{Number: 11, Title: "Sub-issue one", Labels: []string{"task"}, ParentNumber: &parentNum},
		{Number: 12, Title: "Sub-issue two", Labels: []string{"task"}, ParentNumber: &parentNum},
	}

	require.NoError(t, repo.UpsertIssues(input))

	issues, err := repo.GetIssues()
	require.NoError(t, err)
	require.Len(t, issues, 3)

	byNumber := func(slice []domain.Issue) map[int]domain.Issue {
		m := make(map[int]domain.Issue)
		for _, i := range slice {
			m[i.Number] = i
		}
		return m
	}

	got := byNumber(issues)
	assert.Nil(t, got[10].ParentNumber, "parent issue should have nil ParentNumber")
	assert.Equal(t, []int{11, 12}, got[10].SubIssueNumbers, "parent should list sub-issue numbers")
	require.NotNil(t, got[11].ParentNumber, "sub-issue should have a ParentNumber")
	assert.Equal(t, 10, *got[11].ParentNumber)
	require.NotNil(t, got[12].ParentNumber, "sub-issue should have a ParentNumber")
	assert.Equal(t, 10, *got[12].ParentNumber)
}

func TestGitHubRepository_UpsertIssues_UpdatesExisting(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	original := []domain.Issue{
		{Number: 5, Title: "Original issue", Labels: []string{"bug"}},
	}
	require.NoError(t, repo.UpsertIssues(original))

	updated := []domain.Issue{
		{Number: 5, Title: "Updated issue title", Labels: []string{"bug", "priority-high"}},
	}
	require.NoError(t, repo.UpsertIssues(updated))

	issues, err := repo.GetIssues()
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "Updated issue title", issues[0].Title)
	assert.Equal(t, []string{"bug", "priority-high"}, issues[0].Labels)
}

// ---------------------------------------------------------------------------
// Sync tests
// ---------------------------------------------------------------------------

func TestGitHubRepository_SyncPRs_Success(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	rawJSON := `[
		{"number":1,"title":"Add login","headRefName":"feat/login","author":{"login":"alice"},"state":"OPEN","labels":[{"name":"enhancement"}],"isDraft":false},
		{"number":2,"title":"WIP: refactor","headRefName":"chore/refactor","author":{"login":"bob"},"state":"OPEN","labels":[{"name":"wip"}],"isDraft":true}
	]`
	mockRunner := func(_ string, _ ...string) (string, error) {
		return rawJSON, nil
	}
	client := exec.NewPRCommandWithRunner("/repo", mockRunner)

	err := repo.SyncPRs(client)
	require.NoError(t, err)

	prs, err := repo.GetPRs()
	require.NoError(t, err)
	require.Len(t, prs, 2)

	byNumber := func(slice []domain.PullRequest) map[int]domain.PullRequest {
		m := make(map[int]domain.PullRequest)
		for _, p := range slice {
			m[p.Number] = p
		}
		return m
	}
	got := byNumber(prs)
	assert.Equal(t, "Add login", got[1].Title)
	assert.Equal(t, "alice", got[1].Author)
	assert.Equal(t, []string{"enhancement"}, got[1].Labels)
	assert.Equal(t, true, got[2].IsDraft)
}

func TestGitHubRepository_SyncPRs_CLIError_PreservesCachedData(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	// Pre-load cache with one PR.
	cached := []domain.PullRequest{
		{Number: 99, Title: "Cached PR", Branch: "feat/cached", Author: "carol", State: "OPEN", Labels: []string{}, IsDraft: false},
	}
	require.NoError(t, repo.UpsertPRs(cached))

	// SyncPRs with a runner that always fails.
	failRunner := func(_ string, _ ...string) (string, error) {
		return "", errors.New("gh: network error")
	}
	client := exec.NewPRCommandWithRunner("/repo", failRunner)

	err := repo.SyncPRs(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")

	// Cached data must still be there.
	prs, err := repo.GetPRs()
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 99, prs[0].Number)
	assert.Equal(t, "Cached PR", prs[0].Title)
}

func TestGitHubRepository_SyncIssues_Success(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	rawJSON := `[
		{"number":10,"title":"Fix the bug","labels":[{"name":"bug"}]},
		{"number":11,"title":"Add a feature","labels":[{"name":"enhancement"}]}
	]`
	mockRunner := func(_ string, _ ...string) (string, error) {
		return rawJSON, nil
	}
	client := exec.NewIssueCommandWithRunner("/repo", mockRunner)

	err := repo.SyncIssues(client)
	require.NoError(t, err)

	issues, err := repo.GetIssues()
	require.NoError(t, err)
	require.Len(t, issues, 2)

	byNumber := func(slice []domain.Issue) map[int]domain.Issue {
		m := make(map[int]domain.Issue)
		for _, i := range slice {
			m[i.Number] = i
		}
		return m
	}
	got := byNumber(issues)
	assert.Equal(t, "Fix the bug", got[10].Title)
	assert.Equal(t, []string{"bug"}, got[10].Labels)
	assert.Equal(t, "Add a feature", got[11].Title)
}

func TestGitHubRepository_IsolatedByRepoPath(t *testing.T) {
	db := newTestDB(t)
	nexus := NewGitHubRepository(db, "/home/user/development/nexus")
	nova := NewGitHubRepository(db, "/home/user/development/nova")

	// Insert issues for nexus.
	require.NoError(t, nexus.UpsertIssues([]domain.Issue{
		{Number: 1, Title: "Nexus issue", Labels: []string{"bug"}},
	}))
	// Insert a different issue with the same number for nova.
	require.NoError(t, nova.UpsertIssues([]domain.Issue{
		{Number: 1, Title: "Nova issue", Labels: []string{"feature"}},
		{Number: 2, Title: "Nova issue 2", Labels: []string{}},
	}))

	nexusIssues, err := nexus.GetIssues()
	require.NoError(t, err)
	require.Len(t, nexusIssues, 1, "nexus should only see its own issues")
	assert.Equal(t, "Nexus issue", nexusIssues[0].Title)

	novaIssues, err := nova.GetIssues()
	require.NoError(t, err)
	require.Len(t, novaIssues, 2, "nova should only see its own issues")
	assert.Equal(t, "Nova issue", novaIssues[0].Title)

	// PRs are isolated too.
	require.NoError(t, nexus.UpsertPRs([]domain.PullRequest{
		{Number: 10, Title: "Nexus PR", Branch: "feat/nexus", Author: "alice", State: "OPEN", Labels: []string{}},
	}))
	nexusPRs, err := nexus.GetPRs()
	require.NoError(t, err)
	require.Len(t, nexusPRs, 1)

	novaPRs, err := nova.GetPRs()
	require.NoError(t, err)
	assert.Empty(t, novaPRs, "nova should not see nexus PRs")
}

func TestGitHubRepository_SyncIssues_CLIError_PreservesCachedData(t *testing.T) {
	repo := NewGitHubRepository(newTestDB(t), "/repo/nexus")

	// Pre-load cache with one issue.
	cached := []domain.Issue{
		{Number: 42, Title: "Cached issue", Labels: []string{"cached"}},
	}
	require.NoError(t, repo.UpsertIssues(cached))

	// SyncIssues with a runner that always fails.
	failRunner := func(_ string, _ ...string) (string, error) {
		return "", errors.New("gh: auth required")
	}
	client := exec.NewIssueCommandWithRunner("/repo", failRunner)

	err := repo.SyncIssues(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth required")

	// Cached data must still be there.
	issues, err := repo.GetIssues()
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, 42, issues[0].Number)
	assert.Equal(t, "Cached issue", issues[0].Title)
}
