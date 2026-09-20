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
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/fuzzy"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/mission"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/m00nk0d3/grove/internal/updater"
	"github.com/m00nk0d3/grove/internal/version"
)

// worktreeOpDoneMsg carries the result of an add/remove worktree operation.
type worktreeOpDoneMsg struct {
	err error // Error during operation, if any
}

func (m *Model) focusPaneCmd(paneID, label string) tea.Cmd {
	navigator := m.herdrNavigator
	return func() tea.Msg {
		if navigator == nil {
			return paneFocusedMsg{label: label, err: fmt.Errorf("Herdr navigation unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return paneFocusedMsg{label: label, err: navigator.FocusPane(ctx, paneID)}
	}
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

// browserOpenErrMsg carries an error from launching an external process
// (browser, editor, gh CLI, etc.).
type browserOpenErrMsg struct{ err error }

// clearErrorMsg is dispatched after the 5-second auto-dismiss timer fires.
type clearErrorMsg struct{}

// updateCheckedMsg carries the result of the startup version check.
type updateCheckedMsg struct {
	info updater.ReleaseInfo
	err  error
}

// selfUpdateDoneMsg carries the result of a self-update attempt.
type selfUpdateDoneMsg struct{ err error }

// cleanupLoadedMsg carries cleanup candidates loaded from git.
type cleanupLoadedMsg struct {
	candidates []modal.CleanupCandidate
	err        error
}

// cleanupDoneMsg carries the result of a batch worktree/branch cleanup.
type cleanupDoneMsg struct {
	deleted int
	err     error
}

// fuzzyOpenMsg is dispatched when the user opens the fuzzy finder.
type fuzzyOpenMsg struct{}

// fuzzyResultsReadyMsg is dispatched when the async search index build is complete.
type fuzzyResultsReadyMsg struct {
	results []domain.SearchResult
}

// integrationsTickMsg triggers the next periodic Herdr/Sandcastle refresh.
type integrationsTickMsg struct{}

// sandcastleSyncedMsg carries the result of a background Sandcastle snapshot.
type sandcastleSyncedMsg struct {
	snapshot sandcastle.Snapshot
	err      error
}

type sandcastleWorkflowStartedMsg struct {
	workflow domain.WorkflowRunRef
	kind     string
	err      error
}

// herdrSyncedMsg carries the result of a background Herdr snapshot.
type herdrSyncedMsg struct {
	snapshot herdr.Snapshot
	err      error
}

// missionControlUpdatedMsg carries a freshly-built mission-control state.
type missionControlUpdatedMsg struct {
	state domain.MissionControlState
}

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

func (m *Model) jumpToSelectedMission() (tea.Model, tea.Cmd) {
	missions := dashboardMissionsForTab(m.missionState, m.dashboardTab)
	if len(missions) == 0 {
		m.statusErr = "No active workflow selected"
		return m, clearErrorCmd()
	}
	if m.selectedMissionIdx < 0 || m.selectedMissionIdx >= len(missions) {
		m.selectedMissionIdx = 0
	}
	mission := missions[m.selectedMissionIdx]
	return m.jumpToMission(mission)
}

func (m *Model) jumpToMissionRun(runID string) (tea.Model, tea.Cmd) {
	for _, tab := range []dashboardTab{dashboardTabActive, dashboardTabCompleted} {
		for i, mission := range dashboardMissionsForTab(m.missionState, tab) {
			if mission.workflow.RunID == runID || mission.workflow.WorkflowID == runID {
				m.dashboardTab = tab
				m.selectedMissionIdx = i
				return m.jumpToMission(mission)
			}
		}
	}
	m.statusErr = fmt.Sprintf("Workflow %s is no longer available", runID)
	return m, clearErrorCmd()
}

func (m *Model) jumpToMission(mission dashboardMission) (tea.Model, tea.Cmd) {
	runID := mission.workflow.RunID
	if runID == "" {
		runID = mission.workflow.WorkflowID
	}
	for _, agent := range agentsForWorkflow(m.missionState, runID) {
		if agent.PaneID != "" {
			label := agent.Name
			if label == "" {
				label = mission.label
			}
			return m, m.focusPaneCmd(agent.PaneID, label)
		}
	}
	if mission.paneID != "" {
		return m, m.focusPaneCmd(mission.paneID, mission.label)
	}
	for _, session := range m.sessions {
		if mission.worktreePath != "" && pathsEqual(session.WorktreePath, mission.worktreePath) {
			return m, m.focusSessionCmd(session)
		}
	}
	if mission.worktreePath != "" && m.insideHerdr && m.herdrNavigator != nil {
		return m, m.openHerdrWorktreeCmd(mission.worktreePath)
	}
	m.statusErr = fmt.Sprintf("No terminal or Herdr pane found for %s", mission.label)
	return m, clearErrorCmd()
}

func (m *Model) openSelectedMissionInspector() (tea.Model, tea.Cmd) {
	missions := dashboardMissionsForTab(m.missionState, m.dashboardTab)
	if len(missions) == 0 {
		m.statusErr = "No active workflow selected"
		return m, clearErrorCmd()
	}
	if m.selectedMissionIdx < 0 || m.selectedMissionIdx >= len(missions) {
		m.selectedMissionIdx = 0
	}
	workflow := missions[m.selectedMissionIdx].workflow
	if workflow.RunID == "" && workflow.WorkflowID == "" {
		m.statusErr = "No Sandcastle workflow telemetry is available for this mission"
		return m, clearErrorCmd()
	}
	runID := workflow.RunID
	if runID == "" {
		runID = workflow.WorkflowID
	}
	m.activeModal = modal.NewMissionModal(workflow, agentsForWorkflow(m.missionState, runID))
	return m, nil
}

func (m *Model) confirmSelectedWorkflowRemoval() (tea.Model, tea.Cmd) {
	missions := dashboardMissionsForTab(m.missionState, m.dashboardTab)
	if len(missions) == 0 {
		m.statusErr = "No workflow selected"
		return m, clearErrorCmd()
	}
	if m.selectedMissionIdx < 0 || m.selectedMissionIdx >= len(missions) {
		m.selectedMissionIdx = 0
	}
	workflow := missions[m.selectedMissionIdx].workflow
	if workflow.RunID == "" && workflow.WorkflowID == "" {
		m.statusErr = "No Sandcastle workflow record is available for this mission"
		return m, clearErrorCmd()
	}
	m.activeModal = modal.NewWorkflowRemoveModal(workflow)
	return m, nil
}

func (m *Model) retrySelectedWorkflow() (tea.Model, tea.Cmd) {
	missions := dashboardMissionsForTab(m.missionState, m.dashboardTab)
	if len(missions) == 0 {
		m.statusErr = "No workflow selected"
		return m, clearErrorCmd()
	}
	if m.selectedMissionIdx < 0 || m.selectedMissionIdx >= len(missions) {
		m.selectedMissionIdx = 0
	}
	workflow := missions[m.selectedMissionIdx].workflow
	return m.retryWorkflow(workflow)
}

func (m *Model) retryWorkflowByRunID(runID string) (tea.Model, tea.Cmd) {
	workflow, ok := workflowByRunID(m.missionState, runID)
	if !ok {
		m.statusErr = fmt.Sprintf("Workflow %s is no longer available", runID)
		return m, clearErrorCmd()
	}
	return m.retryWorkflow(workflow)
}

func (m *Model) retryWorkflow(workflow domain.WorkflowRunRef) (tea.Model, tea.Cmd) {
	if !strings.EqualFold(workflow.Status, domain.WorkflowFailed) {
		m.statusErr = "Only failed workflows can be retried"
		return m, clearErrorCmd()
	}
	if workflow.Kind == "" {
		m.statusErr = "Cannot retry workflow: workflow kind is unavailable"
		return m, clearErrorCmd()
	}
	if !m.Config.Sandcastle.Enabled {
		m.statusErr = "Sandcastle workflows are disabled in settings"
		return m, clearErrorCmd()
	}
	if m.workflowStarter == nil {
		m.statusErr = "Grove Sandcastle runtime is unavailable"
		return m, clearErrorCmd()
	}
	request := modal.WorkflowLaunchMsg{
		Kind:        workflow.Kind,
		RepoPath:    workflow.Repo,
		AgentKind:   workflow.DefaultAgent,
		IssueNumber: workflow.IssueNumber,
		PRNumber:    workflow.PRNumber,
	}
	m.statusMsg = fmt.Sprintf("Retrying %s workflow…", workflow.Kind)
	return m, m.startSandcastleWorkflowCmd(request)
}

func workflowByRunID(state *domain.MissionControlState, runID string) (domain.WorkflowRunRef, bool) {
	if state == nil {
		return domain.WorkflowRunRef{}, false
	}
	for _, workflow := range state.WorkflowRuns {
		if workflow.RunID == runID || workflow.WorkflowID == runID {
			return workflow, true
		}
	}
	for _, item := range state.WorkItems {
		for _, workflow := range item.LinkedWorkflows {
			if workflow.RunID == runID || workflow.WorkflowID == runID {
				return workflow, true
			}
		}
	}
	return domain.WorkflowRunRef{}, false
}

func agentsForWorkflow(state *domain.MissionControlState, runID string) []domain.AgentRef {
	if state == nil {
		return nil
	}
	var agents []domain.AgentRef
	seen := make(map[string]struct{})
	appendAgent := func(agent domain.AgentRef) {
		if agent.WorkflowRunID != runID {
			return
		}
		if _, ok := seen[agent.AgentID]; ok {
			return
		}
		seen[agent.AgentID] = struct{}{}
		agents = append(agents, agent)
	}
	for _, agent := range state.Agents {
		appendAgent(agent)
	}
	for _, item := range state.WorkItems {
		for _, agent := range item.LinkedAgents {
			appendAgent(agent)
		}
	}
	return agents
}

func workflowPaneForWorktree(state *domain.MissionControlState, worktree domain.Worktree) (string, string) {
	if state == nil {
		return "", ""
	}
	bestPane := ""
	bestLabel := ""
	bestPriority := 0
	var bestUpdatedAt time.Time
	for _, workflow := range state.WorkflowRuns {
		priority := 0
		switch strings.ToLower(workflow.Status) {
		case domain.WorkflowRunning:
			priority = 4
		case domain.WorkflowBlocked:
			priority = 3
		case domain.WorkflowQueued:
			priority = 2
		case domain.WorkflowFailed:
			priority = 1
		default:
			continue
		}
		if !pathsEqual(workflow.WorktreePath, worktree.Path) && workflow.Branch != worktree.Branch {
			continue
		}
		if priority < bestPriority || (priority == bestPriority && !workflow.UpdatedAt.After(bestUpdatedAt)) {
			continue
		}
		runID := workflow.RunID
		if runID == "" {
			runID = workflow.WorkflowID
		}
		for _, agent := range agentsForWorkflow(state, runID) {
			if agent.PaneID != "" {
				label := agent.Name
				if label == "" {
					label = workflow.Title
				}
				if label == "" {
					label = worktree.Branch
				}
				bestPane = agent.PaneID
				bestLabel = label
				bestPriority = priority
				bestUpdatedAt = workflow.UpdatedAt
				break
			}
		}
	}
	return bestPane, bestLabel
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

// integrationTickCmd fires an integrationsTickMsg after the configured interval.
func integrationTickCmd(cfg *domain.Config) tea.Cmd {
	interval := time.Duration(cfg.Herdr.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return integrationsTickMsg{}
	})
}

// herdrSnapshotCmd fetches a Herdr snapshot asynchronously.
func herdrSnapshotCmd(hc sessionHealthChecker) tea.Cmd {
	return func() tea.Msg {
		snap, err := hc.HerdrSnapshot()
		return herdrSyncedMsg{snapshot: snap, err: err}
	}
}

// sandcastleSnapshotCmd fetches a Sandcastle snapshot asynchronously.
func sandcastleSnapshotCmd(hc sessionHealthChecker) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		snap, err := hc.SandcastleSnapshot(ctx)
		return sandcastleSyncedMsg{snapshot: snap, err: err}
	}
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

type paneFocusedMsg struct {
	label string
	err   error
}

type workflowRemovedMsg struct {
	runID string
	stop  bool
	err   error
}

// herdrWorktreeOpenedMsg carries the result of opening and focusing a
// worktree through Herdr.
type herdrWorktreeOpenedMsg struct {
	session domain.Session
	err     error
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
	viewDashboard activeView = iota // Global dashboard view
	viewWorktrees                   // Shows the worktree list (default)
	viewIssues                      // Shows the GitHub issues list
	viewPRs                         // Shows the GitHub pull requests list
)

type dashboardTab int

const (
	dashboardTabActive dashboardTab = iota
	dashboardTabCompleted
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

type contextActionOption struct {
	icon         string
	label        string
	action       string
	workflowKind string
}

// Model represents the root Bubbletea model for the Nexus TUI application.
// It manages the list of git worktrees, user interactions, and active modals.
type Model struct {
	Worktrees          []domain.Worktree    // List of available git worktrees
	RepoPath           string               // Path to the repository root
	Config             *domain.Config       // Loaded application configuration
	selectedIdx        int                  // Currently selected worktree index
	activeModal        modal.Modal          // Currently open modal (if any)
	statusErr          string               // Error message to display (if any)
	statusMsg          string               // Success/info message to display (if any)
	themeIdx           int                  // Index into styles.Themes for the active theme
	view               activeView           // Currently active main panel view
	width              int                  // Terminal width in columns; 0 means use default
	height             int                  // Terminal height in rows; 0 means use default
	prs                []domain.PullRequest // Latest synced pull requests
	issues             []domain.Issue       // Latest synced issues
	lastSynced         time.Time            // When the last successful GitHub sync completed
	syncErr            error                // Error from the most recent GitHub sync attempt
	syncing            bool                 // True while a background GitHub sync is in progress
	selectedIssueIdx   int                  // Currently selected issue index
	selectedPRIdx      int                  // Currently selected PR index
	selectedMissionIdx int                  // Selected workflow/mission on the dashboard
	dashboardTab       dashboardTab         // Active or completed dashboard workflows
	focused            focusedPanel         // Which panel currently has keyboard focus
	ctxScrollOffset    int                  // Scroll position within the context panel
	contextActionIdx   int                  // Selected action in the context panel
	lastMouseX         int
	lastMouseY         int
	lastMouseAt        time.Time

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

	// Fuzzy finder state
	fuzzyAllItems []domain.SearchResult // full unfiltered search index

	// Fuzzy overlay state
	fuzzyActive  bool                  // true while the fuzzy finder overlay is open
	fuzzyInput   textinput.Model       // text input for the fuzzy query
	fuzzyResults []domain.SearchResult // filtered+ranked results for the current query
	fuzzySelIdx  int                   // selected result index
	fuzzyLoading bool                  // true while the search index is being built

	// healthChecker fetches runtime snapshots for Herdr/Sandcastle session
	// health checks. nil when not initialised (tests, standalone mode).
	healthChecker sessionHealthChecker
	// herdrNavigator opens and focuses Herdr-managed worktree panes.
	herdrNavigator herdrNavigator
	// workflowStarter launches Grove-owned Sandcastle workflows.
	workflowStarter sandcastleWorkflowStarter

	// herdrSnapshot holds the last successful Herdr snapshot.
	herdrSnapshot *herdr.Snapshot
	// sandcastleSnapshot holds the last successful Sandcastle snapshot.
	sandcastleSnapshot *sandcastle.Snapshot
	// missionState holds the latest mission-control state.
	missionState *domain.MissionControlState

	// insideHerdr is true when Grove runs inside a Herdr session (HERDR_ENV set).
	insideHerdr bool
	// lastHerdrSync is the time of the last successful Herdr snapshot.
	lastHerdrSync time.Time
	// lastSandcastleSync is the time of the last successful Sandcastle snapshot.
	lastSandcastleSync time.Time
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
		Config:      cfg,
		themeIdx:    themeIdx,
		statusErr:   configErr,
		focused:     panelList,
		insideHerdr: os.Getenv("HERDR_ENV") != "",
		fuzzyInput: func() textinput.Model {
			ti := textinput.New()
			ti.Placeholder = "Search worktrees, issues, PRs, files..."
			return ti
		}(),
	}
}

// Init initializes the model and triggers an initial worktree list load and GitHub sync.
func (m *Model) Init() tea.Cmd {
	m.syncing = true
	// Always start the session tick — it handles both grove-spawned shell sessions
	// (requires m.db) and externally-started Copilot CLI sessions (no DB needed).
	cmds := []tea.Cmd{
		m.refreshWorktreesCmd(),
		m.syncGitHubCmd(false),
		sessionTickCmd(),
		checkForUpdateCmd(),
		integrationTickCmd(m.Config),
	}
	if m.healthChecker != nil {
		if m.Config.Herdr.Enabled {
			cmds = append(cmds, herdrSnapshotCmd(m.healthChecker))
		}
		if m.Config.Sandcastle.Enabled {
			cmds = append(cmds, sandcastleSnapshotCmd(m.healthChecker))
		}
	}
	return tea.Batch(cmds...)
}

type mouseUILayout struct {
	panelTop    int
	panelBottom int
	listX       int
	contextX    int
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.activeModal != nil {
		if handler, ok := m.activeModal.(interface {
			HandleMouse(tea.MouseMsg, int, int) (tea.Model, tea.Cmd)
		}); ok {
			updated, cmd := handler.HandleMouse(msg, m.widthOrDefault(), m.heightOrDefault())
			if next, ok := updated.(modal.Modal); ok {
				m.activeModal = next
			}
			return m, cmd
		}
		return m, nil
	}

	if m.fuzzyActive {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.fuzzySelIdx > 0 {
				m.fuzzySelIdx--
			}
		case tea.MouseButtonWheelDown:
			if m.fuzzySelIdx < len(m.fuzzyResults)-1 {
				m.fuzzySelIdx++
			}
		}
		return m, nil
	}

	layout := m.mouseLayout()
	if msg.Y < layout.panelTop || msg.Y >= layout.panelBottom {
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		switch {
		case msg.X < layout.listX:
			m.focused = panelNav
		case msg.X < layout.contextX:
			m.focused = panelList
		default:
			m.focused = panelCtx
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.moveUp()
		} else {
			m.moveDown()
		}
		return m, m.maybeLazyLoadCmd()
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
	default:
		return m, nil
	}

	doubleClick := msg.X == m.lastMouseX && msg.Y == m.lastMouseY &&
		!m.lastMouseAt.IsZero() && time.Since(m.lastMouseAt) <= 450*time.Millisecond
	m.lastMouseX, m.lastMouseY, m.lastMouseAt = msg.X, msg.Y, time.Now()

	switch {
	case msg.X < layout.listX:
		return m.handleNavClick(msg, layout)
	case msg.X < layout.contextX:
		return m.handleListClick(msg, layout, doubleClick)
	default:
		return m.handleContextClick(msg, layout)
	}
}

func (m *Model) mouseLayout() mouseUILayout {
	width := m.widthOrDefault()
	height := m.heightOrDefault()
	navOuter := navPanelInner + panelOverhead
	ctxOuter := computeCtxInner(width) + panelOverhead
	listOuter := width - navOuter - ctxOuter
	if listOuter < minPathWidth+panelOverhead {
		listOuter = minPathWidth + panelOverhead
	}
	return mouseUILayout{
		panelTop:    1,
		panelBottom: max(1, height-2),
		listX:       navOuter,
		contextX:    navOuter + listOuter,
	}
}

func (m *Model) widthOrDefault() int {
	if m.width > 0 {
		return m.width
	}
	return defaultTermWidth
}

func (m *Model) heightOrDefault() int {
	if m.height > 0 {
		return m.height
	}
	return 24
}

func (m *Model) handleNavClick(msg tea.MouseMsg, layout mouseUILayout) (tea.Model, tea.Cmd) {
	row := msg.Y - layout.panelTop - 1
	if row < 0 || row >= len(navItems) {
		return m, nil
	}
	m.focused = panelNav
	m.view = activeView(row)
	m.ctxScrollOffset = 0
	m.contextActionIdx = 0
	m.currentPage = 0
	return m, nil
}

func (m *Model) handleListClick(msg tea.MouseMsg, layout mouseUILayout, doubleClick bool) (tea.Model, tea.Cmd) {
	m.focused = panelList
	contentRow := msg.Y - layout.panelTop - 1
	if contentRow < 0 {
		return m, nil
	}

	switch m.view {
	case viewDashboard:
		const dashboardTabRow = 13
		const dashboardFirstMissionRow = 14
		if contentRow == dashboardTabRow {
			relativeX := msg.X - layout.listX - 2
			if relativeX >= 39 {
				m.dashboardTab = dashboardTabCompleted
			} else {
				m.dashboardTab = dashboardTabActive
			}
			m.selectedMissionIdx = 0
			return m, nil
		}
		if contentRow < dashboardFirstMissionRow {
			return m, nil
		}
		visibleRow := (contentRow - dashboardFirstMissionRow) / 2
		missions := dashboardMissionsForTab(m.missionState, m.dashboardTab)
		start := 0
		if m.selectedMissionIdx >= 5 {
			start = m.selectedMissionIdx - 4
		}
		idx := start + visibleRow
		if idx < 0 || idx >= len(missions) {
			return m, nil
		}
		m.selectedMissionIdx = idx
		if doubleClick {
			return m.activateSelectedItem()
		}
	case viewIssues:
		row := contentRow - 1
		treeRows := buildIssueTree(m.issues)
		if row < 0 || row >= min(pageSize, len(treeRows)-m.currentPage*pageSize) {
			return m, nil
		}
		treeIdx := m.currentPage*pageSize + row
		if treeIdx < len(treeRows) {
			m.selectedIssueIdx = treeRows[treeIdx].originalIdx
			if doubleClick {
				return m.activateSelectedItem()
			}
		}
	case viewPRs:
		row := contentRow - 1
		idx := m.currentPage*pageSize + row
		if row < 0 || idx >= len(m.prs) {
			return m, nil
		}
		m.selectedPRIdx = idx
		if doubleClick {
			return m.activateSelectedItem()
		}
	default:
		row := contentRow - 1
		start := 0
		panelHeight := m.heightOrDefault() - fixedChromeRows
		maxItems := panelHeight - 1
		if maxItems > 0 && m.selectedIdx >= maxItems {
			start = m.selectedIdx - maxItems + 1
		}
		idx := start + row
		if row < 0 || idx >= len(m.Worktrees) {
			return m, nil
		}
		m.selectedIdx = idx
		if doubleClick {
			return m.activateSelectedItem()
		}
	}
	return m, nil
}

func (m *Model) handleContextClick(msg tea.MouseMsg, layout mouseUILayout) (tea.Model, tea.Cmd) {
	actions := m.availableContextActions()
	if m.ctxScrollOffset > 0 {
		m.focused = panelCtx
		return m, nil
	}
	actionRow := msg.Y - layout.panelTop - 2
	if actionRow < 0 || actionRow >= len(actions) {
		m.focused = panelCtx
		return m, nil
	}
	m.focused = panelCtx
	m.contextActionIdx = actionRow
	return m.runSelectedContextAction()
}

// Update handles incoming messages and returns an updated model and command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		return m.handleMouse(mouseMsg)
	}

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
			return m, m.removeWorktreeCmd(msg.Path, msg.Branch)
		case modal.CleanupConfirmedMsg:
			m.activeModal = nil
			return m, m.performCleanupCmd(msg.Worktrees, msg.Branches)
		case modal.UpdateConfirmedMsg:
			m.activeModal = nil
			m.selfUpdating = true
			return m, selfUpdateCmd(m.latestVersion)
		case modal.WorkflowLaunchMsg:
			m.activeModal = nil
			m.statusMsg = fmt.Sprintf("Starting %s workflow…", msg.Kind)
			return m, m.startSandcastleWorkflowCmd(msg)
		case modal.MissionJumpMsg:
			m.activeModal = nil
			return m.jumpToMissionRun(msg.RunID)
		case modal.MissionRefreshMsg:
			var cmds []tea.Cmd
			if m.healthChecker != nil {
				if m.Config.Herdr.Enabled {
					cmds = append(cmds, herdrSnapshotCmd(m.healthChecker))
				}
				if m.Config.Sandcastle.Enabled {
					cmds = append(cmds, sandcastleSnapshotCmd(m.healthChecker))
				}
			}
			return m, tea.Batch(cmds...)
		case modal.MissionRetryRequestedMsg:
			m.activeModal = nil
			return m.retryWorkflowByRunID(msg.RunID)
		case modal.MissionRemoveRequestedMsg:
			workflow, ok := workflowByRunID(m.missionState, msg.RunID)
			if !ok {
				m.activeModal = nil
				m.statusErr = fmt.Sprintf("Workflow %s is no longer available", msg.RunID)
				return m, clearErrorCmd()
			}
			m.activeModal = modal.NewWorkflowRemoveModal(workflow)
			return m, nil
		case modal.WorkflowRemoveConfirmedMsg:
			m.activeModal = nil
			m.statusMsg = "Removing workflow…"
			return m, m.removeWorkflowCmd(msg.RunID, msg.Stop)
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
			// Only key events are consumed by the modal. Non-key messages
			// (e.g. githubSyncedMsg, debouncedRenderMsg, syncTickMsg) must fall
			// through to the main switch so background events are never silently
			// swallowed while a modal is open.
			if _, ok := msg.(tea.KeyMsg); ok {
				updated, cmd := m.activeModal.Update(msg)
				if next, ok := updated.(modal.Modal); ok {
					m.activeModal = next
				}
				return m, cmd
			}
		}
	}

	// While the fuzzy finder overlay is open, route key events here.
	// Non-key messages (e.g. fuzzyResultsReadyMsg, tea.WindowSizeMsg) fall
	// through to the main switch so background events are never dropped.
	if m.fuzzyActive {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.Type {
			case tea.KeyEsc:
				m.fuzzyActive = false
				m.fuzzyInput.SetValue("")
				m.fuzzyResults = nil
				m.fuzzySelIdx = 0
				return m, nil
			case tea.KeyEnter:
				cmd := m.fuzzyConfirmSelection()
				m.fuzzyActive = false
				m.fuzzyInput.SetValue("")
				m.fuzzyResults = nil
				m.fuzzySelIdx = 0
				return m, cmd
			case tea.KeyUp:
				if m.fuzzySelIdx > 0 {
					m.fuzzySelIdx--
				}
				return m, nil
			case tea.KeyDown:
				if m.fuzzySelIdx < len(m.fuzzyResults)-1 {
					m.fuzzySelIdx++
				}
				return m, nil
			default:
				var inputCmd tea.Cmd
				m.fuzzyInput, inputCmd = m.fuzzyInput.Update(keyMsg)
				query := m.fuzzyInput.Value()
				m.fuzzyResults = fuzzy.FilterAndRank(query, m.fuzzyAllItems)
				m.fuzzySelIdx = 0
				return m, inputCmd
			}
		}
		// Non-key message: fall through to the main switch.
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Dismiss any visible error overlay on the next keypress.
		m.statusErr = ""
		switch msg.Type {
		case tea.KeyTab:
			m.focused = (m.focused + 1) % panelCount
			if m.focused == panelCtx {
				m.contextActionIdx = 0
			}
			return m, nil
		case tea.KeyEnter:
			if m.focused == panelCtx {
				return m.runSelectedContextAction()
			}
			return m.activateSelectedItem()
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyF1:
			m.activeModal = modal.NewHelpModal()
			return m, nil
		case tea.KeyCtrlF:
			return m, m.openFuzzyCmd()
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
		case tea.KeyRunes:
			switch msg.String() {
			case "q":
				return m, tea.Quit
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
			case "a", "A":
				m.focused = panelCtx
				m.contextActionIdx = 0
				return m, nil
			case "d", "D":
				m.view = viewDashboard
				m.ctxScrollOffset = 0
				m.contextActionIdx = 0
			case "v", "V":
				if m.view == viewDashboard {
					return m.openSelectedMissionInspector()
				}
			case "x", "X":
				if m.view == viewDashboard {
					return m.confirmSelectedWorkflowRemoval()
				}
			case "[", "]":
				if m.view == viewDashboard {
					if msg.String() == "[" {
						m.dashboardTab = dashboardTabActive
					} else {
						m.dashboardTab = dashboardTabCompleted
					}
					m.selectedMissionIdx = 0
					return m, nil
				}
			case "w", "W":
				m.view = viewWorktrees
				m.ctxScrollOffset = 0
				m.contextActionIdx = 0
				m.currentPage = 0
			case "i", "I":
				m.view = viewIssues
				m.ctxScrollOffset = 0
				m.contextActionIdx = 0
				m.currentPage = 0
			case "p", "P":
				m.view = viewPRs
				m.ctxScrollOffset = 0
				m.contextActionIdx = 0
				m.currentPage = 0
			case "r", "R":
				if m.syncing {
					return m, nil
				}
				m.syncing = true
				cmds := tea.Batch(m.refreshWorktreesCmd(), m.syncGitHubCmd(true))
				if m.healthChecker != nil {
					if m.Config.Herdr.Enabled {
						cmds = tea.Batch(cmds, herdrSnapshotCmd(m.healthChecker))
					}
					if m.Config.Sandcastle.Enabled {
						cmds = tea.Batch(cmds, sandcastleSnapshotCmd(m.healthChecker))
					}
				}
				return m, cmds
			case "n":
				m.nextPage()
				return m, nil

			case "/":
				return m, m.openFuzzyCmd()
			}
		}

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
		} else {
			m.statusErr = fmt.Sprintf("failed to load worktrees: %v", msg.err)
			return m, clearErrorCmd()
		}

	case browserOpenErrMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to open in browser: %v", msg.err)
			return m, clearErrorCmd()
		}

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
				if pending.prs != nil || pending.issues != nil {
					m.statusErr = fmt.Sprintf("GitHub sync degraded; showing available data: %v", pending.err)
				} else {
					m.statusErr = fmt.Sprintf("GitHub sync failed: %v", pending.err)
				}
			}
			prsUpdated := pending.prs != nil
			issuesUpdated := pending.issues != nil
			if prsUpdated {
				m.prs = pending.prs
				m.clampPRIdx()
			}
			if issuesUpdated {
				m.issues = pending.issues
				m.issueTree = buildIssueTree(m.issues)
				m.clampIssueIdx()
			}
			if prsUpdated || issuesUpdated {
				m.lastSynced = pending.syncedAt
			}
			if prsUpdated {
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
				return m, tea.Batch(nextTick, clearErrorCmd(), m.rebuildMissionStateCmd())
			}
			return m, tea.Batch(nextTick, m.rebuildMissionStateCmd())
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
		return m, m.syncGitHubCmd(false)

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

	case integrationsTickMsg:
		var cmds []tea.Cmd
		if m.Config.Herdr.Enabled && m.healthChecker != nil {
			cmds = append(cmds, herdrSnapshotCmd(m.healthChecker))
		}
		if m.Config.Sandcastle.Enabled && m.healthChecker != nil {
			cmds = append(cmds, sandcastleSnapshotCmd(m.healthChecker))
		}
		cmds = append(cmds, integrationTickCmd(m.Config))
		return m, tea.Batch(cmds...)

	case herdrSyncedMsg:
		if msg.err == nil {
			m.herdrSnapshot = &msg.snapshot
			m.lastHerdrSync = time.Now()
		}
		return m, m.rebuildMissionStateCmd()

	case sandcastleWorkflowStartedMsg:
		if msg.err != nil {
			m.statusMsg = ""
			m.statusErr = fmt.Sprintf("Failed to start %s workflow: %v", msg.kind, msg.err)
			return m, clearErrorCmd()
		}
		if m.sandcastleSnapshot == nil {
			m.sandcastleSnapshot = &sandcastle.Snapshot{
				Integration: domain.ExternalIntegration{
					Name:      "sandcastle",
					Mode:      "connected",
					Available: true,
					Enabled:   true,
				},
			}
		}
		m.sandcastleSnapshot.Workflows = append(m.sandcastleSnapshot.Workflows, msg.workflow)
		m.statusMsg = fmt.Sprintf("Started %s workflow", msg.kind)
		cmds := []tea.Cmd{clearMsgCmd(), m.rebuildMissionStateCmd()}
		if m.healthChecker != nil {
			cmds = append(cmds, sandcastleSnapshotCmd(m.healthChecker))
		}
		return m, tea.Batch(cmds...)

	case sandcastleSyncedMsg:
		if msg.err == nil {
			m.sandcastleSnapshot = &msg.snapshot
			m.lastSandcastleSync = time.Now()
		} else {
			// Keep the last known workflow data while surfacing the current
			// integration failure instead of making Sandcastle look absent.
			if m.sandcastleSnapshot != nil {
				msg.snapshot.Workflows = m.sandcastleSnapshot.Workflows
				msg.snapshot.Agents = m.sandcastleSnapshot.Agents
				msg.snapshot.CapturedAt = m.sandcastleSnapshot.CapturedAt
			}
			m.sandcastleSnapshot = &msg.snapshot
		}
		return m, m.rebuildMissionStateCmd()

	case missionControlUpdatedMsg:
		m.missionState = &msg.state
		missions := dashboardMissionsForTab(m.missionState, m.dashboardTab)
		if len(missions) == 0 {
			m.selectedMissionIdx = 0
		} else if m.selectedMissionIdx >= len(missions) {
			m.selectedMissionIdx = len(missions) - 1
		}
		if inspector, ok := m.activeModal.(*modal.MissionModal); ok {
			if workflow, found := workflowByRunID(m.missionState, inspector.RunID()); found {
				inspector.SetWorkflow(workflow, agentsForWorkflow(m.missionState, inspector.RunID()))
			}
		}

	case sessionFocusedMsg:
		// Focus is best-effort; show a friendly toast regardless of outcome.
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Session active for %s (could not bring to front: %v)", msg.worktreePath, msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Focused session for %s", msg.worktreePath)
		}
		return m, clearMsgCmd()

	case paneFocusedMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to focus %s: %v", msg.label, msg.err)
			return m, clearErrorCmd()
		}
		m.statusMsg = fmt.Sprintf("Focused %s", msg.label)
		return m, clearMsgCmd()

	case workflowRemovedMsg:
		if msg.err != nil {
			m.statusMsg = ""
			m.statusErr = fmt.Sprintf("Failed to remove workflow: %v", msg.err)
			return m, clearErrorCmd()
		}
		if m.sandcastleSnapshot != nil {
			filteredWorkflows := m.sandcastleSnapshot.Workflows[:0]
			for _, workflow := range m.sandcastleSnapshot.Workflows {
				if workflow.RunID != msg.runID && workflow.WorkflowID != msg.runID {
					filteredWorkflows = append(filteredWorkflows, workflow)
				}
			}
			m.sandcastleSnapshot.Workflows = filteredWorkflows
			filteredAgents := m.sandcastleSnapshot.Agents[:0]
			for _, agent := range m.sandcastleSnapshot.Agents {
				if agent.WorkflowRunID != msg.runID {
					filteredAgents = append(filteredAgents, agent)
				}
			}
			m.sandcastleSnapshot.Agents = filteredAgents
		}
		if msg.stop {
			m.statusMsg = "Workflow stopped and removed"
		} else {
			m.statusMsg = "Workflow removed from history"
		}
		return m, tea.Batch(clearMsgCmd(), m.rebuildMissionStateCmd())

	case herdrWorktreeOpenedMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to open worktree in Herdr: %v", msg.err)
			return m, clearErrorCmd()
		}
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
		m.statusMsg = fmt.Sprintf("Opened Herdr pane for %s", msg.session.WorktreePath)
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
			var permErr *updater.PermissionError
			if errors.As(msg.err, &permErr) {
				m.statusErr = fmt.Sprintf("Update staged — run: %s", permErr.InstallCmd)
				slog.Info("self-update requires elevated permissions",
					"staged_path", permErr.StagedPath,
					"install_cmd", permErr.InstallCmd)
			} else {
				m.statusErr = fmt.Sprintf("Update failed: %v", msg.err)
			}
			return m, clearErrorCmd()
		}
		m.statusMsg = "✓ Updated successfully! Please restart grove to use the new version."
		return m, clearMsgCmd()

	case cleanupLoadedMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Failed to load cleanup data: %v", msg.err)
			return m, clearErrorCmd()
		}
		m.activeModal = modal.NewCleanupModal(msg.candidates)
		return m, nil

	case cleanupDoneMsg:
		if msg.err != nil {
			m.statusErr = fmt.Sprintf("Cleanup failed: %v", msg.err)
			return m, tea.Batch(m.refreshWorktreesCmd(), clearErrorCmd())
		}
		m.statusMsg = fmt.Sprintf("✓ Cleaned up %d item(s)", msg.deleted)
		return m, tea.Batch(m.refreshWorktreesCmd(), clearMsgCmd())

	case fuzzyOpenMsg:
		return m, m.buildSearchIndexCmd()

	case fuzzyResultsReadyMsg:
		m.fuzzyAllItems = msg.results
		// Always clear the loading flag regardless of whether the overlay is
		// still open — prevents stale loading state if the user closed the
		// overlay before the index finished building.
		m.fuzzyLoading = false
		// If the overlay is still open, populate results now that the index is ready.
		if m.fuzzyActive {
			query := m.fuzzyInput.Value()
			m.fuzzyResults = fuzzy.FilterAndRank(query, m.fuzzyAllItems)
			m.fuzzySelIdx = 0
		}
	}

	return m, nil
}

