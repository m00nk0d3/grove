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
	orig, origMap := gitCommonDir, ensureLabRepoMap
	gitCommonDir = func(string) (string, error) { return dir, nil }
	ensureLabRepoMap = func(*data.LabStore, string, int) error { return nil }
	t.Cleanup(func() { gitCommonDir, ensureLabRepoMap = orig, origMap })
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

func TestLabView_GroupsEntriesByWhatTheyNeed(t *testing.T) {
	now := time.Now()
	v := newLabView()
	live := entryAt(now, "live", domain.LabKindIdea, domain.LabStatusGrilling, "Live")
	live.Runs = []string{"run-live"}
	blocked := entryAt(now, "blocked", domain.LabKindIdea, domain.LabStatusGrilling, "Blocked")
	blocked.Runs = []string{"run-blocked"}
	failed := entryAt(now, "failed", domain.LabKindIdea, domain.LabStatusGrilling, "Failed")
	failed.Runs = []string{"run-failed"}
	v.setMission(labMissionState(map[string]string{
		"run-live": domain.WorkflowRunning, "run-blocked": domain.WorkflowBlocked, "run-failed": domain.WorkflowFailed,
	}))

	assert.Equal(t, labGroupNotStarted, v.groupOf(domain.LabEntry{Status: domain.LabStatusDraft}))
	assert.Equal(t, labGroupWorking, v.groupOf(live))
	assert.Equal(t, labGroupNeedsYou, v.groupOf(blocked), "an agent waiting on the user")
	assert.Equal(t, labGroupNeedsYou, v.groupOf(failed), "a failed session, to retry")
	assert.Equal(t, labGroupNeedsYou, v.groupOf(domain.LabEntry{Status: domain.LabStatusTicketed}),
		"a session that stopped before publishing, to publish or resume")
	assert.Equal(t, labGroupDone, v.groupOf(domain.LabEntry{Status: domain.LabStatusPublished}))
	assert.Equal(t, labGroupArchived, v.groupOf(domain.LabEntry{Status: domain.LabStatusGrilling, Archived: true}))
}

func TestLabView_NeedsYouPutsTheLongestWaitFirst(t *testing.T) {
	now := time.Now()
	v := newLabView()
	recent := entryAt(now, "recent", domain.LabKindIdea, domain.LabStatusTicketed, "Recent")
	recent.Updated = now
	old := entryAt(now.Add(time.Hour), "old", domain.LabKindIdea, domain.LabStatusTicketed, "Old")
	old.Updated = now.Add(-time.Hour)
	fresh := entryAt(now.Add(2*time.Hour), "fresh", domain.LabKindIdea, domain.LabStatusDraft, "Fresh")
	v.entries = []domain.LabEntry{recent, fresh, old}

	var ids []string
	for _, e := range v.visible() {
		ids = append(ids, e.ID)
	}
	assert.Equal(t, []string{"old", "recent", "fresh"}, ids, "NEEDS YOU first, longest waiting first, then NOT STARTED")
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

	ids := func(entries []domain.LabEntry) []string {
		var out []string
		for _, e := range entries {
			out = append(out, e.ID)
		}
		return out
	}
	assert.Equal(t, []string{"bug", "new-idea", "old-idea", "published"}, ids(v.visible()))
	v.setFilter(domain.LabFilterIdea)
	assert.Equal(t, []string{"new-idea", "old-idea", "published"}, ids(v.visible()))
}

func TestLabView_DoneShowsTheLatestUntilExpanded(t *testing.T) {
	m := newLabModel(t)
	base := time.Now()
	for i := 0; i < labDoneShown+2; i++ {
		m.lab.entries = append(m.lab.entries, entryAt(base.Add(time.Duration(i)*time.Minute), fmt.Sprintf("p%d", i), domain.LabKindIdea, domain.LabStatusPublished, "Shipped"))
	}
	items := m.lab.items()
	require.Len(t, items, labDoneShown+1)
	assert.Equal(t, 2, items[labDoneShown].more, "the rest are behind one row")
	assert.Equal(t, "p6", items[0].entry.ID, "the newest first")
	assert.Contains(t, renderLab(m.lab, styles.NewTheme(styles.Themes[0]), 110, 0, true), "DONE  7")

	m.lab.cursor = labDoneShown
	m, _ = press(t, m, "enter")
	assert.Nil(t, m.lab.page, "Enter on the row shows the rest instead of opening a page")
	assert.Len(t, m.lab.items(), labDoneShown+2)

	m.lab.doneExpanded = false
	m.lab.selectID("p0")
	e, ok := m.lab.selected()
	require.True(t, ok)
	assert.Equal(t, "p0", e.ID, "selecting a hidden entry shows it")
}

