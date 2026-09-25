//go:build demo

// This file exports the website's live demo: a scripted walk through Grove,
// rendered frame by frame by Grove's own renderer and written as JSON for the
// site to play back. It is excluded from normal builds and test runs; run it
// with `make demo`.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/m00nk0d3/grove/internal/version"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

const (
	demoCols = 160
	demoRows = 38
)

// demoStyle is one distinct combination of text attributes in the demo.
type demoStyle struct {
	Fg        string `json:"fg,omitempty"`
	Bg        string `json:"bg,omitempty"`
	Bold      bool   `json:"b,omitempty"`
	Italic    bool   `json:"i,omitempty"`
	Underline bool   `json:"u,omitempty"`
}

// demoFrame is one screen of the demo. Lines are runs of [text, style index].
type demoFrame struct {
	Chapter string            `json:"chapter"`
	Keys    string            `json:"keys,omitempty"`
	Caption string            `json:"caption"`
	HoldMs  int               `json:"hold"`
	Bg      int               `json:"bg"`
	Lines   [][][2]any        `json:"lines"`
	styles  map[demoStyle]int `json:"-"`
}

type demoExport struct {
	Version string      `json:"version"`
	Cols    int         `json:"cols"`
	Rows    int         `json:"rows"`
	Styles  []demoStyle `json:"styles"`
	Frames  []demoFrame `json:"frames"`
}

// demoRecorder renders the model after each scripted step and collects the
// frames, sharing one style table between them.
type demoRecorder struct {
	t      *testing.T
	m      *Model
	styles map[demoStyle]int
	out    demoExport
}

func (r *demoRecorder) styleIndex(s demoStyle) int {
	if i, ok := r.styles[s]; ok {
		return i
	}
	i := len(r.out.Styles)
	r.styles[s] = i
	r.out.Styles = append(r.out.Styles, s)
	return i
}

// capture renders the current screen as a frame.
func (r *demoRecorder) capture(chapter, keys, caption string, hold time.Duration) {
	r.t.Helper()
	theme := styles.NewTheme(styles.Themes[r.m.themeIdx])
	frame := demoFrame{
		Chapter: chapter,
		Keys:    keys,
		Caption: caption,
		HoldMs:  int(hold / time.Millisecond),
		Bg:      r.styleIndex(demoStyle{Bg: strings.ToLower(theme.Bg())}),
	}
	lines := strings.Split(r.m.View(), "\n")
	require.LessOrEqual(r.t, len(lines), demoRows, "frame %q is taller than the demo", caption)
	for _, line := range lines {
		frame.Lines = append(frame.Lines, r.runs(line))
	}
	r.out.Frames = append(r.out.Frames, frame)
}

// runs splits one rendered line into runs of identically styled text.
func (r *demoRecorder) runs(line string) [][2]any {
	var runs [][2]any
	var cur demoStyle
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		idx := r.styleIndex(cur)
		if n := len(runs); n > 0 && runs[n-1][1] == idx {
			runs[n-1][0] = runs[n-1][0].(string) + text.String()
		} else {
			runs = append(runs, [2]any{text.String(), idx})
		}
		text.Reset()
	}
	for i := 0; i < len(line); i++ {
		if line[i] != 0x1b || i+1 >= len(line) || line[i+1] != '[' {
			text.WriteByte(line[i])
			continue
		}
		end := strings.IndexByte(line[i:], 'm')
		if end < 0 {
			break
		}
		next := cur
		params := strings.Split(line[i+2:i+end], ";")
		for j := 0; j < len(params); j++ {
			switch params[j] {
			case "", "0":
				next = demoStyle{}
			case "1":
				next.Bold = true
			case "3":
				next.Italic = true
			case "4":
				next.Underline = true
			case "22":
				next.Bold = false
			case "23":
				next.Italic = false
			case "24":
				next.Underline = false
			case "39":
				next.Fg = ""
			case "49":
				next.Bg = ""
			case "38", "48":
				if j+4 < len(params) && params[j+1] == "2" {
					rgb := make([]int, 3)
					for k := range rgb {
						rgb[k], _ = strconv.Atoi(params[j+2+k])
					}
					hex := fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
					if params[j] == "38" {
						next.Fg = hex
					} else {
						next.Bg = hex
					}
					j += 4
				}
			}
		}
		if next != cur {
			flush()
			cur = next
		}
		i += end
	}
	flush()
	return runs
}