// View returns a string representation of the model's current state.
func (m *Model) View() string {
	actions := m.availableContextActions()
	actionIdx := m.contextActionIdx
	if actionIdx >= len(actions) {
		actionIdx = max(0, len(actions)-1)
	}
	baseView := renderFull(m.Worktrees, m.selectedIdx, m.RepoPath, m.themeIdx, m.view, m.width, m.height, m.syncing, m.lastSynced, m.syncErr, m.issues, m.selectedIssueIdx, m.prs, m.selectedPRIdx, m.focused, m.ctxScrollOffset, m.currentPage, m.sessions, func() *domain.ExternalIntegration {
		if m.herdrSnapshot != nil {
			return &m.herdrSnapshot.Integration
		}
		return nil
	}(), func() *domain.ExternalIntegration {
		if m.sandcastleSnapshot != nil {
			return &m.sandcastleSnapshot.Integration
		}
		return nil
	}(), m.missionState, actionIdx, m.selectedMissionIdx, int(m.dashboardTab))

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
		if ha, ok := m.activeModal.(interface{ SetHeight(int) }); ok {
			ha.SetHeight(h)
		}
		if fs, ok := m.activeModal.(interface{ Fullscreen() bool }); ok && fs.Fullscreen() && w >= 64 && h >= 20 {
			theme := styles.NewTheme(styles.Themes[m.themeIdx])
			box := theme.RenderFullscreenBox(m.activeModal.Title(), m.activeModal.View(), w-4, h-2)
			return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
		}
		return overlay(m.activeModal.Title(), m.activeModal.View())
	}

	if m.fuzzyActive {
		theme := styles.NewTheme(styles.Themes[m.themeIdx])
		overlayContent := renderFuzzyOverlay(m.fuzzyInput, m.fuzzyResults, m.fuzzySelIdx, theme, m.fuzzyLoading, w)
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, overlayContent)
	}

	if m.selfUpdating {
		return overlay("Updating grove", "Downloading and installing update...\n\nPlease wait.")
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
// When force is true, the TTL cache check is skipped and the GitHub CLI is always called.
func (m *Model) syncGitHubCmd(force bool) tea.Cmd {
	repoPath := m.RepoPath
	db := m.db
	ttl := m.Config.GitHub.SyncInterval()
	return func() tea.Msg {
		// If db is available, check cache staleness before hitting the CLI.
		// Skip this check when force=true (explicit user-initiated refresh).
		if !force && db != nil {
			prStale, _ := data.IsCacheStale(db, data.CacheTablePRs, ttl, repoPath)
			issStale, _ := data.IsCacheStale(db, data.CacheTableIssues, ttl, repoPath)
			if !prStale && !issStale {
				// Cache is fresh — return cached rows without calling gh.
				repo := data.NewGitHubRepository(db, repoPath)
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
		var attentionErr error
		if prErr == nil {
			attentionErr = prCmd.EnrichViewerAttention(prs)
		}

		// Persist enriched issues and PRs to DB so the next fresh-cache read
		// returns hierarchy-enriched data rather than a flat list.
		if db != nil {
			ghRepo := data.NewGitHubRepository(db, repoPath)
			if issErr == nil {
				_ = ghRepo.UpsertIssues(issues)
			}
			if prErr == nil {
				_ = ghRepo.UpsertPRs(prs)
			}
		}

		return githubSyncedMsg{
			prs:      prs,
			issues:   issues,
			err:      errors.Join(issErr, prErr, attentionErr),
			syncedAt: time.Now(),
		}
	}
}

// rebuildMissionStateCmd returns a Cmd that rebuilds mission-control state
// from the current model snapshots. Model fields are captured by value to
// avoid data races in the goroutine.
func (m *Model) rebuildMissionStateCmd() tea.Cmd {
	worktrees := append([]domain.Worktree(nil), m.Worktrees...)
	issues := append([]domain.Issue(nil), m.issues...)
	prs := append([]domain.PullRequest(nil), m.prs...)
	sessions := append([]domain.Session(nil), m.sessions...)

	var herdrSnap *herdr.Snapshot
	if m.herdrSnapshot != nil {
		hs := *m.herdrSnapshot
		herdrSnap = &hs
	}
	var scSnap *sandcastle.Snapshot
	if m.sandcastleSnapshot != nil {
		ss := *m.sandcastleSnapshot
		scSnap = &ss
	}
	var prev *domain.MissionControlState
	if m.missionState != nil {
		ps := *m.missionState
		prev = &ps
	}

	repoPath := m.RepoPath
	insideHerdr := m.insideHerdr
	githubLastSync := m.lastSynced
	githubSyncError := ""
	if m.syncErr != nil {
		githubSyncError = m.syncErr.Error()
	}
	return func() tea.Msg {
		state := mission.BuildState(mission.BuildInput{
			RepoPath:           repoPath,
			Worktrees:          worktrees,
			Issues:             issues,
			PullRequests:       prs,
			Sessions:           sessions,
			GitHubLastSync:     githubLastSync,
			GitHubSyncError:    githubSyncError,
			HerdrSnapshot:      herdrSnap,
			SandcastleSnapshot: scSnap,
			PreviousState:      prev,
			Now:                time.Now(),
			InsideHerdr:        insideHerdr,
		})
		return missionControlUpdatedMsg{state: state}
	}
}

// addWorktreeCmd returns a Cmd that creates a new git worktree with a new branch.
// baseBranch is the branch to base off; empty string auto-detects the repo's default branch.
func (m *Model) addWorktreeCmd(branch, path, baseBranch string) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return worktreeOpDoneMsg{err: fmt.Errorf("create worktree parent dir: %w", err)}
		}
		cmd := internalexec.NewGitCommand(repoPath)
		if baseBranch == "" {
			baseBranch = cmd.DefaultBranch()
		}
		err := cmd.AddWorktreeNewBranch(path, branch, baseBranch)
		return worktreeOpDoneMsg{err: err}
	}
}

