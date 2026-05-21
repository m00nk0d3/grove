package data

import (
	"database/sql"
	"fmt"
	"time"
)

// CacheTable is the set of known SQLite tables that support TTL staleness checks.
// Using a typed constant prevents arbitrary strings from being passed to IsCacheStale,
// which builds its query via fmt.Sprintf.
type CacheTable string

const (
	CacheTablePRs    CacheTable = "github_prs"
	CacheTableIssues CacheTable = "github_issues"
)

// IsCacheStale reports whether the most recent synced_at for repoPath in the
// given table is older than ttl, or no rows exist for that repo (stale = needs refresh).
func IsCacheStale(db *DB, table CacheTable, ttl time.Duration, repoPath string) (bool, error) {
	var raw sql.NullString
	query := fmt.Sprintf("SELECT MAX(synced_at) FROM %s WHERE repo_path = ?", table)
	if err := db.Conn.QueryRow(query, repoPath).Scan(&raw); err != nil {
		return true, fmt.Errorf("is cache stale %s: %w", table, err)
	}
	if !raw.Valid || raw.String == "" {
		return true, nil // table is empty
	}
	t, err := time.Parse("2006-01-02 15:04:05", raw.String)
	if err != nil {
		return true, fmt.Errorf("is cache stale %s: parse time %q: %w", table, raw.String, err)
	}
	return time.Since(t) > ttl, nil
}
