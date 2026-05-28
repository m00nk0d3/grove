package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testWorktreePath = "/home/user/worktrees/feat-issue-21-agent-launcher"

var testAgentOptions = []agentOption{
	{name: "Claude Code", key: "a", internal: "claude", available: true},
	{name: "Copilot CLI", key: "c", internal: "copilot", available: true},
	{name: "Aider", key: "d", internal: "aider", available: false},
}

func newTestLauncher() *AgentLauncherModal {
	return newAgentLauncherModal(testAgentOptions, testWorktreePath)
}

// --- Constructor ---

func TestNewAgentLauncherModal_StartsAtSelectStep(t *testing.T) {
	m := newTestLauncher()

	require.NotNil(t, m)
	assert.Equal(t, stepAgentSelect, m.step)
	assert.Equal(t, 0, m.selectedIdx)
	assert.Equal(t, testWorktreePath, m.worktreePath)
}

func TestAgentLauncherModal_Title(t *testing.T) {
	m := newTestLauncher()
	assert.Equal(t, "SPAWN AGENT", m.Title())
}

// --- View: agent selection step ---

func TestAgentLauncherModal_View_ShowsAllAgents(t *testing.T) {
	m := newTestLauncher()
	view := m.View()

	assert.Contains(t, view, "Claude Code")
	assert.Contains(t, view, "Copilot CLI")
	assert.Contains(t, view, "Aider")
}

func TestAgentLauncherModal_View_ShowsAvailabilityStatus(t *testing.T) {
	m := newTestLauncher()
	view := m.View()

	assert.Contains(t, view, "● available")
	assert.Contains(t, view, "✗ not configured")
}

func TestAgentLauncherModal_View_ShowsCursorOnSelected(t *testing.T) {
	m := newTestLauncher()
	view := m.View()

	assert.Contains(t, view, "> ")
}

func TestAgentLauncherModal_View_ShowsNavigationHint(t *testing.T) {
	m := newTestLauncher()
	view := m.View()

	assert.Contains(t, view, "↑/↓")
	assert.Contains(t, view, "Esc cancel")
}

// --- Navigation ---

func TestAgentLauncherModal_DownKey_MovesSelection(t *testing.T) {
	m := newTestLauncher()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, 1, m.selectedIdx)
}

func TestAgentLauncherModal_UpKey_MovesSelectionUp(t *testing.T) {
	m := newTestLauncher()
	m.selectedIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, 0, m.selectedIdx)
}

func TestAgentLauncherModal_DownKey_DoesNotExceedList(t *testing.T) {
	m := newTestLauncher()
	m.selectedIdx = 2

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, 2, m.selectedIdx)
}

func TestAgentLauncherModal_UpKey_DoesNotGoBelowZero(t *testing.T) {
	m := newTestLauncher()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, 0, m.selectedIdx)
}

// --- TDD: Unavailable agent is not selectable ---

func TestAgentLauncherModal_UnavailableAgentNotSelectable(t *testing.T) {
	m := newTestLauncher()
	// Navigate to Aider (index 2), which is unavailable.
	m.selectedIdx = 2

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Entering on an unavailable agent must emit nothing.
	assert.Nil(t, cmd, "unavailable agent should not emit any command")
	assert.Equal(t, stepAgentSelect, m.step, "step should remain on selection")
}

// --- TDD: Selecting Claude/Copilot advances to prompt step ---

func TestAgentLauncherModal_SelectClaude_AdvancesToPrompt(t *testing.T) {
	m := newTestLauncher()
	// Claude is at index 0.

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, stepAgentPrompt, m.step)
	assert.NotNil(t, cmd, "Focus cmd should be returned")
}

func TestAgentLauncherModal_SelectCopilot_AdvancesToPrompt(t *testing.T) {
	m := newTestLauncher()
	m.selectedIdx = 1 // Copilot

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, stepAgentPrompt, m.step)
	assert.NotNil(t, cmd)
}

func TestAgentLauncherModal_SelectClaude_SetsSelectedAgent(t *testing.T) {
	m := newTestLauncher()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, "claude", m.selectedAgent.internal)
}

// --- View: prompt step ---

func TestAgentLauncherModal_View_PromptStep_ShowsAgentNameAndPath(t *testing.T) {
	m := newTestLauncher()
	m.step = stepAgentPrompt
	m.selectedAgent = testAgentOptions[0] // Claude

	view := m.View()

	assert.Contains(t, view, "Claude Code")
	assert.Contains(t, view, testWorktreePath)
}

func TestAgentLauncherModal_View_PromptStep_ShowsConfirmHint(t *testing.T) {
	m := newTestLauncher()
	m.step = stepAgentPrompt
	m.selectedAgent = testAgentOptions[0]

	view := m.View()

	assert.Contains(t, view, "Enter confirm")
	assert.Contains(t, view, "Esc cancel")
}

