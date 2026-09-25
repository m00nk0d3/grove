package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds the color palette and component styles for a named visual theme.
type Theme struct {
	Name    string
	label   string
	light   bool
	accent  string
	bg      string
	surface string
	fg      string
	muted   string
	success string
	warning string
	danger  string
}

// catalog lists every built-in theme in display order: the default first, the
// remaining dark themes, then the light ones. Themes and NewTheme both derive
// from it, so a theme is added in exactly one place.
var catalog = []Theme{
	{Name: "digital-noir", label: "Digital Noir", accent: "#00D9FF", bg: "#0a0e27", surface: "#0d1117", fg: "#E2E8F0", muted: "#4A5568", success: "#00FF88", warning: "#FFD700", danger: "#FF4757"},
	{Name: "matrix", label: "Matrix", accent: "#00FF00", bg: "#000000", surface: "#0a0a0a", fg: "#00FF00", muted: "#006600", success: "#00FF00", warning: "#FFFF00", danger: "#FF0000"},
	{Name: "cyberpunk", label: "Cyberpunk", accent: "#FCEE0A", bg: "#0d0221", surface: "#140a2e", fg: "#E0E0FF", muted: "#6b5b95", success: "#00FF9F", warning: "#FF9E00", danger: "#FF003C"},
	{Name: "synthwave", label: "Synthwave '84", accent: "#ff7edb", bg: "#262335", surface: "#1e1a2e", fg: "#f0eff1", muted: "#848bbd", success: "#72f1b8", warning: "#fede5d", danger: "#fe4450"},
	{Name: "tokyonight", label: "Tokyo Night", accent: "#7aa2f7", bg: "#1a1b26", surface: "#16161e", fg: "#c0caf5", muted: "#565f89", success: "#9ece6a", warning: "#e0af68", danger: "#f7768e"},
	{Name: "catppuccin", label: "Catppuccin Mocha", accent: "#cba6f7", bg: "#1e1e2e", surface: "#181825", fg: "#cdd6f4", muted: "#6c7086", success: "#a6e3a1", warning: "#fab387", danger: "#f38ba8"},
	{Name: "dracula", label: "Dracula", accent: "#bd93f9", bg: "#282a36", surface: "#21222c", fg: "#f8f8f2", muted: "#6272a4", success: "#50fa7b", warning: "#f1fa8c", danger: "#ff5555"},
	{Name: "nord", label: "Nord", accent: "#88c0d0", bg: "#2e3440", surface: "#3b4252", fg: "#eceff4", muted: "#616e88", success: "#a3be8c", warning: "#ebcb8b", danger: "#bf616a"},
	{Name: "kanagawa", label: "Kanagawa", accent: "#7e9cd8", bg: "#1f1f28", surface: "#16161d", fg: "#dcd7ba", muted: "#727169", success: "#98bb6c", warning: "#e6c384", danger: "#c34043"},
	{Name: "rose-pine", label: "Rosé Pine", accent: "#c4a7e7", bg: "#191724", surface: "#1f1d2e", fg: "#e0def4", muted: "#6e6a86", success: "#9ccfd8", warning: "#f6c177", danger: "#eb6f92"},
	{Name: "onedark", label: "One Dark", accent: "#61afef", bg: "#282c34", surface: "#21252b", fg: "#abb2bf", muted: "#5c6370", success: "#98c379", warning: "#e5c07b", danger: "#e06c75"},
	{Name: "gruvbox", label: "Gruvbox", accent: "#fe8019", bg: "#282828", surface: "#1d2021", fg: "#ebdbb2", muted: "#928374", success: "#b8bb26", warning: "#fabd2f", danger: "#fb4934"},
	{Name: "everforest", label: "Everforest", accent: "#a7c080", bg: "#2b3339", surface: "#323c41", fg: "#d3c6aa", muted: "#7a8478", success: "#a7c080", warning: "#e69875", danger: "#e67e80"},
	{Name: "solarized-dark", label: "Solarized Dark", accent: "#268bd2", bg: "#002b36", surface: "#073642", fg: "#93a1a1", muted: "#586e75", success: "#859900", warning: "#b58900", danger: "#dc322f"},
	{Name: "monokai", label: "Monokai", accent: "#66d9ef", bg: "#272822", surface: "#1e1f1c", fg: "#f8f8f2", muted: "#75715e", success: "#a6e22e", warning: "#e6db74", danger: "#f92672"},
	{Name: "ayu-mirage", label: "Ayu Mirage", accent: "#ffcc66", bg: "#1f2430", surface: "#191e2a", fg: "#cccac2", muted: "#707a8c", success: "#87d96c", warning: "#ffa659", danger: "#f28779"},
	{Name: "light", label: "Light", light: true, accent: "#0066CC", bg: "#F0F0F0", surface: "#F5F5F5", fg: "#1A1A1A", muted: "#666666", success: "#008000", warning: "#CC6600", danger: "#CC0000"},
	{Name: "github-light", label: "GitHub Light", light: true, accent: "#0969da", bg: "#ffffff", surface: "#f6f8fa", fg: "#1f2328", muted: "#656d76", success: "#1a7f37", warning: "#9a6700", danger: "#cf222e"},
	{Name: "catppuccin-latte", label: "Catppuccin Latte", light: true, accent: "#8839ef", bg: "#eff1f5", surface: "#e6e9ef", fg: "#4c4f69", muted: "#8c8fa1", success: "#40a02b", warning: "#df8e1d", danger: "#d20f39"},
	{Name: "solarized-light", label: "Solarized Light", light: true, accent: "#268bd2", bg: "#fdf6e3", surface: "#eee8d5", fg: "#073642", muted: "#839496", success: "#859900", warning: "#b58900", danger: "#dc322f"},
	{Name: "rose-pine-dawn", label: "Rosé Pine Dawn", light: true, accent: "#907aa9", bg: "#faf4ed", surface: "#fffaf3", fg: "#575279", muted: "#9893a5", success: "#56949f", warning: "#ea9d34", danger: "#b4637a"},
	{Name: "tokyonight-day", label: "Tokyo Night Day", light: true, accent: "#2e7de9", bg: "#e1e2e7", surface: "#e9e9ec", fg: "#3760bf", muted: "#848cb5", success: "#587539", warning: "#8c6c3e", danger: "#f52a65"},
	{Name: "gruvbox-light", label: "Gruvbox Light", light: true, accent: "#af3a03", bg: "#fbf1c7", surface: "#f2e5bc", fg: "#3c3836", muted: "#7c6f64", success: "#79740e", warning: "#b57614", danger: "#9d0006"},
}

