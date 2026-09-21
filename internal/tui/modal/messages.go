package modal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

// Modal extends tea.Model with a Title for themed overlay rendering.
type Modal interface {
	tea.Model
	Title() string
}

// SettingsSavedMsg is dispatched after the settings screen saves a config change.
type SettingsSavedMsg struct {
	Config *domain.Config
}

// WorktreeCreateConfirmedMsg is sent when the user confirms creating a new worktree.
type WorktreeCreateConfirmedMsg struct {
	Branch     string
	Path       string
	BaseBranch string // empty means "main"
}

// PRWorktreeCreateConfirmedMsg is sent when the user confirms checking out a PR as a new worktree.
type PRWorktreeCreateConfirmedMsg struct {
	Branch string
	Path   string
}

// WorktreeDeleteConfirmedMsg is sent when the user confirms deleting a worktree.
type WorktreeDeleteConfirmedMsg struct {
	Path   string
	Branch string
}

// ModalCancelledMsg is sent when the user cancels a modal (Esc or 'n').
type ModalCancelledMsg struct{}

// MissionJumpMsg requests navigation to the inspected workflow's terminal.
type MissionJumpMsg struct {
	RunID string
}

// MissionRefreshMsg requests an immediate refresh of integration telemetry.
type MissionRefreshMsg struct{}

// MissionRetryRequestedMsg asks Grove to retry a failed inspected run.
type MissionRetryRequestedMsg struct {
	RunID string
}

// MissionRemoveRequestedMsg asks Grove to confirm removal of an inspected run.
type MissionRemoveRequestedMsg struct {
	RunID string
}

// WorkflowRemoveConfirmedMsg confirms stopping/removing a workflow.
type WorkflowRemoveConfirmedMsg struct {
	RunID string
	Stop  bool
}

// ParentWorktreeRequiredMsg is sent when the user tries to create a sub-issue worktree
// but the parent issue has no worktree yet. The app should re-open the create modal
// filtered to the parent issue so the user can create the parent worktree first.
type ParentWorktreeRequiredMsg struct {
	ParentNumber int
}

// UpdateConfirmedMsg is sent when the user confirms the self-update from the update modal.
type UpdateConfirmedMsg struct{}

const (
	WorkflowKindImplement = "imp"
	WorkflowKindReview    = "review"
	WorkflowKindResolve   = "resolve"
	WorkflowKindCI        = "ci"
	WorkflowKindClean     = "clean"
)

// WorkflowLaunchMsg requests a Grove-owned Sandcastle workflow.
type WorkflowLaunchMsg struct {
	Kind        string
	RepoPath    string
	AgentKind   string
	IssueNumber *int
	PRNumber    *int
}

const (
	ContextActionOpen       = "open"
	ContextActionOpenShell  = "open-shell"
	ContextActionClose      = "close-session"
	ContextActionDelete     = "delete-worktree"
	ContextActionOpenGitHub = "open-github"
	ContextActionInspect    = "inspect-mission"
	ContextActionRetryRun   = "retry-workflow"
	ContextActionRemoveRun  = "remove-workflow"
	ContextActionSyncGitHub = "sync-github"
)

// CandidateKind distinguishes the type of a cleanup candidate.
type CandidateKind int

const (
	CandidateWorktree CandidateKind = iota // a worktree whose branch is merged
	CandidateBranch                        // a merged branch with no active worktree
)

// CleanupCandidate represents a single item that can be removed in the cleanup flow.
type CleanupCandidate struct {
	Kind   CandidateKind
	Path   string // worktree path (only for CandidateWorktree)
	Branch string // branch name
}

// CleanupConfirmedMsg is sent when the user confirms deletion in the cleanup modal.
// Worktrees contains paths to remove; Branches contains branch names to delete.
type CleanupConfirmedMsg struct {
	Worktrees []string
	Branches  []string
}
