package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withLabCommonDir points the Lab at a temporary git common directory.
func withLabCommonDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := gitCommonDir
	gitCommonDir = func(string) (string, error) { return dir, nil }
	t.Cleanup(func() { gitCommonDir = orig })
	return dir
}

// runCmd executes cmd and feeds its message back into the model, as the
// Bubble Tea runtime would. Timers such as clearMsgCmd are not run.
func runCmd(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if _, ok := msg.(labsLoadedMsg); !ok {
		return m
	}
	updated, _ := m.Update(msg)
	return updated.(*Model)
}

func press(t *testing.T, m *Model, key string) (*Model, tea.Cmd) {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+s":
		msg = tea.KeyMsg{Type: tea.KeyCtrlS}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	updated, cmd := m.Update(msg)
	return updated.(*Model), cmd
}

func typeInto(t *testing.T, m *Model, text string) *Model {
	t.Helper()
	for _, r := range text {
		if r == '\n' {
			m, _ = press(t, m, "enter")
			continue
		}
		m, _ = press(t, m, string(r))
	}
	return m
}

func newLabModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel()
	m.RepoPath = t.TempDir()
	m.view = viewLab
	m.focused = panelList
	return m
}

func entryAt(t time.Time, id string, kind domain.LabKind, status domain.LabStatus, text string) domain.LabEntry {
	return domain.LabEntry{ID: id, Kind: kind, Status: status, Text: text, Created: t, Updated: t}
}

func TestLabTabOf(t *testing.T) {
	assert.Equal(t, labTabDrafts, labTabOf(domain.LabEntry{Status: domain.LabStatusDraft}))
	assert.Equal(t, labTabActive, labTabOf(domain.LabEntry{Status: domain.LabStatusGrilling}))
	assert.Equal(t, labTabActive, labTabOf(domain.LabEntry{Status: domain.LabStatusShaping}))
	assert.Equal(t, labTabPublished, labTabOf(domain.LabEntry{Status: domain.LabStatusPublished}))
	assert.Equal(t, labTabArchived, labTabOf(domain.LabEntry{Status: domain.LabStatusGrilling, Archived: true}),
		"archiving moves an entry out of its lifecycle tab")
}

func TestLabView_VisibleIsTabAndKindNewestFirst(t *testing.T) {
	base := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	v := newLabView()
	v.entries = []domain.LabEntry{
		entryAt(base, "old-idea", domain.LabKindIdea, domain.LabStatusDraft, "Old idea"),
		entryAt(base.Add(time.Hour), "new-idea", domain.LabKindIdea, domain.LabStatusDraft, "New idea"),
		entryAt(base.Add(2*time.Hour), "bug", domain.LabKindBug, domain.LabStatusDraft, "Bug"),
		entryAt(base, "published", domain.LabKindIdea, domain.LabStatusPublished, "Published"),
	}
	v.setTab(labTabDrafts)

	ids := func(entries []domain.LabEntry) []string {
		var out []string
		for _, e := range entries {
			out = append(out, e.ID)
		}
		return out
	}
	assert.Equal(t, []string{"bug", "new-idea", "old-idea"}, ids(v.visible()))
	v.setFilter(domain.LabFilterIdea)
	assert.Equal(t, []string{"new-idea", "old-idea"}, ids(v.visible()))
	assert.Equal(t, []string{"published"}, ids(v.visibleIn(labTabPublished)))
}

func TestLabView_SetTabWraps(t *testing.T) {
	v := newLabView()
	v.setTab(labTabActive - 1)
	assert.Equal(t, labTabArchived, v.tab)
	v.setTab(labTabArchived + 1)
	assert.Equal(t, labTabActive, v.tab)
}

func TestLabView_SelectIDFollowsEntry(t *testing.T) {
	v := newLabView()
	v.entries = []domain.LabEntry{
		entryAt(time.Now(), "a", domain.LabKindIdea, domain.LabStatusDraft, "A"),
		entryAt(time.Now().Add(time.Minute), "b", domain.LabKindBug, domain.LabStatusDraft, "B"),
	}
	v.setFilter(domain.LabFilterIdea)
	v.selectID("b")

	assert.Equal(t, labTabDrafts, v.tab)
	assert.Equal(t, domain.LabFilterAll, v.filter, "a filter that would hide the entry is cleared")
	e, ok := v.selected()
	require.True(t, ok)
	assert.Equal(t, "b", e.ID)
}