func TestLab_ZeroTogglesTheArchive(t *testing.T) {
	m := newLabModel(t)
	m.lab.entries = []domain.LabEntry{
		entryAt(time.Now(), "active", domain.LabKindIdea, domain.LabStatusDraft, "Active"),
		{ID: "gone", Kind: domain.LabKindIdea, Status: domain.LabStatusDraft, Text: "Gone", Archived: true},
	}
	m, _ = press(t, m, "0")
	assert.True(t, m.lab.archived)
	e, _ := m.lab.selected()
	assert.Equal(t, "gone", e.ID)
	assert.Contains(t, renderLab(m.lab, styles.NewTheme(styles.Themes[0]), 110, 0, true), "LAB › ARCHIVE")
	m, _ = press(t, m, "0")
	assert.False(t, m.lab.archived)
}

func TestLabView_SelectIDFollowsEntry(t *testing.T) {
	v := newLabView()
	v.entries = []domain.LabEntry{
		entryAt(time.Now(), "a", domain.LabKindIdea, domain.LabStatusDraft, "A"),
		entryAt(time.Now().Add(time.Minute), "b", domain.LabKindBug, domain.LabStatusDraft, "B"),
	}
	v.setFilter(domain.LabFilterIdea)
	v.selectID("b")

	assert.False(t, v.archived)
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
	assert.False(t, m.lab.archived, "the list shows the new draft")
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

func TestLab_EditActionEditsDraft(t *testing.T) {
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindIdea, "Plugin API", time.Now())
	require.NoError(t, store.Put(e))

	m := newLabModel(t)
	m.lab.entries, _ = store.Load()

	updated, _ := m.handleLabAction(modal.ContextActionLabEdit)
	m = updated.(*Model)
	edit, ok := m.activeModal.(*modal.LabCaptureModal)
	require.True(t, ok, "Edit opens a draft for editing")
	assert.Equal(t, "Plugin API", edit.Value())

	m = typeInto(t, m, " with hooks")
	m, cmd := press(t, m, "ctrl+s")
	updated, cmd = m.Update(cmd())
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

	updated, cmd := m.handleLabAction(modal.ContextActionLabArchive)
	m = runCmd(t, updated.(*Model), cmd)
	assert.True(t, m.lab.archived, "the list follows the entry into the archive")
	stored, _ := store.Load()
	assert.True(t, stored[0].Archived)
	assert.Equal(t, domain.LabStatusDraft, stored[0].Status, "archiving keeps the lifecycle state")

	updated, cmd = m.handleLabAction(modal.ContextActionLabRestore)
	m = runCmd(t, updated.(*Model), cmd)
	assert.False(t, m.lab.archived, "restoring returns to the active entries")
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

	updated, _ := m.handleLabAction(modal.ContextActionLabDelete)
	m = updated.(*Model)
	assert.Nil(t, m.activeModal)
	assert.Contains(t, m.statusErr, "archive it first")
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

func TestRenderLab_ShowsGroupsCountsAndRows(t *testing.T) {
	now := time.Now()
	v := newLabView()
	v.entries = []domain.LabEntry{
		entryAt(now, "a", domain.LabKindIdea, domain.LabStatusDraft, "Plugin API\n\ndetails"),
		entryAt(now, "b", domain.LabKindBug, domain.LabStatusDraft, "Sync stalls"),
		entryAt(now, "c", domain.LabKindIdea, domain.LabStatusPublished, "Shipped"),
	}
	out := renderLab(v, styles.NewTheme(styles.Themes[0]), 110, 0, true)

	assert.Contains(t, out, "NOT STARTED  2")
	assert.Contains(t, out, "DONE  1")
	assert.NotContains(t, out, "NEEDS YOU", "an empty group has no heading")
	assert.Contains(t, out, "Plugin API")
	assert.Contains(t, out, "Sync stalls")
	assert.Contains(t, out, "Shipped", "every group is on one list")
	assert.Less(t, strings.Index(out, "Sync stalls"), strings.Index(out, "Shipped"), "groups come in order")
	assert.NotContains(t, out, "details", "the list shows titles, not bodies")
	assert.Contains(t, out, "IDEA TO GRILL")
	assert.Contains(t, out, "BUG TO REPORT")
	assert.Contains(t, out, "↵ Start grill")
	assert.Contains(t, out, "↵ Shape bug")
}

func TestRenderLab_EmptyTabExplainsWhatToDo(t *testing.T) {
	v := newLabView()
	out := renderLab(v, styles.NewTheme(styles.Themes[0]), 110, 0, true)
	assert.Contains(t, out, "Press c to capture")
}

func TestRenderLab_FitsPanelHeight(t *testing.T) {
	v := newLabView()
	for i := 0; i < 30; i++ {
		v.entries = append(v.entries, entryAt(time.Now().Add(time.Duration(i)*time.Minute), string(rune('a'+i)), domain.LabKindIdea, domain.LabStatusDraft, "Entry"))
	}
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

	assert.Contains(t, out, "Lab idea")
	assert.Contains(t, out, "Plugin API")
	assert.Contains(t, out, "NEXT\nOpen issue #251")
	assert.Contains(t, out, "Status: PUBLISHED")
	assert.Contains(t, out, "Issues: #251 + 2 tickets")
	assert.Contains(t, out, "Let users extend views.")
}

func TestRenderFull_LabContextPanelShowsEntryNotWorktree(t *testing.T) {
	m := newLabModel(t)
	m.width, m.height = 160, 40
	m.Worktrees = []domain.Worktree{{Path: "/repo/worktree-name", Branch: "feature-branch"}}
	m.lab.entries = []domain.LabEntry{entryAt(time.Now(), "a", domain.LabKindBug, domain.LabStatusDraft, "Sync stalls")}

	out := m.View()
	assert.Contains(t, out, "Lab bug")
	assert.NotContains(t, out, "feature-branch", "the Lab's context panel describes the entry")
}

func TestLabNextAction_ExplainsWhatEnterWillDo(t *testing.T) {
	now := time.Now()
	viewOf := func(e domain.LabEntry, runs map[string]string) labView {
		v := newLabView()
		v.entries = []domain.LabEntry{e}
		v.setMission(labMissionState(runs))
		return v
	}

	idea := entryAt(now, "idea", domain.LabKindIdea, domain.LabStatusDraft, "Idea")
	assert.Equal(t, "Start grill", labNextAction(viewOf(idea, nil), idea))

	bug := entryAt(now, "bug", domain.LabKindBug, domain.LabStatusDraft, "Bug")
	assert.Equal(t, "Shape bug", labNextAction(viewOf(bug, nil), bug))

	working := entryAt(now, "working", domain.LabKindIdea, domain.LabStatusGrilling, "Working")
	working.Runs = []string{"run-1"}
	assert.Equal(t, "Answer the agent in its pane", labNextAction(viewOf(working, map[string]string{"run-1": domain.WorkflowBlocked}), working),
		"a session waiting on something Grove has no card for is answered in the pane")
	assert.Equal(t, "Writing the spec…", labNextAction(viewOf(working, map[string]string{"run-1": domain.WorkflowRunning}), working),
		"a working session says what its agent is doing")
	failed := viewOf(working, map[string]string{"run-1": domain.WorkflowFailed})
	failed.runs["run-1"] = labRun{workflow: domain.WorkflowRunRef{RunID: "run-1", Status: domain.WorkflowFailed}}
	assert.Equal(t, "Retry grill", labNextAction(failed, working))

	review := entryAt(now, "review", domain.LabKindBug, domain.LabStatusShaping, "Review")
	review.Mode = domain.LabModeShape
	assert.Equal(t, "Resume shaping", labNextAction(viewOf(review, nil), review), "a report never written is shaped again")
	drafted := viewOf(review, nil)
	drafted.talks = map[string]labTalk{review.ID: {artifacts: []domain.LabArtifact{{Path: "issue.md", Body: "# Bug\n"}}}}
	assert.Equal(t, "Review the bug report", labNextAction(drafted, review))
	review.SetReview(domain.LabArtifact{Path: "issue.md", Body: "# Bug\n"}, domain.LabReviewApproved)
	assert.Equal(t, "Publish the bug report", labNextAction(drafted, review))

	issue := 42
	published := entryAt(now, "published", domain.LabKindBug, domain.LabStatusPublished, "Published")
	published.Issues.Issue = &issue
	assert.Equal(t, "Open issue #42", labNextAction(viewOf(published, nil), published))

	idea.Archived = true
	assert.Equal(t, "Restore from Actions to continue", labNextAction(viewOf(idea, nil), idea))
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
		paneID := "pane-" + id
		state.WorkflowRuns = append(state.WorkflowRuns, domain.WorkflowRunRef{
			RunID: id, Kind: "grill", Status: status, CurrentStep: "Spec", StartedAt: time.Now().Add(-time.Hour),
		})
		state.Agents = append(state.Agents, domain.AgentRef{AgentID: "agent-" + id, WorkflowRunID: id, PaneID: paneID})
		state.Panes = append(state.Panes, domain.PaneRef{PaneID: paneID})
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

	assert.Equal(t, "waiting", v.visible()[0].ID, "an entry waiting for the user sorts first despite being older")

	s := labStateOf(v, waiting)
	assert.Equal(t, "◆", s.marker)
	assert.Equal(t, "WAITING ON YOU", s.badge)
	assert.Equal(t, labToneAttention, s.tone)

	assert.Equal(t, "SPECCED", labStateOf(v, running).badge)
	assert.Equal(t, "Spec 3/5", labStageLabel(v, running), "a live run's current step is shown")
	assert.NotContains(t, labDetail(v, running, now), "Herdr", "rows do not point at the pane; View agent does")
	assert.Equal(t, "Tickets 4/5", labStageLabel(v, idle), "without a run the stored stage is shown")
	assert.NotContains(t, labDetail(v, idle, now), "Herdr")
}

func TestLabView_FinishedRunLeavesTheEntryWaitingOnTheUser(t *testing.T) {
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusTicketed, "Entry")
	e.Runs = []string{"run-1"}
	v := newLabView()
	v.entries = []domain.LabEntry{e}
	v.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowSucceeded}))

	assert.Equal(t, labGroupNeedsYou, v.groupOf(e), "drafts ready to publish need the user")
	assert.Equal(t, "WAITING ON YOU", labStateOf(v, e).badge)

	unknown := v
	unknown.setMission(nil)
	assert.Equal(t, "TICKETED", labStateOf(unknown, e).badge, "a run Grove has not heard of does not claim to wait")
	assert.Equal(t, "Tickets 4/5", labStageLabel(v, e))
	assert.NotContains(t, labDetail(v, e, time.Now()), "Herdr", "a finished run's pane is not offered")
}

