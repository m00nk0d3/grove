package herdr

import "github.com/m00nk0d3/grove/internal/domain"

// Snapshot is the normalized Herdr state consumed by mission control.
type Snapshot struct {
	Integration domain.ExternalIntegration
	Panes       []domain.PaneRef
	Agents      []domain.AgentRef
	Workspaces  []Workspace
	Worktrees   []Worktree
}

// Workspace is a Herdr workspace. The Herdr list API exposes no filesystem
// path for a workspace, so Path stays empty by design.
type Workspace struct {
	Name string
	Path string
}

// Worktree is an adapter-local representation of a Herdr worktree. It is not
// domain.Worktree: the Herdr list API does not expose Grove's Git-tracked
// worktree fields, so faking them would corrupt the domain model (ADR-0001).
type Worktree struct {
	Path            string
	Branch          string
	Label           string
	OpenWorkspaceID string
}
