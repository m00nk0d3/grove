package data_test

import (
	"testing"
	"time"

	"github.com/m00nk0d3/nexus/internal/data"
	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSession is a helper that returns a minimal valid session for the given path.
func newSession(worktreePath string) domain.Session {
	return domain.Session{
		WorktreePath: worktreePath,
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
}

// ptr helpers for optional fields.
func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// TestUpsertSession_InsertNew verifies that UpsertSession returns a positive ID
// and that the row can be retrieved afterwards.
func TestUpsertSession_InsertNew(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	s := newSession("/repo/main")
	id, err := data.UpsertSession(db, s)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	got, err := data.GetSessionByWorktree(db, "/repo/main")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/repo/main", got.WorktreePath)
	assert.Equal(t, domain.StatusActive, got.Status)
}

// TestUpsertSession_Replace inserts the same worktree_path twice and verifies
// that the second call replaces the first row (updated fields are returned).
func TestUpsertSession_Replace(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// First insert.
	s := newSession("/repo/feat")
	_, err = data.UpsertSession(db, s)
	require.NoError(t, err)

	// Second insert with same path but different fields.
	s2 := domain.Session{
		WorktreePath: "/repo/feat",
		AgentName:    strPtr("copilot"),
		AgentPID:     intPtr(9999),
		Status:       domain.StatusAgentRunning,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, s2)
	require.NoError(t, err)

	got, err := data.GetSessionByWorktree(db, "/repo/feat")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.StatusAgentRunning, got.Status)
	require.NotNil(t, got.AgentName)
	assert.Equal(t, "copilot", *got.AgentName)
	require.NotNil(t, got.AgentPID)
	assert.Equal(t, 9999, *got.AgentPID)
}

// TestGetSessions_Empty verifies that an empty database returns an empty slice
// and no error.
func TestGetSessions_Empty(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sessions, err := data.GetSessions(db)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

// TestGetSessions_Multiple inserts two sessions and verifies both are returned
// with the correct field values.
func TestGetSessions_Multiple(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	s1 := domain.Session{
		WorktreePath: "/repo/alpha",
		ShellPID:     intPtr(1001),
		Prompt:       strPtr("fix the bug"),
		Status:       domain.StatusActive,
		StartedAt:    time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
	}
	s2 := domain.Session{
		WorktreePath: "/repo/beta",
		ShellPID:     intPtr(1002),
		Status:       domain.StatusAgentDone,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}

	_, err = data.UpsertSession(db, s1)
	require.NoError(t, err)
	_, err = data.UpsertSession(db, s2)
	require.NoError(t, err)

	sessions, err := data.GetSessions(db)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Ordered DESC by started_at: s2 is newer so comes first.
	assert.Equal(t, "/repo/beta", sessions[0].WorktreePath)
	assert.Equal(t, domain.StatusAgentDone, sessions[0].Status)
	assert.Equal(t, "/repo/alpha", sessions[1].WorktreePath)
	require.NotNil(t, sessions[1].Prompt)
	assert.Equal(t, "fix the bug", *sessions[1].Prompt)
}

// TestGetSessionByWorktree_Found inserts a session and retrieves it by path.
func TestGetSessionByWorktree_Found(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	s := domain.Session{
		WorktreePath: "/repo/target",
		ShellPID:     intPtr(4242),
		Prompt:       strPtr("add tests"),
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, s)
	require.NoError(t, err)

	got, err := data.GetSessionByWorktree(db, "/repo/target")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "/repo/target", got.WorktreePath)
	require.NotNil(t, got.ShellPID)
	assert.Equal(t, 4242, *got.ShellPID)
	require.NotNil(t, got.Prompt)
	assert.Equal(t, "add tests", *got.Prompt)
}

// TestGetSessionByWorktree_NotFound verifies that a missing path returns nil
// with no error.
func TestGetSessionByWorktree_NotFound(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	got, err := data.GetSessionByWorktree(db, "/repo/nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestDeleteSession inserts a session then deletes it and verifies the row is gone.
func TestDeleteSession(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	s := newSession("/repo/todelete")
	id, err := data.UpsertSession(db, s)
	require.NoError(t, err)

	err = data.DeleteSession(db, id)
	require.NoError(t, err)

	got, err := data.GetSessionByWorktree(db, "/repo/todelete")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestDeleteDeadSessions inserts active and dead sessions and verifies only the
// dead ones are removed.
func TestDeleteDeadSessions(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	active := domain.Session{
		WorktreePath: "/repo/alive",
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	dead := domain.Session{
		WorktreePath: "/repo/dead",
		Status:       domain.StatusDead,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}

	_, err = data.UpsertSession(db, active)
	require.NoError(t, err)
	_, err = data.UpsertSession(db, dead)
	require.NoError(t, err)

	err = data.DeleteDeadSessions(db)
	require.NoError(t, err)

	sessions, err := data.GetSessions(db)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "/repo/alive", sessions[0].WorktreePath)
}

// TestGetSessions_OrderedByStartedAt verifies sessions are returned with the
// most recently started session first (DESC started_at).
func TestGetSessions_OrderedByStartedAt(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC().Truncate(time.Second)
	older := domain.Session{
		WorktreePath: "/repo/older",
		Status:       domain.StatusActive,
		StartedAt:    now.Add(-2 * time.Hour),
	}
	middle := domain.Session{
		WorktreePath: "/repo/middle",
		Status:       domain.StatusActive,
		StartedAt:    now.Add(-time.Hour),
	}
	newer := domain.Session{
		WorktreePath: "/repo/newer",
		Status:       domain.StatusActive,
		StartedAt:    now,
	}

	for _, s := range []domain.Session{older, middle, newer} {
		_, err := data.UpsertSession(db, s)
		require.NoError(t, err)
	}

	sessions, err := data.GetSessions(db)
	require.NoError(t, err)
	require.Len(t, sessions, 3)
	assert.Equal(t, "/repo/newer", sessions[0].WorktreePath)
	assert.Equal(t, "/repo/middle", sessions[1].WorktreePath)
	assert.Equal(t, "/repo/older", sessions[2].WorktreePath)
}

// TestUpsertSession_ClosedDB verifies that operating on a closed DB returns a
// wrapped error containing "upsert session".
func TestUpsertSession_ClosedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = data.UpsertSession(db, newSession("/repo/closed"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert session")
}

// TestDeleteSession_ClosedDB verifies that operating on a closed DB returns a
// wrapped error containing "delete session".
func TestDeleteSession_ClosedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = data.DeleteSession(db, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete session")
}

// TestGetSessions_ClosedDB verifies that operating on a closed DB returns a
// wrapped error containing "get sessions".
func TestGetSessions_ClosedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = data.GetSessions(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get sessions")
}

// TestGetSessionByWorktree_ClosedDB verifies that operating on a closed DB
// returns a wrapped error containing "get session by worktree".
func TestGetSessionByWorktree_ClosedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = data.GetSessionByWorktree(db, "/repo/closed")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get session by worktree")
}

// TestDeleteDeadSessions_ClosedDB verifies that operating on a closed DB
// returns a wrapped error containing "delete dead sessions".
func TestDeleteDeadSessions_ClosedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	err = data.DeleteDeadSessions(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete dead sessions")
}
