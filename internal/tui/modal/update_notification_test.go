package modal

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateNotificationShowsCurrentAndLatestVersions(t *testing.T) {
	m := NewUpdateNotificationModal("1.9.3", "v1.10.0", "install grove")

	view := m.View()

	assert.Contains(t, view, "Current:         v1.9.3")
	assert.Contains(t, view, "Latest release:  v1.10.0")
	assert.NotContains(t, view, "vv1.10.0")
}

func TestUpdateNotificationCopiesInstallCommand(t *testing.T) {
	m := NewUpdateNotificationModal("v1.9.3", "v1.10.0", "install grove")
	var copied string
	m.copyCommand = func(value string) error {
		copied = value
		return nil
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	require.NotNil(t, cmd)
	result := cmd()
	assert.True(t, IsOwnMessage(result))
	_, _ = m.Update(result)

	assert.Equal(t, "install grove", copied)
	assert.Contains(t, m.View(), "Command copied to clipboard")
}

func TestUpdateNotificationReportsClipboardFailure(t *testing.T) {
	m := NewUpdateNotificationModal("v1.9.3", "v1.10.0", "install grove")
	m.copyCommand = func(string) error { return errors.New("clipboard unavailable") }

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	require.NotNil(t, cmd)
	_, _ = m.Update(cmd())

	assert.Contains(t, m.View(), "Copy failed: clipboard unavailable")
	assert.NotContains(t, m.View(), "Command copied")
}