func TestLabView_StalePaneDoesNotKeepSessionLive(t *testing.T) {
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusGrilling, "Entry")
	e.Mode = domain.LabModeGrill
	e.Runs = []string{"run-1"}
	state := labMissionState(map[string]string{"run-1": domain.WorkflowBlocked})
	state.Panes = nil

	v := newLabView()
	v.entries = []domain.LabEntry{e}
	v.setMission(state)

	run, ok := v.latestRun(e)
	require.True(t, ok)
	assert.Empty(t, run.paneID)
	assert.False(t, v.hasLiveSession(e), "Sandcastle telemetry cannot keep a session live after its Herdr pane is gone")
	assert.True(t, v.canGrill(e))
	assert.Equal(t, "Resume grill", labNextAction(v, e))
	assert.NotContains(t, labDetail(v, e, time.Now()), "Herdr")
}

func TestLab_MissionUpdateRefreshesRows(t *testing.T) {
	m := newLabModel(t)
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusGrilling, "Entry")
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}

	theme := styles.NewTheme(styles.Themes[0])
	assert.NotContains(t, renderLab(m.lab, theme, 110, 0, true), "WAITING ON YOU")

	updated, _ := m.Update(missionControlUpdatedMsg{state: *labMissionState(map[string]string{"run-1": domain.WorkflowBlocked})})
	m = updated.(*Model)
	assert.True(t, m.lab.waiting(e))
	assert.Contains(t, renderLab(m.lab, theme, 110, 0, true), "WAITING ON YOU", "the row follows its run")
}

