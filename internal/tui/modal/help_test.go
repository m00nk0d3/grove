package modal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHelpModal_StartsOnKeybindingsTab(t *testing.T) {
	m := NewHelpModal()

	require.NotNil(t, m)
	assert.Equal(t, tabKeybindings, m.activeTab)
	assert.Equal(t, 0, m.scrollOffset)
}

func TestHelpModal_Title_ReturnsGroveHelp(t *testing.T) {
	m := NewHelpModal()

	assert.Equal(t, "GROVE HELP", m.Title())
}

func TestHelpModal_Init_ReturnsNil(t *testing.T) {
	m := NewHelpModal()

	cmd := m.Init()
	assert.Nil(t, cmd)
}

// TestHelpModal_TabSwitching verifies all four tab switching mechanisms.
func TestHelpModal_TabSwitching(t *testing.T) {
	tests := []struct {
		name     string
		key      tea.KeyMsg
		startTab helpTab
		wantTab  helpTab
	}{
		{
			name:     "Tab key advances to next tab",
			key:      tea.KeyMsg{Type: tea.KeyTab},
			startTab: tabKeybindings,
			wantTab:  tabTips,
		},
		{
			name:     "Tab key wraps from About back to Keybindings",
			key:      tea.KeyMsg{Type: tea.KeyTab},
			startTab: tabAbout,
			wantTab:  tabKeybindings,
		},
		{
			name:     "l advances to next tab",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")},
			startTab: tabKeybindings,
			wantTab:  tabTips,
		},
		{
			name:     "h goes to previous tab",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")},
			startTab: tabTips,
			wantTab:  tabKeybindings,
		},
		{
			name:     "h wraps from Keybindings to About",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")},
			startTab: tabKeybindings,
			wantTab:  tabAbout,
		},
		{
			name:     "1 selects Keybindings tab",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")},
			startTab: tabAbout,
			wantTab:  tabKeybindings,
		},
		{
			name:     "2 selects Tips tab",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")},
			startTab: tabKeybindings,
			wantTab:  tabTips,
		},
		{
			name:     "3 selects Troubleshooting tab",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")},
			startTab: tabKeybindings,
			wantTab:  tabTroubleshooting,
		},
		{
			name:     "4 selects About tab",
			key:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")},
			startTab: tabKeybindings,
			wantTab:  tabAbout,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewHelpModal()
			m.activeTab = tc.startTab

			updated, _ := m.Update(tc.key)
			result, ok := updated.(*HelpModal)
			require.True(t, ok)
			assert.Equal(t, tc.wantTab, result.activeTab)
		})
	}
}

// TestHelpModal_EscClosesModal verifies that Esc emits ModalCancelledMsg.
func TestHelpModal_EscClosesModal(t *testing.T) {
	m := NewHelpModal()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok, "expected ModalCancelledMsg, got %T", msg)
}

func TestHelpModal_QClosesModal(t *testing.T) {
	m := NewHelpModal()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok, "expected ModalCancelledMsg, got %T", msg)
}

func TestHelpModal_TabSwitching_ResetsScrollOffset(t *testing.T) {
	m := NewHelpModal()
	m.scrollOffset = 5

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, 0, result.scrollOffset)
}

func TestHelpModal_JKey_IncreasesScrollOffset(t *testing.T) {
	m := NewHelpModal()
	m.scrollOffset = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, 1, result.scrollOffset)
}

func TestHelpModal_KKey_DecreasesScrollOffset(t *testing.T) {
	m := NewHelpModal()
	m.scrollOffset = 3

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, 2, result.scrollOffset)
}

func TestHelpModal_KKey_DoesNotGoBelowZero(t *testing.T) {
	m := NewHelpModal()
	m.scrollOffset = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, 0, result.scrollOffset)
}

func TestHelpModal_DownArrow_IncreasesScrollOffset(t *testing.T) {
	m := NewHelpModal()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, 1, result.scrollOffset)
}

func TestHelpModal_UpArrow_DecreasesScrollOffset(t *testing.T) {
	m := NewHelpModal()
	m.scrollOffset = 2

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, 1, result.scrollOffset)
}

func TestHelpModal_View_ShowsTabHeaders(t *testing.T) {
	m := NewHelpModal()
	view := m.View()

	assert.Contains(t, view, "Keybindings")
	assert.Contains(t, view, "Quick Tips")
	assert.Contains(t, view, "Troubleshooting")
	assert.Contains(t, view, "About")
}

