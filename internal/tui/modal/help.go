package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/tui/styles"
	"github.com/m00nk0d3/nexus/internal/version"
)

type helpTab int

const (
	tabKeybindings    helpTab = iota
	tabTips                   // Quick Tips
	tabTroubleshooting        // Troubleshooting
	tabAbout                  // About
	helpTabCount              // sentinel
)

var helpTabLabels = [helpTabCount]string{
	"Keybindings",
	"Quick Tips",
	"Troubleshooting",
	"About",
}

// bindingGroup holds one section of the Keybindings tab.
type bindingGroup struct {
	title    string
	bindings [][2]string // [keys, description]
}

var keybindingGroups = []bindingGroup{
	{
		title: "NAVIGATION",
		bindings: [][2]string{
			{"↑/↓  j/k", "Navigate within panel"},
			{"←/→  h/l", "Switch between panels / tabs"},
			{"Tab", "Cycle panel focus / tab"},
			{"Enter", "Open selected item"},
		},
	},
	{
		title: "WORKTREE OPERATIONS",
		bindings: [][2]string{
			{"Enter (Issues view)", "Create worktree from selected issue"},
			{"Ctrl+D", "Delete selected worktree"},
			{"s", "Open shell in worktree"},
			{"x", "Close session and terminal tab"},
		},
	},
	{
		title: "AI AGENTS",
		bindings: [][2]string{
			{"a", "Spawn Claude Code"},
			{"c", "Spawn GitHub Copilot"},
			{"Space", "Unified agent launcher"},
		},
	},
	{
		title: "VIEWS",
		bindings: [][2]string{
			{"w / W", "Worktrees view"},
			{"i / I", "Issues view"},
			{"p / P", "PRs view"},
			{"t", "Cycle themes"},
		},
	},
	{
		title: "GLOBAL",
		bindings: [][2]string{
			{"f1 / ?", "Open this help modal"},
			{"g", "Open selected item in GitHub"},
			{"Esc / q / Ctrl+C", "Quit"},
		},
	},
}

// HelpModal is a Bubbletea model for the in-app help overlay (f1 / ?).
// It renders four tabs: Keybindings, Quick Tips, Troubleshooting, and About.
type HelpModal struct {
	activeTab    helpTab
	scrollOffset int
	theme        *styles.Theme
}

// NewHelpModal creates a new HelpModal starting on the Keybindings tab.
func NewHelpModal() *HelpModal {
	return &HelpModal{}
}

// SetTheme injects the current visual theme for styled rendering.
func (m *HelpModal) SetTheme(t styles.Theme) {
	m.theme = &t
}

// Init satisfies tea.Model.
func (m *HelpModal) Init() tea.Cmd { return nil }

// Title returns the modal title for themed overlay rendering.
func (m *HelpModal) Title() string { return "NEXUS HELP" }

// Update handles Bubbletea messages.
func (m *HelpModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }

	case tea.KeyTab:
		m.activeTab = (m.activeTab + 1) % helpTabCount
		m.scrollOffset = 0
		return m, nil

	case tea.KeyUp:
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
		return m, nil

	case tea.KeyDown:
		m.scrollOffset++
		return m, nil

	case tea.KeyRunes:
		switch keyMsg.String() {
		case "q":
			return m, func() tea.Msg { return ModalCancelledMsg{} }
		case "l":
			m.activeTab = (m.activeTab + 1) % helpTabCount
			m.scrollOffset = 0
		case "h":
			m.activeTab = (m.activeTab + helpTabCount - 1) % helpTabCount
			m.scrollOffset = 0
		case "j":
			m.scrollOffset++
		case "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "1":
			m.activeTab = tabKeybindings
			m.scrollOffset = 0
		case "2":
			m.activeTab = tabTips
			m.scrollOffset = 0
		case "3":
			m.activeTab = tabTroubleshooting
			m.scrollOffset = 0
		case "4":
			m.activeTab = tabAbout
			m.scrollOffset = 0
		}
	}

	return m, nil
}

