package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/nexus/internal/data"
	"github.com/m00nk0d3/nexus/internal/domain"
	internalexec "github.com/m00nk0d3/nexus/internal/exec"
	"github.com/m00nk0d3/nexus/internal/tui/modal"
	"github.com/m00nk0d3/nexus/internal/tui/styles"
	"github.com/m00nk0d3/nexus/internal/updater"
	"github.com/m00nk0d3/nexus/internal/version"
)

// worktreeOpDoneMsg carries the result of an add/remove worktree operation.
type worktreeOpDoneMsg struct {
	err error // Error during operation, if any
}

// worktreeSwitchedMsg carries the result of switching to a worktree.
type worktreeSwitchedMsg struct {
	err error // Error during switch, if any
}

// sessionSpawnedMsg carries the result of spawning a new terminal session.
type sessionSpawnedMsg struct {
	session domain.Session
	err     error
}

// githubSyncedMsg carries the result of a background GitHub PR/issue sync.
type githubSyncedMsg struct {
	prs      []domain.PullRequest
	issues   []domain.Issue
	err      error
	syncedAt time.Time
}

// syncTickMsg triggers the next periodic GitHub sync.
type syncTickMsg struct{}

// sessionTickMsg triggers the next periodic session health check.
type sessionTickMsg struct{}

// sessionStatusUpdatedMsg carries the updated sessions list after a health check.
type sessionStatusUpdatedMsg struct{ sessions []domain.Session }

// debouncedRenderMsg fires after the debounce delay to apply pending sync data.
type debouncedRenderMsg struct{}

// lazyLoadContextMsg fires after the hover delay to load worktree context.
type lazyLoadContextMsg struct {
	worktree domain.Worktree
}

// browserOpenErrMsg carries an error from opening an issue or PR in the browser.
type browserOpenErrMsg struct{ err error }

// agentDoneMsg is dispatched when an AI agent process exits.
// It carries enough information to log the run and update UI state.
type agentDoneMsg struct {
	agentName string
	prompt    string
	exitCode  int
	startedAt time.Time
	session   domain.Session // non-zero WorktreePath means a session was recorded
}

// aiderFilesFetchedMsg carries the result of listing modified files for the Aider file picker.
type aiderFilesFetchedMsg struct {
	worktreePath string
	files        []string
	err          error
}

// clearErrorMsg is dispatched after the 5-second auto-dismiss timer fires.
type clearErrorMsg struct{}

// updateCheckedMsg carries the result of the startup version check.
type updateCheckedMsg struct {
	info updater.ReleaseInfo
	err  error
}

// selfUpdateDoneMsg carries the result of a self-update attempt.
type selfUpdateDoneMsg struct{ err error }

// clearErrorCmd returns a Cmd that fires clearErrorMsg after 5 seconds.
func clearErrorCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}

// checkForUpdateCmd fires an async update check on startup.
func checkForUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := updater.CheckLatestRelease(ctx)
		return updateCheckedMsg{info: info, err: err}
	}
}

// selfUpdateCmd runs the self-update in the background.
func selfUpdateCmd(tagName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		err := updater.SelfUpdate(ctx, tagName)
		return selfUpdateDoneMsg{err: err}
	}
}

// sessionTickCmd schedules a sessionTickMsg after 3 seconds.
func sessionTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return sessionTickMsg{}
	})
}

// pollPIDFile waits up to timeout for a file at path to appear and contain a
// valid PID written by the spawned shell process. Returns the PID on success,
// or 0 if the file is absent or unreadable within the timeout.
func pollPIDFile(path string, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

// pidInitUnixCmd returns a POSIX sh one-liner that writes the shell's PID to
// pidFile and then exec-replaces itself with the user's default interactive
// shell. The terminal tab stays interactive; the PID is stable across the exec.
func pidInitUnixCmd(pidFile string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return fmt.Sprintf(`sh -c 'echo $$ > "%s"; exec "%s"'`, pidFile, shell)
}

// sessionKilledMsg carries the result of a kill-session operation.
type sessionKilledMsg struct {
	worktreePath string
	err          error
}

// sessionFocusedMsg carries the result of attempting to bring an existing
// terminal session to the foreground.
type sessionFocusedMsg struct {
	worktreePath string
	err          error
}

// msgAutoDismissDuration is how long the success/info toast stays visible before
// being cleared by clearMsgCmd.
const msgAutoDismissDuration = 3 * time.Second

// clearMsgMsg is dispatched after the success-notification timer fires.
type clearMsgMsg struct{}

// clearMsgCmd returns a Cmd that fires clearMsgMsg after msgAutoDismissDuration.
func clearMsgCmd() tea.Cmd {
	return tea.Tick(msgAutoDismissDuration, func(t time.Time) tea.Msg {
		return clearMsgMsg{}
	})
}

// debouncedRenderCmd schedules a debouncedRenderMsg after delay.
func debouncedRenderCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return debouncedRenderMsg{}
	})
}

// lazyLoadContextCmd fetches PR details for the selected worktree from SQLite
// after a short hover delay, avoiding expensive fetches on rapid navigation.
func (m *Model) lazyLoadContextCmd(worktree domain.Worktree) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return lazyLoadContextMsg{worktree: worktree}
	})
}

// maybeLazyLoadCmd returns a lazyLoadContextCmd if we're in the worktrees view
// and there is a selected worktree, otherwise nil.
func (m *Model) maybeLazyLoadCmd() tea.Cmd {
	if m.view != viewWorktrees {
		return nil
	}
	if selected, ok := m.selectedWorktree(); ok {
		return m.lazyLoadContextCmd(selected)
	}
	return nil
}

// activeView represents the currently active main panel view.
type activeView int

const (
	viewWorktrees activeView = iota // Shows the worktree list (default)
	viewIssues                      // Shows the GitHub issues list
	viewPRs                         // Shows the GitHub pull requests list
)

// focusedPanel identifies which panel currently has keyboard focus.
type focusedPanel int

const (
	panelNav   focusedPanel = iota // Left navigation rail (default focus)
	panelList                      // Main content list
	panelCtx                       // Right context panel
	panelCount                     // Sentinel — used for modular cycling via (p+1)%panelCount
)

const pageSize = 50

// Model represents the root Bubbletea model for the Nexus TUI application.
// It manages the list of git worktrees, user interactions, and active modals.
type Model struct {
	Worktrees        []domain.Worktree    // List of available git worktrees
	RepoPath         string               // Path to the repository root
	Config           *domain.Config       // Loaded application configuration
	selectedIdx      int                  // Currently selected worktree index
	activeModal      modal.Modal          // Currently open modal (if any)
	statusErr        string               // Error message to display (if any)
	statusMsg        string               // Success/info message to display (if any)
	themeIdx         int                  // Index into styles.Themes for the active theme
	view             activeView           // Currently active main panel view
	width            int                  // Terminal width in columns; 0 means use default
	height           int                  // Terminal height in rows; 0 means use default
	prs              []domain.PullRequest // Latest synced pull requests
	issues           []domain.Issue       // Latest synced issues
	lastSynced       time.Time            // When the last successful GitHub sync completed
	syncErr          error                // Error from the most recent GitHub sync attempt
	syncing          bool                 // True while a background GitHub sync is in progress
	selectedIssueIdx int                  // Currently selected issue index
	selectedPRIdx    int                  // Currently selected PR index
	focused          focusedPanel         // Which panel currently has keyboard focus
	ctxScrollOffset  int                  // Scroll position within the context panel

	// Pagination state
	currentPage int // 0-based current page index for issues/PRs lists

	// Debounce state
	pendingSync *githubSyncedMsg // pending sync data waiting for debounce timer

	// DB is optional; when non-nil, agent runs are logged to agent_history.
	db *data.DB

	// sessions holds the last-known list of active terminal sessions.
	sessions []domain.Session

	// latestVersion holds the latest release version discovered on startup (empty if check failed).
	latestVersion string
	// selfUpdating is true while a self-update is in progress.
	selfUpdating bool

	// issueTree caches the depth-first-ordered tree built from m.issues.
	// Rebuilt whenever m.issues is updated (debouncedRenderMsg handler).
	issueTree []issueTreeRow

	// Copilot prompt state
	copilotPromptActive bool            // true while the inline Copilot prompt is open
	copilotPromptInput  textinput.Model // text input for entering the Copilot prompt

	// Claude prompt state
	claudePromptActive bool            // true while the inline Claude prompt is open
	claudePromptInput  textinput.Model // text input for entering the Claude prompt
}

