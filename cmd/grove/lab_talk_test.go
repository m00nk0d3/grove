package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLabCard(t *testing.T, store *data.LabStore, id string, n int) {
	t.Helper()
	dir := filepath.Join(store.EntryDir(id), "questions")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	card := fmt.Sprintf(`{"id":%d,"kind":"choice","question":"Question %d?","context":"lab.go","options":["Yes","No"],"recommended":0,"why":"w"}`, n, n)
	require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.json", n)), []byte(card), 0o644))
}

func writeLabSession(t *testing.T, store *data.LabStore, id string, phase domain.LabSessionPhase, output string) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"phase": phase, "output": output, "pending": []int{}})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(store.EntryDir(id), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(id), "session.json"), raw, 0o644))
}

// labTalkModel is a Lab with one grilling entry whose session run has the
// given status.
func labTalkModel(t *testing.T, status string) (*Model, *data.LabStore, domain.LabEntry) {
	t.Helper()
	store := data.NewLabStore(withLabCommonDir(t))
	e := entryAt(time.Now(), "e1", domain.LabKindIdea, domain.LabStatusGrilling, "Plugin API")
	e.Mode = domain.LabModeGrill
	e.Runs = []string{"run-1"}
	require.NoError(t, store.Put(e))
	m := newLabModel(t)
	m.lab.entries = []domain.LabEntry{e}
	m.lab.setMission(labMissionState(map[string]string{"run-1": status}))
	m.lab.setTab(labTabActive)
	return m, store, e
}

// feed runs cmd and the command its message produces, hops times, as Bubble
// Tea would. It stops before the status timers the last hop returns.
func feed(t *testing.T, m *Model, cmd tea.Cmd, hops int) *Model {
	t.Helper()
	for i := 0; i < hops; i++ {
		require.NotNil(t, cmd, "hop %d produced no command", i)
		updated, next := m.Update(cmd())
		m, cmd = updated.(*Model), next
	}
	return m
}

func TestLabAsk_RowsSayWhatTheSessionWaitsFor(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)

	m.lab.talks = loadLabTalks(store, m.lab.entries)
	assert.Equal(t, "Answer the agent in its pane", labNextAction(m.lab, e), "without a card or session state, the pane is the way")

	writeLabSession(t, store, e.ID, domain.LabPhaseFallback, "Which database?")
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	assert.Equal(t, "Reply to the agent", labNextAction(m.lab, e))

	writeLabSession(t, store, e.ID, domain.LabPhasePermission, "Run rm?")
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	assert.Equal(t, "Allow or deny a command", labNextAction(m.lab, e))

	writeLabCard(t, store, e.ID, 3)
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	assert.Equal(t, "Answer question 3", labNextAction(m.lab, e), "a waiting card comes first")
	writeLabCard(t, store, e.ID, 4)
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	assert.Equal(t, "Answer questions 3–4", labNextAction(m.lab, e))

	m.lab.setMission(labMissionState(map[string]string{"run-1": domain.WorkflowSucceeded}))
	assert.NotContains(t, labNextAction(m.lab, e), "Answer", "a finished session asks nothing")
}

