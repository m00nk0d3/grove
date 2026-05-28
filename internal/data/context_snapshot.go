package data

import (
	"fmt"
	"strings"

	"github.com/m00nk0d3/grove/internal/domain"
)

// SaveContextSnapshot persists a WorktreeContext to the context_snapshots table
// before an agent launch. Returns a wrapped error on failure.
func SaveContextSnapshot(db *DB, worktreeID int64, ctx *domain.WorktreeContext) error {
	fileList := strings.Join(ctx.ChangedFiles, "\n")
	_, err := db.Conn.Exec(
		`INSERT INTO context_snapshots (worktree_id, git_status, file_list, recent_log, diff_summary)
		 VALUES (?, ?, ?, ?, ?)`,
		worktreeID,
		ctx.GitStatus,
		fileList,
		ctx.RecentLog,
		ctx.DiffSummary,
	)
	if err != nil {
		return fmt.Errorf("save context snapshot: %w", err)
	}
	return nil
}
