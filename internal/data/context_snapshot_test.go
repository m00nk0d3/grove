package data_test

import (
	"strings"
	"testing"

	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertTestWorktree(t *testing.T, db *data.DB, path, branch string) int64 {
	t.Helper()
	res, err := db.Conn.Exec(
		"INSERT INTO worktrees (path, branch) VALUES (?, ?)", path, branch,
	)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestSaveContextSnapshot_PopulatesAllFields(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	wtID := insertTestWorktree(t, db, "/repo/feature", "feat/issue-19")

	ctx := &domain.WorktreeContext{
		Path:         "/repo/feature",
		Branch:       "feat/issue-19",
		GitStatus:    "M internal/exec/git.go",
		RecentLog:    "abc1234 feat: add context builder",
		ChangedFiles: []string{"internal/exec/git.go", "internal/domain/context.go"},
		DiffSummary:  "1 file changed, 42 insertions(+)",
	}

	err = data.SaveContextSnapshot(db, wtID, ctx)
	require.NoError(t, err, "SaveContextSnapshot should insert without error")

	var gotStatus, gotFileList, gotLog, gotDiffSummary string
	err = db.Conn.QueryRow(
		"SELECT git_status, file_list, recent_log, diff_summary FROM context_snapshots WHERE worktree_id = ?", wtID,
	).Scan(&gotStatus, &gotFileList, &gotLog, &gotDiffSummary)
	require.NoError(t, err)

	assert.Equal(t, ctx.GitStatus, gotStatus)
	assert.Equal(t, strings.Join(ctx.ChangedFiles, "\n"), gotFileList)
	assert.Equal(t, ctx.RecentLog, gotLog)
	assert.Equal(t, ctx.DiffSummary, gotDiffSummary)
}

func TestSaveContextSnapshot_EmptyChangedFiles(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	wtID := insertTestWorktree(t, db, "/repo/clean", "main")

	ctx := &domain.WorktreeContext{
		Path:         "/repo/clean",
		GitStatus:    "",
		RecentLog:    "abc1234 initial commit",
		ChangedFiles: []string{},
	}

	err = data.SaveContextSnapshot(db, wtID, ctx)
	require.NoError(t, err)

	var gotFileList string
	err = db.Conn.QueryRow(
		"SELECT file_list FROM context_snapshots WHERE worktree_id = ?", wtID,
	).Scan(&gotFileList)
	require.NoError(t, err)
	assert.Equal(t, "", gotFileList)
}

func TestSaveContextSnapshot_MultipleSnapshots(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	wtID := insertTestWorktree(t, db, "/repo/feature", "feat/ctx")

	ctx1 := &domain.WorktreeContext{GitStatus: "M file1.go", ChangedFiles: []string{}}
	ctx2 := &domain.WorktreeContext{GitStatus: "M file2.go", ChangedFiles: []string{}}

	require.NoError(t, data.SaveContextSnapshot(db, wtID, ctx1))
	require.NoError(t, data.SaveContextSnapshot(db, wtID, ctx2))

	var count int
	err = db.Conn.QueryRow(
		"SELECT COUNT(*) FROM context_snapshots WHERE worktree_id = ?", wtID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestSaveContextSnapshot_ClosedDB_ReturnsError(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	ctx := &domain.WorktreeContext{ChangedFiles: []string{}}
	err = data.SaveContextSnapshot(db, 1, ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "save context snapshot")
}
