package modal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLabInspector(state LabInspectorState) *LabInspectorModal {
	m := NewLabInspectorModal(state)
	m.SetWidth(140)
	m.SetHeight(40)
	m.SetTheme(styles.NewTheme(styles.Themes[0]))
	return m
}

func labKey(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func draftState() LabInspectorState {
	return LabInspectorState{
		Entry: domain.LabEntry{
			ID: "e1", Kind: domain.LabKindIdea, Status: domain.LabStatusDraft,
			Text: "Plugin API\n\nLet users extend views.", Created: time.Now().Add(-2 * time.Hour), Updated: time.Now(),
		},
		Badge: "DRAFT",
	}
}

func grillingState(status string) LabInspectorState {
	s := draftState()
	s.Entry.Status = domain.LabStatusSpecced
	s.Entry.Mode = domain.LabModeGrill
	s.Badge = "SPECCED"
	s.StageLabel = "Spec 2/4"
	if status == domain.WorkflowBlocked {
		s.Badge, s.Attention = "WAITING ON YOU", true
	}
	s.Runs = []LabInspectorRun{{
		Workflow: domain.WorkflowRunRef{
			RunID: "run-1", Kind: "grill", Status: status, CurrentStep: "Spec", DefaultAgent: "claude",
			StartedAt: time.Now().Add(-30 * time.Minute),
			Steps: []domain.WorkflowStep{
				{Title: "Interview", Status: "succeeded"},
				{Title: "Spec", Status: "running"},
			},
		},
		PaneID: "w1:p3",
	}}
	return s
}

func TestLabInspector_OverviewOfDraft(t *testing.T) {
	view := newTestLabInspector(draftState()).View()
	assert.Contains(t, view, "LAB // ENTRY INSPECTOR")
	assert.Contains(t, view, "Plugin API")
	assert.Contains(t, view, "DRAFT")
	assert.Contains(t, view, "No session yet.")
	assert.Contains(t, view, "Grill the entry to start one.")
	assert.NotContains(t, view, "open pane", "a draft has no pane to open")
}

func TestLabInspector_OverviewOfBugSuggestsShaping(t *testing.T) {
	s := draftState()
	s.Entry.Kind = domain.LabKindBug
	assert.Contains(t, newTestLabInspector(s).View(), "Shape the entry to start one.")
}

func TestLabInspector_OverviewOfWaitingRun(t *testing.T) {
	view := newTestLabInspector(grillingState(domain.WorkflowBlocked)).View()
	assert.Contains(t, view, "WAITING ON YOU")
	assert.Contains(t, view, "run-1")
	assert.Contains(t, view, "Herdr w1:p3")
	assert.Contains(t, view, "claude")
	assert.Contains(t, view, "open pane")
}

func TestLabInspector_EnterOpensLiveRunPane(t *testing.T) {
	m := newTestLabInspector(grillingState(domain.WorkflowRunning))
	_, cmd := m.Update(labKey("enter"))
	require.NotNil(t, cmd)
	assert.Equal(t, LabOpenPaneMsg{RunID: "run-1"}, cmd())
}

func TestLabInspector_EnterDoesNothingWithoutLiveRun(t *testing.T) {
	_, cmd := newTestLabInspector(draftState()).Update(labKey("enter"))
	assert.Nil(t, cmd)
	_, cmd = newTestLabInspector(grillingState(domain.WorkflowSucceeded)).Update(labKey("enter"))
	assert.Nil(t, cmd, "a finished run's pane is not offered")
}

func TestLabInspector_StepsShowChainAndRuns(t *testing.T) {
	m := newTestLabInspector(grillingState(domain.WorkflowBlocked))
	m.Update(labKey("2"))
	view := m.View()

	assert.Contains(t, view, "BUILD CHAIN  2/4")
	assert.Contains(t, view, "✓  Interview", "steps before the current one are done")
	assert.Contains(t, view, "◆  Spec", "the step waiting on the user is marked")
	assert.Contains(t, view, "○  Tickets")
	assert.Contains(t, view, "RUNS  1")
	assert.Contains(t, view, "grill")
}

func TestLabInspector_ArtifactsAreRequestedAndShown(t *testing.T) {
	m := newTestLabInspector(draftState())
	cmd := m.Init()
	require.NotNil(t, cmd)
	assert.Equal(t, LabArtifactsRequestedMsg{EntryID: "e1"}, cmd())

	m.Update(labKey("3"))
	assert.Contains(t, m.View(), "Loading artifacts")

	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: []domain.LabArtifact{
		{Path: "CONTEXT.md", Body: "# Context\n\n**Entry**: an idea."},
		{Path: "tickets.json", Body: `[{"title":"First slice"}]`},
	}})
	view := m.View()
	assert.Contains(t, view, "CONTEXT.md")
	assert.Contains(t, view, "tickets.json")
	assert.Contains(t, view, "Context")

	m.Update(labKey("]"))
	assert.Contains(t, m.View(), "First slice", "] selects the next artifact")
}

func TestLabInspector_ArtifactsForAnotherEntryAreIgnored(t *testing.T) {
	m := newTestLabInspector(draftState())
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "other", Artifacts: []domain.LabArtifact{{Path: "spec.md"}}})
	m.Update(labKey("3"))
	assert.Contains(t, m.View(), "Loading artifacts")
}