// View renders the help modal content.
func (m *HelpModal) View() string {
	var b strings.Builder

	// Build tab bar styles.
	activeTabSt := lipgloss.NewStyle().Bold(true)
	inactiveTabSt := lipgloss.NewStyle()
	sepSt := lipgloss.NewStyle()
	footerSt := lipgloss.NewStyle()
	footerKeySt := lipgloss.NewStyle()

	if m.theme != nil {
		accent := lipgloss.Color(m.theme.Accent())
		muted := lipgloss.Color(m.theme.Muted())
		activeTabSt = activeTabSt.Foreground(accent).Underline(true)
		inactiveTabSt = inactiveTabSt.Foreground(muted)
		sepSt = sepSt.Foreground(muted)
		footerSt = footerSt.Foreground(muted)
		footerKeySt = footerKeySt.Foreground(accent)
	}

	// Tab bar.
	for i := helpTab(0); i < helpTabCount; i++ {
		label := " " + helpTabLabels[i] + " "
		if i == m.activeTab {
			b.WriteString(activeTabSt.Render(label))
		} else {
			b.WriteString(inactiveTabSt.Render(label))
		}
		if i < helpTabCount-1 {
			b.WriteString(sepSt.Render(" │ "))
		}
	}
	b.WriteString("\n\n")

	// Tab content (with scrolling applied).
	var content string
	switch m.activeTab {
	case tabKeybindings:
		content = m.renderKeybindings()
	case tabTips:
		content = m.styledContent(tipsContent)
	case tabTroubleshooting:
		content = m.styledContent(troubleshootingContent)
	case tabAbout:
		content = m.styledContent(aboutContent())
	}

	lines := strings.Split(content, "\n")
	start := m.scrollOffset
	if maxStart := len(lines) - 1; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	b.WriteString(strings.Join(lines[start:], "\n"))

	// Footer hints.
	b.WriteString("\n\n  ")
	b.WriteString(footerKeySt.Render("Tab/h/l"))
	b.WriteString(footerSt.Render(" switch tabs"))
	b.WriteString(footerSt.Render("   ·   "))
	b.WriteString(footerKeySt.Render("j/k"))
	b.WriteString(footerSt.Render(" scroll"))
	b.WriteString(footerSt.Render("   ·   "))
	b.WriteString(footerKeySt.Render("Esc/q"))
	b.WriteString(footerSt.Render(" close"))

	return b.String()
}

// renderKeybindings renders the Keybindings tab with aligned two-column layout.
func (m *HelpModal) renderKeybindings() string {
	sectionSt := lipgloss.NewStyle().Bold(true)
	keySt := lipgloss.NewStyle()
	descSt := lipgloss.NewStyle()

	if m.theme != nil {
		sectionSt = sectionSt.Foreground(lipgloss.Color(m.theme.Accent()))
		keySt = keySt.Foreground(lipgloss.Color(m.theme.Accent()))
		descSt = descSt.Foreground(lipgloss.Color(m.theme.Fg()))
	}

	// Compute max key width for column alignment.
	maxKey := 0
	for _, g := range keybindingGroups {
		for _, b := range g.bindings {
			if len(b[0]) > maxKey {
				maxKey = len(b[0])
			}
		}
	}

	var b strings.Builder
	for i, g := range keybindingGroups {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(sectionSt.Render(g.title))
		b.WriteString("\n")
		for _, binding := range g.bindings {
			keyPadded := fmt.Sprintf("%-*s", maxKey, binding[0])
			b.WriteString("  ")
			b.WriteString(keySt.Render(keyPadded))
			b.WriteString("  ")
			b.WriteString(descSt.Render(binding[1]))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// styledContent post-processes a plain-text content string, applying lipgloss
// styles for section headers, sub-headers, and label:value pairs.
func (m *HelpModal) styledContent(content string) string {
	if m.theme == nil {
		return content
	}

	sectionSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent())).Bold(true)
	subheaderSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Fg())).Bold(true)
	labelSt := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent()))

	lines := strings.Split(content, "\n")
	var b strings.Builder

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case len(trimmed) == 0:
			b.WriteString(line)

		case isAllCaps(trimmed):
			// ALL-CAPS → section header.
			b.WriteString(sectionSt.Render(line))

		case !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t"):
			// Non-indented mixed-case line → sub-header (bold title / numbered item).
			b.WriteString(subheaderSt.Render(line))

		case strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "•"):
			// Indented "Label: value" line (e.g. "  Symptom:", "  Fix:", "Version:").
			colonIdx := strings.Index(trimmed, ":")
			leadLen := len(line) - len(strings.TrimLeft(line, " \t"))
			indent := line[:leadLen]
			label := trimmed[:colonIdx+1]
			rest := trimmed[colonIdx+1:]
			b.WriteString(indent + labelSt.Render(label) + rest)

		default:
			b.WriteString(line)
		}

		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// isAllCaps reports whether s (trimmed) consists only of uppercase letters,
