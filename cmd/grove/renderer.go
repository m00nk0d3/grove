package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	libtable "github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/m00nk0d3/grove/internal/version"
	"github.com/mattn/go-runewidth"
)

const (
	footerHintsWorktrees = "[Tab] Panel | [j/k] Navigate | [Enter] Open | [a] Actions | [t] Settings | [/] Fuzzy | [q/esc]"
	footerHintsIssues    = "[Tab] Panel | [j/k] Navigate | [Enter] Jump/Open | [a] Actions | [r] Sync | [t] Settings | [/] Fuzzy | [q/esc]"
	footerHintsPRs       = "[Tab] Panel | [j/k] Navigate | [Enter] Checkout | [a] Actions | [t] Settings | [/] Fuzzy | [q/esc]"
	footerHintsDefault   = footerHintsWorktrees
	actionBarHints       = "[enter] Open  [a] Focus actions | [f1] Help"
	defaultTermWidth     = 120
	navPanelInner        = 18
	// ctxPanelInner is no longer a constant — use computeCtxInner(termWidth) instead.
	// panelOverhead: 1 border-left + 1 pad-left + 1 pad-right + 1 border-right
	panelOverhead = 4
	// panelPaddingOverhead: lipgloss Width includes padding, so pass Width(inner + panelPaddingOverhead)
	// to get a content area equal to the *Inner variable (Padding(0,1) = 1+1 = 2).
	panelPaddingOverhead = 2
	// headerOverhead: 1 pad-left + 1 pad-right (no border on header/status-bar)
	headerOverhead = 2
	minPathWidth   = 5
	// fixedChromeRows: 1 header + 1 footer + 1 action bar + 2 panel borders (top+bottom)
	fixedChromeRows = 5

	// ctxMinInner / ctxMaxInner bound the dynamic context-panel content width.
	ctxMinInner = 25
	ctxMaxInner = 60
)

type navItem struct {
	key   string
	label string
}

type dashboardMission struct {
	label        string
	workItemID   string
	status       string
	worktreePath string
	paneID       string
	agentCount   int
	workflow     domain.WorkflowRunRef
}

var navItems = []navItem{
	{"D", "DASHBOARD"},
	{"W", "WORKTREES"},
	{"I", "ISSUES"},
	{"P", "PRs"},
	{"T", "SETTINGS"},
}

// sessionForWorktree returns the session whose WorktreePath matches worktreePath, or nil.
func sessionForWorktree(sessions []domain.Session, worktreePath string) *domain.Session {
	for i := range sessions {
		if pathsEqual(sessions[i].WorktreePath, worktreePath) {
			return &sessions[i]
		}
	}
	return nil
}

// sessionBadge returns a compact badge string for the given session state.
// nil or dead → "" (no badge), any live session → "[session open]".
func sessionBadge(s *domain.Session) string {
	if s == nil || s.Status == domain.StatusDead {
		return ""
	}
	return "[session open]"
}

// countActiveSessions returns the number of sessions whose status is not StatusDead.
func countActiveSessions(sessions []domain.Session) int {
	n := 0
	for _, s := range sessions {
		if s.Status != domain.StatusDead {
			n++
		}
	}
	return n
}

// renderSessionBlock formats a SESSION detail block for the context panel.
// Returns an empty string when s is nil.
func renderSessionBlock(s *domain.Session) string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nSESSION\n")
	b.WriteString(fmt.Sprintf("  Status:  %s\n", s.Status))
	if s.ShellPID != nil {
		b.WriteString(fmt.Sprintf("  Shell:   pid %d\n", *s.ShellPID))
	} else {
		b.WriteString("  Shell:   none\n")
	}
	if s.AgentName != nil {
		b.WriteString(fmt.Sprintf("  Agent:   %s\n", *s.AgentName))
	}
	if s.DegradedReason != nil && *s.DegradedReason != "" {
		b.WriteString(fmt.Sprintf("  Degraded: %s\n", *s.DegradedReason))
	}
	if s.Prompt != nil && *s.Prompt != "" {
		b.WriteString(fmt.Sprintf("  Prompt:  %s\n", *s.Prompt))
	}
	mins := int(time.Since(s.StartedAt).Minutes())
	if mins < 1 {
		b.WriteString("  Started: just now")
	} else {
		b.WriteString(fmt.Sprintf("  Started: %dm ago", mins))
	}
	return b.String()
}

// renderFull builds the complete 3-pane TUI layout.
// termWidth is the terminal column count; 0 falls back to defaultTermWidth.
// termHeight is the terminal row count; 0 disables explicit panel height.
func renderFull(worktrees []domain.Worktree, selectedIdx int, repoPath string, themeIdx int, view activeView, termWidth, termHeight int, syncing bool, lastSynced time.Time, syncErr error, issues []domain.Issue, selectedIssueIdx int, prs []domain.PullRequest, selectedPRIdx int, focused focusedPanel, ctxScroll int, sessions []domain.Session, herdrIntegration *domain.ExternalIntegration, sandcastleIntegration *domain.ExternalIntegration, missionState *domain.MissionControlState, dismissed map[string]bool, selections ...int) string {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	theme := styles.NewTheme(styles.Themes[themeIdx])

	ctxInner := computeCtxInner(termWidth)
	listInner := computeListInner(termWidth)
	headerInner := termWidth - headerOverhead

	// panelHeight is the inner content height for all three side panels.
	// 0 means let lipgloss size naturally (used in tests / zero-height terminals).
	panelHeight := 0
	if termHeight > fixedChromeRows {
		panelHeight = termHeight - fixedChromeRows
	}

	herdrStatus := "disconnected"
	if herdrIntegration != nil {
		var available, enabled bool
		if herdrIntegration.Available {
			available = true
		}
		if herdrIntegration.Enabled {
			enabled = true
		}
		if available && enabled {
			herdrStatus = herdrIntegration.Mode
		} else if !available {
			herdrStatus = "unavailable"
		} else if herdrIntegration.Error != "" {
			n := 20
			if len(herdrIntegration.Error) < n {
				n = len(herdrIntegration.Error)
			}
			herdrStatus = "error: " + herdrIntegration.Error[:n]
		} else {
			herdrStatus = "disabled"
		}
	}
	sandcastleStatus := "disconnected"
	if sandcastleIntegration != nil {
		var available, enabled bool
		if sandcastleIntegration.Available {
			available = true
		}
		if sandcastleIntegration.Enabled {
			enabled = true
		}
		if available && enabled {
			sandcastleStatus = sandcastleIntegration.Mode
		} else if !available {
			sandcastleStatus = "unavailable"
		} else if sandcastleIntegration.Error != "" {
			n := 20
			if len(sandcastleIntegration.Error) < n {
				n = len(sandcastleIntegration.Error)
			}
			sandcastleStatus = "error: " + sandcastleIntegration.Error[:n]
		} else {
			sandcastleStatus = "disabled"
		}
	}
	githubStatus := "unknown"
	if missionState != nil {
		var available, enabled bool
		if missionState.Integrations.GitHub.Available {
			available = true
		}
		if missionState.Integrations.GitHub.Enabled {
			enabled = true
		}
		if available && enabled {
			githubStatus = missionState.Integrations.GitHub.Mode
		} else if !available {
			githubStatus = "unavailable"
		} else if missionState.Integrations.GitHub.Error != "" {
			n := 20
			if len(missionState.Integrations.GitHub.Error) < n {
				n = len(missionState.Integrations.GitHub.Error)
			}
			githubStatus = "error: " + missionState.Integrations.GitHub.Error[:n]
		} else {
			githubStatus = "disabled"
		}
	}

	header := renderHeader(repoPath, theme, headerInner, countActiveSessions(sessions), herdrStatus, sandcastleStatus, githubStatus)
	nav := renderNavRail(theme, panelHeight, view, focused == panelNav)

	var list string
	selectedMissionIdx := 0
	if len(selections) > 1 {
		selectedMissionIdx = selections[1]
	}
	selectedDashboardTab := dashboardTabActive
	if len(selections) > 2 {
		selectedDashboardTab = dashboardTab(selections[2])
	}
	switch view {
	case viewDashboard:
		list = renderDashboard(missionState, worktrees, issues, prs, theme, listInner, panelHeight, focused == panelList, sessions, selectedMissionIdx, selectedDashboardTab, dismissed)
	case viewIssues:
		list = renderIssueList(issues, selectedIssueIdx, worktrees, theme, listInner, panelHeight, focused == panelList, missionState)
	case viewPRs:
		list = renderPRList(prs, selectedPRIdx, theme, listInner, panelHeight, focused == panelList)
	default:
		list = renderWorktreePanel(worktrees, selectedIdx, theme, listInner, panelHeight, focused == panelList, sessions)
	}

	actionIdx := 0
	if len(selections) > 0 {
		actionIdx = selections[0]
	}
	actions := withGlobalContextActions(contextActionsFor(
		view,
		worktrees,
		selectedIdx,
		issues,
		selectedIssueIdx,
		prs,
		selectedPRIdx,
		sessions,
		dashboardActionContext{state: missionState, tab: selectedDashboardTab, selected: selectedMissionIdx},
	))
	ctx := renderContextPanel(view, worktrees, selectedIdx, issues, selectedIssueIdx, prs, selectedPRIdx, theme, panelHeight, ctxScroll, focused == panelCtx, ctxInner, sessions, missionState, actions, actionIdx)
	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, nav, list, ctx)
	footer := renderFooterBar(theme, time.Now().UTC().Format("2006-01-02"), termWidth, syncing, lastSynced, syncErr, view, issues, selectedIssueIdx, prs, selectedPRIdx)
	actionBar := renderActionBar(theme, termWidth)

	return lipgloss.JoinVertical(lipgloss.Left, header, mainRow, footer, actionBar)
}

