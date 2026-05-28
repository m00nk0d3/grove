package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testChangelog = "## Changes\n- Fixed stuff\n- Added cool things"
const testHTMLURL = "https://github.com/m00nk0d3/grove/releases/v1.2.3"

func TestUpdateModal_Title(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", 0)
	assert.Contains(t, m.Title(), "Update Available")
}

func TestUpdateModal_View_ContainsBothVersions(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", 0)
	view := m.View()
	assert.Contains(t, view, "v1.0.0")
	assert.Contains(t, view, "v1.2.3")
}

func TestUpdateModal_View_ShowsChangelog(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", testChangelog, testHTMLURL, 0)
	view := m.View()
	assert.Contains(t, view, "WHAT'S NEW")
	assert.Contains(t, view, "Fixed stuff")
	assert.Contains(t, view, "Added cool things")
}

func TestUpdateModal_View_TruncatesLongChangelog(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "- line")
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	m := NewUpdateModal("v1.0.0", "v1.2.3", body, testHTMLURL, 0)
	view := m.View()
	assert.Contains(t, view, "↓ full changelog at")
	assert.Contains(t, view, testHTMLURL)
}

func TestUpdateModal_View_NoChangelogSection_WhenEmpty(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", 0)
	view := m.View()
	assert.NotContains(t, view, "WHAT'S NEW")
}

func TestUpdateModal_View_ActiveSessionsWarning(t *testing.T) {
	tests := []struct {
		name           string
		activeSessions int
		wantWarning    bool
	}{
		{name: "no sessions", activeSessions: 0, wantWarning: false},
		{name: "one session", activeSessions: 1, wantWarning: true},
		{name: "multiple sessions", activeSessions: 3, wantWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", tt.activeSessions)
			view := m.View()
			if tt.wantWarning {
				assert.Contains(t, view, "⚠")
			} else {
				assert.NotContains(t, view, "⚠")
			}
		})
	}
}

func TestUpdateModal_Update_YConfirms(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", 0)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	require.NotNil(t, cmd)
	assert.Equal(t, UpdateConfirmedMsg{}, cmd())
}

func TestUpdateModal_Update_NCancels(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", 0)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	require.NotNil(t, cmd)
	assert.Equal(t, ModalCancelledMsg{}, cmd())
}

func TestUpdateModal_Update_EscCancels(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "v1.2.3", "", "", 0)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)
	assert.Equal(t, ModalCancelledMsg{}, cmd())
}
