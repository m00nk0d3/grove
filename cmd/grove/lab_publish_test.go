package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
func (g unreachableGitHub) EnsureLabels(string, []string) error { return g.fail() }
func (g unreachableGitHub) AddSubIssue(string, int, int) error  { return g.fail() }
func (g unreachableGitHub) AddBlockedBy(string, int, int) error { return g.fail() }
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
	ensured   []string
	links     []string // "#child ⊂ #parent" and "#issue ⊣ #blocker", in order
	linkErr   error
	linkErrAt int
	// failCreateAt fails the create after that many issues were created.
	failCreateAt int
}

func (f *fakeGitHub) Repo() (string, error) { return "m00nk0d3/grove", nil }
func (f *fakeGitHub) CreateIssue(repo, title, body string, labels []string) (int, string, error) {
	if f.createErr != nil || (f.failCreateAt > 0 && len(f.created) == f.failCreateAt) {
		if f.createErr == nil {
			return 0, "", errors.New("secondary rate limit")
		}
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
	e.Runs = []string{"run-1"}
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
	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "run-1.close"), "the session is ended")
	assert.NoFileExists(t, filepath.Join(store.EntryDir(e.ID), "session.lock"))
	assert.Equal(t, labGroupDone, m.lab.groupOf(stored[0]))
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
	assert.Contains(t, m.statusErr, "created #251, but project not found")
	assert.Contains(t, m.statusErr, "publish again to retry")
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

func TestLabReview_ChangesAreRequestedInGrove(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m, store, e := publishModel(t, false)
	m.herdrNavigator = navigator
	e.Runs = []string{"run-1"}
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))

	updated, _ := m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "issue.md", Action: modal.LabReviewChanges})
	m = updated.(*Model)
	require.IsType(t, &modal.LabMessageModal{}, m.activeModal, "the change request is written in Grove")
	assert.Empty(t, navigator.focusedPane, "the pane is not opened")
	m = typeInto(t, m, "Mention the proxy")
	m, cmd := press(t, m, "ctrl+s")
	m = feed(t, m, cmd, 2)
	assert.Equal(t, "Change request sent to the agent", m.statusMsg)
	raw, err := os.ReadFile(filepath.Join(store.EntryDir(e.ID), "requests", "001.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "issue.md: Mention the proxy")

	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowSucceeded}))
	updated, _ = m.handleLabArtifactReview(modal.LabArtifactReviewMsg{EntryID: e.ID, Path: "issue.md", Action: modal.LabReviewChanges})
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
	require.NotNil(t, m.lab.page, "Enter on a row opens the entry page")
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

func (f *fakeGitHub) EnsureLabels(_ string, labels []string) error {
	f.ensured = append(f.ensured, labels...)
	return nil
}

func (f *fakeGitHub) AddSubIssue(_ string, parent, child int) error {
	if f.linkErr != nil && f.linkErrAt == len(f.links) {
		return f.linkErr
	}
	f.links = append(f.links, fmt.Sprintf("#%d ⊂ #%d", child, parent))
	return nil
}

func (f *fakeGitHub) AddBlockedBy(_ string, issue, blocker int) error {
	if f.linkErr != nil && f.linkErrAt == len(f.links) {
		return f.linkErr
	}
	f.links = append(f.links, fmt.Sprintf("#%d ⊣ #%d", issue, blocker))
	return nil
}

const grilledSpec = "# Offline mode for the dashboard\n\n## Problem Statement\nThe dashboard is empty offline.\n"

const grilledTickets = `{"tickets":[
	{"key":"01","title":"Cache the last sync","body":"## What to build\nCache it.","blocked_by":[]},
	{"key":"02","title":"Show data age","body":"## What to build\nShow age.","blocked_by":["01"]},
	{"key":"03","title":"Sync on reconnect","body":"## What to build\nResync.","blocked_by":["01","02"]}
]}`

const grilledADR = "# Cache until the next sync\n\nCached data stays until the next sync, because a stale view beats an empty one.\n\nMore detail.\n"

const grilledContext = "# Grove\n\n**Lab entry**:\nAn idea or bug.\n"

