package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// Rows the Lab list draws around its entries.
const (
	labBannerRows  = 2 // header and spacing
	labHintRows    = 2 // spacing and key hints
	labRowsPerItem = 2
)

var labEmptyMessage = "Nothing here yet. Press c to capture an idea or report a bug."

// labEmptyArchive explains an empty archive.
const labEmptyArchive = "Nothing archived. Press 0 to go back to the active entries."

// labTone is the color family an entry's state is drawn in.
type labTone int

const (
	labToneMuted labTone = iota
	labToneActive
	labToneDone
	labToneAttention
)

// labState is how an entry's state is drawn: its row marker, status badge,
// and tone. A live run's state takes precedence over the stored status,
// because the run is where session state lives.
type labState struct {
	marker string
	badge  string
	tone   labTone
}

func labStateOf(v labView, e domain.LabEntry) labState {
	if e.Archived {
		return labState{"▣", "ARCHIVED", labToneMuted}
	}
	if run, ok := v.latestRun(e); ok {
		switch strings.ToLower(run.workflow.Status) {
		case domain.WorkflowBlocked:
			return labState{"◆", "WAITING ON YOU", labToneAttention}
		case domain.WorkflowFailed:
			return labState{"✗", "FAILED", labToneAttention}
		}
	}
	// Whatever is listed under NEEDS YOU looks it: a card waiting while the
	// agent works on, or a session that stopped before publishing.
	// An entry whose run Grove has not heard of yet, just started or with
	// Sandcastle unreachable, keeps its status rather than claiming to wait.
	_, runKnown := v.latestRun(e)
	if v.groupOf(e) == labGroupNeedsYou && (runKnown || len(e.Runs) == 0) {
		return labState{"◆", "WAITING ON YOU", labToneAttention}
	}
	if run, ok := v.latestRun(e); ok {
		switch strings.ToLower(run.workflow.Status) {
		case domain.WorkflowQueued, domain.WorkflowRunning:
			return labState{"●", strings.ToUpper(string(e.Status)), labToneActive}
		}
	}
	switch e.Status {
	case domain.LabStatusDraft:
		if e.Kind == domain.LabKindBug {
			return labState{"○", "BUG TO REPORT", labToneMuted}
		}
		return labState{"○", "IDEA TO GRILL", labToneMuted}
	case domain.LabStatusPublished:
		return labState{"✓", "PUBLISHED", labToneDone}
	default:
		return labState{"●", strings.ToUpper(string(e.Status)), labToneActive}
	}
}

func (s labState) style(theme styles.Theme) lipgloss.Style {
	color := theme.Muted()
	switch s.tone {
	case labToneActive:
		color = theme.Accent()
	case labToneDone:
		color = theme.Success()
	case labToneAttention:
		color = theme.Warning()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}

// labStageLabel names the step an entry is on, such as "Interview 1/4". A
// live run's current step is preferred to the stage recorded on the entry.
func labStageLabel(v labView, e domain.LabEntry) string {
	stages := e.LabStages()
	if run, ok := v.latestRun(e); ok && run.live() && run.workflow.CurrentStep != "" {
		for i, stage := range stages {
			if strings.EqualFold(stage, run.workflow.CurrentStep) {
				return fmt.Sprintf("%s %d/%d", stage, i+1, len(stages))
			}
		}
		return run.workflow.CurrentStep
	}
	stage := e.Stage()
	if stage == 0 {
		return ""
	}
	return fmt.Sprintf("%s %d/%d", stages[stage-1], stage, len(stages))
}

// labDetail is the second line of an entry's row: the primary action followed
// by compact workflow metadata.
func labDetail(v labView, e domain.LabEntry, now time.Time) string {
	parts := []string{"↵ " + labNextAction(v, e)}
	if stage := labStageLabel(v, e); stage != "" {
		parts = append(parts, stage)
	}
	if run, ok := v.latestRun(e); ok && run.live() && run.paneID != "" {
		parts = append(parts, "Herdr "+run.paneID)
	}
	parts = append(parts, string(e.Kind), formatFinishedAt(e.Updated, now))
	return strings.Join(parts, "  •  ")
}

// renderLabHeader renders the title, the kind filter, and the archive toggle.
func renderLabHeader(v labView, theme styles.Theme, width int) string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent())).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	title := accent.Render("⌁ LAB")
	if v.archived {
		title += accent.Render(" › ARCHIVE")
	}
	archive := fmt.Sprintf("0 archive (%d)", v.archivedCount())
	if v.archived {
		archive = "0 back to active"
	}
	line := title + "   " + renderLabFilter(v, theme) + muted.Render("  ·  "+archive)
	if lipgloss.Width(line) > width {
		line = title + "   " + renderLabFilter(v, theme)
	}
	return line
}

