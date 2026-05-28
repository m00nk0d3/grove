package modal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCfg returns a minimal *domain.Config for testing.
func newTestCfg() *domain.Config {
	return &domain.Config{
		Appearance: domain.AppearanceConfig{Theme: "digital-noir"},
		GitHub: domain.GitHubConfig{
			AutoSync:            false,
			SyncIntervalMinutes: 5,
		},
		AIAgents: domain.AIAgentsConfig{
			CopilotEnabled: true,
			ClaudeEnabled:  false,
			AiderEnabled:   false,
			ClaudeBinary:   "claude",
			AiderBinary:    "aider",
		},
		Worktrees: domain.WorktreesConfig{
			BaseBranch:   "main",
			WorktreeRoot: "../worktrees",
		},
	}
}

// newTestModal creates a SettingsModal backed by a temp-dir config path.
func newTestModal(t *testing.T) (*SettingsModal, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	m := NewSettingsModal(newTestCfg(), path)
	return m, path
}

// sendKey is a test helper that sends a single tea.KeyMsg to the modal.
func sendKey(m *SettingsModal, keyType tea.KeyType, runes ...rune) (*SettingsModal, tea.Cmd) {
	msg := tea.KeyMsg{Type: keyType}
	if keyType == tea.KeyRunes && len(runes) > 0 {
		msg.Runes = runes
	}
	updated, cmd := m.Update(msg)
	next, ok := updated.(*SettingsModal)
	if !ok {
		return m, cmd
	}
	return next, cmd
}

// sendRune sends a KeyRunes message with the given rune string.
func sendRune(m *SettingsModal, s string) (*SettingsModal, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	updated, cmd := m.Update(msg)
	next, ok := updated.(*SettingsModal)
	if !ok {
		return m, cmd
	}
	return next, cmd
}

// TestSettingsModal_Title verifies the modal title.
func TestSettingsModal_Title(t *testing.T) {
	m, _ := newTestModal(t)
	assert.Equal(t, "SETTINGS", m.Title())
}

// TestSettingsModal_TabNavigation verifies Tab/Shift+Tab and left/right arrows switch tabs.
func TestSettingsModal_TabNavigation(t *testing.T) {
	m, _ := newTestModal(t)
	require.Equal(t, 0, m.activeTab, "should start on first tab")

	// Tab moves forward.
	m, _ = sendKey(m, tea.KeyTab)
	assert.Equal(t, 1, m.activeTab)

	m, _ = sendKey(m, tea.KeyTab)
	assert.Equal(t, 2, m.activeTab)

	m, _ = sendKey(m, tea.KeyTab)
	assert.Equal(t, 3, m.activeTab)

	// Tab wraps around.
	m, _ = sendKey(m, tea.KeyTab)
	assert.Equal(t, 0, m.activeTab, "tab should wrap to first tab")

	// Right arrow on a non-choice field (navigate to GitHub tab first).
	m, _ = sendKey(m, tea.KeyTab) // → tab 1 (GitHub)
	require.Equal(t, 1, m.activeTab)
	m, _ = sendKey(m, tea.KeyRight) // should move to tab 2 (AI Agents)
	assert.Equal(t, 2, m.activeTab)

	// Shift+Tab moves backward.
	m, _ = sendKey(m, tea.KeyShiftTab)
	assert.Equal(t, 1, m.activeTab)

	// Left arrow on a non-choice field moves backward.
	m, _ = sendKey(m, tea.KeyLeft) // → tab 0 (Appearance)
	assert.Equal(t, 0, m.activeTab)

	// Left arrow on a choice field (Appearance/Theme) cycles the choice, NOT the tab.
	themeBefore := m.cfg.Appearance.Theme
	m, _ = sendKey(m, tea.KeyLeft)
	assert.Equal(t, 0, m.activeTab, "tab should not change when Left on a choice field")
	assert.NotEqual(t, themeBefore, m.cfg.Appearance.Theme, "theme should have cycled backward")
}

