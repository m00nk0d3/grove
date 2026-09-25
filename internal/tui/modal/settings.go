package modal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// clearStatusMsg clears the settings status line once its tick fires. seq
// identifies the status it was scheduled for, so an older tick never clears a
// newer message.
type clearStatusMsg struct{ seq int }

// fieldKind selects how a setting is displayed and edited.
type fieldKind int

const (
	fieldBool fieldKind = iota
	fieldText
	fieldNumber
	fieldChoice
)

// settingsField describes a single editable setting within a section.
type settingsField struct {
	label   string
	key     string // the setting's path in config.toml
	help    string
	kind    fieldKind
	unit    string   // suffix shown after a fieldNumber value
	choices []string // options for fieldChoice
	get     func(*domain.Config) string
	set     func(*domain.Config, string) error
}

// settingsSection is one entry in the settings rail. The appearance section
// has no fields: its body is the theme list.
type settingsSection struct {
	title  string
	blurb  string
	fields []settingsField
}

const appearanceSection = 0

// SettingsModal is a BubbleTea model for the in-TUI settings screen. It is
// displayed fullscreen and implements the Modal interface.
type SettingsModal struct {
	cfg           *domain.Config
	configPath    string
	sections      []settingsSection
	activeSection int
	cursors       []int // selected row per section; the theme index for appearance
	editing       bool
	textInput     textinput.Model
	theme         styles.Theme
	width         int
	height        int
	statusMsg     string
	statusErr     bool
	statusSeq     int
}

// NewSettingsModal creates and returns a new SettingsModal.
func NewSettingsModal(cfg *domain.Config, configPath string) *SettingsModal {
	sections := []settingsSection{
		{
			title: "APPEARANCE",
			blurb: "interface theme",
		},
		{
			title: "GITHUB",
			blurb: "issue and pull request sync",
			fields: []settingsField{
				boolField("Auto sync", "github.auto_sync",
					"Refresh issues and pull requests from GitHub in the background. When off, they refresh only at startup and when you press r.",
					func(c *domain.Config) *bool { return &c.GitHub.AutoSync }),
				numberField("Sync interval", "github.sync_interval_minutes", "min",
					"Minutes between background refreshes of GitHub data.",
					func(c *domain.Config) *int { return &c.GitHub.SyncIntervalMinutes }),
			},
		},
		{
			title: "WORKTREES",
			blurb: "where new worktrees are created",
			fields: []settingsField{
				textField("Base branch", "worktrees.base_branch",
					"Branch a new worktree starts from when no parent branch is picked. When the repository has no such branch, its default branch is used.",
					func(c *domain.Config) *string { return &c.Worktrees.BaseBranch }),
				textField("Worktree root", "worktrees.worktree_root",
					"Directory new worktrees are created in, with one folder per repository. A relative path starts at the repository root.",
					func(c *domain.Config) *string { return &c.Worktrees.WorktreeRoot }),
			},
		},
		{
			title: "AGENTS",
			blurb: "herdr panes and the sandcastle runtime",
			fields: []settingsField{
				{
					label:   "Default agent",
					key:     "sandcastle.default_agent",
					help:    "Agent that runs workflow specialists when a workflow does not name one.",
					kind:    fieldChoice,
					choices: []string{"opencode", "pi", "claude"},
					get:     func(c *domain.Config) string { return c.Sandcastle.DefaultAgent },
					set: func(c *domain.Config, v string) error {
						c.Sandcastle.DefaultAgent = v
						return nil
					},
				},
				boolField("Sandcastle runtime", "sandcastle.enabled",
					"Track and start workflows through the Sandcastle runtime.",
					func(c *domain.Config) *bool { return &c.Sandcastle.Enabled }),
				boolField("Herdr integration", "herdr.enabled",
					"Open worktrees and agents in Herdr panes and report their state.",
					func(c *domain.Config) *bool { return &c.Herdr.Enabled }),
				boolField("Herdr worktree API", "herdr.prefer_worktree_api",
					"Create worktrees through Herdr when Grove runs inside it.",
					func(c *domain.Config) *bool { return &c.Herdr.PreferWorktreeAPI }),
				numberField("Poll interval", "herdr.poll_interval_seconds", "s",
					"Seconds between checks of Herdr and Sandcastle status.",
					func(c *domain.Config) *int { return &c.Herdr.PollIntervalSeconds }),
			},
		},
	}

	cursors := make([]int, len(sections))
	cursors[appearanceSection] = themeIndex(cfg.Appearance.Theme)

	return &SettingsModal{
		cfg:        cfg,
		configPath: configPath,
		sections:   sections,
		cursors:    cursors,
		theme:      styles.NewTheme(cfg.Appearance.Theme),
	}
}

