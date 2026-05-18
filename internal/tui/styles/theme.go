package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Themes is the ordered list of available theme names used for cycling.
var Themes = []string{"digital-noir", "matrix", "light", "everforest", "tokyonight", "catppuccin", "kanagawa", "rose-pine", "onedark"}

// Theme holds the color palette and component styles for a named visual theme.
type Theme struct {
	Name    string
	accent  string
	bg      string
	surface string
	fg      string
	muted   string
	success string
	warning string
	danger  string
}

// NewTheme creates a Theme for the given name, defaulting to digital-noir.
func NewTheme(name string) Theme {
	switch name {
	case "matrix":
		return Theme{
			Name:    "matrix",
			accent:  "#00FF00",
			bg:      "#000000",
			surface: "#0a0a0a",
			fg:      "#00FF00",
			muted:   "#006600",
			success: "#00FF00",
			warning: "#FFFF00",
			danger:  "#FF0000",
		}
	case "light":
		return Theme{
			Name:    "light",
			accent:  "#0066CC",
			bg:      "#F0F0F0",
			surface: "#F5F5F5",
			fg:      "#1A1A1A",
			muted:   "#666666",
			success: "#008000",
			warning: "#CC6600",
			danger:  "#CC0000",
		}
	case "everforest":
		return Theme{
			Name:    "everforest",
			accent:  "#a7c080",
			bg:      "#2b3339",
			surface: "#323c41",
			fg:      "#d3c6aa",
			muted:   "#7a8478",
			success: "#a7c080",
			warning: "#e69875",
			danger:  "#e67e80",
		}
	case "tokyonight":
		return Theme{
			Name:    "tokyonight",
			accent:  "#7aa2f7",
			bg:      "#1a1b26",
			surface: "#16161e",
			fg:      "#c0caf5",
			muted:   "#565f89",
			success: "#9ece6a",
			warning: "#e0af68",
			danger:  "#f7768e",
		}
	case "catppuccin":
		return Theme{
			Name:    "catppuccin",
			accent:  "#cba6f7",
			bg:      "#1e1e2e",
			surface: "#181825",
			fg:      "#cdd6f4",
			muted:   "#6c7086",
			success: "#a6e3a1",
			warning: "#fab387",
			danger:  "#f38ba8",
		}
	case "kanagawa":
		return Theme{
			Name:    "kanagawa",
			accent:  "#7e9cd8",
			bg:      "#1f1f28",
			surface: "#16161d",
			fg:      "#dcd7ba",
			muted:   "#727169",
			success: "#98bb6c",
			warning: "#e6c384",
			danger:  "#c34043",
		}
	case "rose-pine":
		return Theme{
			Name:    "rose-pine",
			accent:  "#c4a7e7",
			bg:      "#191724",
			surface: "#1f1d2e",
			fg:      "#e0def4",
			muted:   "#6e6a86",
			success: "#9ccfd8",
			warning: "#f6c177",
			danger:  "#eb6f92",
		}
	case "onedark":
		return Theme{
			Name:    "onedark",
			accent:  "#61afef",
			bg:      "#282c34",
			surface: "#21252b",
			fg:      "#abb2bf",
			muted:   "#5c6370",
			success: "#98c379",
			warning: "#e5c07b",
			danger:  "#e06c75",
		}
	default: // digital-noir
		return Theme{
			Name:    "digital-noir",
			accent:  "#00D9FF",
			bg:      "#0a0e27",
			surface: "#0d1117",
			fg:      "#E2E8F0",
			muted:   "#4A5568",
			success: "#00FF88",
			warning: "#FFD700",
			danger:  "#FF4757",
		}
	}
}

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
	case "locked", "changes":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.danger))
	case "approved":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.success)).Bold(true)
	case "draft", "merged", "closed":
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.muted))
	default:
		return lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color(t.muted))
	}
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
