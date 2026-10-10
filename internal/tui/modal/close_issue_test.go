package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testOpenIssue() domain.Issue {
	return domain.Issue{Number: 7, Title: "Sync stalls", State: "OPEN"}
}

func confirmMsg(t *testing.T, cmd tea.Cmd) IssueCloseConfirmedMsg {
	t.Helper()
	require.NotNil(t, cmd)
	msg, ok := cmd().(IssueCloseConfirmedMsg)
	require.True(t, ok, "expected IssueCloseConfirmedMsg, got %T", cmd())
	return msg
}

func TestNewIssueCloseModal_DefaultsToCompleted(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	require.NotNil(t, m)
	assert.Equal(t, "Close Issue", m.Title())
	assert.Equal(t, "completed", m.SelectedReason())
	assert.True(t, m.CanConfirm())
	assert.Empty(t, m.Comment())
}

func TestIssueCloseModal_View_ShowsIssueReasonAndHints(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)
	view := m.View()

	assert.Contains(t, view, "#7")
	assert.Contains(t, view, "Sync stalls")
	assert.Contains(t, view, "Completed")
	assert.Contains(t, view, "Not planned")
	assert.Contains(t, view, "Duplicate")
	assert.Contains(t, view, "Comment:")
}

func TestIssueCloseModal_Y_EmitsConfirmedWithReason(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	msg := confirmMsg(t, cmd)
	assert.Equal(t, 7, msg.Number)
	assert.Equal(t, "completed", msg.Reason)
	assert.Empty(t, msg.Comment)
}

func TestIssueCloseModal_Navigation_SelectsReason(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(*IssueCloseModal)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(*IssueCloseModal)
	assert.Equal(t, "duplicate", m.SelectedReason())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, "duplicate", confirmMsg(t, cmd).Reason)

	// Moving up clamps at the first reason.
	for range len(CloseIssueReasons) + 1 {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = updated.(*IssueCloseModal)
	}
	assert.Equal(t, "completed", m.SelectedReason())
}

func TestIssueCloseModal_Comment_FlowsIntoConfirmedMsg(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(*IssueCloseModal)
	for _, r := range "shipped in v2" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(*IssueCloseModal)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := confirmMsg(t, cmd)
	assert.Equal(t, "completed", msg.Reason)
	assert.Equal(t, "shipped in v2", msg.Comment)
}

func TestIssueCloseModal_Esc_FromComment_ReturnsToReasons(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(*IssueCloseModal)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Nil(t, cmd, "first Esc only leaves the comment")
	m = updated.(*IssueCloseModal)

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	_, ok := cmd().(ModalCancelledMsg)
	assert.True(t, ok)
}

func TestIssueCloseModal_N_EmitsCancelMsg(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.NotNil(t, cmd)
	_, ok := cmd().(ModalCancelledMsg)
	assert.True(t, ok)
}

func TestIssueCloseModal_ClosedIssue_DisablesConfirm(t *testing.T) {
	m := NewIssueCloseModal(domain.Issue{Number: 9, Title: "Old", State: "CLOSED"}, true)

	assert.False(t, m.CanConfirm())
	assert.Contains(t, m.View(), "already closed")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Nil(t, cmd, "a closed issue must not confirm")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Nil(t, cmd)
}

func TestIssueCloseModal_NonCollaborator_DisablesConfirm(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), false)

	assert.False(t, m.CanConfirm())
	assert.Contains(t, m.View(), "permission")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	assert.Nil(t, cmd, "a non-collaborator must not confirm")
}

func TestIssueCloseModal_SetError_RendersInline(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)
	m.SetError("GitHub rate limit hit — try again later.")

	assert.Contains(t, m.View(), "rate limit")
}

func TestIssueCloseModal_OtherKeys_DoNothing(t *testing.T) {
	m := NewIssueCloseModal(testOpenIssue(), true)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
}