// renderLabFilter renders the kind filter, highlighting the active one.
func renderLabFilter(v labView, theme styles.Theme) string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent()))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	options := []struct {
		key    string
		label  string
		filter domain.LabFilter
	}{
		{"1", "all", domain.LabFilterAll},
		{"2", "ideas", domain.LabFilterIdea},
		{"3", "bugs", domain.LabFilterBug},
	}
	parts := make([]string, len(options))
	for i, o := range options {
		style := muted
		if o.filter == v.filter {
			style = accent
		}
		parts[i] = style.Render(o.key + " " + o.label)
	}
	return muted.Render("Show  ") + strings.Join(parts, muted.Render("  ·  "))
}

// renderLab renders the Lab list: the header, then the entries grouped by
// what they need, two rows each, under a heading per group.
func renderLab(v labView, theme styles.Theme, listInner, panelHeight int, focused bool) string {
	if v.page != nil {
		st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
		if !focused {
			st = theme.MutedBorder(st)
		}
		content := renderLabPage(v, theme, listInner, panelHeight, focused)
		if panelHeight <= 0 {
			return theme.RenderPanel(st, content)
		}
		st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
		return theme.RenderPanel(st, clipContent(content, 0, panelHeight))
	}
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	now := time.Now()

	var b strings.Builder
	b.WriteString(renderLabHeader(v, theme, listInner))
	b.WriteString("\n\n")

	items := v.items()
	lines, starts := renderLabItems(v, items, theme, listInner, focused, now)
	if len(items) == 0 {
		message := labEmptyMessage
		if v.archived {
			message = labEmptyArchive
		}
		b.WriteString(muted.Render("  ◌ " + message))
		b.WriteString("\n")
	} else {
		start, count := 0, len(lines)
		if panelHeight > 0 {
			start, count = labLineWindow(len(lines), panelHeight-labBannerRows-labHintRows, starts, v.cursor)
		}
		if scrollbarWidth(len(lines), count) > 0 {
			// Draw again one column narrower, leaving room for the scrollbar.
			lines, _ = renderLabItems(v, items, theme, listInner-1, focused, now)
		}
		body := strings.Join(lines[start:start+count], "\n")
		b.WriteString(attachScrollbar(body, 0, len(lines), count, start, theme))
		b.WriteString("\n")
	}

	st := theme.GetStyle("worktree-list").Width(listInner + panelPaddingOverhead)
	if !focused {
		st = theme.MutedBorder(st)
	}
	if panelHeight <= 0 {
		return theme.RenderPanel(st, strings.TrimRight(b.String(), "\n"))
	}
	b.WriteString("\n")
	primary := "↵ capture your first item"
	if it, ok := v.selectedItem(); ok {
		primary = "↵ open"
		if it.more > 0 {
			primary = "↵ show every done entry"
		}
	}
	b.WriteString(muted.Render(truncateStr(primary+"  •  c new  •  1-3 kind  •  0 archive  •  a more", listInner)))
	content := clipContent(b.String(), 0, panelHeight)
	st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	return theme.RenderPanel(st, content)
}

