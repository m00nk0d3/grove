package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var theme = styles.NewTheme("digital-noir")

// plain renders src and strips the escape sequences, leaving the layout.
func plain(src string, width int) []string {
	return strings.Split(ansi.Strip(Render(src, width, theme)), "\n")
}

func TestRender_Headings(t *testing.T) {
	lines := plain("# Implementation Report\n\n## Overview\n\n### Details", 30)

	assert.Equal(t, []string{
		"Implementation Report",
		strings.Repeat("━", 30),
		"",
		"Overview",
		strings.Repeat("─", 30),
		"",
		"Details",
	}, lines)
}

func TestRender_JoinsAndWrapsParagraphs(t *testing.T) {
	lines := plain("one two three\nfour five six seven", 14)

	assert.Equal(t, []string{"one two three", "four five six", "seven"}, lines)
}

func TestRender_InlineMarkupIsRemovedAndStyled(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	out := Render("**bold** and `code` and *em* and [a link](https://example.com) and 2 * 3", 80, theme)

	assert.Equal(t, "bold and code and em and a link and 2 * 3", ansi.Strip(out))
	assert.Contains(t, out, "\x1b[1;", "bold text is bold")
	assert.Contains(t, out, "\x1b[4;", "link text is underlined")
	assert.NotContains(t, ansi.Strip(out), "example.com", "the link target is dropped")
}

func TestRender_EscapesAndUnclosedMarkers(t *testing.T) {
	assert.Equal(t, []string{`a *literal* star and a ` + "`" + `stray tick`}, plain(`a \*literal\* star and a `+"`"+`stray tick`, 80))
}

func TestRender_Lists(t *testing.T) {
	src := "- first item that wraps onto another line\n  - nested\n- [x] done\n- [ ] todo\n1. numbered"

	lines := plain(src, 24)

	assert.Equal(t, []string{
		"• first item that wraps",
		"  onto another line",
		"  ◦ nested",
		"✓ done",
		"○ todo",
		"1. numbered",
	}, lines)
}

func TestRender_ListContinuationLinesJoinTheItem(t *testing.T) {
	lines := plain("- starts here\n  and continues\n- next", 40)

	assert.Equal(t, []string{"• starts here and continues", "• next"}, lines)
}

func TestRender_BlockQuote(t *testing.T) {
	lines := plain("> **Issue:** org/repo#1 — a title", 40)

	assert.Equal(t, []string{"▌ Issue: org/repo#1 — a title"}, lines)
}

func TestRender_CodeBlockKeepsLinesAndTruncates(t *testing.T) {
	src := "```sql\nSELECT a,   b\nFROM a_table_with_a_very_long_name_indeed\n```\nafter"

	lines := plain(src, 20)

	assert.Equal(t, []string{
		"╭ sql",
		"│ SELECT a,   b",
		"│ FROM a_table_with…",
		"",
		"after",
	}, lines)
}

func TestRender_TableThatFits(t *testing.T) {
	src := "| File | Change |\n|---|---|\n| `a.go` | New |\n| b.go | Edited |"

	lines := plain(src, 40)

	assert.Equal(t, []string{
		"File │ Change",
		"─────┼───────",
		"a.go │ New",
		"b.go │ Edited",
	}, lines)
}

func TestRender_TableWrapsCellsToFit(t *testing.T) {
	src := "| Path | Note |\n|---|---|\n| backend/src/Services/ProjectKpiPresentation.cs | a longer explanation of the change |"

	lines := plain(src, 40)

	require.Greater(t, len(lines), 3, "a row too wide for the screen wraps onto several lines")
	for _, line := range lines {
		assert.LessOrEqual(t, lipgloss.Width(line), 40, "%q", line)
	}
	assert.Contains(t, strings.Join(lines, "\n"), "Path")
}

func TestRender_TableWithTooManyColumnsBecomesRecords(t *testing.T) {
	src := "| A | B | C | D |\n|---|---|---|---|\n| 1 | 2 | 3 | 4 |"

	lines := plain(src, 20)

	assert.Equal(t, []string{"A: 1", "B: 2", "C: 3", "D: 4"}, lines)
}

func TestRender_TableCellsKeepEscapedAndCodePipes(t *testing.T) {
	cells := splitCells(`| a \| b | ` + "`x | y`" + ` | c |`)
	assert.Equal(t, []string{"a | b", "`x | y`", "c"}, cells)
}

func TestRender_RuleAndComments(t *testing.T) {
	lines := plain("above\n\n---\n\n<!-- hidden\nnote -->\nbelow", 10)

	assert.Equal(t, []string{"above", "", strings.Repeat("─", 12), "", "below"}, lines)
}

func TestRender_SplitsAWordWiderThanTheLine(t *testing.T) {
	lines := plain("see abcdefghijklmnopqrstuvwxyz now", 12)

	assert.Equal(t, []string{"see", "abcdefghijkl", "mnopqrstuvwx", "yz now"}, lines)
}

func TestRender_EveryLineFitsTheWidth(t *testing.T) {
	src := strings.Join([]string{
		"# 🚀 A heading long enough that it has to wrap across more than one line",
		"> quoted **text** that is long enough to wrap and keep its gutter on each line",
		"- a list item with `inline code spans` that wrap and a [link](https://example.com)",
		"  - nested item under it with more words than fit",
		"| Column one | Column two | Column three |",
		"|---|---|---|",
		"| " + strings.Repeat("x", 60) + " | short | `code` |",
		"```",
		strings.Repeat("y", 90),
		"```",
		strings.Repeat("word ", 40),
	}, "\n")

	for _, width := range []int{12, 20, 33, 60, 100} {
		for i, line := range strings.Split(Render(src, width, theme), "\n") {
			assert.LessOrEqual(t, lipgloss.Width(line), width, "width %d line %d: %q", width, i, ansi.Strip(line))
		}
	}
}

func TestRender_EmptyInput(t *testing.T) {
	assert.Equal(t, "", Render("", 40, theme))
	assert.Equal(t, "", Render("\n\n  \n", 40, theme))
}
