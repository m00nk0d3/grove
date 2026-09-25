package modal

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/tui/markdown"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

const (
	// missionContentRow is the row the active tab's content starts on: the
	// command header, a blank, the tab row, and another blank come first.
	missionContentRow = missionTabsRow + 2
	// missionFooterRows is the blank line and key hints below the content.
	missionFooterRows = 2
	// reportChromeRows is the report selector, its details, and the rule
	// above the report body.
	reportChromeRows = 3
	// defaultReportRows is the report body height when the screen height is
	// not known.
	defaultReportRows = 20
)

// renderedReport caches a report laid out for one width and theme, so a
// redraw does not lay the whole document out again.
type renderedReport struct {
	path    string
	modTime time.Time
	width   int
	theme   string
	lines   []string
}

// reportRows is how many lines of the report body fit on screen.
func (m *MissionModal) reportRows() int {
	if m.height < 20 {
		return defaultReportRows
	}
	// The fullscreen box takes two border rows, two margin rows, and a title.
	available := m.height - 5 - missionContentRow - missionFooterRows - reportChromeRows
	return max(available, 1)
}

// reportWidth is the width the report body is laid out at, leaving one
// column for the scroll indicator and one for the gap before it.
func (m *MissionModal) reportWidth() int {
	return m.contentWidth() - 2
}

func (m *MissionModal) currentTheme() styles.Theme {
	if m.theme != nil {
		return *m.theme
	}
	return styles.NewTheme("")
}

// reportLines returns the selected report laid out for the current width and
// theme, laying it out again only when one of them or the report changed.
func (m *MissionModal) reportLines() []string {
	if m.selectedReport < 0 || m.selectedReport >= len(m.reports) {
		return nil
	}
	report := m.reports[m.selectedReport]
	width := m.reportWidth()
	theme := m.currentTheme()
	c := m.rendered
	if c.path != report.Path || !c.modTime.Equal(report.ModTime) || c.width != width || c.theme != theme.Name || c.lines == nil {
		m.rendered = renderedReport{
			path:    report.Path,
			modTime: report.ModTime,
			width:   width,
			theme:   theme.Name,
			lines:   strings.Split(markdown.Render(report.Body, width, theme), "\n"),
		}
	}
	return m.rendered.lines
}

func (m *MissionModal) maxReportScroll() int {
	return max(len(m.reportLines())-m.reportRows(), 0)
}

func (m *MissionModal) renderReports() string {
	switch {
	case m.reportsLoading && !m.reportsLoaded:
		return m.mutedStyle().Render("  Loading reports…")
	case m.reportsErr != nil:
		return m.statusStyle("failed").Render("  Could not read reports: "+m.truncate(m.reportsErr.Error())) +
			"\n" + m.mutedStyle().Render("  Press r to try again.")
	case !m.reportsLoaded:
		return m.mutedStyle().Render("  Reports have not been loaded yet. Press r to load them.")
	case !m.reportsSupported:
		kind := firstNonEmpty(m.workflow.Kind, "this")
		return m.mutedStyle().Render(fmt.Sprintf("  A %s workflow does not write reports.", kind)) + "\n" +
			m.mutedStyle().Render("  Implementation, review, and CI workflows do.")
	case len(m.reports) == 0:
		return m.mutedStyle().Render("  No reports yet. They appear here as the workflow's stages finish.") + "\n" +
			m.mutedStyle().Render("  Press r to check again.")
	}

	width := m.contentWidth()
	report := m.reports[m.selectedReport]
	lines := m.reportLines()
	rows := m.reportRows()
	m.scrollOffset = max(0, min(m.scrollOffset, max(len(lines)-rows, 0)))
	start := m.scrollOffset
	end := min(start+rows, len(lines))

	var b strings.Builder
	b.WriteString(m.reportSelector(width))
	b.WriteString("\n")

	details := fmt.Sprintf("%s  •  updated %s  •  %s", filepath.Base(report.Path), formatTimestamp(report.ModTime), formatSize(len(report.Body)))
	position := fmt.Sprintf("%d–%d of %d", start+1, end, len(lines))
	if len(lines) == 0 {
		position = "empty"
	}
	gap := width - lipgloss.Width(details) - lipgloss.Width(position)
	if gap < 2 {
		details = m.truncateTo(details, max(width-lipgloss.Width(position)-2, 8))
		gap = max(width-lipgloss.Width(details)-lipgloss.Width(position), 2)
	}
	b.WriteString(m.mutedStyle().Render(details + strings.Repeat(" ", gap) + position))
	b.WriteString("\n")
	b.WriteString(m.mutedStyle().Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	visible := lines[start:end]
	bar := reportScrollbar(len(visible), len(lines), start, m.currentTheme())
	for i, line := range visible {
		b.WriteString(line)
		if bar != nil {
			b.WriteString(strings.Repeat(" ", max(m.reportWidth()-lipgloss.Width(line), 0)+1))
			b.WriteString(bar[i])
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// reportSelector lists the run's reports with the one on screen highlighted.
func (m *MissionModal) reportSelector(width int) string {
	var b strings.Builder
	b.WriteString(m.heading(fmt.Sprintf("REPORTS  %d", len(m.reports))))
	for i, report := range m.reports {
		b.WriteString("  ")
		label := " " + report.Title + " "
		if i == m.selectedReport {
			b.WriteString(m.selectedStyle().Render(label))
		} else {
			b.WriteString(m.mutedStyle().Render(label))
		}
	}
	if len(m.reports) > 1 {
		b.WriteString(m.mutedStyle().Render("   [ ] switch"))
	}
	return m.truncateStyled(b.String(), width)
}

func (m *MissionModal) truncateStyled(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// reportScrollbar draws a one-column scroll indicator for rows visible lines
// of a total-line report scrolled to start. It returns nil when the whole
// report fits.
func reportScrollbar(rows, total, start int, theme styles.Theme) []string {
	if rows < 1 || total <= rows {
		return nil
	}
	thumb := max(rows*rows/total, 1)
	top := 0
	if maxStart := total - rows; maxStart > 0 {
		top = start * (rows - thumb) / maxStart
	}
	thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent()))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted()))
	bar := make([]string, rows)
	for i := range bar {
		if i >= top && i < top+thumb {
			bar[i] = thumbStyle.Render("┃")
		} else {
			bar[i] = trackStyle.Render("│")
		}
	}
	return bar
}

func formatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}
