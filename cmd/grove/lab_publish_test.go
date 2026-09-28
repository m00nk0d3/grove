package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No test in this package may reach GitHub. A test that publishes installs a
// fake; any other path to GitHub fails the test that took it.
func init() {
	newLabGitHub = func(string) labGitHub { return unreachableGitHub{} }
}

type unreachableGitHub struct{}

func (unreachableGitHub) fail() error {
	panic("a test reached GitHub without installing a fake")
}
func (g unreachableGitHub) Repo() (string, error) { return "", g.fail() }
func (g unreachableGitHub) CreateIssue(string, string, string, []string) (int, string, error) {
	return 0, "", g.fail()
}
func (g unreachableGitHub) LinkedProjects(string) ([]internalexec.LabProject, error) {
	return nil, g.fail()
}
func (g unreachableGitHub) Project(string, int) (internalexec.LabProject, error) {
	return internalexec.LabProject{}, g.fail()
}
func (g unreachableGitHub) PlaceOnBoard(internalexec.LabProject, string, string) error {
	return g.fail()
}

// fakeGitHub records what publishing would have done.
type fakeGitHub struct {
	boards    []internalexec.LabProject
	created   []string // titles
	bodies    []string
	labels    [][]string
	placed    []string // issue URLs
	statuses  []string
	nextIssue int
	placeErr  error
	createErr error
}

func (f *fakeGitHub) Repo() (string, error) { return "m00nk0d3/grove", nil }
func (f *fakeGitHub) CreateIssue(repo, title, body string, labels []string) (int, string, error) {
	if f.createErr != nil {
		return 0, "", f.createErr
	}
	f.created = append(f.created, title)
	f.bodies = append(f.bodies, body)
	f.labels = append(f.labels, labels)
	f.nextIssue++
	n := 250 + f.nextIssue
	return n, fmt.Sprintf("https://github.com/%s/issues/%d", repo, n), nil
}
func (f *fakeGitHub) LinkedProjects(string) ([]internalexec.LabProject, error) { return f.boards, nil }
func (f *fakeGitHub) Project(owner string, number int) (internalexec.LabProject, error) {
	for _, b := range f.boards {
		if b.Owner == owner && b.Number == number {
			return b, nil
		}
	}
	return internalexec.LabProject{}, errors.New("no such board")
}
func (f *fakeGitHub) PlaceOnBoard(p internalexec.LabProject, url, status string) error {
	if f.placeErr != nil {
		return f.placeErr
	}
	f.placed = append(f.placed, p.Ref()+" "+url)
	f.statuses = append(f.statuses, status)
	return nil
}

func withFakeGitHub(t *testing.T, gh *fakeGitHub) {
	t.Helper()
	orig := newLabGitHub
	newLabGitHub = func(string) labGitHub { return gh }
	t.Cleanup(func() { newLabGitHub = orig })
	configPath := filepath.Join(t.TempDir(), "config.toml")
	origPath := labConfigPath
	labConfigPath = func() string { return configPath }
	t.Cleanup(func() { labConfigPath = origPath })
}

const shapedDraft = "# Sync stalls on token expiry\n\n## Summary\nThe dashboard freezes.\n"

// shapedEntry stores a bug being shaped whose session has drafted issue.md.
func shapedEntry(t *testing.T, store *data.LabStore, approved bool) domain.LabEntry {
	t.Helper()
	e := domain.NewLabEntry(domain.LabKindBug, "Sync stalls", time.Now())
	e.Mode, e.Status = domain.LabModeShape, domain.LabStatusShaping
	require.NoError(t, os.MkdirAll(store.ArtifactsDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.ArtifactsDir(e.ID), "issue.md"), []byte(shapedDraft), 0o644))
	if approved {
		e.SetReview(domain.LabArtifact{Path: "issue.md", Body: shapedDraft}, domain.LabReviewApproved)
	}
	require.NoError(t, store.Put(e))
	return e
}

func publishModel(t *testing.T, approved bool) (*Model, *data.LabStore, domain.LabEntry) {
	t.Helper()
	store := data.NewLabStore(withLabCommonDir(t))
	e := shapedEntry(t, store, approved)
	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	m.lab.setTab(labTabActive)
	return m, store, e
}

// step runs cmd and feeds its message back, returning the model.
func step(t *testing.T, m *Model, cmd tea.Cmd) *Model {
	t.Helper()
	require.NotNil(t, cmd)
	updated, _ := m.Update(cmd())
	return updated.(*Model)
}