// NewModel creates and returns a new Model instance with all required fields initialized.
func NewModel() *Model {
	cfg, err := data.LoadConfig(data.DefaultConfigPath())

	var configErr string
	if err != nil {
		cfg = domain.DefaultConfig()
		configErr = fmt.Sprintf("config load failed: %v", err)
	}

	themeIdx := 0
	for i, name := range styles.Themes {
		if name == cfg.Appearance.Theme {
			themeIdx = i
			break
		}
	}

	return &Model{
		Config:    cfg,
		themeIdx:  themeIdx,
		statusErr: configErr,
		focused:   panelList,
	}
}

// Init initializes the model and triggers an initial worktree list load and GitHub sync.
func (m *Model) Init() tea.Cmd {
	m.syncing = true
	// Always start the session tick — it handles both nexus-spawned shell sessions
	// (requires m.db) and externally-started Copilot CLI sessions (no DB needed).
	return tea.Batch(m.refreshWorktreesCmd(), m.syncGitHubCmd(), sessionTickCmd(), checkForUpdateCmd())
}

// Update handles incoming messages and returns an updated model and command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Route all messages to the active modal while one is open.
	if m.activeModal != nil {
		switch msg := msg.(type) {
		case modal.WorktreeCreateConfirmedMsg:
			m.activeModal = nil
			return m, m.addWorktreeCmd(msg.Branch, msg.Path, msg.BaseBranch)
		case modal.ParentWorktreeRequiredMsg:
			// Re-open create modal filtered to the parent issue so the user can
			// create the parent worktree first. Once done, they can press 'c' again
			// to create the sub-issue worktree (parent branch will exist by then).
			m.activeModal = nil
			for _, iss := range m.issues {
				if iss.Number == msg.ParentNumber {
					m.activeModal = modal.NewCreateModal([]domain.Issue{iss}, m.RepoPath)
					break
				}
			}
			return m, nil
		case modal.PRWorktreeCreateConfirmedMsg:
			m.activeModal = nil
			return m, m.checkoutPRWorktreeCmd(msg.Branch, msg.Path)
		case modal.WorktreeDeleteConfirmedMsg:
			m.activeModal = nil
			return m, m.removeWorktreeCmd(msg.Path)
		case modal.AiderLaunchMsg:
			m.activeModal = nil
			if selected, ok := m.selectedWorktree(); ok {
				return m, m.spawnAiderCmd(selected.Path, msg.Files)
			}
			return m, nil
		case modal.UpdateConfirmedMsg:
			m.activeModal = nil
			m.selfUpdating = true
			return m, selfUpdateCmd(m.latestVersion)
		case modal.SpawnAgentMsg:
			m.activeModal = nil
			switch msg.AgentName {
			case modal.AgentNameCopilot:
				return m, m.spawnCopilotCmd(msg.WorktreePath, msg.Prompt)
			case modal.AgentNameClaude:
				return m, m.spawnClaudeCmd(msg.WorktreePath, msg.Prompt)
			case modal.AgentNameAider:
				return m, m.fetchAiderFilesCmd(msg.WorktreePath)
			}
			return m, nil
		case modal.ModalCancelledMsg:
			m.activeModal = nil
			return m, nil
		case modal.SettingsSavedMsg:
			m.Config = msg.Config
			// Update themeIdx to match the saved theme.
			for i, name := range styles.Themes {
				if name == msg.Config.Appearance.Theme {
					m.themeIdx = i
					break
				}
			}
			// Stay in settings — pass the message on to the modal.
			updated, cmd := m.activeModal.Update(msg)
			if next, ok := updated.(modal.Modal); ok {
				m.activeModal = next
			}
			return m, cmd
		default:
			updated, cmd := m.activeModal.Update(msg)
			if next, ok := updated.(modal.Modal); ok {
				m.activeModal = next
			}
			return m, cmd
		}
	}

	// While the Copilot inline prompt is open, route key events to the textinput.
	// Non-key messages (e.g. agentDoneMsg, tea.WindowSizeMsg) fall through to
	// the main switch below so they are still handled correctly.
	if m.copilotPromptActive {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.Type {
			case tea.KeyEnter:
				prompt := strings.TrimSpace(m.copilotPromptInput.Value())
				m.copilotPromptActive = false
				if selected, ok := m.selectedWorktree(); ok {
					return m, m.spawnCopilotCmd(selected.Path, prompt)
				}
				m.copilotPromptInput.SetValue("")
				return m, nil
			case tea.KeyEsc:
				m.copilotPromptActive = false
				m.copilotPromptInput.SetValue("")
				return m, nil
			default:
				var cmd tea.Cmd
				m.copilotPromptInput, cmd = m.copilotPromptInput.Update(keyMsg)
				return m, cmd
			}
		}
		// Non-key message: fall through to the main switch to handle it normally.
	}

	// While the Claude inline prompt is open, route key events to the textinput.
	// Non-key messages fall through to the main switch below.
	if m.claudePromptActive {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.Type {
			case tea.KeyEnter:
				prompt := strings.TrimSpace(m.claudePromptInput.Value())
				m.claudePromptActive = false
				if selected, ok := m.selectedWorktree(); ok {
					return m, m.spawnClaudeCmd(selected.Path, prompt)
				}
				m.claudePromptInput.SetValue("")
				return m, nil
			case tea.KeyEsc:
				m.claudePromptActive = false
				m.claudePromptInput.SetValue("")
				return m, nil
			default:
				var cmd tea.Cmd
				m.claudePromptInput, cmd = m.claudePromptInput.Update(keyMsg)
				return m, cmd
			}
		}
		// Non-key message: fall through to the main switch to handle it normally.
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Dismiss any visible error overlay on the next keypress.
		m.statusErr = ""
		switch msg.Type {
		case tea.KeyTab:
			m.focused = (m.focused + 1) % panelCount
			return m, nil
		case tea.KeyEnter:
			switch m.view {
			case viewIssues:
				issue, ok := m.selectedIssue()
				if !ok {
					return m, nil
				}
				m.activeModal = modal.NewCreateModalForIssue(issue, m.RepoPath, computeParentBranches(m.issues, m.Worktrees)...)
				return m, nil
			case viewPRs:
				if len(m.prs) == 0 || m.selectedPRIdx >= len(m.prs) {
					return m, nil
				}
				pr := m.prs[m.selectedPRIdx]
				path := prWorktreePath(m.RepoPath, pr.Branch)
				// Guard: if any existing worktree already uses this branch, show an error.
				for _, wt := range m.Worktrees {
					if wt.Branch == pr.Branch {
						m.statusErr = fmt.Sprintf("Worktree for branch %q already exists at %s", pr.Branch, wt.Path)
						return m, clearErrorCmd()
					}
				}
				m.activeModal = modal.NewPRCheckoutModal(pr, path)
				return m, nil
			default:
				if selected, ok := m.selectedWorktree(); ok {
					// If a live session already exists for this worktree, focus it
					// instead of spawning a new one. Skip any stale entry whose
					// shell PID is confirmed dead so that closed terminals don't
					// block re-spawning. We no longer track AgentPID, so we accept
					// the small risk of a duplicate spawn if the agent outlived its
					// terminal window.
					for _, s := range m.sessions {
						if !pathsEqual(s.WorktreePath, selected.Path) {
							continue
						}
						if s.ShellPID != nil && !pidAlive(*s.ShellPID) {
							break // stale — fall through to spawn
						}
						return m, m.focusSessionCmd(s)
					}
					return m, m.spawnSessionCmd(selected.Path)
				}
				return m, nil
			}
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyF1:
			m.activeModal = modal.NewHelpModal()
			return m, nil
		case tea.KeyCtrlD:
			if selected, ok := m.selectedWorktree(); ok {
				m.activeModal = modal.NewDeleteModal(selected)
			}
		case tea.KeyUp:
			m.moveUp()
			return m, m.maybeLazyLoadCmd()
		case tea.KeyDown:
			m.moveDown()
			return m, m.maybeLazyLoadCmd()
		case tea.KeyPgDown:
			m.nextPage()
			return m, nil
		case tea.KeyPgUp:
			m.prevPage()
			return m, nil
		case tea.KeySpace:
			if m.view != viewWorktrees {
				m.statusErr = "Agent launcher is only available in the Worktrees view — press w to switch"
				return m, clearErrorCmd()
			}
			if selected, ok := m.selectedWorktree(); ok {
				m.activeModal = modal.NewAgentLauncherModal(m.Config, selected.Path)
				return m, nil
			}
			m.statusErr = "No worktree selected — select one first"
			return m, clearErrorCmd()
		case tea.KeyRunes:
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case " ":
				// Spacebar can arrive as KeyRunes " " on some terminals (e.g. Windows).
				// Mirror the KeySpace handler above.
				if m.view != viewWorktrees {
					m.statusErr = "Agent launcher is only available in the Worktrees view — press w to switch"
					return m, clearErrorCmd()
				}
				if selected, ok := m.selectedWorktree(); ok {
					m.activeModal = modal.NewAgentLauncherModal(m.Config, selected.Path)
					return m, nil
				}
				m.statusErr = "No worktree selected — select one first"
				return m, clearErrorCmd()
			case "?":
				m.activeModal = modal.NewHelpModal()
				return m, nil
			case "j":
				m.moveDown()
				return m, m.maybeLazyLoadCmd()
			case "k":
				m.moveUp()
				return m, m.maybeLazyLoadCmd()
			case "t":
				m.activeModal = modal.NewSettingsModal(m.Config, data.DefaultConfigPath())
			case "w", "W":
				m.view = viewWorktrees
				m.ctxScrollOffset = 0
				m.currentPage = 0
			case "i", "I":
				m.view = viewIssues
				m.ctxScrollOffset = 0
				m.currentPage = 0
			case "p", "P":
				m.view = viewPRs
				m.ctxScrollOffset = 0
				m.currentPage = 0
			case "n":
				m.nextPage()
				return m, nil
			case "g", "G":
				return m, m.openInBrowserCmd()
			case "s", "S":
				if m.view == viewWorktrees {
					if selected, ok := m.selectedWorktree(); ok {
						return m, m.spawnSessionCmd(selected.Path)
					}
					m.statusErr = "No worktree selected — select one first"
					return m, clearErrorCmd()
				}
			case "c", "C":
				if m.view != viewWorktrees {
					m.statusErr = "Copilot (c) is only available in the Worktrees view — press w to switch"
					return m, clearErrorCmd()
				}
				if !m.Config.AIAgents.CopilotEnabled {
					m.statusErr = "Copilot is disabled — set copilot_enabled = true in ~/.nexus/config.toml"
					return m, clearErrorCmd()
				}
				if _, ok := m.selectedWorktree(); !ok {
					m.statusErr = "No worktree selected — select one first"
					return m, clearErrorCmd()
				}
				if _, err := exec.LookPath("gh"); err != nil {
					m.statusErr = "gh not found on $PATH — install GitHub CLI to use Copilot"
					return m, clearErrorCmd()
				}
				ti := textinput.New()
				ti.Placeholder = "Enter Copilot prompt…"
				focusCmd := ti.Focus()
				m.copilotPromptInput = ti
				m.copilotPromptActive = true
				return m, focusCmd
			case "a", "A":
				if m.view != viewWorktrees {
					m.statusErr = "Claude (a) is only available in the Worktrees view — press w to switch"
					return m, clearErrorCmd()
				}
				if !m.Config.AIAgents.ClaudeEnabled {
					m.statusErr = "Claude is disabled — set claude_enabled = true in ~/.nexus/config.toml"
					return m, clearErrorCmd()
				}
				if _, ok := m.selectedWorktree(); !ok {
					m.statusErr = "No worktree selected — select one first"
					return m, clearErrorCmd()
				}
				if _, err := resolveClaudeBinary(m.Config); err != nil {
					m.statusErr = fmt.Sprintf("claude binary not found: %v", err)
					return m, clearErrorCmd()
				}
				ti := textinput.New()
				ti.Placeholder = "Enter Claude prompt…"
				focusCmd := ti.Focus()
				m.claudePromptInput = ti
				m.claudePromptActive = true
				return m, focusCmd
			case "f", "F":
				if m.view != viewWorktrees {
					m.statusErr = "Aider (f) is only available in the Worktrees view — press w to switch"
					return m, clearErrorCmd()
				}
				if !m.Config.AIAgents.AiderEnabled {
					m.statusErr = "Aider is disabled — set aider_enabled = true in ~/.nexus/config.toml"
					return m, clearErrorCmd()
				}
				selected, ok := m.selectedWorktree()
				if !ok {
					m.statusErr = "No worktree selected — select one first"
					return m, clearErrorCmd()
				}
				if _, err := resolveAiderBinary(m.Config); err != nil {
					m.statusErr = "aider not found on $PATH — install Aider to use this feature"
					return m, clearErrorCmd()
				}
				return m, m.fetchAiderFilesCmd(selected.Path)
			case "x", "X":
				if m.view != viewWorktrees {
					return m, nil
				}
				selected, ok := m.selectedWorktree()
				if !ok {
					return m, nil
				}
				for _, s := range m.sessions {
					if pathsEqual(s.WorktreePath, selected.Path) {
						return m, m.killSessionCmd(s)
					}
				}
				m.statusErr = "No active session for this worktree"
				return m, clearErrorCmd()
			}
		}

	case aiderFilesFetchedMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to list files: %v", msg.err)
			return m, clearErrorCmd()
		}
		m.activeModal = modal.NewAiderFilePicker(msg.files)
		return m, nil

	case worktreeOpDoneMsg:
		// Refresh the worktree list after an add/remove operation.
		// Surface any git error via the status error modal.
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Git operation failed: %v", msg.err)
			return m, tea.Batch(m.refreshWorktreesCmd(), clearErrorCmd())
		}
		return m, m.refreshWorktreesCmd()

	case worktreeSwitchedMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to switch worktree: %v", msg.err)
			return m, clearErrorCmd()
		}
		m.statusErr = ""
		// Refresh worktrees after switching back
		return m, m.refreshWorktreesCmd()

	case sessionSpawnedMsg:
		if msg.err != nil {
			if msg.session.ShellPID != nil {
				// Terminal launched but PID tracking failed — non-fatal.
				m.statusMsg = fmt.Sprintf("Session spawned for %s (PID %d) — tracking failed: %v", msg.session.WorktreePath, *msg.session.ShellPID, msg.err)
				return m, clearMsgCmd()
			}
			m.statusErr = fmt.Sprintf("Failed to spawn session: %v", msg.err)
			return m, clearErrorCmd()
		}
		// Immediately reflect the new session in m.sessions so the badge appears
		// without waiting for the next health-check tick.
		found := false
		for i := range m.sessions {
			if pathsEqual(m.sessions[i].WorktreePath, msg.session.WorktreePath) {
				m.sessions[i] = msg.session
				found = true
				break
			}
		}
		if !found {
			m.sessions = append(m.sessions, msg.session)
		}
		pid := 0
		if msg.session.ShellPID != nil {
			pid = *msg.session.ShellPID
		}
		m.statusMsg = fmt.Sprintf("Session spawned for %s (PID %d)", msg.session.WorktreePath, pid)
		return m, clearMsgCmd()

	case worktreesRefreshedMsg:
		if msg.err == nil {
			m.Worktrees = msg.worktrees
			m.clampSelectedIdx()
			// Always use the main worktree (first entry) as the canonical repo path
			// so the header shows the repo name rather than the current worktree dir.
			if len(msg.worktrees) > 0 {
				m.RepoPath = msg.worktrees[0].Path
			}
		}

	case browserOpenErrMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to open in browser: %v", msg.err)
			return m, clearErrorCmd()
		}

	case agentDoneMsg:
		m.copilotPromptActive = false
		m.copilotPromptInput.SetValue("")
		m.claudePromptActive = false
		m.claudePromptInput.SetValue("")
		if m.db != nil {
			entry := data.AgentHistoryEntry{
				AgentName: msg.agentName,
				Prompt:    msg.prompt,
				ExitCode:  msg.exitCode,
				StartedAt: msg.startedAt,
				EndedAt:   time.Now(),
			}
			if err := data.LogAgentRun(m.db, entry); err != nil {
				m.statusErr = fmt.Sprintf("failed to log agent run: %v", err)
			}
		}
		// Sync the recorded session into m.sessions so the badge appears immediately.
		if msg.session.WorktreePath != "" {
			updated := false
			for i := range m.sessions {
				if pathsEqual(m.sessions[i].WorktreePath, msg.session.WorktreePath) {
					m.sessions[i] = msg.session
					updated = true
					break
				}
			}
			if !updated {
				m.sessions = append(m.sessions, msg.session)
			}
		}
		if msg.exitCode > 1 {
			exitMsg := fmt.Sprintf("⚠ Agent exited with code %d", msg.exitCode)
			if m.statusErr != "" {
				m.statusErr = m.statusErr + "; " + exitMsg
			} else {
				m.statusErr = exitMsg
			}
		}
		if m.statusErr != "" {
			return m, tea.Batch(m.refreshWorktreesCmd(), clearErrorCmd())
		}
		return m, m.refreshWorktreesCmd()

	case githubSyncedMsg:
		// Store pending data and schedule debounce render instead of immediate update.
		m.pendingSync = &msg
		return m, debouncedRenderCmd(100 * time.Millisecond)

	case debouncedRenderMsg:
		if m.pendingSync != nil {
			pending := m.pendingSync
			m.pendingSync = nil
			m.syncing = false
			m.syncErr = pending.err
			if pending.err != nil {
				m.statusErr = fmt.Sprintf("GitHub sync failed: %v", pending.err)
			}
			if pending.err == nil {
				m.prs = pending.prs
				m.issues = pending.issues
				m.issueTree = buildIssueTree(m.issues)
				m.lastSynced = pending.syncedAt
				m.clampIssueIdx()
				m.clampPRIdx()
				// Re-link worktrees to PRs whenever PRs are refreshed.
				if m.db != nil {
					if linked, err := data.LinkWorktreesToPRs(m.db, m.Worktrees, m.prs); err == nil {
						m.Worktrees = linked
					}
				} else {
					m.Worktrees = data.LinkWorktreesToPRsInMemory(m.Worktrees, m.prs)
				}
			}
			nextTick := tea.Tick(m.Config.GitHub.SyncInterval(), func(t time.Time) tea.Msg {
				return syncTickMsg{}
			})
			if pending.err != nil {
				return m, tea.Batch(nextTick, clearErrorCmd())
			}
			return m, nextTick
		}

	case lazyLoadContextMsg:
		// Context data is loaded from SQLite on hover; currently a no-op placeholder
		// because worktree context is rendered directly from m.Worktrees.
		// This hook exists for future lazy-load enrichment.
		_ = msg

	case clearErrorMsg:
		m.statusErr = ""

	case clearMsgMsg:
		m.statusMsg = ""

	case syncTickMsg:
		m.syncing = true
		return m, m.syncGitHubCmd()

	case sessionTickMsg:
		return m, m.checkSessionsCmd()

	case sessionStatusUpdatedMsg:
		var live []domain.Session
		for _, s := range msg.sessions {
			if s.Status != domain.StatusDead {
				live = append(live, s)
			}
		}
		if live == nil {
			live = []domain.Session{}
		}
		m.sessions = live
		return m, sessionTickCmd()

	case sessionFocusedMsg:
		// Focus is best-effort; show a friendly toast regardless of outcome.
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Session active for %s (could not bring to front: %v)", msg.worktreePath, msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Focused session for %s", msg.worktreePath)
		}
		return m, clearMsgCmd()

	case sessionKilledMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Close session: %v", msg.err)
			return m, clearErrorCmd()
		}
		var updated []domain.Session
		for _, s := range m.sessions {
			if !pathsEqual(s.WorktreePath, msg.worktreePath) {
				updated = append(updated, s)
			}
		}
		if updated == nil {
			updated = []domain.Session{}
		}
		m.sessions = updated
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case updateCheckedMsg:
		if msg.err != nil {
			slog.Debug("update check failed", "err", msg.err)
			return m, nil
		}
		newer, err := updater.IsNewer(msg.info.TagName, version.Version)
		if err != nil || !newer {
			return m, nil
		}
		m.latestVersion = msg.info.TagName
		m.activeModal = modal.NewUpdateModal(version.Version, msg.info.TagName, msg.info.Body, msg.info.HTMLURL, len(m.sessions))
		return m, nil

	case selfUpdateDoneMsg:
		m.selfUpdating = false
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Update failed: %v", msg.err)
			return m, clearErrorCmd()
		}
		m.statusMsg = "✓ Updated successfully! Please restart nexus to use the new version."
		return m, clearMsgCmd()
	}

	return m, nil
}