// TestSettingsModal_CursorNavigation verifies j/k and arrow keys move the cursor.
func TestSettingsModal_CursorNavigation(t *testing.T) {
	m, _ := newTestModal(t)
	// Navigate to GitHub tab which has 2 fields (AutoSync + SyncIntervalMinutes).
	m, _ = sendKey(m, tea.KeyTab)
	require.Equal(t, 1, m.activeTab)
	require.Equal(t, 0, m.cursor)

	// j moves down.
	m, _ = sendRune(m, "j")
	assert.Equal(t, 1, m.cursor)

	// k moves up.
	m, _ = sendRune(m, "k")
	assert.Equal(t, 0, m.cursor)

	// k at top stays at 0.
	m, _ = sendRune(m, "k")
	assert.Equal(t, 0, m.cursor)

	// Down arrow.
	m, _ = sendKey(m, tea.KeyDown)
	assert.Equal(t, 1, m.cursor)

	// Up arrow.
	m, _ = sendKey(m, tea.KeyUp)
	assert.Equal(t, 0, m.cursor)
}

// TestSettingsView_ToggleBooleanField verifies Space/Enter on a bool field toggles and saves.
func TestSettingsView_ToggleBooleanField(t *testing.T) {
	m, path := newTestModal(t)

	// Navigate to the GitHub tab (tab 1).
	m, _ = sendKey(m, tea.KeyTab)
	require.Equal(t, 1, m.activeTab)

	// The first field on GitHub tab is AutoSync (bool), currently false.
	require.Equal(t, 0, m.cursor)
	initialVal := m.cfg.GitHub.AutoSync

	// Space toggles it.
	var cmd tea.Cmd
	m, cmd = sendKey(m, tea.KeySpace)
	assert.NotEqual(t, initialVal, m.cfg.GitHub.AutoSync, "Space should toggle the bool field")

	// A save command should have been returned.
	assert.NotNil(t, cmd, "cmd should not be nil after toggle")

	// The config file should now exist on disk.
	_, err := os.Stat(path)
	require.NoError(t, err, "config file should exist after save")

	// Toggle again with Enter.
	m, cmd = sendKey(m, tea.KeyEnter)
	assert.Equal(t, initialVal, m.cfg.GitHub.AutoSync, "Enter should toggle back to original")
	assert.NotNil(t, cmd)
}

// TestSettingsView_EditStringField_SavesConfig verifies string editing and saves.
func TestSettingsView_EditStringField_SavesConfig(t *testing.T) {
	// Use a config where SyncIntervalMinutes is 0 so the textinput starts empty.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := newTestCfg()
	cfg.GitHub.SyncIntervalMinutes = 0
	m := NewSettingsModal(cfg, path)

	// Navigate to GitHub tab, then to the second field (SyncIntervalMinutes — string).
	m, _ = sendKey(m, tea.KeyTab)
	m, _ = sendKey(m, tea.KeyDown) // move to SyncIntervalMinutes field
	require.Equal(t, 1, m.cursor)

	// Press Enter to begin editing.
	m, _ = sendKey(m, tea.KeyEnter)
	require.True(t, m.editing, "should be in editing mode after Enter")

	// Type a new value via the textinput.
	for _, ch := range "15" {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		updated, _ := m.Update(msg)
		if next, ok := updated.(*SettingsModal); ok {
			m = next
		}
	}

	// Confirm with Enter.
	m, cmd := sendKey(m, tea.KeyEnter)
	assert.False(t, m.editing, "should exit editing mode after confirming")
	assert.Equal(t, 15, m.cfg.GitHub.SyncIntervalMinutes)
	assert.NotNil(t, cmd, "save cmd expected")

	// File must be on disk.
	_, err := os.Stat(path)
	require.NoError(t, err)
}

