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
	EnsureLabels(repo string, labels []string) error
	AddSubIssue(repo string, parent, child int) error
	AddBlockedBy(repo string, issue, blocker int) error
}

var newLabGitHub = func(repoPath string) labGitHub { return internalexec.NewGitHubWriter(repoPath) }

// labConfigPath is where a remembered board choice is saved. Tests replace it.
var labConfigPath = data.DefaultConfigPath

// labBacklogStatus is the board status published issues are placed in.
const labBacklogStatus = "Backlog"

// Labels a publication applies.
const (
	labBugLabel    = "bug"
	labEpicLabel   = "epic"
	labTicketLabel = "ready-for-agent"
)

// labPublishRequires lists the artifacts that must be approved before e can be
// published, or nil when e cannot be published.
func labPublishRequires(e domain.LabEntry) []string {
	if e.Archived || e.Status == domain.LabStatusPublished {
		return nil
	}
	switch {
	case e.Mode == domain.LabModeShape && e.Status == domain.LabStatusShaping:
		return []string{domain.LabIssueArtifact}
	case e.Mode == domain.LabModeGrill && (e.Status == domain.LabStatusSpecced || e.Status == domain.LabStatusTicketed):
		return []string{domain.LabSpecArtifact, domain.LabTicketsArtifact}
	}
	return nil
}

// labDraft is what an entry publishes, read from its approved artifacts: one
// issue for a shaped bug, or an epic and its tickets for a grilled idea.
type labDraft struct {
	title   string
	body    string
	labels  []string
	tickets []domain.LabTicket
	// hash identifies every artifact that went into the draft, so publishing
	// can tell the preview is still current.
	hash string
}

// readLabDraft builds e's publication from its approved artifacts.
func readLabDraft(store *data.LabStore, e domain.LabEntry) (labDraft, error) {
	required := labPublishRequires(e)
	if required == nil {
		return labDraft{}, errors.New("this entry cannot be published")
	}
	artifacts, err := store.Artifacts(e.ID)
	if err != nil {
		return labDraft{}, err
	}
	approved := make(map[string]domain.LabArtifact)
	for _, path := range required {
		found := false
		for _, a := range artifacts {
			if a.Path != path {
				continue
			}
			found = true
			if e.ReviewOf(a) != domain.LabReviewApproved {
				return labDraft{}, fmt.Errorf("approve %s in the inspector before publishing", path)
			}
			approved[path] = a
		}
		if !found {
			return labDraft{}, fmt.Errorf("there is no %s to publish yet", path)
		}
	}

	var d labDraft
	var hashes []string
	if e.Mode == domain.LabModeShape {
		issue := approved[domain.LabIssueArtifact]
		d.title, d.body = domain.ParseIssueDraft(issue.Body)
		d.labels = []string{labBugLabel}
		hashes = append(hashes, domain.LabContentHash(issue.Body))
	} else {
		spec, tickets := approved[domain.LabSpecArtifact], approved[domain.LabTicketsArtifact]
		d.tickets, err = domain.ParseLabTickets(tickets.Body)
		if err != nil {
			return labDraft{}, err
		}
		d.title, d.body = domain.ParseIssueDraft(spec.Body)
		d.labels = []string{labEpicLabel}
		hashes = append(hashes, domain.LabContentHash(spec.Body), domain.LabContentHash(tickets.Body))
		// The epic summarises the approved glossary and decision records, so
		// the agents implementing its tickets have them before they are
		// committed.
		var docs []domain.LabArtifact
		for _, a := range artifacts {
			if domain.IsLabRepositoryDocument(a.Path) && e.ReviewOf(a) == domain.LabReviewApproved {
				docs = append(docs, a)
				hashes = append(hashes, a.Path+":"+domain.LabContentHash(a.Body))
			}
		}
		if section := labGlossarySection(docs); section != "" {
			d.body = strings.TrimSpace(d.body) + "\n\n" + section
		}
	}
	if d.title == "" {
		d.title = e.Title()
	}
	d.hash = domain.LabContentHash(strings.Join(hashes, "\n"))
	return d, nil
}