// checkoutPRWorktreeCmd returns a Cmd that fetches a remote PR branch and creates a worktree for it.
func (m *Model) checkoutPRWorktreeCmd(branch, path string) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return worktreeOpDoneMsg{err: fmt.Errorf("create worktree parent dir: %w", err)}
		}
		cmd := internalexec.NewGitCommand(repoPath)
		err := cmd.CheckoutPRWorktree(path, branch)
		return worktreeOpDoneMsg{err: err}
	}
}

// branchSlug converts a branch name to a filesystem-safe slug by replacing
// forward slashes with dashes.
func branchSlug(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

// prWorktreePath derives the filesystem path for a PR worktree using the same
// convention as issue worktrees: ../worktrees/<repo>/<branch-with-slashes-as-dashes>.
// The repo name is included to avoid path collisions when multiple projects share the same parent directory.
func prWorktreePath(repoPath, branch string) string {
	return filepath.Join(filepath.Dir(repoPath), "worktrees", filepath.Base(repoPath), branchSlug(branch))
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

// removeWorktreeCmd removes a git worktree and its local branch.
func (m *Model) removeWorktreeCmd(path, branch string) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		cmd := internalexec.NewGitCommand(repoPath)
		return worktreeOpDoneMsg{err: cmd.RemoveWorktreeAndBranch(path, branch)}
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
	navigator := m.herdrNavigator
	return func() tea.Msg {
		if session.Runtime == domain.RuntimeHerdr {
			if navigator == nil {
				return sessionFocusedMsg{worktreePath: session.WorktreePath, err: fmt.Errorf("Herdr navigation unavailable")}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := navigator.OpenWorktree(ctx, herdr.OpenWorktreeRequest{Path: session.WorktreePath})
			return sessionFocusedMsg{worktreePath: session.WorktreePath, err: err}
		}
		pid := 0
		if session.ShellPID != nil {
			pid = *session.ShellPID
		}
		err := focusSessionWindow(pid)
		return sessionFocusedMsg{worktreePath: session.WorktreePath, err: err}
	}
}

// openHerdrWorktreeCmd opens and focuses a worktree in Herdr, then persists
// the resulting runtime-backed session.
func (m *Model) openHerdrWorktreeCmd(worktreePath string) tea.Cmd {
	navigator := m.herdrNavigator
	db := m.db
	return func() tea.Msg {
		if navigator == nil {
			return herdrWorktreeOpenedMsg{err: fmt.Errorf("Herdr navigation unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pane, err := navigator.OpenWorktree(ctx, herdr.OpenWorktreeRequest{Path: worktreePath})
		if err != nil {
			return herdrWorktreeOpenedMsg{err: err}
		}
		if pane == nil || pane.PaneID == "" {
			return herdrWorktreeOpenedMsg{err: fmt.Errorf("Herdr returned no pane for %s", worktreePath)}
		}
		paneID := pane.PaneID
		session := domain.Session{
			WorktreePath: worktreePath,
			Runtime:      domain.RuntimeHerdr,
			RuntimeID:    paneID,
			PaneID:       &paneID,
			Status:       domain.StatusActive,
			StartedAt:    time.Now().UTC().Truncate(time.Second),
		}
		if db != nil {
			id, err := data.UpsertSession(db, session)
			if err != nil {
				return herdrWorktreeOpenedMsg{err: fmt.Errorf("track Herdr session: %w", err)}
			}
			session.ID = id
		}
		return herdrWorktreeOpenedMsg{session: session}
	}
}

// sessionHealthChecker fetches runtime snapshots for session health checks.
// Production implementation wraps real herdr/sandcastle clients.
// Tests inject a fake or leave it nil.
type sessionHealthChecker interface {
	HerdrSnapshot() (herdr.Snapshot, error)
	SandcastleSnapshot(ctx context.Context) (sandcastle.Snapshot, error)
}

type herdrNavigator interface {
	OpenWorktree(ctx context.Context, req herdr.OpenWorktreeRequest) (*domain.PaneRef, error)
	FocusPane(ctx context.Context, paneID string) error
}

type sandcastleWorkflowStarter interface {
	StartWorkflow(ctx context.Context, req sandcastle.StartWorkflowRequest) (domain.WorkflowRunRef, error)
	RemoveWorkflow(ctx context.Context, repoPath, runID string, stop bool) error
}

func (m *Model) startSandcastleWorkflowCmd(msg modal.WorkflowLaunchMsg) tea.Cmd {
	starter := m.workflowStarter
	repoPath := m.RepoPath
	defaultAgent := m.Config.Sandcastle.DefaultAgent
	if msg.RepoPath != "" {
		repoPath = msg.RepoPath
	}
	if msg.AgentKind != "" {
		defaultAgent = msg.AgentKind
	}
	return func() tea.Msg {
		if starter == nil {
			return sandcastleWorkflowStartedMsg{kind: msg.Kind, err: fmt.Errorf("runtime unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		workflow, err := starter.StartWorkflow(ctx, sandcastle.StartWorkflowRequest{
			Kind:        msg.Kind,
			RepoPath:    repoPath,
			IssueNumber: msg.IssueNumber,
			PRNumber:    msg.PRNumber,
			AgentKind:   defaultAgent,
			Source:      "grove",
		})
		return sandcastleWorkflowStartedMsg{workflow: workflow, kind: msg.Kind, err: err}
	}
}

func (m *Model) removeWorkflowCmd(runID string, stop bool) tea.Cmd {
	manager := m.workflowStarter
	repoPath := m.RepoPath
	return func() tea.Msg {
		if manager == nil {
			return workflowRemovedMsg{runID: runID, stop: stop, err: fmt.Errorf("Sandcastle runtime unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := manager.RemoveWorkflow(ctx, repoPath, runID, stop)
		return workflowRemovedMsg{runID: runID, stop: stop, err: err}
	}
}

// defaultHealthChecker is the production sessionHealthChecker backed by real
// herdr and sandcastle clients.
type defaultHealthChecker struct {
	herdr      herdr.Client
	sandcastle sandcastle.Client
	repoPath   string
}

func (c *defaultHealthChecker) HerdrSnapshot() (herdr.Snapshot, error) {
	return c.herdr.Snapshot()
}

func (c *defaultHealthChecker) SandcastleSnapshot(ctx context.Context) (sandcastle.Snapshot, error) {
	return c.sandcastle.Snapshot(ctx, c.repoPath)
}

// strPtr returns a pointer to the given string value.
func strPtr(s string) *string { return &s }

// enrichHerdrSessions applies snapshot-based liveness checks to Herdr-backed
// sessions. Non-Herdr sessions are passed through unchanged. When the Herdr
// snapshot is unavailable, sessions are marked degraded rather than deleted.
// Sessions whose Sandcastle workflow has terminated (failed/succeeded) are
// removed from the returned list.
func enrichHerdrSessions(
	sessions []domain.Session,
	herdrSnap *herdr.Snapshot,
	herdrErr error,
	scSnap *sandcastle.Snapshot,
	scErr error,
) []domain.Session {
	var alive []domain.Session
	for _, s := range sessions {
		if s.Runtime != domain.RuntimeHerdr {
			alive = append(alive, s)
			continue
		}
		session := s
		session.DegradedReason = nil // Reset before health checks so stale reasons are cleared.

		// Herdr snapshot-based liveness.
		switch {
		case herdrErr != nil:
			session.DegradedReason = strPtr("herdr snapshot unavailable")
		case herdrSnap != nil:
			if session.PaneID != nil {
				found := false
				for _, p := range herdrSnap.Panes {
					if p.PaneID == *session.PaneID {
						found = true
						break
					}
				}
				if !found {
					session.DegradedReason = strPtr("herdr pane not found")
				}
			} else {
				session.DegradedReason = strPtr("herdr pane not found")
			}
		default:
			// Standalone mode: no snapshot, no error — mark degraded.
			session.DegradedReason = strPtr("herdr snapshot unavailable")
		}

		// Sandcastle workflow check (supplementary).
		dead := false
		if session.WorkflowRunID != nil {
			switch {
			case scErr != nil:
				if session.DegradedReason == nil {
					session.DegradedReason = strPtr("sandcastle status unavailable")
				}
			case scSnap != nil:
				for _, wf := range scSnap.Workflows {
					if wf.RunID == *session.WorkflowRunID {
						switch wf.Status {
						case domain.WorkflowFailed, domain.WorkflowSucceeded:
							dead = true
						case domain.WorkflowBlocked:
							if session.DegradedReason == nil {
								session.DegradedReason = strPtr("sandcastle workflow blocked")
							}
						}
						break
					}
				}
			}
		}

		if !dead {
			alive = append(alive, session)
		}
	}
	return alive
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
			// Herdr sessions are always kept — their liveness is determined
			// by the Herdr runtime, not a local PID. Full snapshot-based
			// health checks are deferred to a future phase.
			if s.Runtime == domain.RuntimeHerdr {
				alive = append(alive, s)
				continue
			}
			// Local sessions (or legacy with empty Runtime) use the
			// existing 24-hour heuristic.
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

// checkSessionsCmd reads all tracked sessions from the grove DB and checks
// whether each PID is still alive. Returns sessionStatusUpdatedMsg with the
// live session list. Dead and stale sessions are removed from the DB when a DB
// connection is available.
func (m *Model) checkSessionsCmd() tea.Cmd {
	db := m.db
	current := m.sessions
	hc := m.healthChecker
	return func() tea.Msg {
		var alive []domain.Session

		if db == nil {
			// No grove DB — perform PID health checks on in-memory sessions only.
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

		// Enrich Herdr-backed sessions with snapshot-based liveness.
		if hc != nil {
			// Snapshot IDs before enrichment so we can delete sessions that
			// enrichHerdrSessions removes (e.g. workflows that failed/succeeded).
			preEnrichIDs := make(map[int64]bool, len(alive))
			if db != nil {
				for _, s := range alive {
					preEnrichIDs[s.ID] = true
				}
			}

			var herdrSnap *herdr.Snapshot
			var herdrErr error
			snap, err := hc.HerdrSnapshot()
			herdrSnap, herdrErr = &snap, err
			ctx := context.Background()
			scSnap, scErr := hc.SandcastleSnapshot(ctx)
			alive = enrichHerdrSessions(alive, herdrSnap, herdrErr, &scSnap, scErr)

			// Delete sessions removed by enrichment from the DB.
			if db != nil {
				postEnrichIDs := make(map[int64]bool, len(alive))
				for _, s := range alive {
					postEnrichIDs[s.ID] = true
				}
				for id := range preEnrichIDs {
					if !postEnrichIDs[id] {
						if err := data.DeleteSession(db, id); err != nil {
							slog.Warn("session health check: failed to delete enrichment-pruned session", "id", id, "err", err)
						}
					}
				}

				// Persist Herdr session state so degraded reasons and recoveries
				// are saved to the DB. Only Herdr sessions are enriched, so
				// only those need persistence.
				for i := range alive {
					if alive[i].Runtime == domain.RuntimeHerdr {
						if _, err := data.UpsertSession(db, alive[i]); err != nil {
							slog.Warn("session health check: failed to persist session state", "id", alive[i].ID, "err", err)
						}
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

// buildSearchIndexCmd builds the unified search index used by the fuzzy finder.
// It concurrently fetches files, branches, agent history, and commits from the
// repository, combining them with already-cached worktrees, issues, and PRs.
// Errors from individual async sources are logged at debug level and skipped —
// they are non-fatal so that a missing git binary or empty DB does not break
// the fuzzy finder entirely.
func (m *Model) buildSearchIndexCmd() tea.Cmd {
	// Snapshot all model state upfront to avoid data races inside the goroutine.
	worktrees := make([]domain.Worktree, len(m.Worktrees))
	copy(worktrees, m.Worktrees)
	issues := make([]domain.Issue, len(m.issues))
	copy(issues, m.issues)
	prs := make([]domain.PullRequest, len(m.prs))
	copy(prs, m.prs)
	repoPath := m.RepoPath
	db := m.db

	return func() tea.Msg {
		var (
			mu       sync.Mutex
			allItems []domain.SearchResult
			wg       sync.WaitGroup
		)

		// append is called from goroutines — always hold mu.
		appendItems := func(items []domain.SearchResult) {
			mu.Lock()
			allItems = append(allItems, items...)
			mu.Unlock()
		}

		// Seed from cached sources immediately (no I/O required).
		var cached []domain.SearchResult
		for _, wt := range worktrees {
			cached = append(cached, domain.SearchResult{
				Kind:    domain.KindWorktree,
				Label:   wt.Path,
				Sub:     wt.Branch,
				Icon:    "🌿",
				Payload: wt,
			})
		}
		for _, iss := range issues {
			cached = append(cached, domain.SearchResult{
				Kind:    domain.KindIssue,
				Label:   iss.Title,
				Sub:     fmt.Sprintf("#%d", iss.Number),
				Icon:    "🐛",
				Payload: iss,
			})
		}
		for _, pr := range prs {
			cached = append(cached, domain.SearchResult{
				Kind:    domain.KindPR,
				Label:   pr.Title,
				Sub:     fmt.Sprintf("#%d", pr.Number),
				Icon:    "🔀",
				Payload: pr,
			})
		}
		appendItems(cached)

		// git ls-files
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := exec.Command("git", "-C", repoPath, "ls-files").Output()
			if err != nil {
				slog.Debug("buildSearchIndexCmd: git ls-files failed", "err", err)
				return
			}
			var items []domain.SearchResult
			for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if f == "" {
					continue
				}
				items = append(items, domain.SearchResult{
					Kind:    domain.KindFile,
					Label:   f,
					Sub:     "",
					Icon:    "📄",
					Payload: f,
				})
			}
			appendItems(items)
		}()

		// git branch -a
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := exec.Command("git", "-C", repoPath, "branch", "-a").Output()
			if err != nil {
				slog.Debug("buildSearchIndexCmd: git branch -a failed", "err", err)
				return
			}
			var items []domain.SearchResult
			for _, line := range strings.Split(string(out), "\n") {
				// Strip leading "* " (current branch marker) or leading spaces.
				b := strings.TrimPrefix(line, "* ")
				b = strings.TrimSpace(b)
				if b == "" {
					continue
				}
				items = append(items, domain.SearchResult{
					Kind:    domain.KindBranch,
					Label:   b,
					Sub:     "",
					Icon:    "🌿",
					Payload: b,
				})
			}
			appendItems(items)
		}()

		// agent history
		wg.Add(1)
		go func() {
			defer wg.Done()
			if db == nil {
				return
			}
			history, err := data.GetAgentHistory(db)
			if err != nil {
				slog.Debug("buildSearchIndexCmd: get agent history failed", "err", err)
				return
			}
			var items []domain.SearchResult
			for _, h := range history {
				label := h.Prompt
				if label == "" {
					label = h.AgentName
				}
				items = append(items, domain.SearchResult{
					Kind:    domain.KindAgent,
					Label:   label,
					Sub:     h.AgentName,
					Icon:    "🤖",
					Payload: h,
				})
			}
			appendItems(items)
		}()

		// git log --oneline -50
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := exec.Command("git", "-C", repoPath, "log", "--oneline", "-50").Output()
			if err != nil {
				slog.Debug("buildSearchIndexCmd: git log failed", "err", err)
				return
			}
			var items []domain.SearchResult
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line == "" {
					continue
				}
				// First token is the hash; the remainder is the message.
				parts := strings.SplitN(line, " ", 2)
				hash := parts[0]
				message := ""
				if len(parts) == 2 {
					message = parts[1]
				}
				items = append(items, domain.SearchResult{
					Kind:    domain.KindCommit,
					Label:   message,
					Sub:     hash,
					Icon:    "📦",
					Payload: hash,
				})
			}
			appendItems(items)
		}()

		wg.Wait()
		return fuzzyResultsReadyMsg{results: allItems}
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

func (m *Model) activateSelectedItem() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewDashboard:
		if m.dashboardTab == dashboardTabCompleted {
			return m.openSelectedMissionInspector()
		}
		return m.jumpToSelectedMission()
	case viewIssues:
		issue, ok := m.selectedIssue()
		if !ok {
			return m, nil
		}
		if session := m.sessionForIssue(issue); session != nil {
			return m, m.focusSessionCmd(*session)
		}
		m.activeModal = modal.NewCreateModalForIssue(
			issue,
			m.RepoPath,
			computeParentBranches(m.issues, m.Worktrees)...,
		)
		return m, nil
	case viewPRs:
		if len(m.prs) == 0 || m.selectedPRIdx < 0 || m.selectedPRIdx >= len(m.prs) {
			return m, nil
		}
		pr := m.prs[m.selectedPRIdx]
		if session := m.sessionForPR(pr); session != nil {
			return m, m.focusSessionCmd(*session)
		}
		for _, worktree := range m.Worktrees {
			if worktree.Branch == pr.Branch {
				m.statusErr = fmt.Sprintf("Worktree for branch %q already exists at %s", pr.Branch, worktree.Path)
				return m, clearErrorCmd()
			}
		}
		m.activeModal = modal.NewPRCheckoutModal(pr, prWorktreePath(m.RepoPath, pr.Branch))
		return m, nil
	default:
		selected, ok := m.selectedWorktree()
		if !ok {
			return m, nil
		}
		if paneID, label := workflowPaneForWorktree(m.missionState, selected); paneID != "" {
			return m, m.focusPaneCmd(paneID, label)
		}
		useHerdr := m.insideHerdr && m.Config.Herdr.Enabled && m.Config.Herdr.PreferWorktreeAPI
		for _, session := range m.sessions {
			if !pathsEqual(session.WorktreePath, selected.Path) {
				continue
			}
			if useHerdr && session.Runtime != domain.RuntimeHerdr {
				continue
			}
			if session.ShellPID != nil && !pidAlive(*session.ShellPID) {
				break
			}
			return m, m.focusSessionCmd(session)
		}
		if useHerdr {
			return m, m.openHerdrWorktreeCmd(selected.Path)
		}
		return m, m.spawnSessionCmd(selected.Path)
	}
}

func (m *Model) handleContextAction(action string) (tea.Model, tea.Cmd) {
	switch action {
	case modal.ContextActionInspect:
		return m.openSelectedMissionInspector()
	case modal.ContextActionRetryRun:
		return m.retrySelectedWorkflow()
	case modal.ContextActionRemoveRun:
		return m.confirmSelectedWorkflowRemoval()
	case modal.ContextActionOpen:
		return m.activateSelectedItem()
	case modal.ContextActionOpenShell:
		selected, ok := m.selectedWorktree()
		if !ok {
			m.statusErr = "No worktree selected — select one first"
			return m, clearErrorCmd()
		}
		return m, m.spawnSessionCmd(selected.Path)
	case modal.ContextActionClose:
		selected, ok := m.selectedWorktree()
		if !ok {
			m.statusErr = "No worktree selected — select one first"
			return m, clearErrorCmd()
		}
		for _, session := range m.sessions {
			if pathsEqual(session.WorktreePath, selected.Path) {
				return m, m.killSessionCmd(session)
			}
		}

		m.statusErr = "No active session for this worktree"
		return m, clearErrorCmd()
	case modal.ContextActionDelete:
		selected, ok := m.selectedWorktree()
		if !ok {
			m.statusErr = "No worktree selected — select one first"
			return m, clearErrorCmd()
		}
		m.activeModal = modal.NewDeleteModal(selected)
		return m, nil
	case modal.ContextActionOpenGitHub:
		if cmd := m.openInBrowserCmd(); cmd != nil {
			return m, cmd
		}
		m.statusErr = "No GitHub item selected"
		return m, clearErrorCmd()
	default:
		m.statusErr = fmt.Sprintf("Unknown action: %s", action)
		return m, clearErrorCmd()
	}
}

func (m *Model) availableContextActions() []contextActionOption {
	return contextActionsFor(
		m.view,
		m.Worktrees,
		m.selectedIdx,
		m.issues,
		m.selectedIssueIdx,
		m.prs,
		m.selectedPRIdx,
		m.sessions,
		dashboardActionContext{state: m.missionState, tab: m.dashboardTab, selected: m.selectedMissionIdx},
	)
}

type dashboardActionContext struct {
	state    *domain.MissionControlState
	tab      dashboardTab
	selected int
}

func contextActionsFor(view activeView, worktrees []domain.Worktree, worktreeIdx int, issues []domain.Issue, issueIdx int, prs []domain.PullRequest, prIdx int, sessions []domain.Session, dashboard ...dashboardActionContext) []contextActionOption {
	switch view {
	case viewIssues:
		if len(issues) == 0 || issueIdx < 0 || issueIdx >= len(issues) {
			return nil
		}
		return []contextActionOption{
			{icon: "↵", label: "Open or create worktree", action: modal.ContextActionOpen},
			{icon: "⚡", label: "Implement issue", workflowKind: modal.WorkflowKindImplement},
			{icon: "◉", label: "Open on GitHub", action: modal.ContextActionOpenGitHub},
		}
	case viewPRs:
		if len(prs) == 0 || prIdx < 0 || prIdx >= len(prs) {
			return nil
		}
		return []contextActionOption{
			{icon: "↵", label: "Open or checkout worktree", action: modal.ContextActionOpen},
			{icon: "✓", label: "Review pull request", workflowKind: modal.WorkflowKindReview},
			{icon: "◆", label: "Repair CI", workflowKind: modal.WorkflowKindCI},
			{icon: "⇄", label: "Resolve conflicts", workflowKind: modal.WorkflowKindResolve},
			{icon: "◉", label: "Open on GitHub", action: modal.ContextActionOpenGitHub},
		}
	case viewWorktrees:
		if len(worktrees) == 0 || worktreeIdx < 0 || worktreeIdx >= len(worktrees) {
			return nil
		}
		worktree := worktrees[worktreeIdx]
		actions := []contextActionOption{
			{icon: "↵", label: "Jump to workflow or focus worktree", action: modal.ContextActionOpen},
			{icon: "$", label: "Open separate shell", action: modal.ContextActionOpenShell},
		}
		for _, session := range sessions {
			if pathsEqual(session.WorktreePath, worktree.Path) {
				actions = append(actions, contextActionOption{
					icon:   "×",
					label:  "Close session",
					action: modal.ContextActionClose,
				})
				break
			}
		}
		return append(actions,
			contextActionOption{icon: "!", label: "Delete worktree", action: modal.ContextActionDelete},
			contextActionOption{icon: "◇", label: "Clean merged work", workflowKind: modal.WorkflowKindClean},
		)
	default:
		actions := []contextActionOption{
			{icon: "◎", label: "Inspect workflow", action: modal.ContextActionInspect},
		}
		if len(dashboard) > 0 {
			missions := dashboardMissionsForTab(dashboard[0].state, dashboard[0].tab)
			if len(missions) > 0 {
				selected := dashboard[0].selected
				if selected < 0 || selected >= len(missions) {
					selected = 0
				}
				if strings.EqualFold(missions[selected].workflow.Status, domain.WorkflowFailed) {
					actions = append(actions, contextActionOption{icon: "↻", label: "Retry workflow", action: modal.ContextActionRetryRun})
				}
				label := "Remove workflow"
				if isWorkflowActive(missions[selected].workflow) {
					label = "Stop and remove workflow"
				}
				actions = append(actions, contextActionOption{icon: "×", label: label, action: modal.ContextActionRemoveRun})
			}
		}
		return append(actions, contextActionOption{icon: "◇", label: "Clean merged work", workflowKind: modal.WorkflowKindClean})
	}
}

func isWorkflowActive(workflow domain.WorkflowRunRef) bool {
	switch strings.ToLower(workflow.Status) {
	case domain.WorkflowQueued, domain.WorkflowRunning, domain.WorkflowBlocked:
		return true
	default:
		return false
	}
}

func (m *Model) runSelectedContextAction() (tea.Model, tea.Cmd) {
	actions := m.availableContextActions()
	if len(actions) == 0 {
		m.statusErr = "No actions available for the current selection"
		return m, clearErrorCmd()
	}
	if m.contextActionIdx < 0 || m.contextActionIdx >= len(actions) {
		m.contextActionIdx = 0
	}
	selected := actions[m.contextActionIdx]
	if selected.action != "" {
		return m.handleContextAction(selected.action)
	}
	if !m.Config.Sandcastle.Enabled {
		m.statusErr = "Sandcastle workflows are disabled in settings"
		return m, clearErrorCmd()
	}
	if m.workflowStarter == nil {
		m.statusErr = "Grove Sandcastle runtime is unavailable"
		return m, clearErrorCmd()
	}

	request := modal.WorkflowLaunchMsg{Kind: selected.workflowKind}
	if issue, ok := m.selectedIssue(); ok && m.view == viewIssues {
		number := issue.Number
		request.IssueNumber = &number
	}
	if m.view == viewPRs && m.selectedPRIdx >= 0 && m.selectedPRIdx < len(m.prs) {
		number := m.prs[m.selectedPRIdx].Number
		request.PRNumber = &number
	}
	m.statusMsg = fmt.Sprintf("Starting %s workflow…", selected.workflowKind)
	return m, m.startSandcastleWorkflowCmd(request)
}

// sessionForIssue returns the first live session whose worktree branch contains
// "issue-<N>-" for the given issue. Returns nil when no matching live session
// is found.
func (m *Model) sessionForIssue(issue domain.Issue) *domain.Session {
	needle := fmt.Sprintf("issue-%d-", issue.Number)
	for _, wt := range m.Worktrees {
		if !strings.Contains(wt.Branch, needle) {
			continue
		}
		for i, s := range m.sessions {
			if !pathsEqual(s.WorktreePath, wt.Path) {
				continue
			}
			if s.ShellPID != nil && !pidAlive(*s.ShellPID) {
				continue // stale — skip
			}
			return &m.sessions[i]
		}
	}
	return nil
}

// sessionForPR returns the first live session whose worktree branch matches
// the PR's branch. Returns nil when no matching live session is found.
func (m *Model) sessionForPR(pr domain.PullRequest) *domain.Session {
	for _, wt := range m.Worktrees {
		if wt.Branch != pr.Branch {
			continue
		}
		for i, s := range m.sessions {
			if !pathsEqual(s.WorktreePath, wt.Path) {
				continue
			}
			if s.ShellPID != nil && !pidAlive(*s.ShellPID) {
				continue // stale — skip
			}
			return &m.sessions[i]
		}
	}
	return nil
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
// Ctx panel: advances through visible context actions.
// List panel (default): moves the item cursor down.
func (m *Model) moveDown() {
	switch m.focused {
	case panelNav:
		n := int(m.view) + 1
		if n > int(viewPRs) {
			n = int(viewDashboard)
		}
		m.view = activeView(n)
	case panelCtx:
		if actions := m.availableContextActions(); m.contextActionIdx < len(actions)-1 {
			m.contextActionIdx++
		}
	default: // panelList
		switch m.view {
		case viewDashboard:
			if m.selectedMissionIdx < len(dashboardMissionsForTab(m.missionState, m.dashboardTab))-1 {
				m.selectedMissionIdx++
			}
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
// Ctx panel: moves backward through visible context actions.
// List panel (default): moves the item cursor up.
func (m *Model) moveUp() {
	switch m.focused {
	case panelNav:
		n := int(m.view) - 1
		if n < 0 {
			n = int(viewPRs)
		}
		m.view = activeView(n)
	case panelCtx:
		if m.contextActionIdx > 0 {
			m.contextActionIdx--
		}
	default: // panelList
		switch m.view {
		case viewDashboard:
			if m.selectedMissionIdx > 0 {
				m.selectedMissionIdx--
			}
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

// fuzzyConfirmSelection navigates the UI to the selected fuzzy result.
// Returns a Cmd to execute as part of the selection (e.g. spawn a session), or nil.
func (m *Model) fuzzyConfirmSelection() tea.Cmd {
	if len(m.fuzzyResults) == 0 || m.fuzzySelIdx >= len(m.fuzzyResults) {
		return nil
	}
	result := m.fuzzyResults[m.fuzzySelIdx]
	switch result.Kind {
	case domain.KindWorktree:
		m.view = viewWorktrees
		m.ctxScrollOffset = 0
		m.currentPage = 0
		if wt, ok := result.Payload.(domain.Worktree); ok {
			for i, w := range m.Worktrees {
				if w.Path == wt.Path {
					m.selectedIdx = i
					break
				}
			}
		}
	case domain.KindIssue:
		m.view = viewIssues
		m.ctxScrollOffset = 0
		m.currentPage = 0
		if iss, ok := result.Payload.(domain.Issue); ok {
			for i, issue := range m.issues {
				if issue.Number == iss.Number {
					m.selectedIssueIdx = i
					break
				}
			}
		}
	case domain.KindPR:
		m.view = viewPRs
		m.ctxScrollOffset = 0
		m.currentPage = 0
		if pr, ok := result.Payload.(domain.PullRequest); ok {
			for i, p := range m.prs {
				if p.Number == pr.Number {
					m.selectedPRIdx = i
					break
				}
			}
		}
	case domain.KindFile:
		if relPath, ok := result.Payload.(string); ok {
			return openFileInEditorCmd(relPath, m.RepoPath)
		}
	case domain.KindBranch:
		if branch, ok := result.Payload.(string); ok {
			path := prWorktreePath(m.RepoPath, branch)
			m.activeModal = modal.NewBranchCheckoutModal(branch, path)
		}
	case domain.KindCommit:
		if hash, ok := result.Payload.(string); ok {
			return openCommitInBrowserCmd(hash, m.RepoPath)
		}
	case domain.KindAgent:
		// No-op — future: show detail modal.
	}
	return nil
}

// openFuzzyCmd opens the fuzzy finder overlay, triggering a background index build
// when the search index has not yet been populated.
func (m *Model) openFuzzyCmd() tea.Cmd {
	m.fuzzyInput.SetValue("")
	focusCmd := m.fuzzyInput.Focus()
	m.fuzzyActive = true
	m.fuzzySelIdx = 0
	if len(m.fuzzyAllItems) > 0 {
		m.fuzzyLoading = false
		m.fuzzyResults = fuzzy.FilterAndRank("", m.fuzzyAllItems)
		return focusCmd
	}
	m.fuzzyLoading = true
	m.fuzzyResults = nil
	return tea.Batch(focusCmd, func() tea.Msg { return fuzzyOpenMsg{} })
}

// openFileInEditorCmd opens relPath (relative to repoPath) in the user's configured
// $EDITOR. Falls back to "vi" on Unix-like systems and "notepad" on Windows.
func openFileInEditorCmd(relPath, repoPath string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}
	fullPath := filepath.Join(repoPath, relPath)
	cmd := exec.Command(editor, fullPath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return browserOpenErrMsg{err: err} })
}

// openCommitInBrowserCmd opens the given commit SHA in the browser via the gh CLI.
func openCommitInBrowserCmd(hash, repoPath string) tea.Cmd {
	cmd := exec.Command("gh", "browse", hash)
	cmd.Dir = repoPath
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return browserOpenErrMsg{err: err} })
}

// loadCleanupDataCmd detects stale worktrees (branch merged into default) and
// merged branches without an active worktree, then returns a cleanupLoadedMsg
// carrying the candidates for the CleanupModal.
func (m *Model) loadCleanupDataCmd() tea.Cmd {
	repoPath := m.RepoPath
	worktrees := m.Worktrees // snapshot to avoid data race
	return func() tea.Msg {
		git := internalexec.NewGitCommand(repoPath)
		defaultBranch := git.DefaultBranch()

		mergedBranches, err := git.ListMergedBranches(defaultBranch)
		if err != nil {
			return cleanupLoadedMsg{err: err}
		}

		// Index merged branches for O(1) lookup.
		mergedSet := make(map[string]bool, len(mergedBranches))
		for _, b := range mergedBranches {
			mergedSet[b] = true
		}

		// Index branches that already have a worktree.
		worktreeByBranch := make(map[string]bool, len(worktrees))
		for _, wt := range worktrees {
			worktreeByBranch[wt.Branch] = true
		}

		var candidates []modal.CleanupCandidate

		// Stale worktrees: non-main worktrees whose branch is fully merged.
		// worktrees[0] is always the main (primary) worktree — skip it.
		for _, wt := range worktrees[1:] {
			if mergedSet[wt.Branch] {
				candidates = append(candidates, modal.CleanupCandidate{
					Kind:   modal.CandidateWorktree,
					Path:   wt.Path,
					Branch: wt.Branch,
				})
			}
		}

		// Merged branches without a worktree.
		for _, b := range mergedBranches {
			if !worktreeByBranch[b] {
				candidates = append(candidates, modal.CleanupCandidate{
					Kind:   modal.CandidateBranch,
					Branch: b,
				})
			}
		}

		return cleanupLoadedMsg{candidates: candidates}
	}
}

// performCleanupCmd removes the given worktree paths and deletes the given branch
// names, accumulating errors and returning a cleanupDoneMsg with total deleted count.
func (m *Model) performCleanupCmd(worktreePaths []string, branches []string) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		git := internalexec.NewGitCommand(repoPath)
		var errs []error
		deleted := 0

		for _, path := range worktreePaths {
			if err := git.RemoveWorktree(path, true); err != nil {
				errs = append(errs, fmt.Errorf("remove worktree %s: %w", path, err))
			} else {
				deleted++
			}
		}

		for _, branch := range branches {
			if err := git.DeleteBranch(branch, true); err != nil {
				errs = append(errs, fmt.Errorf("delete branch %s: %w", branch, err))
			} else {
				deleted++
			}
		}

		return cleanupDoneMsg{deleted: deleted, err: errors.Join(errs...)}
	}
}
