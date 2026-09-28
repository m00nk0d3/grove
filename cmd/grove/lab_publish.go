package main

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/tui/modal"
)

// labGitHub is what publishing needs from GitHub. Tests replace it; nothing
// in a test reaches GitHub.
type labGitHub interface {
	Repo() (string, error)
	CreateIssue(repo, title, body string, labels []string) (int, string, error)
	LinkedProjects(repo string) ([]internalexec.LabProject, error)
	Project(owner string, number int) (internalexec.LabProject, error)
	PlaceOnBoard(p internalexec.LabProject, issueURL, status string) error
}

var newLabGitHub = func(repoPath string) labGitHub { return internalexec.NewGitHubWriter(repoPath) }

// labConfigPath is where a remembered board choice is saved. Tests replace it.
var labConfigPath = data.DefaultConfigPath

// labBacklogStatus is the board status published issues are placed in.
const labBacklogStatus = "Backlog"

// labIssueDraft is the artifact a shaped bug publishes.
const labIssueDraft = "issue.md"

// labPublishRequires lists the artifacts that must be approved before e can be
// published, or nil when e cannot be published.
func labPublishRequires(e domain.LabEntry) []string {
	if e.Archived || e.Status == domain.LabStatusPublished {
		return nil
	}
	if e.Mode == domain.LabModeShape && e.Status == domain.LabStatusShaping {
		return []string{labIssueDraft}
	}
	return nil
}

// labPendingPublish is a previewed publication waiting for confirmation: what
// was shown, and the boards it may go to.
type labPendingPublish struct {
	plan     modal.LabPublishPlan
	hash     string
	projects map[string]internalexec.LabProject
}

// labPublishPlanMsg carries a prepared preview, or why there is none.
type labPublishPlanMsg struct {
	pending labPendingPublish
	err     error
}

// labPublishedMsg reports a publication. Entries are reloaded even when it
// failed partway, so an issue that was created is recorded.
type labPublishedMsg struct {
	loaded   labsLoadedMsg
	number   int
	boardRef string
	chosen   bool
	err      error
}

// approvedIssueDraft returns e's approved issue draft.
func approvedIssueDraft(store *data.LabStore, e domain.LabEntry) (domain.LabArtifact, error) {
	artifacts, err := store.Artifacts(e.ID)
	if err != nil {
		return domain.LabArtifact{}, err
	}
	for _, a := range artifacts {
		if a.Path != labIssueDraft {
			continue
		}
		if e.ReviewOf(a) != domain.LabReviewApproved {
			return domain.LabArtifact{}, fmt.Errorf("approve %s in the inspector before publishing", labIssueDraft)
		}
		return a, nil
	}
	return domain.LabArtifact{}, fmt.Errorf("there is no %s to publish yet", labIssueDraft)
}

// prepareLabPublish builds the preview of publishing e.
func (m *Model) prepareLabPublish(e domain.LabEntry) (tea.Model, tea.Cmd) {
	if labPublishRequires(e) == nil {
		m.statusErr = "This entry cannot be published"
		return m, clearErrorCmd()
	}
	repoPath := m.RepoPath
	configured := m.Config.Lab.Project
	m.statusMsg = "Preparing the preview…"
	return m, func() tea.Msg {
		fail := func(err error) tea.Msg { return labPublishPlanMsg{err: err} }
		store, err := labStoreFor(repoPath)
		if err != nil {
			return fail(err)
		}
		draft, err := approvedIssueDraft(store, e)
		if err != nil {
			return fail(err)
		}
		title, body := domain.ParseIssueDraft(draft.Body)
		if title == "" {
			title = e.Title()
		}
		gh := newLabGitHub(repoPath)
		repo, err := gh.Repo()
		if err != nil {
			return fail(err)
		}

		var projects []internalexec.LabProject
		if configured != "" {
			owner, number, err := internalexec.ParseProjectRef(configured)
			if err != nil {
				return fail(fmt.Errorf("lab.project: %w", err))
			}
			p, err := gh.Project(owner, number)
			if err != nil {
				return fail(err)
			}
			projects = []internalexec.LabProject{p}
		} else if projects, err = gh.LinkedProjects(repo); err != nil {
			return fail(err)
		}

		pending := labPendingPublish{
			hash:     domain.LabContentHash(draft.Body),
			projects: make(map[string]internalexec.LabProject),
			plan: modal.LabPublishPlan{
				EntryID:  e.ID,
				Repo:     repo,
				Title:    title,
				Body:     body,
				Labels:   []string{"bug"},
				Board:    -1,
				Existing: e.Issues.Issue,
			},
		}
		for _, p := range projects {
			pending.plan.Boards = append(pending.plan.Boards, modal.LabPublishBoard{Ref: p.Ref(), Title: p.Title})
			pending.projects[p.Ref()] = p
		}
		if len(projects) > 0 {
			pending.plan.Board = 0
		}
		return labPublishPlanMsg{pending: pending}
	}
}