func boolField(label, key, help string, ptr func(*domain.Config) *bool) settingsField {
	return settingsField{
		label: label, key: key, help: help, kind: fieldBool,
		get: func(c *domain.Config) string { return strconv.FormatBool(*ptr(c)) },
		set: func(c *domain.Config, v string) error {
			*ptr(c) = v == "true"
			return nil
		},
	}
}

func textField(label, key, help string, ptr func(*domain.Config) *string) settingsField {
	return settingsField{
		label: label, key: key, help: help, kind: fieldText,
		get: func(c *domain.Config) string { return *ptr(c) },
		set: func(c *domain.Config, v string) error {
			if v == "" {
				return errors.New("value cannot be empty")
			}
			*ptr(c) = v
			return nil
		},
	}
}

func numberField(label, key, unit, help string, ptr func(*domain.Config) *int) settingsField {
	return settingsField{
		label: label, key: key, unit: unit, help: help, kind: fieldNumber,
		get: func(c *domain.Config) string { return strconv.Itoa(*ptr(c)) },
		set: func(c *domain.Config, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return errors.New("enter a whole number of at least 1")
			}
			*ptr(c) = n
			return nil
		},
	}
}

// themeIndex returns the position of name in styles.Themes, or 0 (the default
// theme) when it is not a known theme.
func themeIndex(name string) int {
	for i, t := range styles.Themes {
		if t == name {
			return i
		}
	}
	return 0
}

// Title satisfies the Modal interface.
func (m *SettingsModal) Title() string { return "◈ SETTINGS // SYSTEM CONFIGURATION" }

// Fullscreen asks the app to render the settings screen across the terminal.
func (m *SettingsModal) Fullscreen() bool { return true }

// SetWidth stores the terminal width for rendering.
func (m *SettingsModal) SetWidth(w int) { m.width = w }

// SetHeight stores the terminal height for rendering.
func (m *SettingsModal) SetHeight(h int) { m.height = h }

// SetTheme stores the visual theme for rendering.
func (m *SettingsModal) SetTheme(t styles.Theme) { m.theme = t }

// Init satisfies tea.Model. No initial command needed.
func (m *SettingsModal) Init() tea.Cmd { return nil }

// Update handles all BubbleTea messages for the settings modal.
func (m *SettingsModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.statusMsg = ""
			m.statusErr = false
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey processes keyboard input.
func (m *SettingsModal) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		switch msg.Type {
		case tea.KeyEnter:
			field := m.currentField()
			if err := field.set(m.cfg, strings.TrimSpace(m.textInput.Value())); err != nil {
				// Keep the editor open so the value can be corrected.
				return m, m.setStatus("✗ "+field.label+": "+err.Error(), true)
			}
			m.editing = false
			return m, m.saveAndNotify(field.key)
		case tea.KeyEsc:
			m.editing = false
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	case tea.KeyTab:
		m.switchSection(1)
	case tea.KeyShiftTab:
		m.switchSection(-1)
	case tea.KeyUp:
		m.moveCursor(-1)
	case tea.KeyDown:
		m.moveCursor(1)
	case tea.KeyHome:
		m.cursors[m.activeSection] = 0
	case tea.KeyEnd:
		m.cursors[m.activeSection] = m.rowCount() - 1
	case tea.KeyRight:
		if m.hasChoiceSelected() {
			return m, m.cycleChoice(1)
		}
		m.switchSection(1)
	case tea.KeyLeft:
		if m.hasChoiceSelected() {
			return m, m.cycleChoice(-1)
		}
		m.switchSection(-1)
	case tea.KeyEnter:
		return m, m.activate()
	case tea.KeySpace:
		if m.activeSection == appearanceSection {
			return m, m.applyTheme()
		}
		if f, ok := m.selectedField(); ok && f.kind == fieldBool {
			return m, m.toggleBool()
		}
	case tea.KeyRunes:
		switch msg.String() {
		case "j":
			m.moveCursor(1)
		case "k":
			m.moveCursor(-1)
		case "h":
			if m.hasChoiceSelected() {
				return m, m.cycleChoice(-1)
			}
		case "l":
			if m.hasChoiceSelected() {
				return m, m.cycleChoice(1)
			}
		case "g":
			m.cursors[m.activeSection] = 0
		case "G":
			m.cursors[m.activeSection] = m.rowCount() - 1
		case "q":
			return m, func() tea.Msg { return ModalCancelledMsg{} }
		default:
			if s := msg.String(); len(s) == 1 && s[0] >= '1' && int(s[0]-'1') < len(m.sections) {
				m.activeSection = int(s[0] - '1')
			}
		}
	}
	return m, nil
}

