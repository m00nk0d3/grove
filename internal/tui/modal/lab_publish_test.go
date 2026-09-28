package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPublish(plan LabPublishPlan) *LabPublishModal {
	m := NewLabPublishModal(plan)
	m.SetWidth(140)
	m.SetHeight(40)
	m.SetTheme(styles.NewTheme(styles.Themes[0]))
	return m
}

func publishKey(m *LabPublishModal, key string) tea.Msg {
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

func TestLabPublish_ResumeSaysTheIssueExists(t *testing.T) {
	plan := basePlan()
	existing := 251
	plan.Existing = &existing
	assert.Contains(t, newTestPublish(plan).View(), "Issue #251 was created by an earlier attempt")
}