func TestLabInspector_ReloadKeepsSelectedArtifact(t *testing.T) {
	m := newTestLabInspector(draftState())
	artifacts := []domain.LabArtifact{{Path: "CONTEXT.md"}, {Path: "spec.md"}}
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: artifacts})
	m.Update(labKey("3"))
	m.Update(labKey("]"))

	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: append([]domain.LabArtifact{{Path: "adr/0001-x.md"}}, artifacts...)})
	assert.Equal(t, "spec.md", m.artifacts[m.selected].Path)
}

func TestLabInspector_EmptyArtifactsExplainWhatAppears(t *testing.T) {
	m := newTestLabInspector(draftState())
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1"})
	m.Update(labKey("3"))
	assert.Contains(t, m.View(), "Nothing drafted yet")
}

func TestLabInspector_ArtifactErrorOffersRetry(t *testing.T) {
	m := newTestLabInspector(draftState())
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Err: fmt.Errorf("permission denied")})
	m.Update(labKey("3"))
	assert.Contains(t, m.View(), "Could not read artifacts: permission denied")
	assert.Contains(t, m.View(), "Press r to try again")
}

func TestLabInspector_LongDocumentScrolls(t *testing.T) {
	var body strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&body, "line %d\n\n", i)
	}
	m := newTestLabInspector(draftState())
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: []domain.LabArtifact{{Path: "spec.md", Body: body.String()}}})
	m.Update(labKey("3"))
	first := m.View()
	assert.Contains(t, first, "line 1")

	m.Update(labKey("pgdown"))
	assert.NotContains(t, m.View(), "line 1\n", "paging moves the document")
	m.Update(labKey("G"))
	assert.Contains(t, m.View(), "line 200")
	assert.Contains(t, m.View(), "of ")
}

func TestLabInspector_CaptureShowsText(t *testing.T) {
	m := newTestLabInspector(draftState())
	m.Update(labKey("4"))
	assert.Contains(t, m.View(), "Let users extend views.")
}

func TestLabInspector_TabCyclesAndEscCloses(t *testing.T) {
	m := newTestLabInspector(draftState())
	for i := 0; i < int(labInspectorTabCount); i++ {
		m.Update(labKey("tab"))
	}
	assert.Equal(t, labInspectorOverview, m.activeTab, "Tab wraps around")

	_, cmd := m.Update(labKey("esc"))
	require.NotNil(t, cmd)
	assert.IsType(t, ModalCancelledMsg{}, cmd())
}

func TestLabInspector_SetStateUpdatesView(t *testing.T) {
	m := newTestLabInspector(grillingState(domain.WorkflowRunning))
	assert.NotContains(t, m.View(), "WAITING ON YOU")
	m.SetState(grillingState(domain.WorkflowBlocked))
	assert.Contains(t, m.View(), "WAITING ON YOU")
}

func reviewState(approved bool) (LabInspectorState, domain.LabArtifact) {
	s := grillingState(domain.WorkflowBlocked)
	s.Entry.Mode, s.Entry.Status = domain.LabModeShape, domain.LabStatusShaping
	s.PublishRequires = []string{"issue.md"}
	issue := domain.LabArtifact{Path: "issue.md", Body: "# Sync stalls"}
	if approved {
		s.Entry.SetReview(issue, domain.LabReviewApproved)
	}
	return s, issue
}

func TestLabInspector_ReviewKeysActOnSelectedArtifact(t *testing.T) {
	s, issue := reviewState(false)
	m := newTestLabInspector(s)
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: []domain.LabArtifact{issue}})

	_, cmd := m.Update(labKey("a"))
	assert.Nil(t, cmd, "review keys only work in the Artifacts tab")

	m.ShowArtifacts()
	for key, action := range map[string]string{"a": LabReviewApprove, "x": LabReviewDiscard, "e": LabReviewEdit, "c": LabReviewChanges} {
		_, cmd := m.Update(labKey(key))
		require.NotNil(t, cmd, key)
		assert.Equal(t, LabArtifactReviewMsg{EntryID: "e1", Path: "issue.md", Hash: domain.LabContentHash(issue.Body), Action: action}, cmd(), key)
	}
	assert.Contains(t, m.View(), "◌ issue.md")
	assert.Contains(t, m.View(), "DRAFT")
}

func TestLabInspector_PublishNeedsEveryRequiredApproval(t *testing.T) {
	s, issue := reviewState(false)
	m := newTestLabInspector(s)
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: []domain.LabArtifact{issue}})
	_, cmd := m.Update(labKey("p"))
	assert.Nil(t, cmd, "an unapproved draft cannot be published")
	assert.NotContains(t, m.View(), "publish")

	s, issue = reviewState(true)
	m.SetState(s)
	m.ShowArtifacts()
	assert.Contains(t, m.View(), "✓ issue.md")
	_, cmd = m.Update(labKey("p"))
	require.NotNil(t, cmd)
	assert.Equal(t, LabPublishRequestedMsg{EntryID: "e1"}, cmd())

	// The agent revises the draft: the approval no longer applies.
	m.SetArtifacts(LabArtifactsLoadedMsg{EntryID: "e1", Artifacts: []domain.LabArtifact{{Path: "issue.md", Body: "# Revised"}}})
	_, cmd = m.Update(labKey("p"))
	assert.Nil(t, cmd)
	assert.Contains(t, m.View(), "◌ issue.md")
}
