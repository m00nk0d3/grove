package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
)

// issuesFetchedMsg carries the result of a background gh issue list call.
type issuesFetchedMsg struct {
	issues []domain.Issue // List of fetched issues, or nil on error
	err    error          // Error during fetch, if any
}

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
}

// aiderFilesFetchedMsg carries the result of listing modified files for the Aider file picker.
type aiderFilesFetchedMsg struct {
	worktreePath string
	files        []string
	err          error
}

// clearErrorMsg is dispatched after the 5-second auto-dismiss timer fires.
type clearErrorMsg struct{}

// clearErrorCmd returns a Cmd that fires clearErrorMsg after 5 seconds.
func clearErrorCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return clearErrorMsg{}
	})
}

// sessionTickCmd schedules a sessionTickMsg after 3 seconds.
func sessionTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return sessionTickMsg{}
	})
}

// msgAutoDismissDuration is how long the success/info toast stays visible before
// being cleared by clearMsgCmd. Must stay in sync with the label in renderInfoModal.
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
	cmds := []tea.Cmd{m.refreshWorktreesCmd(), m.syncGitHubCmd()}
	if m.db != nil {
		cmds = append(cmds, sessionTickCmd())
	}
	return tea.Batch(cmds...)
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
					return m, m.switchWorktreeCmd(selected.Path)
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
		case tea.KeyCtrlN:
			return m, m.fetchIssuesCmd()
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
			}
		}

	case issuesFetchedMsg:
		if msg.err == nil {
			m.activeModal = modal.NewCreateModal(msg.issues, m.RepoPath, computeParentBranches(m.issues, m.Worktrees)...)
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
		m.sessions = msg.sessions
		return m, sessionTickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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

// fetchIssuesCmd returns a Cmd that fetches open GitHub issues in the background,
// allowing the user to create worktrees from issues.
func (m *Model) fetchIssuesCmd() tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		cmd := internalexec.NewIssueCommand(repoPath)
		issues, err := cmd.ListOpenIssues()
		return issuesFetchedMsg{issues: issues, err: err}
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

// spawnCopilotCmd returns a Cmd that runs gh copilot suggest in the worktree
// directory and dispatches agentDoneMsg when the process exits.
func (m *Model) spawnCopilotCmd(worktreePath, prompt string) tea.Cmd {
	startedAt := time.Now()
	cmd := buildCopilotCmd(worktreePath, prompt)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		exitCode := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return agentDoneMsg{
			agentName: "copilot",
			prompt:    prompt,
			exitCode:  exitCode,
			startedAt: startedAt,
		}
	})
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

// spawnClaudeCmd returns a Cmd that runs the Claude binary in the worktree
// directory and dispatches agentDoneMsg when the process exits.
func (m *Model) spawnClaudeCmd(worktreePath, prompt string) tea.Cmd {
	binaryPath, err := resolveClaudeBinary(m.Config)
	if err != nil {
		m.statusErr = fmt.Sprintf("claude binary not found: %v", err)
		return clearErrorCmd()
	}
	startedAt := time.Now()
	cmd := buildClaudeCmd(worktreePath, prompt, binaryPath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		exitCode := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return agentDoneMsg{
			agentName: "claude",
			prompt:    prompt,
			exitCode:  exitCode,
			startedAt: startedAt,
		}
	})
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

