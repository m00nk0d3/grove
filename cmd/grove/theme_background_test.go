package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

// uncoloredCells reports the visible characters in frame drawn while no
// background colour is in effect, which the terminal paints with its own
// default background instead of the theme's.
func uncoloredCells(frame string) []string {
	var bad []string
	hasBackground := false
	for line, text := range strings.Split(frame, "\n") {
		for i := 0; i < len(text); i++ {
			if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
				end := strings.IndexByte(text[i:], 'm')
				if end < 0 {
					break
				}
				params := text[i+2 : i+end]
				if params == "" || params == "0" {
					hasBackground = false
				}
				for _, p := range strings.Split(params, ";") {
					if p == "48" || (len(p) == 2 && p[0] == '4' && p[1] >= '0' && p[1] <= '7') {
						hasBackground = true
					}
				}
				i += end
				continue
			}
			if !hasBackground {
				bad = append(bad, fmt.Sprintf("line %d: %q", line, text[i:min(len(text), i+20)]))
				// One report per uncoloured run is enough to locate it.
				for i+1 < len(text) && text[i+1] != '\x1b' {
					i++
				}
			}
		}
	}
	return bad
}

func TestView_LightThemePaintsEveryCellWithABackground(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })

	lightIdx := -1
	for i, name := range styles.Themes {
		if name == "light" {
			lightIdx = i
		}
	}
	require.GreaterOrEqual(t, lightIdx, 0)

	for _, view := range []activeView{viewDashboard, viewWorktrees, viewIssues, viewPRs} {
		model := NewModel()
		require.NotNil(t, model)
		model.themeIdx = lightIdx
		model.width, model.height = 160, 40
		model.view = view
		model.Worktrees = []domain.Worktree{{Path: "/tmp/grove", Branch: "main", IsClean: true}}
		model.issues = []domain.Issue{{Number: 7, Title: "Light theme issue", State: "OPEN"}}
		model.prs = []domain.PullRequest{{Number: 8, Title: "Light theme PR", State: "OPEN"}}
		model.missionState = &domain.MissionControlState{
			Status:       "running",
			UpdatedAt:    time.Now(),
			WorkflowRuns: []domain.WorkflowRunRef{{WorkflowID: "run-one", Title: "Implement issue #7", Status: domain.WorkflowRunning}},
		}

		frame := model.View()

		require.Empty(t, uncoloredCells(frame), "view %d draws cells on the terminal's default background", view)
	}
}
