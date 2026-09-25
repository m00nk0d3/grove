package modal

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	githubSection    = 1
	worktreesSection = 2
	agentsSection    = 3
)

// newTestCfg returns a minimal *domain.Config for testing.
func newTestCfg() *domain.Config {
	cfg := domain.DefaultConfig()
	cfg.GitHub.AutoSync = false
	return cfg
}

// newTestModal creates a SettingsModal backed by a temp-dir config path.
func newTestModal(t *testing.T) (*SettingsModal, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	return NewSettingsModal(newTestCfg(), path), path
}

// sendKey sends a single key of the given type to the modal.
func sendKey(m *SettingsModal, keyType tea.KeyType) (*SettingsModal, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: keyType})
	return updated.(*SettingsModal), cmd
}

// sendRune sends a KeyRunes message with the given rune string.
func sendRune(m *SettingsModal, s string) (*SettingsModal, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(*SettingsModal), cmd
}

// typeText sends each rune of s to the modal as a separate key.
func typeText(m *SettingsModal, s string) *SettingsModal {
	for _, r := range s {
		m, _ = sendRune(m, string(r))
	}
	return m
}

// savedConfig reports the config a command dispatches, if it dispatches
// SettingsSavedMsg. Status-clear ticks in a batch are not waited for.
func savedConfig(t *testing.T, cmd tea.Cmd) *domain.Config {
	t.Helper()
	if cmd == nil {
		return nil
	}
	results := make(chan tea.Msg, 8)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		go func() { results <- c() }()
	}
	run(cmd)
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case msg := <-results:
			switch msg := msg.(type) {
			case SettingsSavedMsg:
				return msg.Config
			case tea.BatchMsg:
				for _, c := range msg {
					if c != nil {
						run(c)
					}
				}
			}
		case <-deadline:
			return nil
		}
	}
}

func loadSaved(t *testing.T, path string) *domain.Config {
	t.Helper()
	cfg, err := data.LoadConfig(path)
	require.NoError(t, err)
	return cfg
}

func TestSettingsModal_IsFullscreenWithTitle(t *testing.T) {
	m, _ := newTestModal(t)
	assert.True(t, m.Fullscreen())
	assert.Contains(t, m.Title(), "SETTINGS")
}

func TestSettingsModal_SectionNavigation(t *testing.T) {
	m, _ := newTestModal(t)
	require.Equal(t, appearanceSection, m.activeSection)

	m, _ = sendKey(m, tea.KeyTab)
	assert.Equal(t, githubSection, m.activeSection)
	m, _ = sendKey(m, tea.KeyShiftTab)
	m, _ = sendKey(m, tea.KeyShiftTab)
	assert.Equal(t, agentsSection, m.activeSection, "shift+tab wraps to the last section")
	m, _ = sendKey(m, tea.KeyTab)
	assert.Equal(t, appearanceSection, m.activeSection, "tab wraps to the first section")

	m, _ = sendRune(m, "2")
	assert.Equal(t, githubSection, m.activeSection, "a digit jumps to that section")

	m, _ = sendKey(m, tea.KeyRight)
	assert.Equal(t, worktreesSection, m.activeSection, "right switches section off a choice field")
	m, _ = sendKey(m, tea.KeyLeft)
	assert.Equal(t, githubSection, m.activeSection)
}

func TestSettingsModal_ThemeListStartsOnTheActiveTheme(t *testing.T) {
	cfg := newTestCfg()
	cfg.Appearance.Theme = "nord"
	m := NewSettingsModal(cfg, filepath.Join(t.TempDir(), "config.toml"))

	assert.Equal(t, "nord", styles.Themes[m.cursors[appearanceSection]])
	assert.Equal(t, "nord", m.hoveredTheme().Name)
}

func TestSettingsModal_BrowsingThemesPreviewsWithoutApplying(t *testing.T) {
	m, path := newTestModal(t)

	m, cmd := sendKey(m, tea.KeyDown)
	m, _ = sendRune(m, "j")

	assert.Nil(t, cmd, "moving through the list saves nothing")
	assert.Equal(t, styles.Themes[2], m.hoveredTheme().Name)
	assert.Equal(t, "digital-noir", m.cfg.Appearance.Theme)
	assert.NoFileExists(t, path)
	assert.Contains(t, m.View(), m.hoveredTheme().Label())
}

