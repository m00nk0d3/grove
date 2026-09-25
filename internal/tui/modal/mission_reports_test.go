package modal

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reportsWorkflow() domain.WorkflowRunRef {
	wf := missionTestWorkflow()
	wf.Kind = "imp"
	n := 42
	wf.IssueNumber = &n
	return wf
}

// longReport is a report with more lines than a 40-row screen shows.
func longReport(title, path string) domain.WorkflowReport {
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&b, "Paragraph number %d.\n\n", i)
	}
	return domain.WorkflowReport{Title: title, Path: path, ModTime: time.Now(), Body: b.String()}
}

// openReports switches a new inspector to the reports tab and answers its
// request with msg.
func openReports(t *testing.T, msg MissionReportsLoadedMsg) *MissionModal {
	t.Helper()
	m := NewMissionModal(reportsWorkflow(), nil)
	theme := styles.NewTheme("digital-noir")
	m.SetTheme(theme)
	m.SetWidth(120)
	m.SetHeight(40)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	require.NotNil(t, cmd)
	require.IsType(t, MissionReportsRequestedMsg{}, cmd())
	msg.RunID = m.RunID()
	m.SetReports(msg)
	return m
}

func sendMissionRune(m *MissionModal, r string) tea.Cmd {
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	return cmd
}

func TestMissionModal_EnteringReportsRequestsThem(t *testing.T) {
	m := NewMissionModal(reportsWorkflow(), nil)

	for _, enter := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'5'}},
		{Type: tea.KeyLeft}, // from overview, left wraps to the last tab
	} {
		m.activeTab = missionOverview
		_, cmd := m.Update(enter)

		require.Equal(t, missionReports, m.activeTab)
		require.NotNil(t, cmd)
		msg, ok := cmd().(MissionReportsRequestedMsg)
		require.True(t, ok)
		assert.Equal(t, "run-42", msg.RunID)
		assert.Equal(t, 42, *msg.Workflow.IssueNumber)
	}
	assert.Contains(t, m.View(), "Loading reports")
}

func TestMissionModal_OtherTabsDoNotRequestReports(t *testing.T) {
	m := NewMissionModal(reportsWorkflow(), nil)

	cmd := sendMissionRune(m, "2")

	assert.Equal(t, missionSteps, m.activeTab)
	assert.Nil(t, cmd)
}

func TestMissionModal_RefreshOnReportsAlsoReloadsThem(t *testing.T) {
	m := openReports(t, MissionReportsLoadedMsg{Supported: true})

	cmd := sendMissionRune(m, "r")

	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	var kinds []string
	for _, c := range batch {
		kinds = append(kinds, fmt.Sprintf("%T", c()))
	}
	assert.ElementsMatch(t, []string{"modal.MissionRefreshMsg", "modal.MissionReportsRequestedMsg"}, kinds)
}

func TestMissionModal_ShowsTheSelectedReport(t *testing.T) {
	m := openReports(t, MissionReportsLoadedMsg{Supported: true, Reports: []domain.WorkflowReport{
		{Title: "Implementation report", Path: "/g/agent-flow/issue-42-implementation-report.md", ModTime: time.Now(), Body: "# Implementation Report\n\nAdded **the helper**."},
		{Title: "Review verdict", Path: "/g/agent-flow/issue-42-review-verdict.md", ModTime: time.Now(), Body: "# Review Cycle 1: APPROVED"},
	}})

	view := m.View()
	assert.Contains(t, view, "REPORTS  2")
	assert.Contains(t, view, "issue-42-implementation-report.md")
	assert.Contains(t, view, "Added the helper.", "the report is rendered, not shown as raw Markdown")
	assert.NotContains(t, view, "**the helper**")
	assert.Contains(t, view, "[ ] switch")

	sendMissionRune(m, "]")
	view = m.View()
	assert.Contains(t, view, "Review Cycle 1: APPROVED")
	assert.Contains(t, view, "issue-42-review-verdict.md")

	sendMissionRune(m, "]")
	assert.Equal(t, 0, m.selectedReport, "switching wraps around")
	sendMissionRune(m, "[")
	assert.Equal(t, 1, m.selectedReport)
}

