package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
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

func TestLabContextActions_DependOnState(t *testing.T) {
	labels := func(actions []contextActionOption) []string {
		var out []string
		for _, a := range actions {
			out = append(out, a.label)
		}
		return out
	}
	assert.Equal(t, []string{"Capture new entry"}, labels(labContextActions(domain.LabEntry{}, false)))
	assert.Equal(t, []string{"Edit", "Archive", "Delete", "Capture new entry"},
		labels(labContextActions(domain.LabEntry{Status: domain.LabStatusDraft}, true)))
	assert.Equal(t, []string{"Archive", "Capture new entry"},
		labels(labContextActions(domain.LabEntry{Status: domain.LabStatusGrilling}, true)))
	assert.Equal(t, []string{"Restore", "Delete", "Capture new entry"},
		labels(labContextActions(domain.LabEntry{Status: domain.LabStatusPublished, Archived: true}, true)))
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
	out := renderLabContext(e, 40, time.Now())

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
