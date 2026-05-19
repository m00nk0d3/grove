package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	libtable "github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/m00nk0d3/nexus/internal/tui/styles"
	"github.com/mattn/go-runewidth"
)

const (
	appVersion           = "1.0"
	footerHintsWorktrees = "[Tab] Panel | [j/k] Navigate | [Enter] Select | [Space] Agents | [t] Settings | [g] GH | [esc] Quit"
	footerHintsPRs       = "[Tab] Panel | [j/k] Navigate | [Enter] Checkout | [t] Settings | [g] GH | [esc] Quit"
	footerHintsDefault   = footerHintsWorktrees
	actionBarHints       = "[c-n] New  [c-d] Delete  [c-l] Lock | [f1] Help"
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

var navItems = []navItem{
	{"W", "WORKTREES"},
	{"I", "ISSUES"},
	{"P", "PRs"},
	{"T", "SETTINGS"},
}

// pathsEqual reports whether two filesystem paths refer to the same location.
// It normalises both paths with filepath.Clean (converting forward slashes to
// OS-native separators on Windows) and compares case-insensitively so that
// mixed-separator paths from the Copilot session-store match paths returned by
// git worktree list on Windows.
func pathsEqual(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
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
func renderFull(worktrees []domain.Worktree, selectedIdx int, repoPath string, themeIdx int, view activeView, termWidth, termHeight int, syncing bool, lastSynced time.Time, syncErr error, issues []domain.Issue, selectedIssueIdx int, prs []domain.PullRequest, selectedPRIdx int, focused focusedPanel, ctxScroll int, currentPage int, sessions []domain.Session) string {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	theme := styles.NewTheme(styles.Themes[themeIdx])

	navOuter := navPanelInner + panelOverhead
	ctxInner := computeCtxInner(termWidth)
	ctxOuter := ctxInner + panelOverhead
	listOuter := termWidth - navOuter - ctxOuter
	if listOuter < minPathWidth+panelOverhead {
		listOuter = minPathWidth + panelOverhead
	}
	listInner := listOuter - panelOverhead
	headerInner := termWidth - headerOverhead

	// panelHeight is the inner content height for all three side panels.
	// 0 means let lipgloss size naturally (used in tests / zero-height terminals).
	panelHeight := 0
	if termHeight > fixedChromeRows {
		panelHeight = termHeight - fixedChromeRows
	}

	header := renderHeader(repoPath, theme, headerInner, countActiveSessions(sessions))
	nav := renderNavRail(theme, panelHeight, view, focused == panelNav)

	// Apply pagination slicing for issue/PR list panels.
	pageStart := currentPage * pageSize
	visibleIssues := issues
	visibleSelectedIssueIdx := selectedIssueIdx
	if len(issues) > pageSize {
		end := pageStart + pageSize
		if end > len(issues) {
			end = len(issues)
		}
		visibleIssues = issues[pageStart:end]
		visibleSelectedIssueIdx = selectedIssueIdx - pageStart
		if visibleSelectedIssueIdx < 0 {
			visibleSelectedIssueIdx = 0
		}
	}
	visiblePRs := prs
	visibleSelectedPRIdx := selectedPRIdx
	if len(prs) > pageSize {
		end := pageStart + pageSize
		if end > len(prs) {
			end = len(prs)
		}
		visiblePRs = prs[pageStart:end]
		visibleSelectedPRIdx = selectedPRIdx - pageStart
		if visibleSelectedPRIdx < 0 {
			visibleSelectedPRIdx = 0
		}
	}

	var list string
	switch view {
	case viewIssues:
		list = renderIssueList(visibleIssues, visibleSelectedIssueIdx, worktrees, theme, listInner, panelHeight, focused == panelList)
	case viewPRs:
		list = renderPRList(visiblePRs, visibleSelectedPRIdx, theme, listInner, panelHeight, focused == panelList)
	default:
		list = renderWorktreePanel(worktrees, selectedIdx, theme, listInner, panelHeight, focused == panelList, sessions)
	}

	ctx := renderContextPanel(view, worktrees, selectedIdx, issues, selectedIssueIdx, prs, selectedPRIdx, theme, panelHeight, ctxScroll, focused == panelCtx, ctxInner, sessions)
	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, nav, list, ctx)
	footer := renderFooterBar(theme, time.Now().UTC().Format("2006-01-02"), termWidth, syncing, lastSynced, syncErr, view, issues, prs, currentPage)
	actionBar := renderActionBar(theme, termWidth)

	return lipgloss.JoinVertical(lipgloss.Left, header, mainRow, footer, actionBar)
}

func renderHeader(repoPath string, theme styles.Theme, innerWidth int, activeSessions int) string {
	if repoPath == "" {
		repoPath = "./"
	}
	text := fmt.Sprintf(
		"NEXUS v%s: GIT WORKTREE ORCHESTRATOR | Repo: %s | Local Path: %s",
		appVersion, filepath.Base(repoPath), repoPath,
	)
	if activeSessions > 0 {
		text += fmt.Sprintf(" | %d active session(s)", activeSessions)
	}
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
	nameW := listInner - fixedTotal
	if nameW < 10 {
		nameW = 10
	}

	// Cap rendered rows and virtual-scroll so selectedIdx is always in the window.
	startIdx := 0
	visible := worktrees
	if panelHeight > 0 {
		maxItems := panelHeight - 1
		if maxItems < 0 {
			maxItems = 0
		}
		if maxItems > 0 && selectedIdx >= maxItems {
			startIdx = selectedIdx - maxItems + 1
		}
		end := startIdx + maxItems
		if end > len(worktrees) {
			end = len(worktrees)
		}
		visible = worktrees[startIdx:end]
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
		Width(listInner).
		StyleFunc(colStyle)

	for _, e := range entries {
		t.Row(e.cursor, e.name, e.path, e.status, e.updated, e.ghid)
	}

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(t.Render())
}

func renderContextPanel(view activeView, worktrees []domain.Worktree, worktreeIdx int, issues []domain.Issue, issueIdx int, prs []domain.PullRequest, prIdx int, theme styles.Theme, panelHeight int, ctxScroll int, focused bool, ctxInner int, sessions []domain.Session) string {
	var content string
	switch view {
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
			statusText := "Open"
			statusDot := "●"
			if issueHasWorktree(iss.Number, worktrees) {
				statusText = "In Progress"
				statusDot = "◉"
			}
			assigneesStr := formatAssignees(iss.Assignees)
			hierarchyStr := buildIssueHierarchyStr(iss, issues, ctxInner)
			content = fmt.Sprintf("Context: Issue #%d\n%s\n\nStatus: %s %s\nAssigned: %s\nLabels: %s%s\n\n%s\n\n[g] Open in GitHub", iss.Number, title, statusDot, statusText, assigneesStr, labels, hierarchyStr, body)
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
			content = fmt.Sprintf("Context: PR #%d\n%s\n\nBranch: %s\nAuthor: @%s\nStatus: %s\nLabels: %s\n\n%s\n\n[g] Open in GitHub", pr.Number, title, branch, author, state, labels, body)
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
					"Context: PR #%d\n%s\n\nGH Title: %s\nAuthor: @%s\nStatus: %s %s\nLabels: %s\n\n%s\n\nAGENT COMMANDS:\n[a] Spawn Claude Code\n[c] Spawn Copilot\n[f] Spawn Aider\n[s] Open Shell in WT",
					pr.Number, titleTrunc, pr.Title, pr.Author, statusDot, pr.State, labels, body,
				)
			} else {
				const pathLabel = "Path: "
				pathTrunc := truncateStr(wt.Path, ctxInner-len(pathLabel))
				prHint := buildPRHint(wt.Branch, issues, worktrees)
				content = fmt.Sprintf(
					"Context: %s\nBranch: %s\nPath: %s%s\n\nAGENT COMMANDS:\n[a] Spawn Claude Code\n[c] Spawn Copilot\n[f] Spawn Aider\n[s] Open Shell in WT",
					filepath.Base(wt.Path), wt.Branch, pathTrunc, prHint,
				)
			}
			content += renderSessionBlock(sess)
		}
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

func renderIssueList(issues []domain.Issue, selectedIdx int, worktrees []domain.Worktree, theme styles.Theme, listInner, panelHeight int, focused bool) string {
	// Fixed column widths. titleColW fills all remaining space (no upper cap).
	const (
		numColW    = 5
		statusColW = 11
		assignColW = 12
		labelsColW = 20
		fixedTotal = numColW + statusColW + assignColW + labelsColW // 48
	)
	titleColW := listInner - fixedTotal
	if titleColW < 10 {
		titleColW = 10
	}

	// Build tree-ordered rows and find the tree index for the selected issue.
	treeRows := buildIssueTree(issues)
	selectedTreeIdx := 0
	for ti, row := range treeRows {
		if row.originalIdx == selectedIdx {
			selectedTreeIdx = ti
			break
		}
	}

	// Cap rendered rows and virtual-scroll so selectedTreeIdx is always in the window.
	// The header row occupies 1 line, so at most panelHeight-1 data rows fit.
	treeStartIdx := 0
	visible := treeRows
	if panelHeight > 0 {
		maxItems := panelHeight - 1
		if maxItems < 0 {
			maxItems = 0
		}
		if maxItems > 0 && selectedTreeIdx >= maxItems {
			treeStartIdx = selectedTreeIdx - maxItems + 1
		}
		end := treeStartIdx + maxItems
		if end > len(treeRows) {
			end = len(treeRows)
		}
		visible = treeRows[treeStartIdx:end]
	}

	// Pre-build cell values and capture status per visible row for use in StyleFunc.
	type rowEntry struct{ num, title, status, assign, labels string }
	entries := make([]rowEntry, len(visible))
	statusValues := make([]string, len(visible))
	for i, row := range visible {
		issue := row.issue
		status := "Open"
		if issueHasWorktree(issue.Number, worktrees) {
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
			status: status,
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
		Width(listInner).
		StyleFunc(colStyle)

	for _, e := range entries {
		t.Row(e.num, e.title, e.status, e.assign, e.labels)
	}

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(t.Render())
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
	prTitleColW := listInner - prFixedTotal
	if prTitleColW < 10 {
		prTitleColW = 10
	}

	// Virtual-scroll so selectedIdx is always in the window.
	prStartIdx := 0
	visible := prs
	if panelHeight > 0 {
		maxItems := panelHeight - 1
		if maxItems < 0 {
			maxItems = 0
		}
		if maxItems > 0 && selectedIdx >= maxItems {
			prStartIdx = selectedIdx - maxItems + 1
		}
		end := prStartIdx + maxItems
		if end > len(prs) {
			end = len(prs)
		}
		visible = prs[prStartIdx:end]
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
		entries[i] = prEntry{
			cursor: cursor,
			num:    fmt.Sprintf("%-6d", pr.Number),
			title:  truncateStr(pr.Title, prTitleColW),
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
		Width(listInner).
		StyleFunc(colStyle)

	for _, e := range entries {
		t.Row(e.cursor, e.num, e.title, e.branch, e.assign, e.status)
	}

	if len(prs) == 0 {
		t.Row("", "", "No open PRs.", "", "", "")
	}

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight > 0 {
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	}
	return st.Render(t.Render())
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

func renderFooterBar(theme styles.Theme, date string, termWidth int, syncing bool, lastSynced time.Time, syncErr error, view activeView, issues []domain.Issue, prs []domain.PullRequest, currentPage int) string {
	hints := footerHintsDefault
	if view == viewPRs {
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

	var pageInfo string
	switch view {
	case viewIssues:
		if len(issues) > pageSize {
			totalPages := (len(issues) + pageSize - 1) / pageSize
			start := currentPage*pageSize + 1
			end := start + pageSize - 1
			if end > len(issues) {
				end = len(issues)
			}
			pageInfo = fmt.Sprintf(" | Page %d/%d (%d-%d of %d issues)", currentPage+1, totalPages, start, end, len(issues))
		}
	case viewPRs:
		if len(prs) > pageSize {
			totalPages := (len(prs) + pageSize - 1) / pageSize
			start := currentPage*pageSize + 1
			end := start + pageSize - 1
			if end > len(prs) {
				end = len(prs)
			}
			pageInfo = fmt.Sprintf(" | Page %d/%d (%d-%d of %d PRs)", currentPage+1, totalPages, start, end, len(prs))
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

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF0000")).
		Padding(0, 1).
		MaxWidth(60).
		Render("✗ " + msg + "\n\nPress any key to dismiss  •  Auto-dismisses in 5s")

	return overlayBottomRight(baseView, box, termWidth)
}

// renderInfoModal renders a floating success/info notification box anchored to the
// bottom-right corner of the terminal viewport, overlaid on top of the base view.
func renderInfoModal(msg string, termWidth, termHeight int, baseView string) string {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	_ = termHeight // reserved for future height-capping logic

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