// View returns a string representation of the model's current state.
func (m *Model) View() string {
	baseView := renderFull(m.Worktrees, m.selectedIdx, m.RepoPath, m.themeIdx, m.view, m.width, m.height, m.syncing, m.lastSynced, m.syncErr, m.issues, m.selectedIssueIdx, m.prs, m.selectedPRIdx, m.focused, m.ctxScrollOffset, m.currentPage, m.sessions)

	w, h := m.width, m.height
	if w <= 0 {
		w = defaultTermWidth
	}
	if h <= 0 {
		h = 24
	}

	// Overlay helpers — center a themed RenderBox over the full base view.
	overlay := func(title, content string) string {
		theme := styles.NewTheme(styles.Themes[m.themeIdx])
		box := theme.RenderBox(title, content, w)
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
	}

	if m.activeModal != nil {
		if wa, ok := m.activeModal.(interface{ SetWidth(int) }); ok {
			wa.SetWidth(w)
		}
		if ta, ok := m.activeModal.(interface{ SetTheme(styles.Theme) }); ok {
			ta.SetTheme(styles.NewTheme(styles.Themes[m.themeIdx]))
		}
		return overlay(m.activeModal.Title(), m.activeModal.View())
	}

	if m.copilotPromptActive {
		return overlay("Spawn Copilot",
			fmt.Sprintf("> %s\n\nEnter confirm (prompt optional)  •  Esc cancel", m.copilotPromptInput.View()))
	}

	if m.claudePromptActive {
		return overlay("Spawn Claude Code",
			fmt.Sprintf("> %s\n\nEnter confirm (prompt optional)  •  Esc cancel", m.claudePromptInput.View()))
	}

	if m.selfUpdating {
		return overlay("Updating nexus", "Downloading and installing update...\n\nPlease wait.")
	}

	if m.statusErr != "" {
		return renderErrorModal(m.statusErr, w, h, baseView)
	}

	if m.statusMsg != "" {
		return renderInfoModal(m.statusMsg, w, h, baseView)
	}

	return baseView
}

