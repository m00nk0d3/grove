package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPublish(plan LabPublishPlan) *LabPublishPreview {
	m := NewLabPublishPreview(plan)
	m.SetWidth(140)
	m.SetHeight(40)
	m.SetTheme(styles.NewTheme(styles.Themes[0]))
	return m
}

func publishKey(m *LabPublishPreview, key string) tea.Msg {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	}
	_, cmd := m.Update(msg)
	if cmd == nil {
		return nil
	}
	return cmd()
}

func basePlan() LabPublishPlan {
	return LabPublishPlan{
		EntryID: "e1",
		Repo:    "m00nk0d3/grove",
		Title:   "Sync stalls on token expiry",
		Body:    "## Summary\n\nThe dashboard freezes.",
		Labels:  []string{"bug"},
		Boards:  []LabPublishBoard{{Ref: "m00nk0d3/3", Title: "Roadmap"}},
		Board:   0,
	}
}

func TestLabPublish_PreviewShowsEverythingThatWillBeCreated(t *testing.T) {
	view := newTestPublish(basePlan()).View()
	for _, want := range []string{"m00nk0d3/grove", "bug", "Roadmap (m00nk0d3/3), status Backlog", "Sync stalls on token expiry", "The dashboard freezes.", "Nothing is sent to GitHub until you confirm"} {
		assert.Contains(t, view, want)
	}
	assert.NotContains(t, view, "←/→", "a single board is not a choice")
}

func TestLabPublish_ConfirmAndCancel(t *testing.T) {
	m := newTestPublish(basePlan())
	assert.Equal(t, LabPublishConfirmedMsg{EntryID: "e1", BoardRef: "m00nk0d3/3"}, publishKey(m, "y"))
	assert.IsType(t, ModalCancelledMsg{}, publishKey(m, "n"))
	assert.IsType(t, ModalCancelledMsg{}, publishKey(m, "esc"))
	assert.Nil(t, publishKey(m, "p"), "other keys do nothing")
}

func TestLabPublish_ChoosingAmongBoardsIsRemembered(t *testing.T) {
	plan := basePlan()
	plan.Boards = []LabPublishBoard{{Ref: "o/1", Title: "One"}, {Ref: "o/2", Title: "Two"}}
	m := newTestPublish(plan)
	assert.Contains(t, m.View(), "←/→")

	publishKey(m, "right")
	assert.Equal(t, LabPublishConfirmedMsg{EntryID: "e1", BoardRef: "o/2", Chosen: true}, publishKey(m, "y"))

	publishKey(m, "right") // past the last board is "no board"
	assert.Contains(t, m.View(), "Board          none")
	assert.Equal(t, LabPublishConfirmedMsg{EntryID: "e1"}, publishKey(m, "y"), "no board is not remembered as a choice")

	publishKey(m, "right")
	msg, ok := publishKey(m, "y").(LabPublishConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, "o/1", msg.BoardRef, "the choice wraps around")
}

func TestLabPublish_NoLinkedBoardIsExplained(t *testing.T) {
	plan := basePlan()
	plan.Boards, plan.Board = nil, -1
	m := newTestPublish(plan)
	assert.Contains(t, m.View(), "the repository has no linked project board")
	assert.Equal(t, LabPublishConfirmedMsg{EntryID: "e1"}, publishKey(m, "y"))
}

func TestLabPublish_ResumeIsExplained(t *testing.T) {
	plan := basePlan()
	plan.Resume = "Issue #251 was created by an earlier attempt; it is not created again."
	view := newTestPublish(plan).View()
	assert.Contains(t, view, "Issue #251 was created by an earlier attempt")
	assert.NotContains(t, view, "Nothing is sent to GitHub until you confirm")
}

func TestLabPublish_EpicListsItsTicketsInOrder(t *testing.T) {
	plan := basePlan()
	plan.Labels = []string{"epic"}
	plan.Tickets = []LabPublishTicket{
		{Key: "01", Title: "Cache the last sync"},
		{Key: "02", Title: "Show data age", BlockedBy: []string{"01"}},
	}
	view := newTestPublish(plan).View()
	assert.Contains(t, view, "TICKETS  2 sub-issues, labelled ready-for-agent, in this order")
	assert.Contains(t, view, "can start immediately")
	assert.Contains(t, view, "blocked by 01")
}

func TestLabPublish_TicketTitlesStayReadableWhenNarrow(t *testing.T) {
	plan := basePlan()
	plan.Tickets = []LabPublishTicket{{Key: "01", Title: "Cache the last sync"}}
	m := NewLabPublishPreview(plan) // no width: the narrowest layout
	assert.Contains(t, m.View(), "Cache the last sync")
}