func TestLab_CaptureStoresEntryAndSelectsIt(t *testing.T) {
	commonDir := withLabCommonDir(t)
	m := newLabModel(t)

	m, _ = press(t, m, "c")
	require.IsType(t, &modal.LabCaptureModal{}, m.activeModal)
	m, _ = press(t, m, "tab") // switch to bug
	m = typeInto(t, m, "Sync stalls\n\nWhen the gh token expires.")
	m, cmd := press(t, m, "ctrl+s")
	require.NotNil(t, cmd)
	updated, cmd := m.Update(cmd())
	m = runCmd(t, updated.(*Model), cmd)

	assert.Nil(t, m.activeModal)
	assert.Equal(t, labTabDrafts, m.lab.tab, "the list moves to the new draft")
	e, ok := m.lab.selected()
	require.True(t, ok)
	assert.Equal(t, "Sync stalls", e.Title())
	assert.Equal(t, domain.LabKindBug, e.Kind)
	assert.Contains(t, m.statusMsg, `Captured "Sync stalls"`)

	stored, err := data.NewLabStore(commonDir).Load()
	require.NoError(t, err)
	require.Len(t, stored, 1, "the entry is on disk")
	assert.Equal(t, e.ID, stored[0].ID)
}

func TestLab_CaptureKindFollowsBugFilter(t *testing.T) {
	m := newLabModel(t)
	m, _ = press(t, m, "3")
	m, _ = press(t, m, "c")
	capture, ok := m.activeModal.(*modal.LabCaptureModal)
	require.True(t, ok)
	assert.Equal(t, domain.LabKindBug, capture.Kind())
}

func TestLab_EnterEditsDraft(t *testing.T) {
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	require.NoError(t, store.Put(e))

	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	m.lab.setTab(labTabDrafts)

	m, _ = press(t, m, "enter")
	edit, ok := m.activeModal.(*modal.LabCaptureModal)
	require.True(t, ok, "Enter on a draft opens it for editing")
	assert.Equal(t, "Plugin API", edit.Value())

	m = typeInto(t, m, " with hooks")
	m, cmd := press(t, m, "ctrl+s")
	updated, cmd := m.Update(cmd())
	m = runCmd(t, updated.(*Model), cmd)

	stored, err := store.Load()
	require.NoError(t, err)
	require.Len(t, stored, 1, "editing replaces the entry")
	assert.Equal(t, e.ID, stored[0].ID)
	assert.Equal(t, "Plugin API with hooks", stored[0].Text)
	assert.True(t, stored[0].Updated.After(e.Updated) || stored[0].Updated.Equal(e.Updated))
	assert.Contains(t, m.statusMsg, "Saved")
}

func TestLab_EnterWithNoEntryOpensCapture(t *testing.T) {
	m := newLabModel(t)
	m, _ = press(t, m, "enter")
	assert.IsType(t, &modal.LabCaptureModal{}, m.activeModal)
}

func TestLab_EditRejectsNonDraft(t *testing.T) {
	m := newLabModel(t)
	m.lab.entries = []domain.LabEntry{entryAt(time.Now(), "g", domain.LabKindIdea, domain.LabStatusGrilling, "Grilling")}
	updated, _ := m.handleLabCaptureSubmitted(modal.LabCaptureSubmittedMsg{ID: "g", Kind: domain.LabKindIdea, Text: "changed"})
	assert.Equal(t, "Only drafts can be edited", updated.(*Model).statusErr)
}

func TestLab_ArchiveAndRestore(t *testing.T) {
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	require.NoError(t, store.Put(e))

	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	m.lab.setTab(labTabDrafts)

	updated, cmd := m.handleLabAction(modal.ContextActionLabArchive)
	m = runCmd(t, updated.(*Model), cmd)
	assert.Equal(t, labTabArchived, m.lab.tab, "the list follows the entry to its new tab")
	stored, _ := store.Load()
	assert.True(t, stored[0].Archived)
	assert.Equal(t, domain.LabStatusDraft, stored[0].Status, "archiving keeps the lifecycle state")

	updated, cmd = m.handleLabAction(modal.ContextActionLabRestore)
	m = runCmd(t, updated.(*Model), cmd)
	assert.Equal(t, labTabDrafts, m.lab.tab)
	stored, _ = store.Load()
	assert.False(t, stored[0].Archived)
}