// openInBrowserCmd returns a Cmd that opens the selected issue or PR in the browser
// using the gh CLI. Returns nil when in viewWorktrees or when the relevant list is empty.
func (m *Model) openInBrowserCmd() tea.Cmd {
	switch m.view {
	case viewIssues:
		if len(m.issues) == 0 || m.selectedIssueIdx >= len(m.issues) {
			return nil
		}
		num := m.issues[m.selectedIssueIdx].Number
		cmd := exec.Command("gh", "issue", "view", fmt.Sprintf("%d", num), "--web")
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return browserOpenErrMsg{err: err} })
	case viewPRs:
		if len(m.prs) == 0 || m.selectedPRIdx >= len(m.prs) {
			return nil
		}
		num := m.prs[m.selectedPRIdx].Number
		cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", num), "--web")
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return browserOpenErrMsg{err: err} })
	default:
		return nil
	}
}

// syncGitHubCmd returns a Cmd that fetches open PRs and issues from GitHub in the background.
func (m *Model) syncGitHubCmd() tea.Cmd {
	repoPath := m.RepoPath
	db := m.db
	ttl := m.Config.GitHub.SyncInterval()
	return func() tea.Msg {
		// If db is available, check cache staleness before hitting the CLI.
		if db != nil {
			prStale, _ := data.IsCacheStale(db, data.CacheTablePRs, ttl)
			issStale, _ := data.IsCacheStale(db, data.CacheTableIssues, ttl)
			if !prStale && !issStale {
				// Cache is fresh — return cached rows without calling gh.
				repo := data.NewGitHubRepository(db)
				prs, err := repo.GetPRs()
				if err == nil {
					issues, err2 := repo.GetIssues()
					if err2 == nil {
						return githubSyncedMsg{prs: prs, issues: issues, syncedAt: time.Now()}
					}
				}
				// If reading cache fails, fall through to CLI sync.
			}
		}
		issueCmd := internalexec.NewIssueCommand(repoPath)
		prCmd := internalexec.NewPRCommand(repoPath)
		issues, issErr := issueCmd.ListOpenIssues()

		// Best-effort hierarchy enrichment — failures are silently ignored.
		if issErr == nil && len(issues) > 0 {
			owner, repo, err := issueCmd.GetRepoOwnerAndName()
			if err != nil {
				slog.Debug("hierarchy: get repo owner/name", "err", err)
			} else {
				nums := make([]int, len(issues))
				for i, iss := range issues {
					nums[i] = iss.Number
				}
				hier, err := issueCmd.FetchIssueHierarchy(nums, owner, repo)
				if err != nil {
					slog.Debug("hierarchy: fetch failed", "err", err)
				} else if hier != nil {
					// Build child→parent reverse map.
					childToParent := make(map[int]int)
					for parentNum, children := range hier {
						for _, child := range children {
							childToParent[child] = parentNum
						}
					}
					for i := range issues {
						n := issues[i].Number
						if p, ok := childToParent[n]; ok {
							pCopy := p
							issues[i].ParentNumber = &pCopy
						}
						if children, ok := hier[n]; ok && len(children) > 0 {
							issues[i].SubIssueNumbers = children
						}
					}
				}
			}
			// Fallback: parse issue bodies for parent-reference patterns
			// (catches repos that track hierarchy via body text rather than
			// GitHub's native sub-issues API).
			internalexec.EnrichHierarchyFromBodies(issues)
		}

		prs, prErr := prCmd.ListOpenPRs()

		// Persist enriched issues and PRs to DB so the next fresh-cache read
		// returns hierarchy-enriched data rather than a flat list.
		if db != nil {
			ghRepo := data.NewGitHubRepository(db)
			if issErr == nil {
				_ = ghRepo.UpsertIssues(issues)
			}
			if prErr == nil {
				_ = ghRepo.UpsertPRs(prs)
			}
		}

		return githubSyncedMsg{prs: prs, issues: issues, err: errors.Join(issErr, prErr), syncedAt: time.Now()}
	}
}

