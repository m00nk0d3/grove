package main

import (
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/tui/modal"
)

// labTalk is what an entry's session has asked the user, read from the
// entry's question protocol files. See docs/LAB_DESIGN.md, "Question
// protocol".
type labTalk struct {
	session    domain.LabSession
	hasSession bool
	questions  []domain.LabQuestionRecord
	// coverage is the interview's topic coverage, once the agent writes it.
	coverage    domain.LabCoverage
	hasCoverage bool
	// artifacts are the drafts written so far, for review on the entry page.
	artifacts []domain.LabArtifact
	// docs are the drafted repository documents and what approving each
	// would change in the checkout.
	docs []labDoc
}

// labDoc is a drafted repository document, such as CONTEXT.md or a decision
// record, beside what approving it would change in the checkout.
type labDoc struct {
	artifact domain.LabArtifact
	change   data.LabDocumentChange
	err      error
}

// labArtifact returns the draft at path, if the agent has written it.
func (t labTalk) labArtifact(path string) (domain.LabArtifact, bool) {
	for _, a := range t.artifacts {
		if a.Path == path {
			return a, true
		}
	}
	return domain.LabArtifact{}, false
}

// loadLabTalks reads the protocol files and drafts of every entry that may
// have a session. Repository documents are compared with checkout's files.
func loadLabTalks(store *data.LabStore, checkout string, entries []domain.LabEntry) map[string]labTalk {
	talks := make(map[string]labTalk)
	for _, e := range entries {
		if e.Archived || e.Status == domain.LabStatusPublished || len(e.Runs) == 0 {
			continue
		}
		var t labTalk
		t.session, t.hasSession = store.Session(e.ID)
		t.questions, _ = store.Questions(e.ID)
		t.coverage, t.hasCoverage = store.Coverage(e.ID)
		t.artifacts, _ = store.Artifacts(e.ID)
		for _, a := range t.artifacts {
			if domain.IsLabRepositoryDocument(a.Path) {
				change, err := data.LabDocumentChanges(checkout, a)
				t.docs = append(t.docs, labDoc{artifact: a, change: change, err: err})
			}
		}
		talks[e.ID] = t
	}
	return talks
}

// labTalksLoadedMsg carries freshly read protocol files.
type labTalksLoadedMsg struct {
	talks map[string]labTalk
}

// loadLabTalksCmd rereads the protocol files of the repository's entries. It
// is nil when no entry has had a session.
func loadLabTalksCmd(repoPath string, entries []domain.LabEntry) tea.Cmd {
	hasRuns := false
	for _, e := range entries {
		hasRuns = hasRuns || len(e.Runs) > 0
	}
	if !hasRuns {
		return nil
	}
	return func() tea.Msg {
		store, err := labStoreFor(repoPath)
		if err != nil {
			return nil
		}
		return labTalksLoadedMsg{talks: loadLabTalks(store, repoPath, entries)}
	}
}

// labAsk is what a live session is waiting for from the user.
type labAsk int

const (
	labAskNone labAsk = iota
	labAskQuestion
	labAskReply
	labAskPermission
	labAskReview
)

// labStagePublish is the session step that waits for Grove to publish.
const labStagePublish = "publish"

// ask reports what e's live session is waiting for. A question card is
// answerable as soon as it appears; a reply or a permission decision only
// while the run is blocked on it.
func (v labView) ask(e domain.LabEntry) (labAsk, labTalk) {
	run, ok := v.latestRun(e)
	if !ok || !run.live() {
		return labAskNone, labTalk{}
	}
	t, ok := v.talks[e.ID]
	if !ok {
		return labAskNone, labTalk{}
	}
	if len(domain.LabPendingQuestions(t.questions)) > 0 {
		return labAskQuestion, t
	}
	if !run.waiting() || !t.hasSession {
		return labAskNone, t
	}
	switch t.session.Phase {
	case domain.LabPhaseFallback:
		return labAskReply, t
	case domain.LabPhasePermission:
		return labAskPermission, t
	case domain.LabPhaseReview:
		// A draft waiting between stages. Once every draft is approved the
		// session waits at Publish, which the Lab handles as publishing.
		if t.session.Stage != labStagePublish {
			return labAskReview, t
		}
	}
	return labAskNone, t
}

// labAskAction describes what Enter does for a session waiting on the user,
// or "" when it is not waiting for an answer.
func labAskAction(v labView, e domain.LabEntry) string {
	ask, t := v.ask(e)
	switch ask {
	case labAskQuestion:
		pending := domain.LabPendingQuestions(t.questions)
		if len(pending) > 1 {
			return fmt.Sprintf("Answer questions %d–%d", pending[0].Number, pending[len(pending)-1].Number)
		}
		return fmt.Sprintf("Answer question %d", pending[0].Number)
	case labAskReply:
		return "Reply to the agent"
	case labAskPermission:
		return "Allow or deny a command"
	case labAskReview:
		switch t.session.Stage {
		case "spec":
			return "Review the spec"
		case "tickets":
			return "Review the tickets"
		}
		return "Review the drafted bug report"
	}
	return ""
}