// --- Prompt submission ---

func TestAgentLauncherModal_PromptEnter_EmitsSpawnAgentMsg(t *testing.T) {
	m := newTestLauncher()
	m.step = stepAgentPrompt
	m.selectedAgent = testAgentOptions[0] // Claude
	m.promptInput.SetValue("fix the bug")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	spawnMsg, ok := msg.(SpawnAgentMsg)
	require.True(t, ok)
	assert.Equal(t, "claude", spawnMsg.AgentName)
	assert.Equal(t, testWorktreePath, spawnMsg.WorktreePath)
	assert.Equal(t, "fix the bug", spawnMsg.Prompt)
}

func TestAgentLauncherModal_PromptEnter_TrimsWhitespace(t *testing.T) {
	m := newTestLauncher()
	m.step = stepAgentPrompt
	m.selectedAgent = testAgentOptions[1] // Copilot
	m.promptInput.SetValue("  suggest some code  ")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	spawnMsg := msg.(SpawnAgentMsg)
	assert.Equal(t, "suggest some code", spawnMsg.Prompt)
}

func TestAgentLauncherModal_EmptyPrompt_EmitsSpawnAgentMsg(t *testing.T) {
	m := newTestLauncher()
	m.step = stepAgentPrompt
	m.selectedAgent = testAgentOptions[0] // Claude
	// Leave promptInput empty (default)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "empty prompt should still emit a spawn command")
	msg := cmd()
	spawnMsg, ok := msg.(SpawnAgentMsg)
	require.True(t, ok)
	assert.Equal(t, "claude", spawnMsg.AgentName)
	assert.Empty(t, spawnMsg.Prompt)
}

// --- Aider: skips prompt and emits SpawnAgentMsg directly ---

func TestAgentLauncherModal_SelectAider_WhenAvailable_EmitsSpawnMsg(t *testing.T) {
	opts := []agentOption{
		{name: "Aider", key: "d", internal: "aider", available: true},
	}
	m := newAgentLauncherModal(opts, testWorktreePath)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	spawnMsg, ok := msg.(SpawnAgentMsg)
	require.True(t, ok)
	assert.Equal(t, "aider", spawnMsg.AgentName)
	assert.Equal(t, testWorktreePath, spawnMsg.WorktreePath)
	assert.Empty(t, spawnMsg.Prompt)
}

// --- Esc cancels ---

func TestAgentLauncherModal_Esc_OnSelectStep_EmitsCancelMsg(t *testing.T) {
	m := newTestLauncher()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok)
}

func TestAgentLauncherModal_Esc_OnPromptStep_EmitsCancelMsg(t *testing.T) {
	m := newTestLauncher()
	m.step = stepAgentPrompt
	m.selectedAgent = testAgentOptions[0]

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(ModalCancelledMsg)
	assert.True(t, ok)
}

// --- buildAgentOptions ---

func TestBuildAgentOptions_AllDisabled_AllUnavailable(t *testing.T) {
	cfg := &domain.Config{}
	opts := buildAgentOptions(cfg)

	require.Len(t, opts, 3)
	for _, opt := range opts {
		assert.False(t, opt.available, "all agents should be unavailable when disabled in config")
	}
}

func TestBuildAgentOptions_AlwaysReturnsThreeOptions(t *testing.T) {
	cfg := &domain.Config{}
	opts := buildAgentOptions(cfg)

	assert.Len(t, opts, 3)
}

func TestBuildAgentOptions_OptionsHaveExpectedInternalNames(t *testing.T) {
	cfg := &domain.Config{}
	opts := buildAgentOptions(cfg)

	assert.Equal(t, "claude", opts[0].internal)
	assert.Equal(t, "copilot", opts[1].internal)
	assert.Equal(t, "aider", opts[2].internal)
}

// --- Cursor auto-advance ---

// TestNewAgentLauncherModal_CursorSkipsToFirstAvailable verifies that the constructor
// places the cursor on the first available agent, not blindly at index 0.
func TestNewAgentLauncherModal_CursorSkipsToFirstAvailable(t *testing.T) {
	opts := []agentOption{
		{name: "Claude Code", key: "a", internal: "claude", available: false},
		{name: "Copilot CLI", key: "c", internal: "copilot", available: true},
		{name: "Aider", key: "d", internal: "aider", available: false},
	}
	m := newAgentLauncherModal(opts, testWorktreePath)

	assert.Equal(t, 1, m.selectedIdx, "cursor should skip unavailable Claude and land on Copilot")
}

