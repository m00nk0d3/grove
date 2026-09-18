package domain

type MissionControlState struct {
	Status  string
	Details map[string]interface{}
}

type WorkItem struct {
	ID        string
	CreatedAt int64
	UpdatedAt int64
}

type WorkflowRunRef struct {
	WorkflowID string
	RunID    string
}

type AgentRef struct {
	AgentID string
}

type PaneRef struct {
	PaneID string
}

type IntegrationStatus struct {
	Status  string
	Message string
}

type ExternalIntegration struct {
	Name        string
	Description string
	URL         string
}

type Warning struct {
	Message string
	Level   string
}

// Normalized enums
const (
	UnknownState = "unknown"
	DegradedState = "degraded"
)