// openLabAsk opens the card for what e's session is waiting on, and reports
// whether there was one.
func (m *Model) openLabAsk(e domain.LabEntry) bool {
	ask, t := m.lab.ask(e)
	switch ask {
	case labAskQuestion:
		pending := domain.LabPendingQuestions(t.questions)
		position := ""
		if len(pending) > 1 {
			position = fmt.Sprintf("1 of %d", len(pending))
		}
		m.activeModal = modal.NewLabQuestionModal(e.ID, pending[0].Number, *pending[0].Question, position)
	case labAskReply:
		m.activeModal = modal.NewLabReplyModal(e.ID, t.session.Output)
	case labAskPermission:
		m.activeModal = modal.NewLabPermissionModal(e.ID, t.session.Output)
	case labAskReview:
		inspector := modal.NewLabInspectorModal(m.labInspectorState(e))
		inspector.ShowArtifacts()
		m.activeModal = inspector
	default:
		return false
	}
	return true
}

// labAnswerRecordedMsg reports an answer written, with the entry's questions
// as they are after it.
type labAnswerRecordedMsg struct {
	entryID   string
	questions []domain.LabQuestionRecord
	err       error
}

// handleLabAnswerSubmitted records an answer. The session runtime delivers it
// once every waiting card has one.
func (m *Model) handleLabAnswerSubmitted(msg modal.LabAnswerSubmittedMsg) (tea.Model, tea.Cmd) {
	repoPath := m.RepoPath
	return m, func() tea.Msg {
		store, err := labStoreFor(repoPath)
		if err != nil {
			return labAnswerRecordedMsg{entryID: msg.EntryID, err: err}
		}
		if msg.Revise {
			err = store.ReviseAnswer(msg.EntryID, msg.Number, msg.Choices, msg.Text, time.Now())
		} else {
			err = store.AnswerQuestion(msg.EntryID, msg.Number, msg.Choices, msg.Text, time.Now())
		}
		questions, loadErr := store.Questions(msg.EntryID)
		if err == nil {
			err = loadErr
		}
		return labAnswerRecordedMsg{entryID: msg.EntryID, questions: questions, err: err}
	}
}

// handleLabAnswerRecorded moves on to the next waiting card, if any.
func (m *Model) handleLabAnswerRecorded(msg labAnswerRecordedMsg) (tea.Model, tea.Cmd) {
	if msg.questions != nil {
		t := m.lab.talks[msg.entryID]
		t.questions = msg.questions
		if m.lab.talks == nil {
			m.lab.talks = make(map[string]labTalk)
		}
		m.lab.talks[msg.entryID] = t
	}
	if _, ok := m.activeModal.(*modal.LabQuestionModal); ok {
		m.activeModal = nil
	}
	if p := m.lab.page; p != nil && p.entryID == msg.entryID {
		// The page shows the next waiting card, if there is one.
		p.revising, p.card = false, nil
		m.syncLabPage()
	}
	if msg.err != nil {
		if errors.Is(msg.err, data.ErrLabAlreadyAnswered) {
			m.statusMsg = "That question was already answered"
			return m, clearMsgCmd()
		}
		m.statusErr = msg.err.Error()
		return m, clearErrorCmd()
	}
	if e, ok := m.lab.entry(msg.entryID); ok && m.lab.page == nil {
		if ask, _ := m.lab.ask(e); ask == labAskQuestion && m.openLabAsk(e) {
			return m, nil
		}
	}
	m.statusMsg = "Answer sent to the agent"
	return m, clearMsgCmd()
}

// labRequestSentMsg reports a request written for a session's agent.
type labRequestSentMsg struct {
	kind  domain.LabRequestKind
	allow bool
	err   error
}

// handleLabRequestSubmitted writes a reply, change request, or permission
// decision for the session runtime to deliver.
func (m *Model) handleLabRequestSubmitted(msg modal.LabRequestSubmittedMsg) (tea.Model, tea.Cmd) {
	switch m.activeModal.(type) {
	case *modal.LabMessageModal, *modal.LabPermissionModal:
		m.activeModal = nil
	}
	repoPath := m.RepoPath
	return m, func() tea.Msg {
		store, err := labStoreFor(repoPath)
		if err != nil {
			return labRequestSentMsg{err: err}
		}
		req := domain.LabRequest{Kind: msg.Kind, Text: msg.Text}
		if msg.Kind == domain.LabRequestPermission {
			allow := msg.Allow
			req.Allow = &allow
		}
		_, err = store.SendRequest(msg.EntryID, req, time.Now())
		return labRequestSentMsg{kind: msg.Kind, allow: msg.Allow, err: err}
	}
}

func (m *Model) handleLabRequestSent(msg labRequestSentMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusErr = msg.err.Error()
		return m, clearErrorCmd()
	}
	switch {
	case msg.kind == domain.LabRequestPermission && msg.allow:
		m.statusMsg = "Allowed; the agent continues"
	case msg.kind == domain.LabRequestPermission:
		m.statusMsg = "Denied; the agent continues"
	case msg.kind == domain.LabRequestChange:
		m.statusMsg = "Change request sent to the agent"
	default:
		m.statusMsg = "Reply sent to the agent"
	}
	return m, clearMsgCmd()
}

// ensureLabRepoMap builds the repository map for the checkout's commit,
// unless it exists. Tests replace it.
var ensureLabRepoMap = buildLabRepoMap

func buildLabRepoMap(store *data.LabStore, checkout string, tokens int) error {
	git := internalexec.NewGitCommand(checkout)
	commit, err := git.HeadCommit()
	if err != nil {
		return err
	}
	files, err := git.TrackedFiles()
	if err != nil {
		return err
	}
	_, err = store.EnsureRepoMap(checkout, files, commit, tokens)
	return err
}
