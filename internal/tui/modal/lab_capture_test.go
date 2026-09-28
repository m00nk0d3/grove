package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func typeIntoCapture(t *testing.T, m *LabCaptureModal, text string) {
	t.Helper()
	for _, r := range text {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == '\n' {
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		}
		m.Update(msg)
	}
}

func submitCapture(t *testing.T, m *LabCaptureModal) tea.Msg {
	t.Helper()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestLabCaptureModal_TypedTextIsSubmitted(t *testing.T) {
	m := NewLabCaptureModal(domain.LabKindIdea)
	typeIntoCapture(t, m, "Plugin API\n\nLet users extend views.")

	msg, ok := submitCapture(t, m).(LabCaptureSubmittedMsg)
	require.True(t, ok, "Ctrl+S submits the entry")
	assert.Empty(t, msg.ID, "a new entry has no ID yet")
	assert.Equal(t, domain.LabKindIdea, msg.Kind)
	assert.Equal(t, "Plugin API\n\nLet users extend views.", msg.Text)
}

func TestLabCaptureModal_TabSwitchesKind(t *testing.T) {
	m := NewLabCaptureModal(domain.LabKindIdea)
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, domain.LabKindBug, m.Kind())
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, domain.LabKindIdea, m.Kind())
	assert.Empty(t, m.Value(), "Tab does not type into the editor")
}

func TestLabCaptureModal_EmptyTextIsRejected(t *testing.T) {
	m := NewLabCaptureModal(domain.LabKindBug)
	typeIntoCapture(t, m, "   \n  ")

	assert.Nil(t, submitCapture(t, m), "blank text is not submitted")
	assert.Contains(t, m.View(), "Write something first")

	typeIntoCapture(t, m, "x")
	assert.NotContains(t, m.View(), "Write something first", "typing clears the error")
}

func TestLabCaptureModal_EscCancels(t *testing.T) {
	m := NewLabCaptureModal(domain.LabKindIdea)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	assert.IsType(t, ModalCancelledMsg{}, cmd())
}

func TestLabEditModal_PrefillsAndKeepsID(t *testing.T) {
	e := domain.LabEntry{ID: "20260928-081530-abcdef", Kind: domain.LabKindBug, Text: "Sync stalls"}
	m := NewLabEditModal(e)
	assert.Equal(t, "EDIT LAB ENTRY", m.Title())
	assert.Equal(t, "Sync stalls", m.Value())
	typeIntoCapture(t, m, " on expiry")

	msg, ok := submitCapture(t, m).(LabCaptureSubmittedMsg)
	require.True(t, ok)
	assert.Equal(t, e.ID, msg.ID)
	assert.Equal(t, domain.LabKindBug, msg.Kind)
	assert.Equal(t, "Sync stalls on expiry", msg.Text)
}

func TestNewLabCaptureModal_InvalidKindDefaultsToIdea(t *testing.T) {
	assert.Equal(t, domain.LabKindIdea, NewLabCaptureModal("").Kind())
}
