package modal

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/domain"
)

type agentStep int

const (
	stepAgentSelect agentStep = iota
	stepAgentPrompt
)

type agentOption struct {
	name      string // Display name (e.g. "Claude Code")
	key       string // Keyboard shortcut shown in UI (e.g. "a")
	internal  string // Internal identifier ("claude", "copilot", "aider")
	available bool   // Whether the agent can be used right now
}

// AgentLauncherModal is a Bubbletea model for the unified agent launcher overlay.
// Step 1: shows all three agents with availability status.
// Step 2: inline prompt input (for Claude/Copilot).
// Aider skips the prompt and emits SpawnAgentMsg directly (file picker is handled by the app).
type AgentLauncherModal struct {
	step          agentStep
	options       []agentOption
	selectedIdx   int
	worktreePath  string
	promptInput   textinput.Model
	selectedAgent agentOption
	initialPrompt string // pre-seeded prompt text; pre-fills input when advancing to prompt step
	width         int    // terminal width, set by the parent view
}

// newAgentLauncherModal is the internal constructor used in production and in tests.
func newAgentLauncherModal(options []agentOption, worktreePath string) *AgentLauncherModal {
	ti := textinput.New()
	ti.CharLimit = 200

	// Start cursor on the first available agent so Enter works immediately.
	startIdx := 0
	for i, opt := range options {
		if opt.available {
			startIdx = i
			break
		}
	}

	return &AgentLauncherModal{
		step:         stepAgentSelect,
		options:      options,
		selectedIdx:  startIdx,
		worktreePath: worktreePath,
		promptInput:  ti,
	}
}

// buildAgentOptions returns the three fixed agent slots with their current availability.
// Availability = enabled in config AND binary found on PATH.
func buildAgentOptions(cfg *domain.Config) []agentOption {
	claudeAvail := cfg.AIAgents.ClaudeEnabled
	if claudeAvail {
		bin := cfg.AIAgents.ClaudeBinary
		if bin == "" {
			bin = "claude"
		}
		_, err := exec.LookPath(bin)
		claudeAvail = err == nil
	}

	copilotAvail := cfg.AIAgents.CopilotEnabled
	if copilotAvail {
		_, err := exec.LookPath("gh")
		copilotAvail = err == nil
	}

	aiderAvail := cfg.AIAgents.AiderEnabled
	if aiderAvail {
		_, err := exec.LookPath("aider")
		aiderAvail = err == nil
	}

	return []agentOption{
		{name: "Claude Code", key: "a", internal: AgentNameClaude, available: claudeAvail},
		{name: "Copilot CLI", key: "c", internal: AgentNameCopilot, available: copilotAvail},
		{name: "Aider", key: "d", internal: AgentNameAider, available: aiderAvail},
	}
}

// NewAgentLauncherModal creates an AgentLauncherModal with availability computed from config.
func NewAgentLauncherModal(cfg *domain.Config, worktreePath string) *AgentLauncherModal {
	return newAgentLauncherModal(buildAgentOptions(cfg), worktreePath)
}

// NewAgentLauncherModalWithPrompt creates an AgentLauncherModal with a pre-seeded prompt.
// When the user selects an agent that uses the prompt step (Claude or Copilot),
// the prompt input will be pre-filled with initialPrompt so the user can review/edit before confirming.
func NewAgentLauncherModalWithPrompt(cfg *domain.Config, worktreePath, initialPrompt string) *AgentLauncherModal {
	m := newAgentLauncherModal(buildAgentOptions(cfg), worktreePath)
	m.initialPrompt = initialPrompt
	return m
}

// Init satisfies tea.Model.
func (m *AgentLauncherModal) Init() tea.Cmd { return nil }

// Update handles Bubbletea messages for the agent launcher.
func (m *AgentLauncherModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)

	// Non-key messages are forwarded to the text input when in prompt step.
	if !isKey {
		if m.step == stepAgentPrompt {
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Esc always cancels regardless of step.
	if keyMsg.Type == tea.KeyEsc {
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	if m.step == stepAgentPrompt {
		return m.updatePromptStep(keyMsg)
	}

	return m.updateSelectStep(keyMsg)
}

func (m *AgentLauncherModal) updatePromptStep(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keyMsg.Type == tea.KeyEnter {
		prompt := strings.TrimSpace(m.promptInput.Value())
		selected := m.selectedAgent
		path := m.worktreePath
		return m, func() tea.Msg {
			return SpawnAgentMsg{
				AgentName:    selected.internal,
				WorktreePath: path,
				Prompt:       prompt,
			}
		}
	}

	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(keyMsg)
	return m, cmd
}

func (m *AgentLauncherModal) updateSelectStep(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "down", "j":
		if m.selectedIdx < len(m.options)-1 {
			m.selectedIdx++
		}
	case "enter":
		if m.selectedIdx < len(m.options) {
			return m.selectAgent(m.options[m.selectedIdx])
		}
	default:
		// Jump-select by shortcut key (e.g. "a" for Claude, "c" for Copilot).
		for _, opt := range m.options {
			if opt.key == keyMsg.String() {
				return m.selectAgent(opt)
			}
		}
	}
	return m, nil
}

func (m *AgentLauncherModal) selectAgent(opt agentOption) (tea.Model, tea.Cmd) {
	if !opt.available {
		return m, nil
	}
	m.selectedAgent = opt

	// Aider uses a file picker flow; emit immediately and let the app handle it.
	if opt.internal == AgentNameAider {
		path := m.worktreePath
		return m, func() tea.Msg {
			return SpawnAgentMsg{AgentName: AgentNameAider, WorktreePath: path}
		}
	}

	// Claude and Copilot: advance to the inline prompt step.
	m.promptInput.Placeholder = fmt.Sprintf("Enter %s prompt…", opt.name)
	m.promptInput.SetValue("")
	if m.initialPrompt != "" {
		m.promptInput.SetValue(m.initialPrompt)
		m.promptInput.CursorEnd()
	}
	focusCmd := m.promptInput.Focus()
	m.step = stepAgentPrompt
	return m, focusCmd
}

// SetWidth receives the terminal width from the parent view so the text input
// can span the full inner content area of the popup box (total width minus the
// 4-column border+padding overhead added by RenderBox).
func (m *AgentLauncherModal) SetWidth(w int) {
	m.width = w
	inner := w - 4
	if inner < 20 {
		inner = 20
	}
	m.promptInput.Width = inner
}

// Title returns the modal title for themed overlay rendering.
func (m *AgentLauncherModal) Title() string { return "SPAWN AGENT" }

// View renders the current state of the modal.
func (m *AgentLauncherModal) View() string {
	if m.step == stepAgentPrompt {
		return m.viewPrompt()
	}
	return m.viewAgentList()
}

func (m *AgentLauncherModal) viewAgentList() string {
	var b strings.Builder
	b.WriteString("Select an agent to spawn in:\n\n")

	for i, opt := range m.options {
		cursor := "  "
		if i == m.selectedIdx {
			cursor = "> "
		}
		status := "● available"
		if !opt.available {
			status = "✗ not configured"
		}
		b.WriteString(fmt.Sprintf("%s[%s] %-14s %s\n", cursor, opt.key, opt.name, status))
	}

	b.WriteString("\n↑/↓ navigate  •  Enter select  •  Esc cancel")
	return b.String()
}

func (m *AgentLauncherModal) viewPrompt() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Spawning %s in:\n%s\n\n", m.selectedAgent.name, m.worktreePath))
	b.WriteString(m.promptInput.View())
	b.WriteString("\n\nEnter confirm (prompt optional)  •  Esc cancel")
	return b.String()
}
