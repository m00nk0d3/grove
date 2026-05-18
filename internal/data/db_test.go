package data_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/m00nk0d3/nexus/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableExists queries sqlite_master to verify that a table was created by
// the schema migration.  It accepts the raw *sql.DB so the helper stays
// independent of the data.DB wrapper.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
		name,
	).Scan(&found)
	return err == nil && found == name
}

// TestNewDB_CreatesSchema verifies that NewDB opens a valid connection and
// runs the DDL migration that produces all five required tables.
func TestNewDB_CreatesSchema(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err, "NewDB should open without error")
	require.NotNil(t, db, "NewDB should return a non-nil DB")
	t.Cleanup(func() { _ = db.Close() })

	tables := []string{
		"worktrees",
		"github_prs",
		"github_issues",
		"agent_history",
		"context_snapshots",
	}

	for _, table := range tables {
		assert.Truef(t, tableExists(t, db.Conn, table),
			"expected table %q to exist after schema migration", table)
	}
}

// TestNewDB_WorktreeInsertAndQuery verifies that the worktrees table accepts
// a row with (path, branch) and that the row can be retrieved by path.
func TestNewDB_WorktreeInsertAndQuery(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() { _ = db.Close() })

	const (
		wantPath   = "/repo/worktrees/feat-issue-10"
		wantBranch = "feat/issue-10-sqlite-db-setup"
	)

	_, err = db.Conn.Exec(
		"INSERT INTO worktrees (path, branch) VALUES (?, ?)",
		wantPath, wantBranch,
	)
	require.NoError(t, err, "inserting a worktree row should succeed")

	var gotPath, gotBranch string
	err = db.Conn.QueryRow(
		"SELECT path, branch FROM worktrees WHERE path = ?",
		wantPath,
	).Scan(&gotPath, &gotBranch)
	require.NoError(t, err, "querying worktree by path should return exactly one row")

	assert.Equal(t, wantPath, gotPath)
	assert.Equal(t, wantBranch, gotBranch)
}

// TestNewDB_GithubPRInsertAndQuery verifies that the github_prs table accepts
// a row with (number, title, branch, state) and that the row can be retrieved
// by PR number.
func TestNewDB_GithubPRInsertAndQuery(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() { _ = db.Close() })

	const (
		wantNumber = 42
		wantTitle  = "feat: add SQLite persistence layer"
		wantBranch = "feat/issue-10-sqlite-db-setup"
		wantState  = "open"
	)

	_, err = db.Conn.Exec(
		"INSERT INTO github_prs (number, title, branch, state) VALUES (?, ?, ?, ?)",
		wantNumber, wantTitle, wantBranch, wantState,
	)
	require.NoError(t, err, "inserting a github_pr row should succeed")

	var gotNumber int
	var gotTitle, gotBranch, gotState string
	err = db.Conn.QueryRow(
		"SELECT number, title, branch, state FROM github_prs WHERE number = ?",
		wantNumber,
	).Scan(&gotNumber, &gotTitle, &gotBranch, &gotState)
	require.NoError(t, err, "querying github_pr by number should return exactly one row")

	assert.Equal(t, wantNumber, gotNumber)
	assert.Equal(t, wantTitle, gotTitle)
	assert.Equal(t, wantBranch, gotBranch)
	assert.Equal(t, wantState, gotState)
}

// TestDB_Close verifies that calling Close on an open DB returns no error and
// does not panic.
func TestDB_Close(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NotNil(t, db)

	err = db.Close()
	assert.NoError(t, err, "Close should return no error on a live connection")
}

// TestNewDB_AgentHistoryInsertAndQuery verifies that the agent_history table
// accepts a row and that it can be retrieved by agent_name.
func TestNewDB_AgentHistoryInsertAndQuery(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const (
		wantName     = "go-specialist"
		wantPrompt   = "implement xyz"
		wantExitCode = 0
	)

	_, err = db.Conn.Exec(
		"INSERT INTO agent_history (agent_name, prompt, exit_code) VALUES (?, ?, ?)",
		wantName, wantPrompt, wantExitCode,
	)
	require.NoError(t, err, "inserting an agent_history row should succeed")

	var gotName, gotPrompt string
	var gotExitCode int
	err = db.Conn.QueryRow(
		"SELECT agent_name, prompt, exit_code FROM agent_history WHERE agent_name = ?",
		wantName,
	).Scan(&gotName, &gotPrompt, &gotExitCode)
	require.NoError(t, err, "querying agent_history by agent_name should return exactly one row")

	assert.Equal(t, wantName, gotName)
	assert.Equal(t, wantPrompt, gotPrompt)
	assert.Equal(t, wantExitCode, gotExitCode)
}

// TestNewDB_ContextSnapshotInsertAndQuery verifies that the context_snapshots
// table accepts a row linked to a worktree and that it can be retrieved.
func TestNewDB_ContextSnapshotInsertAndQuery(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Conn.Exec(
		"INSERT INTO worktrees (path, branch) VALUES (?, ?)",
		"/repo/feat", "feat/branch",
	)
	require.NoError(t, err)

	var worktreeID int64
	require.NoError(t, db.Conn.QueryRow(
		"SELECT id FROM worktrees WHERE path = ?", "/repo/feat",
	).Scan(&worktreeID))

	const (
		wantStatus   = "M internal/data/db.go"
		wantFileList = "db.go\ndb_test.go"
	)

	_, err = db.Conn.Exec(
		"INSERT INTO context_snapshots (worktree_id, git_status, file_list) VALUES (?, ?, ?)",
		worktreeID, wantStatus, wantFileList,
	)
	require.NoError(t, err, "inserting a context_snapshot row should succeed")

	var gotStatus, gotFileList string
	err = db.Conn.QueryRow(
		"SELECT git_status, file_list FROM context_snapshots WHERE worktree_id = ?",
		worktreeID,
	).Scan(&gotStatus, &gotFileList)
	require.NoError(t, err, "querying context_snapshots by worktree_id should return exactly one row")

	assert.Equal(t, wantStatus, gotStatus)
	assert.Equal(t, wantFileList, gotFileList)
}

// TestNewDB_MigrationsAreIdempotent verifies that opening a database twice
// does not re-run already-applied migrations or return an error.
func TestNewDB_MigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := data.NewDB(path)
	require.NoError(t, err, "first open should succeed")
	require.NoError(t, db1.Close())

	db2, err := data.NewDB(path)
	require.NoError(t, err, "second open should not re-run migrations or fail")
	require.NoError(t, db2.Close())
}

// TestNewDB_SchemaVersionsTracked verifies that applied migrations are
// recorded in the schema_migrations table.
func TestNewDB_SchemaVersionsTracked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := data.NewDB(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows, err := db.Conn.Query("SELECT filename FROM schema_migrations ORDER BY filename")
	require.NoError(t, err)
	defer rows.Close()

	var filenames []string
	for rows.Next() {
		var fn string
		require.NoError(t, rows.Scan(&fn))
		filenames = append(filenames, fn)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []string{"001_init_schema.sql", "002_add_diff_summary_to_context_snapshots.sql", "003_add_issue_hierarchy.sql"}, filenames)
}
