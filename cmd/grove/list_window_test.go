package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testIssues(n int) []domain.Issue {
	issues := make([]domain.Issue, n)
	for i := range issues {
		issues[i] = domain.Issue{Number: i + 1, Title: fmt.Sprintf("issue %d", i+1)}
	}
	return issues
}

func testPRs(n int) []domain.PullRequest {
	prs := make([]domain.PullRequest, n)
	for i := range prs {
		prs[i] = domain.PullRequest{Number: i + 1, Title: fmt.Sprintf("pull request %d", i+1)}
	}
	return prs
}

func testWorktrees(n int) []domain.Worktree {
	worktrees := make([]domain.Worktree, n)
	for i := range worktrees {
		worktrees[i] = domain.Worktree{Path: fmt.Sprintf("/repo/wt-%d", i+1), Branch: fmt.Sprintf("feat/%d", i+1)}
	}
	return worktrees
}

func testWorkflows(n int) *domain.MissionControlState {
	runs := make([]domain.WorkflowRunRef, n)
	for i := range runs {
		runs[i] = domain.WorkflowRunRef{
			WorkflowID: fmt.Sprintf("run-%d", i+1),
			Title:      fmt.Sprintf("workflow %d", i+1),
			Status:     domain.WorkflowRunning,
		}
	}
	return &domain.MissionControlState{WorkflowRuns: runs}
}

func TestListWindow_SizesInRowsNotItems(t *testing.T) {
	// A list of two-row items gets half as many items as there are rows. Sizing
	// the window in items against a budget measured in rows is what let these
	// lists draw past the bottom of the panel.
	start, count := listWindow(10, 2, 20, 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 5, count)

	start, count = listWindow(10, 1, 20, 0)
	assert.Equal(t, 0, start)
	assert.Equal(t, 10, count)
}

func TestListWindow_KeepsSelectionVisible(t *testing.T) {
	const total, rows = 120, 10
	for selected := 0; selected < total; selected++ {
		start, count := listWindow(rows, 1, total, selected)
		assert.GreaterOrEqual(t, selected, start, "selection %d is above the window", selected)
		assert.Less(t, selected, start+count, "selection %d is below the window", selected)
		assert.LessOrEqual(t, start+count, total, "window runs past the end of the list")
	}
}

func TestListWindow_ShortListIsNotPadded(t *testing.T) {
	start, count := listWindow(40, 1, 3, 2)
	assert.Equal(t, 0, start)
	assert.Equal(t, 3, count, "a list shorter than the panel shows every item and no more")
}

func TestListWindow_LastWindowEndsAtTheLastItem(t *testing.T) {
	start, count := listWindow(10, 1, 25, 24)
	assert.Equal(t, 15, start)
	assert.Equal(t, 10, count)
	assert.Equal(t, 25, start+count, "the last window ends flush with the list")
}

// Every list panel is drawn inside a fixed box, so what it renders has to fit
// that box exactly: a list that overflows is clipped, and a window large enough
// to overflow is also large enough to hold the whole list, which is why these
// lists stopped scrolling at the same time as they started drawing past the
// bottom of the panel.
func TestListPanels_FillTheirPanelExactly(t *testing.T) {
	theme := styles.NewTheme(styles.Themes[0])

	// A panel that overflows is truncated to its maximum height, so its height
	// alone proves nothing. What the truncation eats first is the panel's bottom
	// border, so that is what says whether the content fit.
	fits := func(t *testing.T, what string, out string, panelHeight int) {
		t.Helper()
		lines := strings.Split(out, "\n")
		assert.Equal(t, panelHeight+2, len(lines), "%s: panel should be exactly as tall as it was asked to be", what)
		assert.Contains(t, lines[len(lines)-1], "╰", "%s: content overflowed and pushed the bottom border off the panel", what)
	}

	for _, panelHeight := range []int{5, 8, 12, 20, 25, 40} {
		for _, count := range []int{0, 1, 3, 60, 200} {
			where := fmt.Sprintf("%d items in a panel of %d rows", count, panelHeight)
			fits(t, "issues, "+where,
				renderIssueList(testIssues(count), 0, nil, theme, 120, panelHeight, true), panelHeight)
			fits(t, "PRs, "+where,
				renderPRList(testPRs(count), 0, theme, 120, panelHeight, true), panelHeight)
			fits(t, "worktrees, "+where,
				renderWorktreePanel(testWorktrees(count), 0, theme, 120, panelHeight, true, nil), panelHeight)
			fits(t, "dashboard, "+where,
				renderDashboard(testWorkflows(count), nil, nil, nil, theme, 120, panelHeight, true, nil, 0, dashboardTabActive, nil), panelHeight)
		}
	}
}