func TestLab_EnterOnLiveRunFocusesItsPane(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m := newLabModel(t)
	m.herdrNavigator = navigator
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusGrilling, "Entry")
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))

	m, _ = press(t, m, "enter")
	require.NotNil(t, m.lab.page, "Enter on a row opens the entry page")
	m, cmd := press(t, m, "enter")
	require.NotNil(t, cmd)
	cmd()
	assert.Equal(t, "pane-run-1", navigator.focusedPane, "Enter on the page takes the user to the waiting agent")
	assert.Nil(t, m.activeModal)
}

func TestLab_AStoppedSessionWithoutItsDraftsIsResumed(t *testing.T) {
	m := newLabModel(t)
	ended := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusSpecced, "Entry")
	ended.Mode = domain.LabModeGrill
	m.lab.entries = []domain.LabEntry{ended}

	_, reviewing := m.lab.reviewing(ended)
	assert.False(t, reviewing, "a draft never written is not reviewed")
	assert.False(t, m.lab.publishable(ended))
	assert.Equal(t, "Resume grill", labNextAction(m.lab, ended))
}

func TestLab_PageClosesWhenEntryIsDeleted(t *testing.T) {
	m := newLabModel(t)
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusDraft, "Entry")
	m.lab.entries = []domain.LabEntry{e}
	m.openLabPage(e)

	updated, _ := m.Update(labsLoadedMsg{entries: nil})
	assert.Nil(t, updated.(*Model).lab.page)
}

