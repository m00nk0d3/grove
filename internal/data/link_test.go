package data_test

import (
	"testing"

	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkWorktreesToPRs exercises LinkWorktreesToPRs across five behaviours:
//  1. exact branch match  → LinkedPR points at the matched PR
//  2. no branch match     → LinkedPR remains nil
//  3. duplicate branches  → the PR with the highest number wins
//  4. match persisted     → linked_pr column is updated in SQLite
//  5. no-match persisted  → linked_pr column is set to NULL in SQLite
func TestLinkWorktreesToPRs(t *testing.T) {
	t.Run("exact match links correctly", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		worktrees := []domain.Worktree{
			{Path: "/repo/feat-issue-13", Branch: "feat/issue-13"},
		}
		prs := []domain.PullRequest{
			{Number: 13, Title: "feat: link PRs to worktrees", Branch: "feat/issue-13", State: "OPEN"},
		}

		got, err := data.LinkWorktreesToPRs(db, worktrees, prs)
		require.NoError(t, err)
		require.Len(t, got, 1)

		require.NotNil(t, got[0].LinkedPR, "LinkedPR should be set for an exact branch match")
		assert.Equal(t, 13, got[0].LinkedPR.Number)
		assert.Equal(t, "feat/issue-13", got[0].LinkedPR.Branch)
	})

	t.Run("no match leaves LinkedPR nil", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		worktrees := []domain.Worktree{
			{Path: "/repo/feat-issue-13", Branch: "feat/issue-13"},
		}
		prs := []domain.PullRequest{
			{Number: 99, Title: "unrelated PR", Branch: "other-branch", State: "OPEN"},
		}

		got, err := data.LinkWorktreesToPRs(db, worktrees, prs)
		require.NoError(t, err)
		require.Len(t, got, 1)

		assert.Nil(t, got[0].LinkedPR, "LinkedPR should be nil when no PR branch matches the worktree branch")
	})

	t.Run("duplicate branch picks highest PR number", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		worktrees := []domain.Worktree{
			{Path: "/repo/feat-issue-13", Branch: "feat/issue-13"},
		}
		prs := []domain.PullRequest{
			{Number: 5, Title: "older PR", Branch: "feat/issue-13", State: "OPEN"},
			{Number: 42, Title: "newer PR", Branch: "feat/issue-13", State: "OPEN"},
		}

		got, err := data.LinkWorktreesToPRs(db, worktrees, prs)
		require.NoError(t, err)
		require.Len(t, got, 1)

		require.NotNil(t, got[0].LinkedPR, "LinkedPR should be set when at least one PR matches")
		assert.Equal(t, 42, got[0].LinkedPR.Number, "should pick the highest PR number when branch is duplicated")
	})

	t.Run("result is persisted to SQLite", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		const (
			worktreePath   = "/repo/feat-issue-13"
			worktreeBranch = "feat/issue-13"
			prNumber       = 13
		)

		// Pre-insert the worktree row so LinkWorktreesToPRs has a row to UPDATE.
		_, err = db.Conn.Exec(
			"INSERT INTO worktrees (path, branch) VALUES (?, ?)",
			worktreePath, worktreeBranch,
		)
		require.NoError(t, err, "inserting the seed worktree row should succeed")

		worktrees := []domain.Worktree{
			{Path: worktreePath, Branch: worktreeBranch},
		}
		prs := []domain.PullRequest{
			{Number: prNumber, Title: "feat: link PRs to worktrees", Branch: worktreeBranch, State: "OPEN"},
		}

		_, err = data.LinkWorktreesToPRs(db, worktrees, prs)
		require.NoError(t, err)

		// Read back the persisted linked_pr value.
		var gotLinkedPR int
		err = db.Conn.QueryRow(
			"SELECT linked_pr FROM worktrees WHERE path = ?",
			worktreePath,
		).Scan(&gotLinkedPR)
		require.NoError(t, err, "querying linked_pr after linking should return a row")

		assert.Equal(t, prNumber, gotLinkedPR, "linked_pr column should be persisted to SQLite")
	})

	t.Run("unmatched worktree persists NULL to SQLite", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		const (
			worktreePath   = "/repo/feat-issue-13"
			worktreeBranch = "feat/issue-13"
		)

		// Pre-seed worktree row with a linked_pr value so we can confirm it is
		// cleared to NULL when no PR matches.
		_, err = db.Conn.Exec(
			"INSERT INTO worktrees (path, branch, linked_pr) VALUES (?, ?, ?)",
			worktreePath, worktreeBranch, 99,
		)
		require.NoError(t, err, "inserting the seed worktree row should succeed")

		worktrees := []domain.Worktree{
			{Path: worktreePath, Branch: worktreeBranch},
		}
		prs := []domain.PullRequest{
			{Number: 99, Title: "unrelated PR", Branch: "other-branch", State: "OPEN"},
		}

		_, err = data.LinkWorktreesToPRs(db, worktrees, prs)
		require.NoError(t, err)

		// linked_pr must be NULL — scan into *int so a NULL value doesn't cause an error.
		var gotLinkedPR *int
		err = db.Conn.QueryRow(
			"SELECT linked_pr FROM worktrees WHERE path = ?",
			worktreePath,
		).Scan(&gotLinkedPR)
		require.NoError(t, err, "querying linked_pr after linking should return a row")

		assert.Nil(t, gotLinkedPR, "linked_pr column should be NULL when no PR matches the worktree branch")
	})
}