// renderDashboard renders aggregate operations telemetry. Detailed issue, PR,
// and worktree records remain in their dedicated views.
func renderDashboard(missionState *domain.MissionControlState, worktrees []domain.Worktree, issues []domain.Issue, prs []domain.PullRequest, theme styles.Theme, listInner, panelHeight int, focused bool, sessions []domain.Session, selectedMissionIdx int, selectedTab dashboardTab, dismissed map[string]bool) string {
	var workflows []domain.WorkflowRunRef
	var agents []domain.AgentRef
	status := domain.UnknownState
	if missionState != nil {
		workflows = missionState.WorkflowRuns
		agents = missionState.Agents
		if missionState.Status != "" {
			status = missionState.Status
		}
	}

	activeSessions := countActiveSessions(sessions)
	activeAgents := countStatuses(agentStatuses(agents), domain.AgentWorking)
	if activeAgents == 0 {
		for _, session := range sessions {
			if session.Status != domain.StatusDead && session.AgentName != nil {
				activeAgents++
			}
		}
	}
	activeMissions := dashboardMissions(missionState, dismissed)
	completedMissions := completedDashboardMissions(missionState, dismissed)
	activeWorkflows := len(activeMissions)
	attentionCount := countAttentionPRs(prs)
	blocked := countStatuses(workflowStatuses(workflows), domain.WorkflowBlocked, domain.WorkflowFailed) +
		countStatuses(agentStatuses(agents), domain.AgentBlocked, domain.AgentFailed)

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent())).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	success := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success())).Bold(true)
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warning())).Bold(true)

	var b strings.Builder
	b.WriteString(accent.Render("◈ MISSION CONTROL // LIVE OPERATIONS"))
	b.WriteString("\n")
	b.WriteString(muted.Render(fmt.Sprintf(
		"SYSTEM %-10s  %d issues  •  %d PRs  •  %d sessions  •  ",
		strings.ToUpper(status),
		len(issues),
		len(prs),
		activeSessions,
	)))
	if attentionCount > 0 {
		b.WriteString(warning.Render(fmt.Sprintf("%d PRs NEED ATTENTION", attentionCount)))
	} else {
		b.WriteString(success.Render("PR INBOX CLEAR"))
	}
	b.WriteString("\n\n")

	cardWidth := listInner / 4
	if cardWidth < 12 {
		cardWidth = 12
	}
	cards := []string{
		renderDashboardCard(theme, "WORKTREES", len(worktrees), cardWidth),
		renderDashboardCard(theme, "AGENTS", activeAgents, cardWidth),
		renderDashboardCard(theme, "WORKFLOWS", activeWorkflows, cardWidth),
		renderDashboardCard(theme, "OPEN PRs", len(prs), cardWidth),
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	b.WriteString("\n\n")

	b.WriteString(accent.Render("◫ OPERATIONAL PULSE"))
	b.WriteString("\n")
	pulseWidth := listInner - 25
	if pulseWidth < 8 {
		pulseWidth = 8
	}
	b.WriteString(renderPulseRow(theme, "RUNNING", activeWorkflows+activeAgents, maxInt(len(workflows)+len(agents), 1), pulseWidth))
	b.WriteString("\n")
	b.WriteString(renderPulseRow(theme, "BLOCKED", blocked, maxInt(len(workflows)+len(agents), 1), pulseWidth))
	b.WriteString("\n")
	b.WriteString(renderPulseRow(theme, "WORKTREES", activeSessions, maxInt(len(worktrees), 1), pulseWidth))
	b.WriteString("\n\n")

	activeTabStyle := muted
	completedTabStyle := muted
	if selectedTab == dashboardTabCompleted {
		completedTabStyle = accent
	} else {
		activeTabStyle = accent
	}
	b.WriteString(accent.Render("⌁ WORKFLOWS"))
	b.WriteString("  ")
	b.WriteString(activeTabStyle.Render(fmt.Sprintf("[ ACTIVE / ATTENTION %02d ]", len(activeMissions))))
	b.WriteString("  ")
	b.WriteString(completedTabStyle.Render(fmt.Sprintf("[ COMPLETED %02d ]", len(completedMissions))))
	b.WriteString(muted.Render("   [ / ] switch"))
	b.WriteString("\n")
	missions := activeMissions
	if selectedTab == dashboardTabCompleted {
		missions = completedMissions
	}
	if selectedMissionIdx >= len(missions) {
		selectedMissionIdx = max(0, len(missions)-1)
	}
	start, visible := missionWindow(theme, listInner, panelHeight, len(missions), selectedMissionIdx)
	end := start + visible
	// The rows are built apart from the banner so the scroll indicator can run
	// down the list itself rather than the full height of the panel.
	var rows strings.Builder
	listWidth := listInner - scrollbarWidth(len(missions), visible)
	for i := start; i < end; i++ {
		mission := missions[i]
		stateStyle := success
		if strings.EqualFold(mission.status, string(domain.StatusBlocked)) || strings.EqualFold(mission.status, string(domain.StatusFailed)) {
			stateStyle = warning
		}
		target := "no terminal"
		if mission.paneID != "" {
			target = "Herdr " + mission.paneID
		} else if mission.worktreePath != "" {
			target = filepath.Base(mission.worktreePath)
		}
		detail := fmt.Sprintf("%d agent  •  %s", mission.agentCount, target)
		// On the completed tab the run is over, so "0 agent" says nothing and
		// when it finished says a lot.
		if selectedTab == dashboardTabCompleted {
			if finished := missionFinishedAt(mission); !finished.IsZero() {
				detail = fmt.Sprintf("%s  •  %s", detail, formatFinishedAt(finished, time.Now()))
			}
		}
		nameWidth := listWidth - 18
		if nameWidth < 12 {
			nameWidth = 12
		}
		cursor := "  "
		if focused && i == selectedMissionIdx {
			cursor = "> "
		}
		rows.WriteString(fmt.Sprintf("%s%s  %-*s  %s\n",
			cursor,
			stateStyle.Render("●"),
			nameWidth,
			truncateStr(mission.label, nameWidth),
			stateStyle.Render(strings.ToUpper(defaultStatus(mission.status))),
		))
		rows.WriteString(muted.Render(fmt.Sprintf("     %s", detail)))
		rows.WriteString("\n")
	}
	if visible > 0 {
		b.WriteString(attachScrollbar(
			strings.TrimRight(rows.String(), "\n"),
			0, len(missions), visible, start, theme,
		))
		b.WriteString("\n")
	}
	if len(missions) == 0 {
		b.WriteString(muted.Render("  ◌ No active workflows. Start from Issues or PRs."))
		b.WriteString("\n")
	}

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		if len(missions) > 0 {
			b.WriteString("\n")
			b.WriteString(muted.Render("────────────────────────────────"))
			b.WriteString("\n↑↓ navigate  •  [m] dismiss done  •  [x] remove  •  [r] retry")
		}
		// The mission window is already sized to the rows left over after the
		// banner and the hints, so this clip only bites on a panel too short to
		// hold the banner at all, where it keeps the panel's own border rather
		// than letting the overflow push it off screen. The clip that used to
		// stand here sliced by line number using the mission index, so selecting
		// a workflow cut the banner off at an arbitrary point.
		content := clipContent(strings.TrimRight(b.String(), "\n"), 0, panelHeight)
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
		return st.Render(content)
	}

	return st.Render(strings.TrimRight(b.String(), "\n"))
}

func dashboardMissions(state *domain.MissionControlState, dismissed map[string]bool) []dashboardMission {
	return dashboardMissionsMatching(state, isActionableWorkflow, true, dismissed)
}

func completedDashboardMissions(state *domain.MissionControlState, dismissed map[string]bool) []dashboardMission {
	return dashboardMissionsMatching(state, func(workflow domain.WorkflowRunRef) bool {
		if !strings.EqualFold(workflow.Status, domain.WorkflowSucceeded) {
			return false
		}
		if dismissed == nil {
			return false
		}
		runID := workflow.RunID
		if runID == "" {
			runID = workflow.WorkflowID
		}
		return dismissed[runID]
	}, false, nil)
}

func dashboardMissionsForTab(state *domain.MissionControlState, tab dashboardTab, dismissed map[string]bool) []dashboardMission {
	if tab == dashboardTabCompleted {
		return completedDashboardMissions(state, dismissed)
	}
	return dashboardMissions(state, dismissed)
}

func dashboardMissionsMatching(state *domain.MissionControlState, includeWorkflow func(domain.WorkflowRunRef) bool, includeWorkItems bool, dismissed map[string]bool) []dashboardMission {
	if state == nil {
		return nil
	}
	var missions []dashboardMission
	seenRuns := make(map[string]struct{})
	for _, item := range state.WorkItems {
		if includeWorkItems && !isActiveMission(item) {
			continue
		}
		for _, workflow := range item.LinkedWorkflows {
			if !includeWorkflow(workflow) {
				continue
			}
			if isDismissed(workflow, dismissed) {
				continue
			}
			mission := missionFromWorkflow(workflow, item)
			missions = append(missions, mission)
			seenRuns[workflowIdentity(workflow)] = struct{}{}
		}
		if includeWorkItems && len(item.LinkedWorkflows) == 0 {
			missions = append(missions, dashboardMission{
				label:        item.ID,
				workItemID:   item.ID,
				status:       item.Status,
				worktreePath: workItemPath(item),
				paneID:       workItemPane(item),
				agentCount:   len(item.LinkedAgents),
			})
		}
	}
	for _, workflow := range state.WorkflowRuns {
		if !includeWorkflow(workflow) {
			continue
		}
		if isDismissed(workflow, dismissed) {
			continue
		}
		if _, ok := seenRuns[workflowIdentity(workflow)]; ok {
			continue
		}
		missions = append(missions, dashboardMission{
			label:        workflowLabel(workflow),
			status:       workflow.Status,
			worktreePath: workflow.WorktreePath,
			paneID:       paneForWorktree(state.Panes, workflow.WorktreePath),
			workflow:     workflow,
		})
	}
	return missions
}