func TestLab_DeleteConfirmsThenRemoves(t *testing.T) {
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	require.NoError(t, store.Put(e))

	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	m.lab.setTab(labTabDrafts)

	updated, _ := m.handleLabAction(modal.ContextActionLabDelete)
	m = updated.(*Model)
	require.IsType(t, &modal.LabDeleteModal{}, m.activeModal)

	m, cmd := press(t, m, "y")
	updated, cmd = m.Update(cmd())
	m = runCmd(t, updated.(*Model), cmd)

	assert.Nil(t, m.activeModal)
	stored, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, stored)
	assert.Empty(t, m.lab.entries)
}

func TestLab_DeleteRefusedWhileInProgress(t *testing.T) {
	m := newLabModel(t)
	m.lab.entries = []domain.LabEntry{entryAt(time.Now(), "g", domain.LabKindIdea, domain.LabStatusGrilling, "Grilling")}
	m.lab.setTab(labTabActive)

	updated, _ := m.handleLabAction(modal.ContextActionLabDelete)
	m = updated.(*Model)
	assert.Nil(t, m.activeModal)
	assert.Contains(t, m.statusErr, "archive it first")
}

func TestLab_BracketsSwitchTabs(t *testing.T) {
	m := newLabModel(t)
	m, _ = press(t, m, "]")
	assert.Equal(t, labTabDrafts, m.lab.tab)
	m, _ = press(t, m, "[")
	m, _ = press(t, m, "[")
	assert.Equal(t, labTabArchived, m.lab.tab)
}

func TestLab_RefreshLoadsEntriesFromStore(t *testing.T) {
	commonDir := withLabCommonDir(t)
	require.NoError(t, data.NewLabStore(commonDir).Put(domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())))

	m := newLabModel(t)
	updated, _ := m.Update(worktreesRefreshedMsg{worktrees: []domain.Worktree{{Path: m.RepoPath}}, labs: mustLoad(t, commonDir)})
	m = updated.(*Model)
	require.Len(t, m.lab.entries, 1)
	assert.Equal(t, "Plugin API", m.lab.entries[0].Title())
}

func TestLab_RefreshReportsStoreError(t *testing.T) {
	m := newLabModel(t)
	updated, _ := m.Update(worktreesRefreshedMsg{labsErr: assert.AnError})
	assert.Equal(t, assert.AnError.Error(), updated.(*Model).statusErr)
}

func mustLoad(t *testing.T, commonDir string) []domain.LabEntry {
	t.Helper()
	entries, err := data.NewLabStore(commonDir).Load()
	require.NoError(t, err)
	return entries
}

func TestRenderLab_ShowsTabsCountsAndRows(t *testing.T) {
	now := time.Now()
	v := newLabView()
	v.entries = []domain.LabEntry{
		entryAt(now, "a", domain.LabKindIdea, domain.LabStatusDraft, "Plugin API\n\ndetails"),
		entryAt(now, "b", domain.LabKindBug, domain.LabStatusDraft, "Sync stalls"),
		entryAt(now, "c", domain.LabKindIdea, domain.LabStatusPublished, "Shipped"),
	}
	v.setTab(labTabDrafts)
	out := renderLab(v, styles.NewTheme(styles.Themes[0]), 110, 0, true)

	assert.Contains(t, out, "[ DRAFTS 02 ]")
	assert.Contains(t, out, "[ PUBLISHED 01 ]")
	assert.Contains(t, out, "Plugin API")
	assert.Contains(t, out, "Sync stalls")
	assert.NotContains(t, out, "Shipped", "other tabs' entries are not listed")
	assert.NotContains(t, out, "details", "the list shows titles, not bodies")
	assert.Contains(t, out, "DRAFT")
}

func TestRenderLab_EmptyTabExplainsWhatToDo(t *testing.T) {
	v := newLabView()
	v.setTab(labTabDrafts)
	out := renderLab(v, styles.NewTheme(styles.Themes[0]), 110, 0, true)
	assert.Contains(t, out, "Press c to capture")
}