// handleLabPublishPlan opens the preview.
func (m *Model) handleLabPublishPlan(msg labPublishPlanMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = ""
	if msg.err != nil {
		m.statusErr = msg.err.Error()
		return m, clearErrorCmd()
	}
	pending := msg.pending
	m.labPending = &pending
	m.activeModal = modal.NewLabPublishModal(pending.plan)
	return m, nil
}

// publishLabEntry performs a confirmed publication. It holds the entry's lock,
// publishes exactly what the preview showed, and records the issue the moment
// it exists so an interrupted publication resumes instead of duplicating it.
func (m *Model) publishLabEntry(msg modal.LabPublishConfirmedMsg) (tea.Model, tea.Cmd) {
	m.activeModal = nil
	pending := m.labPending
	m.labPending = nil
	if pending == nil || pending.plan.EntryID != msg.EntryID {
		m.statusErr = "The preview is no longer current; publish again"
		return m, clearErrorCmd()
	}
	repoPath := m.RepoPath
	m.statusMsg = "Publishing to GitHub…"
	return m, func() tea.Msg {
		result := labPublishedMsg{boardRef: msg.BoardRef, chosen: msg.Chosen}
		done := func(err error, store *data.LabStore, status string) tea.Msg {
			result.err = err
			if store != nil {
				entries, loadErr := store.Load()
				result.loaded = labsLoadedMsg{entries: entries, selectID: msg.EntryID, status: status, err: loadErr}
			}
			return result
		}
		store, err := labStoreFor(repoPath)
		if err != nil {
			return done(err, nil, "")
		}
		release, err := store.LockEntry(msg.EntryID, labProcessAlive)
		if err != nil {
			return done(err, store, "")
		}
		defer release()

		entries, err := store.Load()
		if err != nil {
			return done(err, store, "")
		}
		var e domain.LabEntry
		for _, candidate := range entries {
			if candidate.ID == msg.EntryID {
				e = candidate
			}
		}
		if e.ID == "" {
			return done(errors.New("the entry no longer exists"), store, "")
		}
		draft, err := approvedIssueDraft(store, e)
		if err != nil {
			return done(err, store, "")
		}
		if domain.LabContentHash(draft.Body) != pending.hash {
			return done(fmt.Errorf("%s changed after the preview; review it and publish again", labIssueDraft), store, "")
		}

		plan := pending.plan
		gh := newLabGitHub(repoPath)
		if e.Issues.Issue == nil {
			number, _, err := gh.CreateIssue(plan.Repo, plan.Title, plan.Body, plan.Labels)
			if err != nil {
				return done(err, store, "")
			}
			e.Issues.Issue = &number
			e.Updated = time.Now().UTC()
			if err := store.Put(e); err != nil {
				return done(fmt.Errorf("created issue #%d but could not record it: %w", number, err), store, "")
			}
		}
		result.number = *e.Issues.Issue

		if msg.BoardRef != "" {
			p, ok := pending.projects[msg.BoardRef]
			if !ok {
				return done(fmt.Errorf("unknown board %s", msg.BoardRef), store, "")
			}
			url := fmt.Sprintf("https://github.com/%s/issues/%d", plan.Repo, result.number)
			if err := gh.PlaceOnBoard(p, url, labBacklogStatus); err != nil {
				return done(fmt.Errorf("created issue #%d, but %w — publish again to retry the board", result.number, err), store, "")
			}
		}

		e.Status = domain.LabStatusPublished
		e.Updated = time.Now().UTC()
		if err := store.Put(e); err != nil {
			return done(err, store, "")
		}
		closeErr := store.CloseSession(e.ID)
		return done(closeErr, store, fmt.Sprintf("Published %q as #%d in %s", plan.Title, result.number, plan.Repo))
	}
}