func (m *SettingsModal) switchSection(delta int) {
	n := len(m.sections)
	m.activeSection = (m.activeSection + delta + n) % n
}

// rowCount is the number of selectable rows in the active section.
func (m *SettingsModal) rowCount() int {
	if m.activeSection == appearanceSection {
		return len(styles.Themes)
	}
	return len(m.sections[m.activeSection].fields)
}

func (m *SettingsModal) moveCursor(delta int) {
	c := m.cursors[m.activeSection] + delta
	if c < 0 {
		c = 0
	}
	if last := m.rowCount() - 1; c > last {
		c = last
	}
	m.cursors[m.activeSection] = c
}

// selectedField returns the field under the cursor. The appearance section has
// no fields, so ok is false there.
func (m *SettingsModal) selectedField() (settingsField, bool) {
	fields := m.sections[m.activeSection].fields
	c := m.cursors[m.activeSection]
	if c < 0 || c >= len(fields) {
		return settingsField{}, false
	}
	return fields[c], true
}

// currentField returns the field under the cursor for code paths that have
// already established one is selected.
func (m *SettingsModal) currentField() settingsField {
	f, _ := m.selectedField()
	return f
}

func (m *SettingsModal) hasChoiceSelected() bool {
	f, ok := m.selectedField()
	return ok && f.kind == fieldChoice
}

// hoveredTheme is the theme under the cursor in the appearance list, which the
// preview shows before it is applied.
func (m *SettingsModal) hoveredTheme() styles.Theme {
	return styles.NewTheme(styles.Themes[m.cursors[appearanceSection]])
}

// activate handles Enter: apply the hovered theme, toggle a switch, advance a
// choice, or open the editor for a text or number field.
func (m *SettingsModal) activate() tea.Cmd {
	if m.activeSection == appearanceSection {
		return m.applyTheme()
	}
	field, ok := m.selectedField()
	if !ok {
		return nil
	}
	switch field.kind {
	case fieldBool:
		return m.toggleBool()
	case fieldChoice:
		return m.cycleChoice(1)
	default:
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(field.get(m.cfg))
		ti.CursorEnd()
		_ = ti.Focus()
		m.textInput = ti
		m.editing = true
		return nil
	}
}

func (m *SettingsModal) applyTheme() tea.Cmd {
	name := styles.Themes[m.cursors[appearanceSection]]
	if m.cfg.Appearance.Theme == name {
		return m.setStatus("● "+styles.NewTheme(name).Label()+" is already active", false)
	}
	m.cfg.Appearance.Theme = name
	return m.saveAndNotify("appearance.theme")
}

func (m *SettingsModal) toggleBool() tea.Cmd {
	field := m.currentField()
	next := "true"
	if field.get(m.cfg) == "true" {
		next = "false"
	}
	_ = field.set(m.cfg, next)
	return m.saveAndNotify(field.key)
}

