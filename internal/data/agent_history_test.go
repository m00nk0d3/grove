package data_test

import (
	"testing"
	"time"

	"github.com/m00nk0d3/nexus/internal/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogAgentRun_Success verifies that LogAgentRun inserts a row into
// agent_history and that the values can be read back from the database.
func TestLogAgentRun_Success(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	entry := data.AgentHistoryEntry{
		AgentName: "copilot",
		Prompt:    "fix the null pointer dereference",
		ExitCode:  0,
		StartedAt: now,
		EndedAt:   now.Add(5 * time.Second),
	}

	err = data.LogAgentRun(db, entry)
	require.NoError(t, err, "LogAgentRun should insert without error")

	// Verify the row was written and values match.
	var gotAgent, gotPrompt string
	var gotExit int
	err = db.Conn.QueryRow(
		"SELECT agent_name, prompt, exit_code FROM agent_history WHERE agent_name = ?",
		"copilot",
	).Scan(&gotAgent, &gotPrompt, &gotExit)
	require.NoError(t, err, "inserted row should be queryable")

	assert.Equal(t, "copilot", gotAgent)
	assert.Equal(t, "fix the null pointer dereference", gotPrompt)
	assert.Equal(t, 0, gotExit)
}

// TestLogAgentRun_WithWorktreeID verifies that LogAgentRun correctly stores
// a non-nil WorktreeID foreign key.
func TestLogAgentRun_WithWorktreeID(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Insert a worktree to satisfy the FK constraint.
	res, err := db.Conn.Exec(
		"INSERT INTO worktrees (path, branch) VALUES (?, ?)",
		"/repo/feat", "feat/branch",
	)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)

	wtID := id
	entry := data.AgentHistoryEntry{
		AgentName:  "copilot",
		WorktreeID: &wtID,
		Prompt:     "add unit tests",
		ExitCode:   0,
		StartedAt:  time.Now(),
		EndedAt:    time.Now(),
	}

	err = data.LogAgentRun(db, entry)
	require.NoError(t, err)

	// Verify worktree_id was stored.
	var gotWorktreeID int64
	err = db.Conn.QueryRow(
		"SELECT worktree_id FROM agent_history WHERE agent_name = ?", "copilot",
	).Scan(&gotWorktreeID)
	require.NoError(t, err)
	assert.Equal(t, wtID, gotWorktreeID)
}

// TestLogAgentRun_NonZeroExitCode verifies that a non-zero exit code is stored.
func TestLogAgentRun_NonZeroExitCode(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	entry := data.AgentHistoryEntry{
		AgentName: "copilot",
		Prompt:    "suggest fix",
		ExitCode:  1,
		StartedAt: time.Now(),
		EndedAt:   time.Now(),
	}

	require.NoError(t, data.LogAgentRun(db, entry))

	var gotExit int
	require.NoError(t, db.Conn.QueryRow(
		"SELECT exit_code FROM agent_history WHERE agent_name = ?", "copilot",
	).Scan(&gotExit))
	assert.Equal(t, 1, gotExit)
}

// TestLogAgentRun_ClosedDB verifies that a closed DB returns a wrapped error
// containing the "log agent run" context string.
func TestLogAgentRun_ClosedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	// Close immediately to make the connection invalid.
	require.NoError(t, db.Close())

	entry := data.AgentHistoryEntry{
		AgentName: "copilot",
		Prompt:    "test",
		ExitCode:  0,
		StartedAt: time.Now(),
		EndedAt:   time.Now(),
	}

	err = data.LogAgentRun(db, entry)
	require.Error(t, err, "LogAgentRun on a closed DB should return an error")
	assert.Contains(t, err.Error(), "log agent run",
		"error should include 'log agent run' context string")
}

// TestGetAgentHistory verifies the GetAgentHistory function across a range of scenarios.
func TestGetAgentHistory(t *testing.T) {
	t.Run("empty DB returns empty slice", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		runs, err := data.GetAgentHistory(db)
		require.NoError(t, err)
		assert.Empty(t, runs)
	})

	t.Run("single inserted run returns correct fields", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		now := time.Now().UTC().Truncate(time.Second)
		entry := data.AgentHistoryEntry{
			AgentName: "copilot",
			Prompt:    "fix the null pointer",
			ExitCode:  0,
			StartedAt: now,
			EndedAt:   now.Add(5 * time.Second),
		}
		require.NoError(t, data.LogAgentRun(db, entry))

		runs, err := data.GetAgentHistory(db)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, "copilot", runs[0].AgentName)
		assert.Equal(t, "fix the null pointer", runs[0].Prompt)
		assert.Equal(t, now, runs[0].StartedAt)
	})

	t.Run("multiple runs returns all of them", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		for i := 0; i < 3; i++ {
			entry := data.AgentHistoryEntry{
				AgentName: "copilot",
				Prompt:    "prompt",
				ExitCode:  0,
				StartedAt: time.Now(),
				EndedAt:   time.Now(),
			}
			require.NoError(t, data.LogAgentRun(db, entry))
		}

		runs, err := data.GetAgentHistory(db)
		require.NoError(t, err)
		assert.Len(t, runs, 3)
		// Spot-check that the agent name is correct.
		assert.Equal(t, "copilot", runs[0].AgentName)
	})

	t.Run("run with NULL prompt returns empty string", func(t *testing.T) {
		db, err := data.NewDB(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })

		// Insert a row with a NULL prompt directly to bypass AgentHistoryEntry.
		_, err = db.Conn.Exec(
			`INSERT INTO agent_history (agent_name, prompt, exit_code, started_at, ended_at)
			 VALUES (?, NULL, ?, ?, ?)`,
			"claude", 0, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
		)
		require.NoError(t, err)

		runs, err := data.GetAgentHistory(db)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, "claude", runs[0].AgentName)
		assert.Equal(t, "", runs[0].Prompt, "NULL prompt should be empty string")
	})
}