// Themes is the ordered list of available theme names. The first entry is the
// default.
var Themes = func() []string {
	names := make([]string, len(catalog))
	for i, t := range catalog {
		names[i] = t.Name
	}
	return names
}()

// NewTheme creates a Theme for the given name, defaulting to digital-noir.
func NewTheme(name string) Theme {
	for _, t := range catalog {
		if t.Name == name {
			return t
		}
	}
	return catalog[0]
}

// Label returns the theme's human-readable name.
func (t Theme) Label() string { return t.label }

// IsLight reports whether the theme is meant for a light background.
func (t Theme) IsLight() bool { return t.light }

// GetStyle returns a lipgloss.Style for the named component.
// Unknown component names return a default surface/foreground style.
func (t Theme) GetStyle(component string) lipgloss.Style {
	switch component {
	case "header":
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.accent)).
			Foreground(lipgloss.Color(t.bg)).
			Bold(true).
			Padding(0, 1)
	case "nav-rail":
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.surface)).
			Foreground(lipgloss.Color(t.fg)).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.accent)).
			Padding(0, 1)
	case "worktree-list":
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.surface)).
			Foreground(lipgloss.Color(t.fg)).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.accent)).
			Padding(0, 1)
	case "selected-row":
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.accent)).
			Foreground(lipgloss.Color(t.bg)).
			Bold(true)
	case "status-bar":
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.surface)).
			Foreground(lipgloss.Color(t.muted))
	case "modal-border":
		return lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.accent))
	case "error":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.danger))
	case "success":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.success))
	case "context-panel":
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.surface)).
			Foreground(lipgloss.Color(t.fg)).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(t.accent)).
			Padding(0, 1)
	case "table-header":
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.muted)).
			Bold(true)
	default:
		return lipgloss.NewStyle().
			Background(lipgloss.Color(t.surface)).
			Foreground(lipgloss.Color(t.fg))
	}
}

// StatusStyle returns a lipgloss.Style for a worktree or PR status value.
// Background is always set to the surface color so padded cells don't bleed
// terminal-default black into the panel background.
func (t Theme) StatusStyle(status string) lipgloss.Style {
	bg := lipgloss.Color(t.surface)
	switch strings.ToLower(status) {
	case "checked", "checked out": // reserved for future git worktree "checkedout" state
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.accent))
	case "in progress":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.accent))
	case "idle", "clean", "open":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.success))
	case "created", "dirty", "review":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.warning))
	case "locked", "changes", "action":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.danger))
	case "approved":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.success)).Bold(true)
	case "draft", "merged", "closed":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.muted))
	default:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.muted))
	}
}