// spawnAiderCmd returns a Cmd that runs aider with the selected files in the
// worktree directory and dispatches agentDoneMsg when the process exits.
func (m *Model) spawnAiderCmd(worktreePath string, files []string) tea.Cmd {
	binaryPath, err := resolveAiderBinary(m.Config)
	if err != nil {
		m.statusErr = fmt.Sprintf("aider not found: %v", err)
		return clearErrorCmd()
	}
	startedAt := time.Now()
	cmd := buildAiderCmd(worktreePath, files, binaryPath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		exitCode := 0
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return agentDoneMsg{
			agentName: "aider",
			exitCode:  exitCode,
			startedAt: startedAt,
		}
	})
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
// terminal window rooted at path without blocking the caller.
//
//   - Windows: cmd /C start cmd /K "cd /d <path>"
//   - macOS:   open -a Terminal <path>
//   - Linux:   $TERMINAL --working-directory=<path>  (fallback: x-terminal-emulator, then xterm)
func buildNewTerminalCmd(path, goos string) *exec.Cmd {
	switch goos {
	case "windows":
		// Quote the path so worktree paths that contain spaces are handled correctly.
		// setWindowsCmdLine overrides SysProcAttr.CmdLine on Windows to bypass
		// Go's arg escaping, which would mangle the inner double-quotes and cause
		// cmd.exe to report "filename, directory name, or volume label syntax is
		// incorrect" (ERROR_INVALID_NAME / exit 1).
		cmd := exec.Command("cmd", "/C", "start", "cmd", "/K", fmt.Sprintf(`cd /d "%s"`, path))
		setWindowsCmdLine(cmd, path)
		return cmd
	case "darwin":
		return exec.Command("open", "-a", "Terminal", path)
	default:
		// Linux: respect the $TERMINAL env var when set.
		if term := os.Getenv("TERMINAL"); term != "" {
			return exec.Command(term, "--working-directory="+path)
		}
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		// Try common terminal emulators in order; x-terminal-emulator is the
		// Debian/Ubuntu update-alternatives symlink that works on most distros.
		for _, candidate := range []string{"x-terminal-emulator", "xterm"} {
			if _, err := exec.LookPath(candidate); err == nil {
				return exec.Command(candidate, "-e", fmt.Sprintf("cd %q; %s", path, shell))
			}
		}
		// Last resort: return an xterm cmd; Start() will surface the error if
		// xterm is not installed either.
		return exec.Command("xterm", "-e", fmt.Sprintf("cd %q; %s", path, shell))
	}
}

// spawnSessionCmd opens a new terminal window at worktreePath in the background
// and dispatches a sessionSpawnedMsg with the PID once the process is launched.
// The PID is persisted in active_sessions when a DB connection is available.
func (m *Model) spawnSessionCmd(worktreePath string) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		cmd := buildNewTerminalCmd(worktreePath, runtime.GOOS)
		if err := cmd.Start(); err != nil {
			return sessionSpawnedMsg{err: fmt.Errorf("spawn session: %w", err)}
		}
		pid := cmd.Process.Pid
		session := domain.Session{
			WorktreePath: worktreePath,
			// NOTE: on Windows (cmd /C start) and macOS (open -a Terminal) the
			// launcher process exits almost immediately, so this PID is best-effort
			// — it will be dead by the time Mission Control queries it.
			// On Linux the terminal emulator process stays alive, so PID is reliable.
			ShellPID:  &pid,
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

// checkSessionsCmd reads all tracked sessions from the DB, checks whether each
// PID is still alive, updates session status for agent-only survivors, and
// removes fully dead sessions. Returns sessionStatusUpdatedMsg with the updated list.
// When db is nil it is a no-op that returns the current sessions unchanged.
func (m *Model) checkSessionsCmd() tea.Cmd {
	db := m.db
	current := m.sessions
	return func() tea.Msg {
		if db == nil {
			return sessionStatusUpdatedMsg{sessions: current}
		}
		all, err := data.GetSessions(db)
		if err != nil {
			slog.Warn("session health check: failed to read sessions from DB", "err", err)
			return sessionStatusUpdatedMsg{sessions: current}
		}
		var alive []domain.Session
		for _, s := range all {
			if s.ShellPID == nil {
				// No PID to check — prune dead status rows, keep everything else.
				if s.Status == domain.StatusDead {
					if err := data.DeleteSession(db, s.ID); err != nil {
						slog.Warn("session health check: failed to delete dead session", "id", s.ID, "err", err)
					}
					continue
				}
				alive = append(alive, s)
				continue
			}
			shellAlive := pidAlive(*s.ShellPID)
			agentAlive := s.AgentPID != nil && pidAlive(*s.AgentPID)

			if shellAlive {
				alive = append(alive, s)
				continue
			}
			// Shell is dead.
			if agentAlive {
				// Agent is still running — transition to agent_running.
				s.Status = domain.StatusAgentRunning
				if _, err := data.UpsertSession(db, s); err != nil {
					slog.Warn("session health check: failed to update session status", "id", s.ID, "err", err)
				}
				alive = append(alive, s)
				continue
			}
			// Both are dead — remove from DB.
			if err := data.DeleteSession(db, s.ID); err != nil {
				slog.Warn("session health check: failed to delete dead session", "id", s.ID, "err", err)
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