// grilledEntry stores a grilled idea whose drafts are written and approved,
// tickets.json only when approveTickets is set.
func grilledEntry(t *testing.T, store *data.LabStore, approveTickets bool) domain.LabEntry {
	t.Helper()
	e := domain.NewLabEntry(domain.LabKindIdea, "Offline mode", time.Now())
	e.Mode, e.Status = domain.LabModeGrill, domain.LabStatusTicketed
	e.Runs = []string{"run-1"}
	files := map[string]string{
		"spec.md":                grilledSpec,
		"tickets.json":           grilledTickets,
		"docs/adr/0003-cache.md": grilledADR,
		"CONTEXT.md":             grilledContext,
	}
	for path, body := range files {
		full := filepath.Join(store.ArtifactsDir(e.ID), filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
		if path != "tickets.json" || approveTickets {
			e.SetReview(domain.LabArtifact{Path: path, Body: body}, domain.LabReviewApproved)
		}
	}
	require.NoError(t, store.Put(e))
	return e
}

func epicModel(t *testing.T, gh *fakeGitHub, approveTickets bool) (*Model, *data.LabStore, domain.LabEntry) {
	t.Helper()
	withFakeGitHub(t, gh)
	store := data.NewLabStore(withLabCommonDir(t))
	e := grilledEntry(t, store, approveTickets)
	m := newLabModel(t)
	m.lab.entries, _ = store.Load()
	return m, store, e
}

// publishEpic previews and confirms, as the user would, and returns the model.
func publishEpic(t *testing.T, m *Model, e domain.LabEntry, boardRef string) *Model {
	t.Helper()
	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	require.IsType(t, &modal.LabPublishModal{}, m.activeModal, m.statusErr)
	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID, BoardRef: boardRef})
	return step(t, updated.(*Model), cmd)
}

var wantEpicLinks = []string{
	"#252 ⊂ #251",
	"#253 ⊂ #251", "#253 ⊣ #252",
	"#254 ⊂ #251", "#254 ⊣ #252", "#254 ⊣ #253",
}

func TestLabPublishEpic_EpicThenTicketsThenLinksThenBoard(t *testing.T) {
	gh := &fakeGitHub{boards: []internalexec.LabProject{{ID: "PVT_1", Owner: "m00nk0d3", Number: 3, Title: "Roadmap"}}}
	m, store, e := epicModel(t, gh, true)

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	publishPreview := m.activeModal.(*modal.LabPublishModal)
	publishPreview.SetWidth(140)
	preview := publishPreview.View()
	assert.Contains(t, preview, "TICKETS  3 sub-issues")
	assert.Contains(t, preview, "Offline mode for the dashboard")
	assert.Empty(t, gh.created, "nothing is created before confirmation")

	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID, BoardRef: "m00nk0d3/3"})
	m = step(t, updated.(*Model), cmd)
	require.Empty(t, m.statusErr)

	assert.Equal(t, []string{"epic", "ready-for-agent"}, gh.ensured, "missing labels are created first")
	assert.Equal(t, []string{"Offline mode for the dashboard", "Cache the last sync", "Show data age", "Sync on reconnect"}, gh.created,
		"the epic, then its tickets blockers first")
	assert.Equal(t, [][]string{{"epic"}, {"ready-for-agent"}, {"ready-for-agent"}, {"ready-for-agent"}}, gh.labels)
	assert.Equal(t, wantEpicLinks, gh.links, "every ticket is a sub-issue of the epic and linked to its real blockers")
	assert.Len(t, gh.placed, 4, "the epic and every ticket go to the board")
	assert.Equal(t, []string{"Backlog", "Backlog", "Backlog", "Backlog"}, gh.statuses)

	epicBody := gh.bodies[0]
	assert.Contains(t, epicBody, "## Problem Statement")
	assert.NotContains(t, epicBody, "# Offline mode for the dashboard", "the title is not repeated in the body")
	assert.Contains(t, epicBody, "## Glossary and decisions")
	assert.Contains(t, epicBody, "**Cache until the next sync** (`docs/adr/0003-cache.md`): Cached data stays until the next sync")
	assert.NotContains(t, epicBody, "More detail.", "a decision is summarised by its first paragraph")
	assert.Contains(t, epicBody, "<summary><code>CONTEXT.md</code></summary>")
	assert.Equal(t, "## What to build\nCache it.", gh.bodies[1])

	stored, _ := store.Load()
	got := stored[0]
	assert.Equal(t, domain.LabStatusPublished, got.Status)
	require.NotNil(t, got.Issues.Epic)
	assert.Equal(t, 251, *got.Issues.Epic)
	assert.Equal(t, []int{252, 253, 254}, got.Issues.Tickets)
	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "run-1.close"))
	assert.Contains(t, m.statusMsg, `Published epic "Offline mode for the dashboard" as #251 with 3 tickets`)
}

