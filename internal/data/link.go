package data

import (
	"fmt"

	"github.com/m00nk0d3/grove/internal/domain"
)

// LinkWorktreesToPRs matches each worktree to its corresponding pull request by
// branch name, then persists the linked_pr column to the database.
//
// When multiple PRs share the same branch, the one with the highest Number wins.
// Worktrees with no matching PR have LinkedPR set to nil and linked_pr set to NULL.
func LinkWorktreesToPRs(db *DB, worktrees []domain.Worktree, prs []domain.PullRequest) ([]domain.Worktree, error) {
	result := LinkWorktreesToPRsInMemory(worktrees, prs)

	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("link worktrees to prs: begin tx: %w", err)
	}

	for _, wt := range result {
		// Persist: NULL when no PR matched, PR number otherwise.
		var linkedPR interface{}
		if wt.LinkedPR != nil {
			linkedPR = wt.LinkedPR.Number
		}
		if _, err := tx.Exec(
			"UPDATE worktrees SET linked_pr=? WHERE path=?",
			linkedPR, wt.Path,
		); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("link worktrees to prs: update worktree %s: %w", wt.Path, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("link worktrees to prs: commit: %w", err)
	}

	return result, nil
}

// LinkWorktreesToPRsInMemory matches worktrees to PRs by branch without any DB interaction.
func LinkWorktreesToPRsInMemory(worktrees []domain.Worktree, prs []domain.PullRequest) []domain.Worktree {
	prByBranch := make(map[string]domain.PullRequest, len(prs))
	for _, pr := range prs {
		if existing, ok := prByBranch[pr.Branch]; !ok || pr.Number > existing.Number {
			prByBranch[pr.Branch] = pr
		}
	}
	result := make([]domain.Worktree, len(worktrees))
	for i, wt := range worktrees {
		if pr, ok := prByBranch[wt.Branch]; ok {
			matched := pr
			wt.LinkedPR = &matched
		}
		result[i] = wt
	}
	return result
}