// addWorktreeCmd returns a Cmd that creates a new git worktree with a new branch.
// baseBranch is the branch to base off; empty string defaults to "main".
func (m *Model) addWorktreeCmd(branch, path, baseBranch string) tea.Cmd {
	if baseBranch == "" {
		baseBranch = "main"
	}
	repoPath := m.RepoPath
	return func() tea.Msg {
		cmd := internalexec.NewGitCommand(repoPath)
		err := cmd.AddWorktreeNewBranch(path, branch, baseBranch)
		return worktreeOpDoneMsg{err: err}
	}
}

// checkoutPRWorktreeCmd returns a Cmd that fetches a remote PR branch and creates a worktree for it.
func (m *Model) checkoutPRWorktreeCmd(branch, path string) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		cmd := internalexec.NewGitCommand(repoPath)
		err := cmd.CheckoutPRWorktree(path, branch)
		return worktreeOpDoneMsg{err: err}
	}
}

// prWorktreePath derives the filesystem path for a PR worktree using the same
// convention as issue worktrees: ../worktrees/<branch-with-slashes-as-dashes>.
func prWorktreePath(repoPath, branch string) string {
	slug := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(filepath.Dir(repoPath), "worktrees", slug)
}

// computeParentBranches returns the branches of any worktrees associated with
// parent issues (i.e., issues that have sub-issues). Used to populate the base
// branch picker in the create-worktree modal.
func computeParentBranches(issues []domain.Issue, worktrees []domain.Worktree) []string {
	var branches []string
	for _, iss := range issues {
		if len(iss.SubIssueNumbers) == 0 {
			continue
		}
		needle := fmt.Sprintf("issue-%d-", iss.Number)
		for _, wt := range worktrees {
			if strings.Contains(wt.Branch, needle) {
				branches = append(branches, wt.Branch)
				break
			}
		}
	}
	return branches
}

// removeWorktreeCmd returns a Cmd that removes a git worktree.
func (m *Model) removeWorktreeCmd(path string) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		cmd := internalexec.NewGitCommand(repoPath)
		err := cmd.RemoveWorktree(path, true)
		return worktreeOpDoneMsg{err: err}
	}
}

// switchWorktreeCmd returns a Cmd that launches a shell in the specified worktree directory,
// allowing the user to work within the worktree before returning to the TUI.
func (m *Model) switchWorktreeCmd(path string) tea.Cmd {
	cmd := buildShellCmd(path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return worktreeSwitchedMsg{err: err}
	})
}

// buildCopilotCmd constructs the exec.Cmd for running gh copilot in interactive
// mode with the given prompt pre-loaded in the specified worktree directory.
// When prompt is empty, runs "gh copilot" (interactive mode, no pre-loaded prompt).
// It is extracted as a top-level function to keep it unit-testable.
func buildCopilotCmd(worktreePath, prompt string) *exec.Cmd {
	var args []string
	if prompt != "" {
		args = []string{"copilot", "-i", prompt}
	} else {
		args = []string{"copilot"}
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = worktreePath
	return cmd
}

// buildSpawnSession constructs a domain.Session for a newly-launched agent
// terminal and persists it to db when db is non-nil. prompt may be empty
// (Aider does not take a pre-loaded prompt).
func buildSpawnSession(db *data.DB, worktreePath, agentName string, pid int, prompt string, startedAt time.Time) domain.Session {
	agentNameVal := agentName
	var shellPID *int
	if pid != 0 {
		shellPID = &pid
	}
	sess := domain.Session{
		WorktreePath: worktreePath,
		ShellPID:     shellPID,
		AgentName:    &agentNameVal,
		Status:       domain.StatusActive,
		StartedAt:    startedAt.UTC().Truncate(time.Second),
	}
	if prompt != "" {
		promptVal := prompt
		sess.Prompt = &promptVal
	}
	if db != nil {
		id, err := data.UpsertSession(db, sess)
		if err == nil {
			sess.ID = id
		}
	}
	return sess
}

// spawnCopilotCmd opens a new terminal tab/window running gh copilot at
// worktreePath and dispatches agentDoneMsg once the launch completes.
// The TUI is not suspended — nexus keeps running in the current terminal.
func (m *Model) spawnCopilotCmd(worktreePath, prompt string) tea.Cmd {
	startedAt := time.Now()
	db := m.db // capture m.db so the closure does not close over m (data race)
	var shellCmd string
	if prompt != "" {
		shellCmd = "gh copilot -i " + shellQuote(prompt)
	} else {
		shellCmd = "gh copilot"
	}
	return func() tea.Msg {
		pid, spawnErr := spawnAgentInTerminalWindow(worktreePath, shellCmd)
		exitCode := 0
		if spawnErr != nil {
			exitCode = 1
		}
		var sess domain.Session
		if spawnErr == nil {
			sess = buildSpawnSession(db, worktreePath, "copilot", pid, prompt, startedAt)
		}
		return agentDoneMsg{
			agentName: "copilot",
			prompt:    prompt,
			exitCode:  exitCode,
			startedAt: startedAt,
			session:   sess,
		}
	}
}

// resolveClaudeBinary returns the resolved path for the Claude binary.
// It reads cfg.AIAgents.ClaudeBinary, defaulting to "claude", then
// uses exec.LookPath to verify the binary is on the PATH.
func resolveClaudeBinary(cfg *domain.Config) (string, error) {
	bin := cfg.AIAgents.ClaudeBinary
	if bin == "" {
		bin = "claude"
	}
	return exec.LookPath(bin)
}

// resolveAiderBinary returns the resolved path for the Aider binary.
// It reads cfg.AIAgents.AiderBinary, defaulting to "aider", then
// uses exec.LookPath to verify the binary is on the PATH.
func resolveAiderBinary(cfg *domain.Config) (string, error) {
	bin := cfg.AIAgents.AiderBinary
	if bin == "" {
		bin = "aider"
	}
	return exec.LookPath(bin)
}

// buildClaudeCmd constructs the exec.Cmd for running the Claude CLI with the
// given prompt in the specified worktree directory.
// It is extracted as a top-level function to keep it unit-testable.
func buildClaudeCmd(worktreePath, prompt, binaryPath string) *exec.Cmd {
	var cmd *exec.Cmd
	if prompt != "" {
		cmd = exec.Command(binaryPath, prompt)
	} else {
		cmd = exec.Command(binaryPath)
	}
	cmd.Dir = worktreePath
	return cmd
}

// spawnClaudeCmd opens a new terminal tab/window running the Claude binary at
// worktreePath and dispatches agentDoneMsg once the launch completes.
// The TUI is not suspended — nexus keeps running in the current terminal.
func (m *Model) spawnClaudeCmd(worktreePath, prompt string) tea.Cmd {
	binaryPath, err := resolveClaudeBinary(m.Config)
	if err != nil {
		m.statusErr = fmt.Sprintf("claude binary not found: %v", err)
		return clearErrorCmd()
	}
	startedAt := time.Now()
	db := m.db // capture m.db so the closure does not close over m (data race)
	var shellCmd string
	if prompt != "" {
		shellCmd = binaryPath + " " + shellQuote(prompt)
	} else {
		shellCmd = binaryPath
	}
	return func() tea.Msg {
		pid, spawnErr := spawnAgentInTerminalWindow(worktreePath, shellCmd)
		exitCode := 0
		if spawnErr != nil {
			exitCode = 1
		}
		var sess domain.Session
		if spawnErr == nil {
			sess = buildSpawnSession(db, worktreePath, "claude", pid, prompt, startedAt)
		}
		return agentDoneMsg{
			agentName: "claude",
			prompt:    prompt,
			exitCode:  exitCode,
			startedAt: startedAt,
			session:   sess,
		}
	}
}

// fetchAiderFilesCmd returns a Cmd that lists modified files in the worktree
// using git ls-files, dispatching aiderFilesFetchedMsg with the result.
func (m *Model) fetchAiderFilesCmd(worktreePath string) tea.Cmd {
	return func() tea.Msg {
		cmd := internalexec.NewGitCommand(worktreePath)
		files, err := cmd.ListModifiedFiles(worktreePath)
		return aiderFilesFetchedMsg{worktreePath: worktreePath, files: files, err: err}
	}
}

// buildAiderCmd constructs the exec.Cmd for running aider with the given files
// in the specified worktree directory. Extracted as a top-level function for testability.
func buildAiderCmd(worktreePath string, files []string, binaryPath string) *exec.Cmd {
	cmd := exec.Command(binaryPath, files...)
	cmd.Dir = worktreePath
	return cmd
}

// spawnAiderCmd opens a new terminal tab/window running aider with the selected
// files at worktreePath and dispatches agentDoneMsg once the launch completes.
// The TUI is not suspended — nexus keeps running in the current terminal.
func (m *Model) spawnAiderCmd(worktreePath string, files []string) tea.Cmd {
	binaryPath, err := resolveAiderBinary(m.Config)
	if err != nil {
		m.statusErr = fmt.Sprintf("aider not found: %v", err)
		return clearErrorCmd()
	}
	startedAt := time.Now()
	db := m.db // capture m.db so the closure does not close over m (data race)
	parts := make([]string, 0, len(files)+1)
	parts = append(parts, binaryPath)
	for _, f := range files {
		parts = append(parts, shellQuote(f))
	}
	shellCmd := strings.Join(parts, " ")
	return func() tea.Msg {
		pid, spawnErr := spawnAgentInTerminalWindow(worktreePath, shellCmd)
		exitCode := 0
		if spawnErr != nil {
			exitCode = 1
		}
		var sess domain.Session
		if spawnErr == nil {
			sess = buildSpawnSession(db, worktreePath, "aider", pid, "", startedAt)
		}
		return agentDoneMsg{
			agentName: "aider",
			exitCode:  exitCode,
			startedAt: startedAt,
			session:   sess,
		}
	}
}

// buildShellCmd constructs a platform-appropriate shell command for the given directory.
// On Windows without a SHELL env var, it uses cmd.exe with /K flag to keep the shell open.
// When SHELL is set (e.g. Git Bash), it respects that on all platforms.
func buildShellCmd(path string) *exec.Cmd {
	return buildShellCmdForOS(path, runtime.GOOS, os.Getenv("SHELL"))
}

// buildShellCmdForOS constructs a shell command for a specific OS and shell value.
// It exists to keep buildShellCmd testable across platforms.
// On Windows with no shell configured, it falls back to cmd.exe.
// When shell is set (e.g. via SHELL env var in Git Bash), it is used on any OS.
func buildShellCmdForOS(path, goos, shell string) *exec.Cmd {
	// On Windows, prefer the SHELL env var when set (e.g. Git Bash / MSYS2).
	// Only fall back to cmd.exe when no Unix-compatible shell is configured.
	if goos == "windows" && shell == "" {
		cmd := exec.Command("cmd", "/K")
		cmd.Dir = path
		return cmd
	}

	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell)
	cmd.Dir = path
	return cmd
}

