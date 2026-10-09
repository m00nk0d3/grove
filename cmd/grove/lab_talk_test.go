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

func TestLabAsk_CardsAreAnsweredInGroveOneAfterAnother(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeLabCard(t, store, e.ID, 1)
	writeLabCard(t, store, e.ID, 2)
	m.lab.talks = loadLabTalks(store, m.lab.entries)

	m, _ = press(t, m, "enter")
	card, ok := m.activeModal.(*modal.LabQuestionModal)
	require.True(t, ok, "Enter opens the first waiting card, got %T", m.activeModal)
	assert.Equal(t, 1, card.Number())
	assert.Equal(t, "QUESTION 1 · 1 of 2", card.Title())

	m, _ = press(t, m, "2")
	m, _ = press(t, m, "tab")
	m = typeInto(t, m, "only for now")
	var cmd tea.Cmd
	m, cmd = press(t, m, "enter")
	m = feed(t, m, cmd, 2)

	var answer domain.LabAnswer
	raw, err := os.ReadFile(filepath.Join(store.EntryDir(e.ID), "questions", "001.answer.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &answer))
	assert.Equal(t, []int{1}, answer.Choices)
	assert.Equal(t, "only for now", answer.Text)

	next, ok := m.activeModal.(*modal.LabQuestionModal)
	require.True(t, ok, "the next waiting card opens, got %T", m.activeModal)
	assert.Equal(t, 2, next.Number())

	m, cmd = press(t, m, "enter")
	m = feed(t, m, cmd, 2)
	assert.Nil(t, m.activeModal)
	assert.Equal(t, "Answer sent to the agent", m.statusMsg)
}

func TestLabAsk_RepliesAndPermissionDecisionsBecomeRequests(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeLabSession(t, store, e.ID, domain.LabPhaseFallback, "Which database?")
	m.lab.talks = loadLabTalks(store, m.lab.entries)

	m, _ = press(t, m, "enter")
	_, ok := m.activeModal.(*modal.LabMessageModal)
	require.True(t, ok, "a turn without a card opens a reply, got %T", m.activeModal)
	assert.Contains(t, m.activeModal.View(), "Which database?", "the agent's output is shown")
	m = typeInto(t, m, "Postgres")
	m, cmd := press(t, m, "ctrl+s")
	m = feed(t, m, cmd, 2)
	assert.Equal(t, "Reply sent to the agent", m.statusMsg)

	writeLabSession(t, store, e.ID, domain.LabPhasePermission, "Allow git push?")
	m.lab.talks = loadLabTalks(store, m.lab.entries)
	m, _ = press(t, m, "enter")
	require.IsType(t, &modal.LabPermissionModal{}, m.activeModal)
	m, cmd = press(t, m, "n")
	m = feed(t, m, cmd, 2)
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

func TestLabAsk_DraftsBetweenStagesAreReviewedInTheInspector(t *testing.T) {
	m, store, e := labTalkModel(t, domain.WorkflowBlocked)
	writeSession := func(stage string) {
		raw := fmt.Sprintf(`{"phase":"review","stage":%q,"pending":[]}`, stage)
		require.NoError(t, os.WriteFile(filepath.Join(store.EntryDir(e.ID), "session.json"), []byte(raw), 0o644))
		m.lab.talks = loadLabTalks(store, m.lab.entries)
	}
	require.NoError(t, os.MkdirAll(store.EntryDir(e.ID), 0o755))

	writeSession("spec")
	assert.Equal(t, "Review the spec", labNextAction(m.lab, e))
	writeSession("tickets")
	assert.Equal(t, "Review the tickets", labNextAction(m.lab, e))
	m, _ = press(t, m, "enter")
	require.IsType(t, &modal.LabInspectorModal{}, m.activeModal, "Enter opens the draft for review")

	m.activeModal = nil
	writeSession("publish")
	assert.NotContains(t, labNextAction(m.lab, e), "Review the", "a session waiting to publish is published, not reviewed")
}
