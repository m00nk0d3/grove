package modal

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// clearStatusMsg clears the settings status message after the tick fires.
type clearStatusMsg struct{}

// settingsField describes a single editable setting within a tab.
type settingsField struct {
	label   string
	kind    string   // "string" | "bool" | "choice"
	choices []string // non-nil for kind == "choice"
	get     func(*domain.Config) string
	set     func(*domain.Config, string)
}

// SettingsModal is a BubbleTea model for the in-TUI settings screen.
// It is displayed as a modal overlay and implements the Modal interface.
type SettingsModal struct {
	cfg        *domain.Config
	configPath string
	tabs       []string
	fields     [][]settingsField
	activeTab  int
	cursor     int
	editing    bool
	textInput  textinput.Model
	theme      styles.Theme
	width      int
	statusMsg  string
}

// NewSettingsModal creates and returns a new SettingsModal.
func NewSettingsModal(cfg *domain.Config, configPath string) *SettingsModal {
	appearanceFields := []settingsField{
		{
			label:   "Theme",
			kind:    "choice",
			choices: styles.Themes,
			get:     func(c *domain.Config) string { return c.Appearance.Theme },
			set:     func(c *domain.Config, v string) { c.Appearance.Theme = v },
		},
	}

	githubFields := []settingsField{
		{
			label: "AutoSync",
			kind:  "bool",
			get: func(c *domain.Config) string {
				if c.GitHub.AutoSync {
					return "true"
				}
				return "false"
			},
			set: func(c *domain.Config, v string) { c.GitHub.AutoSync = v == "true" },
		},
		{
			label: "SyncIntervalMinutes",
			kind:  "string",
			get:   func(c *domain.Config) string { return strconv.Itoa(c.GitHub.SyncIntervalMinutes) },
			set: func(c *domain.Config, v string) {
				if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
					c.GitHub.SyncIntervalMinutes = n
				}
			},
		},
	}

	aiFields := []settingsField{
		{
			label: "CopilotEnabled",
			kind:  "bool",
			get: func(c *domain.Config) string {
				if c.AIAgents.CopilotEnabled {
					return "true"
				}
				return "false"
			},
			set: func(c *domain.Config, v string) { c.AIAgents.CopilotEnabled = v == "true" },
		},
		{
			label: "ClaudeEnabled",
			kind:  "bool",
			get: func(c *domain.Config) string {
				if c.AIAgents.ClaudeEnabled {
					return "true"
				}
				return "false"
			},
			set: func(c *domain.Config, v string) { c.AIAgents.ClaudeEnabled = v == "true" },
		},
		{
			label: "AiderEnabled",
			kind:  "bool",
			get: func(c *domain.Config) string {
				if c.AIAgents.AiderEnabled {
					return "true"
				}
				return "false"
			},
			set: func(c *domain.Config, v string) { c.AIAgents.AiderEnabled = v == "true" },
		},
		{
			label: "ClaudeBinary",
			kind:  "string",
			get:   func(c *domain.Config) string { return c.AIAgents.ClaudeBinary },
			set:   func(c *domain.Config, v string) { c.AIAgents.ClaudeBinary = v },
		},
		{
			label: "AiderBinary",
			kind:  "string",
			get:   func(c *domain.Config) string { return c.AIAgents.AiderBinary },
			set:   func(c *domain.Config, v string) { c.AIAgents.AiderBinary = v },
		},
	}

	worktreeFields := []settingsField{
		{
			label: "BaseBranch",
			kind:  "string",
			get:   func(c *domain.Config) string { return c.Worktrees.BaseBranch },
			set:   func(c *domain.Config, v string) { c.Worktrees.BaseBranch = v },
		},
		{
			label: "WorktreeRoot",
			kind:  "string",
			get:   func(c *domain.Config) string { return c.Worktrees.WorktreeRoot },
			set:   func(c *domain.Config, v string) { c.Worktrees.WorktreeRoot = v },
		},
	}

	return &SettingsModal{
		cfg:        cfg,
		configPath: configPath,
		tabs:       []string{"Appearance", "GitHub", "AI Agents", "Worktrees"},
		fields:     [][]settingsField{appearanceFields, githubFields, aiFields, worktreeFields},
		theme:      styles.NewTheme("digital-noir"),
	}
}