// getShell returns the user's preferred shell, or /bin/sh as a fallback.
// It reads the SHELL environment variable on Unix-like systems.
func getShell() string {
	shell := os.Getenv("SHELL")
	if shell != "" {
		return shell
	}
	return "/bin/sh"
}

// buildNewTerminalCmd constructs a platform-specific command that opens a new
// terminal window rooted at path without blocking the caller. When pidFile is
// non-empty the spawned shell writes its PID to the file before becoming
// interactive, enabling the caller to track session lifetime.
//
//   - Windows: cmd /C start cmd /K "cd /d <path>"  (pidFile not supported)
//   - macOS:   osascript do script with PID preamble, or open -a Terminal fallback
//   - Linux:   $TERMINAL / x-terminal-emulator / xterm with PID preamble
func buildNewTerminalCmd(path, pidFile, goos string) *exec.Cmd {
	switch goos {
	case "windows":
		cmd := exec.Command("cmd", "/C", "start", "cmd", "/K", fmt.Sprintf(`cd /d "%s"`, path))
		setWindowsCmdLine(cmd, path)
		return cmd
	case "darwin":
		if pidFile != "" {
			return exec.Command("osascript", "-e",
				fmt.Sprintf(`tell app "Terminal" to do script "echo $$ > \"%s\"; cd %s"`, pidFile, shellSingleQuote(path)))
		}
		return exec.Command("open", "-a", "Terminal", path)
	default:
		if term := os.Getenv("TERMINAL"); term != "" {
			if pidFile != "" {
				return exec.Command(term, "-e", "sh", "-c",
					fmt.Sprintf(`echo $$ > "%s"; cd %s; exec "${SHELL:-sh}"`, pidFile, shellSingleQuote(path)))
			}
			return exec.Command(term, "--working-directory="+path)
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		for _, candidate := range []string{"x-terminal-emulator", "xterm"} {
			if _, err := exec.LookPath(candidate); err == nil {
				if pidFile != "" {
					return exec.Command(candidate, "-e", "sh", "-c",
						fmt.Sprintf(`echo $$ > "%s"; cd %s; exec "${SHELL:-sh}"`, pidFile, shellSingleQuote(path)))
				}
				return exec.Command(candidate, "-e", fmt.Sprintf("cd %s; %s", shellSingleQuote(path), shell))
			}
		}
		if pidFile != "" {
			return exec.Command("xterm", "-e", "sh", "-c",
				fmt.Sprintf(`echo $$ > "%s"; cd %s; exec "${SHELL:-sh}"`, pidFile, shellSingleQuote(path)))
		}
		return exec.Command("xterm", "-e", fmt.Sprintf("cd %s; %s", shellSingleQuote(path), shell))
	}
}

// buildNewTerminalWithCmdCmd constructs a platform-specific command that opens
// a new terminal window at path and runs agentCmd inside it. Used by the Unix
// platform file to implement spawnAgentInTerminalWindow.
//
// Terminal detection order (env-var based):
//
//   - macOS: Ghostty → Terminal.app (osascript fallback)
//   - Linux: Ghostty ($TERM=xterm-ghostty) → Alacritty ($TERM=alacritty) →
//     Kitty ($KITTY_WINDOW_ID, no remote-control) → $TERMINAL → xterm
func buildNewTerminalWithCmdCmd(path, agentCmd, goos string) *exec.Cmd {
	script := fmt.Sprintf("cd %s && %s", shellSingleQuote(path), agentCmd)
	switch goos {
	case "darwin":
		switch os.Getenv("TERM_PROGRAM") {
		case "ghostty":
			// Ghostty: pass command as positional args after --.
			return exec.Command("ghostty", "--working-directory="+path, "--", "sh", "-c", agentCmd)
		}
		// macOS fallback: open a new Terminal.app window via osascript.
		return exec.Command("osascript", "-e",
			fmt.Sprintf(`tell app "Terminal" to do script "cd %s && %s"`, shellSingleQuote(path), escapeAppleScriptStr(agentCmd)))
	default: // Linux
		switch os.Getenv("TERM") {
		case "xterm-ghostty":
			return exec.Command("ghostty", "--working-directory="+path, "--", "sh", "-c", agentCmd)
		case "alacritty":
			// Alacritty without IPC socket: spawn a new window.
			return exec.Command("alacritty", "--working-directory", path, "-e", "sh", "-c", agentCmd)
		}
		// Kitty without remote control: open a new kitty window.
		if os.Getenv("KITTY_WINDOW_ID") != "" {
			return exec.Command("kitty", "--directory", path, "sh", "-c", agentCmd)
		}
		if term := os.Getenv("TERMINAL"); term != "" {
			return exec.Command(term, "-e", script)
		}
		for _, candidate := range []string{"x-terminal-emulator", "xterm"} {
			if _, err := exec.LookPath(candidate); err == nil {
				return exec.Command(candidate, "-e", script)
			}
		}
		return exec.Command("xterm", "-e", script)
	}
}

// buildNewTabWithCmdCmd tries to open agentCmd in a new tab/pane of the current
// terminal emulator. When agentCmd is empty a plain interactive shell is opened
// instead. When pidFile is non-empty and agentCmd is empty the spawned shell is
// wrapped to write its PID to pidFile before exec-replacing itself so the
// caller can track when the tab is closed.
//
// Returns (cmd, true) if a tab-capable emulator is detected, or (nil, false)
// to signal the caller should fall back to a new window.
//
// Detection is env-var based. Priority:
//
//  1. Multiplexers (tmux, zellij) — checked first on all platforms so that
//     users who layer a GUI terminal on top of a multiplexer still get the
//     expected behaviour.
//
//  2. Kitty remote-control ($KITTY_WINDOW_ID).  Requires allow_remote_control
//     in kitty.conf; falls back to a new kitty window on failure.
//
//  3. Alacritty IPC ($ALACRITTY_SOCKET, v0.13+).
//
//  4. Platform-specific: Windows Terminal, iTerm2, Terminal.app, Konsole.
//
// Ghostty does not yet expose a stable tab-open CLI; it is handled as a new
// window in buildNewTerminalWithCmdCmd.
func buildNewTabWithCmdCmd(path, agentCmd, pidFile, goos string) (*exec.Cmd, bool) {
	// 1. Multiplexers — take precedence over GUI terminal tabs.
	if os.Getenv("TMUX") != "" {
		args := []string{"new-window", "-c", path}
		if agentCmd != "" {
			args = append(args, agentCmd)
		} else if pidFile != "" {
			args = append(args, pidInitUnixCmd(pidFile))
		}
		return exec.Command("tmux", args...), true
	}
	if os.Getenv("ZELLIJ") != "" || os.Getenv("ZELLIJ_SESSION_NAME") != "" {
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "sh"
		}
		if agentCmd != "" {
			return exec.Command("zellij", "run", "--cwd", path, "--", shell, "-c", agentCmd), true
		}
		if pidFile != "" {
			return exec.Command("zellij", "run", "--cwd", path, "--",
				"sh", "-c", fmt.Sprintf(`echo $$ > "%s"; exec "%s"`, pidFile, shell)), true
		}
		return exec.Command("zellij", "run", "--cwd", path, "--", shell), true
	}

	// 2. Kitty remote-control (cross-platform, Linux + macOS).
	if goos != "windows" && os.Getenv("KITTY_WINDOW_ID") != "" {
		args := []string{"@", "new-window", "--new-tab", "--cwd", path}
		if agentCmd != "" {
			args = append(args, "sh", "-c", agentCmd)
		} else if pidFile != "" {
			args = append(args, "sh", "-c", fmt.Sprintf(`echo $$ > "%s"; exec "${SHELL:-sh}"`, pidFile))
		}
		return exec.Command("kitty", args...), true
	}

	// 3. Alacritty IPC — available when $ALACRITTY_SOCKET is set (v0.13+).
	if os.Getenv("ALACRITTY_SOCKET") != "" {
		args := []string{"msg", "create-tab", "--working-directory", path}
		if agentCmd != "" {
			args = append(args, "--", "sh", "-c", agentCmd)
		} else if pidFile != "" {
			args = append(args, "--", "sh", "-c", fmt.Sprintf(`echo $$ > "%s"; exec "${SHELL:-sh}"`, pidFile))
		}
		return exec.Command("alacritty", args...), true
	}

	// 4. Platform-specific tab APIs.
	switch goos {
	case "windows":
		// Windows Terminal: the spawnTerminalWindow function in terminal_windows.go
		// intercepts WT_SESSION before calling here and uses a PowerShell PID-file
		// instead. This branch handles agent commands only.
		if os.Getenv("WT_SESSION") != "" {
			args := []string{"-w", "0", "new-tab", "--startingDirectory", path}
			if agentCmd != "" {
				args = append(args, "cmd", "/K", agentCmd)
			}
			return exec.Command("wt", args...), true
		}
	case "darwin":
		switch os.Getenv("TERM_PROGRAM") {
		case "iTerm.app":
			var script string
			if agentCmd == "" && pidFile != "" {
				script = fmt.Sprintf(
					`tell application "iTerm2" to tell current window to create tab with default profile command %s`,
					shellQuote(fmt.Sprintf(`bash -c 'echo $$ > "%s"; cd %q; exec "${SHELL:-bash}"'`, pidFile, path)),
				)
			} else if agentCmd == "" {
				script = fmt.Sprintf(
					`tell application "iTerm2" to tell current window to create tab with default profile command %s`,
					shellQuote(fmt.Sprintf("bash -c 'cd %q; exec ${SHELL:-bash}'", path)),
				)
			} else {
				script = fmt.Sprintf(
					`tell application "iTerm2" to tell current window to create tab with default profile command %s`,
					shellQuote(fmt.Sprintf("bash -c %s", shellQuote(fmt.Sprintf("cd %q && %s", path, agentCmd)))),
				)
			}
			return exec.Command("osascript", "-e", script), true
		case "Apple_Terminal":
			var script string
			if agentCmd == "" && pidFile != "" {
				script = fmt.Sprintf(`tell app "Terminal" to do script "echo $$ > \"%s\"; cd %q" in front window`, pidFile, path)
			} else if agentCmd == "" {
				script = fmt.Sprintf(`tell app "Terminal" to do script "cd %q" in front window`, path)
			} else {
				script = fmt.Sprintf(`tell app "Terminal" to do script "cd %q && %s" in front window`, path, agentCmd)
			}
			return exec.Command("osascript", "-e", script), true
			// ghostty: no stable tab-open CLI yet — falls through to new window.
		}
	default: // Linux
		if os.Getenv("KONSOLE_VERSION") != "" {
			if agentCmd == "" && pidFile != "" {
				return exec.Command("konsole", "--new-tab", "-e", "sh", "-c",
					fmt.Sprintf(`echo $$ > "%s"; cd %q; exec "${SHELL:-bash}"`, pidFile, path)), true
			}
			if agentCmd == "" {
				return exec.Command("konsole", "--new-tab", "--workdir", path), true
			}
			shell := os.Getenv("SHELL")
			if shell == "" {
				shell = "bash"
			}
			return exec.Command("konsole", "--new-tab", "-e", shell, "-c",
				fmt.Sprintf("cd %q && %s", path, agentCmd)), true
		}
	}
	return nil, false
}

