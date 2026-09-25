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

// MissionReportsRequestedMsg asks Grove to load the reports the inspected
// workflow has written. Grove answers with MissionReportsLoadedMsg.
type MissionReportsRequestedMsg struct {
	RunID    string
	Workflow domain.WorkflowRunRef
}

// MissionReportsLoadedMsg carries the reports found for a workflow run.
// Supported is false for a workflow kind that never writes reports, so the
// inspector can tell that apart from reports that are not written yet.
type MissionReportsLoadedMsg struct {
	RunID     string
	Reports   []domain.WorkflowReport
	Supported bool
	Err       error
}

// ownMsg is implemented by messages a modal schedules for itself, such as a
// timer that clears its status line. The app routes them back to the open
// modal instead of treating them as app events.
type ownMsg interface{ modalOwned() }

// IsOwnMessage reports whether msg was scheduled by a modal for itself and
// belongs to the open modal.
func IsOwnMessage(msg tea.Msg) bool {
	_, ok := msg.(ownMsg)
	return ok
}

func (clearStatusMsg) modalOwned() {}

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
	WorkflowKindAddress   = "address"
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
	ContextActionMarkDone   = "mark-done"
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