// handleLabPublished applies a publication result.
func (m *Model) handleLabPublished(msg labPublishedMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = ""
	var cmds []tea.Cmd
	if msg.loaded.entries != nil || msg.loaded.err != nil {
		_, cmd := m.handleLabsLoaded(msg.loaded)
		cmds = append(cmds, cmd)
	}
	if msg.err != nil {
		m.statusErr = msg.err.Error()
		return m, tea.Batch(append(cmds, clearErrorCmd())...)
	}
	if msg.chosen && msg.boardRef != "" {
		m.Config.Lab.Project = msg.boardRef
		if err := data.SaveConfig(m.Config, labConfigPath()); err != nil {
			m.statusErr = fmt.Sprintf("Published, but the board choice was not saved: %v", err)
			cmds = append(cmds, clearErrorCmd())
		}
	}
	return m, tea.Batch(append(cmds, m.syncGitHubCmd(true))...)
}

// labEditorClosedMsg reports that the user's editor exited.
type labEditorClosedMsg struct {
	entryID string
	err     error
}

// labEditorCommand builds the command that opens path in the user's editor:
// $VISUAL, then $EDITOR, then the platform's default.
func labEditorCommand(path string) *osexec.Cmd {
	editor := strings.Fields(os.Getenv("VISUAL"))
	if len(editor) == 0 {
		editor = strings.Fields(os.Getenv("EDITOR"))
	}
	if len(editor) == 0 {
		if runtime.GOOS == "windows" {
			editor = []string{"notepad"}
		} else {
			editor = []string{"vi"}
		}
	}
	return osexec.Command(editor[0], append(editor[1:], path)...)
}

// handleLabArtifactReview applies a review action from the inspector.
func (m *Model) handleLabArtifactReview(msg modal.LabArtifactReviewMsg) (tea.Model, tea.Cmd) {
	e, ok := m.lab.entry(msg.EntryID)
	if !ok {
		return m, nil
	}
	switch msg.Action {
	case modal.LabReviewChanges:
		run, live := m.lab.latestRun(e)
		if !live || !run.live() || run.paneID == "" {
			m.statusErr = "The session has ended; resume it to ask the agent for changes"
			return m, clearErrorCmd()
		}
		m.activeModal = nil
		m.statusMsg = fmt.Sprintf("Tell the agent what to change in %s", msg.Path)
		return m.openLabRunPane(firstNonEmptyString(run.workflow.RunID, run.workflow.WorkflowID))
	case modal.LabReviewEdit:
		store, err := labStoreFor(m.RepoPath)
		if err != nil {
			m.statusErr = err.Error()
			return m, clearErrorCmd()
		}
		path := store.ArtifactsDir(e.ID) + string(os.PathSeparator) + strings.ReplaceAll(msg.Path, "/", string(os.PathSeparator))
		return m, tea.ExecProcess(labEditorCommand(path), func(err error) tea.Msg {
			return labEditorClosedMsg{entryID: e.ID, err: err}
		})
	case modal.LabReviewApprove, modal.LabReviewDiscard:
		state := domain.LabReviewApproved
		verb := "Approved"
		if msg.Action == modal.LabReviewDiscard {
			state, verb = domain.LabReviewDiscarded, "Discarded"
		}
		repoPath := m.RepoPath
		return m, func() tea.Msg {
			store, err := labStoreFor(repoPath)
			if err != nil {
				return labsLoadedMsg{err: err}
			}
			artifacts, err := store.Artifacts(e.ID)
			if err != nil {
				return labsLoadedMsg{err: err}
			}
			for _, a := range artifacts {
				if a.Path != msg.Path {
					continue
				}
				if domain.LabContentHash(a.Body) != msg.Hash {
					return labsLoadedMsg{err: fmt.Errorf("%s changed while you were reading it; review it again", msg.Path)}
				}
				status := fmt.Sprintf("%s %s", verb, msg.Path)
				// An approved repository document goes into the checkout,
				// uncommitted; the approval stands only once it is there.
				if state == domain.LabReviewApproved && domain.IsLabRepositoryDocument(a.Path) {
					target, err := data.WriteLabDocument(repoPath, a)
					if err != nil {
						return labsLoadedMsg{err: err}
					}
					status = fmt.Sprintf("Approved %s — written to %s, uncommitted", msg.Path, target)
				}
				e.SetReview(a, state)
				e.Updated = time.Now().UTC()
				if err := store.Put(e); err != nil {
					return labsLoadedMsg{err: err}
				}
				entries, err := store.Load()
				return labsLoadedMsg{entries: entries, selectID: e.ID, status: status, err: err}
			}
			return labsLoadedMsg{err: fmt.Errorf("%s no longer exists", msg.Path)}
		}
	}
	return m, nil
}