// sgrReset is the sequence lipgloss emits at the end of every styled span.
const sgrReset = "\x1b[0m"

// FillBackground makes background the backdrop of every cell in s that no
// span colours itself. A styled span ends with a full SGR reset, which clears
// the enclosing panel's background along with its own attributes, so the text
// and padding that follow it on the same line fall back to the terminal's
// default background. On a dark terminal that shows as black patches through a
// light theme. The background is re-asserted at the start of every line and
// after every reset. Under a colour profile that emits no escapes this returns
// s unchanged.
func FillBackground(s string, background lipgloss.TerminalColor) string {
	if _, none := background.(lipgloss.NoColor); none || background == nil {
		return s
	}
	const marker = "x"
	styled := lipgloss.NewStyle().Background(background).Render(marker)
	seq, _, found := strings.Cut(styled, marker)
	if !found || seq == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = seq + strings.ReplaceAll(line, sgrReset, sgrReset+seq) + sgrReset
	}
	return strings.Join(lines, "\n")
}

// RenderPanel renders content with st, first carrying st's background through
// every styled span inside content so the whole panel keeps one backdrop. The
// border is drawn on the same background.
func (t Theme) RenderPanel(st lipgloss.Style, content string) string {
	bg := st.GetBackground()
	return st.BorderBackground(bg).Render(FillBackground(content, bg))
}

// Fill paints the theme's base background behind every cell of a full frame
// that no panel or span colours itself, such as the gaps between panels and the
// whitespace lipgloss adds when it joins or places blocks.
func (t Theme) Fill(frame string) string {
	return FillBackground(frame, lipgloss.Color(t.bg))
}

// Accent returns the theme's accent color hex string.
func (t Theme) Accent() string { return t.accent }

// Muted returns the theme's muted color hex string.
func (t Theme) Muted() string { return t.muted }

// Fg returns the theme's foreground color hex string.
func (t Theme) Fg() string { return t.fg }

// Bg returns the theme's background color hex string.
func (t Theme) Bg() string { return t.bg }

// Success returns the theme's success color hex string.
func (t Theme) Success() string { return t.success }

// Warning returns the theme's warning color hex string.
func (t Theme) Warning() string { return t.warning }

// Danger returns the theme's danger color hex string.
func (t Theme) Danger() string { return t.danger }

// Surface returns the theme's panel surface color hex string.
func (t Theme) Surface() string { return t.surface }

// MutedBorder returns s with the border foreground dimmed to the muted color.
// Apply this to unfocused panels to visually de-emphasize them relative to the
// currently focused panel.
func (t Theme) MutedBorder(s lipgloss.Style) lipgloss.Style {
	return s.BorderForeground(lipgloss.Color(t.muted))
}

// RenderBox renders content inside a rounded-border panel with an optional title.
// width sets the total rendered width in terminal columns; 0 means size to content.
func (t Theme) RenderBox(title, content string, width int) string {
	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(t.accent)).
		Padding(0, 1)
	// Overhead: 1 border + 1 padding on each side = 4 columns total.
	if width > 4 {
		style = style.Width(width - 4)
	}
	if title != "" {
		return style.Render(fmt.Sprintf("%s\n%s", title, content))
	}
	return style.Render(content)
}

// RenderFullscreenBox renders a large modal panel with fixed terminal-relative
// dimensions while preserving the active theme's border and background.
func (t Theme) RenderFullscreenBox(title, content string, width, height int) string {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(t.bg)).
		Foreground(lipgloss.Color(t.fg)).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(t.accent)).
		Padding(0, 1)
	if width > 4 {
		style = style.Width(width - 4)
	}
	if height > 2 {
		style = style.Height(height - 2)
	}
	if title != "" {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.accent)).
			Bold(true)
		return t.RenderPanel(style, titleStyle.Render(title)+"\n"+content)
	}
	return t.RenderPanel(style, content)
}

// RenderTable renders a padded table with styled muted column headers.
func (t Theme) RenderTable(rows [][]string, headers []string) string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.muted)).
		Bold(true)

	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	for i, h := range headers {
		b.WriteString(headerStyle.Render(fmt.Sprintf("%-*s", colWidths[i], h)))
		if i < len(headers)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")

	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				b.WriteString(fmt.Sprintf("%-*s", colWidths[i], cell))
				if i < len(row)-1 {
					b.WriteString("  ")
				}
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