func (m *SettingsModal) cycleChoice(delta int) tea.Cmd {
	field := m.currentField()
	if len(field.choices) == 0 {
		return nil
	}
	idx := -1
	for i, c := range field.choices {
		if c == field.get(m.cfg) {
			idx = i
			break
		}
	}
	n := len(field.choices)
	switch {
	case idx < 0 && delta > 0:
		idx = 0
	case idx < 0:
		idx = n - 1
	default:
		idx = (idx + delta + n) % n
	}
	_ = field.set(m.cfg, field.choices[idx])
	return m.saveAndNotify(field.key)
}

// setStatus shows msg on the status line and schedules it to clear.
func (m *SettingsModal) setStatus(msg string, isErr bool) tea.Cmd {
	m.statusSeq++
	seq := m.statusSeq
	m.statusMsg = msg
	m.statusErr = isErr
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearStatusMsg{seq: seq} })
}

// saveAndNotify writes the config to disk, reports the result on the status
// line, and dispatches SettingsSavedMsg so the app applies the change.
func (m *SettingsModal) saveAndNotify(key string) tea.Cmd {
	if err := data.SaveConfig(m.cfg, m.configPath); err != nil {
		return m.setStatus(fmt.Sprintf("✗ Save failed: %v", err), true)
	}
	saved := m.cfg
	return tea.Batch(
		func() tea.Msg { return SettingsSavedMsg{Config: saved} },
		m.setStatus("✓ Saved "+key, false),
	)
}

// ── Rendering ────────────────────────────────────────────────────────────────

const (
	settingsRailWidth    = 24
	settingsMinRailWidth = 64 // below this the rail collapses into a strip
	settingsListWidth    = 40
	settingsMinPreview   = 30
)

// layout reports the columns and rows available to View. The app draws a
// fullscreen modal inside a double border with one column of padding on each
// side and a title row, on a terminal of at least 64x20; below that it falls
// back to a centred box, which sizes to its content.
func (m *SettingsModal) layout() (width, rows int) {
	if m.width >= 64 && m.height >= 20 {
		return m.width - 10, m.height - 5
	}
	if m.width > 6 {
		return m.width - 6, 0
	}
	return 80, 0
}

type settingsPalette struct {
	accent, muted, fg, success, warning, danger lipgloss.Style
	bold                                        lipgloss.Style
}

func newSettingsPalette(t styles.Theme) settingsPalette {
	c := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	return settingsPalette{
		accent:  c(t.Accent()),
		muted:   c(t.Muted()),
		fg:      c(t.Fg()),
		success: c(t.Success()),
		warning: c(t.Warning()),
		danger:  c(t.Danger()),
		bold:    c(t.Accent()).Bold(true),
	}
}