// key sends a key press through the model, as a terminal would.
func (r *demoRecorder) key(k string) tea.Cmd {
	var msg tea.KeyMsg
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "pgdown":
		msg = tea.KeyMsg{Type: tea.KeyPgDown}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	_, cmd := r.m.Update(msg)
	return cmd
}

// closeModal delivers the cancellation a modal's Esc asks for.
func (r *demoRecorder) closeModal() {
	if cmd := r.key("esc"); cmd != nil {
		if msg, ok := cmd().(modal.ModalCancelledMsg); ok {
			r.m.Update(msg)
		}
	}
}

func demoInt(n int) *int { return &n }

func connectedIntegration(name string) domain.ExternalIntegration {
	return domain.ExternalIntegration{Name: name, Available: true, Enabled: true, Mode: "connected"}
}

var demoSteps = []string{"planning", "tests", "implementation", "verification", "domain-review", "documentation", "delivery", "report", "publish", "review"}

// demoRunSteps returns the full workflow's stages with the first done of them
// succeeded and the next one running.
func demoRunSteps(started time.Time, done int) []domain.WorkflowStep {
	steps := make([]domain.WorkflowStep, len(demoSteps))
	for i, id := range demoSteps {
		steps[i] = domain.WorkflowStep{ID: id, Title: id, Status: domain.WorkflowQueued}
		switch {
		case i < done:
			steps[i].Status = domain.WorkflowSucceeded
			steps[i].StartedAt = started.Add(time.Duration(i) * 3 * time.Minute)
			steps[i].CompletedAt = steps[i].StartedAt.Add(3 * time.Minute)
		case i == done:
			steps[i].Status = domain.WorkflowRunning
			steps[i].StartedAt = started.Add(time.Duration(i) * 3 * time.Minute)
		}
	}
	return steps
}

const demoReport = `# Implementation Report

> **Issue:** m00nk0d3/grove#224 — keep one periodic GitHub sync running

## Overview

Every completed GitHub sync scheduled another timer, so each manual refresh
started a second background sync chain. A sync tick now carries the
generation that scheduled it, and only the newest one syncs. Turning
` + "`auto_sync`" + ` off stops the chain; turning it on starts one immediately.

## What Changed

- ` + "`scheduleSyncTick`" + ` supersedes any tick still pending.
- The tick handler ignores superseded ticks and does nothing with auto sync off.
- Saving settings starts, stops, or reschedules the chain.

## Files Changed

| File | Change |
|---|---|
| ` + "`cmd/grove/app.go`" + ` | One sync chain; settings take effect at once |
| ` + "`cmd/grove/config_behaviour_test.go`" + ` | New. Tick generation and settings tests |

## Verification

` + "```" + `
go test ./...   ok
npm test        ok
` + "```" + `
`

const demoVerdict = `# Review Cycle 1: APPROVED

## Summary

The change keeps exactly one periodic sync chain alive and makes the auto sync
setting take effect when it is saved. Superseded ticks are ignored rather than
cancelled, which is safe because a tick does nothing but start a sync.

## Blockers

- _None_
`