func TestLabPublishEpic_ResumesAfterFailedTicket(t *testing.T) {
	gh := &fakeGitHub{failCreateAt: 2}
	m, store, e := epicModel(t, gh, true)

	m = publishEpic(t, m, e, "")
	assert.Contains(t, m.statusErr, "epic #251 and 1 of 3 tickets are published")
	assert.Contains(t, m.statusErr, "publish again to finish")
	stored, _ := store.Load()
	assert.Equal(t, domain.LabStatusTicketed, stored[0].Status)
	assert.Equal(t, []int{252}, stored[0].Issues.Tickets)

	gh.failCreateAt = 0
	m.statusErr = ""
	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	assert.Contains(t, m.activeModal.View(), "Epic #251 and 1 of 3 tickets were created by an earlier attempt")
	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID})
	m = step(t, updated.(*Model), cmd)

	require.Empty(t, m.statusErr)
	assert.Equal(t, []string{"Offline mode for the dashboard", "Cache the last sync", "Show data age", "Sync on reconnect"}, gh.created,
		"the resume creates only what is missing")
	stored, _ = store.Load()
	assert.Equal(t, []int{252, 253, 254}, stored[0].Issues.Tickets)
	assert.Equal(t, domain.LabStatusPublished, stored[0].Status)
}

func TestLabPublishEpic_ResumesAfterFailedLinkWithoutRepeatingLinks(t *testing.T) {
	gh := &fakeGitHub{linkErr: errors.New("secondary rate limit"), linkErrAt: 3}
	m, _, e := epicModel(t, gh, true)

	m = publishEpic(t, m, e, "")
	assert.Contains(t, m.statusErr, "every ticket is created, but secondary rate limit")
	assert.Len(t, gh.links, 3)

	gh.linkErr = nil
	m.statusErr = ""
	m = publishEpic(t, m, e, "")
	require.Empty(t, m.statusErr)
	assert.Equal(t, wantEpicLinks, gh.links, "each link is made exactly once across both attempts")
	assert.Len(t, gh.created, 4, "nothing is created twice")
}

func TestLabPublishEpic_NeedsApprovedValidTickets(t *testing.T) {
	gh := &fakeGitHub{}
	m, store, e := epicModel(t, gh, false)

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	assert.Contains(t, m.statusErr, "approve tickets.json in the inspector before publishing")

	cyclic := `{"tickets":[{"key":"01","title":"a","blocked_by":["02"]},{"key":"02","title":"b","blocked_by":["01"]}]}`
	require.NoError(t, os.WriteFile(filepath.Join(store.ArtifactsDir(e.ID), "tickets.json"), []byte(cyclic), 0o644))
	e.SetReview(domain.LabArtifact{Path: "tickets.json", Body: cyclic}, domain.LabReviewApproved)
	require.NoError(t, store.Put(e))
	m.lab.entries, _ = store.Load()
	m.statusErr = ""

	updated, cmd = m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	assert.Contains(t, m.statusErr, "block each other in a cycle")
	assert.Empty(t, gh.created)
	assert.Empty(t, gh.ensured, "nothing at all is sent for a draft that cannot be published")
}

func TestLabPublishRequires(t *testing.T) {
	grill := domain.LabEntry{Mode: domain.LabModeGrill}
	for status, want := range map[domain.LabStatus][]string{
		domain.LabStatusGrilling:  nil,
		domain.LabStatusSpecced:   {"spec.md", "tickets.json"},
		domain.LabStatusTicketed:  {"spec.md", "tickets.json"},
		domain.LabStatusPublished: nil,
	} {
		grill.Status = status
		assert.Equal(t, want, labPublishRequires(grill), string(status))
	}
	assert.Equal(t, []string{"issue.md"}, labPublishRequires(domain.LabEntry{Mode: domain.LabModeShape, Status: domain.LabStatusShaping}))
	assert.Nil(t, labPublishRequires(domain.LabEntry{Mode: domain.LabModeShape, Status: domain.LabStatusShaping, Archived: true}))
}