func isDismissed(workflow domain.WorkflowRunRef, dismissed map[string]bool) bool {
	if dismissed == nil || !strings.EqualFold(workflow.Status, domain.WorkflowSucceeded) {
		return false
	}
	runID := workflow.RunID
	if runID == "" {
		runID = workflow.WorkflowID
	}
	return runID != "" && dismissed[runID]
}

const (
	// missionRowsPerItem: a workflow occupies two rows, its name and the detail
	// line beneath it.
	missionRowsPerItem = 2
	// defaultVisibleMissions is used only when no panel height is known, which
	// is the case in tests and on a zero-height terminal.
	defaultVisibleMissions = 5
	// dashboardHintRows: the blank line, rule and key hints below the list.
	dashboardHintRows = 3
	// listHeaderRows: the column header every table list draws above its rows.
	listHeaderRows = 1
)

// computeListInner reports the inner width of the list panel for a terminal of
// the given width. The click hit-test sizes its windows from the same figure the
// renderer draws with.
func computeListInner(termWidth int) int {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	navOuter := navPanelInner + panelOverhead
	ctxOuter := computeCtxInner(termWidth) + panelOverhead
	listOuter := termWidth - navOuter - ctxOuter
	if listOuter < minPathWidth+panelOverhead {
		listOuter = minPathWidth + panelOverhead
	}
	return listOuter - panelOverhead
}

// listWindow reports which slice of a list is on screen and the index it starts
// at, sized to the space available and always containing the selected item.
//
// available is a budget in terminal rows and rowsPerItem is how many rows one
// item draws. Sizing a window in items against a budget measured in rows is what
// let these lists draw past the bottom of their panel; it also stopped them
// scrolling, because a window too large to fit is a window large enough to hold
// every item, so the offset never moved off zero.
func listWindow(available, rowsPerItem, total, selected int) (start, count int) {
	if rowsPerItem < 1 {
		rowsPerItem = 1
	}
	count = available / rowsPerItem
	if count < 1 {
		count = 1
	}
	if count > total {
		count = total
	}
	if count <= 0 {
		return 0, 0
	}
	if selected >= count {
		start = selected - count + 1
	}
	if start+count > total {
		start = total - count
	}
	if start < 0 {
		start = 0
	}
	return start, count
}

// dashboardBannerRows reports how many rows the dashboard draws above its
// workflow list. Only the card block's height varies, so it is measured; the
// rest is a fixed sequence of lines. The renderer and the click hit-test both
// size the list from this, and TestDashboardBannerRowsMatchesRender pins it
// against what the dashboard actually renders.
func dashboardBannerRows(theme styles.Theme, listInner int) int {
	cardWidth := listInner / 4
	if cardWidth < 12 {
		cardWidth = 12
	}
	// Title, system line and a blank; the cards; a blank; the pulse header, its
	// three rows and a blank; the tab line.
	return 3 + lipgloss.Height(renderDashboardCard(theme, "WORKTREES", 0, cardWidth)) + 7
}

// missionWindow reports which slice of the workflow list the dashboard shows.
// The renderer and the click hit-test both call it, because when they disagreed
// a click landed on a different workflow than the one under the pointer.
func missionWindow(theme styles.Theme, listInner, panelHeight, total, selected int) (start, count int) {
	available := defaultVisibleMissions * missionRowsPerItem
	if panelHeight > 0 {
		available = panelHeight - dashboardBannerRows(theme, listInner) - dashboardHintRows
	}
	return listWindow(available, missionRowsPerItem, total, selected)
}

// scrollbarColumn renders the one-column scroll indicator drawn down the right
// edge of a list: a track whose thumb is as long as the visible share of the
// list and sits as far down as the window does. It returns an empty string when
// the whole list fits, so a short list keeps the full panel width for itself.
func scrollbarColumn(rows, total, visible, start int, theme styles.Theme) string {
	if rows < 1 || visible < 1 || total <= visible {
		return ""
	}
	thumb := rows * visible / total
	if thumb < 1 {
		thumb = 1
	}
	if thumb > rows {
		thumb = rows
	}
	// Spread the offset over the scrollable range, not the whole list, so the
	// last window puts the thumb flush with the bottom of the track.
	top := 0
	if maxStart := total - visible; maxStart > 0 {
		top = start * (rows - thumb) / maxStart
	}
	if top > rows-thumb {
		top = rows - thumb
	}
	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent()))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	lines := make([]string, rows)
	for i := range lines {
		if i >= top && i < top+thumb {
			lines[i] = thumbStyle.Render("█")
		} else {
			lines[i] = trackStyle.Render("│")
		}
	}
	return strings.Join(lines, "\n")
}

// scrollbarWidth reports the columns a list must give up to its scroll
// indicator, so the body is built at the width it will actually be drawn at.
func scrollbarWidth(total, visible int) int {
	if visible >= 1 && total > visible {
		return 1
	}
	return 0
}

// attachScrollbar joins the scroll indicator to the right of a list body,
// holding it clear of the headerRows the body draws above its first item.
func attachScrollbar(body string, headerRows, total, visible, start int, theme styles.Theme) string {
	// The track is measured from the body rather than from the item count, so it
	// stays right for a list whose items are more than one row tall.
	bar := scrollbarColumn(lipgloss.Height(body)-headerRows, total, visible, start, theme)
	if bar == "" {
		return body
	}
	if headerRows > 0 {
		bar = strings.Repeat("\n", headerRows) + bar
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, body, bar)
}

// missionFinishedAt reports when a run stopped. The runtime rewrites the
// workflow's record on every change and once more as it exits, so on a finished
// run the last update is the completion. A run that never reported an update
// falls back to when it started, and one that reported neither is left blank
// rather than shown as the zero date.
func missionFinishedAt(mission dashboardMission) time.Time {
	if !mission.workflow.UpdatedAt.IsZero() {
		return mission.workflow.UpdatedAt
	}
	return mission.workflow.StartedAt
}

// A timestamp is read at a glance or not at all: today's runs want the time,
// this year's want the day, and anything older wants the year.
func formatFinishedAt(finished, now time.Time) string {
	finished = finished.Local()
	now = now.Local()
	switch {
	case sameDay(finished, now):
		return finished.Format("15:04")
	case sameDay(finished, now.AddDate(0, 0, -1)):
		return "yesterday " + finished.Format("15:04")
	case finished.Year() == now.Year():
		return finished.Format("2 Jan 15:04")
	default:
		return finished.Format("2 Jan 2006")
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func missionFromWorkflow(workflow domain.WorkflowRunRef, item domain.WorkItem) dashboardMission {
	paneID := ""
	agentIDs := make(map[string]struct{})
	agentCount := 0
	for _, agent := range item.LinkedAgents {
		if agent.WorkflowRunID == workflow.RunID {
			agentIDs[agent.AgentID] = struct{}{}
			agentCount++
		}
	}
	for _, pane := range item.LinkedPanes {
		if _, ok := agentIDs[pane.AgentID]; ok {
			paneID = pane.PaneID
			break
		}
	}
	if paneID == "" {
		paneID = workItemPane(item)
	}
	path := workflow.WorktreePath
	if path == "" {
		path = workItemPath(item)
	}
	return dashboardMission{
		label:        workflowLabel(workflow),
		workItemID:   item.ID,
		status:       workflow.Status,
		worktreePath: path,
		paneID:       paneID,
		agentCount:   agentCount,
		workflow:     workflow,
	}
}

func workflowLabel(workflow domain.WorkflowRunRef) string {
	if workflow.Title != "" {
		return workflow.Title
	}
	if workflow.WorkflowID != "" {
		return workflow.WorkflowID
	}
	if workflow.RunID != "" {
		return workflow.RunID
	}
	return "workflow"
}

func workflowIdentity(workflow domain.WorkflowRunRef) string {
	if workflow.RunID != "" {
		return workflow.RunID
	}
	return workflow.WorkflowID
}

func workItemPath(item domain.WorkItem) string {
	for _, workflow := range item.LinkedWorkflows {
		if workflow.WorktreePath != "" {
			return workflow.WorktreePath
		}
	}
	for _, pane := range item.LinkedPanes {
		if pane.CWD != "" {
			return pane.CWD
		}
	}
	return ""
}

func workItemPane(item domain.WorkItem) string {
	if item.PaneRef != nil {
		return item.PaneRef.PaneID
	}
	if len(item.LinkedPanes) > 0 {
		return item.LinkedPanes[0].PaneID
	}
	return ""
}

func paneForWorktree(panes []domain.PaneRef, worktreePath string) string {
	if worktreePath == "" {
		return ""
	}
	for _, pane := range panes {
		if pathsEqual(pane.CWD, worktreePath) {
			return pane.PaneID
		}
	}
	return ""
}

func renderDashboardCard(theme styles.Theme, label string, value, width int) string {
	if width < 8 {
		width = 8
	}
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent())).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	return lipgloss.NewStyle().
		Width(width-2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Muted())).
		Padding(0, 1).
		Render(labelStyle.Render(label) + "\n" + valueStyle.Render(fmt.Sprintf("%02d", value)))
}

func renderPulseRow(theme styles.Theme, label string, value, total, width int) string {
	filled := value * width / total
	if value > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	barColor := theme.Success()
	if label == "BLOCKED" && value > 0 {
		barColor = theme.Warning()
	}
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(barColor)).Render(strings.Repeat("━", filled))
	track := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted())).Render(strings.Repeat("─", width-filled))
	return fmt.Sprintf("  %-9s %s%s %02d", label, bar, track, value)
}