func TestExportDemo(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	if v := os.Getenv("GROVE_DEMO_VERSION"); v != "" {
		version.Version = v
	}

	now := time.Now()
	m := NewModel()
	require.NotNil(t, m)
	// The demo never reads or writes the user's own configuration.
	m.Config = domain.DefaultConfig()
	m.themeIdx = 0
	m.dismissedWorkflows = map[string]bool{}
	m.sessions = nil
	m.width, m.height = demoCols, demoRows
	m.RepoPath = "/src/grove"
	m.herdrSnapshot = &herdr.Snapshot{Integration: connectedIntegration("herdr")}
	m.sandcastleSnapshot = &sandcastle.Snapshot{Integration: connectedIntegration("sandcastle")}
	m.lastSynced = now.Add(-2 * time.Minute)
	m.Worktrees = []domain.Worktree{
		{Path: "/src/grove", Branch: "main", IsClean: true},
		{Path: "/src/worktrees/grove/feat-issue-218-worktree-root", Branch: "feat/issue-218-worktree-root"},
		{Path: "/src/worktrees/grove/fix-issue-229-ci-cache", Branch: "fix/issue-229-ci-cache", IsClean: true},
		{Path: "/src/worktrees/grove/feat-issue-226-inspector-reports", Branch: "feat/issue-226-inspector-reports", IsClean: true},
	}
	m.issues = []domain.Issue{
		{Number: 210, Title: "Mission control: settings overhaul", State: "OPEN", Labels: []string{"epic"}, SubIssueNumbers: []int{218, 219}},
		{Number: 218, Title: "Honour worktree_root when creating worktrees", State: "OPEN", ParentNumber: demoInt(210), ProjectStatus: "In Progress"},
		{Number: 219, Title: "Honour base_branch for new worktrees", State: "OPEN", ParentNumber: demoInt(210)},
		{Number: 224, Title: "Keep one periodic GitHub sync running", State: "OPEN", Labels: []string{"bug"}},
		{Number: 226, Title: "Show a workflow's reports in the inspector", State: "OPEN", ProjectStatus: "Done"},
		{Number: 230, Title: "Refresh the runbook", State: "OPEN", Labels: []string{"docs"}},
		{Number: 233, Title: "Rust validation command", State: "OPEN", Labels: []string{"enhancement"}},
	}
	m.prs = []domain.PullRequest{
		{Number: 231, Title: "feat(tui): render markdown reports", Branch: "feat/issue-226-inspector-reports", State: "OPEN", ReviewRequested: true},
		{Number: 229, Title: "fix(ci): cache runtime dependencies", Branch: "fix/issue-229-ci-cache", State: "OPEN", ChecksFailing: true, IsMine: true},
		{Number: 227, Title: "docs: rewrite the JSON contract", Branch: "docs/issue-227-contract", State: "OPEN", ReviewDecision: "APPROVED", IsMine: true},
	}

	runs := []domain.WorkflowRunRef{
		{RunID: "run_218", Kind: "imp", Title: "Implement issue #218", Status: domain.WorkflowRunning, CurrentStep: "implementation", IssueNumber: demoInt(218), WorktreePath: m.Worktrees[1].Path, Progress: domain.WorkflowProgress{Completed: 2, Total: 10, Percent: 20}, Steps: demoRunSteps(now.Add(-8*time.Minute), 2), StartedAt: now.Add(-8 * time.Minute), UpdatedAt: now},
		{RunID: "run_231", Kind: "review", Title: "Review pull request #231", Status: domain.WorkflowRunning, CurrentStep: "domain-review", PRNumber: demoInt(231), StartedAt: now.Add(-5 * time.Minute), UpdatedAt: now},
		{RunID: "run_229", Kind: "ci", Title: "Repair CI #229", Status: domain.WorkflowBlocked, CurrentStep: "Waiting for a person", PRNumber: demoInt(229), StartedAt: now.Add(-21 * time.Minute), UpdatedAt: now},
	}
	agents := []domain.AgentRef{
		{AgentID: "a218", Kind: "claude", Name: "af-implementer-218", WorkflowRunID: "run_218", Status: domain.AgentWorking, Summary: "Editing internal/tui/modal/create.go", PaneID: "w1:p2"},
		{AgentID: "a231", Kind: "claude", Name: "af-pull-request-reviewer-231", WorkflowRunID: "run_231", Status: domain.AgentWorking, Summary: "Checking the API contract", PaneID: "w1:p3"},
		{AgentID: "a229", Kind: "claude", Name: "af-ci-fixer-229", WorkflowRunID: "run_229", Status: domain.AgentBlocked, Summary: "Waiting for a person: approve npm cache path", PaneID: "w1:p4"},
	}
	setState := func(runs []domain.WorkflowRunRef, agents []domain.AgentRef) {
		var items []domain.WorkItem
		for _, run := range runs {
			item := domain.WorkItem{ID: run.RunID, Status: "running", LinkedWorkflows: []domain.WorkflowRunRef{run}}
			for _, a := range agents {
				if a.WorkflowRunID == run.RunID {
					item.LinkedAgents = append(item.LinkedAgents, a)
					item.LinkedPanes = append(item.LinkedPanes, domain.PaneRef{PaneID: a.PaneID, AgentID: a.AgentID})
				}
			}
			items = append(items, item)
		}
		m.missionState = &domain.MissionControlState{
			Status: "running", UpdatedAt: now, WorkflowRuns: runs, Agents: agents, WorkItems: items,
			Integrations: domain.IntegrationStatus{Herdr: connectedIntegration("herdr"), Sandcastle: connectedIntegration("sandcastle"), GitHub: connectedIntegration("github")},
		}
	}
	setState(runs, agents)

	r := &demoRecorder{t: t, m: m, styles: map[demoStyle]int{}, out: demoExport{Version: version.Version, Cols: demoCols, Rows: demoRows}}
	const (
		short = 1100 * time.Millisecond
		mid   = 2200 * time.Millisecond
		long  = 3600 * time.Millisecond
	)

	// Mission control.
	r.capture("Mission control", "grove", "Grove opens on mission control: every workflow, its agents, and the Herdr pane each one runs in.", long)
	r.key("j")
	r.capture("Mission control", "j", "Move through the runs with j and k.", short)
	r.key("j")
	r.capture("Mission control", "j", "A blocked run is waiting for a person. Enter jumps straight to its Herdr pane.", mid)

	// Start a workflow from an issue.
	r.key("i")
	r.capture("Start a workflow", "i", "Issues, with sub-issues shown under their parent.", mid)
	for i := 0; i < 3; i++ {
		r.key("j")
	}
	r.capture("Start a workflow", "j j j", "Select issue #224.", short)
	r.key("a")
	r.capture("Start a workflow", "a", "The Actions panel lists what the selection can do.", mid)
	r.key("j")
	r.capture("Start a workflow", "j", "Implement issue runs the imp workflow in a new Herdr tab.", mid)
	r.key("enter")
	started := now
	run224 := domain.WorkflowRunRef{RunID: "run_224", Kind: "imp", Title: "Implement issue #224", Status: domain.WorkflowRunning, CurrentStep: "planning", IssueNumber: demoInt(224), WorktreePath: "/src/worktrees/grove/fix-issue-224-sync-tick", Branch: "fix/issue-224-sync-tick", DefaultAgent: "claude", Progress: domain.WorkflowProgress{Completed: 0, Total: 10}, Steps: demoRunSteps(started, 0), StartedAt: started, UpdatedAt: started}
	agent224 := domain.AgentRef{AgentID: "a224", Kind: "claude", Name: "af-planner-224", WorkflowRunID: "run_224", Status: domain.AgentWorking, Summary: "Reading the sync loop", PaneID: "w1:p5"}
	setState(append([]domain.WorkflowRunRef{run224}, runs...), append([]domain.AgentRef{agent224}, agents...))
	m.statusMsg = "Started imp workflow"
	r.capture("Start a workflow", "enter", "The run starts in its own Herdr tab, with the agent backend chosen in settings.", mid)
	m.statusMsg = ""
	m.focused = panelList
	r.key("d")
	m.selectedMissionIdx = 0
	r.capture("Start a workflow", "d", "Back on the dashboard, the new run is at the top.", mid)

	// Follow the run in the mission inspector.
	r.key("v")
	r.capture("Mission inspector", "v", "v opens the mission inspector for the selected run.", mid)
	r.key("2")
	inspector, ok := m.activeModal.(*modal.MissionModal)
	require.True(t, ok, "v opens the mission inspector")
	for _, done := range []int{3, 6, 9} {
		run224.Steps = demoRunSteps(started.Add(-time.Duration(done)*3*time.Minute), done)
		run224.StartedAt = run224.Steps[0].StartedAt
		run224.Progress = domain.WorkflowProgress{Completed: done, Total: 10, Percent: done * 10}
		run224.CurrentStep = demoSteps[done]
		inspector.SetWorkflow(run224, []domain.AgentRef{agent224})
		caption := "Stages complete as the run goes: tests, implementation, verification, reviews."
		keys := ""
		if done == 3 {
			keys = "2"
		}
		r.capture("Mission inspector", keys, caption, short+300*time.Millisecond)
	}
	run224.Status = domain.WorkflowSucceeded
	run224.CurrentStep = "Complete"
	run224.Progress = domain.WorkflowProgress{Completed: 10, Total: 10, Percent: 100}
	run224.Steps = demoRunSteps(run224.StartedAt, 10)
	run224.UpdatedAt = now.Add(31 * time.Minute)
	agent224.Status = domain.AgentDone
	agent224.Summary = "Pull request #232 opened and approved"
	inspector.SetWorkflow(run224, []domain.AgentRef{agent224})
	r.key("1")
	r.capture("Mission inspector", "1", "The run succeeded: its pull request is open and its review approved it.", mid)

	// Read what the run wrote.
	r.key("5")
	inspector.SetReports(modal.MissionReportsLoadedMsg{RunID: "run_224", Supported: true, Reports: []domain.WorkflowReport{
		{Title: "Implementation report", Path: "/src/grove/.git/agent-flow/issue-224-implementation-report.md", ModTime: run224.UpdatedAt, Body: demoReport},
		{Title: "Review verdict", Path: "/src/grove/.git/agent-flow/issue-224-review-verdict.md", ModTime: run224.UpdatedAt, Body: demoVerdict},
	}})
	r.capture("Reports", "5", "Reports: what the run wrote about itself, rendered in the terminal.", long)
	r.key("pgdown")
	r.capture("Reports", "PgDn", "Scroll with j, k, PgUp, and PgDn.", mid)
	r.key("]")
	r.capture("Reports", "]", "] switches to the review verdict.", mid)
	r.closeModal()

	// Pick a theme.
	// A scratch home directory, so the settings screen shows the usual
	// ~/.grove/config.toml and saving never touches the real one.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	m.activeModal = modal.NewSettingsModal(m.Config, filepath.Join(home, ".grove", "config.toml"))
	r.capture("Themes", "t", "t opens settings. Appearance lists 23 themes with a live preview.", mid)
	r.key("j")
	r.capture("Themes", "j", "Browsing previews a theme without applying it.", short)
	r.key("j")
	r.capture("Themes", "j", "Cyberpunk.", short)
	r.key("enter")
	m.themeIdx = themeIndexOf(m.Config.Appearance.Theme)
	r.capture("Themes", "enter", "Enter applies it, and the whole screen follows.", mid)
	r.closeModal()
	r.capture("Themes", "esc", "Everything is saved to ~/.grove/config.toml as you go.", long)

	out := os.Getenv("GROVE_DEMO_OUT")
	if out == "" {
		out = filepath.Join("..", "..", "website", "public", "demo", "frames.json")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(out), 0o755))
	data, err := json.Marshal(r.out)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(out, append(data, '\n'), 0o644))
	t.Logf("wrote %d frames, %d styles, %d bytes to %s", len(r.out.Frames), len(r.out.Styles), len(data), out)
}

func themeIndexOf(name string) int {
	for i, theme := range styles.Themes {
		if theme == name {
			return i
		}
	}
	return 0
}
