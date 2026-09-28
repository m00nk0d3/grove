package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/tui/styles"
)

// inspectorChrome draws the shared furniture of the fullscreen inspectors:
// fields, headings, metric cards, section panels, and status colors.
type inspectorChrome struct {
	theme *styles.Theme
	width int
}

func (c inspectorChrome) field(label, value string) string {
	return fmt.Sprintf("  %s %s\n", c.mutedStyle().Render(fmt.Sprintf("%-14s", label)), value)
}

func (c inspectorChrome) heading(value string) string {
	return c.accentStyle().Bold(true).Render("◆ " + value)
}

func (c inspectorChrome) metricCard(label, value string, width int) string {
	return lipgloss.NewStyle().
		Width(max(8, width-2)).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(c.accentStyle().GetForeground()).
		Padding(0, 1).
		Render(c.mutedStyle().Render(label) + "\n" + c.accentStyle().Bold(true).Render(value))
}

func (c inspectorChrome) sectionPanel(title, content string, width int) string {
	return lipgloss.NewStyle().
		Width(max(20, width-4)).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(c.mutedStyle().GetForeground()).
		Padding(0, 1).
		Render(c.accentStyle().Bold(true).Render(title) + "\n\n" + content)
}

func (c inspectorChrome) contentWidth() int {
	width := c.width - 12
	if width < 48 {
		return 48
	}
	if width > 132 {
		return 132
	}
	return width
}

func (c inspectorChrome) truncateTo(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(value)
	if len(runes) >= width {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

func (c inspectorChrome) accentStyle() lipgloss.Style {
	if c.theme == nil {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.theme.Accent()))
}

func (c inspectorChrome) mutedStyle() lipgloss.Style {
	if c.theme == nil {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.theme.Muted()))
}

func (c inspectorChrome) keyStyle() lipgloss.Style { return c.accentStyle().Bold(true) }

func (c inspectorChrome) selectedStyle() lipgloss.Style {
	if c.theme == nil {
		return lipgloss.NewStyle().Bold(true)
	}
	return c.theme.GetStyle("selected-row")
}

func (c inspectorChrome) statusStyle(status string) lipgloss.Style {
	if c.theme == nil {
		return lipgloss.NewStyle()
	}
	switch strings.ToLower(status) {
	case "running", "working":
		return c.accentStyle()
	case "succeeded", "done":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(c.theme.Success()))
	case "failed", "blocked":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(c.theme.Warning()))
	default:
		return c.mutedStyle()
	}
}

func (c inspectorChrome) currentTheme() styles.Theme {
	if c.theme != nil {
		return *c.theme
	}
	return styles.NewTheme("")
}

// keyHints renders "key label  ·  key label" pairs for an inspector footer.
func (c inspectorChrome) keyHints(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(c.mutedStyle().Render("  ·  "))
		}
		b.WriteString(c.keyStyle().Render(pairs[i]))
		b.WriteString(c.mutedStyle().Render(" " + pairs[i+1]))
	}
	return b.String()
}