func TestRenderLab_FitsPanelHeight(t *testing.T) {
	v := newLabView()
	for i := 0; i < 30; i++ {
		v.entries = append(v.entries, entryAt(time.Now().Add(time.Duration(i)*time.Minute), string(rune('a'+i)), domain.LabKindIdea, domain.LabStatusDraft, "Entry"))
	}
	v.setTab(labTabDrafts)
	v.cursor = 29
	const panelHeight = 12
	out := renderLab(v, styles.NewTheme(styles.Themes[0]), 80, panelHeight, true)
	assert.LessOrEqual(t, strings.Count(out, "\n")+1, panelHeight+2, "the panel never grows past its height")
	assert.Contains(t, out, "> ", "the selected row stays visible")
}

func TestRenderLabContext(t *testing.T) {
	epic := 251
	e := entryAt(time.Now(), "a", domain.LabKindIdea, domain.LabStatusPublished, "Plugin API\n\nLet users extend views.")
	e.Issues = domain.LabIssues{Epic: &epic, Tickets: []int{252, 253}}
	out := renderLabContext(newLabView(), e, 40, time.Now())

	assert.Contains(t, out, "Context: Lab idea")
	assert.Contains(t, out, "Plugin API")
	assert.Contains(t, out, "Status: PUBLISHED")
	assert.Contains(t, out, "Issues: #251 + 2 tickets")
	assert.Contains(t, out, "Let users extend views.")
}

func TestRenderFull_LabContextPanelShowsEntryNotWorktree(t *testing.T) {
	m := newLabModel(t)
	m.width, m.height = 160, 40
	m.Worktrees = []domain.Worktree{{Path: "/repo/worktree-name", Branch: "feature-branch"}}
	m.lab.entries = []domain.LabEntry{entryAt(time.Now(), "a", domain.LabKindBug, domain.LabStatusDraft, "Sync stalls")}
	m.lab.setTab(labTabDrafts)

	out := m.View()
	assert.Contains(t, out, "Context: Lab bug")
	assert.NotContains(t, out, "feature-branch", "the Lab's context panel describes the entry")
}

// The Lab lives in the git common directory, so a linked worktree reads the
// entries captured from the main checkout. This runs real git.
func TestLab_WorktreesShareOneLab(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	mainDir := filepath.Join(root, "main")
	linked := filepath.Join(root, "linked")
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.MkdirAll(mainDir, 0o755))
	git(mainDir, "init", "-q")
	git(mainDir, "commit", "-q", "--allow-empty", "-m", "init")
	git(mainDir, "worktree", "add", "-q", "-b", "feature", linked)

	fromMain, err := labStoreFor(mainDir)
	require.NoError(t, err)
	require.NoError(t, fromMain.Put(domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())))

	entries, err := loadLabs(linked)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Plugin API", entries[0].Title())
	assert.True(t, filepath.IsAbs(fromMain.Dir()), "the Lab path does not depend on Grove's working directory")

	out, err := exec.Command("git", "-C", mainDir, "status", "--porcelain").Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)), "git never sees the Lab")
}

// labMissionState reports one run per status, each with an agent in a pane.
func labMissionState(runs map[string]string) *domain.MissionControlState {
	state := &domain.MissionControlState{}
	for id, status := range runs {
		state.WorkflowRuns = append(state.WorkflowRuns, domain.WorkflowRunRef{
			RunID: id, Kind: "grill", Status: status, CurrentStep: "Spec", StartedAt: time.Now().Add(-time.Hour),
		})
		state.Agents = append(state.Agents, domain.AgentRef{AgentID: "agent-" + id, WorkflowRunID: id, PaneID: "pane-" + id})
	}
	return state
}