// dashboardBannerRows is what sizes the workflow list, and the click hit-test
// reads it too, so it has to match the banner the dashboard actually draws. If
// it under-counts, the workflow rows push the key hints off the bottom of the
// panel; if it over-counts, the list gives up rows it could have used.
func TestDashboardBannerRowsMatchesRender(t *testing.T) {
	theme := styles.NewTheme(styles.Themes[0])
	const listInner, panelHeight = 120, 30

	out := renderDashboard(testWorkflows(40), nil, nil, nil, theme, listInner, panelHeight, true, nil, 0, dashboardTabActive, nil)

	_, count := missionWindow(theme, listInner, panelHeight, 40, 0)
	shown := 0
	for i := 1; i <= 40; i++ {
		if strings.Contains(out, fmt.Sprintf("workflow %d ", i)) {
			shown++
		}
	}
	assert.Equal(t, count, shown, "the dashboard should draw exactly the window it sized")
	assert.Contains(t, out, "↑↓ navigate", "the key hints should survive a full workflow list")
}

func TestRenderDashboard_SelectionIsAlwaysInTheWindow(t *testing.T) {
	theme := styles.NewTheme(styles.Themes[0])
	state := testWorkflows(40)
	for _, selected := range []int{0, 1, 9, 20, 39} {
		out := renderDashboard(state, nil, nil, nil, theme, 120, 30, true, nil, selected, dashboardTabActive, nil)
		assert.Contains(t, out, fmt.Sprintf("workflow %d ", selected+1),
			"workflow at index %d should be on screen when it is selected", selected)
	}
}

func TestScrollbarColumn_OnlyWhenThereIsMoreToSee(t *testing.T) {
	theme := styles.NewTheme(styles.Themes[0])
	assert.Empty(t, scrollbarColumn(10, 10, 10, 0, theme), "no indicator when the whole list fits")
	assert.Empty(t, scrollbarColumn(10, 4, 10, 0, theme), "no indicator when the list is shorter than the panel")
	assert.NotEmpty(t, scrollbarColumn(10, 100, 10, 0, theme), "an indicator when there is more list than panel")
	assert.Equal(t, 0, scrollbarWidth(10, 10), "a list that fits keeps the full width")
	assert.Equal(t, 1, scrollbarWidth(100, 10), "a list that scrolls gives up one column")
}

func TestScrollbarColumn_ThumbTracksThePosition(t *testing.T) {
	theme := styles.NewTheme(styles.Themes[0])
	const rows, total, visible = 10, 100, 10

	thumbRows := func(bar string) (first, last int) {
		first, last = -1, -1
		for i, line := range strings.Split(bar, "\n") {
			if strings.Contains(line, "█") {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		return first, last
	}

	top, _ := thumbRows(scrollbarColumn(rows, total, visible, 0, theme))
	assert.Equal(t, 0, top, "at the top of the list the thumb sits at the top of the track")

	_, bottom := thumbRows(scrollbarColumn(rows, total, visible, total-visible, theme))
	assert.Equal(t, rows-1, bottom, "at the end of the list the thumb sits at the bottom of the track")

	middleTop, _ := thumbRows(scrollbarColumn(rows, total, visible, (total-visible)/2, theme))
	assert.Greater(t, middleTop, 0, "halfway down the list the thumb has left the top")
	assert.Less(t, middleTop, rows-1, "halfway down the list the thumb has not reached the bottom")
}

func TestRenderIssueList_ShowsScrollIndicatorOnlyWhenScrollable(t *testing.T) {
	theme := styles.NewTheme(styles.Themes[0])
	assert.Contains(t, renderIssueList(testIssues(200), 0, nil, theme, 120, 20, true), "█",
		"a list longer than the panel shows where it is")
	assert.NotContains(t, renderIssueList(testIssues(3), 0, nil, theme, 120, 20, true), "█",
		"a list that fits shows no indicator")
}

// The renderer and the click hit-test size their windows separately, so a click
// lands on the wrong row the moment the two disagree.
func TestListClickHitTestAgreesWithRenderer(t *testing.T) {
	m := NewModel()
	m.view = viewIssues
	m.width = 160
	m.height = 30
	m.issues = testIssues(200)
	m.selectedIssueIdx = 150

	treeRows := buildIssueTree(m.issues)
	selectedTreeIdx := 0
	for ti, row := range treeRows {
		if row.originalIdx == m.selectedIssueIdx {
			selectedTreeIdx = ti
			break
		}
	}

	rendererStart, rendererCount := listWindow(m.listPanelHeight()-listHeaderRows, 1, len(treeRows), selectedTreeIdx)
	hitTestStart, hitTestCount := m.listRowWindow(len(treeRows), selectedTreeIdx)

	require.Equal(t, rendererStart, hitTestStart, "hit-test and renderer must start at the same row")
	require.Equal(t, rendererCount, hitTestCount, "hit-test and renderer must show the same number of rows")
}

func TestListPageStep_MovesByAScreenful(t *testing.T) {
	m := NewModel()
	m.view = viewIssues
	m.width = 160
	m.height = 30
	m.issues = testIssues(200)

	_, count := m.listRowWindow(len(m.issues), 0)
	assert.Equal(t, count-1, m.listPageStep(len(m.issues)),
		"a page is a screenful less one row of overlap")

	m.nextPage()
	assert.Equal(t, count-1, m.selectedIssueIdx)
	m.prevPage()
	assert.Equal(t, 0, m.selectedIssueIdx)
}