// View renders the settings screen.
func (m *SettingsModal) View() string {
	width, rows := m.layout()
	p := newSettingsPalette(m.theme)

	meta := p.muted.Render(ansi.Truncate(fmt.Sprintf("config %s  •  %d themes  •  changes save instantly", displayPath(m.configPath), len(styles.Themes)), width, "…"))
	rule := p.muted.Render(strings.Repeat("─", max(width, 1)))

	bodyRows := 0
	if rows > 0 {
		bodyRows = max(rows-4, 6)
	}

	var body string
	if width >= settingsMinRailWidth {
		mainWidth := width - settingsRailWidth - 3
		rail := m.renderRail(p, bodyRows)
		main := m.renderSection(p, mainWidth, bodyRows)
		sep := p.muted.Render(strings.TrimSuffix(strings.Repeat("│\n", max(lipgloss.Height(rail), lipgloss.Height(main))), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, fixedBlock(rail, settingsRailWidth, bodyRows), " ", sep, " ", fixedBlock(main, mainWidth, bodyRows))
	} else {
		strip := m.renderStrip(p, width)
		main := m.renderSection(p, width, max(bodyRows-2, 0))
		body = fixedBlock(strip+"\n\n"+main, width, bodyRows)
	}

	return strings.Join([]string{meta, rule, body, rule, m.renderFooter(p, width)}, "\n")
}

// renderRail draws the section list down the left of the screen, with the
// active theme's name and swatch below it.
func (m *SettingsModal) renderRail(p settingsPalette, rows int) string {
	var b strings.Builder
	b.WriteString(p.muted.Render("SECTIONS"))
	b.WriteString("\n\n")
	for i, s := range m.sections {
		label := fmt.Sprintf("%02d  %s", i+1, s.title)
		if i == m.activeSection {
			b.WriteString(p.bold.Render("▸ " + label))
		} else {
			b.WriteString(p.fg.Render("  " + label))
		}
		b.WriteString("\n")
	}
	active := styles.NewTheme(m.cfg.Appearance.Theme)
	footer := p.muted.Render("ACTIVE THEME") + "\n" + p.fg.Render(active.Label()) + "\n" + swatch(active)
	content := strings.TrimRight(b.String(), "\n")
	if rows > 0 {
		gap := rows - lipgloss.Height(content) - lipgloss.Height(footer)
		if gap >= 1 {
			return content + strings.Repeat("\n", gap+1) + footer
		}
		return content
	}
	return content + "\n\n" + footer
}

// renderStrip draws the sections as one row for a terminal too narrow for the
// rail.
func (m *SettingsModal) renderStrip(p settingsPalette, width int) string {
	parts := make([]string, len(m.sections))
	for i, s := range m.sections {
		label := fmt.Sprintf("%d %s", i+1, s.title)
		if i == m.activeSection {
			parts[i] = p.bold.Render("[" + label + "]")
		} else {
			parts[i] = p.muted.Render(" " + label + " ")
		}
	}
	return ansi.Truncate(strings.Join(parts, " "), width, "…")
}

// renderSection draws the active section's heading and body.
func (m *SettingsModal) renderSection(p settingsPalette, width, rows int) string {
	s := m.sections[m.activeSection]
	heading := p.bold.Render("◆ "+s.title) + p.muted.Render("  //  "+s.blurb)
	bodyRows := 0
	if rows > 0 {
		bodyRows = max(rows-2, 3)
	}
	var body string
	if m.activeSection == appearanceSection {
		body = m.renderThemes(p, width, bodyRows)
	} else {
		body = m.renderFields(p, width, bodyRows)
	}
	return ansi.Truncate(heading, width, "…") + "\n\n" + body
}

// renderThemes draws the theme list, grouped into dark and light, beside a
// preview of the theme under the cursor.
func (m *SettingsModal) renderThemes(p settingsPalette, width, rows int) string {
	listWidth := min(settingsListWidth, width)
	var lines []string
	cursorLine := 0
	group := func(title string, count int) {
		label := fmt.Sprintf("%s %02d ", title, count)
		lines = append(lines, p.muted.Render(label+strings.Repeat("─", max(listWidth-lipgloss.Width(label), 0))))
	}
	dark, light := 0, 0
	for _, name := range styles.Themes {
		if styles.NewTheme(name).IsLight() {
			light++
		} else {
			dark++
		}
	}
	for i, name := range styles.Themes {
		t := styles.NewTheme(name)
		if i == 0 {
			group("DARK", dark)
		} else if t.IsLight() && !styles.NewTheme(styles.Themes[i-1]).IsLight() {
			lines = append(lines, "")
			group("LIGHT", light)
		}
		hovered := i == m.cursors[appearanceSection]
		active := name == m.cfg.Appearance.Theme
		marker, labelStyle := "  ", p.fg
		if hovered {
			marker, labelStyle = "▸ ", p.bold
			cursorLine = len(lines)
		}
		state := "  "
		if active {
			state = p.success.Render("● ")
		}
		nameWidth := max(listWidth-lipgloss.Width(marker)-2-2-swatchWidth, 6)
		label := ansi.Truncate(t.Label(), nameWidth, "…")
		label += strings.Repeat(" ", max(nameWidth-lipgloss.Width(label), 0))
		lines = append(lines, marker+state+labelStyle.Render(label)+"  "+swatch(t))
	}

	lines = windowLines(lines, cursorLine, rows)
	list := strings.Join(lines, "\n")
	if width-listWidth-2 < settingsMinPreview {
		return list
	}
	preview := renderThemePreview(m.hoveredTheme(), m.cfg.Appearance.Theme, min(width-listWidth-2, 52))
	return lipgloss.JoinHorizontal(lipgloss.Top, fixedBlock(list, listWidth, 0), "  ", preview)
}

// renderFields draws a section's settings with the selected one's detail below.
func (m *SettingsModal) renderFields(p settingsPalette, width, rows int) string {
	s := m.sections[m.activeSection]
	labelWidth := 22
	valueWidth := 20
	var b strings.Builder
	for i, f := range s.fields {
		selected := i == m.cursors[m.activeSection]
		marker, labelStyle := "  ", p.fg
		if selected {
			marker, labelStyle = "▸ ", p.bold
		}
		label := strings.ToUpper(f.label)
		label += strings.Repeat(" ", max(labelWidth-lipgloss.Width(label), 0))
		var value string
		if selected && m.editing {
			m.textInput.Width = valueWidth
			value = p.accent.Render("❯ ") + m.textInput.View()
		} else {
			value = m.renderValue(p, f)
		}
		value += strings.Repeat(" ", max(valueWidth-lipgloss.Width(value), 0))
		row := marker + labelStyle.Render(label) + value
		if width-lipgloss.Width(row) > len(f.key)+2 {
			row += "  " + p.muted.Render(f.key)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	if f, ok := m.selectedField(); ok {
		b.WriteString("\n")
		title := "DETAIL "
		b.WriteString(p.muted.Render(title + strings.Repeat("─", max(min(width, 72)-len(title), 0))))
		b.WriteString("\n")
		for _, line := range strings.Split(wrapWords(f.help, min(width, 72)), "\n") {
			b.WriteString(p.fg.Render(line))
			b.WriteString("\n")
		}
		b.WriteString(p.muted.Render(fieldAction(f, m.editing)))
	}
	out := strings.TrimRight(b.String(), "\n")
	if rows > 0 {
		lines := strings.Split(out, "\n")
		if len(lines) > rows {
			out = strings.Join(lines[:rows], "\n")
		}
	}
	return out
}

func (m *SettingsModal) renderValue(p settingsPalette, f settingsField) string {
	v := f.get(m.cfg)
	switch f.kind {
	case fieldBool:
		if v == "true" {
			return p.success.Render("◉ ON")
		}
		return p.muted.Render("○ OFF")
	case fieldChoice:
		return p.muted.Render("◀ ") + p.accent.Render(v) + p.muted.Render(" ▶")
	case fieldNumber:
		return p.accent.Render(v) + p.muted.Render(" "+f.unit)
	default:
		if v == "" {
			return p.muted.Render("—")
		}
		return p.accent.Render(v)
	}
}

func fieldAction(f settingsField, editing bool) string {
	if editing {
		return "enter save  •  esc cancel"
	}
	switch f.kind {
	case fieldBool:
		return "enter / space toggle"
	case fieldChoice:
		return "← → choose  •  " + strings.Join(f.choices, " / ")
	default:
		return "enter edit"
	}
}

func (m *SettingsModal) renderFooter(p settingsPalette, width int) string {
	var hints string
	switch {
	case m.editing:
		hints = "enter save  •  esc cancel"
	case m.activeSection == appearanceSection:
		hints = "↑↓ browse  •  enter apply  •  tab section  •  esc close"
	default:
		hints = "↑↓ select  •  enter change  •  tab section  •  esc close"
	}
	right := p.muted.Render(hints)
	left := ""
	switch {
	case m.statusMsg != "" && m.statusErr:
		left = p.danger.Render(m.statusMsg)
	case m.statusMsg != "":
		left = p.success.Render(m.statusMsg)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		if left != "" {
			return ansi.Truncate(left, width, "…")
		}
		return ansi.Truncate(right, width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderThemePreview draws a miniature Grove screen in t's own colours, so a
// theme can be judged before it is applied.
func renderThemePreview(t styles.Theme, activeName string, width int) string {
	inner := max(width-4, 10)
	fg := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	accent, muted, text := fg(t.Accent()).Bold(true), fg(t.Muted()), fg(t.Fg())

	kind := "DARK"
	if t.IsLight() {
		kind = "LIGHT"
	}
	title := accent.Render("PREVIEW") + muted.Render("  "+kind)
	if t.Name == activeName {
		title += fg(t.Success()).Render("  ● ACTIVE")
	}

	header := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Accent())).
		Foreground(lipgloss.Color(t.Bg())).
		Bold(true).
		Width(inner).
		Render(ansi.Truncate(" GROVE // "+strings.ToUpper(t.Label()), inner, "…"))

	row := func(selected bool, branch, status, color string) string {
		branch = ansi.Truncate(branch, inner-14, "…")
		pad := strings.Repeat(" ", max(inner-4-lipgloss.Width(branch)-lipgloss.Width(status), 1))
		if selected {
			return lipgloss.NewStyle().
				Background(lipgloss.Color(t.Accent())).
				Foreground(lipgloss.Color(t.Bg())).
				Bold(true).
				Width(inner).
				Render("▸ " + branch + pad + "● " + status)
		}
		return text.Render("  "+branch+pad) + fg(color).Render("● "+status)
	}

	barWidth := max(inner-10, 4)
	filled := barWidth * 5 / 8
	bar := fg(t.Success()).Render(strings.Repeat("━", filled)) + muted.Render(strings.Repeat("─", barWidth-filled)) + text.Render(" 62%")

	chips := func(pairs ...string) string {
		var parts []string
		for i := 0; i+1 < len(pairs); i += 2 {
			parts = append(parts, fg(pairs[i+1]).Render("■")+muted.Render(" "+pairs[i]))
		}
		return strings.Join(parts, "  ")
	}

	content := strings.Join([]string{
		title,
		header,
		row(true, "feat/theme-picker", "running", t.Accent()),
		row(false, "fix/sync-cache", "review", t.Warning()),
		row(false, "chore/deps", "clean", t.Success()),
		row(false, "hotfix/login", "failed", t.Danger()),
		"",
		muted.Render("PULSE ") + bar,
		"",
		chips("accent", t.Accent(), "ok", t.Success(), "warn", t.Warning(), "err", t.Danger()),
	}, "\n")

	st := lipgloss.NewStyle().
		Background(lipgloss.Color(t.Surface())).
		Foreground(lipgloss.Color(t.Fg())).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.Accent())).
		Padding(0, 1).
		Width(inner + 2)
	return t.RenderPanel(st, content)
}

// swatchWidth is the columns swatch occupies.
const swatchWidth = 7

// swatch renders a theme's signal colours (accent, success, warning, danger)
// as separate dots. The theme's background is left to the preview: drawn here,
// the chips of adjacent rows merge into one block.
func swatch(t styles.Theme) string {
	dots := make([]string, 0, 4)
	for _, hex := range []string{t.Accent(), t.Success(), t.Warning(), t.Danger()} {
		dots = append(dots, lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("●"))
	}
	return strings.Join(dots, " ")
}

// windowLines returns at most rows lines of lines, positioned so the line at
// focus stays in view. rows of 0 or less returns every line.
func windowLines(lines []string, focus, rows int) []string {
	if rows <= 0 || len(lines) <= rows {
		return lines
	}
	start := focus - rows/2
	if start < 0 {
		start = 0
	}
	if start > len(lines)-rows {
		start = len(lines) - rows
	}
	return lines[start : start+rows]
}

// fixedBlock pads every line of s to width columns, truncating longer ones,
// and pads or clips it to rows lines when rows is positive, so blocks joined
// side by side stay aligned.
func fixedBlock(s string, width, rows int) string {
	lines := strings.Split(s, "\n")
	if rows > 0 {
		if len(lines) > rows {
			lines = lines[:rows]
		}
		for len(lines) < rows {
			lines = append(lines, "")
		}
	}
	for i, line := range lines {
		line = ansi.Truncate(line, width, "…")
		lines[i] = line + strings.Repeat(" ", max(width-lipgloss.Width(line), 0))
	}
	return strings.Join(lines, "\n")
}

// wrapWords wraps s at word boundaries to lines of at most width columns.
func wrapWords(s string, width int) string {
	if width < 10 {
		width = 10
	}
	var lines []string
	var line string
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) > width:
			lines = append(lines, line)
			line = word
		default:
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// displayPath abbreviates the home directory in path to ~.
func displayPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return filepath.ToSlash(path)
}