func workflowStatuses(workflows []domain.WorkflowRunRef) []string {
	statuses := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		statuses = append(statuses, workflow.Status)
	}
	return statuses
}

func agentStatuses(agents []domain.AgentRef) []string {
	statuses := make([]string, 0, len(agents))
	for _, agent := range agents {
		statuses = append(statuses, agent.Status)
	}
	return statuses
}

func countStatuses(statuses []string, wanted ...string) int {
	count := 0
	for _, status := range statuses {
		for _, candidate := range wanted {
			if strings.EqualFold(status, candidate) {
				count++
				break
			}
		}
	}
	return count
}

func isActiveMission(item domain.WorkItem) bool {
	return item.Degraded ||
		strings.EqualFold(item.Status, string(domain.StatusRunning)) ||
		strings.EqualFold(item.Status, string(domain.StatusBlocked)) ||
		strings.EqualFold(item.Status, string(domain.StatusFailed)) ||
		len(item.LinkedAgents) > 0
}

func isActionableWorkflow(workflow domain.WorkflowRunRef) bool {
	switch strings.ToLower(workflow.Status) {
	case domain.WorkflowQueued, domain.WorkflowRunning, domain.WorkflowBlocked, domain.WorkflowFailed, domain.WorkflowSucceeded:
		return true
	default:
		return false
	}
}

func defaultStatus(status string) string {
	if status == "" {
		return domain.UnknownState
	}
	return status
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderHeader(repoPath string, theme styles.Theme, innerWidth int, activeSessions int, herdrStatus, sandcastleStatus, githubStatus string) string {
	if repoPath == "" {
		repoPath = "./"
	}
	text := fmt.Sprintf(
		"GROVE %s: GIT WORKTREE ORCHESTRATOR | Repo: %s",
		version.Version, filepath.Base(repoPath),
	)
	if activeSessions > 0 {
		text += fmt.Sprintf(" | %d active session(s)", activeSessions)
	}
	text += fmt.Sprintf(" | Herdr: %s | Sandcastle: %s | GitHub: %s", herdrStatus, sandcastleStatus, githubStatus)
	return theme.GetStyle("header").Width(innerWidth).Render(text)
}

func renderNavRail(theme styles.Theme, panelHeight int, view activeView, focused bool) string {
	var b strings.Builder
	for i, item := range navItems {
		cursor := "  "
		if activeView(i) == view {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s: %s\n", cursor, item.key, item.label))
	}
	st := theme.GetStyle("nav-rail").Width(navPanelInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight)
	}
	return st.Render(strings.TrimRight(b.String(), "\n"))
}

func renderWorktreePanel(worktrees []domain.Worktree, selectedIdx int, theme styles.Theme, listInner, panelHeight int, focused bool, sessions []domain.Session) string {
	const (
		cursorW    = 2
		pathW      = 30
		statusW    = 10
		updatedW   = 10
		ghidW      = 6
		fixedTotal = cursorW + pathW + statusW + updatedW + ghidW // 58
	)
	// Virtual-scroll so selectedIdx is always in the window. The header row takes
	// one of the panel's lines.
	startIdx := 0
	visible := worktrees
	if panelHeight > 0 {
		start, count := listWindow(panelHeight-listHeaderRows, 1, len(worktrees), selectedIdx)
		startIdx = start
		visible = worktrees[start : start+count]
	}

	bodyWidth := listInner - scrollbarWidth(len(worktrees), len(visible))
	nameW := bodyWidth - fixedTotal
	if nameW < 10 {
		nameW = 10
	}

	type wtEntry struct {
		cursor, name, path, status, updated, ghid string
		prState                                   string
	}
	entries := make([]wtEntry, len(visible))
	for i, wt := range visible {
		cursor := "  "
		if i+startIdx == selectedIdx {
			cursor = "> "
		}
		ghID := "-"
		var prState string
		if wt.LinkedPR != nil {
			ghID = fmt.Sprintf("#%d", wt.LinkedPR.Number)
			prState = wt.LinkedPR.State
		}
		sha := wt.CommitSHA
		if len(sha) > 8 {
			sha = sha[:8]
		}
		if sha == "" {
			sha = "—"
		}
		entries[i] = wtEntry{
			cursor:  cursor,
			name:    nameWithBadge(filepath.Base(wt.Path), nameW, sessionForWorktree(sessions, wt.Path)),
			path:    truncateStr(wt.Path, pathW),
			status:  worktreeStatus(wt),
			updated: sha,
			ghid:    ghID,
			prState: prState,
		}
	}

	selSt := theme.GetStyle("selected-row")
	normalSt := theme.GetStyle("_")
	surfaceBg := normalSt.GetBackground()
	normalFg := normalSt.GetForeground()

	colStyle := func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		switch col {
		case 0:
			base = lipgloss.NewStyle().Width(cursorW).AlignHorizontal(lipgloss.Left)
		case 1:
			base = lipgloss.NewStyle().Width(nameW).AlignHorizontal(lipgloss.Left)
		case 2:
			base = lipgloss.NewStyle().Width(pathW).AlignHorizontal(lipgloss.Left)
		case 3:
			base = lipgloss.NewStyle().Width(statusW).AlignHorizontal(lipgloss.Right)
		case 4:
			base = lipgloss.NewStyle().Width(updatedW).AlignHorizontal(lipgloss.Right)
		case 5:
			base = lipgloss.NewStyle().Width(ghidW).AlignHorizontal(lipgloss.Right)
		default:
			return lipgloss.NewStyle()
		}
		if row == libtable.HeaderRow {
			return base.Foreground(lipgloss.Color(theme.Muted())).Bold(true)
		}
		if i := row + startIdx; i == selectedIdx {
			return base.
				Background(selSt.GetBackground()).
				Foreground(selSt.GetForeground()).
				Bold(true)
		}
		if col == 3 {
			st := theme.StatusStyle(strings.ToLower(entries[row].status))
			return base.Background(st.GetBackground()).Foreground(st.GetForeground())
		}
		if col == 5 && entries[row].prState != "" {
			return base.Background(surfaceBg).Foreground(prStateColor(entries[row].prState))
		}
		return base.Background(surfaceBg).Foreground(normalFg)
	}

	t := libtable.New().
		Headers("", "NAME", "PATH", "STATUS", "SHA", "PR").
		BorderTop(false).BorderBottom(false).
		BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).BorderRow(false).
		Wrap(false).
		Width(bodyWidth).
		StyleFunc(colStyle)

	for _, e := range entries {
		t.Row(e.cursor, e.name, e.path, e.status, e.updated, e.ghid)
	}

	body := attachScrollbar(t.Render(), listHeaderRows, len(worktrees), len(visible), startIdx, theme)

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(body)
}

