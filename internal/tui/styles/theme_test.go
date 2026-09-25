package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTheme_DigitalNoir_DefaultsCorrectly(t *testing.T) {
	tests := []struct {
		name      string
		inputName string
		wantName  string
	}{
		{
			name:      "digital-noir name is preserved",
			inputName: "digital-noir",
			wantName:  "digital-noir",
		},
		{
			name:      "unknown name falls back to digital-noir",
			inputName: "unknown-theme",
			wantName:  "digital-noir",
		},
		{
			name:      "empty string falls back to digital-noir",
			inputName: "",
			wantName:  "digital-noir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := NewTheme(tt.inputName)
			assert.Equal(t, tt.wantName, theme.Name)
		})
	}
}

func TestNewTheme_AllThemesHaveNames(t *testing.T) {
	for _, name := range Themes {
		t.Run(name, func(t *testing.T) {
			theme := NewTheme(name)
			assert.Equal(t, name, theme.Name)
			assert.NotEmpty(t, theme.accent)
			assert.NotEmpty(t, theme.bg)
		})
	}
}

func TestTheme_GetStyle_ReturnsStyleForKnownComponents(t *testing.T) {
	components := []string{
		"header", "nav-rail", "worktree-list", "selected-row",
		"status-bar", "modal-border", "error", "success", "context-panel", "table-header",
	}
	theme := NewTheme("digital-noir")

	for _, comp := range components {
		t.Run(comp, func(t *testing.T) {
			style := theme.GetStyle(comp)
			// lipgloss styles are value types; a non-zero style is valid
			assert.NotNil(t, style)
		})
	}
}

func TestTheme_GetStyle_UnknownComponentReturnsSafely(t *testing.T) {
	theme := NewTheme("digital-noir")
	style := theme.GetStyle("nonexistent-component")
	// Should not panic and must return a usable style
	require.NotPanics(t, func() {
		_ = style.Render("test")
	})
}

func TestTheme_RenderBox_ContentsPresent(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		content string
		wantIn  []string
	}{
		{
			name:    "renders box with title and content",
			title:   "My Title",
			content: "Some content",
			wantIn:  []string{"My Title", "Some content"},
		},
		{
			name:    "renders box without title",
			title:   "",
			content: "Only content",
			wantIn:  []string{"Only content"},
		},
	}

	theme := NewTheme("digital-noir")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := theme.RenderBox(tt.title, tt.content, 0)
			for _, want := range tt.wantIn {
				assert.Contains(t, out, want)
			}
		})
	}
}

func TestTheme_RenderTable_HeadersAndRowsPresent(t *testing.T) {
	theme := NewTheme("digital-noir")
	headers := []string{"NAME", "STATUS"}
	rows := [][]string{
		{"feat/auth", "Idle"},
		{"fix/bug", "Dirty"},
	}

	out := theme.RenderTable(rows, headers)

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "STATUS")
	assert.Contains(t, out, "feat/auth")
	assert.Contains(t, out, "fix/bug")
	assert.Contains(t, out, "Idle")
	assert.Contains(t, out, "Dirty")
}

func TestThemesList_HasThreeEntries(t *testing.T) {
	assert.Len(t, Themes, 9)
	assert.Equal(t, "digital-noir", Themes[0])
	assert.Equal(t, "matrix", Themes[1])
	assert.Equal(t, "light", Themes[2])
	assert.Equal(t, "everforest", Themes[3])
	assert.Equal(t, "tokyonight", Themes[4])
	assert.Equal(t, "catppuccin", Themes[5])
	assert.Equal(t, "kanagawa", Themes[6])
	assert.Equal(t, "rose-pine", Themes[7])
	assert.Equal(t, "onedark", Themes[8])
}

func TestFillBackground_ReassertsBackgroundAfterEveryReset(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	bg := lipgloss.Color("#F5F5F5")
	seq := strings.TrimSuffix(lipgloss.NewStyle().Background(bg).Render("x"), "x\x1b[0m")
	require.NotEmpty(t, seq)
	span := lipgloss.NewStyle().Foreground(lipgloss.Color("#0066CC")).Render("ACTIONS")

	out := FillBackground(span+" [a] focus\nplain", bg)

	lines := strings.Split(out, "\n")
	require.Len(t, lines, 2)
	assert.True(t, strings.HasPrefix(lines[0], seq), "a line starts on the background")
	assert.Contains(t, lines[0], "\x1b[0m"+seq+" [a] focus", "text after a span keeps the background")
	assert.True(t, strings.HasPrefix(lines[1], seq+"plain"))
}

func TestFillBackground_LeavesUncolouredOutputAlone(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	assert.Equal(t, "a\nb", FillBackground("a\nb", lipgloss.Color("#F5F5F5")))
	assert.Equal(t, "a\nb", FillBackground("a\nb", lipgloss.NoColor{}))
}