// shellQuote wraps s in double quotes with escaping for safe embedding in a
// POSIX shell command string. Escapes \, ", $, and backtick to prevent
// command substitution or variable expansion inside the quoted argument.
func shellQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `$`, `\$`)
	s = strings.ReplaceAll(s, "`", "\\`")
	return `"` + s + `"`
}

// shellSingleQuote returns s wrapped in POSIX single quotes, safe for use in
// any sh/bash command string and inside AppleScript do-script double-quoted
// strings (single quotes are transparent to AppleScript's string parser).
// Embedded single-quotes are escaped with the '\” idiom.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// escapeAppleScriptStr escapes s for embedding inside an AppleScript
// double-quoted string. Backslashes and double-quotes are escaped so
// AppleScript does not interpret them as string delimiters.
func escapeAppleScriptStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// spawnSessionCmd opens a new terminal window at worktreePath in the background
// and dispatches a sessionSpawnedMsg with the PID once the process is launched.
// The PID is persisted in active_sessions when a DB connection is available.
func (m *Model) spawnSessionCmd(worktreePath string) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		pid, err := spawnTerminalWindow(worktreePath)
		if err != nil {
			return sessionSpawnedMsg{err: fmt.Errorf("spawn session: %w", err)}
		}
		// pid == 0 means the launcher exited immediately (e.g. Windows Terminal
		// new-tab) and there is no trackable long-lived PID. Store nil so the
		// health-check loop keeps the session alive rather than pruning it.
		var shellPID *int
		if pid != 0 {
			shellPID = &pid
		}
		session := domain.Session{
			WorktreePath: worktreePath,
			ShellPID:     shellPID,
			Status:       domain.StatusActive,
			StartedAt:    time.Now().UTC().Truncate(time.Second),
		}
		if db != nil {
			id, err := data.UpsertSession(db, session)
			if err != nil {
				// Non-fatal: terminal is running but we could not persist the PID.
				return sessionSpawnedMsg{session: session, err: fmt.Errorf("track session: %w", err)}
			}
			session.ID = id
		}
		return sessionSpawnedMsg{session: session}
	}
}

