package styles

import (
	"fmt"
	"math"
	"strconv"
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

func TestThemes_DefaultComesFirst(t *testing.T) {
	require.NotEmpty(t, Themes)
	assert.Equal(t, "digital-noir", Themes[0])
}

func TestThemes_KeepsEveryPreviouslyConfigurableName(t *testing.T) {
	// A config written by an earlier release names one of these; dropping or
	// renaming one would silently fall back to the default theme.
	for _, name := range []string{"digital-noir", "matrix", "light", "everforest", "tokyonight", "catppuccin", "kanagawa", "rose-pine", "onedark"} {
		assert.Contains(t, Themes, name)
	}
}

func TestThemes_NamesAreUniqueAndResolve(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range Themes {
		assert.False(t, seen[name], "duplicate theme %q", name)
		seen[name] = true
		theme := NewTheme(name)
		assert.Equal(t, name, theme.Name)
		assert.NotEmpty(t, theme.Label(), "theme %q has no label", name)
	}
}

func TestThemes_DarkThemesPrecedeLightThemes(t *testing.T) {
	sawLight := false
	for _, name := range Themes {
		if NewTheme(name).IsLight() {
			sawLight = true
		} else {
			assert.False(t, sawLight, "dark theme %q listed after a light theme", name)
		}
	}
	assert.True(t, sawLight, "catalog should include light themes")
}

func TestThemes_PalettesAreValidAndReadable(t *testing.T) {
	for _, name := range Themes {
		theme := NewTheme(name)
		for _, hex := range []string{theme.Accent(), theme.Bg(), theme.Surface(), theme.Fg(), theme.Muted(), theme.Success(), theme.Warning(), theme.Danger()} {
			_, err := parseHex(hex)
			require.NoError(t, err, "theme %q color %q", name, hex)
		}
		// Body text must stay legible on both of the theme's backgrounds.
		assert.GreaterOrEqual(t, contrast(t, theme.Fg(), theme.Bg()), 4.5, "theme %q fg on bg", name)
		assert.GreaterOrEqual(t, contrast(t, theme.Fg(), theme.Surface()), 4.5, "theme %q fg on surface", name)
	}
}

func parseHex(hex string) ([3]float64, error) {
	var rgb [3]float64
	if len(hex) != 7 || hex[0] != '#' {
		return rgb, fmt.Errorf("not a #rrggbb color: %q", hex)
	}
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			return rgb, err
		}
		rgb[i] = float64(v) / 255
	}
	return rgb, nil
}

// contrast returns the WCAG contrast ratio between two #rrggbb colors.
func contrast(t *testing.T, a, b string) float64 {
	t.Helper()
	luminance := func(hex string) float64 {
		rgb, err := parseHex(hex)
		require.NoError(t, err)
		var l [3]float64
		for i, c := range rgb {
			if c <= 0.03928 {
				l[i] = c / 12.92
			} else {
				l[i] = math.Pow((c+0.055)/1.055, 2.4)
			}
		}
		return 0.2126*l[0] + 0.7152*l[1] + 0.0722*l[2]
	}
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
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