// A row's title and badge share one line, and badges line up in one column,
// however long the badge.
func TestRenderLab_RowsFitAndBadgesAlign(t *testing.T) {
	now := time.Now()
	waiting := entryAt(now, "w", domain.LabKindIdea, domain.LabStatusGrilling, "Offline mode for the dashboard")
	waiting.Runs = []string{"run-w"}
	shaping := entryAt(now.Add(-time.Minute), "s", domain.LabKindBug, domain.LabStatusDraft, "Sync stalls")
	v := newLabView()
	v.entries = []domain.LabEntry{waiting, shaping}
	v.setMission(labMissionState(map[string]string{"run-w": domain.WorkflowBlocked}))

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
	assert.Equal(t, column("Offline mode for the dashboard", "WAITING ON YOU"), column("Sync stalls", "BUG TO REPORT"))
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
		v.setArchived(e.Archived)
		return v
	}
	now := time.Now()

	assert.Equal(t, []string{"Capture new entry"}, labels(newLabView()))
	assert.Equal(t, []string{"Grill", "Edit", "Archive", "Delete", "Capture new entry"},
		labels(viewOf(entryAt(now, "i", domain.LabKindIdea, domain.LabStatusDraft, "Idea"), nil)))
	assert.Equal(t, []string{"Shape into an issue", "Edit", "Archive", "Delete", "Capture new entry"},
		labels(viewOf(entryAt(now, "b", domain.LabKindBug, domain.LabStatusDraft, "Bug"), nil)), "a bug draft can be shaped")

	shaping := entryAt(now, "s", domain.LabKindBug, domain.LabStatusShaping, "Bug")
	shaping.Runs = []string{"run-1"}
	assert.Equal(t, []string{"End session", "View agent", "Archive", "Capture new entry"},
		labels(viewOf(shaping, map[string]string{"run-1": domain.WorkflowBlocked})), "a live session is ended, not restarted")
	assert.Equal(t, []string{"Resume shaping", "Archive", "Capture new entry"},
		labels(viewOf(shaping, map[string]string{"run-1": domain.WorkflowSucceeded})), "an ended session can be resumed")

	archived := entryAt(now, "a", domain.LabKindBug, domain.LabStatusDraft, "Bug")
	archived.Archived = true
	assert.Equal(t, []string{"Restore", "Delete", "Capture new entry"}, labels(viewOf(archived, nil)))
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
	return m, store, starter, e
}

func TestLab_EnterOnBugDraftStartsShapingSession(t *testing.T) {
	m, store, starter, e := shapeModel(t)

	m, _ = press(t, m, "enter")
	require.NotNil(t, m.lab.page, "Enter on a row opens the entry page")
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
	selected, ok := m.lab.selected()
	require.True(t, ok)
	assert.Equal(t, e.ID, selected.ID, "the list follows the entry")
	m.lab.setMission(labMissionState(map[string]string{"run-shape-1": domain.WorkflowRunning}))
	m.lab.entries = stored
	assert.Equal(t, labGroupWorking, m.lab.groupOf(stored[0]), "the entry is WORKING once its run reports in")
	assert.NoFileExists(t, filepath.Join(store.EntryDir(e.ID), "session.lock"), "the lock is released after the start")
	assert.Contains(t, m.statusMsg, "its questions appear here in the Lab")
}

