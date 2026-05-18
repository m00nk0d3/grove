package modal

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/domain"
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
	Path string
}

// ModalCancelledMsg is sent when the user cancels a modal (Esc or 'n').
type ModalCancelledMsg struct{}

// AiderLaunchMsg is sent when the user confirms Aider file selection.
type AiderLaunchMsg struct {
	Files []string
}

// Agent name constants — used in SpawnAgentMsg.AgentName and the app dispatch switch.
const (
	AgentNameClaude  = "claude"
	AgentNameCopilot = "copilot"
	AgentNameAider   = "aider"
)

// SpawnAgentMsg is sent when the user confirms spawning an AI agent from the launcher.
type SpawnAgentMsg struct {
	AgentName    string   // AgentNameClaude, AgentNameCopilot, or AgentNameAider
	WorktreePath string   // Path to the worktree directory
	Prompt       string   // Prompt text (empty for Aider which uses file picker)
	Files        []string // Reserved for Aider file-picker; populated in follow-on issue
}
