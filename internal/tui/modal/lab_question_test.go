package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func questionKey(m tea.Model, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "tab":
		return m.Update(tea.KeyMsg{Type: tea.KeyTab})
	case "esc":
		return m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	case " ":
		return m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	}
	return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func submitted(t *testing.T, cmd tea.Cmd) LabAnswerSubmittedMsg {
	t.Helper()
	require.NotNil(t, cmd)
	msg, ok := cmd().(LabAnswerSubmittedMsg)
	require.True(t, ok)
	return msg
}

func TestLabQuestionCard_EnterAcceptsTheRecommendation(t *testing.T) {
	m := NewLabQuestionCard("e1", 2, domain.LabQuestion{ID: 2, Kind: domain.LabQuestionChoice, Question: "Keep it?", Options: []string{"Yes", "No", "Later"}, Recommended: []int{2}}, "")
	assert.Contains(t, m.View(), "(recommended)")
	_, cmd := questionKey(m, "enter")
	assert.Equal(t, LabAnswerSubmittedMsg{EntryID: "e1", Number: 2, Choices: []int{2}}, submitted(t, cmd))
}

func TestLabQuestionCard_MultiTogglesAndRequiresAPick(t *testing.T) {
	m := NewLabQuestionCard("e1", 1, domain.LabQuestion{ID: 1, Kind: domain.LabQuestionMulti, Options: []string{"A", "B", "C"}, Recommended: []int{0}}, "")
	questionKey(m, "1") // untoggle the recommendation
	_, cmd := questionKey(m, "enter")
	assert.Nil(t, cmd, "an empty answer is not sent")
	assert.Contains(t, m.View(), "Pick at least one option")
	questionKey(m, "2")
	questionKey(m, "3")
	_, cmd = questionKey(m, "enter")
	assert.Equal(t, []int{1, 2}, submitted(t, cmd).Choices)
}

func TestLabQuestionCard_TheNoteTakesEveryKeyUntilEsc(t *testing.T) {
	m := NewLabQuestionCard("e1", 1, domain.LabQuestion{ID: 1, Kind: domain.LabQuestionChoice, Options: []string{"A", "B"}, Recommended: []int{0}}, "")
	questionKey(m, "tab")
	for _, r := range "j 2" {
		questionKey(m, string(r))
	}
	_, cmd := questionKey(m, "esc")
	if cmd != nil {
		_, cancelled := cmd().(ModalCancelledMsg)
		assert.False(t, cancelled, "Esc in the note returns to the options")
	}
	questionKey(m, "2")
	_, cmd = questionKey(m, "enter")
	answer := submitted(t, cmd)
	assert.Equal(t, []int{1}, answer.Choices, "keys typed in the note did not move the choice")
	assert.Equal(t, "j 2", answer.Text)
}

func TestLabQuestionCard_TextCardsStartFromTheSuggestion(t *testing.T) {
	m := NewLabQuestionCard("e1", 4, domain.LabQuestion{ID: 4, Kind: domain.LabQuestionText, RecommendedText: "Windows 11"}, "")
	_, cmd := questionKey(m, "enter")
	assert.Equal(t, LabAnswerSubmittedMsg{EntryID: "e1", Number: 4, Text: "Windows 11"}, submitted(t, cmd))
}
