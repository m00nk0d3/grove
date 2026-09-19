package herdr

import "github.com/m00nk0d3/grove/internal/domain"

// Snapshot is the normalized Herdr state consumed by mission control.
// Adapter-specific data will be added when the Herdr adapter is implemented.
type Snapshot struct {
	Integration domain.ExternalIntegration
	Panes       []domain.PaneRef
	Agents      []domain.AgentRef
	Workspaces  []Workspace
}

type Workspace struct {
	Name string
	Path string
}