func TestMissionModal_ScrollsALongReport(t *testing.T) {
	m := openReports(t, MissionReportsLoadedMsg{Supported: true, Reports: []domain.WorkflowReport{
		longReport("Implementation Report", "/g/a.md"),
	}})
	require.Greater(t, m.maxReportScroll(), 0)

	assert.Contains(t, m.View(), "Paragraph number 1.")
	assert.NotContains(t, m.View(), "Paragraph number 80.")

	sendMissionRune(m, "j")
	assert.Equal(t, 1, m.scrollOffset)
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	assert.Equal(t, 1+m.reportRows()-2, m.scrollOffset)

	sendMissionRune(m, "G")
	assert.Equal(t, m.maxReportScroll(), m.scrollOffset)
	assert.Contains(t, m.View(), "Paragraph number 80.")
	sendMissionRune(m, "j")
	assert.Equal(t, m.maxReportScroll(), m.scrollOffset, "scrolling stops at the end")

	sendMissionRune(m, "g")
	assert.Equal(t, 0, m.scrollOffset)
	sendMissionRune(m, "k")
	assert.Equal(t, 0, m.scrollOffset, "scrolling stops at the top")
}

func TestMissionModal_ReportsFillExactlyTheScreen(t *testing.T) {
	for _, height := range []int{24, 40, 60} {
		m := openReports(t, MissionReportsLoadedMsg{Supported: true, Reports: []domain.WorkflowReport{
			longReport("Implementation Report", "/g/a.md"),
		}})
		m.SetHeight(height)

		view := m.View()

		// The app gives a fullscreen modal the terminal less five rows: two
		// border rows, two margin rows, and the title.
		assert.Equal(t, height-5, lipgloss.Height(view), "height %d", height)
		for i, line := range strings.Split(view, "\n") {
			assert.LessOrEqual(t, lipgloss.Width(line), 120-10, "height %d line %d", height, i)
		}
	}
}

func TestMissionModal_ReloadKeepsTheReportOnScreen(t *testing.T) {
	first := domain.WorkflowReport{Title: "Implementation report", Path: "/g/impl.md", ModTime: time.Now(), Body: "impl"}
	second := domain.WorkflowReport{Title: "Review verdict", Path: "/g/verdict.md", ModTime: time.Now(), Body: "verdict"}
	m := openReports(t, MissionReportsLoadedMsg{Supported: true, Reports: []domain.WorkflowReport{first, second}})
	sendMissionRune(m, "]")
	require.Equal(t, 1, m.selectedReport)

	m.SetReports(MissionReportsLoadedMsg{RunID: m.RunID(), Supported: true, Reports: []domain.WorkflowReport{first, second}})

	assert.Equal(t, 1, m.selectedReport)
	assert.Contains(t, m.View(), "verdict.md")
}

func TestMissionModal_ReportEmptyStates(t *testing.T) {
	tests := []struct {
		name string
		msg  MissionReportsLoadedMsg
		want string
	}{
		{name: "none written yet", msg: MissionReportsLoadedMsg{Supported: true}, want: "No reports yet"},
		{name: "kind writes none", msg: MissionReportsLoadedMsg{Supported: false}, want: "does not write reports"},
		{name: "read failed", msg: MissionReportsLoadedMsg{Supported: true, Err: errors.New("permission denied")}, want: "permission denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := openReports(t, tt.msg)
			assert.Contains(t, m.View(), tt.want)
		})
	}
}

func TestMissionModal_ReportKeysDoNothingOnOtherTabs(t *testing.T) {
	m := openReports(t, MissionReportsLoadedMsg{Supported: true, Reports: []domain.WorkflowReport{
		longReport("A", "/g/a.md"), longReport("B", "/g/b.md"),
	}})
	sendMissionRune(m, "1")

	sendMissionRune(m, "]")
	sendMissionRune(m, "G")

	assert.Equal(t, 0, m.selectedReport)
	assert.Equal(t, 0, m.scrollOffset)
}

func TestIsOwnMessage(t *testing.T) {
	assert.True(t, IsOwnMessage(clearStatusMsg{}), "a modal's status timer belongs to the modal")
	assert.False(t, IsOwnMessage(tea.WindowSizeMsg{}))
	assert.False(t, IsOwnMessage(MissionReportsLoadedMsg{}), "the app delivers loaded reports itself")
}