func TestLabView_RunStateDrivesRows(t *testing.T) {
	now := time.Now()
	waiting := entryAt(now.Add(-time.Hour), "waiting", domain.LabKindIdea, domain.LabStatusGrilling, "Waiting entry")
	waiting.Runs = []string{"run-w"}
	running := entryAt(now, "running", domain.LabKindIdea, domain.LabStatusSpecced, "Running entry")
	running.Runs = []string{"run-r"}
	idle := entryAt(now, "idle", domain.LabKindIdea, domain.LabStatusTicketed, "Idle entry")

	v := newLabView()
	v.entries = []domain.LabEntry{waiting, running, idle}
	v.setMission(labMissionState(map[string]string{"run-w": domain.WorkflowBlocked, "run-r": domain.WorkflowRunning}))

	assert.Equal(t, "waiting", v.visibleIn(labTabActive)[0].ID, "an entry waiting for the user sorts first despite being older")

	s := labStateOf(v, waiting)
	assert.Equal(t, "◆", s.marker)
	assert.Equal(t, "WAITING ON YOU", s.badge)
	assert.Equal(t, labToneAttention, s.tone)

	assert.Equal(t, "SPECCED", labStateOf(v, running).badge)
	assert.Equal(t, "Spec 2/4", labStageLabel(v, running), "a live run's current step is shown")
	assert.Contains(t, labDetail(v, running, now), "Herdr pane-run-r")
	assert.Equal(t, "Tickets 3/4", labStageLabel(v, idle), "without a run the stored stage is shown")
	assert.NotContains(t, labDetail(v, idle, now), "Herdr")
}

func TestLabView_FinishedRunDoesNotOverrideStatus(t *testing.T) {
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusTicketed, "Entry")
	e.Runs = []string{"run-1"}
	v := newLabView()
	v.entries = []domain.LabEntry{e}
	v.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowSucceeded}))

	assert.Equal(t, "TICKETED", labStateOf(v, e).badge)
	assert.Equal(t, "Tickets 3/4", labStageLabel(v, e))
	assert.NotContains(t, labDetail(v, e, time.Now()), "Herdr", "a finished run's pane is not offered")
}

func TestLab_MissionUpdateRefreshesRowsAndInspector(t *testing.T) {
	m := newLabModel(t)
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusGrilling, "Entry")
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setTab(labTabActive)

	updated, _ := m.openLabInspector()
	m = updated.(*Model)
	inspector, ok := m.activeModal.(*modal.LabInspectorModal)
	require.True(t, ok)
	assert.NotContains(t, inspector.View(), "WAITING ON YOU")

	updated, _ = m.Update(missionControlUpdatedMsg{state: *labMissionState(map[string]string{"run-1": domain.WorkflowBlocked})})
	m = updated.(*Model)
	assert.True(t, m.lab.waiting(e))
	assert.Contains(t, m.activeModal.View(), "WAITING ON YOU", "an open inspector follows its run")
}

func TestLab_VOpensInspectorAndLoadsArtifacts(t *testing.T) {
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	require.NoError(t, store.Put(e))
	require.NoError(t, os.MkdirAll(store.ArtifactsDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.ArtifactsDir(e.ID), "spec.md"), []byte("# Plugin API spec"), 0o644))

	m := newLabModel(t)
	m.width, m.height = 140, 40
	m.lab.entries, _ = store.Load()
	m.lab.setTab(labTabDrafts)

	m, cmd := press(t, m, "v")
	inspector, ok := m.activeModal.(*modal.LabInspectorModal)
	require.True(t, ok, "v opens the inspector")
	require.NotNil(t, cmd, "opening asks for the artifacts")

	// The request goes to Grove, which reads the files and hands them back.
	updated, load := m.Update(cmd())
	m = updated.(*Model)
	require.NotNil(t, load)
	updated, _ = m.Update(load())
	m = updated.(*Model)

	m, _ = press(t, m, "3")
	view := inspector.View()
	assert.Contains(t, view, "spec.md")
	assert.Contains(t, view, "Plugin API spec")
}

func TestLab_EnterOnLiveRunFocusesItsPane(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m := newLabModel(t)
	m.herdrNavigator = navigator
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusGrilling, "Entry")
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))
	m.lab.setTab(labTabActive)

	m, cmd := press(t, m, "enter")
	require.NotNil(t, cmd)
	cmd()
	assert.Equal(t, "pane-run-1", navigator.focusedPane, "Enter takes the user to the waiting agent")
	assert.Nil(t, m.activeModal)
}