func renderContextPanel(view activeView, worktrees []domain.Worktree, worktreeIdx int, issues []domain.Issue, issueIdx int, prs []domain.PullRequest, prIdx int, theme styles.Theme, panelHeight int, ctxScroll int, focused bool, ctxInner int, sessions []domain.Session, missionState *domain.MissionControlState, actions []contextActionOption, actionIdx int) string {
	var content string
	switch view {
	case viewDashboard:
		content = renderDashboardTelemetry(missionState, worktrees, issues, prs, sessions, ctxInner, theme)
	case viewIssues:
		if len(issues) == 0 || issueIdx < 0 || issueIdx >= len(issues) {
			content = "No issue selected.\nPress I to view issues."
		} else {
			iss := issues[issueIdx]
			labelsStr := formatLabels(iss.Labels)
			title := wrapText(iss.Title, ctxInner)
			// "Labels: " prefix = 8 chars; wrap to remaining width to avoid re-wrap.
			labels := wrapText(labelsStr, ctxInner-8)
			body := wrapText(sanitizeBody(strings.ReplaceAll(iss.Body, "\r", "")), ctxInner)
			if body == "" {
				body = "(no description)"
			}
			// The text reports GitHub's view of the issue; the filled dot marks
			// that this machine has a worktree for it.
			statusText := "Open"
			if iss.ProjectStatus != "" {
				statusText = iss.ProjectStatus
			} else if issueHasWorktree(iss.Number, worktrees) {
				statusText = "In Progress"
			}
			statusDot := "●"
			if issueHasWorktree(iss.Number, worktrees) {
				statusDot = "◉"
			}
			assigneesStr := formatAssignees(iss.Assignees)
			hierarchyStr := buildIssueHierarchyStr(iss, issues, ctxInner)
			content = fmt.Sprintf("Context: Issue #%d\n%s\n\nStatus: %s %s\nAssigned: %s\nLabels: %s%s\n\n%s", iss.Number, title, statusDot, statusText, assigneesStr, labels, hierarchyStr, body)
		}
	case viewPRs:
		if len(prs) == 0 || prIdx < 0 || prIdx >= len(prs) {
			content = "No PR selected.\nPress P to view PRs."
		} else {
			pr := prs[prIdx]
			state := pr.State
			if pr.IsDraft {
				state = "DRAFT"
			}
			labelsStr := formatLabels(pr.Labels)
			body := wrapText(sanitizeBody(strings.ReplaceAll(pr.Body, "\r", "")), ctxInner)
			if body == "" {
				body = "(no description)"
			}
			title := wrapText(pr.Title, ctxInner)
			branch := truncateStr(pr.Branch, ctxInner-8) // "Branch: " prefix = 8 chars
			author := truncateStr(pr.Author, ctxInner-9) // "Author: @" prefix = 9 chars
			// "Labels: " prefix = 8 chars; wrap to remaining width to avoid re-wrap.
			labels := wrapText(labelsStr, ctxInner-8)
			attention := "None"
			if reasons := pr.AttentionReasons(); len(reasons) > 0 {
				attention = strings.Join(reasons, " • ")
			}
			review := pr.ReviewDecision
			if review == "" {
				review = "PENDING"
			}
			activity := renderPRActivity(pr, ctxInner)
			content = fmt.Sprintf(
				"Context: PR #%d\n%s\n\nBranch: %s\nAuthor: @%s\nStatus: %s\nReview: %s\nAttention: %s\nLabels: %s\n\n%s%s",
				pr.Number,
				title,
				branch,
				author,
				state,
				review,
				attention,
				labels,
				body,
				activity,
			)
		}
	default: // viewWorktrees
		if len(worktrees) == 0 || worktreeIdx < 0 || worktreeIdx >= len(worktrees) {
			content = "No worktree selected.\nSelect a worktree to\nview context."
		} else {
			wt := worktrees[worktreeIdx]
			sess := sessionForWorktree(sessions, wt.Path)
			if wt.LinkedPR != nil {
				pr := wt.LinkedPR
				// "Labels: " = 8 chars; "Author: @" = 9 chars; "GH Title: " = 10 chars
				labelsStr := formatLabels(pr.Labels)
				labels := wrapText(labelsStr, ctxInner-8)
				titleTrunc := truncateStr(pr.Title, ctxInner-10) // "GH Title: " prefix = 10 chars
				statusDot := lipgloss.NewStyle().Foreground(prStateColor(pr.State)).Render("●")
				body := wrapText(sanitizeBody(strings.ReplaceAll(pr.Body, "\r", "")), ctxInner)
				if body == "" {
					body = "(no description)"
				}
				content = fmt.Sprintf(
					"Context: PR #%d\n%s\n\nGH Title: %s\nAuthor: @%s\nStatus: %s %s\nLabels: %s\n\n%s",
					pr.Number, titleTrunc, pr.Title, pr.Author, statusDot, pr.State, labels, body,
				)
			} else {
				const pathLabel = "Path: "
				pathTrunc := truncateStr(wt.Path, ctxInner-len(pathLabel))
				prHint := buildPRHint(wt.Branch, issues, worktrees)
				content = fmt.Sprintf(
					"Context: %s\nBranch: %s\nPath: %s%s",
					filepath.Base(wt.Path), wt.Branch, pathTrunc, prHint,
				)
			}
			content += renderSessionBlock(sess)
		}
	}
	if len(actions) > 0 {
		content = renderContextActions(theme, actions, actionIdx, focused, ctxInner) + "\n\n" + content
	}
	st := theme.GetStyle("context-panel").Width(ctxInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		content = clipContent(content, ctxScroll, panelHeight)
		// MaxHeight(panelHeight+2): hard-cap the rendered output at panelHeight inner
		// rows + 2 border rows. MaxHeight applies AFTER borders, so this prevents
		// any lipgloss re-wrap from making the panel taller than the terminal allows.
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(content)
}

func renderContextActions(theme styles.Theme, actions []contextActionOption, actionIdx int, focused bool, width int) string {
	accent := lipgloss.Color(theme.Accent())
	muted := lipgloss.Color(theme.Muted())
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("◆ ACTIONS")
	badge := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Bg())).
		Background(accent).
		Bold(true).
		Padding(0, 1)

	var b strings.Builder
	b.WriteString(header)
	if focused {
		b.WriteString("  ")
		b.WriteString(badge.Render("ACTIVE"))
	} else {
		b.WriteString("  ")
		b.WriteString(lipgloss.NewStyle().Foreground(muted).Render("[a] focus"))
	}

	visible := len(actions)
	if !focused && visible > 2 {
		visible = 2
	}
	rowWidth := max(1, width-2)
	for i := 0; i < visible; i++ {
		action := actions[i]
		icon := action.icon
		if icon == "" {
			icon = "•"
		}
		label := truncateStr(action.label, max(1, rowWidth-4))
		row := fmt.Sprintf("%s  %s", icon, label)
		b.WriteByte('\n')
		if focused && i == actionIdx {
			b.WriteString(theme.GetStyle("selected-row").Width(rowWidth).Render("▶ " + row))
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  " + row))
	}
	if !focused && len(actions) > visible {
		b.WriteByte('\n')
		b.WriteString(lipgloss.NewStyle().Foreground(muted).Italic(true).
			Render(fmt.Sprintf("  +%d more actions", len(actions)-visible)))
	}
	if focused {
		b.WriteByte('\n')
		b.WriteString(lipgloss.NewStyle().Foreground(muted).
			Render("↑/↓ select  •  Enter run"))
	}
	return b.String()
}

func renderDashboardTelemetry(missionState *domain.MissionControlState, worktrees []domain.Worktree, issues []domain.Issue, prs []domain.PullRequest, sessions []domain.Session, width int, theme styles.Theme) string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent())).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))

	var integrations domain.IntegrationStatus
	var warnings []domain.Warning
	var workflows []domain.WorkflowRunRef
	var agents []domain.AgentRef
	var panes []domain.PaneRef
	var updatedAt time.Time
	if missionState != nil {
		integrations = missionState.Integrations
		warnings = missionState.Warnings
		workflows = missionState.WorkflowRuns
		agents = missionState.Agents
		panes = missionState.Panes
		updatedAt = missionState.UpdatedAt
	}

	var b strings.Builder
	b.WriteString(accent.Render("◈ SYSTEM TELEMETRY"))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("NETWORK STATUS"))
	b.WriteString("\n")
	b.WriteString(renderIntegrationLine(theme, "HERDR", integrations.Herdr))
	b.WriteString("\n")
	b.WriteString(renderIntegrationLine(theme, "SANDCASTLE", integrations.Sandcastle))
	b.WriteString("\n")
	b.WriteString(renderIntegrationLine(theme, "GITHUB", integrations.GitHub))
	b.WriteString("\n\n")

	b.WriteString(muted.Render("RUNTIME TOPOLOGY"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Worktrees   %02d\n", len(worktrees)))
	b.WriteString(fmt.Sprintf("  Sessions    %02d\n", countActiveSessions(sessions)))
	b.WriteString(fmt.Sprintf("  Agents      %02d\n", len(agents)))
	b.WriteString(fmt.Sprintf("  Workflows   %02d\n", len(workflows)))
	b.WriteString(fmt.Sprintf("  Panes       %02d\n", len(panes)))
	b.WriteString(fmt.Sprintf("  GitHub      %02d issues / %02d PRs\n", len(issues), len(prs)))

	b.WriteString("\n")
	b.WriteString(muted.Render("SIGNAL"))
	b.WriteString("\n")
	if updatedAt.IsZero() {
		b.WriteString("  Awaiting first telemetry frame\n")
	} else {
		b.WriteString(fmt.Sprintf("  Frame age   %s\n", compactAge(updatedAt)))
	}
	if len(warnings) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success())).Render("  ● All monitored systems nominal"))
		b.WriteString("\n")
	} else {
		for i, warning := range warnings {
			if i == 3 {
				b.WriteString(fmt.Sprintf("  +%d more alerts\n", len(warnings)-i))
				break
			}
			message := truncateStr(warning.Message, maxInt(width-4, 10))
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Warning())).Render("  ▲ " + message))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(muted.Render("QUICK ACCESS"))
	b.WriteString("\n  [w] Worktrees\n  [i] Issues\n  [p] Pull requests\n  [/] Global search")
	return b.String()
}

func renderIntegrationLine(theme styles.Theme, label string, integration domain.ExternalIntegration) string {
	state := integration.Mode
	color := theme.Warning()
	symbol := "◌"
	if integration.Available && integration.Enabled {
		color = theme.Success()
		symbol = "●"
	} else if state == "" {
		state = "offline"
	}
	if integration.Error != "" {
		color = theme.Warning()
		symbol = "▲"
	}
	if state == "" {
		state = "connected"
	}
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
	return fmt.Sprintf("  %s %-11s %s", statusStyle.Render(symbol), label, statusStyle.Render(strings.ToUpper(state)))
}

func compactAge(t time.Time) string {
	age := time.Since(t)
	if age < time.Minute {
		return "just now"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(age.Hours()))
}

// issueTreeRow represents a single row in the tree-ordered issue list.
type issueTreeRow struct {
	issue       domain.Issue
	prefix      string // "", "├─ ", or "└─ "
	originalIdx int    // index into the original flat issues slice
}

// buildIssueTree returns issues ordered depth-first: each top-level issue is
// immediately followed by its sub-issues. The originalIdx field refers to the
// position in the input slice so callers can map back to selectedIdx.
func buildIssueTree(issues []domain.Issue) []issueTreeRow {
	// Index issues by number for fast child lookup.
	byNum := make(map[int]int, len(issues)) // number → slice index
	for i, iss := range issues {
		byNum[iss.Number] = i
	}

	// Identify top-level issues (no ParentNumber).
	var rows []issueTreeRow
	emitted := make([]bool, len(issues))

	for i, iss := range issues {
		if iss.ParentNumber != nil {
			continue // will be emitted as a child
		}
		rows = append(rows, issueTreeRow{issue: iss, prefix: "", originalIdx: i})
		emitted[i] = true

		// Emit children in the order they appear in SubIssueNumbers.
		for ci, childNum := range iss.SubIssueNumbers {
			idx, ok := byNum[childNum]
			if !ok {
				continue
			}
			isLast := ci == len(iss.SubIssueNumbers)-1
			pfx := "├─ "
			if isLast {
				pfx = "└─ "
			}
			rows = append(rows, issueTreeRow{issue: issues[idx], prefix: pfx, originalIdx: idx})
			emitted[idx] = true
		}
	}

	// Append any orphaned sub-issues (parent not in the list) at the bottom.
	for i, iss := range issues {
		if !emitted[i] {
			rows = append(rows, issueTreeRow{issue: iss, prefix: "", originalIdx: i})
		}
	}
	return rows
}

