package data

import (
	"database/sql"
	"fmt"
	"time"
)

// AgentHistoryEntry holds the data for a single AI agent invocation to be
// persisted in the agent_history table.
type AgentHistoryEntry struct {
	AgentName  string // Name of the agent, e.g. "copilot"
	WorktreeID *int64 // Optional FK to worktrees.id; nil if not linked
	Prompt     string // The prompt that was sent to the agent
	ExitCode   int    // Process exit code (0 = success)
	StartedAt  time.Time
	EndedAt    time.Time
}

// AgentRun is a lightweight read model for agent history entries, used by the
// fuzzy finder to surface recent AI agent invocations.
type AgentRun struct {
	AgentName string
	Prompt    string
	StartedAt time.Time
}

// LogAgentRun inserts an agent invocation record into the agent_history table.
// Returns a wrapped error on failure so callers can detect "log agent run" context.
func LogAgentRun(db *DB, entry AgentHistoryEntry) error {
	_, err := db.Conn.Exec(
		`INSERT INTO agent_history (agent_name, worktree_id, prompt, exit_code, started_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.AgentName,
		entry.WorktreeID,
		entry.Prompt,
		entry.ExitCode,
		entry.StartedAt,
		entry.EndedAt,
	)
	if err != nil {
		return fmt.Errorf("log agent run: %w", err)
	}
	return nil
}

// GetAgentHistory returns up to the last 50 agent history entries ordered by
// most-recent first (ORDER BY id DESC LIMIT 50). An empty database returns an
// empty slice and no error. NULL prompt values are returned as empty strings;
// NULL or missing started_at values are returned as the zero time.Time.
func GetAgentHistory(db *DB) ([]AgentRun, error) {
	rows, err := db.Conn.Query(
		`SELECT agent_name, prompt, started_at
		 FROM agent_history
		 ORDER BY id DESC
		 LIMIT 50`,
	)
	if err != nil {
		return nil, fmt.Errorf("get agent history: %w", err)
	}
	defer rows.Close()

	var runs []AgentRun
	for rows.Next() {
		var (
			agentName string
			prompt    sql.NullString
			startedAt sql.NullString
		)
		if err := rows.Scan(&agentName, &prompt, &startedAt); err != nil {
			return nil, fmt.Errorf("get agent history: scan row: %w", err)
		}
		run := AgentRun{
			AgentName: agentName,
		}
		if prompt.Valid {
			run.Prompt = prompt.String
		}
		if startedAt.Valid && startedAt.String != "" {
			if t, err := parseSessionTime(startedAt.String); err == nil {
				run.StartedAt = t
			}
			// on parse error, StartedAt stays as zero time — non-fatal
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get agent history: %w", err)
	}
	if runs == nil {
		runs = []AgentRun{}
	}
	return runs, nil
}