func TestSettingsModal_EnterAppliesAndSavesTheHoveredTheme(t *testing.T) {
	m, path := newTestModal(t)
	m, _ = sendKey(m, tea.KeyDown)
	want := m.hoveredTheme().Name

	m, cmd := sendKey(m, tea.KeyEnter)

	assert.Equal(t, want, m.cfg.Appearance.Theme)
	saved := savedConfig(t, cmd)
	require.NotNil(t, saved, "applying a theme dispatches SettingsSavedMsg")
	assert.Equal(t, want, saved.Appearance.Theme)
	assert.Equal(t, want, loadSaved(t, path).Appearance.Theme)
}

func TestSettingsModal_ReapplyingTheActiveThemeDoesNotSave(t *testing.T) {
	m, path := newTestModal(t)

	m, cmd := sendKey(m, tea.KeyEnter)

	assert.Nil(t, savedConfig(t, cmd))
	assert.NoFileExists(t, path)
	assert.Contains(t, m.statusMsg, "already active")
}

func TestSettingsModal_ThemeCursorStaysInTheList(t *testing.T) {
	m, _ := newTestModal(t)

	m, _ = sendKey(m, tea.KeyUp)
	assert.Equal(t, 0, m.cursors[appearanceSection])

	m, _ = sendRune(m, "G")
	assert.Equal(t, len(styles.Themes)-1, m.cursors[appearanceSection])
	m, _ = sendKey(m, tea.KeyDown)
	assert.Equal(t, len(styles.Themes)-1, m.cursors[appearanceSection])

	m, _ = sendRune(m, "g")
	assert.Equal(t, 0, m.cursors[appearanceSection])
}

func TestSettingsModal_ToggleBooleanField(t *testing.T) {
	m, path := newTestModal(t)
	m, _ = sendKey(m, tea.KeyTab) // GitHub; the cursor is on auto sync
	require.False(t, m.cfg.GitHub.AutoSync)

	m, cmd := sendKey(m, tea.KeySpace)
	assert.True(t, m.cfg.GitHub.AutoSync)
	assert.NotNil(t, savedConfig(t, cmd))
	assert.True(t, loadSaved(t, path).GitHub.AutoSync)

	m, _ = sendKey(m, tea.KeyEnter)
	assert.False(t, m.cfg.GitHub.AutoSync, "enter toggles back")
}

func TestSettingsModal_EditNumberField(t *testing.T) {
	m, path := newTestModal(t)
	m, _ = sendKey(m, tea.KeyTab)
	m, _ = sendKey(m, tea.KeyDown) // sync interval

	m, _ = sendKey(m, tea.KeyEnter)
	require.True(t, m.editing)
	m.textInput.SetValue("")
	m = typeText(m, "15")
	m, cmd := sendKey(m, tea.KeyEnter)

	assert.False(t, m.editing)
	assert.Equal(t, 15, m.cfg.GitHub.SyncIntervalMinutes)
	assert.NotNil(t, savedConfig(t, cmd))
	assert.Equal(t, 15, loadSaved(t, path).GitHub.SyncIntervalMinutes)
}

func TestSettingsModal_RejectsAnInvalidNumberAndKeepsEditing(t *testing.T) {
	for _, input := range []string{"soon", "0", "-3"} {
		t.Run(input, func(t *testing.T) {
			m, path := newTestModal(t)
			m, _ = sendKey(m, tea.KeyTab)
			m, _ = sendKey(m, tea.KeyDown)
			m, _ = sendKey(m, tea.KeyEnter)
			m.textInput.SetValue(input)

			m, cmd := sendKey(m, tea.KeyEnter)

			assert.True(t, m.editing, "the editor stays open for a correction")
			assert.True(t, m.statusErr)
			assert.Equal(t, 5, m.cfg.GitHub.SyncIntervalMinutes)
			assert.Nil(t, savedConfig(t, cmd))
			assert.NoFileExists(t, path)
		})
	}
}

func TestSettingsModal_EditTextFieldRejectsEmpty(t *testing.T) {
	m, _ := newTestModal(t)
	m, _ = sendRune(m, "3") // worktrees; the cursor is on base branch
	m, _ = sendKey(m, tea.KeyEnter)
	m.textInput.SetValue("   ")

	m, _ = sendKey(m, tea.KeyEnter)

	assert.True(t, m.editing)
	assert.Equal(t, "main", m.cfg.Worktrees.BaseBranch)
}

func TestSettingsModal_EscCancelsEditThenCloses(t *testing.T) {
	m, _ := newTestModal(t)
	m, _ = sendRune(m, "3")
	m, _ = sendKey(m, tea.KeyEnter)
	m = typeText(m, "-scratch")

	m, cmd := sendKey(m, tea.KeyEsc)
	assert.False(t, m.editing)
	assert.Nil(t, cmd)
	assert.Equal(t, "main", m.cfg.Worktrees.BaseBranch, "a cancelled edit is discarded")

	_, cmd = sendKey(m, tea.KeyEsc)
	require.NotNil(t, cmd)
	assert.IsType(t, ModalCancelledMsg{}, cmd())
}

