package data

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/m00nk0d3/nexus/internal/domain"
)

// DefaultCopilotDBPath returns the path to the Copilot CLI session-store database.
// Falls back to the current directory if the home directory cannot be determined.
func DefaultCopilotDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".copilot", "session-store.db")
}

// GetActiveCopilotSessions reads the Copilot CLI session-store at storePath and
// returns one domain.Session per unique CWD for sessions created within the
// activityWindow. Only the most recent session per CWD is included.
//
// Returns nil (no error) if the database does not exist — this lets the caller
// treat a missing Copilot installation as a no-op rather than an error.
func GetActiveCopilotSessions(storePath string, activityWindow time.Duration) ([]domain.Session, error) {
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		return nil, nil
	}

	db, err := sql.Open("sqlite", storePath)
	if err != nil {
		return nil, fmt.Errorf("open copilot session store: %w", err)
	}
	defer db.Close()

	since := time.Now().UTC().Add(-activityWindow).Format(time.RFC3339)

	// ORDER BY cwd, created_at DESC so that the first row we see for each CWD
	// is always the most recent session.
	rows, err := db.Query(`
		SELECT cwd, COALESCE(summary, ''), created_at
		FROM sessions
		WHERE created_at >= ?
		ORDER BY cwd, created_at DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("query copilot sessions: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var result []domain.Session
	for rows.Next() {
		var cwd, summary, createdAtStr string
		if err := rows.Scan(&cwd, &summary, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scan copilot session: %w", err)
		}
		// Normalise to OS-native separators before dedup and storage so that
		// mixed-separator paths from the Copilot CLI match paths returned by
		// git worktree list (e.g. "D:/dev/foo" vs "D:\dev\foo" on Windows).
		normalised := filepath.Clean(cwd)
		if seen[normalised] {
			continue
		}
		seen[normalised] = true

		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)

		agentName := "copilot"
		s := domain.Session{
			WorktreePath: normalised,
			AgentName:    &agentName,
			Status:       domain.StatusAgentRunning,
			StartedAt:    createdAt,
		}
		if summary != "" {
			s.Prompt = &summary
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate copilot sessions: %w", err)
	}
	return result, nil
}