func TestLab_EnterWithoutLiveRunOpensInspector(t *testing.T) {
	m := newLabModel(t)
	m.lab.entries = []domain.LabEntry{entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusTicketed, "Entry")}
	m.lab.setTab(labTabActive)

	m, _ = press(t, m, "enter")
	assert.IsType(t, &modal.LabInspectorModal{}, m.activeModal)
}

func TestLab_InspectorClosesWhenEntryIsDeleted(t *testing.T) {
	m := newLabModel(t)
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusDraft, "Entry")
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setTab(labTabDrafts)
	updated, _ := m.openLabInspector()
	m = updated.(*Model)

	updated, _ = m.Update(labsLoadedMsg{entries: nil})
	assert.Nil(t, updated.(*Model).activeModal)
}

// A row's title and badge share one line, and badges line up in one column,
// however long the badge.
func TestRenderLab_RowsFitAndBadgesAlign(t *testing.T) {
	now := time.Now()
	waiting := entryAt(now, "w", domain.LabKindIdea, domain.LabStatusGrilling, "Offline mode for the dashboard")
	waiting.Runs = []string{"run-w"}
	shaping := entryAt(now.Add(-time.Minute), "s", domain.LabKindBug, domain.LabStatusShaping, "Sync stalls")
	v := newLabView()
	v.entries = []domain.LabEntry{waiting, shaping}
	v.setMission(labMissionState(map[string]string{"run-w": domain.WorkflowBlocked}))
	v.setTab(labTabActive)

	out := ansi.Strip(renderLab(v, styles.NewTheme(styles.Themes[0]), 77, 20, true))
	lines := strings.Split(out, "\n")
	column := func(title, badge string) int {
		for _, line := range lines {
			if strings.Contains(line, title) {
				require.Contains(t, line, badge, "the badge is on the title's line")
				return strings.Index(line, badge)
			}
		}
		t.Fatalf("row %q not rendered", title)
		return -1
	}
	assert.Equal(t, column("Offline mode for the dashboard", "WAITING ON YOU"), column("Sync stalls", "SHAPING"))
}

func TestLabContextActions_DependOnStateAndSession(t *testing.T) {
	labels := func(v labView) []string {
		var out []string
		for _, a := range labContextActions(v) {
			out = append(out, a.label)
		}
		return out
	}
	viewOf := func(e domain.LabEntry, runs map[string]string) labView {
		v := newLabView()
		v.entries = []domain.LabEntry{e}
		v.setMission(labMissionState(runs))
		v.setTab(labTabOf(e))
		return v
	}
	now := time.Now()

	assert.Equal(t, []string{"Capture new entry"}, labels(newLabView()))
	assert.Equal(t, []string{"Inspect", "Edit", "Archive", "Delete", "Capture new entry"},
		labels(viewOf(entryAt(now, "i", domain.LabKindIdea, domain.LabStatusDraft, "Idea"), nil)))
	assert.Equal(t, []string{"Shape into an issue", "Inspect", "Edit", "Archive", "Delete", "Capture new entry"},
		labels(viewOf(entryAt(now, "b", domain.LabKindBug, domain.LabStatusDraft, "Bug"), nil)), "a bug draft can be shaped")

	shaping := entryAt(now, "s", domain.LabKindBug, domain.LabStatusShaping, "Bug")
	shaping.Runs = []string{"run-1"}
	assert.Equal(t, []string{"Inspect", "End session", "Archive", "Capture new entry"},
		labels(viewOf(shaping, map[string]string{"run-1": domain.WorkflowBlocked})), "a live session is ended, not restarted")
	assert.Equal(t, []string{"Resume shaping", "Inspect", "Archive", "Capture new entry"},
		labels(viewOf(shaping, map[string]string{"run-1": domain.WorkflowSucceeded})), "an ended session can be resumed")

	archived := entryAt(now, "a", domain.LabKindBug, domain.LabStatusDraft, "Bug")
	archived.Archived = true
	assert.Equal(t, []string{"Inspect", "Restore", "Delete", "Capture new entry"}, labels(viewOf(archived, nil)))
}

// fakeLabStarter records the sessions Grove starts.
type fakeLabStarter struct {
	requests []sandcastle.StartWorkflowRequest
	err      error
}