// Title satisfies the Modal interface.
func (m *SettingsModal) Title() string { return "SETTINGS" }

// SetWidth stores the terminal width for rendering.
func (m *SettingsModal) SetWidth(w int) { m.width = w }

// SetTheme stores the visual theme for rendering.
func (m *SettingsModal) SetTheme(t styles.Theme) { m.theme = t }

// Init satisfies tea.Model. No initial command needed.
func (m *SettingsModal) Init() tea.Cmd { return nil }

// Update handles all BubbleTea messages for the settings modal.
func (m *SettingsModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey processes keyboard input.
func (m *SettingsModal) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── Editing mode: route most keys to the textinput ──────────────────────
	if m.editing {
		switch msg.Type {
		case tea.KeyEnter:
			// Confirm the edit.
			field := m.currentField()
			newVal := strings.TrimSpace(m.textInput.Value())
			field.set(m.cfg, newVal)
			m.editing = false
			return m, m.saveAndNotify()

		case tea.KeyEsc:
			// Cancel the edit — discard the in-progress value.
			m.editing = false
			return m, nil

		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	// ── Normal mode ──────────────────────────────────────────────────────────
	switch msg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }

	case tea.KeyTab:
		m.activeTab = (m.activeTab + 1) % len(m.tabs)
		m.cursor = 0
		return m, nil

	case tea.KeyShiftTab:
		m.activeTab = (m.activeTab + len(m.tabs) - 1) % len(m.tabs)
		m.cursor = 0
		return m, nil

	case tea.KeyRight:
		if m.currentField().kind == "choice" {
			return m, m.cycleChoiceNext()
		}
		m.activeTab = (m.activeTab + 1) % len(m.tabs)
		m.cursor = 0
		return m, nil

	case tea.KeyLeft:
		if m.currentField().kind == "choice" {
			return m, m.cycleChoicePrev()
		}
		m.activeTab = (m.activeTab + len(m.tabs) - 1) % len(m.tabs)
		m.cursor = 0
		return m, nil

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case tea.KeyDown:
		max := len(m.fields[m.activeTab]) - 1
		if m.cursor < max {
			m.cursor++
		}
		return m, nil

	case tea.KeyEnter:
		return m, m.activateField()

	case tea.KeySpace:
		return m, m.toggleBool()

	case tea.KeyRunes:
		switch msg.String() {
		case "j":
			max := len(m.fields[m.activeTab]) - 1
			if m.cursor < max {
				m.cursor++
			}
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil
	}

	return m, nil
}

// currentField returns the currently selected settingsField.
func (m *SettingsModal) currentField() settingsField {
	tab := m.fields[m.activeTab]
	if m.cursor < 0 || m.cursor >= len(tab) {
		return tab[0]
	}
	return tab[m.cursor]
}

// activateField handles Enter on the currently selected field.
// For bool and choice fields it updates immediately; for string fields it opens
// the textinput for inline editing.
func (m *SettingsModal) activateField() tea.Cmd {
	field := m.currentField()
	switch field.kind {
	case "bool":
		return m.toggleBool()
	case "choice":
		return m.cycleChoiceNext()
	case "string":
		m.editing = true
		ti := textinput.New()
		ti.SetValue(field.get(m.cfg))
		// Place cursor at end.
		ti.CursorEnd()
		_ = ti.Focus()
		m.textInput = ti
		return nil
	}
	return nil
}

// toggleBool flips the current bool field value and saves.
func (m *SettingsModal) toggleBool() tea.Cmd {
	field := m.currentField()
	if field.kind != "bool" {
		return nil
	}
	current := field.get(m.cfg) == "true"
	if current {
		field.set(m.cfg, "false")
	} else {
		field.set(m.cfg, "true")
	}
	return m.saveAndNotify()
}

// cycleChoiceNext advances the current choice field to the next option and saves.
func (m *SettingsModal) cycleChoiceNext() tea.Cmd {
	field := m.currentField()
	if field.kind != "choice" || len(field.choices) == 0 {
		return nil
	}
	current := field.get(m.cfg)
	nextIdx := 0
	for i, c := range field.choices {
		if c == current {
			nextIdx = (i + 1) % len(field.choices)
			break
		}
	}
	field.set(m.cfg, field.choices[nextIdx])
	return m.saveAndNotify()
}

// cycleChoicePrev moves the current choice field to the previous option and saves.
func (m *SettingsModal) cycleChoicePrev() tea.Cmd {
	field := m.currentField()
	if field.kind != "choice" || len(field.choices) == 0 {
		return nil
	}
	current := field.get(m.cfg)
	prevIdx := len(field.choices) - 1
	for i, c := range field.choices {
		if c == current {
			prevIdx = (i - 1 + len(field.choices)) % len(field.choices)
			break
		}
	}
	field.set(m.cfg, field.choices[prevIdx])
	return m.saveAndNotify()
}

// saveAndNotifysaves the config to disk, sets a success status message, and
// returns a batch of commands: a SettingsSavedMsg dispatch and a 3-second
// status-clear tick.
func (m *SettingsModal) saveAndNotify() tea.Cmd {
	if err := data.SaveConfig(m.cfg, m.configPath); err != nil {
		m.statusMsg = fmt.Sprintf("✗ Save failed: %v", err)
		return nil
	}
	m.statusMsg = "✓ Settings saved"
	saved := m.cfg
	return tea.Batch(
		func() tea.Msg { return SettingsSavedMsg{Config: saved} },
		tea.Tick(3*time.Second, func(_ time.Time) tea.Msg { return clearStatusMsg{} }),
	)
}

// View renders the settings modal content.
func (m *SettingsModal) View() string {
	var b strings.Builder

	accentSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent())).Bold(true).Underline(true)
	mutedSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted()))
	sepSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted()))
	selectedRowSt := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Accent())).
		Foreground(lipgloss.Color(m.theme.Bg())).
		Bold(true)
	labelSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Fg()))
	valueSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent()))

	// Tab bar.
	for i, tab := range m.tabs {
		label := " " + tab + " "
		if i == m.activeTab {
			b.WriteString(accentSt.Render(label))
		} else {
			b.WriteString(mutedSt.Render(label))
		}
		if i < len(m.tabs)-1 {
			b.WriteString(sepSt.Render(" │ "))
		}
	}
	b.WriteString("\n\n")

	// Field list for the active tab.
	fields := m.fields[m.activeTab]
	// Compute label column width.
	maxLabel := 0
	for _, f := range fields {
		if len(f.label) > maxLabel {
			maxLabel = len(f.label)
		}
	}

	for i, f := range fields {
		selected := i == m.cursor
		labelPadded := fmt.Sprintf("  %-*s", maxLabel+2, f.label)
		var valueStr string

		if selected && m.editing && f.kind == "string" {
			valueStr = m.textInput.View()
		} else {
			valueStr = m.renderFieldValue(f)
		}

		row := fmt.Sprintf("%s  %s", labelPadded, valueStr)
		if selected {
			b.WriteString(selectedRowSt.Render(row))
		} else {
			b.WriteString(labelSt.Render(labelPadded) + "  " + valueSt.Render(valueStr))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Status / hint bar.
	if m.statusMsg != "" {
		successSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Success()))
		b.WriteString("  " + successSt.Render(m.statusMsg))
	} else {
		hintSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted()))
		keySt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent()))
		b.WriteString("  ")
		if m.currentField().kind == "choice" {
			b.WriteString(keySt.Render("← →"))
			b.WriteString(hintSt.Render(" cycle  "))
		} else {
			b.WriteString(keySt.Render("Enter"))
			b.WriteString(hintSt.Render(" edit  "))
			b.WriteString(keySt.Render("Space"))
			b.WriteString(hintSt.Render(" toggle  "))
		}
		b.WriteString(keySt.Render("Tab"))
		b.WriteString(hintSt.Render(" tab  "))
		b.WriteString(keySt.Render("Esc"))
		b.WriteString(hintSt.Render(" back"))
	}

	return b.String()
}

// renderFieldValue returns the display string for a field's current value.
func (m *SettingsModal) renderFieldValue(f settingsField) string {
	val := f.get(m.cfg)
	switch f.kind {
	case "bool":
		if val == "true" {
			return "[✓]"
		}
		return "[ ]"
	case "choice":
		return fmt.Sprintf("◀ %s ▶", val)
	default:
		return val
	}
}