// TestSettingsModal_ChoiceCycling verifies Enter/Right/Left on a choice field cycles values.
func TestSettingsModal_ChoiceCycling(t *testing.T) {
	m, _ := newTestModal(t)

	// Tab 0 (Appearance) has the Theme choice field.
	require.Equal(t, 0, m.activeTab)
	require.Equal(t, 0, m.cursor)

	initial := m.cfg.Appearance.Theme
	initialIdx := 0
	for i, name := range styles.Themes {
		if name == initial {
			initialIdx = i
			break
		}
	}
	nextIdx := (initialIdx + 1) % len(styles.Themes)

	// Enter cycles to the next choice.
	m, _ = sendKey(m, tea.KeyEnter)
	assert.Equal(t, styles.Themes[nextIdx], m.cfg.Appearance.Theme)

	// Right arrow also cycles forward.
	m, _ = sendKey(m, tea.KeyRight)
	assert.Equal(t, styles.Themes[(nextIdx+1)%len(styles.Themes)], m.cfg.Appearance.Theme)

	// Left arrow cycles backward.
	themeBefore := m.cfg.Appearance.Theme
	m, _ = sendKey(m, tea.KeyLeft)
	wantIdx := 0
	for i, name := range styles.Themes {
		if name == themeBefore {
			wantIdx = (i - 1 + len(styles.Themes)) % len(styles.Themes)
			break
		}
	}
	assert.Equal(t, styles.Themes[wantIdx], m.cfg.Appearance.Theme)

	// Cycling wraps forward: advance to the last theme, then wrap.
	m.cfg.Appearance.Theme = styles.Themes[len(styles.Themes)-1]
	m, _ = sendKey(m, tea.KeyEnter)
	assert.Equal(t, styles.Themes[0], m.cfg.Appearance.Theme, "should wrap to first after last")

	// Cycling wraps backward: go left from first.
	m.cfg.Appearance.Theme = styles.Themes[0]
	m, _ = sendKey(m, tea.KeyLeft)
	assert.Equal(t, styles.Themes[len(styles.Themes)-1], m.cfg.Appearance.Theme, "should wrap to last when going left from first")
}

// TestSettingsModal_EscCancelsEdit verifies Esc during string editing discards the change.
func TestSettingsModal_EscCancelsEdit(t *testing.T) {
	// Use SyncIntervalMinutes=0 so the initial textinput is empty.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := newTestCfg()
	cfg.GitHub.SyncIntervalMinutes = 0
	m := NewSettingsModal(cfg, path)

	// Navigate to GitHub tab → SyncIntervalMinutes.
	m, _ = sendKey(m, tea.KeyTab)
	m, _ = sendKey(m, tea.KeyDown)
	original := m.cfg.GitHub.SyncIntervalMinutes

	// Begin editing.
	m, _ = sendKey(m, tea.KeyEnter)
	require.True(t, m.editing)

	// Type something new.
	for _, ch := range "999" {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		updated, _ := m.Update(msg)
		if next, ok := updated.(*SettingsModal); ok {
			m = next
		}
	}

	// Esc cancels without saving.
	m, _ = sendKey(m, tea.KeyEsc)
	assert.False(t, m.editing, "should exit editing mode on Esc")
	assert.Equal(t, original, m.cfg.GitHub.SyncIntervalMinutes, "value should not change on cancel")
}

// TestSettingsModal_EscClosesSettings verifies Esc while not editing sends ModalCancelledMsg.
func TestSettingsModal_EscClosesSettings(t *testing.T) {
	m, _ := newTestModal(t)
	require.False(t, m.editing)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_ = updated

	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok, "Esc while not editing should send ModalCancelledMsg, got %T", msg)
}

// TestSettingsModal_SetThemeAndWidth verifies SetTheme and SetWidth are applied without panic.
func TestSettingsModal_SetThemeAndWidth(t *testing.T) {
	m, _ := newTestModal(t)

	require.NotPanics(t, func() {
		m.SetTheme(styles.NewTheme("matrix"))
	})
	assert.Equal(t, "matrix", m.theme.Name)

	require.NotPanics(t, func() {
		m.SetWidth(120)
	})
	assert.Equal(t, 120, m.width)
}

// TestSettingsModal_ViewRendersTabBar verifies the View output contains tab names.
func TestSettingsModal_ViewRendersTabBar(t *testing.T) {
	m, _ := newTestModal(t)
	m.SetTheme(styles.NewTheme("digital-noir"))
	m.SetWidth(80)

	view := m.View()
	assert.True(t, strings.Contains(view, "Appearance") || strings.Contains(view, "APPEARANCE"),
		"view should contain Appearance tab label")
	assert.True(t, strings.Contains(view, "GitHub") || strings.Contains(view, "GITHUB"),
		"view should contain GitHub tab label")
}