func TestLab_ShapeRefusedWhileAnotherGroveHoldsTheEntry(t *testing.T) {
	m, store, starter, e := shapeModel(t)
	require.NoError(t, os.MkdirAll(store.EntryDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.lock"),
		[]byte(`{"pid":1,"host":"another-machine","since":"2026-09-28T08:00:00Z"}`), 0o644))

	updated, cmd := m.startLabSession(e, domain.LabModeShape)
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

	updated, cmd := m.startLabSession(e, domain.LabModeShape)
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
	updated, _ := m.startLabSession(e, domain.LabModeShape)
	assert.Contains(t, updated.(*Model).statusErr, "Sandcastle is disabled")
	assert.Empty(t, starter.requests, "nothing is started")
}

func TestLab_EndSessionWritesCloseMarker(t *testing.T) {
	m, store, _, e := shapeModel(t)
	e.Status = domain.LabStatusShaping
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))

	updated, cmd := m.handleLabAction(modal.ContextActionLabEnd)
	m = runCmd(t, updated.(*Model), cmd)
	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "run-1.close"))
	assert.Contains(t, m.statusMsg, "Ending the session")
}

func TestLab_EnterOnIdeaDraftStartsGrillSession(t *testing.T) {
	m, store, starter, _ := shapeModel(t)
	idea := domain.NewLabEntry(domain.LabKindIdea, "Offline mode", time.Now())
	require.NoError(t, store.Put(idea))
	m.lab.entries, _ = store.Load()
	m.lab.selectID(idea.ID)

	m, _ = press(t, m, "enter")
	require.NotNil(t, m.lab.page, "Enter on a row opens the entry page")
	m, cmd := press(t, m, "enter")
	updated, _ := m.Update(cmd())
	m = updated.(*Model)

	require.Len(t, starter.requests, 1)
	assert.Equal(t, "grill", starter.requests[0].Kind)
	assert.Equal(t, idea.ID, starter.requests[0].EntryID)
	stored, _ := store.Load()
	for _, e := range stored {
		if e.ID == idea.ID {
			assert.Equal(t, domain.LabModeGrill, e.Mode)
			assert.Equal(t, domain.LabStatusGrilling, e.Status)
		}
	}
	assert.Contains(t, m.statusMsg, "Grilling")
}

func TestLab_ResumedGrillKeepsItsStage(t *testing.T) {
	m, store, starter, _ := shapeModel(t)
	e := domain.NewLabEntry(domain.LabKindIdea, "Offline mode", time.Now())
	e.Mode, e.Status, e.Runs = domain.LabModeGrill, domain.LabStatusSpecced, []string{"run-old"}
	require.NoError(t, store.Put(e))
	m.lab.entries, _ = store.Load()
	m.lab.setMission(labMissionState(map[string]string{"run-old": domain.WorkflowSucceeded}))
	m.lab.selectID(e.ID)

	assert.Contains(t, labelsOf(labContextActions(m.lab)), "Resume grilling")
	updated, cmd := m.handleLabAction(modal.ContextActionLabGrill)
	updated, _ = updated.(*Model).Update(cmd())

	require.Len(t, starter.requests, 1)
	stored, _ := store.Load()
	for _, s := range stored {
		if s.ID == e.ID {
			assert.Equal(t, domain.LabStatusSpecced, s.Status, "resuming does not move the entry back")
			assert.Equal(t, []string{"run-old", "run-shape-1"}, s.Runs)
		}
	}
}

func labelsOf(actions []contextActionOption) []string {
	var out []string
	for _, a := range actions {
		out = append(out, a.label)
	}
	return out
}