func TestLabPublish_PreviewThenPublish(t *testing.T) {
	gh := &fakeGitHub{boards: []internalexec.LabProject{{ID: "PVT_1", Owner: "m00nk0d3", Number: 3, Title: "Roadmap"}}}
	withFakeGitHub(t, gh)
	m, store, e := publishModel(t, true)

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	preview, ok := m.activeModal.(*modal.LabPublishModal)
	require.True(t, ok, "publishing opens the preview first")
	assert.Empty(t, gh.created, "nothing is created before confirmation")
	assert.Contains(t, preview.View(), "Roadmap (m00nk0d3/3)")
	assert.Contains(t, preview.View(), "Sync stalls on token expiry")

	// y in the preview confirms with the board it shows.
	_, confirm := preview.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	require.NotNil(t, confirm)
	confirmed, ok := confirm().(modal.LabPublishConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, "m00nk0d3/3", confirmed.BoardRef)

	// Grove's reply to the confirmation is the publication itself. Only that
	// command is run: the GitHub sync that follows is not.
	updated, cmd = m.publishLabEntry(confirmed)
	m = step(t, updated.(*Model), cmd)

	assert.Equal(t, []string{"Sync stalls on token expiry"}, gh.created, "the title is the draft's heading")
	assert.Equal(t, []string{"## Summary\nThe dashboard freezes."}, gh.bodies, "the heading is not repeated in the body")
	assert.Equal(t, [][]string{{"bug"}}, gh.labels)
	assert.Equal(t, []string{"m00nk0d3/3 https://github.com/m00nk0d3/grove/issues/251"}, gh.placed)
	assert.Equal(t, []string{"Backlog"}, gh.statuses)

	stored, _ := store.Load()
	require.NotNil(t, stored[0].Issues.Issue)
	assert.Equal(t, 251, *stored[0].Issues.Issue)
	assert.Equal(t, domain.LabStatusPublished, stored[0].Status)
	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "session.close"), "the session is ended")
	assert.NoFileExists(t, filepath.Join(store.EntryDir(e.ID), "session.lock"))
	assert.Equal(t, labTabPublished, m.lab.tab)
	assert.Contains(t, m.statusMsg, "as #251 in m00nk0d3/grove")
}

func TestLabPublish_UnapprovedDraftIsNotPublished(t *testing.T) {
	gh := &fakeGitHub{}
	withFakeGitHub(t, gh)
	m, _, _ := publishModel(t, false)

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	assert.Nil(t, m.activeModal)
	assert.Contains(t, m.statusErr, "approve issue.md in the inspector before publishing")
	assert.Empty(t, gh.created)
}

func TestLabPublish_DraftChangedAfterPreviewIsNotPublished(t *testing.T) {
	gh := &fakeGitHub{}
	withFakeGitHub(t, gh)
	m, store, e := publishModel(t, true)

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	require.IsType(t, &modal.LabPublishModal{}, m.activeModal)

	// The agent revises the draft while the preview is open.
	require.NoError(t, os.WriteFile(filepath.Join(store.ArtifactsDir(e.ID), "issue.md"), []byte(shapedDraft+"\nMore.\n"), 0o644))
	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID})
	m = step(t, updated.(*Model), cmd)

	assert.Empty(t, gh.created, "a revision the user has not approved is never published")
	assert.Contains(t, m.statusErr, "approve issue.md")
}

func TestLabPublish_BoardFailureResumesWithoutDuplicating(t *testing.T) {
	gh := &fakeGitHub{
		boards:   []internalexec.LabProject{{ID: "PVT_1", Owner: "m00nk0d3", Number: 3, Title: "Roadmap"}},
		placeErr: errors.New("project not found"),
	}
	withFakeGitHub(t, gh)
	m, store, e := publishModel(t, true)

	publish := func() {
		updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
		m = step(t, updated.(*Model), cmd)
		updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID, BoardRef: "m00nk0d3/3"})
		m = step(t, updated.(*Model), cmd)
	}
	publish()
	assert.Contains(t, m.statusErr, "created issue #251, but")
	assert.Contains(t, m.statusErr, "publish again to retry the board")
	stored, _ := store.Load()
	require.NotNil(t, stored[0].Issues.Issue, "the created issue is recorded despite the failure")
	assert.Equal(t, domain.LabStatusShaping, stored[0].Status)

	gh.placeErr = nil
	m.statusErr = ""
	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	assert.Contains(t, m.activeModal.View(), "Issue #251 was created by an earlier attempt")
	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID, BoardRef: "m00nk0d3/3"})
	m = step(t, updated.(*Model), cmd)

	assert.Equal(t, []string{"Sync stalls on token expiry"}, gh.created, "the retry does not create a second issue")
	assert.Len(t, gh.placed, 1)
	stored, _ = store.Load()
	assert.Equal(t, domain.LabStatusPublished, stored[0].Status)
}

