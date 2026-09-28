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

var labTabShortLabels = [labTabCount]string{"ACTIVE", "DRAFTS", "PUBLISHED", "ARCHIVED"}

var labEmptyMessages = [labTabCount]string{
	"Nothing in progress. Grill or shape a draft to start.",
	"No drafts. Press c to capture an idea or a bug.",
	"Nothing published yet.",
	"No archived entries.",
}

// labMarker is the state glyph drawn at the start of an entry's row.
func labMarker(e domain.LabEntry) string {
	switch {
	case e.Archived:
		return "▣"
	case e.Status == domain.LabStatusDraft:
		return "○"
	case e.Status == domain.LabStatusPublished:
		return "✓"
	default:
		return "●"
	}
}

// labBadge is the status label drawn at the end of an entry's row.
func labBadge(e domain.LabEntry) string {
	if e.Archived {
		return "ARCHIVED"
	}
	return strings.ToUpper(string(e.Status))
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
	return muted.Render("Kind  ") + strings.Join(parts, muted.Render("  ·  "))
}

// renderLab renders the Lab list: lifecycle tabs, the kind filter, and two
// rows per entry.
func renderLab(v labView, theme styles.Theme, listInner, panelHeight int, focused bool) string {
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	stateStyle := func(e domain.LabEntry) lipgloss.Style {
		switch {
		case e.Archived || e.Status == domain.LabStatusDraft:
			return muted
		default:
			return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Success()))
		}
	}

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
	var rows strings.Builder
	for i := start; i < start+count; i++ {
		e := entries[i]
		badge := labBadge(e)
		titleWidth := max(12, listWidth-lipgloss.Width(badge)-6)
		cursor := "  "
		if focused && i == v.cursor {
			cursor = "> "
		}
		style := stateStyle(e)
		rows.WriteString(fmt.Sprintf("%s%s  %-*s  %s\n",
			cursor,
			style.Render(labMarker(e)),
			titleWidth,
			truncateStr(e.Title(), titleWidth),
			style.Render(badge),
		))
		detail := fmt.Sprintf("%s  •  %s", e.Kind, formatFinishedAt(e.Updated, time.Now()))
		rows.WriteString(muted.Render("     " + truncateStr(detail, max(1, listWidth-5))))
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
	b.WriteString(muted.Render(truncateStr("c capture  •  ↵ next step  •  a actions  •  1/2/3 kind  •  [ / ] tab", listInner)))
	content := clipContent(b.String(), 0, panelHeight)
	st = st.Height(panelHeight).MaxHeight(panelHeight + 2)
	return theme.RenderPanel(st, content)
}

// renderLabContext renders the context panel for a Lab entry.
func renderLabContext(e domain.LabEntry, width int, now time.Time) string {
	status := labBadge(e)
	if e.Archived {
		status = "ARCHIVED (" + strings.ToUpper(string(e.Status)) + ")"
	}
	issues := "—"
	switch {
	case e.Issues.Epic != nil:
		issues = fmt.Sprintf("#%d", *e.Issues.Epic)
		if n := len(e.Issues.Tickets); n > 0 {
			issues += fmt.Sprintf(" + %d tickets", n)
		}
	case e.Issues.Issue != nil:
		issues = fmt.Sprintf("#%d", *e.Issues.Issue)
	}
	body := e.Body()
	if body == "" {
		body = "(no details)"
	}
	return fmt.Sprintf(
		"Context: Lab %s\n%s\n\nStatus: %s\nCaptured: %s\nUpdated: %s\nIssues: %s\n\n%s",
		e.Kind,
		wrapText(e.Title(), width),
		status,
		formatFinishedAt(e.Created, now),
		formatFinishedAt(e.Updated, now),
		issues,
		wrapText(body, width),
	)
}