// TestNewAgentLauncherModal_AllUnavailable_CursorRemainsAtZero verifies graceful
// behaviour when no agent is available — cursor stays at 0 (no panic).
func TestNewAgentLauncherModal_AllUnavailable_CursorRemainsAtZero(t *testing.T) {
	opts := []agentOption{
		{name: "Claude Code", key: "a", internal: "claude", available: false},
		{name: "Copilot CLI", key: "c", internal: "copilot", available: false},
		{name: "Aider", key: "d", internal: "aider", available: false},
	}
	m := newAgentLauncherModal(opts, testWorktreePath)

	assert.Equal(t, 0, m.selectedIdx, "cursor stays at 0 when nothing is available")
}

// --- Shortcut keys (a / c / d) ---

// TestAgentLauncherModal_ShortcutA_SelectsClaude verifies that pressing "a" in the
// selection step immediately activates Claude (same as navigating to it and pressing Enter).
func TestAgentLauncherModal_ShortcutA_SelectsClaude(t *testing.T) {
	m := newTestLauncher()
	m.selectedIdx = 1 // Start on Copilot to make the test non-trivial.

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, stepAgentPrompt, m.step, "pressing 'a' must advance to the prompt step")
	assert.NotNil(t, cmd, "Focus cmd must be returned")
	assert.Equal(t, "claude", m.selectedAgent.internal)
}

// TestAgentLauncherModal_ShortcutC_SelectsCopilot verifies the "c" shortcut.
func TestAgentLauncherModal_ShortcutC_SelectsCopilot(t *testing.T) {
	m := newTestLauncher()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated.(*AgentLauncherModal)

	assert.Equal(t, stepAgentPrompt, m.step)
	assert.NotNil(t, cmd)
	assert.Equal(t, "copilot", m.selectedAgent.internal)
}

// TestAgentLauncherModal_ShortcutD_WhenUnavailable_IsNoOp verifies that pressing the
// Aider shortcut when Aider is unavailable does nothing (matches Enter behaviour).
func TestAgentLauncherModal_ShortcutD_WhenUnavailable_IsNoOp(t *testing.T) {
	m := newTestLauncher() // testAgentOptions has Aider as unavailable.

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	assert.Nil(t, cmd, "shortcut for unavailable agent must not emit a command")
	assert.Equal(t, stepAgentSelect, m.step, "step must not change for unavailable shortcut")
}

// TestAgentLauncherModal_ShortcutD_WhenAvailable_EmitsSpawnMsg verifies that pressing
// the Aider shortcut when Aider is available skips the prompt and emits SpawnAgentMsg.
func TestAgentLauncherModal_ShortcutD_WhenAvailable_EmitsSpawnMsg(t *testing.T) {
	opts := []agentOption{
		{name: "Claude Code", key: "a", internal: "claude", available: true},
		{name: "Aider", key: "d", internal: "aider", available: true},
	}
	m := newAgentLauncherModal(opts, testWorktreePath)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	require.NotNil(t, cmd)

	msg := cmd()
	spawnMsg, ok := msg.(SpawnAgentMsg)
	require.True(t, ok)
	assert.Equal(t, "aider", spawnMsg.AgentName)
}

// --- NewAgentLauncherModalWithPrompt ---

func TestNewAgentLauncherModalWithPrompt_PreFillsOnAgentSelect(t *testing.T) {
	cfg := &domain.Config{}
	m := NewAgentLauncherModalWithPrompt(cfg, testWorktreePath, "review this PR")

	require.NotNil(t, m)
	assert.Equal(t, "review this PR", m.initialPrompt)
	assert.Equal(t, stepAgentSelect, m.step, "should start on selection step")
}

func TestNewAgentLauncherModalWithPrompt_PreFillsPromptInput_OnClaudeSelect(t *testing.T) {
	opts := []agentOption{
		{name: "Claude Code", key: "a", internal: AgentNameClaude, available: true},
	}
	m := newAgentLauncherModal(opts, testWorktreePath)
	m.initialPrompt = "review the PR"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*AgentLauncherModal)

	require.NotNil(t, cmd, "Focus cmd should be returned")
	assert.Equal(t, stepAgentPrompt, m.step)
	assert.Equal(t, "review the PR", m.promptInput.Value(), "prompt input should be pre-filled with initialPrompt")
}

func TestNewAgentLauncherModalWithPrompt_PreFilledPrompt_SubmitsCorrectly(t *testing.T) {
	opts := []agentOption{
		{name: "Claude Code", key: "a", internal: AgentNameClaude, available: true},
	}
	m := newAgentLauncherModal(opts, testWorktreePath)
	m.initialPrompt = "review the PR"

	// Select Claude — advances to prompt step with pre-filled value.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*AgentLauncherModal)
	require.Equal(t, stepAgentPrompt, m.step)

	// Submit without modifying the prompt — should use pre-filled value.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	spawnMsg, ok := msg.(SpawnAgentMsg)
	require.True(t, ok)
	assert.Equal(t, "review the PR", spawnMsg.Prompt)
}
