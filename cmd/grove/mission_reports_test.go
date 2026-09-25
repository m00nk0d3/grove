package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/mission"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCommonDir replaces gitCommonDir for one test, recording the directory it
// was asked about.
func stubCommonDir(t *testing.T, commonDir string, err error) *string {
	t.Helper()
	asked := new(string)
	previous := gitCommonDir
	gitCommonDir = func(dir string) (string, error) {
		*asked = dir
		return commonDir, err
	}
	t.Cleanup(func() { gitCommonDir = previous })
	return asked
}

func impWorkflow(worktree string) domain.WorkflowRunRef {
	n := 42
	return domain.WorkflowRunRef{RunID: "run-42", Kind: "imp", IssueNumber: &n, WorktreePath: worktree}
}

func TestLoadMissionReports_ReadsFromTheSharedGitDirectory(t *testing.T) {
	common := t.TempDir()
	reports := filepath.Join(common, mission.ReportsDir)
	require.NoError(t, os.MkdirAll(reports, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(reports, "issue-42-implementation-report.md"), []byte("# Report"), 0o644))
	worktree := t.TempDir()
	asked := stubCommonDir(t, common, nil)

	msg := loadMissionReportsCmd(modal.MissionReportsRequestedMsg{RunID: "run-42", Workflow: impWorkflow(worktree)}, "/repo")().(modal.MissionReportsLoadedMsg)

	require.NoError(t, msg.Err)
	assert.Equal(t, "run-42", msg.RunID)
	assert.True(t, msg.Supported)
	require.Len(t, msg.Reports, 1)
	assert.Equal(t, "# Report", msg.Reports[0].Body)
	assert.Equal(t, worktree, *asked, "the run's own worktree is used while it exists")
}

func TestLoadMissionReports_FallsBackToTheRepositoryWhenTheWorktreeIsGone(t *testing.T) {
	asked := stubCommonDir(t, t.TempDir(), nil)

	msg := loadMissionReportsCmd(modal.MissionReportsRequestedMsg{RunID: "run-42", Workflow: impWorkflow(filepath.Join(t.TempDir(), "removed"))}, "/repo")().(modal.MissionReportsLoadedMsg)

	require.NoError(t, msg.Err)
	assert.Empty(t, msg.Reports)
	assert.Equal(t, "/repo", *asked)
}

func TestLoadMissionReports_ReportsAGitFailure(t *testing.T) {
	stubCommonDir(t, "", errors.New("not a git repository"))

	msg := loadMissionReportsCmd(modal.MissionReportsRequestedMsg{RunID: "run-42", Workflow: impWorkflow("")}, "/repo")().(modal.MissionReportsLoadedMsg)

	require.Error(t, msg.Err)
	assert.Contains(t, msg.Err.Error(), "not a git repository")
}

func TestLoadMissionReports_SkipsGitForAKindWithoutReports(t *testing.T) {
	asked := stubCommonDir(t, "", errors.New("must not be called"))

	msg := loadMissionReportsCmd(modal.MissionReportsRequestedMsg{RunID: "run-7", Workflow: domain.WorkflowRunRef{Kind: "clean"}}, "/repo")().(modal.MissionReportsLoadedMsg)

	assert.NoError(t, msg.Err)
	assert.False(t, msg.Supported)
	assert.Empty(t, *asked)
}

func TestModel_RoutesLoadedReportsToTheInspectorThatAsked(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	inspector := modal.NewMissionModal(impWorkflow(""), nil)
	model.activeModal = inspector
	inspector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	report := domain.WorkflowReport{Title: "Implementation report", Path: "/g/issue-42-implementation-report.md", Body: "# Delivered"}

	model.Update(modal.MissionReportsLoadedMsg{RunID: "another-run", Supported: true, Reports: []domain.WorkflowReport{report}})
	assert.NotContains(t, inspector.View(), "Delivered", "a reply for another run is ignored")

	model.Update(modal.MissionReportsLoadedMsg{RunID: inspector.RunID(), Supported: true, Reports: []domain.WorkflowReport{report}})
	assert.Contains(t, inspector.View(), "Delivered")
}

func TestModel_AnswersAReportsRequest(t *testing.T) {
	stubCommonDir(t, t.TempDir(), nil)
	model := NewModel()
	require.NotNil(t, model)
	model.activeModal = modal.NewMissionModal(impWorkflow(""), nil)

	_, cmd := model.Update(modal.MissionReportsRequestedMsg{RunID: "run-42", Workflow: impWorkflow("")})

	require.NotNil(t, cmd)
	msg, ok := cmd().(modal.MissionReportsLoadedMsg)
	require.True(t, ok)
	assert.Equal(t, "run-42", msg.RunID)
}
