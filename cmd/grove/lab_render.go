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
	labBannerRows  = 3 // tabs, kind filter, spacing
	labHintRows    = 2 // divider and key hints
	labRowsPerItem = 2
)

var labTabShortLabels = [labTabCount]string{"WORKING", "INBOX", "ISSUES", "ARCHIVE"}

var labEmptyMessages = [labTabCount]string{
	"Nothing needs attention and no agents are working.",
	"Nothing captured. Press c to add an idea or report a bug.",
	"No issues created from the Lab yet.",
	"Nothing archived.",
}

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

// renderLabTabs renders the lifecycle tabs with their entry counts, falling
// back to shorter labels when the full ones do not fit.
func renderLabTabs(v labView, theme styles.Theme, width int) string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent())).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	build := func(labels [labTabCount]string, hint bool) string {
		var b strings.Builder
		b.WriteString(accent.Render("⌁ LAB"))
		for tab := labTab(0); tab < labTabCount; tab++ {
			style := muted
			if tab == v.tab {
				style = accent
			}
			b.WriteString("  ")
			b.WriteString(style.Render(fmt.Sprintf("[ %s %02d ]", labels[tab], len(v.visibleIn(tab)))))
		}
		if hint {
			b.WriteString(muted.Render("   [ / ] switch"))
		}
		return b.String()
	}
	for _, candidate := range []string{build(labTabLabels, true), build(labTabLabels, false), build(labTabShortLabels, false)} {
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
	}
	return build(labTabShortLabels, false)
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

// renderLab renders the Lab list: lifecycle tabs, the kind filter, and two
// rows per entry.
func renderLab(v labView, theme styles.Theme, listInner, panelHeight int, focused bool) string {
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	now := time.Now()

	var b strings.Builder
	b.WriteString(renderLabTabs(v, theme, listInner))
	b.WriteString("\n")
	b.WriteString(renderLabFilter(v, theme))
	b.WriteString("\n\n")

	entries := v.visible()
	start, count := 0, len(entries)
	if panelHeight > 0 {
		start, count = listWindow(panelHeight-labBannerRows-labHintRows, labRowsPerItem, len(entries), v.cursor)
	}
	listWidth := listInner - scrollbarWidth(len(entries), count)
	// Badges share one column, as wide as the widest one on screen. A row is
	// cursor (2), marker (1), two gaps (2 + 2), title, and badge.
	badgeWidth := 0
	for i := start; i < start+count; i++ {
		badgeWidth = max(badgeWidth, lipgloss.Width(labStateOf(v, entries[i]).badge))
	}
	titleWidth := max(12, listWidth-badgeWidth-7)
	var rows strings.Builder
	for i := start; i < start+count; i++ {
		e := entries[i]
		state := labStateOf(v, e)
		cursor := "  "
		if focused && i == v.cursor {
			cursor = "> "
		}
		style := state.style(theme)
		rows.WriteString(fmt.Sprintf("%s%s  %-*s  %s\n",
			cursor,
			style.Render(state.marker),
			titleWidth,
			truncateStr(e.Title(), titleWidth),
			style.Render(state.badge),
		))
		rows.WriteString(muted.Render("     " + truncateStr(labDetail(v, e, now), max(1, listWidth-5))))
		rows.WriteString("\n")
	}
	if count > 0 {
		b.WriteString(attachScrollbar(strings.TrimRight(rows.String(), "\n"), 0, len(entries), count, start, theme))
		b.WriteString("\n")
	} else {
		b.WriteString(muted.Render("  ◌ " + labEmptyMessages[v.tab]))
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
	if e, ok := v.selected(); ok {
		primary = "↵ " + labNextAction(v, e)
	}
	b.WriteString(muted.Render(truncateStr(primary+"  •  c new  •  v details  •  a more  •  [ / ] lane", listInner)))
	content := clipContent(b.String(), 0, panelHeight)
	st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	return theme.RenderPanel(st, content)
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
		session += "\nPane: Herdr " + run.paneID + "  (Enter to open)"
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