// labGlossarySection summarises approved repository documents for an epic:
// each decision record by its title and first paragraph, and each glossary in
// full, folded.
func labGlossarySection(docs []domain.LabArtifact) string {
	var decisions, glossaries []string
	for _, a := range docs {
		title, body := domain.ParseIssueDraft(a.Body)
		switch {
		case strings.HasSuffix(a.Path, "CONTEXT.md") || a.Path == "CONTEXT-MAP.md":
			glossaries = append(glossaries, fmt.Sprintf("<details>\n<summary><code>%s</code></summary>\n\n%s\n\n</details>", a.Path, strings.TrimSpace(a.Body)))
		case strings.Contains(a.Path, "docs/adr/"):
			if title == "" {
				title = a.Path
			}
			summary, _, _ := strings.Cut(strings.TrimSpace(body), "\n\n")
			decisions = append(decisions, fmt.Sprintf("- **%s** (`%s`): %s", title, a.Path, strings.TrimSpace(summary)))
		}
	}
	if len(decisions) == 0 && len(glossaries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Glossary and decisions\n\n")
	b.WriteString("Settled while this epic was grilled, and approved into the repository.\n")
	if len(decisions) > 0 {
		b.WriteString("\n### Decisions\n\n")
		b.WriteString(strings.Join(decisions, "\n"))
		b.WriteString("\n")
	}
	if len(glossaries) > 0 {
		b.WriteString("\n### Glossary\n\n")
		b.WriteString(strings.Join(glossaries, "\n\n"))
		b.WriteString("\n")
	}
	return b.String()
}

// labResumeNote describes what an earlier, interrupted publication of e
// already created, or "" when nothing was.
func labResumeNote(e domain.LabEntry, d labDraft) string {
	switch {
	case e.Mode == domain.LabModeShape && e.Issues.Issue != nil:
		return fmt.Sprintf("Issue #%d was created by an earlier attempt; it is placed on the board, not created again.", *e.Issues.Issue)
	case e.Mode == domain.LabModeGrill && e.Issues.Epic != nil:
		return fmt.Sprintf("Epic #%d and %d of %d tickets were created by an earlier attempt; publishing finishes the rest.",
			*e.Issues.Epic, len(e.Issues.Published), len(d.tickets))
	}
	return ""
}

// labPendingPublish is a previewed publication waiting for confirmation: what
// was shown, and the boards it may go to.
type labPendingPublish struct {
	plan     modal.LabPublishPlan
	hash     string
	tickets  []domain.LabTicket
	projects map[string]internalexec.LabProject
}

// labPublishPlanMsg carries a prepared preview, or why there is none.
type labPublishPlanMsg struct {
	pending labPendingPublish
	err     error
}

// labPublishedMsg reports a publication. Entries are reloaded even when it
// failed partway, so whatever was created is recorded.
type labPublishedMsg struct {
	loaded   labsLoadedMsg
	boardRef string
	chosen   bool
	err      error
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
		d, err := readLabDraft(store, e)
		if err != nil {
			return fail(err)
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
			hash:     d.hash,
			tickets:  d.tickets,
			projects: make(map[string]internalexec.LabProject),
			plan: modal.LabPublishPlan{
				EntryID: e.ID,
				Repo:    repo,
				Title:   d.title,
				Body:    d.body,
				Labels:  d.labels,
				Board:   -1,
				Resume:  labResumeNote(e, d),
			},
		}
		for _, t := range d.tickets {
			pending.plan.Tickets = append(pending.plan.Tickets, modal.LabPublishTicket{Key: t.Key, Title: t.Title, BlockedBy: t.BlockedBy})
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

// labPublication publishes one entry, saving its progress after every step
// that creates or links something, so an interrupted publication resumes
// instead of repeating work.
type labPublication struct {
	gh      labGitHub
	store   *data.LabStore
	entry   domain.LabEntry
	plan    modal.LabPublishPlan
	tickets []domain.LabTicket
	board   *internalexec.LabProject
}

func (p *labPublication) save() error {
	p.entry.Updated = time.Now().UTC()
	return p.store.Put(p.entry)
}

func (p *labPublication) issueURL(number int) string {
	return fmt.Sprintf("https://github.com/%s/issues/%d", p.plan.Repo, number)
}

func (p *labPublication) place(number int) error {
	if p.board == nil {
		return nil
	}
	if err := p.gh.PlaceOnBoard(*p.board, p.issueURL(number), labBacklogStatus); err != nil {
		return fmt.Errorf("created #%d, but %w — publish again to retry", number, err)
	}
	return nil
}

// issue publishes a shaped bug as one issue and returns its number.
func (p *labPublication) issue() (int, error) {
	if p.entry.Issues.Issue == nil {
		if err := p.gh.EnsureLabels(p.plan.Repo, p.plan.Labels); err != nil {
			return 0, err
		}
		number, _, err := p.gh.CreateIssue(p.plan.Repo, p.plan.Title, p.plan.Body, p.plan.Labels)
		if err != nil {
			return 0, err
		}
		p.entry.Issues.Issue = &number
		if err := p.save(); err != nil {
			return 0, fmt.Errorf("created issue #%d but could not record it: %w", number, err)
		}
	}
	number := *p.entry.Issues.Issue
	return number, p.place(number)
}

// epic publishes a grilled idea: the epic, then its tickets blockers first,
// each a sub-issue of the epic, then their blocked-by links, then the board.
func (p *labPublication) epic() (int, error) {
	if err := p.gh.EnsureLabels(p.plan.Repo, []string{labEpicLabel, labTicketLabel}); err != nil {
		return 0, err
	}
	if p.entry.Issues.Epic == nil {
		number, _, err := p.gh.CreateIssue(p.plan.Repo, p.plan.Title, p.plan.Body, []string{labEpicLabel})
		if err != nil {
			return 0, err
		}
		p.entry.Issues.Epic = &number
		if err := p.save(); err != nil {
			return 0, fmt.Errorf("created epic #%d but could not record it: %w", number, err)
		}
	}
	epic := *p.entry.Issues.Epic
	if p.entry.Issues.Published == nil {
		p.entry.Issues.Published = make(map[string]domain.LabPublishedTicket)
	}
	progress := func(done int) error {
		return fmt.Errorf("epic #%d and %d of %d tickets are published", epic, done, len(p.tickets))
	}

	for _, t := range p.tickets {
		if _, ok := p.entry.Issues.Published[t.Key]; ok {
			continue
		}
		number, _, err := p.gh.CreateIssue(p.plan.Repo, t.Title, strings.TrimSpace(t.Body), []string{labTicketLabel})
		if err != nil {
			return 0, fmt.Errorf("%w; creating ticket %s failed: %v — publish again to finish", progress(len(p.entry.Issues.Published)), t.Key, err)
		}
		p.entry.Issues.Published[t.Key] = domain.LabPublishedTicket{Number: number}
		p.entry.Issues.Tickets = append(p.entry.Issues.Tickets, number)
		if err := p.save(); err != nil {
			return 0, fmt.Errorf("created ticket #%d but could not record it: %w", number, err)
		}
	}

	for _, t := range p.tickets {
		published := p.entry.Issues.Published[t.Key]
		if !published.SubIssue {
			if err := p.gh.AddSubIssue(p.plan.Repo, epic, published.Number); err != nil {
				return 0, fmt.Errorf("every ticket is created, but %w — publish again to finish", err)
			}
			published.SubIssue = true
			p.entry.Issues.Published[t.Key] = published
			if err := p.save(); err != nil {
				return 0, err
			}
		}
		for _, blocker := range t.BlockedBy {
			if containsString(published.BlockedBy, blocker) {
				continue
			}
			if err := p.gh.AddBlockedBy(p.plan.Repo, published.Number, p.entry.Issues.Published[blocker].Number); err != nil {
				return 0, fmt.Errorf("every ticket is created, but %w — publish again to finish", err)
			}
			published.BlockedBy = append(published.BlockedBy, blocker)
			p.entry.Issues.Published[t.Key] = published
			if err := p.save(); err != nil {
				return 0, err
			}
		}
	}

	if err := p.place(epic); err != nil {
		return 0, err
	}
	for _, t := range p.tickets {
		if err := p.place(p.entry.Issues.Published[t.Key].Number); err != nil {
			return 0, err
		}
	}
	return epic, nil
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// publishLabEntry performs a confirmed publication. It holds the entry's lock,
// publishes exactly what the preview showed, and records each issue and link
// the moment it exists, so an interrupted publication resumes instead of
// duplicating anything.
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
		d, err := readLabDraft(store, e)
		if err != nil {
			return done(err, store, "")
		}
		if d.hash != pending.hash {
			return done(errors.New("the drafts changed after the preview; review them and publish again"), store, "")
		}

		pub := &labPublication{gh: newLabGitHub(repoPath), store: store, entry: e, plan: pending.plan, tickets: pending.tickets}
		if msg.BoardRef != "" {
			board, ok := pending.projects[msg.BoardRef]
			if !ok {
				return done(fmt.Errorf("unknown board %s", msg.BoardRef), store, "")
			}
			pub.board = &board
		}
		var number int
		var status string
		if e.Mode == domain.LabModeShape {
			number, err = pub.issue()
			status = fmt.Sprintf("Published %q as #%d in %s", pub.plan.Title, number, pub.plan.Repo)
		} else {
			number, err = pub.epic()
			status = fmt.Sprintf("Published epic %q as #%d with %d tickets in %s", pub.plan.Title, number, len(pub.tickets), pub.plan.Repo)
		}
		if err != nil {
			return done(err, store, "")
		}

		pub.entry.Status = domain.LabStatusPublished
		if err := pub.save(); err != nil {
			return done(err, store, "")
		}
		return done(store.CloseSession(e.ID), store, status)
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
