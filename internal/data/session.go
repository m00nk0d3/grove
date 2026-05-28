package data

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// UpsertSession inserts or replaces a session row in active_sessions.
// When worktree_path already exists the old row is replaced atomically.
// Returns the row ID of the inserted/replaced record.
func UpsertSession(db *DB, s domain.Session) (int64, error) {
	var startedAt interface{}
	if !s.StartedAt.IsZero() {
		// Store as RFC3339 — modernc.org/sqlite canonicalises datetime text to
		// this format when reading back, so we match it on the write side too.
		startedAt = s.StartedAt.UTC().Format(time.RFC3339)
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339)

	res, err := db.Conn.Exec(
		`INSERT OR REPLACE INTO active_sessions
		 (worktree_path, shell_pid, agent_name, prompt, status, started_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.WorktreePath,
		s.ShellPID,
		s.AgentName,
		s.Prompt,
		string(s.Status),
		startedAt,
		updatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("upsert session: %w", err)
	}
	return id, nil
}

// GetSessions returns all session rows ordered by started_at DESC.
// An empty database returns an empty slice and no error.
func GetSessions(db *DB) ([]domain.Session, error) {
	rows, err := db.Conn.Query(
		`SELECT id, worktree_path, shell_pid, agent_name, prompt, status, started_at, updated_at
		 FROM active_sessions
		 ORDER BY started_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}
	defer rows.Close()

	var sessions []domain.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("get sessions: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}
	if sessions == nil {
		sessions = []domain.Session{}
	}
	return sessions, nil
}

// GetSessionByWorktree returns the session for worktreePath, or nil (no error)
// when no row matches.
func GetSessionByWorktree(db *DB, worktreePath string) (*domain.Session, error) {
	row := db.Conn.QueryRow(
		`SELECT id, worktree_path, shell_pid, agent_name, prompt, status, started_at, updated_at
		 FROM active_sessions
		 WHERE LOWER(worktree_path) = LOWER(?)`,
		worktreePath,
	)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by worktree: %w", err)
	}
	return &s, nil
}

// DeleteSession deletes the session with the given id.
func DeleteSession(db *DB, id int64) error {
	_, err := db.Conn.Exec(`DELETE FROM active_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteDeadSessions removes all session rows whose status is 'dead'.
// It is a batch convenience for callers that want to purge dead sessions outside
// the normal health-check loop (e.g. on startup, in integration tests, or from a
// maintenance CLI). The session health-check tick in checkSessionsCmd uses
// DeleteSession per-row instead so it can log each individual failure.
func DeleteDeadSessions(db *DB) error {
	_, err := db.Conn.Exec(`DELETE FROM active_sessions WHERE status = ?`, string(domain.StatusDead))
	if err != nil {
		return fmt.Errorf("delete dead sessions: %w", err)
	}
	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanSession reads one row from s into a domain.Session.
// Returns sql.ErrNoRows (unwrapped) when no row was found so callers can
// distinguish "not found" from other errors.
func scanSession(s rowScanner) (domain.Session, error) {
	var (
		sess      domain.Session
		shellPID  sql.NullInt64
		agentName sql.NullString
		prompt    sql.NullString
		startedAt sql.NullString
		updatedAt sql.NullString
	)
	err := s.Scan(
		&sess.ID,
		&sess.WorktreePath,
		&shellPID,
		&agentName,
		&prompt,
		&sess.Status,
		&startedAt,
		&updatedAt,
	)
	if err != nil {
		return domain.Session{}, err
	}
	if shellPID.Valid {
		v := int(shellPID.Int64)
		sess.ShellPID = &v
	}
	if agentName.Valid {
		sess.AgentName = &agentName.String
	}
	if prompt.Valid {
		sess.Prompt = &prompt.String
	}
	if startedAt.Valid && startedAt.String != "" {
		t, err := parseSessionTime(startedAt.String)
		if err != nil {
			return domain.Session{}, fmt.Errorf("parse started_at %q: %w", startedAt.String, err)
		}
		sess.StartedAt = t
	}
	if updatedAt.Valid && updatedAt.String != "" {
		t, err := parseSessionTime(updatedAt.String)
		if err != nil {
			return domain.Session{}, fmt.Errorf("parse updated_at %q: %w", updatedAt.String, err)
		}
		sess.UpdatedAt = t
	}
	return sess, nil
}

// parseSessionTime parses a datetime string using the formats that
// modernc.org/sqlite may return for DATETIME columns.
// It tries RFC3339 (the canonical form written by this package and returned by
// the driver) and the plain SQLite text format used by CURRENT_TIMESTAMP.
func parseSessionTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised datetime format: %q", s)
}