func renderIssueList(issues []domain.Issue, selectedIdx int, worktrees []domain.Worktree, theme styles.Theme, listInner, panelHeight int, focused bool, missionStates ...*domain.MissionControlState) string {
	// Fixed column widths. titleColW fills all remaining space (no upper cap).
	const (
		numColW    = 5
		statusColW = 11
		assignColW = 12
		labelsColW = 20
		fixedTotal = numColW + statusColW + assignColW + labelsColW // 48
	)
	// Build tree-ordered rows and find the tree index for the selected issue.
	treeRows := buildIssueTree(issues)
	selectedTreeIdx := 0
	for ti, row := range treeRows {
		if row.originalIdx == selectedIdx {
			selectedTreeIdx = ti
			break
		}
	}

	// Scroll the whole list as one run of items, keeping the selection in view.
	// The header row takes one of the panel's lines.
	treeStartIdx := 0
	visible := treeRows
	if panelHeight > 0 {
		start, count := listWindow(panelHeight-listHeaderRows, 1, len(treeRows), selectedTreeIdx)
		treeStartIdx = start
		visible = treeRows[start : start+count]
	}

	// The table is built at the width it will be drawn at, so the scroll
	// indicator takes its column from the list rather than overflowing the panel.
	bodyWidth := listInner - scrollbarWidth(len(treeRows), len(visible))
	titleColW := bodyWidth - fixedTotal
	if titleColW < 10 {
		titleColW = 10
	}

	// Pre-build cell values and capture status per visible row for use in StyleFunc.
	type rowEntry struct{ num, title, status, assign, labels string }
	entries := make([]rowEntry, len(visible))
	statusValues := make([]string, len(visible))
	for i, row := range visible {
		issue := row.issue
		// Precedence: a live Grove workflow, then the issue's GitHub project
		// board status in the board's own wording, then a local worktree.
		status := "Open"
		if len(missionStates) > 0 {
			status = issueWorkflowStatus(issue.Number, missionStates[0])
		}
		if status == "Open" && issue.ProjectStatus != "" {
			status = issue.ProjectStatus
		}
		if status == "Open" && issueHasWorktree(issue.Number, worktrees) {
			status = "In Progress"
		}
		statusValues[i] = status
		pfxRunes := []rune(row.prefix)
		var titleCell string
		if len(pfxRunes) > 0 {
			titleCell = row.prefix + truncateStr(issue.Title, titleColW-len(pfxRunes))
		} else {
			titleCell = truncateStr(issue.Title, titleColW)
		}
		entries[i] = rowEntry{
			num:    fmt.Sprintf(" %-4d", issue.Number),
			title:  titleCell,
			status: truncateStr(status, statusColW),
			assign: truncateStr(formatAssignees(issue.Assignees), assignColW),
			labels: truncateStr(strings.Join(issue.Labels, " "), labelsColW),
		}
	}

	// Capture styles outside StyleFunc to avoid repeated allocations.
	selSt := theme.GetStyle("selected-row")
	normalSt := theme.GetStyle("_") // unknown key → default: Background(surface).Foreground(fg)
	surfaceBg := normalSt.GetBackground()
	normalFg := normalSt.GetForeground()

	colStyle := func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		switch col {
		case 0:
			base = lipgloss.NewStyle().Width(numColW).AlignHorizontal(lipgloss.Left)
		case 1:
			base = lipgloss.NewStyle().Width(titleColW).AlignHorizontal(lipgloss.Left)
		case 2:
			base = lipgloss.NewStyle().Width(statusColW).AlignHorizontal(lipgloss.Right)
		case 3:
			base = lipgloss.NewStyle().Width(assignColW).AlignHorizontal(lipgloss.Right)
		case 4:
			base = lipgloss.NewStyle().Width(labelsColW).AlignHorizontal(lipgloss.Right)
		default:
			return lipgloss.NewStyle()
		}
		if row == libtable.HeaderRow {
			return base.
				Foreground(lipgloss.Color(theme.Muted())).
				Bold(true)
		}
		if row+treeStartIdx == selectedTreeIdx {
			return base.
				Background(selSt.GetBackground()).
				Foreground(selSt.GetForeground()).
				Bold(true)
		}
		if col == 2 {
			st := theme.StatusStyle(strings.ToLower(statusValues[row]))
			return base.
				Background(st.GetBackground()).
				Foreground(st.GetForeground())
		}
		return base.Background(surfaceBg).Foreground(normalFg)
	}

	t := libtable.New().
		Headers("#", "TITLE", "STATUS", "ASSIGNED", "LABELS").
		BorderTop(false).BorderBottom(false).
		BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).BorderRow(false).
		Wrap(false).
		Width(bodyWidth).
		StyleFunc(colStyle)

	for _, e := range entries {
		t.Row(e.num, e.title, e.status, e.assign, e.labels)
	}

	body := attachScrollbar(t.Render(), listHeaderRows, len(treeRows), len(visible), treeStartIdx, theme)

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(body)
}

func issueWorkflowStatus(issueNumber int, state *domain.MissionControlState) string {
	if state == nil {
		return "Open"
	}
	bestPriority := 0
	status := "Open"
	for _, workflow := range state.WorkflowRuns {
		if workflow.IssueNumber == nil || *workflow.IssueNumber != issueNumber {
			continue
		}
		switch strings.ToLower(workflow.Status) {
		case domain.WorkflowRunning, domain.WorkflowQueued:
			if bestPriority < 4 {
				bestPriority = 4
				status = "In Progress"
			}
		case domain.WorkflowBlocked:
			if bestPriority < 3 {
				bestPriority = 3
				status = "Blocked"
			}
		case domain.WorkflowFailed:
			if bestPriority < 2 {
				bestPriority = 2
				status = "Failed"
			}
		}
	}
	return status
}

func renderPRList(prs []domain.PullRequest, selectedIdx int, theme styles.Theme, listInner, panelHeight int, focused bool) string {
	const (
		prCursorW    = 2
		prNumColW    = 6
		prBranchColW = 20
		prAssignColW = 12
		prStatusColW = 8
		prFixedTotal = prCursorW + prNumColW + prBranchColW + prAssignColW + prStatusColW
	)
	// Virtual-scroll so selectedIdx is always in the window. The header row takes
	// one of the panel's lines.
	prStartIdx := 0
	visible := prs
	if panelHeight > 0 {
		start, count := listWindow(panelHeight-listHeaderRows, 1, len(prs), selectedIdx)
		prStartIdx = start
		visible = prs[start : start+count]
	}

	bodyWidth := listInner - scrollbarWidth(len(prs), len(visible))
	prTitleColW := bodyWidth - prFixedTotal
	if prTitleColW < 10 {
		prTitleColW = 10
	}

	type prEntry struct{ cursor, num, title, branch, assign, status string }
	entries := make([]prEntry, len(visible))
	stateValues := make([]string, len(visible))
	for i, pr := range visible {
		cursor := "  "
		if i+prStartIdx == selectedIdx {
			cursor = "> "
		}
		status := prDisplayStatus(pr)
		stateValues[i] = strings.ToLower(status)
		title := pr.Title
		if pr.NeedsAttention() {
			title = "⚠ " + title
		}
		entries[i] = prEntry{
			cursor: cursor,
			num:    fmt.Sprintf("%-6d", pr.Number),
			title:  truncateStr(title, prTitleColW),
			branch: truncateStr(pr.Branch, prBranchColW),
			assign: truncateStr(strings.Join(pr.Assignees, ","), prAssignColW),
			status: status,
		}
	}

	selSt := theme.GetStyle("selected-row")
	normalSt := theme.GetStyle("_")
	surfaceBg := normalSt.GetBackground()
	normalFg := normalSt.GetForeground()

	colStyle := func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		switch col {
		case 0:
			base = lipgloss.NewStyle().Width(prCursorW).AlignHorizontal(lipgloss.Left)
		case 1:
			base = lipgloss.NewStyle().Width(prNumColW).AlignHorizontal(lipgloss.Left)
		case 2:
			base = lipgloss.NewStyle().Width(prTitleColW).AlignHorizontal(lipgloss.Left)
		case 3:
			base = lipgloss.NewStyle().Width(prBranchColW).AlignHorizontal(lipgloss.Right)
		case 4:
			base = lipgloss.NewStyle().Width(prAssignColW).AlignHorizontal(lipgloss.Right)
		case 5:
			base = lipgloss.NewStyle().Width(prStatusColW).AlignHorizontal(lipgloss.Right)
		default:
			return lipgloss.NewStyle()
		}
		if row == libtable.HeaderRow {
			return base.
				Foreground(lipgloss.Color(theme.Muted())).
				Bold(true)
		}
		if row+prStartIdx == selectedIdx {
			return base.
				Background(selSt.GetBackground()).
				Foreground(selSt.GetForeground()).
				Bold(true)
		}
		if col == 5 {
			st := theme.StatusStyle(stateValues[row])
			return base.
				Background(st.GetBackground()).
				Foreground(st.GetForeground())
		}
		return base.Background(surfaceBg).Foreground(normalFg)
	}

	t := libtable.New().
		Headers("", "#", "TITLE", "BRANCH", "ASSIGNED", "STATUS").
		BorderTop(false).BorderBottom(false).
		BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).BorderRow(false).
		Wrap(false).
		Width(bodyWidth).
		StyleFunc(colStyle)

	for _, e := range entries {
		t.Row(e.cursor, e.num, e.title, e.branch, e.assign, e.status)
	}

	if len(prs) == 0 {
		t.Row("", "", "No open PRs.", "", "", "")
	}

	body := attachScrollbar(t.Render(), listHeaderRows, len(prs), len(visible), prStartIdx, theme)

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(body)
}