// killSessionCmd gracefully kills the shell and agent processes for the given
// session and removes the session record from the DB.
// On Unix, SIGTERM is sent first; SIGKILL follows after a 3-second timeout.
// On Windows, the process is terminated immediately (no SIGTERM equivalent).
func (m *Model) killSessionCmd(session domain.Session) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		if session.ShellPID != nil {
			gracefulKillPID(*session.ShellPID)
		}
		if db != nil && session.ID != 0 {
			if err := data.DeleteSession(db, session.ID); err != nil {
				return sessionKilledMsg{worktreePath: session.WorktreePath, err: err}
			}
		}
		return sessionKilledMsg{worktreePath: session.WorktreePath}
	}
}

// focusSessionCmd attempts to bring the terminal window for the given session
// to the foreground and dispatches sessionFocusedMsg with the outcome.
func (m *Model) focusSessionCmd(session domain.Session) tea.Cmd {
	return func() tea.Msg {
		pid := 0
		if session.ShellPID != nil {
			pid = *session.ShellPID
		}
		err := focusSessionWindow(pid)
		return sessionFocusedMsg{worktreePath: session.WorktreePath, err: err}
	}
}

// filterAliveSessions returns only sessions that should be considered live:
//   - StatusDead sessions are always excluded.
//   - Sessions with no ShellPID are kept for up to 24 hours (e.g. Windows
//     Terminal new-tab, where no stable long-lived PID is available).
//   - Sessions with a ShellPID are kept only when that PID is still alive.
func filterAliveSessions(sessions []domain.Session) []domain.Session {
	var alive []domain.Session
	for _, s := range sessions {
		if s.Status == domain.StatusDead {
			continue
		}
		if s.ShellPID == nil {
			// No PID — keep alive up to 24 hours.
			if time.Since(s.StartedAt) <= 24*time.Hour {
				alive = append(alive, s)
			}
			continue
		}
		if pidAlive(*s.ShellPID) {
			alive = append(alive, s)
		}
	}
	return alive
}

// checkSessionsCmd reads all tracked sessions from the nexus DB and checks
// whether each PID is still alive. Returns sessionStatusUpdatedMsg with the
// live session list. Dead and stale sessions are removed from the DB when a DB
// connection is available.
func (m *Model) checkSessionsCmd() tea.Cmd {
	db := m.db
	current := m.sessions
	return func() tea.Msg {
		var alive []domain.Session

		if db == nil {
			// No nexus DB — perform PID health checks on in-memory sessions only.
			alive = filterAliveSessions(current)
		} else {
			all, err := data.GetSessions(db)
			if err != nil {
				slog.Warn("session health check: failed to read sessions from DB", "err", err)
				// Fall back to in-memory state so the UI doesn't go blank.
				alive = filterAliveSessions(current)
			} else {
				alive = filterAliveSessions(all)
				// Remove sessions that didn't survive the filter from the DB.
				aliveIDs := make(map[int64]bool, len(alive))
				for _, s := range alive {
					aliveIDs[s.ID] = true
				}
				for _, s := range all {
					if aliveIDs[s.ID] {
						continue
					}
					logKey := "session health check: failed to delete dead session"
					if s.ShellPID == nil {
						logKey = "session health check: failed to delete stale session"
					}
					if err := data.DeleteSession(db, s.ID); err != nil {
						slog.Warn(logKey, "id", s.ID, "err", err)
					}
				}
			}
		}

		if alive == nil {
			alive = []domain.Session{}
		}
		return sessionStatusUpdatedMsg{sessions: alive}
	}
}

type worktreesRefreshedMsg struct {
	worktrees []domain.Worktree
	err       error
}

// refreshWorktreesCmd returns a Cmd that reloads the worktree list from git.
func (m *Model) refreshWorktreesCmd() tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		cmd := internalexec.NewGitCommand(repoPath)
		worktrees, err := cmd.ListWorktrees()
		return worktreesRefreshedMsg{worktrees: worktrees, err: err}
	}
}

func (m *Model) selectedWorktree() (domain.Worktree, bool) {
	if len(m.Worktrees) == 0 || m.selectedIdx < 0 || m.selectedIdx >= len(m.Worktrees) {
		return domain.Worktree{}, false
	}

	return m.Worktrees[m.selectedIdx], true
}

func (m *Model) selectedIssue() (domain.Issue, bool) {
	if len(m.issues) == 0 || m.selectedIssueIdx < 0 || m.selectedIssueIdx >= len(m.issues) {
		return domain.Issue{}, false
	}

	return m.issues[m.selectedIssueIdx], true
}

func (m *Model) clampSelectedIdx() {
	if len(m.Worktrees) == 0 {
		m.selectedIdx = 0
		return
	}

	if m.selectedIdx < 0 {
		m.selectedIdx = 0
		return
	}

	if m.selectedIdx >= len(m.Worktrees) {
		m.selectedIdx = len(m.Worktrees) - 1
	}
}

func (m *Model) clampIssueIdx() {
	if len(m.issues) == 0 {
		m.selectedIssueIdx = 0
		return
	}
	if m.selectedIssueIdx >= len(m.issues) {
		m.selectedIssueIdx = len(m.issues) - 1
	}
}

func (m *Model) clampPRIdx() {
	if len(m.prs) == 0 {
		m.selectedPRIdx = 0
		return
	}
	if m.selectedPRIdx >= len(m.prs) {
		m.selectedPRIdx = len(m.prs) - 1
	}
}

// nextPage advances to the next page for the current list view (issues or PRs).
func (m *Model) nextPage() {
	switch m.view {
	case viewIssues:
		maxPage := (len(m.issues) - 1) / pageSize
		if m.currentPage < maxPage {
			m.currentPage++
			m.selectedIssueIdx = m.currentPage * pageSize
		}
	case viewPRs:
		maxPage := (len(m.prs) - 1) / pageSize
		if m.currentPage < maxPage {
			m.currentPage++
			m.selectedPRIdx = m.currentPage * pageSize
		}
	}
}

// prevPage retreats to the previous page for the current list view.
func (m *Model) prevPage() {
	if m.currentPage > 0 {
		m.currentPage--
		switch m.view {
		case viewIssues:
			m.selectedIssueIdx = m.currentPage * pageSize
		case viewPRs:
			m.selectedPRIdx = m.currentPage * pageSize
		}
	}
}

// moveDown advances the selection within the currently focused panel.
// Nav panel: cycles the active view forward.
// Ctx panel: scrolls the context content down.
// List panel (default): moves the item cursor down.
func (m *Model) moveDown() {
	switch m.focused {
	case panelNav:
		n := int(m.view) + 1
		if n > int(viewPRs) {
			n = int(viewWorktrees)
		}
		m.view = activeView(n)
	case panelCtx:
		m.ctxScrollOffset++
	default: // panelList
		switch m.view {
		case viewIssues:
			tree := m.issueTree
			if tree == nil {
				tree = buildIssueTree(m.issues)
			}
			for ti, r := range tree {
				if r.originalIdx == m.selectedIssueIdx {
					if ti < len(tree)-1 {
						m.selectedIssueIdx = tree[ti+1].originalIdx
						m.ctxScrollOffset = 0
					}
					break
				}
			}
		case viewPRs:
			if m.selectedPRIdx < len(m.prs)-1 {
				m.selectedPRIdx++
				m.ctxScrollOffset = 0
			}
		default:
			if m.selectedIdx < len(m.Worktrees)-1 {
				m.selectedIdx++
				m.ctxScrollOffset = 0
			}
		}
	}
}

// moveUp retreats the selection within the currently focused panel.
// Nav panel: cycles the active view backward.
// Ctx panel: scrolls the context content up.
// List panel (default): moves the item cursor up.
func (m *Model) moveUp() {
	switch m.focused {
	case panelNav:
		n := int(m.view) - 1
		if n < int(viewWorktrees) {
			n = int(viewPRs)
		}
		m.view = activeView(n)
	case panelCtx:
		if m.ctxScrollOffset > 0 {
			m.ctxScrollOffset--
		}
	default: // panelList
		switch m.view {
		case viewIssues:
			tree := m.issueTree
			if tree == nil {
				tree = buildIssueTree(m.issues)
			}
			for ti, r := range tree {
				if r.originalIdx == m.selectedIssueIdx {
					if ti > 0 {
						m.selectedIssueIdx = tree[ti-1].originalIdx
						m.ctxScrollOffset = 0
					}
					break
				}
			}
		case viewPRs:
			if m.selectedPRIdx > 0 {
				m.selectedPRIdx--
				m.ctxScrollOffset = 0
			}
		default:
			if m.selectedIdx > 0 {
				m.selectedIdx--
				m.ctxScrollOffset = 0
			}
		}
	}
}