func TestSettingsModal_DefaultAgentCyclesAndSaves(t *testing.T) {
	m, path := newTestModal(t)
	m, _ = sendRune(m, "4") // agents; the cursor is on default agent
	require.Equal(t, "opencode", m.cfg.Sandcastle.DefaultAgent)

	m, cmd := sendKey(m, tea.KeyRight)
	assert.Equal(t, agentsSection, m.activeSection, "right cycles a choice instead of switching section")
	assert.Equal(t, "pi", m.cfg.Sandcastle.DefaultAgent)
	assert.NotNil(t, savedConfig(t, cmd))

	m, _ = sendKey(m, tea.KeyEnter)
	assert.Equal(t, "claude", m.cfg.Sandcastle.DefaultAgent)
	m, _ = sendKey(m, tea.KeyRight)
	assert.Equal(t, "opencode", m.cfg.Sandcastle.DefaultAgent, "the choice wraps")
	m, _ = sendKey(m, tea.KeyLeft)
	assert.Equal(t, "claude", m.cfg.Sandcastle.DefaultAgent)
	assert.Equal(t, "claude", loadSaved(t, path).Sandcastle.DefaultAgent)
}

func TestSettingsModal_AgentSwitchesSave(t *testing.T) {
	m, path := newTestModal(t)
	m, _ = sendRune(m, "4")
	m, _ = sendKey(m, tea.KeyDown) // sandcastle runtime
	m, _ = sendKey(m, tea.KeySpace)
	m, _ = sendKey(m, tea.KeyDown) // herdr integration
	m, _ = sendKey(m, tea.KeySpace)

	saved := loadSaved(t, path)
	assert.False(t, saved.Sandcastle.Enabled)
	assert.False(t, saved.Herdr.Enabled)
}

func TestSettingsModal_StaleStatusTickDoesNotClearANewerMessage(t *testing.T) {
	m, _ := newTestModal(t)
	m.setStatus("first", false)
	stale := clearStatusMsg{seq: m.statusSeq}
	m.setStatus("second", false)

	updated, _ := m.Update(stale)
	assert.Equal(t, "second", updated.(*SettingsModal).statusMsg)

	updated, _ = m.Update(clearStatusMsg{seq: m.statusSeq})
	assert.Empty(t, updated.(*SettingsModal).statusMsg)
}

func TestSettingsModal_ViewShowsSectionsThemesAndPreview(t *testing.T) {
	m, _ := newTestModal(t)
	m.SetWidth(160)
	m.SetHeight(45)

	view := m.View()

	for _, s := range []string{"APPEARANCE", "GITHUB", "WORKTREES", "AGENTS", "PREVIEW", "DARK", "LIGHT"} {
		assert.Contains(t, view, s)
	}
	for _, name := range styles.Themes {
		assert.Contains(t, view, styles.NewTheme(name).Label(), "a %d-row screen lists every theme", 45)
	}
}

func TestSettingsModal_ViewShowsFieldKeysAndDetail(t *testing.T) {
	m, _ := newTestModal(t)
	m.SetWidth(160)
	m.SetHeight(45)
	m, _ = sendRune(m, "4")

	view := m.View()

	assert.Contains(t, view, "DEFAULT AGENT")
	assert.Contains(t, view, "sandcastle.default_agent")
	assert.Contains(t, view, "opencode / pi / claude")
}

func TestSettingsModal_ViewFitsTheScreen(t *testing.T) {
	sizes := []struct{ width, height int }{{160, 45}, {100, 24}, {64, 20}, {60, 0}}
	for _, size := range sizes {
		for section := range 4 {
			m, _ := newTestModal(t)
			m.SetWidth(size.width)
			m.SetHeight(size.height)
			m.activeSection = section
			m.cursors[appearanceSection] = len(styles.Themes) - 1

			view := m.View()

			width, rows := m.layout()
			for i, line := range strings.Split(view, "\n") {
				assert.LessOrEqual(t, lipgloss.Width(line), width,
					"%dx%d section %d line %d overflows", size.width, size.height, section, i)
			}
			if rows > 0 {
				assert.Equal(t, rows, lipgloss.Height(view),
					"%dx%d section %d fills exactly the rows it is given", size.width, size.height, section)
			}
			if section == appearanceSection {
				assert.Contains(t, view, styles.NewTheme(styles.Themes[len(styles.Themes)-1]).Label(),
					"%dx%d keeps the hovered theme in view", size.width, size.height)
			}
		}
	}
}