// clipContent slices content lines for bounded panel rendering.
// offset skips the first N lines; maxLines caps the visible output.
// If maxLines is 0 the content is returned unchanged.
func clipContent(content string, offset, maxLines int) string {
	if maxLines <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	if offset > 0 {
		if offset >= len(lines) {
			offset = len(lines) - 1
		}
		lines = lines[offset:]
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func renderFooterBar(theme styles.Theme, date string, termWidth int, syncing bool, lastSynced time.Time, syncErr error, view activeView, issues []domain.Issue, selectedIssueIdx int, prs []domain.PullRequest, selectedPRIdx int) string {
	hints := footerHintsDefault
	switch view {
	case viewIssues:
		hints = footerHintsIssues
	case viewPRs:
		hints = footerHintsPRs
	}

	var syncStatus string
	switch {
	case syncErr != nil:
		syncStatus = "✗ sync err"
	case syncing:
		syncStatus = "⟳ syncing"
	case !lastSynced.IsZero():
		mins := int(time.Since(lastSynced).Minutes())
		if mins < 1 {
			syncStatus = "✓ synced just now"
		} else {
			syncStatus = fmt.Sprintf("✓ synced %dm ago", mins)
		}
	}

	// The lists scroll as one continuous run of items, so the position that
	// matters is where the selection sits in the whole list, not which fixed
	// block of fifty it happens to fall in.
	var pageInfo string
	switch view {
	case viewIssues:
		if len(issues) > 0 {
			pageInfo = fmt.Sprintf(" | %d/%d issues", min(selectedIssueIdx+1, len(issues)), len(issues))
		}
	case viewPRs:
		if len(prs) > 0 {
			pageInfo = fmt.Sprintf(" | %d/%d PRs", min(selectedPRIdx+1, len(prs)), len(prs))
		}
	}

	// Build the right side (date + optional sync status) first, then truncate
	// only the hints so the sync status is never clipped on narrow terminals.
	right := fmt.Sprintf("  [%s]", date)
	if syncStatus != "" {
		right += "  " + syncStatus
	}
	maxHints := termWidth - len([]rune(right)) - len([]rune(pageInfo))
	if maxHints < 0 {
		maxHints = 0
	}
	content := truncateStr(hints, maxHints) + pageInfo + right

	return theme.GetStyle("status-bar").Width(termWidth).Render(content)
}

func renderActionBar(theme styles.Theme, termWidth int) string {
	hints := truncateStr(actionBarHints, termWidth)
	return theme.GetStyle("status-bar").Width(termWidth).Render(hints)
}

// computeCtxInner returns the inner content width for the context panel.
// It scales to ~30 % of the terminal width and is clamped to [ctxMinInner, ctxMaxInner].
func computeCtxInner(termWidth int) int {
	inner := termWidth * 30 / 100
	if inner < ctxMinInner {
		return ctxMinInner
	}
	if inner > ctxMaxInner {
		return ctxMaxInner
	}
	return inner
}

// sanitizeBody strips control characters from a PR/issue body that would
// corrupt terminal rendering (e.g. backspace 0x08, form feed 0x0C produced
// by PowerShell backtick escapes when the PR body is created via `gh pr create`
// with double-quoted strings containing markdown code spans).
// Line feeds (0x0A) are preserved; carriage returns are handled separately.
func sanitizeBody(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
		// Drop all other control chars (0x00-0x1F except \n and \t),
		// including \b (0x08, backspace) and \f (0x0C, form feed).
	}
	return b.String()
}

// wrapText word-wraps s to at most width runes per line.
// Existing newlines are preserved; each segment is wrapped independently.
// If width <= 0 the string is returned unchanged.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	for i, seg := range strings.Split(s, "\n") {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(wrapLine(seg, width))
	}
	return out.String()
}

// wrapLine wraps a single newline-free string at word boundaries using display
// cell width (so multi-cell characters like emoji and CJK are measured correctly).
// Falls back to a hard break when a word exceeds width.
func wrapLine(s string, width int) string {
	if runewidth.StringWidth(s) <= width {
		return s
	}
	var out strings.Builder
	runes := []rune(s)
	for {
		// Find the rune index where display cells would exceed width.
		cells, cut := 0, len(runes)
		for i, r := range runes {
			rw := runewidth.RuneWidth(r)
			if cells+rw > width {
				cut = i
				break
			}
			cells += rw
		}
		if cut == len(runes) {
			// All remaining runes fit.
			out.WriteString(string(runes))
			break
		}
		// Prefer a word-boundary break.
		if runes[cut] == ' ' {
			out.WriteString(string(runes[:cut]))
			out.WriteByte('\n')
			runes = runes[cut+1:]
		} else {
			breakAt := -1
			for i := cut - 1; i >= 0; i-- {
				if runes[i] == ' ' {
					breakAt = i
					break
				}
			}
			if breakAt < 0 {
				// No space found — hard break at the cut point.
				out.WriteString(string(runes[:cut]))
				out.WriteByte('\n')
				runes = runes[cut:]
			} else if breakAt == 0 {
				// Segment starts with a leading space (e.g. after a previous break).
				// Skip it silently so we don't emit a spurious blank line.
				runes = runes[1:]
			} else {
				out.WriteString(string(runes[:breakAt]))
				out.WriteByte('\n')
				runes = runes[breakAt+1:]
			}
		}
		if runewidth.StringWidth(string(runes)) <= width {
			out.WriteString(string(runes))
			break
		}
	}
	return out.String()
}