// digits, spaces, and punctuation — with at least one uppercase letter.
func isAllCaps(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) > 0 && s == strings.ToUpper(s) && strings.ContainsAny(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

const tipsContent = `COMMON WORKFLOWS

1. Create a worktree from a GitHub issue
   Open the Issues view, select an issue, then press Enter.
   Choose the branch type and confirm.
   Nexus creates the branch and worktree automatically.

2. Switch to a worktree and start coding
   Navigate to the worktree with ↑/↓, then press Enter or s.
   Your shell opens directly in the worktree directory.

3. Spawn an AI agent in the right context
   Select a worktree, then press a (Claude), c (Copilot),
   or Space to open the unified launcher. Nexus suspends
   itself and resumes when the agent exits.

4. Keep GitHub data fresh
   Nexus auto-syncs on startup and at the configured interval
   (~5 min default).

5. Open an issue or PR in the browser
   While in the Issues or PRs view, press g to open the
   selected item in your default browser via gh CLI.

6. Switch themes on the fly
   Press t to cycle through Digital Noir, Matrix, and Light.`

const troubleshootingContent = `COMMON ISSUES

GitHub authentication failure
  Symptom: "gh: not logged in" or empty issues/PRs list.
  Fix: Run  gh auth login  in your terminal and follow the
  prompts to authenticate with your GitHub account.

Agent not found (Claude / Copilot / Aider)
  Symptom: "binary not found" error when spawning an agent.
  Fix: Install the missing tool, or disable it in config:
    ~/.nexus/config.toml  →  ai_agents.claude_enabled = false
  For Claude: https://docs.anthropic.com/en/docs/claude-code
  For Copilot: gh extension install github/gh-copilot

git not in PATH
  Symptom: Nexus starts but shows no worktrees.
  Fix: Ensure git is installed and accessible in your PATH.
  Run  git --version  to verify.

Worktree list is stale
  Symptom: Deleted worktrees still appear.
  Fix: Run  git worktree prune  in your repo, then press r
  in Nexus to refresh.

Config not loading
  Symptom: Warning banner on startup.
  Fix: Check  ~/.nexus/config.toml  for TOML syntax errors.
  Delete or reset the file to restore defaults.`

func aboutContent() string {
	return fmt.Sprintf(`NEXUS — Git Worktree Orchestrator & AI Agent Hub

Version:   %s
Repo:      github.com/m00nk0d3/nexus
License:   MIT

Built with the Charm.sh ecosystem:
  • Bubbletea  — TUI framework
  • Lipgloss   — terminal styling
  • Bubbles    — UI components

Nexus helps you manage multiple git worktrees and
launch AI coding agents with the correct filesystem
context — all from a single terminal interface.`, version.Version)
}