func TestLabView_StatusAdvancesWithGrillRun(t *testing.T) {
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusGrilling, "Offline mode")
	e.Mode, e.Runs = domain.LabModeGrill, []string{"run-1"}
	v := newLabView()
	v.entries = []domain.LabEntry{e}

	state := labMissionState(map[string]string{"run-1": domain.WorkflowRunning})
	state.WorkflowRuns[0].CurrentStep = "Tickets"
	v.setMission(state)
	advances := v.statusAdvances()
	require.Len(t, advances, 1)
	assert.Equal(t, domain.LabStatusTicketed, advances[0].Status)

	state.WorkflowRuns[0].CurrentStep = "Interview"
	v.entries[0].Status = domain.LabStatusTicketed
	v.setMission(state)
	assert.Empty(t, v.statusAdvances(), "a status never moves back")

	shape := entryAt(time.Now(), "s", domain.LabKindBug, domain.LabStatusShaping, "Bug")
	shape.Mode, shape.Runs = domain.LabModeShape, []string{"run-1"}
	v.entries = []domain.LabEntry{shape}
	assert.Empty(t, v.statusAdvances(), "shaping has no stages to advance")
}

func TestLabReview_ApprovingRepositoryDocumentWritesItToTheCheckout(t *testing.T) {
	store := data.NewLabStore(withLabCommonDir(t))
	e := domain.NewLabEntry(domain.LabKindIdea, "Offline mode", time.Now())
	e.Mode, e.Status = domain.LabModeGrill, domain.LabStatusSpecced
	require.NoError(t, store.Put(e))
	context := "# Grove\n\n**Lab entry**:\nAn idea or bug.\n"
	require.NoError(t, os.MkdirAll(store.ArtifactsDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.ArtifactsDir(e.ID), "CONTEXT.md"), []byte(context), 0o644))

	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	updated, cmd := m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "CONTEXT.md", Hash: domain.LabContentHash(context), Action: modal.LabReviewApprove})
	updated, _ = updated.(*Model).Update(cmd())
	m = updated.(*Model)

	written, err := os.ReadFile(filepath.Join(m.RepoPath, "CONTEXT.md"))
	require.NoError(t, err)
	assert.Equal(t, context, string(written))
	assert.Contains(t, m.statusMsg, "written to")
	assert.Contains(t, m.statusMsg, "uncommitted")
	stored, _ := store.Load()
	assert.Equal(t, domain.LabReviewApproved, stored[0].ReviewOf(domain.LabArtifact{Path: "CONTEXT.md", Body: context}))
}

func TestLabReview_ApprovalThatWouldDropContentIsRefused(t *testing.T) {
	store := data.NewLabStore(withLabCommonDir(t))
	e := domain.NewLabEntry(domain.LabKindIdea, "Offline mode", time.Now())
	require.NoError(t, store.Put(e))
	draft := "# Grove\n\n**Lab entry**:\nAn idea or bug.\n"
	require.NoError(t, os.MkdirAll(store.ArtifactsDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.ArtifactsDir(e.ID), "CONTEXT.md"), []byte(draft), 0o644))

	m := newLabModel(t)
	existing := "# Grove\n\n**Worktree**:\nA checkout.\n"
	require.NoError(t, os.WriteFile(filepath.Join(m.RepoPath, "CONTEXT.md"), []byte(existing), 0o644))
	m.lab.entries, _ = store.Load()

	updated, cmd := m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "CONTEXT.md", Hash: domain.LabContentHash(draft), Action: modal.LabReviewApprove})
	updated, _ = updated.(*Model).Update(cmd())
	m = updated.(*Model)

	assert.Contains(t, m.statusErr, "would drop content")
	kept, _ := os.ReadFile(filepath.Join(m.RepoPath, "CONTEXT.md"))
	assert.Equal(t, existing, string(kept), "the checkout's glossary is untouched")
	stored, _ := store.Load()
	assert.Equal(t, domain.LabReviewDraft, stored[0].ReviewOf(domain.LabArtifact{Path: "CONTEXT.md", Body: draft}), "the approval is not recorded")
}

func TestLab_EnterOnPublishedEntryOpensItsEpicInIssues(t *testing.T) {
	m := newLabModel(t)
	epic := 251
	e := entryAt(time.Now(), "e", domain.LabKindIdea, domain.LabStatusPublished, "Offline mode")
	e.Mode, e.Issues.Epic = domain.LabModeGrill, &epic
	m.lab.entries = []domain.LabEntry{e}
	m.issues = []domain.Issue{{Number: 240, Title: "Other"}, {Number: 251, Title: "Offline mode"}}

	assert.Contains(t, labelsOf(labContextActions(m.lab)), "Open on GitHub")
	m, _ = press(t, m, "enter")
	require.NotNil(t, m.lab.page, "Enter on a row opens the entry page")
	m, _ = press(t, m, "enter")
	assert.Equal(t, viewIssues, m.view)
	assert.Equal(t, 1, m.selectedIssueIdx, "the epic is selected, with its sub-issues beneath it")
}