func truncateStr(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

// nameWithBadge returns a name+badge string fitting within maxW runes.
// If the badge fits after the full name, both are shown. Otherwise the name
// is truncated to make room. If maxW is too small for any badge, just the name.
func nameWithBadge(name string, maxW int, s *domain.Session) string {
	badge := sessionBadge(s)
	if badge == "" {
		return truncateStr(name, maxW)
	}
	badgeRunes := len([]rune(badge))
	combined := name + " " + badge
	if len([]rune(combined)) <= maxW {
		return combined
	}
	// Truncate name to leave room for " " + badge.
	nameMax := maxW - badgeRunes - 1
	if nameMax < 2 {
		return truncateStr(name, maxW)
	}
	return truncateStr(name, nameMax) + " " + badge
}

// worktreeStatus maps domain fields to a display status string.
func worktreeStatus(wt domain.Worktree) string {
	if wt.IsLocked {
		return "Locked"
	}
	if wt.IsClean {
		return "Idle"
	}
	return "Dirty"
}

// prStateColor returns the lipgloss color for a given PR state string.
func prStateColor(state string) lipgloss.Color {
	switch state {
	case "OPEN":
		return lipgloss.Color("#00D9FF")
	case "MERGED":
		return lipgloss.Color("#9B59B6")
	case "CLOSED":
		return lipgloss.Color("#E74C3C")
	default:
		return lipgloss.Color("#4A5568")
	}
}

// prDisplayStatus returns the short status label shown in the PR list STATUS column.
// Priority: DRAFT > review decision > raw state.
func prDisplayStatus(pr domain.PullRequest) string {
	if pr.IsDraft {
		return "DRAFT"
	}
	if pr.NeedsAttention() {
		return "ACTION"
	}
	switch pr.ReviewDecision {
	case "APPROVED":
		return "APPROVED"
	case "CHANGES_REQUESTED":
		return "CHANGES"
	case "REVIEW_REQUIRED":
		return "REVIEW"
	}
	return pr.State
}

func countAttentionPRs(prs []domain.PullRequest) int {
	count := 0
	for _, pr := range prs {
		if pr.NeedsAttention() {
			count++
		}
	}
	return count
}

func renderPRActivity(pr domain.PullRequest, width int) string {
	activities := append([]domain.PullRequestActivity(nil), pr.Comments...)
	activities = append(activities, pr.Reviews...)
	if len(activities) == 0 {
		return "\n\nRecent activity: none"
	}
	sort.SliceStable(activities, func(i, j int) bool {
		return activities[i].CreatedAt.After(activities[j].CreatedAt)
	})
	if len(activities) > 3 {
		activities = activities[:3]
	}

	var b strings.Builder
	b.WriteString("\n\nRecent activity:")
	for _, activity := range activities {
		kind := "commented"
		if activity.State != "" {
			kind = strings.ToLower(strings.ReplaceAll(activity.State, "_", " "))
		}
		b.WriteString(fmt.Sprintf("\n@%s %s", activity.Author, kind))
		if activity.Body != "" {
			b.WriteString("\n")
			b.WriteString(wrapText(sanitizeBody(activity.Body), width))
		}
	}
	return b.String()
}

// formatAssignees formats a slice of assignee logins into "@user1,@user2" format.
// Returns "-" when there are no assignees.
func formatAssignees(assignees []string) string {
	if len(assignees) == 0 {
		return "-"
	}
	parts := make([]string, len(assignees))
	for i, a := range assignees {
		parts[i] = "@" + a
	}
	return strings.Join(parts, ",")
}

// issueHasWorktree returns true if any worktree's branch contains "issue-<number>-"
// or ends with "issue-<number>", indicating a worktree was created for this issue.
func issueHasWorktree(issueNumber int, worktrees []domain.Worktree) bool {
	withDash := fmt.Sprintf("issue-%d-", issueNumber)
	atEnd := fmt.Sprintf("issue-%d", issueNumber)
	for _, wt := range worktrees {
		if strings.Contains(wt.Branch, withDash) || strings.HasSuffix(wt.Branch, atEnd) {
			return true
		}
	}
	return false
}

// formatLabels formats a slice of label strings into "[label1][label2]" format.
func formatLabels(labels []string) string {
	strs := make([]string, len(labels))
	for i, l := range labels {
		strs[i] = "[" + l + "]"
	}
	return strings.Join(strs, "")
}

// renderErrorModal renders a floating error notification box anchored to the
// bottom-right corner of the terminal viewport, overlaid on top of the base view
// so the user retains context while the error is displayed.
func renderErrorModal(msg string, termWidth, termHeight int, baseView string) string {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	if termHeight <= 0 {
		termHeight = 24
	}

	maxW := termWidth - 4
	if maxW < 40 {
		maxW = 40
	}
	if maxW > 120 {
		maxW = 120
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF0000")).
		Padding(0, 1).
		MaxWidth(maxW).
		Render("✗ " + msg + "\n\nPress any key to dismiss  •  Auto-dismisses in 5s")

	return overlayBottomRight(baseView, box, termWidth)
}

// renderInfoModal renders a floating success/info notification box anchored to the
// bottom-right corner of the terminal viewport, overlaid on top of the base view.
func renderInfoModal(msg string, termWidth, termHeight int, baseView string) string {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	if termHeight <= 0 {
		termHeight = 24
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FF88")).
		Padding(0, 1).
		MaxWidth(60).
		Render(fmt.Sprintf("✓ %s\n\nAuto-dismisses in %.0fs", msg, msgAutoDismissDuration.Seconds()))

	return overlayBottomRight(baseView, box, termWidth)
}

// overlayBottomRight places the overlay string at the bottom-right corner of the
// base string. ANSI escape codes in both strings are handled correctly via
// charmbracelet/x/ansi. base is assumed to be a multi-line string filling the
// terminal; lines shorter than required are padded with spaces.
func overlayBottomRight(base, overlay string, termWidth int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Visual width of the overlay block.
	ow := 0
	for _, l := range overlayLines {
		if w := lipgloss.Width(l); w > ow {
			ow = w
		}
	}

	// First base line that the overlay occupies.
	startLine := len(baseLines) - len(overlayLines)
	if startLine < 0 {
		startLine = 0
	}

	// Horizontal start column for the overlay (right-aligned).
	startCol := termWidth - ow
	if startCol < 0 {
		startCol = 0
	}

	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, ol := range overlayLines {
		idx := startLine + i
		if idx >= len(result) {
			break
		}
		bl := result[idx]
		bw := lipgloss.Width(bl)
		switch {
		case bw < startCol:
			bl += strings.Repeat(" ", startCol-bw)
		case bw > startCol:
			bl = ansi.Truncate(bl, startCol, "")
		}
		result[idx] = bl + ol
	}

	return strings.Join(result, "\n")
}

// issueRegexp matches the issue number in branch names like "feat/issue-42-something".
var issueRegexp = regexp.MustCompile(`issue-(\d+)`)

// extractIssueNumber parses the issue number from a branch name, returning 0 if not found.
func extractIssueNumber(branch string) int {
	m := issueRegexp.FindStringSubmatch(branch)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// buildIssueHierarchyStr returns a formatted string fragment (starting with "\n") describing
// the parent and sub-issue relationships of the given issue, or "" when none exist.
func buildIssueHierarchyStr(iss domain.Issue, allIssues []domain.Issue, ctxInner int) string {
	var b strings.Builder
	if iss.ParentNumber != nil {
		b.WriteString(fmt.Sprintf("\nParent: #%d", *iss.ParentNumber))
		// Look up parent title if available.
		for _, other := range allIssues {
			if other.Number == *iss.ParentNumber {
				title := truncateStr(other.Title, ctxInner-12)
				b.WriteString(" " + title)
				break
			}
		}
	}
	if len(iss.SubIssueNumbers) > 0 {
		b.WriteString("\nSub-issues:")
		for _, childNum := range iss.SubIssueNumbers {
			line := fmt.Sprintf("\n  #%d", childNum)
			for _, other := range allIssues {
				if other.Number == childNum {
					line += " " + truncateStr(other.Title, ctxInner-8)
					break
				}
			}
			b.WriteString(line)
		}
	}
	return b.String()
}

// buildPRHint returns a PR-target hint for a worktree that has no linked PR,
// when the worktree belongs to a sub-issue whose parent has an open worktree.
// Returns "" when no hint applies.
func buildPRHint(branch string, issues []domain.Issue, worktrees []domain.Worktree) string {
	issNum := extractIssueNumber(branch)
	if issNum == 0 {
		return ""
	}
	// Find the issue entry.
	var theIssue *domain.Issue
	for i := range issues {
		if issues[i].Number == issNum {
			theIssue = &issues[i]
			break
		}
	}
	if theIssue == nil || theIssue.ParentNumber == nil {
		return ""
	}
	parentNum := *theIssue.ParentNumber
	needle := fmt.Sprintf("issue-%d-", parentNum)
	for _, wt := range worktrees {
		if strings.Contains(wt.Branch, needle) {
			return fmt.Sprintf("\n\nPR target: gh pr create --base %s", wt.Branch)
		}
	}
	return ""
}

// kindBadge returns a short display label for the given result kind.
func kindBadge(k domain.ResultKind) string {
	switch k {
	case domain.KindWorktree:
		return "worktree"
	case domain.KindIssue:
		return "issue"
	case domain.KindPR:
		return "pr"
	case domain.KindFile:
		return "file"
	case domain.KindBranch:
		return "branch"
	case domain.KindAgent:
		return "agent"
	case domain.KindCommit:
		return "commit"
	default:
		return string(k)
	}
}

// renderFuzzyOverlay renders the floating fuzzy finder panel.
//
// Layout:
//
//	╭──────────────────────────────────────────╮
//	│  > [input text]                     [/]  │
//	│──────────────────────────────────────────│
//	│  🗂 worktree  feat/issue-42-auth         │
//	│  📌 issue     #42  Fix JWT auth...       │
//	╰──────────────────────────────────────────╯
func renderFuzzyOverlay(input textinput.Model, results []domain.SearchResult, selIdx int, theme styles.Theme, loading bool, termWidth int) string {
	const maxVisible = 12

	overlayWidth := termWidth - 8
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > 100 {
		overlayWidth = 100
	}

	// Inner content width: overlayWidth minus border (2) minus padding (2).
	innerWidth := overlayWidth - 4

	accentColor := lipgloss.Color(theme.Accent())
	mutedColor := lipgloss.Color(theme.Muted())
	fgColor := lipgloss.Color(theme.Fg())
	bgColor := lipgloss.Color(theme.Bg())

	// Header row: "  > [input]  [/]"
	badge := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Render("[/]")
	prompt := lipgloss.NewStyle().
		Foreground(accentColor).
		Render("> ")
	inputView := input.View()
	// Available width for the input field itself (subtract prompt + badge + spaces).
	inputAreaWidth := innerWidth - lipgloss.Width(prompt) - lipgloss.Width(badge) - 2
	if inputAreaWidth < 1 {
		inputAreaWidth = 1
	}
	inputLine := lipgloss.NewStyle().
		Width(innerWidth).
		Render(prompt + truncateStr(inputView, inputAreaWidth) + "  " + badge)

	divider := lipgloss.NewStyle().
		Foreground(mutedColor).
		Render(strings.Repeat("─", innerWidth))

	// Build result rows.
	var rows strings.Builder
	switch {
	case loading:
		rows.WriteString(lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Width(innerWidth).
			Render("Searching..."))
	case len(results) == 0:
		rows.WriteString(lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true).
			Width(innerWidth).
			Render("No results"))
	default:
		visible := results
		if len(visible) > maxVisible {
			visible = visible[:maxVisible]
		}
		for i, r := range visible {
			icon := r.Icon
			badge := lipgloss.NewStyle().
				Foreground(mutedColor).
				Width(8).
				Render(kindBadge(r.Kind))
			label := r.Label
			sub := r.Sub

			// Compute available width for label + sub text.
			iconW := runewidth.StringWidth(icon)
			badgeW := lipgloss.Width(badge)
			// "  " (2) between icon and badge, "  " (2) after badge, "  " (2) between label and sub.
			fixedW := iconW + 2 + badgeW + 2
			remaining := innerWidth - fixedW
			if remaining < 1 {
				remaining = 1
			}
			// Split remaining: 60% label, 40% sub.
			labelW := (remaining * 60) / 100
			subW := remaining - labelW
			if subW < 4 {
				subW = 0
				labelW = remaining
			}

			labelStr := lipgloss.NewStyle().Width(labelW).Render(truncateStr(label, labelW))
			subStr := ""
			if subW > 0 {
				subStr = lipgloss.NewStyle().
					Foreground(mutedColor).
					Width(subW).
					Render(truncateStr(sub, subW))
			}

			line := fmt.Sprintf("%s  %s  %s%s", icon, badge, labelStr, subStr)

			if i == selIdx {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color(theme.Accent())).
					Foreground(bgColor).
					Bold(true).
					Width(innerWidth).
					Render(line)
			} else {
				line = lipgloss.NewStyle().
					Foreground(fgColor).
					Width(innerWidth).
					Render(line)
			}

			if i > 0 {
				rows.WriteString("\n")
			}
			rows.WriteString(line)
		}
		if len(results) > maxVisible {
			more := fmt.Sprintf("…and %d more", len(results)-maxVisible)
			rows.WriteString("\n" + lipgloss.NewStyle().Foreground(mutedColor).Width(innerWidth).Render(more))
		}
	}

	content := inputLine + "\n" + divider + "\n" + rows.String()

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1).
		Width(overlayWidth - 2). // -2: rounded border left+right
		Render(content)
}