func TestLabPage_CardsAreAnsweredInlineOneAfterAnother(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeLabCard(t, store, e.ID, 1)
	writeLabCard(t, store, e.ID, 2)
	m.lab.talks = loadLabTalks(store, m.lab.entries)

	m, _ = press(t, m, "enter")
	require.NotNil(t, m.lab.page, "Enter opens the entry page")
	require.Nil(t, m.activeModal, "the card is on the page, not in a modal")
	require.NotNil(t, m.lab.page.card)
	assert.Equal(t, "QUESTION 1 · 1 of 2", m.lab.page.card.Title())
	assert.Contains(t, m.View(), "Question 1?", "the page shows the card")

	m, _ = press(t, m, "2")
	m, _ = press(t, m, "tab")
	m = typeInto(t, m, "only for now. q?")
	assert.NotNil(t, m.lab.page, "keys typed in the note never reach the global key map")
	var cmd tea.Cmd
	m, cmd = press(t, m, "enter")
	m = feed(t, m, cmd, 2)

	var answer domain.LabAnswer
	raw, err := os.ReadFile(filepath.Join(store.EntryDir(e.ID), "questions", "001.answer.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &answer))
	assert.Equal(t, []int{1}, answer.Choices)
	assert.Equal(t, "only for now. q?", answer.Text)

	require.NotNil(t, m.lab.page.card, "the next waiting card replaces it")
	assert.Equal(t, 2, m.lab.page.card.Number())
	assert.Contains(t, m.View(), "DECISIONS (1)")

	m, cmd = press(t, m, "enter")
	m = feed(t, m, cmd, 2)
	assert.Nil(t, m.lab.page.card)
	assert.Equal(t, "Answer sent to the agent", m.statusMsg)

	m, _ = press(t, m, "esc")
	assert.Nil(t, m.lab.page, "Esc returns to the list")
}

func TestLabPage_AnAnswerIsChangedFromTheDecisionsLog(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeLabCard(t, store, e.ID, 1)
	writeLabCard(t, store, e.ID, 2)
	require.NoError(t, store.AnswerQuestion(e.ID, 1, []int{1}, "", time.Now()))
	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.json"), []byte(`{"phase":"question","stage":"interview","pending":[2]}`), 0o644))
	m.lab.talks = loadLabTalks(store, m.lab.entries)

	m, _ = press(t, m, "enter")
	m, _ = press(t, m, "tab") // the note
	m, _ = press(t, m, "tab") // the decisions log
	require.Equal(t, labFocusLog, m.lab.page.focus)
	m, _ = press(t, m, "enter")
	require.True(t, m.lab.page.revising, "Enter on a decision reopens its card")
	assert.Equal(t, "CHANGE ANSWER 1", m.lab.page.card.Title())

	m, _ = press(t, m, "1")
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd, 2)

	var answer domain.LabAnswer
	raw, err := os.ReadFile(filepath.Join(store.EntryDir(e.ID), "questions", "001.answer.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &answer))
	assert.Equal(t, []int{0}, answer.Choices)
	require.Len(t, answer.Revisions, 1, "the earlier answer is kept")
	assert.Equal(t, []int{1}, answer.Revisions[0].Choices)
	assert.Equal(t, 2, m.lab.page.card.Number(), "the waiting card comes back")
	assert.Contains(t, m.View(), "Yes (changed)")

	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.json"), []byte(`{"phase":"review","stage":"spec","pending":[]}`), 0o644))
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	m.syncLabPage()
	m.lab.page.focus = labFocusLog
	m, _ = press(t, m, "enter")
	assert.False(t, m.lab.page.revising, "after the interview, answers are not changed here")
	assert.Contains(t, m.statusMsg, "only while the agent is asking")
}

func TestLabPage_RepliesAndPermissionDecisionsBecomeRequests(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeLabSession(t, store, e.ID, domain.LabPhaseFallback, "Which database?")
	m.lab.talks = loadLabTalks(store, m.lab.entries)

	m, _ = press(t, m, "enter")
	require.NotNil(t, m.lab.page)
	assert.True(t, m.lab.page.replyOn, "a turn without a card opens a reply on the page")
	assert.Contains(t, m.View(), "Which database?", "the agent's output is shown")
	m = typeInto(t, m, "Postgres")
	m, cmd := press(t, m, "ctrl+s")
	m = feed(t, m, cmd, 1)
	assert.Equal(t, "Reply sent to the agent", m.statusMsg)
	assert.Contains(t, m.View(), "Waiting for the agent")

	writeLabSession(t, store, e.ID, domain.LabPhaseWorking, "")
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	m.syncLabPage()
	writeLabSession(t, store, e.ID, domain.LabPhasePermission, "Allow git push?")
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	m.syncLabPage()
	assert.Contains(t, m.View(), "PERMISSION REQUEST")
	m, cmd = press(t, m, "n")
	m = feed(t, m, cmd, 1)
	assert.Equal(t, "Denied; the agent continues", m.statusMsg)

	var reply, permission domain.LabRequest
	raw, err := os.ReadFile(filepath.Join(store.EntryDir(e.ID), "requests", "001.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &reply))
	raw, err = os.ReadFile(filepath.Join(store.EntryDir(e.ID), "requests", "002.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &permission))
	assert.Equal(t, domain.LabRequestReply, reply.Kind)
	assert.Equal(t, "Postgres", reply.Text)
	assert.Equal(t, domain.LabRequestPermission, permission.Kind)
	require.NotNil(t, permission.Allow)
	assert.False(t, *permission.Allow)
}

func TestLabPage_DraftsBetweenStagesAreReviewedInTheInspector(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeSession := func(raw string) {
		require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.json"), []byte(raw), 0o644))
		m.lab.talks = loadLabTalks(store, m.lab.entries)
	}
	require.NoError(t, os.MkdirAll(store.EntryDir(e.ID), 0o755))

	writeSession(`{"phase":"review","stage":"spec","pending":[]}`)
	assert.Equal(t, "Review the spec", labNextAction(m.lab, e))
	writeSession(`{"phase":"review","stage":"tickets","pending":[],"problems":["ticket 02 body: the \"## Why\" section is missing"]}`)
	assert.Equal(t, "Review the tickets", labNextAction(m.lab, e))
	m, _ = press(t, m, "enter")
	assert.Contains(t, m.View(), "could not fix these problems", "problems left in the draft are shown")
	assert.Contains(t, m.View(), "ticket 02 body")
	m, _ = press(t, m, "enter")
	require.IsType(t, &modal.LabInspectorModal{}, m.activeModal, "Enter opens the draft for review")

	m.activeModal = nil
	writeSession(`{"phase":"review","stage":"publish","pending":[]}`)
	assert.NotContains(t, labNextAction(m.lab, e), "Review the", "a session waiting to publish is published, not reviewed")
}

func TestLabPage_StepperFollowsTheRun(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	state := labMissionState(map[string]string{"run-1": domain.WorkflowBlocked})
	state.WorkflowRuns[0].Steps = []domain.WorkflowStep{
		{Title: "Scout", Status: "succeeded"},
		{Title: "Interview", Status: "blocked"},
		{Title: "Spec", Status: "queued"},
	}
	m.lab.setMission(state)
	require.NoError(t, os.MkdirAll(store.EntryDir(e.ID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "coverage.json"),
		[]byte(`{"scope":"covered","triggers":"covered","data":"open"}`), 0o644))
	m.lab.talks = loadLabTalks(store, m.lab.entries)

	steps := labPageSteps(m.lab, e)
	require.Len(t, steps, 3)
	assert.Equal(t, labPageStep{title: "Interview", status: "blocked", detail: "2/9"}, steps[1])

	idea := entryAt(time.Now(), "fresh", domain.LabKindIdea, domain.LabStatusDraft, "Fresh")
	bug := entryAt(time.Now(), "bug", domain.LabKindBug, domain.LabStatusDraft, "Bug")
	assert.Len(t, labPageSteps(m.lab, idea), 5, "an idea goes through the grill's steps")
	assert.Equal(t, "Review", labPageSteps(m.lab, bug)[1].title, "a bug is shaped, reviewed, and published")
}

func TestLabPage_ActionsPanelServesThePage(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeLabCard(t, store, e.ID, 1)
	require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.json"), []byte(`{"phase":"question","stage":"interview","pending":[1]}`), 0o644))
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	m, _ = press(t, m, "enter")

	m, _ = press(t, m, ".")
	assert.Equal(t, panelCtx, m.focused, ". opens the actions")
	labels := labelsOf(labContextActions(m.lab))
	assert.Contains(t, labels, "Write the spec now")
	assert.Contains(t, labels, "View agent")

	updated, cmd := m.handleLabAction(modal.ContextActionLabFinishInterview)
	m = feed(t, updated.(*Model), cmd, 1)
	assert.Equal(t, panelList, m.focused, "the page gets the keyboard back")
	raw, err := os.ReadFile(filepath.Join(store.EntryDir(e.ID), "requests", "001.json"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"finish_interview"`)

	m.focused = panelCtx
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*Model)
	assert.Equal(t, panelList, m.focused, "Esc in the actions returns to the page")
	assert.NotNil(t, m.lab.page)
}
