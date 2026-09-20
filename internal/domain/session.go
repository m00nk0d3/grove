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

// SessionRuntime identifies how a session is backed.
type SessionRuntime string

const (
	// RuntimeLocal is the default for terminal-launched sessions.
	RuntimeLocal SessionRuntime = "local"
	// RuntimeHerdr indicates a Herdr-backed pane session.
	RuntimeHerdr SessionRuntime = "herdr"
)

// Session represents an active terminal session tracked by Nexus.
type Session struct {
	ID             int64
	WorktreePath   string
	Runtime        SessionRuntime
	RuntimeID      string
	ShellPID       *int
	AgentName      *string
	WorkflowRunID  *string
	WorkspaceID    *string
	TabID          *string
	PaneID         *string
	Prompt         *string
	DegradedReason *string
	Status         SessionStatus
	StartedAt      time.Time
	UpdatedAt      time.Time
}
