package domain

import "time"

// SessionStatus represents the lifecycle state of an active session.
type SessionStatus string

const (
	// StatusActive means the terminal is open at this worktree.
	StatusActive SessionStatus = "active"
	// StatusDead means the terminal process is no longer alive.
	StatusDead SessionStatus = "dead"
)

// Session represents an active terminal session tracked by Nexus.
type Session struct {
	ID           int64
	WorktreePath string
	ShellPID     *int
	AgentName    *string
	Prompt       *string
	Status       SessionStatus
	StartedAt    time.Time
	UpdatedAt    time.Time
}