// renderLabItems draws every row of the list and returns the lines with the
// index of each item's first line.
func renderLabItems(v labView, items []labItem, theme styles.Theme, width int, focused bool, now time.Time) ([]string, []int) {
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	heading := func(g labGroup) lipgloss.Style {
		color := theme.Muted()
		switch g {
		case labGroupNeedsYou:
			color = theme.Warning()
		case labGroupWorking:
			color = theme.Accent()
		case labGroupDone:
			color = theme.Success()
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
	}
	// Badges share one column, as wide as the widest one listed. A row is
	// cursor (2), marker (1), two gaps (2 + 2), title, and badge.
	badgeWidth := 0
	for _, it := range items {
		if it.more == 0 {
			badgeWidth = max(badgeWidth, lipgloss.Width(labStateOf(v, it.entry).badge))
		}
	}
	titleWidth := max(12, width-badgeWidth-7)

	var lines []string
	starts := make([]int, len(items))
	last := labGroup(-1)
	for i, it := range items {
		if it.group != last {
			if last >= 0 {
				lines = append(lines, "")
			}
			lines = append(lines, heading(it.group).Render(fmt.Sprintf("%s  %d", labGroupLabels[it.group], v.groupCount(it.group))))
			last = it.group
		}
		starts[i] = len(lines)
		cursor := "  "
		if focused && i == v.cursor {
			cursor = "> "
		}
		if it.more > 0 {
			lines = append(lines, cursor+muted.Render(fmt.Sprintf("… %d more done — Enter to show them", it.more)))
			continue
		}
		e := it.entry
		state := labStateOf(v, e)
		style := state.style(theme)
		lines = append(lines,
			fmt.Sprintf("%s%s  %-*s  %s", cursor, style.Render(state.marker), titleWidth, truncateStr(e.Title(), titleWidth), style.Render(state.badge)),
			muted.Render("     "+truncateStr(labDetail(v, e, now), max(1, width-5))),
		)
	}
	return lines, starts
}

// labLineWindow returns the lines to show of total: as many as fit in
// available, keeping the selected item, and the heading just above it,
// in view.
func labLineWindow(total, available int, starts []int, selected int) (start, count int) {
	if available < 1 {
		available = 1
	}
	if total <= available {
		return 0, total
	}
	if selected < 0 || selected >= len(starts) {
		return 0, available
	}
	first := starts[selected]
	end := first + labRowsPerItem
	if selected+1 < len(starts) && starts[selected+1] < end {
		end = starts[selected+1]
	}
	start = max(0, first-1) // keep the group heading in view when it is right above
	if end-start > available {
		start = first
	}
	if end > start+available {
		start = end - available
	}
	start = min(start, total-available)
	return start, available
}

// labIssuesSummary describes an entry's published issues.
func labIssuesSummary(e domain.LabEntry) string {
	switch {
	case e.Issues.Epic != nil:
		s := fmt.Sprintf("#%d", *e.Issues.Epic)
		if n := len(e.Issues.Tickets); n > 0 {
			s += fmt.Sprintf(" + %d tickets", n)
		}
		return s
	case e.Issues.Issue != nil:
		return fmt.Sprintf("#%d", *e.Issues.Issue)
	default:
		return "—"
	}
}

// renderLabContext renders the context panel for a Lab entry.
func renderLabContext(v labView, e domain.LabEntry, width int, now time.Time) string {
	state := labStateOf(v, e)
	status := state.badge
	if e.Archived {
		status = "ARCHIVED (" + strings.ToUpper(string(e.Status)) + ")"
	}
	var session string
	if stage := labStageLabel(v, e); stage != "" {
		session += "\nStage: " + stage
	}
	if run, ok := v.latestRun(e); ok && run.live() && run.paneID != "" {
		session += "\nPane: Herdr " + run.paneID + "  (View agent in Actions)"
	}
	if owner, ok := v.locks[e.ID]; ok {
		session += fmt.Sprintf("\nLocked: by Grove on %s (pid %d) since %s", owner.Host, owner.PID, formatFinishedAt(owner.Since, now))
	}
	body := e.Body()
	if body == "" {
		body = "(no details)"
	}
	return fmt.Sprintf(
		"Lab %s\n%s\n\nNEXT\n%s\n\nStatus: %s%s\nCaptured: %s\nUpdated: %s\nIssues: %s\n\n%s",
		e.Kind,
		wrapText(e.Title(), width),
		wrapText(labNextAction(v, e), width),
		status,
		session,
		formatFinishedAt(e.Created, now),
		formatFinishedAt(e.Updated, now),
		labIssuesSummary(e),
		wrapText(body, width),
	)
}