func TestLabPublish_ChosenBoardIsRemembered(t *testing.T) {
	gh := &fakeGitHub{boards: []internalexec.LabProject{
		{ID: "PVT_1", Owner: "o", Number: 1, Title: "One"},
		{ID: "PVT_2", Owner: "o", Number: 2, Title: "Two"},
	}}
	withFakeGitHub(t, gh)
	m, _, e := publishModel(t, true)

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID, BoardRef: "o/2", Chosen: true})
	m = step(t, updated.(*Model), cmd)

	assert.Equal(t, "o/2", m.Config.Lab.Project)
	saved, err := data.LoadConfig(labConfigPath())
	require.NoError(t, err)
	assert.Equal(t, "o/2", saved.Lab.Project, "the choice is saved to the configuration")
	assert.Equal(t, []string{"o/2 https://github.com/m00nk0d3/grove/issues/251"}, gh.placed)
}

func TestLabPublish_ConfiguredBoardIsUsed(t *testing.T) {
	gh := &fakeGitHub{boards: []internalexec.LabProject{
		{ID: "PVT_1", Owner: "o", Number: 1, Title: "One"},
		{ID: "PVT_2", Owner: "o", Number: 2, Title: "Two"},
	}}
	withFakeGitHub(t, gh)
	m, _, _ := publishModel(t, true)
	m.Config.Lab.Project = "o/2"

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	view := m.activeModal.View()
	assert.Contains(t, view, "Two (o/2)")
	assert.NotContains(t, view, "←/→", "a configured board is not offered as a choice")
}

func TestLabReview_ApproveAndDiscardFollowContent(t *testing.T) {
	m, store, e := publishModel(t, false)
	hash := domain.LabContentHash(shapedDraft)

	updated, cmd := m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "issue.md", Hash: hash, Action: modal.LabReviewApprove})
	m = step(t, updated.(*Model), cmd)
	stored, _ := store.Load()
	assert.Equal(t, domain.LabReviewApproved, stored[0].ReviewOf(domain.LabArtifact{Path: "issue.md", Body: shapedDraft}))
	assert.Contains(t, m.statusMsg, "Approved issue.md")

	updated, cmd = m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "issue.md", Hash: "stale", Action: modal.LabReviewDiscard})
	m = step(t, updated.(*Model), cmd)
	assert.Contains(t, m.statusErr, "changed while you were reading it")
	stored, _ = store.Load()
	assert.Equal(t, domain.LabReviewApproved, stored[0].ReviewOf(domain.LabArtifact{Path: "issue.md", Body: shapedDraft}), "a stale decision is not applied")
}

func TestLabReview_ChangesOpenTheLivePane(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m, _, e := publishModel(t, false)
	m.herdrNavigator = navigator
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))

	_, cmd := m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "issue.md", Action: modal.LabReviewChanges})
	require.NotNil(t, cmd)
	cmd()
	assert.Equal(t, "pane-run-1", navigator.focusedPane)

	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowSucceeded}))
	updated, _ := m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "issue.md", Action: modal.LabReviewChanges})
	assert.Contains(t, updated.(*Model).statusErr, "resume it to ask the agent for changes")
}

func TestLab_EnterOnDraftedEntryOpensReview(t *testing.T) {
	m, _, e := publishModel(t, false)
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	state := labMissionState(map[string]string{"run-1": domain.WorkflowBlocked})
	state.WorkflowRuns[0].CurrentStep = "Publish"
	m.lab.setMission(state)

	m, _ = press(t, m, "enter")
	inspector, ok := m.activeModal.(*modal.LabInspectorModal)
	require.True(t, ok, "a finished draft is reviewed in Grove, not in the pane")
	assert.Contains(t, inspector.View(), "3 ARTIFACTS")
}

func TestLabEditorCommand(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code --wait")
	cmd := labEditorCommand("/x/issue.md")
	assert.Equal(t, []string{"code", "--wait", "/x/issue.md"}, cmd.Args)

	t.Setenv("VISUAL", "nvim")
	assert.Equal(t, []string{"nvim", "/x/issue.md"}, labEditorCommand("/x/issue.md").Args)
}