func TestLab_EscalateShapingBugHandsOverToGrill(t *testing.T) {
	m, store, starter, e := shapeModel(t)
	e.Mode, e.Status, e.Runs = domain.LabModeShape, domain.LabStatusShaping, []string{"run-1"}
	require.NoError(t, store.Put(e))
	m.lab.entries, _ = store.Load()
	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowBlocked}))

	assert.Contains(t, labelsOf(labContextActions(m.lab)), "Escalate to grill")
	updated, cmd := m.handleLabAction(modal.ContextActionLabEscalate)
	updated, _ = updated.(*Model).Update(cmd())
	m = updated.(*Model)

	assert.FileExists(t, filepath.Join(store.EntryDir(e.ID), "run-1.close"), "the shape session is ended")
	require.Len(t, starter.requests, 1)
	assert.Equal(t, "grill", starter.requests[0].Kind)
	stored, _ := store.Load()
	assert.Equal(t, domain.LabModeGrill, stored[0].Mode)
	assert.Equal(t, domain.LabStatusGrilling, stored[0].Status)
	assert.Equal(t, []string{"run-1", "run-shape-1"}, stored[0].Runs)
	assert.Equal(t, domain.LabKindBug, stored[0].Kind, "it is still the bug it was")
}

func TestLab_EscalatePublishedBugKeepsItsIssue(t *testing.T) {
	m, store, starter, e := shapeModel(t)
	bug := 240
	e.Mode, e.Status, e.Runs = domain.LabModeShape, domain.LabStatusPublished, []string{"run-1"}
	e.Issues.Issue = &bug
	require.NoError(t, store.Put(e))
	m.lab.entries, _ = store.Load()

	updated, cmd := m.handleLabAction(modal.ContextActionLabEscalate)
	updated.(*Model).Update(cmd())

	require.Len(t, starter.requests, 1)
	stored, _ := store.Load()
	assert.Equal(t, domain.LabStatusGrilling, stored[0].Status)
	require.NotNil(t, stored[0].Issues.Issue)
	assert.Equal(t, 240, *stored[0].Issues.Issue)
}

func TestLabView_CanEscalateOnlyShapedBugs(t *testing.T) {
	v := newLabView()
	for _, tc := range []struct {
		e    domain.LabEntry
		want bool
	}{
		{domain.LabEntry{Kind: domain.LabKindBug, Mode: domain.LabModeShape, Status: domain.LabStatusShaping}, true},
		{domain.LabEntry{Kind: domain.LabKindBug, Mode: domain.LabModeShape, Status: domain.LabStatusPublished}, true},
		{domain.LabEntry{Kind: domain.LabKindBug, Status: domain.LabStatusDraft}, false},
		{domain.LabEntry{Kind: domain.LabKindBug, Mode: domain.LabModeGrill, Status: domain.LabStatusGrilling}, false},
		{domain.LabEntry{Kind: domain.LabKindIdea, Mode: domain.LabModeGrill, Status: domain.LabStatusGrilling}, false},
		{domain.LabEntry{Kind: domain.LabKindBug, Mode: domain.LabModeShape, Status: domain.LabStatusShaping, Archived: true}, false},
	} {
		assert.Equal(t, tc.want, v.canEscalate(tc.e), "%s %s %s", tc.e.Kind, tc.e.Mode, tc.e.Status)
	}
}

func TestLabPublishEpic_EscalatedBugBecomesSubIssue(t *testing.T) {
	gh := &fakeGitHub{}
	withFakeGitHub(t, gh)
	store := data.NewLabStore(withLabCommonDir(t))
	e := grilledEntry(t, store, true)
	bug := 240
	e.Kind, e.Issues.Issue = domain.LabKindBug, &bug
	require.NoError(t, store.Put(e))
	m := newLabModel(t)
	m.lab.entries, _ = store.Load()

	updated, cmd := m.handleLabAction(modal.ContextActionLabPublish)
	m = step(t, updated.(*Model), cmd)
	preview := m.activeModal.(*modal.LabPublishModal)
	preview.SetWidth(140)
	assert.Contains(t, preview.View(), "Bug #240, which this grew out of, becomes a sub-issue of the epic")

	updated, cmd = m.publishLabEntry(modal.LabPublishConfirmedMsg{EntryID: e.ID})
	m = step(t, updated.(*Model), cmd)
	require.Empty(t, m.statusErr)

	assert.True(t, strings.HasPrefix(gh.bodies[0], "_Grew out of bug #240._"), "the epic says where it came from")
	assert.Equal(t, append(append([]string{}, wantEpicLinks...), "#240 ⊂ #251"), gh.links, "the bug is linked after the tickets")
	stored, _ := store.Load()
	assert.True(t, stored[0].Issues.BugLinked)
}
