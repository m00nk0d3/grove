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
	Correlations Correlations
}

type WorkItem struct {
	ID              string
	CreatedAt       int64
	UpdatedAt       int64
	LinkedPR        *PullRequest `json:"linked_pr,omitempty"`
	LinkedIssue     *Issue       `json:"linked_issue,omitempty"`
}

type WorkflowRunRef struct {
	WorkflowID string
	RunID      string
}

type AgentRef struct {
	AgentID string
}

type PaneRef struct {
	PaneID string
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

// Phase 2: Correlations metadata for tracking linkage relationships
type Correlations struct {
	WorktreeToPR      map[string]string // worktree path -> linked PR number or ""
	WorktreeToIssue   map[string]string // worktree path -> linked issue number or ""
	WorkflowToWorktree map[string]string // workflow ID -> linked worktree path
	AgentToWorkflow   map[string]string // agent ID -> linked workflow ID
	PaneToAgent       map[string]string // pane ID -> linked agent ID (empty if linked to workitem directly)
}