func TestHelpModal_View_KeybindingsTab_ShowsNavigationSection(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	assert.Contains(t, view, "NAVIGATION")
}

func TestHelpModal_View_KeybindingsTab_ShowsContextActionsSection(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	assert.Contains(t, view, "CONTEXT ACTIONS")
}

func TestHelpModal_View_TipsTab_ShowsContent(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabTips
	view := m.View()

	assert.True(t, len(view) > 0)
	assert.Contains(t, strings.ToLower(view), "worktree")
}

func TestHelpModal_View_TroubleshootingTab_ShowsContent(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabTroubleshooting
	view := m.View()

	assert.True(t, len(view) > 0)
	assert.Contains(t, strings.ToLower(view), "github")
}

func TestHelpModal_View_AboutTab_ShowsRepoURL(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabAbout
	view := m.View()

	assert.Contains(t, view, "github.com/m00nk0d3/grove")
}

func TestHelpModal_View_ShowsHelpHint(t *testing.T) {
	m := NewHelpModal()
	view := m.View()

	assert.Contains(t, view, "Esc")
}

func TestHelpModal_NonKeyMsg_DoesNothing(t *testing.T) {
	m := NewHelpModal()
	original := m.activeTab

	updated, cmd := m.Update("not a key message")
	result, ok := updated.(*HelpModal)
	require.True(t, ok)

	assert.Equal(t, original, result.activeTab)
	assert.Nil(t, cmd)
}

// ---------------------------------------------------------------------------
// Phase 4: LAB view documentation tests (Issue #229)
// ---------------------------------------------------------------------------

// TestHelpModal_View_KeybindingsTab_ShowsLabDocumentation verifies that the Keybindings tab
// includes the Lab view section with keys l/L, ↑/↓, a/A, e/E, 1, 2, 3, d/D.
func TestHelpModal_View_KeybindingsTab_ShowsLabDocumentation(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	// LAB section header
	assert.Contains(t, view, "LAB VIEW")

	// Lab binding keys (with spaces as in the actual rendered output)
	assert.Contains(t, view, "↑/↓  j/k")
	assert.Contains(t, view, "a / A")
	assert.Contains(t, view, "e / E")
	assert.Contains(t, view, "1")
	assert.Contains(t, view, "2")
	assert.Contains(t, view, "3")
	assert.Contains(t, view, "d / D")

	// Description text for each binding
	assert.Contains(t, view, "Create new entry")
	assert.Contains(t, view, "Edit selected entry")
	assert.Contains(t, view, "Delete selected entry")
}

// TestHelpModal_View_KeybindingsTab_LabSectionHasCorrectKeys verifies that the LAB section has the correct keys.
func TestHelpModal_View_KeybindingsTab_LabSectionHasCorrectKeys(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	// Verify LAB is documented with proper key format (should contain l/L pattern)
	assert.Contains(t, view, "l / L")

	// The section should document navigation between kind options
	assert.Contains(t, view, "Navigate entries")
}

// TestHelpModal_View_KeybindingsTab_LabDeleteBindingPresent verifies that the delete binding is documented.
func TestHelpModal_View_KeybindingsTab_LabDeleteBindingPresent(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	assert.Contains(t, view, "d / D")
	assert.Contains(t, view, "Delete selected entry")
}

// TestHelpModal_View_KeybindingsTab_LabAddIdeaBindingPresent verifies that the add idea binding is documented.
func TestHelpModal_View_KeybindingsTab_LabAddIdeaBindingPresent(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	assert.Contains(t, view, "a / A")
	assert.Contains(t, view, "Create new entry")
}

// TestHelpModal_View_KeybindingsTab_LabAddBugBindingPresent verifies that the add bug binding is documented.
func TestHelpModal_View_KeybindingsTab_LabAddBugBindingPresent(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	assert.Contains(t, view, "e / E")
	assert.Contains(t, view, "Edit selected entry")
}

// TestHelpModal_View_KeybindingsTab_LabFilterByKindBindingPresent verifies that the filter binding is documented.
func TestHelpModal_View_KeybindingsTab_LabFilterByKindBindingPresent(t *testing.T) {
	m := NewHelpModal()
	m.activeTab = tabKeybindings
	view := m.View()

	assert.Contains(t, view, "1")
	assert.Contains(t, view, "2")
	assert.Contains(t, view, "3")
}