func TestLab_PublishedEntryNotYetSyncedSaysSo(t *testing.T) {
	m := newLabModel(t)
	issue := 260
	e := entryAt(time.Now(), "e", domain.LabKindBug, domain.LabStatusPublished, "Sync stalls")
	e.Mode, e.Issues.Issue = domain.LabModeShape, &issue
	m.lab.entries = []domain.LabEntry{e}

	updated, _ := m.openLabIssue(e)
	m = updated.(*Model)
	assert.Equal(t, viewLab, m.view)
	assert.Contains(t, m.statusMsg, "#260 is not among the synced issues yet")
}

func TestLab_ArchivingLiveSessionAsksThenEndsIt(t *testing.T) {
	m, store, _, e := shapeModel(t)
	e.Mode, e.Status, e.Runs = domain.LabModeShape, domain.LabStatusShaping, []string{"run-1"}
	require.NoError(t, store.Put(e))
	m.lab.entries, _ = store.Load()
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))

	updated, _ := m.handleLabAction(modal.ContextActionLabArchive)
	m = updated.(*Model)
	require.IsType(t, &modal.LabArchiveModal{}, m.activeModal, "archiving a live session asks first")
	stored, _ := store.Load()
	assert.False(t, stored[0].Archived, "nothing changes before confirmation")

	m, cmd := press(t, m, "y")
	updated, cmd = m.Update(cmd())
	m = runCmd(t, updated.(*Model), cmd)

	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "run-1.close"))
	stored, _ = store.Load()
	assert.True(t, stored[0].Archived)
	assert.True(t, m.lab.archived)
}

func TestLab_DeleteRefusedWhileSessionIsLive(t *testing.T) {
	m := newLabModel(t)
	e := entryAt(time.Now(), "e", domain.LabKindBug, domain.LabStatusShaping, "Bug")
	e.Archived, e.Runs = true, []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowRunning}))
	m.lab.setArchived(true)

	updated, _ := m.handleLabAction(modal.ContextActionLabDelete)
	m = updated.(*Model)
	assert.Nil(t, m.activeModal)
	assert.Contains(t, m.statusErr, "End the entry's session before deleting it")
}

func TestLab_ForeignLockIsShownAndCleared(t *testing.T) {
	commonDir := withLabCommonDir(t)
	store := data.NewLabStore(commonDir)
	e := domain.NewLabEntry(domain.LabKindBug, "Sync stalls", time.Now())
	require.NoError(t, store.Put(e))
	require.NoError(t, os.MkdirAll(store.EntryDir(e.ID), 0o755))
	lockPath := filepath.Join(store.EntryDir(e.ID), "session.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte(`{"pid":42,"host":"build-server","since":"2026-09-28T08:00:00Z"}`), 0o644))

	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	m.lab.locks, _ = store.Locks()

	assert.Contains(t, renderLabContext(m.lab, e, 60, time.Now()), "Locked: by Grove on build-server (pid 42)")
	assert.Contains(t, labelsOf(labContextActions(m.lab)), "Clear lock from build-server")

	updated, cmd := m.handleLabAction(modal.ContextActionLabClearLock)
	m = runCmd(t, updated.(*Model), cmd)
	assert.NoFileExists(t, lockPath)
	assert.Empty(t, m.lab.locks, "the reloaded locks no longer include it")
	assert.NotContains(t, labelsOf(labContextActions(m.lab)), "Clear lock from build-server")
}

func TestLab_LocalLockIsNotOfferedForClearing(t *testing.T) {
	v := newLabView()
	e := entryAt(time.Now(), "e", domain.LabKindBug, domain.LabStatusDraft, "Bug")
	v.entries = []domain.LabEntry{e}
	v.locks = map[string]data.LabLockOwner{"e": {PID: 1, Host: labLocalHost()}}
	_, foreign := v.foreignLock(e)
	assert.False(t, foreign, "a lock on this machine is taken over automatically")
}
