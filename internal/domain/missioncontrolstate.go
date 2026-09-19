package domain

import "time"

type MissionControlState struct {
	Status       string
	Details      map[string]interface{}
	RepoPath     string
	WorkItems    []WorkItem
	Worktrees    []Worktree
	Issues       []Issue
	PullRequests []PullRequest
	Sessions     []Session
	WorkflowRuns []WorkflowRunRef
	Agents       []AgentRef
	Panes        []PaneRef
	Integrations IntegrationStatus
	Warnings     []Warning
	UpdatedAt    time.Time
}

type WorkItem struct {
	ID              string
	CreatedAt       int64
	UpdatedAt       int64
	LinkedPR        *PullRequest     `json:"linked_pr,omitempty"`
	LinkedIssue     *Issue           `json:"linked_issue,omitempty"`
	LinkedWorkflows []WorkflowRunRef `json:"linked_workflows,omitempty"`
	LinkedAgents    []AgentRef       `json:"linked_agents,omitempty"`
	LinkedPanes     []PaneRef        `json:"linked_panes,omitempty"`
}

type WorkflowRunRef struct {
	WorkflowID   string
	RunID        string
	WorktreePath string
	Branch       string
	Status       string
	IssueNumber  *int
	PRNumber     *int
	Labels       []string
}

type AgentRef struct {
	AgentID       string
	Name          string
	WorkflowRunID string
	Status        string
}

type PaneRef struct {
	PaneID  string
	AgentID string
	CWD     string
	Command string
	Status  string
}

type IntegrationStatus struct {
	Herdr      ExternalIntegration
	Sandcastle ExternalIntegration
	GitHub     ExternalIntegration
}

type ExternalIntegration struct {
	Name        string
	Description string
	URL         string
	Available   bool
	Enabled     bool
	Mode        string
	Version     string
	Error       string
	LastSync    time.Time
}

type Warning struct {
	Message string
	Level   string
}

// Normalized enums
const (
	UnknownState  = "unknown"
	DegradedState = "degraded"
)