func (f *fakeLabStarter) StartWorkflow(_ context.Context, req sandcastle.StartWorkflowRequest) (domain.WorkflowRunRef, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return domain.WorkflowRunRef{}, f.err
	}
	return domain.WorkflowRunRef{RunID: "run-shape-1", Kind: req.Kind, Status: domain.WorkflowQueued}, nil
}

func (f *fakeLabStarter) RemoveWorkflow(context.Context, string, string, bool) error { return nil }

func shapeModel(t *testing.T) (*Model, *data.LabStore, *fakeLabStarter, domain.LabEntry) {
	t.Helper()
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindBug, "Sync stalls when the gh token expires", time.Now())
	require.NoError(t, store.Put(e))

	starter := &fakeLabStarter{}
	m := newLabModel(t)
	m.workflowStarter = starter
	m.Config.Sandcastle.Enabled = true
	m.Config.Sandcastle.DefaultAgent = "claude"
	m.lab.entries, _ = store.Load()
	m.lab.setTab(labTabDrafts)
	return m, store, starter, e
}

func TestLab_EnterOnBugDraftStartsShapingSession(t *testing.T) {
	m, store, starter, e := shapeModel(t)

	m, cmd := press(t, m, "enter")
	require.NotNil(t, cmd)
	updated, _ := m.Update(cmd())
	m = updated.(*Model)

	require.Len(t, starter.requests, 1)
	req := starter.requests[0]
	assert.Equal(t, "shape", req.Kind)
	assert.Equal(t, e.ID, req.EntryID)
	assert.Equal(t, "claude", req.AgentKind, "the configured default agent is used")
	assert.Equal(t, m.RepoPath, req.RepoPath)

	stored, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, domain.LabStatusShaping, stored[0].Status)
	assert.Equal(t, domain.LabModeShape, stored[0].Mode)
	assert.Equal(t, []string{"run-shape-1"}, stored[0].Runs)
	assert.Equal(t, labTabActive, m.lab.tab, "the list follows the entry into Active")
	assert.NoFileExists(t, filepath.Join(store.EntryDir(e.ID), "session.lock"), "the lock is released after the start")
	assert.Contains(t, m.statusMsg, "answer the agent in its Herdr pane")
}

func TestLab_ShapeRefusedWhileAnotherGroveHoldsTheEntry(t *testing.T) {
	m, store, starter, e := shapeModel(t)
	require.NoError(t, os.MkdirAll(store.EntryDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.lock"),
		[]byte(`{"pid":1,"host":"another-machine","since":"2026-09-28T08:00:00Z"}`), 0o644))

	updated, cmd := m.startLabShape(e)
	m = updated.(*Model)
	updated, _ = m.Update(cmd())
	m = updated.(*Model)

	assert.Empty(t, starter.requests, "no second session is started")
	assert.Equal(t, "Entry is in use by Grove on another-machine (pid 1)", m.statusErr)
	stored, _ := store.Load()
	assert.Equal(t, domain.LabStatusDraft, stored[0].Status)
}

func TestLab_ShapeStartFailureLeavesDraft(t *testing.T) {
	m, store, starter, e := shapeModel(t)
	starter.err = fmt.Errorf("herdr is not running")

	updated, cmd := m.startLabShape(e)
	updated, _ = updated.(*Model).Update(cmd())
	m = updated.(*Model)

	assert.Contains(t, m.statusErr, "herdr is not running")
	stored, _ := store.Load()
	assert.Equal(t, domain.LabStatusDraft, stored[0].Status)
	assert.Empty(t, stored[0].Runs)
}

func TestLab_ShapeRequiresSandcastle(t *testing.T) {
	m, _, starter, e := shapeModel(t)
	m.Config.Sandcastle.Enabled = false
	updated, _ := m.startLabShape(e)
	assert.Contains(t, updated.(*Model).statusErr, "Sandcastle is disabled")
	assert.Empty(t, starter.requests, "nothing is started")
}

func TestLab_EndSessionWritesCloseMarker(t *testing.T) {
	m, store, _, e := shapeModel(t)
	e.Status = domain.LabStatusShaping
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))
	m.lab.setTab(labTabActive)

	updated, cmd := m.handleLabAction(modal.ContextActionLabEnd)
	m = runCmd(t, updated.(*Model), cmd)
	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "session.close"))
	assert.Contains(t, m.statusMsg, "Ending the session")
}
