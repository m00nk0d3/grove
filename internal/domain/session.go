package domain

import "time"

// SessionStatus represents the lifecycle state of an active session.
type SessionStatus string

const (
	// StatusActive means the shell is open and no agent is running.
	StatusActive SessionStatus = "active"
	// StatusAgentRunning means an AI agent process is currently executing.
	StatusAgentRunning SessionStatus = "agent_running"
	// StatusAgentDone means the agent finished successfully.
	StatusAgentDone SessionStatus = "agent_done"
	// StatusAgentFailed means the agent process exited with an error.
	StatusAgentFailed SessionStatus = "agent_failed"
	// StatusDead means the shell process is no longer alive.
	StatusDead SessionStatus = "dead"
)

// Session represents an active terminal session tracked by Nexus.
type Session struct {
	ID           int64
	WorktreePath string
	ShellPID     *int
	AgentName    *string
	AgentPID     *int
	Prompt       *string
	Status       SessionStatus
	StartedAt    time.Time
}
