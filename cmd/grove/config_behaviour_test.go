package main

import (
	"path/filepath"
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncTick_IsIgnoredWhenAutoSyncIsOff(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.Config.GitHub.AutoSync = false

	_, cmd := model.Update(syncTickMsg{gen: model.syncTickGen})

	assert.Nil(t, cmd, "no background sync runs with auto sync off")
	assert.False(t, model.syncing)
}

func TestSyncTick_OnlyTheLatestScheduledTickSyncs(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.Config.GitHub.AutoSync = true
	require.NotNil(t, model.scheduleSyncTick())
	stale := syncTickMsg{gen: model.syncTickGen}
	require.NotNil(t, model.scheduleSyncTick(), "a later sync schedules its own tick")

	_, cmd := model.Update(stale)
	assert.Nil(t, cmd, "a superseded tick does not start a second sync chain")

	_, cmd = model.Update(syncTickMsg{gen: model.syncTickGen})
	assert.NotNil(t, cmd)
}

func TestScheduleSyncTick_SchedulesNothingWithAutoSyncOff(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.Config.GitHub.AutoSync = false

	assert.Nil(t, model.scheduleSyncTick())
	assert.Zero(t, model.syncTickInterval)
}

func TestSettingsSaved_StartsAndStopsAutoSync(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.Config.GitHub.AutoSync = false
	model.scheduleSyncTick()
	model.activeModal = modal.NewSettingsModal(model.Config, filepath.Join(t.TempDir(), "config.toml"))

	model.Config.GitHub.AutoSync = true
	_, cmd := model.Update(modal.SettingsSavedMsg{Config: model.Config})
	require.NotNil(t, cmd)
	assert.Equal(t, model.Config.GitHub.SyncInterval(), model.syncTickInterval, "turning auto sync on schedules a tick now")

	gen := model.syncTickGen
	model.Config.GitHub.SyncIntervalMinutes = 30
	model.Update(modal.SettingsSavedMsg{Config: model.Config})
	assert.Greater(t, model.syncTickGen, gen, "a new interval replaces the pending tick")
	assert.Equal(t, model.Config.GitHub.SyncInterval(), model.syncTickInterval)

	model.Config.GitHub.AutoSync = false
	model.Update(modal.SettingsSavedMsg{Config: model.Config})
	assert.Zero(t, model.syncTickInterval, "turning auto sync off leaves no tick pending")
}

func TestCreateModal_UsesTheConfiguredWorktreeRoot(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "grove")
	issue := domain.Issue{Number: 7, Title: "Add thing"}
	create := modal.NewCreateModalForIssue(issue, repo)
	create.SetWorktreeConfig(domain.WorktreesConfig{WorktreeRoot: ".trees"})

	assert.Equal(t, filepath.Join(repo, ".trees", "grove", "feat-issue-7-"), create.WorktreePath())
}

func TestPRWorktreePath_UsesTheConfiguredWorktreeRoot(t *testing.T) {
	root := t.TempDir()

	got := prWorktreePath(filepath.Join(root, "repo"), domain.WorktreesConfig{WorktreeRoot: filepath.Join(root, "wt")}, "fix/issue-3-x")

	assert.Equal(t, filepath.Join(root, "wt", "repo", "fix-issue-3-x"), got)
}
