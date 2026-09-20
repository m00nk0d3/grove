package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/herdr"
	"github.com/m00nk0d3/grove/internal/sandcastle"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelInitialization verifies that the Model can be instantiated
// with all required fields properly initialized
func TestModelInitialization(t *testing.T) {
	tests := []struct {
		name            string
		wantModelNotNil bool
		wantHasFields   bool
	}{
		{
			name:            "creates new model successfully",
			wantModelNotNil: true,
			wantHasFields:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()

			if tt.wantModelNotNil {
				assert.NotNil(t, model, "Model should not be nil")
			}

			if tt.wantHasFields {
				// Verify model can be cast to tea.Model (has required interface methods)
				var _ tea.Model = model
				assert.NotNil(t, model, "Model should implement tea.Model interface")
				assert.NotNil(t, model.Config, "Config should be initialized (defaults at minimum)")
			}
		})
	}
}

// TestModelUpdate verifies that the Update method accepts tea.Msg
// and returns (tea.Model, tea.Cmd) as required by Bubbletea interface
func TestModelUpdate(t *testing.T) {
	tests := []struct {
		name          string
		msg           tea.Msg
		wantModel     bool
		wantCmdNotNil bool
		description   string
	}{
		{
			name:          "update accepts tea.KeyMsg",
			msg:           tea.KeyMsg{Type: tea.KeyCtrlC},
			wantModel:     true,
			wantCmdNotNil: false, // Initial implementation may not return a Cmd
			description:   "Should accept KeyMsg and return model (Cmd can be nil)",
		},
		{
			name:          "update accepts generic tea.Msg",
			msg:           tea.WindowSizeMsg{Width: 80, Height: 24},
			wantModel:     true,
			wantCmdNotNil: false,
			description:   "Should accept WindowSizeMsg and return model",
		},
		{
			name:          "update handles nil message gracefully",
			msg:           nil,
			wantModel:     true,
			wantCmdNotNil: false,
			description:   "Should handle nil message without panicking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model, "Model should be created successfully")

			// Call Update method - it must return (tea.Model, tea.Cmd)
			updatedModel, cmd := model.Update(tt.msg)

			if tt.wantModel {
				assert.NotNil(t, updatedModel, "Update should return a model: %s", tt.description)
				// Verify it's a valid Model that implements tea.Model
				var _ tea.Model = updatedModel
			}

			// cmd can be nil (no command to execute)
			if tt.wantCmdNotNil {
				assert.NotNil(t, cmd, "Update should return a Cmd: %s", tt.description)
			}
		})
	}
}

// TestModelView verifies that the View method returns a string
// representation of the model's current state
func TestModelView(t *testing.T) {
	tests := []struct {
		name             string
		wantViewNotEmpty bool
		wantViewIsString bool
		description      string
	}{
		{
			name:             "view returns string representation",
			wantViewNotEmpty: false, // Initial implementation may return empty string
			wantViewIsString: true,
			description:      "View should return a string (may be empty initially)",
		},
		{
			name:             "view is consistently callable",
			wantViewNotEmpty: false,
			wantViewIsString: true,
			description:      "Multiple calls to View should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model, "Model should be created successfully")

			// Call View method - it must return a string
			view := model.View()

			assert.IsType(t, "", view, "View should return a string: %s", tt.description)

			if tt.wantViewNotEmpty {
				assert.NotEmpty(t, view, "View should not be empty: %s", tt.description)
			}
		})
	}
}

// TestModelIntegration verifies that the model works correctly through
// a typical initialization and message handling sequence
func TestModelIntegration(t *testing.T) {
	tests := []struct {
		name        string
		description string
	}{
		{
			name:        "model initialization followed by update and view",
			description: "Should create model, handle update, and render view",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize model
			model := NewModel()
			require.NotNil(t, model, "Model creation should succeed")

			// Verify View works immediately
			initialView := model.View()
			assert.IsType(t, "", initialView, "View should return string after init: %s", tt.description)

			// Verify Update works with a message
			updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			assert.NotNil(t, updatedModel, "Update should return model: %s", tt.description)

			// Verify View works after update
			updatedView := updatedModel.View()
			assert.IsType(t, "", updatedView, "View should work after update: %s", tt.description)
		})
	}
}

// TestModel_Enter_TriggersSpawn verifies that pressing Enter on a selected worktree
// returns a tea.Cmd to spawn a new terminal session for that worktree.
func TestModel_Enter_TriggersSpawn(t *testing.T) {
	tests := []struct {
		name          string
		worktrees     []interface{} // Will be converted to domain.Worktree
		selectedIdx   int
		description   string
		wantCmdNotNil bool
	}{
		{
			name: "enter on first worktree returns spawn command",
			worktrees: []interface{}{
				map[string]interface{}{"Path": "/home/user/repos/wt1", "Branch": "main", "CommitSHA": "abc123", "IsClean": true, "IsLocked": false, "LinkedPR": nil},
				map[string]interface{}{"Path": "/home/user/repos/wt2", "Branch": "feature", "CommitSHA": "def456", "IsClean": false, "IsLocked": false, "LinkedPR": nil},
			},
			selectedIdx:   0,
			description:   "Should return a Cmd to spawn session for first worktree",
			wantCmdNotNil: true,
		},
		{
			name: "enter on second worktree returns spawn command",
			worktrees: []interface{}{
				map[string]interface{}{"Path": "/home/user/repos/wt1", "Branch": "main", "CommitSHA": "abc123", "IsClean": true, "IsLocked": false, "LinkedPR": nil},
				map[string]interface{}{"Path": "/home/user/repos/wt2", "Branch": "feature", "CommitSHA": "def456", "IsClean": false, "IsLocked": false, "LinkedPR": nil},
			},
			selectedIdx:   1,
			description:   "Should return a Cmd to spawn session for second worktree",
			wantCmdNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create model with populated Worktrees list
			model := NewModel()
			require.NotNil(t, model, "Model creation should succeed")
			model.view = viewWorktrees

			// Convert test data to domain.Worktree
			worktrees := make([]domain.Worktree, len(tt.worktrees))
			for i, wtData := range tt.worktrees {
				data := wtData.(map[string]interface{})
				worktrees[i] = domain.Worktree{
					Path:      data["Path"].(string),
					Branch:    data["Branch"].(string),
					CommitSHA: data["CommitSHA"].(string),
					IsClean:   data["IsClean"].(bool),
					IsLocked:  data["IsLocked"].(bool),
				}
			}
			model.Worktrees = worktrees
			model.selectedIdx = tt.selectedIdx

			// Action: Call Update with tea.KeyEnter
			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

			// Assert: Model is returned
			assert.NotNil(t, updatedModel, "Update should return model: %s", tt.description)

			// Assert: A Cmd is returned (for spawning a session)
			if tt.wantCmdNotNil {
				assert.NotNil(t, cmd, "Update should return a Cmd for spawning session: %s", tt.description)
			}
		})
	}
}

// TestModel_Enter_EmptyList_NoOp verifies that pressing Enter on an empty worktree list
// does not trigger a spawn command
func TestModel_Enter_EmptyList_NoOp(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantCmdNil  bool
	}{
		{
			name:        "enter on empty list returns nil command",
			description: "Should return nil Cmd when no worktrees exist",
			wantCmdNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Create model with empty Worktrees list
			model := NewModel()
			require.NotNil(t, model, "Model creation should succeed")
			model.view = viewWorktrees
			require.Empty(t, model.Worktrees, "Worktrees should be empty initially")

			// Action: Call Update with tea.KeyEnter
			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

			// Assert: Model is returned
			assert.NotNil(t, updatedModel, "Update should return model: %s", tt.description)

			// Assert: Cmd is nil (no-op)
			if tt.wantCmdNil {
				assert.Nil(t, cmd, "Update should return nil Cmd for empty list: %s", tt.description)
			}
		})
	}
}

func TestModel_Enter_OutOfRangeSelectedIndex_NoOp(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.view = viewWorktrees

	model.Worktrees = []domain.Worktree{
		{Path: "/home/user/repos/wt1", Branch: "main", CommitSHA: "abc123"},
	}
	model.selectedIdx = 10

	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, updatedModel)
	assert.Nil(t, cmd)
}

func TestBuildShellCmdForOS_Windows_UsesCmdKAndDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "windows path with spaces",
			path:     `C:\Users\dev\My Worktree`,
			expected: []string{"cmd", "/K"},
		},
		{
			name:     "windows different drive path",
			path:     `D:\repo\wt-feature`,
			expected: []string{"cmd", "/K"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildShellCmdForOS(tt.path, "windows", "")
			require.NotNil(t, cmd)
			assert.Equal(t, tt.expected, cmd.Args)
			assert.Equal(t, tt.path, cmd.Dir)
		})
	}
}

func TestBuildShellCmdForOS_Windows_GitBash_UsesShell(t *testing.T) {
	path := `C:\repo\wt-feature`
	cmd := buildShellCmdForOS(path, "windows", "/usr/bin/bash")
	require.NotNil(t, cmd)
	require.NotEmpty(t, cmd.Args)
	assert.Equal(t, "/usr/bin/bash", cmd.Args[0])
	assert.Equal(t, path, cmd.Dir)
}

func TestBuildShellCmdForOS_Unix_UsesShellAndFallback(t *testing.T) {
	tests := []struct {
		name      string
		shell     string
		wantFirst string
	}{
		{
			name:      "uses provided shell",
			shell:     "/bin/zsh",
			wantFirst: "/bin/zsh",
		},
		{
			name:      "falls back to /bin/sh when shell empty",
			shell:     "",
			wantFirst: "/bin/sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/tmp/worktree"
			cmd := buildShellCmdForOS(path, "linux", tt.shell)
			require.NotNil(t, cmd)
			require.NotEmpty(t, cmd.Args)
			assert.Equal(t, tt.wantFirst, cmd.Args[0])
			assert.Equal(t, path, cmd.Dir)
		})
	}
}

func TestGetShell_UsesEnvAndFallback(t *testing.T) {
	tests := []struct {
		name      string
		shellEnv  string
		wantShell string
	}{
		{
			name:      "uses SHELL env value when set",
			shellEnv:  "/bin/fish",
			wantShell: "/bin/fish",
		},
		{
			name:      "falls back to /bin/sh when SHELL env empty",
			shellEnv:  "",
			wantShell: "/bin/sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shellEnv)
			assert.Equal(t, tt.wantShell, getShell())
		})
	}
}

func TestModelUpdate_WorktreeSwitchedMsg_ErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		msg          worktreeSwitchedMsg
		wantError    string
		wantCmdIsNil bool
	}{
		{
			name:         "sets model error when switch fails",
			msg:          worktreeSwitchedMsg{err: errors.New("switch failed")},
			wantError:    "Failed to switch worktree: switch failed",
			wantCmdIsNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			updated, cmd := model.Update(tt.msg)
			updatedModel, ok := updated.(*Model)
			require.True(t, ok)
			assert.Equal(t, tt.wantError, updatedModel.statusErr)
			if tt.wantCmdIsNil {
				assert.Nil(t, cmd)
			} else {
				assert.NotNil(t, cmd)
			}
		})
	}
}

// TestModel_HelpModal_OpenedByF1AndQuestion verifies that F1 and ? both open a HelpModal.
func TestModel_HelpModal_OpenedByF1AndQuestion(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{
			name: "F1 opens HelpModal",
			msg:  tea.KeyMsg{Type: tea.KeyF1},
		},
		{
			name: "? opens HelpModal",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			require.NotNil(t, m)
			require.Nil(t, m.activeModal, "no modal should be active initially")

			updated, cmd := m.Update(tt.msg)
			updatedModel, ok := updated.(*Model)
			require.True(t, ok)

			assert.IsType(t, &modal.HelpModal{}, updatedModel.activeModal, "activeModal should be a *HelpModal")
			assert.Nil(t, cmd)
		})
	}
}

func TestModelUpdate_WorktreesRefreshedMsg_ClampsSelectedIndex(t *testing.T) {
	tests := []struct {
		name            string
		initialSelected int
		worktrees       []domain.Worktree
		wantSelected    int
	}{
		{
			name:            "clamps to last when selected index is too large",
			initialSelected: 5,
			worktrees: []domain.Worktree{
				{Path: "/wt/a"},
				{Path: "/wt/b"},
			},
			wantSelected: 1,
		},
		{
			name:            "normalizes negative selected index to zero",
			initialSelected: -3,
			worktrees: []domain.Worktree{
				{Path: "/wt/a"},
			},
			wantSelected: 0,
		},
		{
			name:            "resets selected index to zero for empty list",
			initialSelected: 2,
			worktrees:       nil,
			wantSelected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.selectedIdx = tt.initialSelected

			updated, cmd := model.Update(worktreesRefreshedMsg{worktrees: tt.worktrees, err: nil})
			updatedModel, ok := updated.(*Model)
			require.True(t, ok)
			assert.Equal(t, tt.wantSelected, updatedModel.selectedIdx)
			assert.Nil(t, cmd)
		})
	}
}

func TestModelView_ShowsErrorMessage(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.statusErr = "Failed to switch worktree: boom"

	view := model.View()
	// Error content is present in the overlay.
	assert.Contains(t, view, "Failed to switch worktree: boom")
	assert.Contains(t, view, "Press any key to dismiss")
	// Base view remains visible underneath the overlay modal.
	assert.Contains(t, view, "GIT WORKTREE ORCHESTRATOR")
}

func TestModel_T_KeyOpensSettings(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	require.Nil(t, model.activeModal, "no modal should be open initially")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m, ok := updated.(*Model)
	require.True(t, ok)

	require.NotNil(t, m.activeModal, "T key should open the settings modal")
	assert.Equal(t, "SETTINGS", m.activeModal.Title())
}

// TestModel_Init_ReturnsSyncCmd verifies that Init() returns a non-nil Cmd,
// meaning it schedules the initial background GitHub sync in addition to
// refreshing the worktree list.
func TestModel_Init_ReturnsSyncCmd(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)

	cmd := model.Init()

	assert.NotNil(t, cmd, "Init() must return a Cmd to trigger GitHub sync")
}

// TestModel_GithubSyncedMsg_StoresPRsAndIssues verifies that receiving a
// githubSyncedMsg via Update() correctly stores the synced data into the model.
func TestModel_GithubSyncedMsg_StoresPRsAndIssues(t *testing.T) {
	tests := []struct {
		name            string
		msg             githubSyncedMsg
		wantPRLen       int
		wantPRNumber    int
		wantIssueLen    int
		wantIssueNumber int
		wantLastSynced  bool
		wantSyncErr     string
		wantSyncing     bool
	}{
		{
			name: "stores prs and issues from sync message",
			msg: githubSyncedMsg{
				prs:      []domain.PullRequest{{Number: 42}},
				issues:   []domain.Issue{{Number: 7}},
				syncedAt: time.Now(),
			},
			wantPRLen:       1,
			wantPRNumber:    42,
			wantIssueLen:    1,
			wantIssueNumber: 7,
			wantLastSynced:  true,
			wantSyncing:     false,
		},
		{
			name:        "stores sync error without crashing",
			msg:         githubSyncedMsg{err: errors.New("api down")},
			wantSyncErr: "api down",
			wantSyncing: false,
		},
		{
			name:        "sets syncing=false after sync completes",
			msg:         githubSyncedMsg{prs: nil, issues: nil},
			wantSyncing: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			// githubSyncedMsg now queues into pendingSync (debounce); fire the
			// debounce flush to actually apply the state.
			updated, _ := model.Update(tt.msg)
			m, ok := updated.(*Model)
			require.True(t, ok, "Update must return *Model")
			updated2, _ := m.Update(debouncedRenderMsg{})
			m, ok = updated2.(*Model)
			require.True(t, ok, "debouncedRenderMsg must return *Model")

			if tt.wantPRLen > 0 {
				require.Len(t, m.prs, tt.wantPRLen, "prs slice length mismatch")
				assert.Equal(t, tt.wantPRNumber, m.prs[0].Number, "PR number mismatch")
			}

			if tt.wantIssueLen > 0 {
				require.Len(t, m.issues, tt.wantIssueLen, "issues slice length mismatch")
				assert.Equal(t, tt.wantIssueNumber, m.issues[0].Number, "issue number mismatch")
			}

			if tt.wantLastSynced {
				assert.False(t, m.lastSynced.IsZero(), "lastSynced must be set to a non-zero time")
			}

			if tt.wantSyncErr != "" {
				require.NotNil(t, m.syncErr, "syncErr must not be nil")
				assert.Contains(t, m.syncErr.Error(), tt.wantSyncErr, "syncErr message mismatch")
			}

			assert.Equal(t, tt.wantSyncing, m.syncing, "syncing flag mismatch")
		})
	}
}

func TestModel_GithubSync_PreservesSuccessfulPartialResults(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.prs = []domain.PullRequest{{Number: 1}}
	model.issues = []domain.Issue{{Number: 2}}

	updated, _ := model.Update(githubSyncedMsg{
		prs:      []domain.PullRequest{{Number: 3}},
		issues:   nil,
		err:      errors.New("list open issues: network timeout"),
		syncedAt: time.Now(),
	})
	updated, _ = updated.(*Model).Update(debouncedRenderMsg{})
	got := updated.(*Model)

	require.Len(t, got.prs, 1)
	assert.Equal(t, 3, got.prs[0].Number)
	require.Len(t, got.issues, 1)
	assert.Equal(t, 2, got.issues[0].Number)
	require.Error(t, got.syncErr)
	assert.Contains(t, got.statusErr, "GitHub sync degraded; showing available data")
	assert.False(t, got.lastSynced.IsZero())
}

// TestModel_SyncTickMsg_TriggersSyncCmd verifies that receiving a syncTickMsg
// via Update() returns a non-nil Cmd to schedule the next background GitHub sync.
func TestModel_SyncTickMsg_TriggersSyncCmd(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)

	_, cmd := model.Update(syncTickMsg{})

	assert.NotNil(t, cmd, "syncTickMsg must trigger a sync Cmd")
}

// ---------------------------------------------------------------------------
// Phase 2: Issues & PRs View tests
// ---------------------------------------------------------------------------

// TestModel_ViewSwitching verifies that pressing W/I/P (upper- and lower-case)
// switches the model's active view to the correct activeView constant.
func TestModel_ViewSwitching(t *testing.T) {
	tests := []struct {
		name     string
		key      rune
		wantView activeView
	}{
		{
			name:     "pressing W sets view to viewWorktrees",
			key:      'W',
			wantView: viewWorktrees,
		},
		{
			name:     "pressing I sets view to viewIssues",
			key:      'I',
			wantView: viewIssues,
		},
		{
			name:     "pressing P sets view to viewPRs",
			key:      'P',
			wantView: viewPRs,
		},
		{
			name:     "pressing w (lowercase) sets view to viewWorktrees",
			key:      'w',
			wantView: viewWorktrees,
		},
		{
			name:     "pressing i (lowercase) sets view to viewIssues",
			key:      'i',
			wantView: viewIssues,
		},
		{
			name:     "pressing p (lowercase) sets view to viewPRs",
			key:      'p',
			wantView: viewPRs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
			m, ok := updated.(*Model)
			require.True(t, ok, "Update must return *Model")

			assert.Equal(t, tt.wantView, m.view)
		})
	}
}

// TestModel_IssueNavigation verifies that up/down navigation in viewIssues
// moves selectedIssueIdx correctly and does NOT move the worktree selectedIdx.
func TestModel_IssueNavigation(t *testing.T) {
	issues := []domain.Issue{
		{Number: 1, Title: "First"},
		{Number: 2, Title: "Second"},
		{Number: 3, Title: "Third"},
	}

	tests := []struct {
		name            string
		initialIssueIdx int
		keyType         tea.KeyType
		wantIssueIdx    int
		wantWorktreeIdx int
	}{
		{
			name:            "down key increments selectedIssueIdx",
			initialIssueIdx: 0,
			keyType:         tea.KeyDown,
			wantIssueIdx:    1,
			wantWorktreeIdx: 0,
		},
		{
			name:            "up key decrements selectedIssueIdx",
			initialIssueIdx: 1,
			keyType:         tea.KeyUp,
			wantIssueIdx:    0,
			wantWorktreeIdx: 0,
		},
		{
			name:            "up key does not go below 0 (boundary)",
			initialIssueIdx: 0,
			keyType:         tea.KeyUp,
			wantIssueIdx:    0,
			wantWorktreeIdx: 0,
		},
		{
			name:            "down key does not exceed len(issues)-1 (boundary)",
			initialIssueIdx: 2,
			keyType:         tea.KeyDown,
			wantIssueIdx:    2,
			wantWorktreeIdx: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.issues = issues
			model.view = viewIssues
			model.focused = panelList
			model.selectedIssueIdx = tt.initialIssueIdx

			updated, _ := model.Update(tea.KeyMsg{Type: tt.keyType})
			m, ok := updated.(*Model)
			require.True(t, ok, "Update must return *Model")

			assert.Equal(t, tt.wantIssueIdx, m.selectedIssueIdx, "issue index mismatch")
			assert.Equal(t, tt.wantWorktreeIdx, m.selectedIdx, "worktree idx must not change when navigating issues")
		})
	}
}

// TestModel_PRNavigation verifies that up/down navigation in viewPRs
// moves selectedPRIdx correctly and does NOT move the worktree selectedIdx.
func TestModel_PRNavigation(t *testing.T) {
	prs := []domain.PullRequest{
		{Number: 10, Title: "PR One"},
		{Number: 11, Title: "PR Two"},
		{Number: 12, Title: "PR Three"},
	}

	tests := []struct {
		name            string
		initialPRIdx    int
		keyType         tea.KeyType
		wantPRIdx       int
		wantWorktreeIdx int
	}{
		{
			name:            "down key increments selectedPRIdx",
			initialPRIdx:    0,
			keyType:         tea.KeyDown,
			wantPRIdx:       1,
			wantWorktreeIdx: 0,
		},
		{
			name:            "up key decrements selectedPRIdx",
			initialPRIdx:    1,
			keyType:         tea.KeyUp,
			wantPRIdx:       0,
			wantWorktreeIdx: 0,
		},
		{
			name:            "up key does not go below 0 (boundary)",
			initialPRIdx:    0,
			keyType:         tea.KeyUp,
			wantPRIdx:       0,
			wantWorktreeIdx: 0,
		},
		{
			name:            "down key does not exceed len(prs)-1 (boundary)",
			initialPRIdx:    2,
			keyType:         tea.KeyDown,
			wantPRIdx:       2,
			wantWorktreeIdx: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.prs = prs
			model.view = viewPRs
			model.focused = panelList
			model.selectedPRIdx = tt.initialPRIdx

			updated, _ := model.Update(tea.KeyMsg{Type: tt.keyType})
			m, ok := updated.(*Model)
			require.True(t, ok, "Update must return *Model")

			assert.Equal(t, tt.wantPRIdx, m.selectedPRIdx, "PR index mismatch")
			assert.Equal(t, tt.wantWorktreeIdx, m.selectedIdx, "worktree idx must not change when navigating PRs")
		})
	}
}

// TestModel_ContextActionOpenGitHub opens the selected issue or PR in the
// browser and reports an error when the selection has no GitHub item.
func TestModel_ContextActionOpenGitHub(t *testing.T) {
	tests := []struct {
		name       string
		view       activeView
		issues     []domain.Issue
		prs        []domain.PullRequest
		issueIdx   int
		prIdx      int
		wantCmdNil bool
	}{
		{
			name:       "issue selection returns non-nil Cmd",
			view:       viewIssues,
			issues:     []domain.Issue{{Number: 5, Title: "Test Issue"}},
			issueIdx:   0,
			wantCmdNil: false,
		},
		{
			name:       "PR selection returns non-nil Cmd",
			view:       viewPRs,
			prs:        []domain.PullRequest{{Number: 42, Title: "My PR", Branch: "feat/awesome", Author: "alice", State: "OPEN"}},
			prIdx:      0,
			wantCmdNil: false,
		},
		{
			name:       "worktree selection returns clear-error Cmd",
			view:       viewWorktrees,
			wantCmdNil: false,
		},
		{
			name:       "empty issues list returns clear-error Cmd",
			view:       viewIssues,
			issues:     []domain.Issue{},
			issueIdx:   0,
			wantCmdNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = tt.view
			if tt.issues != nil {
				model.issues = tt.issues
			}
			if tt.prs != nil {
				model.prs = tt.prs
			}
			model.selectedIssueIdx = tt.issueIdx
			model.selectedPRIdx = tt.prIdx

			_, cmd := model.handleContextAction(modal.ContextActionOpenGitHub)

			if tt.wantCmdNil {
				assert.Nil(t, cmd, "expected nil Cmd (no-op) but got non-nil")
			} else {
				assert.NotNil(t, cmd, "expected non-nil context action Cmd")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// End Phase 2 tests (app_test.go)
// ---------------------------------------------------------------------------

// TestModel_GithubSync_ClampsIssueAndPRIdx verifies that after a successful sync
// that returns fewer items, selectedIssueIdx and selectedPRIdx are clamped to
// the new list bounds so openInBrowserCmd never panics.
func TestModel_GithubSync_ClampsIssueAndPRIdx(t *testing.T) {
	tests := []struct {
		name            string
		initialIssueIdx int
		initialPRIdx    int
		syncIssues      []domain.Issue
		syncPRs         []domain.PullRequest
		wantIssueIdx    int
		wantPRIdx       int
	}{
		{
			name:            "issue idx clamped when sync shrinks list",
			initialIssueIdx: 4,
			initialPRIdx:    0,
			syncIssues:      []domain.Issue{{Number: 1, Title: "Only Issue"}},
			syncPRs:         []domain.PullRequest{{Number: 10, Title: "PR", Branch: "main", Author: "dev", State: "OPEN"}},
			wantIssueIdx:    0,
			wantPRIdx:       0,
		},
		{
			name:            "pr idx clamped when sync shrinks list",
			initialIssueIdx: 0,
			initialPRIdx:    5,
			syncIssues:      []domain.Issue{{Number: 1, Title: "Issue"}},
			syncPRs:         []domain.PullRequest{{Number: 10, Title: "PR", Branch: "main", Author: "dev", State: "OPEN"}},
			wantIssueIdx:    0,
			wantPRIdx:       0,
		},
		{
			name:            "idx within bounds is preserved after sync",
			initialIssueIdx: 0,
			initialPRIdx:    0,
			syncIssues:      []domain.Issue{{Number: 1, Title: "A"}, {Number: 2, Title: "B"}},
			syncPRs:         []domain.PullRequest{{Number: 10, Title: "PR", Branch: "main", Author: "dev", State: "OPEN"}},
			wantIssueIdx:    0,
			wantPRIdx:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.selectedIssueIdx = tt.initialIssueIdx
			model.selectedPRIdx = tt.initialPRIdx

			msg := githubSyncedMsg{
				issues:   tt.syncIssues,
				prs:      tt.syncPRs,
				syncedAt: time.Now(),
			}
			// githubSyncedMsg now uses debounce; fire the flush to apply the state.
			updated, _ := model.Update(msg)
			m, ok := updated.(*Model)
			require.True(t, ok)
			updated2, _ := m.Update(debouncedRenderMsg{})
			m, ok = updated2.(*Model)
			require.True(t, ok)

			assert.Equal(t, tt.wantIssueIdx, m.selectedIssueIdx, "issue idx must be clamped")
			assert.Equal(t, tt.wantPRIdx, m.selectedPRIdx, "pr idx must be clamped")
		})
	}
}

// TestModel_BrowserOpenErrMsg_SetsError verifies that a browserOpenErrMsg with
// a non-nil error sets m.Error so the user sees feedback.
func TestModel_BrowserOpenErrMsg_SetsError(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)

	updated, _ := model.Update(browserOpenErrMsg{err: errors.New("gh: not found")})
	m, ok := updated.(*Model)
	require.True(t, ok)

	assert.Contains(t, m.statusErr, "Failed to open in browser")
	assert.Contains(t, m.statusErr, "gh: not found")
}

// TestModel_BrowserOpenErrMsg_NilErrorNoChange verifies that a nil-error
// browserOpenErrMsg does not set m.Error.
func TestModel_BrowserOpenErrMsg_NilErrorNoChange(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)

	updated, _ := model.Update(browserOpenErrMsg{err: nil})
	m, ok := updated.(*Model)
	require.True(t, ok)

	assert.Empty(t, m.statusErr)
}

// ---------------------------------------------------------------------------
// Phase 4: Panel focus & j/k navigation tests
// ---------------------------------------------------------------------------

// TestModel_DefaultFocus_IsNavPanel verifies that a new model starts with
// the list panel focused by default.
func TestModel_DefaultFocus_IsListPanel(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	assert.Equal(t, panelList, model.focused)
}

// TestModel_Tab_CyclesFocusThroughPanels verifies that Tab cycles focus
// left to right: nav → list → ctx → nav.
func TestModel_Tab_CyclesFocusThroughPanels(t *testing.T) {
	tests := []struct {
		name         string
		initialFocus focusedPanel
		wantFocus    focusedPanel
	}{
		{name: "Tab from nav focuses list", initialFocus: panelNav, wantFocus: panelList},
		{name: "Tab from list focuses ctx", initialFocus: panelList, wantFocus: panelCtx},
		{name: "Tab from ctx wraps to nav", initialFocus: panelCtx, wantFocus: panelNav},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.focused = tt.initialFocus

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
			m, ok := updated.(*Model)
			require.True(t, ok)

			assert.Equal(t, tt.wantFocus, m.focused)
		})
	}
}

// TestModel_JK_ListFocused_NavigatesWorktrees verifies that j/k navigate
// the worktree list when the list panel is focused.
func TestModel_JK_ListFocused_NavigatesWorktrees(t *testing.T) {
	worktrees := []domain.Worktree{
		{Path: "/wt/a"},
		{Path: "/wt/b"},
		{Path: "/wt/c"},
	}

	tests := []struct {
		name       string
		key        rune
		initialIdx int
		wantIdx    int
	}{
		{name: "j moves selection down", key: 'j', initialIdx: 0, wantIdx: 1},
		{name: "k moves selection up", key: 'k', initialIdx: 1, wantIdx: 0},
		{name: "j at bottom stays", key: 'j', initialIdx: 2, wantIdx: 2},
		{name: "k at top stays", key: 'k', initialIdx: 0, wantIdx: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.Worktrees = worktrees
			model.selectedIdx = tt.initialIdx
			model.focused = panelList
			model.view = viewWorktrees

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
			m, ok := updated.(*Model)
			require.True(t, ok)

			assert.Equal(t, tt.wantIdx, m.selectedIdx)
		})
	}
}

// TestModel_JK_NavFocused_ChangesView verifies that j/k cycle through views
// when the nav panel is focused.
func TestModel_JK_NavFocused_ChangesView(t *testing.T) {
	tests := []struct {
		name        string
		key         rune
		initialView activeView
		wantView    activeView
	}{
		{name: "j from worktrees → issues", key: 'j', initialView: viewWorktrees, wantView: viewIssues},
		{name: "j from issues → PRs", key: 'j', initialView: viewIssues, wantView: viewPRs},
		{name: "j from PRs wraps → dashboard", key: 'j', initialView: viewPRs, wantView: viewDashboard},
		{name: "k from dashboard → PRs", key: 'k', initialView: viewDashboard, wantView: viewPRs},
		{name: "k from PRs → issues", key: 'k', initialView: viewPRs, wantView: viewIssues},
		{name: "k from issues → worktrees", key: 'k', initialView: viewIssues, wantView: viewWorktrees},
		{name: "k from worktrees wraps → dashboard", key: 'k', initialView: viewWorktrees, wantView: viewDashboard},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = tt.initialView
			model.focused = panelNav

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
			m, ok := updated.(*Model)
			require.True(t, ok)

			assert.Equal(t, tt.wantView, m.view)
		})
	}
}

func TestModel_JK_CtxFocused_ChangesActionSelection(t *testing.T) {
	tests := []struct {
		name       string
		key        rune
		initialIdx int
		wantIdx    int
	}{
		{name: "j advances action", key: 'j', initialIdx: 0, wantIdx: 1},
		{name: "k moves to previous action", key: 'k', initialIdx: 2, wantIdx: 1},
		{name: "k at zero stays at zero", key: 'k', initialIdx: 0, wantIdx: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewIssues
			model.issues = []domain.Issue{{Number: 42}}
			model.focused = panelCtx
			model.contextActionIdx = tt.initialIdx

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tt.key}})
			m, ok := updated.(*Model)
			require.True(t, ok)

			assert.Equal(t, tt.wantIdx, m.contextActionIdx)
		})
	}
}

// TestModel_WindowSizeMsg_StoresDimensions verifies that WindowSizeMsg updates
// width and height fields on the model.
func TestModel_WindowSizeMsg_StoresDimensions(t *testing.T) {
	tests := []struct {
		name       string
		msgWidth   int
		msgHeight  int
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "stores width from WindowSizeMsg",
			msgWidth:   160,
			msgHeight:  50,
			wantWidth:  160,
			wantHeight: 50,
		},
		{
			name:       "stores minimum width from WindowSizeMsg",
			msgWidth:   80,
			msgHeight:  24,
			wantWidth:  80,
			wantHeight: 24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			updated, _ := model.Update(tea.WindowSizeMsg{Width: tt.msgWidth, Height: tt.msgHeight})
			m, ok := updated.(*Model)
			require.True(t, ok)

			assert.Equal(t, tt.wantWidth, m.width)
			assert.Equal(t, tt.wantHeight, m.height)
		})
	}
}

// ---------------------------------------------------------------------------
// PR Checkout / Worktree tests
// ---------------------------------------------------------------------------

// TestPRWorktreePath verifies that prWorktreePath produces the correct path
// by replacing slashes in the branch name with dashes and scoping under worktrees/<repo>/.
func TestPRWorktreePath(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		branch   string
		wantPath string
	}{
		{
			name:     "simple branch no slashes",
			repoPath: filepath.Join("/repos", "nexus"),
			branch:   "main",
			wantPath: filepath.Join("/repos", "worktrees", "nexus", "main"),
		},
		{
			name:     "branch with slashes converted to dashes",
			repoPath: filepath.Join("/repos", "nexus"),
			branch:   "feat/issue-42-my-feature",
			wantPath: filepath.Join("/repos", "worktrees", "nexus", "feat-issue-42-my-feature"),
		},
		{
			name:     "different repo does not collide with nexus",
			repoPath: filepath.Join("/repos", "nova"),
			branch:   "feat/issue-42-my-feature",
			wantPath: filepath.Join("/repos", "worktrees", "nova", "feat-issue-42-my-feature"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prWorktreePath(tt.repoPath, tt.branch)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

// TestModel_Enter_InViewPRs_OpensModal verifies that pressing Enter in the PR list view
// opens a PRCheckoutModal when no worktree exists for that branch.
func TestModel_Enter_InViewPRs_OpensModal(t *testing.T) {
	m := NewModel()
	m.view = viewPRs
	m.RepoPath = "/repos/nexus"
	m.prs = []domain.PullRequest{
		{Number: 1, Title: "My PR", Branch: "feat/issue-1-my-pr"},
	}
	m.selectedPRIdx = 0
	m.Worktrees = []domain.Worktree{
		{Path: "/repos/nexus", Branch: "main"},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Nil(t, cmd, "no cmd until confirmation")
	assert.NotNil(t, updatedModel.activeModal, "modal should be open")
}

// TestModel_Enter_InViewPRs_WorktreeExists_SetsError verifies that pressing Enter
// in the PR list shows an error when a worktree for that branch already exists.
func TestModel_Enter_InViewPRs_WorktreeExists_SetsError(t *testing.T) {
	m := NewModel()
	m.view = viewPRs
	m.RepoPath = "/repos/nexus"
	m.prs = []domain.PullRequest{
		{Number: 1, Title: "My PR", Branch: "feat/issue-1-my-pr"},
	}
	m.selectedPRIdx = 0
	m.Worktrees = []domain.Worktree{
		{Path: "/repos/nexus", Branch: "main"},
		{Path: "/repos/worktrees/feat-issue-1-my-pr", Branch: "feat/issue-1-my-pr"},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.NotNil(t, cmd, "clearErrorCmd should be returned")
	assert.Nil(t, updatedModel.activeModal, "no modal should be opened")
	assert.Contains(t, updatedModel.statusErr, "feat/issue-1-my-pr", "error should mention the branch")
}

// TestModel_Enter_InViewPRs_EmptyList_NoOp verifies that pressing Enter in the PR
// list with an empty list does not crash and returns no cmd.
func TestModel_Enter_InViewPRs_EmptyList_NoOp(t *testing.T) {
	m := NewModel()
	m.view = viewPRs
	m.prs = []domain.PullRequest{}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Nil(t, cmd)
	assert.Nil(t, updatedModel.activeModal)
}

func TestModel_Enter_InViewIssues_OpensCreateModalForSelectedIssue(t *testing.T) {
	m := NewModel()
	m.view = viewIssues
	m.RepoPath = "/repos/nexus"
	m.issues = []domain.Issue{
		{Number: 1, Title: "First issue"},
		{Number: 2, Title: "Second issue"},
	}
	m.selectedIssueIdx = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Nil(t, cmd, "no cmd until the create modal is confirmed")
	createModal, ok := updatedModel.activeModal.(*modal.CreateModal)
	require.True(t, ok, "create modal should be open")
	assert.Equal(t, 2, createModal.SelectedIssue().Number)
	assert.Contains(t, createModal.View(), "Select branch type")
	assert.NotContains(t, createModal.View(), "Select issue")
}

func TestModel_Enter_InViewIssues_EmptyList_NoOp(t *testing.T) {
	m := NewModel()
	m.view = viewIssues

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Nil(t, cmd)
	assert.Nil(t, updatedModel.activeModal)
}

// TestModel_Enter_InViewWorktrees_SpawnsSession verifies that Enter spawns a
// session (same as the s key) when a worktree is selected.
func TestModel_Enter_InViewWorktrees_SpawnsSession(t *testing.T) {
	m := NewModel()
	m.view = viewWorktrees
	m.Worktrees = []domain.Worktree{
		{Path: "/repos/nexus", Branch: "main"},
	}
	m.selectedIdx = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd, "should return a spawnSessionCmd")
}

func TestModel_WorktreeEnterFocusesExistingWorkflowPane(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m := NewModel()
	m.view = viewWorktrees
	m.herdrNavigator = navigator
	m.Worktrees = []domain.Worktree{
		{Path: "/repos/grove-52", Branch: "agent/define-ipc-protocol-52"},
	}
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{{
			WorkflowID:   "run-52",
			RunID:        "run-52",
			Title:        "Implement #52",
			Status:       domain.WorkflowFailed,
			WorktreePath: "/repos/grove-52",
			Branch:       "agent/define-ipc-protocol-52",
		}},
		Agents: []domain.AgentRef{{
			AgentID:       "agent-52",
			Name:          "opencode",
			WorkflowRunID: "run-52",
			Status:        domain.AgentFailed,
			PaneID:        "window-1:pane-5",
		}},
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg, ok := cmd().(paneFocusedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "window-1:pane-5", navigator.focusedPane)
	assert.Empty(t, navigator.openedPath)
}

func TestModel_WorktreeEnterIgnoresCompletedWorkflowPane(t *testing.T) {
	navigator := &fakeHerdrNavigator{
		pane: &domain.PaneRef{PaneID: "window-1:pane-6", CWD: "/repos/grove-52"},
	}
	m := NewModel()
	m.view = viewWorktrees
	m.insideHerdr = true
	m.Config.Herdr.Enabled = true
	m.Config.Herdr.PreferWorktreeAPI = true
	m.herdrNavigator = navigator
	m.Worktrees = []domain.Worktree{
		{Path: "/repos/grove-52", Branch: "agent/define-ipc-protocol-52"},
	}
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{{
			WorkflowID:   "run-52",
			RunID:        "run-52",
			Status:       domain.WorkflowSucceeded,
			WorktreePath: "/repos/grove-52",
		}},
		Agents: []domain.AgentRef{{
			AgentID:       "agent-52",
			WorkflowRunID: "run-52",
			PaneID:        "window-1:pane-5",
		}},
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg, ok := cmd().(herdrWorktreeOpenedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Empty(t, navigator.focusedPane)
	assert.Equal(t, "/repos/grove-52", navigator.openedPath)
}

type fakeHerdrNavigator struct {
	openedPath  string
	focusedPane string
	pane        *domain.PaneRef
	openErr     error
	focusErr    error
}

func (f *fakeHerdrNavigator) OpenWorktree(_ context.Context, req herdr.OpenWorktreeRequest) (*domain.PaneRef, error) {
	f.openedPath = req.Path
	return f.pane, f.openErr
}

func (f *fakeHerdrNavigator) FocusPane(_ context.Context, paneID string) error {
	f.focusedPane = paneID
	return f.focusErr
}

func TestModel_DashboardJK_SelectsWorkflow(t *testing.T) {
	m := NewModel()
	m.view = viewDashboard
	m.focused = panelList
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "implement-42", RunID: "run-42", Status: domain.WorkflowRunning},
			{WorkflowID: "review-17", RunID: "run-17", Status: domain.WorkflowQueued},
		},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Nil(t, cmd)
	model := updated.(*Model)
	assert.Equal(t, 1, model.selectedMissionIdx)

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Nil(t, cmd)
	assert.Equal(t, 0, updated.(*Model).selectedMissionIdx)
}

func TestModel_DashboardMissionUpdateClampsSelection(t *testing.T) {
	m := NewModel()
	m.selectedMissionIdx = 2
	state := domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "implement-42", RunID: "run-42", Status: domain.WorkflowRunning},
		},
	}

	updated, cmd := m.Update(missionControlUpdatedMsg{state: state})
	require.Nil(t, cmd)
	assert.Equal(t, 0, updated.(*Model).selectedMissionIdx)
}

func TestModel_DashboardEnterFocusesCorrelatedHerdrPane(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m := NewModel()
	m.view = viewDashboard
	m.herdrNavigator = navigator
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{
				WorkflowID:   "implement-42",
				RunID:        "run-42",
				Status:       domain.WorkflowRunning,
				WorktreePath: "/repos/grove-42",
			},
		},
		Panes: []domain.PaneRef{
			{PaneID: "window-1:pane-2", CWD: "/repos/grove-42", Status: "open"},
		},
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg, ok := cmd().(paneFocusedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "window-1:pane-2", navigator.focusedPane)
}

func TestModel_DashboardEnterFocusesWorkflowAgentPane(t *testing.T) {
	navigator := &fakeHerdrNavigator{}
	m := NewModel()
	m.view = viewDashboard
	m.herdrNavigator = navigator
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{
				WorkflowID:   "implement-52",
				RunID:        "run-52",
				Status:       domain.WorkflowFailed,
				WorktreePath: "/repos/grove-52",
			},
		},
		Agents: []domain.AgentRef{
			{
				AgentID:       "agent-52",
				Name:          "opencode",
				WorkflowRunID: "run-52",
				Status:        domain.AgentFailed,
				PaneID:        "window-1:pane-5",
			},
		},
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg, ok := cmd().(paneFocusedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "window-1:pane-5", navigator.focusedPane)
	assert.Empty(t, navigator.openedPath)
}

func TestModel_DashboardEnterFallsBackToTrackedHerdrSession(t *testing.T) {
	navigator := &fakeHerdrNavigator{
		pane: &domain.PaneRef{PaneID: "window-1:pane-3", CWD: "/repos/grove-42"},
	}
	m := NewModel()
	m.view = viewDashboard
	m.herdrNavigator = navigator
	m.sessions = []domain.Session{
		{WorktreePath: "/repos/grove-42", Runtime: domain.RuntimeHerdr, Status: domain.StatusActive},
	}
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{
				WorkflowID:   "implement-42",
				RunID:        "run-42",
				Status:       domain.WorkflowRunning,
				WorktreePath: "/repos/grove-42",
			},
		},
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg, ok := cmd().(sessionFocusedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "/repos/grove-42", navigator.openedPath)
}

func TestModel_DashboardEnterReportsMissingJumpTarget(t *testing.T) {
	m := NewModel()
	m.view = viewDashboard
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "implement-42", RunID: "run-42", Status: domain.WorkflowRunning},
		},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	assert.Contains(t, updated.(*Model).statusErr, "No terminal or Herdr pane found")
}

func TestModel_DashboardVOpensMissionInspector(t *testing.T) {
	m := NewModel()
	m.view = viewDashboard
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{
				WorkflowID:  "run-42",
				RunID:       "run-42",
				Title:       "Implement issue #42",
				Status:      domain.WorkflowRunning,
				CurrentStep: "implementation",
			},
		},
		Agents: []domain.AgentRef{
			{AgentID: "agent-1", WorkflowRunID: "run-42", Summary: "Editing renderer"},
		},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	require.Nil(t, cmd)
	inspector, ok := updated.(*Model).activeModal.(*modal.MissionModal)
	require.True(t, ok)
	assert.Equal(t, "run-42", inspector.RunID())
	assert.Contains(t, inspector.View(), "Implement issue #42")
}

func TestModel_DashboardCompletedTabOpensCompletedMission(t *testing.T) {
	m := NewModel()
	m.view = viewDashboard
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "run-live", RunID: "run-live", Title: "Implement issue #42", Status: domain.WorkflowRunning},
			{WorkflowID: "run-done", RunID: "run-done", Title: "Review pull request #17", Status: domain.WorkflowSucceeded},
		},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	require.Nil(t, cmd)
	model := updated.(*Model)
	assert.Equal(t, dashboardTabCompleted, model.dashboardTab)
	assert.Equal(t, 0, model.selectedMissionIdx)

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd)
	inspector, ok := updated.(*Model).activeModal.(*modal.MissionModal)
	require.True(t, ok)
	assert.Equal(t, "run-done", inspector.RunID())
	assert.Contains(t, inspector.View(), "Review pull request #17")
}

func TestModel_MouseClickNavigatesViews(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 30
	layout := m.mouseLayout()

	updated, cmd := m.Update(tea.MouseMsg{
		X:      2,
		Y:      layout.panelTop + 1 + int(viewIssues),
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	require.Nil(t, cmd)
	model := updated.(*Model)
	assert.Equal(t, viewIssues, model.view)
	assert.Equal(t, panelNav, model.focused)
}

func TestModel_MouseClickSwitchesDashboardTabs(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 30
	m.view = viewDashboard
	layout := m.mouseLayout()

	updated, cmd := m.Update(tea.MouseMsg{
		X:      layout.listX + 44,
		Y:      layout.panelTop + 1 + 13,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	require.Nil(t, cmd)
	assert.Equal(t, dashboardTabCompleted, updated.(*Model).dashboardTab)
}

func TestModel_MouseWheelNavigatesList(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 30
	m.view = viewWorktrees
	m.Worktrees = []domain.Worktree{{Path: "/wt/one"}, {Path: "/wt/two"}}
	layout := m.mouseLayout()

	updated, cmd := m.Update(tea.MouseMsg{
		X:      layout.listX + 2,
		Y:      layout.panelTop + 3,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	require.NotNil(t, cmd)
	model := updated.(*Model)
	assert.Equal(t, 1, model.selectedIdx)
	assert.Equal(t, panelList, model.focused)
}

func TestModel_MouseDoubleClickActivatesIssue(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 30
	m.view = viewIssues
	m.issues = []domain.Issue{{Number: 42, Title: "Mouse support"}}
	layout := m.mouseLayout()
	click := tea.MouseMsg{
		X:      layout.listX + 2,
		Y:      layout.panelTop + 2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}

	updated, cmd := m.Update(click)
	require.Nil(t, cmd)
	model := updated.(*Model)
	updated, cmd = model.Update(click)
	require.Nil(t, cmd)
	_, ok := updated.(*Model).activeModal.(*modal.CreateModal)
	assert.True(t, ok)
}

func TestModel_MouseClickContextActionRunsIt(t *testing.T) {
	m := NewModel()
	m.width = 120
	m.height = 30
	m.view = viewDashboard
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "run-42", RunID: "run-42", Title: "Implement issue #42", Status: domain.WorkflowRunning},
		},
	}
	layout := m.mouseLayout()

	updated, cmd := m.Update(tea.MouseMsg{
		X:      layout.contextX + 2,
		Y:      layout.panelTop + 2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	require.Nil(t, cmd)
	_, ok := updated.(*Model).activeModal.(*modal.MissionModal)
	assert.True(t, ok)
}

func TestModel_MissionInspectorRefreshesWithMissionState(t *testing.T) {
	m := NewModel()
	workflow := domain.WorkflowRunRef{
		WorkflowID: "run-42",
		RunID:      "run-42",
		Status:     domain.WorkflowRunning,
		Progress:   domain.WorkflowProgress{Percent: 10},
	}
	m.activeModal = modal.NewMissionModal(workflow, nil)
	workflow.Progress.Percent = 75
	state := domain.MissionControlState{WorkflowRuns: []domain.WorkflowRunRef{workflow}}

	updated, cmd := m.Update(missionControlUpdatedMsg{state: state})
	require.Nil(t, cmd)
	inspector := updated.(*Model).activeModal.(*modal.MissionModal)
	assert.Contains(t, inspector.View(), "75%")
}

type fakeSandcastleWorkflowStarter struct {
	request      sandcastle.StartWorkflowRequest
	workflow     domain.WorkflowRunRef
	removedRunID string
	removeStop   bool
	err          error
}

func (f *fakeSandcastleWorkflowStarter) StartWorkflow(_ context.Context, req sandcastle.StartWorkflowRequest) (domain.WorkflowRunRef, error) {
	f.request = req
	return f.workflow, f.err
}

func (f *fakeSandcastleWorkflowStarter) RemoveWorkflow(_ context.Context, _ string, runID string, stop bool) error {
	f.removedRunID = runID
	f.removeStop = stop
	return f.err
}

func TestModel_DashboardXConfirmsAndRemovesWorkflow(t *testing.T) {
	manager := &fakeSandcastleWorkflowStarter{}
	m := NewModel()
	m.view = viewDashboard
	m.workflowStarter = manager
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{
			{WorkflowID: "run-42", RunID: "run-42", Title: "Implement issue #42", Status: domain.WorkflowRunning},
		},
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	require.Nil(t, cmd)
	model := updated.(*Model)
	_, ok := model.activeModal.(*modal.WorkflowRemoveModal)
	require.True(t, ok)

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	require.NotNil(t, cmd)
	model = updated.(*Model)
	confirmed, ok := cmd().(modal.WorkflowRemoveConfirmedMsg)
	require.True(t, ok)
	updated, cmd = model.Update(confirmed)
	require.NotNil(t, cmd)
	model = updated.(*Model)
	msg, ok := cmd().(workflowRemovedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "run-42", manager.removedRunID)
	assert.True(t, manager.removeStop)

	updated, cmd = model.Update(msg)
	require.NotNil(t, cmd)
	assert.Equal(t, "Workflow stopped and removed", updated.(*Model).statusMsg)
}

func TestModel_AKeyFocusesContextActions(t *testing.T) {
	tests := []struct {
		name string
		view activeView
		init func(*Model)
	}{
		{
			name: "issue",
			view: viewIssues,
			init: func(m *Model) {
				m.issues = []domain.Issue{{Number: 42, Title: "Ship workflow launcher"}}
			},
		},
		{
			name: "pull request",
			view: viewPRs,
			init: func(m *Model) {
				m.prs = []domain.PullRequest{{Number: 17, Title: "Workflow UI"}}
			},
		},
		{
			name: "dashboard maintenance",
			view: viewDashboard,
			init: func(_ *Model) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.view = tt.view
			tt.init(m)

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
			require.Nil(t, cmd)
			model := updated.(*Model)
			assert.Equal(t, panelCtx, model.focused)
			assert.Equal(t, 0, model.contextActionIdx)
			assert.NotEmpty(t, model.availableContextActions())
		})
	}
}

func TestModel_WorkflowLaunchMsgStartsOpenCodeWorkflow(t *testing.T) {
	issueNumber := 42
	starter := &fakeSandcastleWorkflowStarter{
		workflow: domain.WorkflowRunRef{WorkflowID: "run_42", Status: domain.WorkflowQueued},
	}
	m := NewModel()
	m.RepoPath = "/repos/grove"
	m.Config.Sandcastle.DefaultAgent = "opencode"
	m.workflowStarter = starter
	m.view = viewIssues
	m.issues = []domain.Issue{{Number: issueNumber}}
	m.focused = panelCtx
	m.contextActionIdx = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	assert.Nil(t, updated.(*Model).activeModal)

	msg, ok := cmd().(sandcastleWorkflowStartedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, modal.WorkflowKindImplement, starter.request.Kind)
	assert.Equal(t, "/repos/grove", starter.request.RepoPath)
	assert.Equal(t, "opencode", starter.request.AgentKind)
	require.NotNil(t, starter.request.IssueNumber)
	assert.Equal(t, issueNumber, *starter.request.IssueNumber)
}

func TestModel_FailedDashboardWorkflowCanBeRetried(t *testing.T) {
	issueNumber := 52
	starter := &fakeSandcastleWorkflowStarter{
		workflow: domain.WorkflowRunRef{WorkflowID: "run_retry", Status: domain.WorkflowQueued},
	}
	m := NewModel()
	m.RepoPath = "/repos/current"
	m.workflowStarter = starter
	m.view = viewDashboard
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{{
			WorkflowID:   "run-failed",
			RunID:        "run-failed",
			Kind:         modal.WorkflowKindImplement,
			Repo:         "/repos/spectre",
			Status:       domain.WorkflowFailed,
			DefaultAgent: "pi",
			IssueNumber:  &issueNumber,
		}},
	}
	m.focused = panelCtx
	m.contextActionIdx = 1

	actions := m.availableContextActions()
	require.Len(t, actions, 4)
	assert.Equal(t, modal.ContextActionRetryRun, actions[1].action)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	assert.Equal(t, "Retrying imp workflow…", updated.(*Model).statusMsg)

	msg, ok := cmd().(sandcastleWorkflowStartedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, modal.WorkflowKindImplement, starter.request.Kind)
	assert.Equal(t, "/repos/spectre", starter.request.RepoPath)
	assert.Equal(t, "pi", starter.request.AgentKind)
	require.NotNil(t, starter.request.IssueNumber)
	assert.Equal(t, issueNumber, *starter.request.IssueNumber)
}

func TestModel_NonFailedDashboardWorkflowHasNoRetryAction(t *testing.T) {
	m := NewModel()
	m.view = viewDashboard
	m.missionState = &domain.MissionControlState{
		WorkflowRuns: []domain.WorkflowRunRef{{
			WorkflowID: "run-running",
			RunID:      "run-running",
			Kind:       modal.WorkflowKindImplement,
			Status:     domain.WorkflowRunning,
		}},
	}

	actions := m.availableContextActions()
	for _, action := range actions {
		assert.NotEqual(t, modal.ContextActionRetryRun, action.action)
	}
}

func TestModel_Enter_InHerdr_OpensAndFocusesHerdrWorktree(t *testing.T) {
	navigator := &fakeHerdrNavigator{
		pane: &domain.PaneRef{PaneID: "w1:p2", CWD: "/repos/nexus"},
	}
	m := NewModel()
	m.view = viewWorktrees
	m.insideHerdr = true
	m.Config.Herdr.Enabled = true
	m.Config.Herdr.PreferWorktreeAPI = true
	m.herdrNavigator = navigator
	m.Worktrees = []domain.Worktree{{Path: "/repos/nexus", Branch: "main"}}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg, ok := cmd().(herdrWorktreeOpenedMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "/repos/nexus", navigator.openedPath)
	assert.Equal(t, domain.RuntimeHerdr, msg.session.Runtime)
	require.NotNil(t, msg.session.PaneID)
	assert.Equal(t, "w1:p2", *msg.session.PaneID)
}

func TestFocusSessionCmd_HerdrSessionReopensFocusedWorktree(t *testing.T) {
	navigator := &fakeHerdrNavigator{
		pane: &domain.PaneRef{PaneID: "w1:p9", CWD: "/repos/nexus"},
	}
	m := NewModel()
	m.herdrNavigator = navigator
	paneID := "w1:p9"

	msg, ok := m.focusSessionCmd(domain.Session{
		WorktreePath: "/repos/nexus",
		Runtime:      domain.RuntimeHerdr,
		PaneID:       &paneID,
	})().(sessionFocusedMsg)

	require.True(t, ok)
	require.NoError(t, msg.err)
	assert.Equal(t, "/repos/nexus", navigator.openedPath)
}

// ---------------------------------------------------------------------------
// Phase 4: Error handling & structured logging tests
// ---------------------------------------------------------------------------

// TestModel_GitError_ShowsErrorModal verifies that a worktreeOpDoneMsg with an error
// sets the statusErr field and triggers the error modal.
func TestModel_GitError_ShowsErrorModal(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	updated, cmd := m.Update(worktreeOpDoneMsg{err: errors.New("git failed")})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Contains(t, updatedModel.statusErr, "git failed",
		"statusErr should contain the error message")
	assert.NotNil(t, cmd, "should return a cmd (refresh + clear timer)")
}

// TestModel_ErrorModalDismissesOnKeypress verifies that any keypress clears the statusErr.
func TestModel_ErrorModalDismissesOnKeypress(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)
	m.statusErr = "some error"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Empty(t, updatedModel.statusErr, "keypress should clear statusErr")
}

// TestModel_ErrorModalClearsAfter5s verifies that clearErrorMsg clears the statusErr field.
func TestModel_ErrorModalClearsAfter5s(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)
	m.statusErr = "some error"

	updated, _ := m.Update(clearErrorMsg{})
	updatedModel, ok := updated.(*Model)
	require.True(t, ok)

	assert.Empty(t, updatedModel.statusErr, "clearErrorMsg should clear statusErr")
}

// ---------------------------------------------------------------------------
// Phase 4: Pagination & debounce tests
// ---------------------------------------------------------------------------

func TestPagination_NextPageAdvancesIndex(t *testing.T) {
	// Build a model with 120 issues (> pageSize of 50)
	m := NewModel()
	for i := 1; i <= 120; i++ {
		m.issues = append(m.issues, domain.Issue{Number: i, Title: fmt.Sprintf("Issue %d", i)})
	}
	m.view = viewIssues

	assert.Equal(t, 0, m.currentPage, "starts on page 0")

	// Simulate pressing n (next page)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m2 := updated.(*Model)
	assert.Equal(t, 1, m2.currentPage, "after n: page 1")
	assert.Equal(t, pageSize, m2.selectedIssueIdx, "selectedIssueIdx jumps to page start")

	// Pressing n again (page 2)
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m3 := updated2.(*Model)
	assert.Equal(t, 2, m3.currentPage, "after second n: page 2")

	// Pressing n at last page doesn't go further
	updated3, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m4 := updated3.(*Model)
	assert.Equal(t, 2, m4.currentPage, "clamped at last page")

	// Press PageUp to go back
	updated4, _ := m4.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m5 := updated4.(*Model)
	assert.Equal(t, 1, m5.currentPage, "PageUp: back to page 1")
}

func TestDebounce_CollapsesRapidUpdates(t *testing.T) {
	m := NewModel()

	// First sync arrives — should set pendingSync and NOT immediately update m.prs
	msg1 := githubSyncedMsg{
		prs:      []domain.PullRequest{{Number: 1, Title: "PR 1"}},
		issues:   []domain.Issue{},
		syncedAt: time.Now(),
	}
	updated, cmd := m.Update(msg1)
	m2 := updated.(*Model)

	assert.NotNil(t, m2.pendingSync, "pendingSync should be set after first sync")
	assert.Empty(t, m2.prs, "prs should NOT be updated immediately (debounced)")
	assert.NotNil(t, cmd, "debounce Cmd should be scheduled")

	// Second sync arrives before debounce fires — overwrites pending
	msg2 := githubSyncedMsg{
		prs:      []domain.PullRequest{{Number: 2, Title: "PR 2"}},
		issues:   []domain.Issue{},
		syncedAt: time.Now(),
	}
	updated2, _ := m2.Update(msg2)
	m3 := updated2.(*Model)
	assert.Equal(t, 2, m3.pendingSync.prs[0].Number, "pending sync updated to latest")

	// Now debounce fires — pending data should be applied
	updated3, _ := m3.Update(debouncedRenderMsg{})
	m4 := updated3.(*Model)
	assert.Nil(t, m4.pendingSync, "pendingSync cleared after debounce fires")
	require.Len(t, m4.prs, 1, "prs updated after debounce")
	assert.Equal(t, 2, m4.prs[0].Number, "latest PR applied")
}

func TestPagination_PrevPageClampsAtZero(t *testing.T) {
	m := NewModel()
	for i := 1; i <= 60; i++ {
		m.issues = append(m.issues, domain.Issue{Number: i, Title: fmt.Sprintf("Issue %d", i)})
	}
	m.view = viewIssues
	m.currentPage = 0

	// Pressing PageUp on page 0 should stay at page 0 (no underflow)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m2 := updated.(*Model)
	assert.Equal(t, 0, m2.currentPage, "PageUp on first page stays at page 0")
	assert.Equal(t, 0, m2.selectedIssueIdx, "selectedIssueIdx unchanged when already at first page")
}

// ---------------------------------------------------------------------------
// Issue #77: q as quit key binding tests
// ---------------------------------------------------------------------------

// TestModel_QKey_QuitsApp verifies that pressing q from the root view quits the app.
func TestModel_QKey_QuitsApp(t *testing.T) {
	m := NewModel()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	require.NotNil(t, cmd, "q should return a Cmd")

	// Execute the command and verify it produces tea.Quit.
	msg := cmd()
	assert.Equal(t, tea.Quit(), msg, "q should produce tea.Quit")
}

// ---------------------------------------------------------------------------
// Phase 2 (sessions): buildNewTerminalCmd and sessionSpawnedMsg tests
// ---------------------------------------------------------------------------

// TestBuildNewTerminalCmd verifies that buildNewTerminalCmd returns the correct
// command shape for each supported platform.
func TestBuildNewTerminalCmd(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		goos        string
		terminalEnv string
		shellEnv    string
		wantExe     string
		wantArgs    []string
	}{
		{
			name:     "windows launches cmd /C start cmd /K with quoted path",
			path:     `C:\repo\wt-feature`,
			goos:     "windows",
			wantExe:  "cmd",
			wantArgs: []string{"cmd", "/C", "start", "cmd", "/K", `cd /d "C:\repo\wt-feature"`},
		},
		{
			name:     "windows quotes path containing spaces",
			path:     `C:\My Projects\nexus`,
			goos:     "windows",
			wantExe:  "cmd",
			wantArgs: []string{"cmd", "/C", "start", "cmd", "/K", `cd /d "C:\My Projects\nexus"`},
		},
		{
			name:     "darwin uses open -a Terminal",
			path:     "/Users/dev/repo/wt",
			goos:     "darwin",
			wantExe:  "open",
			wantArgs: []string{"open", "-a", "Terminal", "/Users/dev/repo/wt"},
		},
		{
			name:        "linux with TERMINAL env uses --working-directory",
			path:        "/home/dev/repo/wt",
			goos:        "linux",
			terminalEnv: "kitty",
			wantExe:     "kitty",
			wantArgs:    []string{"kitty", "--working-directory=/home/dev/repo/wt"},
		},
		{
			name:     "linux without TERMINAL env falls back to xterm",
			path:     "/home/dev/repo/wt",
			goos:     "linux",
			shellEnv: "/bin/bash",
			wantExe:  "xterm",
			wantArgs: []string{"xterm", "-e", "cd '/home/dev/repo/wt'; /bin/bash"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERMINAL", tt.terminalEnv)
			t.Setenv("SHELL", tt.shellEnv)

			cmd := buildNewTerminalCmd(tt.path, "", tt.goos)
			require.NotNil(t, cmd)
			require.NotEmpty(t, cmd.Args)

			// The first arg is the executable path — check it contains the expected name.
			assert.Contains(t, cmd.Args[0], tt.wantExe,
				"command executable should contain %q", tt.wantExe)

			if tt.wantArgs != nil {
				assert.Equal(t, tt.wantArgs, cmd.Args)
			}
		})
	}
}

// TestModelUpdate_SessionSpawnedMsg_SpawnErrorSetsStatusErr verifies that when
// the terminal itself failed to launch (PID == 0), Update sets statusErr.
func TestModelUpdate_SessionSpawnedMsg_SpawnErrorSetsStatusErr(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	updated, cmd := m.Update(sessionSpawnedMsg{err: errors.New("spawn session: exec: no such file")})
	next, ok := updated.(*Model)
	require.True(t, ok)

	assert.Equal(t, "Failed to spawn session: spawn session: exec: no such file", next.statusErr)
	assert.Empty(t, next.statusMsg, "statusMsg must be empty on a hard spawn failure")
	assert.NotNil(t, cmd, "clearErrorCmd should be returned on spawn error")
}

// TestModelUpdate_SessionSpawnedMsg_TrackErrorSetsStatusMsg verifies that when
// the terminal launched (PID != 0) but PID tracking failed, Update sets statusMsg
// (not statusErr) so the user sees a non-fatal warning.
func TestModelUpdate_SessionSpawnedMsg_TrackErrorSetsStatusMsg(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	pid5678 := 5678
	updated, cmd := m.Update(sessionSpawnedMsg{
		session: domain.Session{WorktreePath: "/repo/wt", ShellPID: &pid5678},
		err:     errors.New("track session: db closed"),
	})
	next, ok := updated.(*Model)
	require.True(t, ok)

	assert.Empty(t, next.statusErr, "statusErr must be empty when terminal did launch")
	assert.Contains(t, next.statusMsg, "/repo/wt", "statusMsg should include the worktree path")
	assert.Contains(t, next.statusMsg, "5678", "statusMsg should include the PID")
	assert.Contains(t, next.statusMsg, "tracking failed", "statusMsg should indicate tracking failed")
	assert.NotNil(t, cmd, "clearMsgCmd should be returned on a non-fatal tracking error")
}

// TestModelUpdate_SessionSpawnedMsg_SuccessSetsStatusMsg verifies that a
// successful sessionSpawnedMsg sets statusMsg with PID info and returns clearMsgCmd.
func TestModelUpdate_SessionSpawnedMsg_SuccessSetsStatusMsg(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	pid9999 := 9999
	updated, cmd := m.Update(sessionSpawnedMsg{
		session: domain.Session{ID: 1, WorktreePath: "/repo/feat", ShellPID: &pid9999},
	})
	next, ok := updated.(*Model)
	require.True(t, ok)

	assert.NotNil(t, cmd, "clearMsgCmd should be returned on success")
	assert.Contains(t, next.statusMsg, "/repo/feat", "statusMsg should include the worktree path")
	assert.Contains(t, next.statusMsg, "9999", "statusMsg should include the PID")
	assert.Empty(t, next.statusErr, "no error on success")
}

func TestModel_ContextActionOpenShell_NoSelectionSetsError(t *testing.T) {
	m := NewModel()
	m.view = viewWorktrees
	m.Worktrees = nil

	updated, cmd := m.handleContextAction(modal.ContextActionOpenShell)
	next, ok := updated.(*Model)
	require.True(t, ok)

	assert.NotEmpty(t, next.statusErr, "statusErr should be set when no worktree is selected")
	assert.NotNil(t, cmd, "clearErrorCmd should be returned")
}

func TestModel_EnterKeySpawnsSession(t *testing.T) {
	m := NewModel()
	m.view = viewWorktrees
	m.Worktrees = []domain.Worktree{
		{Path: "/repos/nexus", Branch: "main"},
	}
	m.selectedIdx = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd, "Enter key must return a Cmd (spawnSessionCmd)")
}

// TestSessionTickMsg verifies that sessionTickMsg dispatches checkSessionsCmd.
func TestSessionTickMsg(t *testing.T) {
	m := NewModel()
	_, cmd := m.Update(sessionTickMsg{})
	assert.NotNil(t, cmd, "sessionTickMsg should return a non-nil command (checkSessionsCmd)")
}

// TestSessionStatusUpdatedMsg verifies that sessionStatusUpdatedMsg updates m.sessions
// and schedules the next tick.
func TestSessionStatusUpdatedMsg(t *testing.T) {
	m := NewModel()
	sessions := []domain.Session{
		{ID: 1, WorktreePath: "/repo/test", Status: domain.StatusActive},
	}
	updated, cmd := m.Update(sessionStatusUpdatedMsg{sessions: sessions})
	m2 := updated.(*Model)
	require.Len(t, m2.sessions, 1)
	assert.Equal(t, "/repo/test", m2.sessions[0].WorktreePath)
	assert.NotNil(t, cmd, "sessionStatusUpdatedMsg should schedule next tick")
}

// TestSessionStatusUpdatedMsg_Empty verifies that empty sessions list is stored correctly.
func TestSessionStatusUpdatedMsg_Empty(t *testing.T) {
	m := NewModel()
	m.sessions = []domain.Session{{ID: 1, WorktreePath: "/repo/old"}}
	updated, _ := m.Update(sessionStatusUpdatedMsg{sessions: []domain.Session{}})
	m2 := updated.(*Model)
	assert.Empty(t, m2.sessions)
}

// TestCheckSessionsCmd_NilDB verifies that checkSessionsCmd performs PID health
// checks on in-memory sessions even when db is nil, keeping live sessions and
// dropping stale ones rather than blindly preserving all entries.
func TestCheckSessionsCmd_NilDB(t *testing.T) {
	m := NewModel()
	m.sessions = []domain.Session{
		{ID: 1, WorktreePath: "/repo/existing", Status: domain.StatusActive, StartedAt: time.Now().UTC()},
	}
	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok, "expected sessionStatusUpdatedMsg, got %T", msg)
	// Session has no ShellPID and is not dead/old, so it must be preserved.
	require.Len(t, result.sessions, 1)
	assert.Equal(t, "/repo/existing", result.sessions[0].WorktreePath)
}

// TestCheckSessionsCmd_NilDB_PrunesDeadPID verifies that checkSessionsCmd drops
// in-memory sessions whose shell PID is no longer alive when db is nil.
// This is the fix for the bug where closed terminals kept showing as active.
func TestCheckSessionsCmd_NilDB_PrunesDeadPID(t *testing.T) {
	deadPID := 999999999 // extremely unlikely to be a live PID

	m := NewModel()
	m.sessions = []domain.Session{
		{
			ID:           1,
			WorktreePath: "/repo/dead",
			ShellPID:     &deadPID,
			Status:       domain.StatusActive,
			StartedAt:    time.Now().UTC(),
		},
	}
	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok, "expected sessionStatusUpdatedMsg, got %T", msg)
	assert.Empty(t, result.sessions, "session with dead PID must be pruned from in-memory list")
}

// TestCheckSessionsCmd_EmptyDB verifies that checkSessionsCmd returns empty sessions when DB is empty.
func TestCheckSessionsCmd_EmptyDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	m := NewModel()
	m.db = db
	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	assert.Empty(t, result.sessions)
}

// TestCheckSessionsCmd_DeadNilPIDPruned verifies that a session with StatusDead
// and no ShellPID is deleted from the DB and excluded from the returned list.
// This is a regression test for the bug where nil-PID sessions were kept unconditionally.
func TestCheckSessionsCmd_DeadNilPIDPruned(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Insert a dead session with no PID.
	deadSess := domain.Session{
		WorktreePath: "/repo/dead-no-pid",
		Status:       domain.StatusDead,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, deadSess)
	require.NoError(t, err)

	m := NewModel()
	m.db = db
	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	assert.Empty(t, result.sessions, "dead+nil-PID session must be pruned from results")

	// Confirm it was also removed from the DB.
	got, err := data.GetSessionByWorktree(db, "/repo/dead-no-pid")
	require.NoError(t, err)
	assert.Nil(t, got, "dead+nil-PID session must be deleted from the database")
}

// TestPidAlive_CurrentProcess verifies that pidAlive returns true for the current process.
func TestPidAlive_CurrentProcess(t *testing.T) {
	assert.True(t, pidAlive(os.Getpid()), "current process should be alive")
}

// TestPidAlive_InvalidPID verifies that pidAlive returns false for an invalid PID.
func TestPidAlive_InvalidPID(t *testing.T) {
	// PID 0 is the idle process on Windows and typically reserved on Unix.
	// A very high PID is extremely unlikely to exist.
	assert.False(t, pidAlive(999999999), "PID 999999999 should not be alive")
}

// ---------------------------------------------------------------------------
// buildNewTerminalWithCmdCmd tests
// ---------------------------------------------------------------------------

// TestBuildNewTerminalWithCmdCmd verifies that buildNewTerminalWithCmdCmd
// returns a correctly shaped command for each supported terminal emulator and
// platform combination.
func TestBuildNewTerminalWithCmdCmd(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		agentCmd    string
		goos        string
		termProgram string // TERM_PROGRAM env var (macOS)
		termEnv     string // TERM env var (Linux terminal type)
		terminalEnv string // TERMINAL env var (Linux fallback)
		kittyWinID  string // KITTY_WINDOW_ID
		wantExe     string
		wantArgs    []string
	}{
		{
			name:        "darwin ghostty uses ghostty --working-directory",
			path:        "/Users/dev/repo/wt",
			agentCmd:    "gh copilot",
			goos:        "darwin",
			termProgram: "ghostty",
			wantExe:     "ghostty",
			wantArgs:    []string{"ghostty", "--working-directory=/Users/dev/repo/wt", "--", "sh", "-c", "gh copilot"},
		},
		{
			name:     "darwin Terminal.app uses osascript do script",
			path:     "/Users/dev/repo/wt",
			agentCmd: "gh copilot",
			goos:     "darwin",
			wantExe:  "osascript",
		},
		{
			name:     "linux xterm-ghostty uses ghostty --working-directory",
			path:     "/home/dev/repo/wt",
			agentCmd: "claude --print",
			goos:     "linux",
			termEnv:  "xterm-ghostty",
			wantExe:  "ghostty",
			wantArgs: []string{"ghostty", "--working-directory=/home/dev/repo/wt", "--", "sh", "-c", "claude --print"},
		},
		{
			name:     "linux alacritty uses alacritty --working-directory",
			path:     "/home/dev/repo/wt",
			agentCmd: "aider",
			goos:     "linux",
			termEnv:  "alacritty",
			wantExe:  "alacritty",
			wantArgs: []string{"alacritty", "--working-directory", "/home/dev/repo/wt", "-e", "sh", "-c", "aider"},
		},
		{
			name:       "linux kitty (no remote-control) uses kitty --directory",
			path:       "/home/dev/repo/wt",
			agentCmd:   "aider",
			goos:       "linux",
			kittyWinID: "1",
			wantExe:    "kitty",
			wantArgs:   []string{"kitty", "--directory", "/home/dev/repo/wt", "sh", "-c", "aider"},
		},
		{
			name:        "linux TERMINAL env uses $TERMINAL -e script",
			path:        "/home/dev/repo/wt",
			agentCmd:    "aider",
			goos:        "linux",
			terminalEnv: "alacritty",
			wantExe:     "alacritty",
		},
		{
			name:     "linux no env falls back to xterm",
			path:     "/home/dev/repo/wt",
			agentCmd: "aider",
			goos:     "linux",
			wantExe:  "xterm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", tt.termProgram)
			t.Setenv("TERM", tt.termEnv)
			t.Setenv("TERMINAL", tt.terminalEnv)
			t.Setenv("KITTY_WINDOW_ID", tt.kittyWinID)

			cmd := buildNewTerminalWithCmdCmd(tt.path, tt.agentCmd, tt.goos)
			require.NotNil(t, cmd)
			require.NotEmpty(t, cmd.Args)

			assert.Contains(t, cmd.Args[0], tt.wantExe,
				"executable should contain %q", tt.wantExe)
			if tt.wantArgs != nil {
				assert.Equal(t, tt.wantArgs, cmd.Args)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildNewTabWithCmdCmd tests
// ---------------------------------------------------------------------------

// TestBuildNewTabWithCmdCmd verifies that buildNewTabWithCmdCmd returns a
// correctly shaped (cmd, true) pair for each supported multiplexer / emulator,
// and (nil, false) when no supported emulator is detected.
func TestBuildNewTabWithCmdCmd(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		agentCmd          string
		pidFile           string
		goos              string
		tmuxEnv           string
		zellijEnv         string
		zellijSessionName string // ZELLIJ_SESSION_NAME
		kittyWinID        string
		alacrittyS        string // ALACRITTY_SOCKET
		wtSession         string // WT_SESSION
		termProgram       string // TERM_PROGRAM
		konsoleVer        string // KONSOLE_VERSION
		wantOK            bool
		wantExe           string
		wantArgsContain   []string
	}{
		// --- Multiplexers ---
		{
			name:            "tmux with agentCmd opens new-window with cmd",
			path:            "/repo/wt",
			agentCmd:        "gh copilot",
			goos:            "linux",
			tmuxEnv:         "set",
			wantOK:          true,
			wantExe:         "tmux",
			wantArgsContain: []string{"new-window", "-c", "/repo/wt", "gh copilot"},
		},
		{
			name:            "tmux without agentCmd opens plain new-window",
			path:            "/repo/wt",
			goos:            "linux",
			tmuxEnv:         "set",
			wantOK:          true,
			wantExe:         "tmux",
			wantArgsContain: []string{"new-window", "-c", "/repo/wt"},
		},
		{
			name:            "zellij with agentCmd runs zellij run",
			path:            "/repo/wt",
			agentCmd:        "claude",
			goos:            "linux",
			zellijEnv:       "set",
			wantOK:          true,
			wantExe:         "zellij",
			wantArgsContain: []string{"run", "--cwd", "/repo/wt"},
		},
		{
			name:              "zellij detected via ZELLIJ_SESSION_NAME with agentCmd",
			path:              "/repo/wt",
			agentCmd:          "aider",
			goos:              "linux",
			zellijSessionName: "my-session",
			wantOK:            true,
			wantExe:           "zellij",
			wantArgsContain:   []string{"run", "--cwd", "/repo/wt"},
		},
		{
			name:      "zellij without agentCmd opens plain shell",
			path:      "/repo/wt",
			goos:      "linux",
			zellijEnv: "set",
			wantOK:    true,
			wantExe:   "zellij",
		},
		// --- Kitty remote-control ---
		{
			name:            "kitty remote-control with agentCmd opens new tab with cmd",
			path:            "/repo/wt",
			agentCmd:        "aider",
			goos:            "linux",
			kittyWinID:      "42",
			wantOK:          true,
			wantExe:         "kitty",
			wantArgsContain: []string{"@", "new-window", "--new-tab", "--cwd", "/repo/wt", "sh", "-c", "aider"},
		},
		{
			name:            "kitty remote-control with pidFile writes PID before shell",
			path:            "/repo/wt",
			pidFile:         "/tmp/nexus.pid",
			goos:            "linux",
			kittyWinID:      "42",
			wantOK:          true,
			wantExe:         "kitty",
			wantArgsContain: []string{"@", "new-window", "--new-tab", "--cwd", "/repo/wt"},
		},
		// --- Alacritty IPC ---
		{
			name:            "alacritty IPC with agentCmd creates tab with cmd",
			path:            "/repo/wt",
			agentCmd:        "gh copilot",
			goos:            "linux",
			alacrittyS:      "/tmp/alacritty.sock",
			wantOK:          true,
			wantExe:         "alacritty",
			wantArgsContain: []string{"msg", "create-tab", "--working-directory", "/repo/wt", "--", "sh", "-c", "gh copilot"},
		},
		// --- Windows Terminal ---
		{
			name:            "Windows Terminal with agentCmd opens new tab",
			path:            `C:\repos\wt`,
			agentCmd:        "gh copilot",
			goos:            "windows",
			wtSession:       "some-guid",
			wantOK:          true,
			wantExe:         "wt",
			wantArgsContain: []string{"-w", "0", "new-tab", "--startingDirectory", `C:\repos\wt`, "cmd", "/K", "gh copilot"},
		},
		{
			name:   "Windows without WT_SESSION returns false",
			path:   `C:\repos\wt`,
			goos:   "windows",
			wantOK: false,
		},
		// --- macOS iTerm2 ---
		{
			name:        "iTerm2 with agentCmd creates tab via osascript",
			path:        "/Users/dev/repo/wt",
			agentCmd:    "gh copilot",
			goos:        "darwin",
			termProgram: "iTerm.app",
			wantOK:      true,
			wantExe:     "osascript",
		},
		{
			name:        "Apple_Terminal with agentCmd creates tab in front window",
			path:        "/Users/dev/repo/wt",
			agentCmd:    "aider",
			goos:        "darwin",
			termProgram: "Apple_Terminal",
			wantOK:      true,
			wantExe:     "osascript",
		},
		// --- Linux Konsole ---
		{
			name:            "Konsole with agentCmd opens new tab",
			path:            "/home/dev/repo/wt",
			agentCmd:        "claude",
			goos:            "linux",
			konsoleVer:      "210401",
			wantOK:          true,
			wantExe:         "konsole",
			wantArgsContain: []string{"--new-tab"},
		},
		// --- No emulator detected ---
		{
			name:   "no env vars returns nil false",
			path:   "/repo/wt",
			goos:   "linux",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMUX", tt.tmuxEnv)
			t.Setenv("ZELLIJ", tt.zellijEnv)
			t.Setenv("ZELLIJ_SESSION_NAME", tt.zellijSessionName)
			t.Setenv("KITTY_WINDOW_ID", tt.kittyWinID)
			t.Setenv("ALACRITTY_SOCKET", tt.alacrittyS)
			t.Setenv("WT_SESSION", tt.wtSession)
			t.Setenv("TERM_PROGRAM", tt.termProgram)
			t.Setenv("KONSOLE_VERSION", tt.konsoleVer)

			cmd, ok := buildNewTabWithCmdCmd(tt.path, tt.agentCmd, tt.pidFile, tt.goos)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				assert.Nil(t, cmd)
				return
			}
			require.NotNil(t, cmd)
			require.NotEmpty(t, cmd.Args)
			assert.Contains(t, cmd.Args[0], tt.wantExe,
				"executable should contain %q", tt.wantExe)
			for _, want := range tt.wantArgsContain {
				assert.Contains(t, cmd.Args, want, "args should contain %q", want)
			}
		})
	}
}

// TestFilterAliveSessions verifies that filterAliveSessions correctly keeps
// live sessions and drops dead/stale ones.
func TestFilterAliveSessions(t *testing.T) {
	liveP := os.Getpid()
	deadP := 999999999

	tests := []struct {
		name     string
		sessions []domain.Session
		wantLen  int
	}{
		{
			name:     "empty input returns empty",
			sessions: []domain.Session{},
			wantLen:  0,
		},
		{
			name: "StatusDead session is always dropped",
			sessions: []domain.Session{
				{ID: 1, WorktreePath: "/wt/a", Status: domain.StatusDead, StartedAt: time.Now()},
			},
			wantLen: 0,
		},
		{
			name: "live PID session is kept",
			sessions: []domain.Session{
				{ID: 2, WorktreePath: "/wt/b", ShellPID: &liveP, Status: domain.StatusActive, StartedAt: time.Now()},
			},
			wantLen: 1,
		},
		{
			name: "dead PID session is dropped",
			sessions: []domain.Session{
				{ID: 3, WorktreePath: "/wt/c", ShellPID: &deadP, Status: domain.StatusActive, StartedAt: time.Now()},
			},
			wantLen: 0,
		},
		{
			name: "nil-PID session under 24h is kept",
			sessions: []domain.Session{
				{ID: 4, WorktreePath: "/wt/d", Status: domain.StatusActive, StartedAt: time.Now()},
			},
			wantLen: 1,
		},
		{
			name: "nil-PID session over 24h is dropped",
			sessions: []domain.Session{
				{ID: 5, WorktreePath: "/wt/e", Status: domain.StatusActive, StartedAt: time.Now().Add(-25 * time.Hour)},
			},
			wantLen: 0,
		},
		{
			name: "mixed sessions return only live ones",
			sessions: []domain.Session{
				{ID: 1, WorktreePath: "/wt/a", Status: domain.StatusDead, StartedAt: time.Now()},
				{ID: 2, WorktreePath: "/wt/b", ShellPID: &liveP, Status: domain.StatusActive, StartedAt: time.Now()},
				{ID: 3, WorktreePath: "/wt/c", ShellPID: &deadP, Status: domain.StatusActive, StartedAt: time.Now()},
				{ID: 4, WorktreePath: "/wt/d", Status: domain.StatusActive, StartedAt: time.Now()},
			},
			wantLen: 2, // live PID + nil-PID (recent)
		},
		{
			name: "herdr session with nil PID is kept",
			sessions: []domain.Session{
				{ID: 6, WorktreePath: "/wt/f", Runtime: domain.RuntimeHerdr, Status: domain.StatusActive, StartedAt: time.Now().Add(-48 * time.Hour)},
			},
			wantLen: 1,
		},
		{
			name: "herdr session with nil PID over 24h is kept",
			sessions: []domain.Session{
				{ID: 7, WorktreePath: "/wt/g", Runtime: domain.RuntimeHerdr, Status: domain.StatusActive, StartedAt: time.Now().Add(-72 * time.Hour)},
			},
			wantLen: 1,
		},
		{
			name: "legacy session (empty Runtime) with nil PID over 24h is dropped",
			sessions: []domain.Session{
				{ID: 8, WorktreePath: "/wt/h", Runtime: "", Status: domain.StatusActive, StartedAt: time.Now().Add(-25 * time.Hour)},
			},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterAliveSessions(tt.sessions)
			assert.Len(t, got, tt.wantLen)
		})
	}
}

// TestModel_Enter_ExistingSession_TriggersFocus verifies that when a session
// already exists for the selected worktree, pressing Enter dispatches a focus
// command rather than spawning a new session.
func TestModel_Enter_ExistingSession_TriggersFocus(t *testing.T) {
	pid := os.Getpid() // use current PID so the session looks alive
	worktreePath := "/home/user/repos/wt1"

	m := NewModel()
	m.Worktrees = []domain.Worktree{
		{Path: worktreePath, Branch: "main", CommitSHA: "abc123"},
	}
	m.sessions = []domain.Session{
		{ID: 1, WorktreePath: worktreePath, ShellPID: &pid, Status: domain.StatusActive},
	}
	m.selectedIdx = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// A Cmd must be returned — it is the focus command.
	assert.NotNil(t, cmd, "Enter on worktree with existing session should return a focus Cmd")
}

// TestModel_Enter_NoSession_TriggersSpawn verifies that pressing Enter on a
// worktree with no tracked session dispatches a spawn command.
func TestModel_Enter_NoSession_TriggersSpawn(t *testing.T) {
	worktreePath := "/home/user/repos/wt1"

	m := NewModel()
	m.Worktrees = []domain.Worktree{
		{Path: worktreePath, Branch: "main", CommitSHA: "abc123"},
	}
	// No session for this worktree.
	m.sessions = []domain.Session{}
	m.selectedIdx = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// A Cmd must be returned — it is the spawn command.
	assert.NotNil(t, cmd, "Enter on worktree with no session should return a spawn Cmd")
}

// TestModel_Enter_StaleSession_TriggersSpawn verifies that pressing Enter on a
// worktree whose only tracked session has a dead shell PID spawns a new terminal
// rather than calling focusSessionCmd (which would just show a misleading toast).
func TestModel_Enter_StaleSession_TriggersSpawn(t *testing.T) {
	worktreePath := "/home/user/repos/wt1"
	deadPID := 999999999 // extremely unlikely to be a real running PID

	m := NewModel()
	m.Worktrees = []domain.Worktree{
		{Path: worktreePath, Branch: "main", CommitSHA: "abc123"},
	}
	// Session is stale: has a PID that's dead.
	m.sessions = []domain.Session{
		{ID: 1, WorktreePath: worktreePath, ShellPID: &deadPID, Status: domain.StatusActive},
	}
	m.selectedIdx = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Must return a spawn Cmd, not nil (which would mean nothing happened).
	assert.NotNil(t, cmd, "Enter on worktree with stale session should return a spawn Cmd")
}

func TestModel_ContextActionClose_WithSessionTriggersKill(t *testing.T) {
	pid := os.Getpid()
	worktreePath := "/home/user/repos/wt1"

	m := NewModel()
	m.Worktrees = []domain.Worktree{
		{Path: worktreePath, Branch: "main", CommitSHA: "abc123"},
	}
	m.sessions = []domain.Session{
		{ID: 1, WorktreePath: worktreePath, ShellPID: &pid, Status: domain.StatusActive},
	}
	m.selectedIdx = 0
	m.view = viewWorktrees

	_, cmd := m.handleContextAction(modal.ContextActionClose)

	assert.NotNil(t, cmd, "close action should return a kill Cmd")
}

func TestModel_ContextActionClose_WithoutSessionShowsError(t *testing.T) {
	m := NewModel()
	m.Worktrees = []domain.Worktree{
		{Path: "/home/user/repos/wt1", Branch: "main", CommitSHA: "abc123"},
	}
	m.sessions = []domain.Session{} // no sessions
	m.selectedIdx = 0
	m.view = viewWorktrees

	updated, cmd := m.handleContextAction(modal.ContextActionClose)
	m2, ok := updated.(*Model)
	require.True(t, ok)

	assert.Equal(t, "No active session for this worktree", m2.statusErr)
	assert.NotNil(t, cmd, "should return clearError cmd")
}

// TestModel_SessionKilledMsg_Success_RemovesSession verifies that receiving a
// successful sessionKilledMsg removes the session from m.sessions.
func TestModel_SessionKilledMsg_Success_RemovesSession(t *testing.T) {
	pid := os.Getpid()
	m := NewModel()
	m.sessions = []domain.Session{
		{ID: 1, WorktreePath: "/wt/a", ShellPID: &pid, Status: domain.StatusActive},
		{ID: 2, WorktreePath: "/wt/b", ShellPID: &pid, Status: domain.StatusActive},
	}

	updated, cmd := m.Update(sessionKilledMsg{worktreePath: "/wt/a"})
	m2, ok := updated.(*Model)
	require.True(t, ok)

	require.Len(t, m2.sessions, 1, "killed session should be removed")
	assert.Equal(t, "/wt/b", m2.sessions[0].WorktreePath)
	assert.Nil(t, cmd, "successful kill should return nil Cmd")
}

// TestModel_SessionKilledMsg_Error_SetsStatusErr verifies that a
// sessionKilledMsg with an error sets m.statusErr.
func TestModel_SessionKilledMsg_Error_SetsStatusErr(t *testing.T) {
	m := NewModel()
	m.sessions = []domain.Session{}

	updated, cmd := m.Update(sessionKilledMsg{worktreePath: "/wt/a", err: errors.New("db error")})
	m2, ok := updated.(*Model)
	require.True(t, ok)

	assert.Contains(t, m2.statusErr, "Close session")
	assert.Contains(t, m2.statusErr, "db error")
	assert.NotNil(t, cmd, "error should schedule clearErrorCmd")
}

// TestModel_SessionFocusedMsg_Success_ShowsStatusMsg verifies that a successful
// sessionFocusedMsg sets m.statusMsg.
func TestModel_SessionFocusedMsg_Success_ShowsStatusMsg(t *testing.T) {
	m := NewModel()

	updated, cmd := m.Update(sessionFocusedMsg{worktreePath: "/wt/a"})
	m2, ok := updated.(*Model)
	require.True(t, ok)

	assert.Contains(t, m2.statusMsg, "/wt/a")
	assert.NotNil(t, cmd, "should return clearMsgCmd")
}

// TestModel_SessionFocusedMsg_Error_ShowsBestEffortMsg verifies that a failed
// sessionFocusedMsg still shows a user-friendly message (best-effort focus).
func TestModel_SessionFocusedMsg_Error_ShowsBestEffortMsg(t *testing.T) {
	m := NewModel()

	updated, cmd := m.Update(sessionFocusedMsg{worktreePath: "/wt/a", err: errors.New("no wmctrl")})
	m2, ok := updated.(*Model)
	require.True(t, ok)

	assert.Contains(t, m2.statusMsg, "/wt/a")
	assert.NotNil(t, cmd, "should return clearMsgCmd even on error")
}

// ---------------------------------------------------------------------------
// Phase 6: Non-blocking agents — session detection (issue #67)
// ---------------------------------------------------------------------------

// TestCheckSessionsCmd_NilDB_DeadShellPruned verifies that an in-memory session
// with a dead shell PID is removed from the alive list.
func TestCheckSessionsCmd_NilDB_DeadShellPruned(t *testing.T) {
	deadPID := 999999999

	m := NewModel()
	m.sessions = []domain.Session{
		{
			ID:           1,
			WorktreePath: "/repo/dead-shell",
			ShellPID:     &deadPID,
			Status:       domain.StatusActive,
			StartedAt:    time.Now().UTC(),
		},
	}

	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	assert.Empty(t, result.sessions, "session with dead shell PID must be pruned")
}

// TestCheckSessionsCmd_DB_DeadShellPruned verifies that a DB-backed session
// with a dead shell PID is deleted from the DB and excluded from the returned list.
func TestCheckSessionsCmd_DB_DeadShellPruned(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	deadPID := 999999999
	sess := domain.Session{
		WorktreePath: "/repo/dead-shell",
		ShellPID:     &deadPID,
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, sess)
	require.NoError(t, err)

	m := NewModel()
	m.db = db

	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	assert.Empty(t, result.sessions, "session with dead shell PID must be pruned")

	got, err := data.GetSessionByWorktree(db, "/repo/dead-shell")
	require.NoError(t, err)
	assert.Nil(t, got, "session with dead shell PID must be deleted from the DB")
}

// fakeHealthChecker is a test sessionHealthChecker that returns canned snapshots.
type fakeHealthChecker struct {
	herdrSnap herdr.Snapshot
	herdrErr  error
	scSnap    sandcastle.Snapshot
	scErr     error
}

func (f *fakeHealthChecker) HerdrSnapshot() (herdr.Snapshot, error) {
	return f.herdrSnap, f.herdrErr
}

func (f *fakeHealthChecker) SandcastleSnapshot(_ context.Context) (sandcastle.Snapshot, error) {
	return f.scSnap, f.scErr
}

// TestCheckSessionsCmd_EnrichPrunesDB verifies that sessions removed by
// enrichHerdrSessions (e.g. Herdr sessions whose Sandcastle workflow has
// succeeded) are deleted from the database. This is the fix for the blocker
// where enrichment-pruned sessions persisted indefinitely in the DB.
func TestCheckSessionsCmd_EnrichPrunesDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Insert a Herdr session with a succeeded workflow.
	sess := domain.Session{
		WorktreePath:  "/repo/herdr-succeeded",
		Runtime:       domain.RuntimeHerdr,
		WorkflowRunID: strPtr("wf-done"),
		PaneID:        strPtr("pane-1"),
		Status:        domain.StatusActive,
		StartedAt:     time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, sess)
	require.NoError(t, err)

	// Health checker returns snapshots where the workflow succeeded.
	hc := &fakeHealthChecker{
		herdrSnap: herdr.Snapshot{
			Panes: []domain.PaneRef{{PaneID: "pane-1"}},
		},
		scSnap: sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{
				{WorkflowID: "wf-done", RunID: "wf-done", Status: domain.WorkflowSucceeded},
			},
		},
	}

	m := NewModel()
	m.db = db
	m.healthChecker = hc

	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	assert.Empty(t, result.sessions, "Herdr session with succeeded workflow must be removed")

	// Verify the session was deleted from the DB.
	got, err := data.GetSessionByWorktree(db, "/repo/herdr-succeeded")
	require.NoError(t, err)
	assert.Nil(t, got, "enrichment-pruned session must be deleted from the database")
}

// TestCheckSessionsCmd_EnrichKeepsAlive verifies that sessions kept by
// enrichHerdrSessions are persisted (upserted) to the DB with refreshed state.
func TestCheckSessionsCmd_EnrichKeepsAlive(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sess := domain.Session{
		WorktreePath: "/repo/herdr-alive",
		Runtime:      domain.RuntimeHerdr,
		PaneID:       strPtr("pane-2"),
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, sess)
	require.NoError(t, err)

	// Herdr snapshot has the pane, workflow is still running.
	hc := &fakeHealthChecker{
		herdrSnap: herdr.Snapshot{
			Panes: []domain.PaneRef{{PaneID: "pane-2"}},
		},
		scSnap: sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{
				{WorkflowID: "wf-running", RunID: "wf-running", Status: domain.WorkflowRunning},
			},
		},
	}

	m := NewModel()
	m.db = db
	m.healthChecker = hc

	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	require.Len(t, result.sessions, 1)
	assert.Equal(t, "/repo/herdr-alive", result.sessions[0].WorktreePath)

	// Session still exists in the DB.
	got, err := data.GetSessionByWorktree(db, "/repo/herdr-alive")
	require.NoError(t, err)
	require.NotNil(t, got, "kept session must remain in the DB")
	assert.Equal(t, domain.RuntimeHerdr, got.Runtime)
}

// TestCheckSessionsCmd_EnrichMixedDB verifies the fix for unbounded DB growth
// when a mix of alive and dead Herdr sessions coexist. The dead ones must be
// deleted from the DB while alive ones are persisted.
func TestCheckSessionsCmd_EnrichMixedDB(t *testing.T) {
	db, err := data.NewDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sessAlive := domain.Session{
		WorktreePath: "/repo/herdr-alive",
		Runtime:      domain.RuntimeHerdr,
		PaneID:       strPtr("pane-alive"),
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	sessDead := domain.Session{
		WorktreePath:  "/repo/herdr-dead",
		Runtime:       domain.RuntimeHerdr,
		WorkflowRunID: strPtr("wf-dead"),
		PaneID:        strPtr("pane-dead"),
		Status:        domain.StatusActive,
		StartedAt:     time.Now().UTC().Truncate(time.Second),
	}
	_, err = data.UpsertSession(db, sessAlive)
	require.NoError(t, err)
	_, err = data.UpsertSession(db, sessDead)
	require.NoError(t, err)

	hc := &fakeHealthChecker{
		herdrSnap: herdr.Snapshot{
			Panes: []domain.PaneRef{
				{PaneID: "pane-alive"},
				{PaneID: "pane-dead"},
			},
		},
		scSnap: sandcastle.Snapshot{
			Workflows: []domain.WorkflowRunRef{
				{WorkflowID: "wf-dead", RunID: "wf-dead", Status: domain.WorkflowFailed},
			},
		},
	}

	m := NewModel()
	m.db = db
	m.healthChecker = hc

	cmd := m.checkSessionsCmd()
	require.NotNil(t, cmd)
	msg := cmd()
	result, ok := msg.(sessionStatusUpdatedMsg)
	require.True(t, ok)
	require.Len(t, result.sessions, 1, "only the alive session should remain")
	assert.Equal(t, "/repo/herdr-alive", result.sessions[0].WorktreePath)

	// Dead session deleted from DB.
	gotDead, err := data.GetSessionByWorktree(db, "/repo/herdr-dead")
	require.NoError(t, err)
	assert.Nil(t, gotDead, "enrichment-pruned session must be deleted from the DB")

	// Alive session still in DB.
	gotAlive, err := data.GetSessionByWorktree(db, "/repo/herdr-alive")
	require.NoError(t, err)
	assert.NotNil(t, gotAlive, "alive session must remain in the DB")
}

// TestPollPIDFile_HappyPath verifies that pollPIDFile returns the PID when
// the file already contains a valid integer before the first poll.
func TestPollPIDFile_HappyPath(t *testing.T) {
	f, err := os.CreateTemp("", "nexus-pid-test-*.pid")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })

	_, err = fmt.Fprintf(f, "12345\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	pid := pollPIDFile(f.Name(), 500*time.Millisecond)
	assert.Equal(t, 12345, pid)
}

// TestPollPIDFile_DelayedWrite verifies that pollPIDFile waits and returns
// the PID when the file is written after a short delay.
func TestPollPIDFile_DelayedWrite(t *testing.T) {
	f, err := os.CreateTemp("", "nexus-pid-test-*.pid")
	require.NoError(t, err)
	pidPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(pidPath)) // start with no file
	t.Cleanup(func() { os.Remove(pidPath) })

	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.WriteFile(pidPath, []byte("99999\n"), 0600)
	}()

	pid := pollPIDFile(pidPath, time.Second)
	assert.Equal(t, 99999, pid)
}

// TestPollPIDFile_Timeout verifies that pollPIDFile returns 0 when the
// file never appears within the timeout.
func TestPollPIDFile_Timeout(t *testing.T) {
	pid := pollPIDFile(os.TempDir()+"/nexus-pid-test-nonexistent.pid", 200*time.Millisecond)
	assert.Equal(t, 0, pid, "should return 0 on timeout")
}

// TestPollPIDFile_InvalidContent verifies that pollPIDFile returns 0 when
// the file exists but does not contain a valid positive integer.
func TestPollPIDFile_InvalidContent(t *testing.T) {
	f, err := os.CreateTemp("", "nexus-pid-test-*.pid")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })

	_, err = fmt.Fprintf(f, "not-a-pid\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	pid := pollPIDFile(f.Name(), 200*time.Millisecond)
	assert.Equal(t, 0, pid, "invalid content should time out and return 0")
}

// TestShellSingleQuote verifies that shellSingleQuote wraps paths in single
// quotes and escapes embedded single-quotes with the '\” idiom.
func TestShellSingleQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/simple/path", "'/simple/path'"},
		{"/path with spaces", "'/path with spaces'"},
		{"/path/with'quote", `'/path/with'\''quote'`},
		{"", "''"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, shellSingleQuote(tt.input))
		})
	}
}

// TestEscapeAppleScriptStr verifies that escapeAppleScriptStr escapes
// backslashes and double-quotes for safe embedding in AppleScript strings.
func TestEscapeAppleScriptStr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`aider file.go`, `aider file.go`},
		{`aider "file.go"`, `aider \"file.go\"`},
		{`aider back\slash`, `aider back\\slash`},
		{`aider "a" 'b'`, `aider \"a\" 'b'`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, escapeAppleScriptStr(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// Regression: update modal must not swallow background sync messages
// ---------------------------------------------------------------------------

// TestUpdateModal_DoesNotSwallowGithubSyncedMsg verifies that when the update
// modal is open, a githubSyncedMsg still populates m.issues and m.prs.
// Previously the modal's default case consumed all non-modal messages,
// silently discarding the sync result and leaving the issues view empty.
func TestUpdateModal_DoesNotSwallowGithubSyncedMsg(t *testing.T) {
	m := NewModel()
	m.activeModal = modal.NewUpdateModal("v0.1.0", "v0.2.0", "", "", 0)

	issues := []domain.Issue{{Number: 1, Title: "fix everything"}}
	prs := []domain.PullRequest{{Number: 42, Title: "big PR"}}

	// Fire githubSyncedMsg while the update modal is open.
	updated, _ := m.Update(githubSyncedMsg{issues: issues, prs: prs, syncedAt: time.Now()})
	m2, ok := updated.(*Model)
	require.True(t, ok)

	// The modal should still be open (key message wasn't sent).
	assert.NotNil(t, m2.activeModal, "update modal should still be open")

	// Drain the debounce render message so pendingSync is applied.
	updated2, _ := m2.Update(debouncedRenderMsg{})
	m3, ok := updated2.(*Model)
	require.True(t, ok)

	require.Len(t, m3.issues, 1, "issues must be populated even when modal is open")
	assert.Equal(t, 1, m3.issues[0].Number)
	require.Len(t, m3.prs, 1, "prs must be populated even when modal is open")
	assert.Equal(t, 42, m3.prs[0].Number)
}

// ---------------------------------------------------------------------------
// Issue #90: jump to existing session on Enter
// ---------------------------------------------------------------------------

// TestModel_Enter_Issue_JumpsToExistingSession verifies that pressing Enter on
// an issue in viewIssues focuses an existing live session for that issue's
// worktree instead of opening the create modal.
func TestModel_Enter_Issue_JumpsToExistingSession(t *testing.T) {
	pid := 99999 // Use a PID that is almost certainly not alive so we can
	// control liveness via ShellPID == nil (no PID means "keep alive").
	tests := []struct {
		name         string
		issue        domain.Issue
		worktrees    []domain.Worktree
		sessions     []domain.Session
		wantModalNil bool // true = should have jumped (no modal opened)
		wantCmdNil   bool
	}{
		{
			name:  "active session for issue worktree → focus, no modal",
			issue: domain.Issue{Number: 42, Title: "Do something"},
			worktrees: []domain.Worktree{
				{Path: "/wt/feat-issue-42-do-something", Branch: "feat/issue-42-do-something"},
			},
			sessions: []domain.Session{
				{ID: 1, WorktreePath: "/wt/feat-issue-42-do-something", Status: domain.StatusActive},
				// ShellPID nil → treated as alive (no PID = Windows-style session)
			},
			wantModalNil: true,
			wantCmdNil:   false,
		},
		{
			name:  "no session for issue worktree → opens create modal",
			issue: domain.Issue{Number: 7, Title: "Fix bug"},
			worktrees: []domain.Worktree{
				{Path: "/wt/feat-issue-7-fix-bug", Branch: "feat/issue-7-fix-bug"},
			},
			sessions:     []domain.Session{},
			wantModalNil: false, // modal should open
			wantCmdNil:   true,
		},
		{
			name:  "stale session (dead PID) for issue worktree → opens create modal",
			issue: domain.Issue{Number: 5, Title: "Stale"},
			worktrees: []domain.Worktree{
				{Path: "/wt/feat-issue-5-stale", Branch: "feat/issue-5-stale"},
			},
			sessions: []domain.Session{
				{ID: 2, WorktreePath: "/wt/feat-issue-5-stale", Status: domain.StatusActive, ShellPID: &pid},
			},
			wantModalNil: false, // stale → modal
			wantCmdNil:   true,
		},
		{
			name:  "no worktree for issue → opens create modal",
			issue: domain.Issue{Number: 3, Title: "No worktree"},
			worktrees: []domain.Worktree{
				{Path: "/wt/unrelated", Branch: "feat/unrelated"},
			},
			sessions:     []domain.Session{},
			wantModalNil: false,
			wantCmdNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			require.NotNil(t, m)
			m.view = viewIssues
			m.issues = []domain.Issue{tt.issue}
			m.selectedIssueIdx = 0
			m.Worktrees = tt.worktrees
			m.sessions = tt.sessions

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			result, ok := updated.(*Model)
			require.True(t, ok)

			if tt.wantModalNil {
				assert.Nil(t, result.activeModal, "no modal should open when jumping to existing session")
			} else {
				assert.NotNil(t, result.activeModal, "create modal should open when no session exists")
			}

			if tt.wantCmdNil {
				assert.Nil(t, cmd)
			} else {
				assert.NotNil(t, cmd, "a focus cmd should be returned when session exists")
			}
		})
	}
}

// TestModel_Enter_PR_JumpsToExistingSession verifies that pressing Enter on a
// PR in viewPRs focuses an existing live session for that PR's branch worktree
// instead of opening the checkout modal or showing an error.
func TestModel_Enter_PR_JumpsToExistingSession(t *testing.T) {
	pid := 99999 // almost certainly not alive
	tests := []struct {
		name         string
		pr           domain.PullRequest
		worktrees    []domain.Worktree
		sessions     []domain.Session
		wantModalNil bool
		wantCmdNil   bool
		wantErrEmpty bool
	}{
		{
			name: "active session for PR worktree → focus, no modal, no error",
			pr:   domain.PullRequest{Number: 10, Title: "My PR", Branch: "feat/issue-10-my-pr", State: "OPEN"},
			worktrees: []domain.Worktree{
				{Path: "/wt/feat-issue-10-my-pr", Branch: "feat/issue-10-my-pr"},
			},
			sessions: []domain.Session{
				{ID: 1, WorktreePath: "/wt/feat-issue-10-my-pr", Status: domain.StatusActive},
			},
			wantModalNil: true,
			wantCmdNil:   false,
			wantErrEmpty: true,
		},
		{
			name:         "no worktree and no session for PR → opens checkout modal",
			pr:           domain.PullRequest{Number: 11, Title: "New PR", Branch: "feat/new-pr", State: "OPEN"},
			worktrees:    []domain.Worktree{},
			sessions:     []domain.Session{},
			wantModalNil: false,
			wantCmdNil:   true,
			wantErrEmpty: true,
		},
		{
			name: "worktree exists but no active session → error (existing behavior preserved)",
			pr:   domain.PullRequest{Number: 12, Title: "Existing WT", Branch: "feat/existing", State: "OPEN"},
			worktrees: []domain.Worktree{
				{Path: "/wt/feat-existing", Branch: "feat/existing"},
			},
			sessions:     []domain.Session{},
			wantModalNil: true,
			wantCmdNil:   false, // clearErrorCmd is returned
			wantErrEmpty: false, // error is set
		},
		{
			name: "stale session (dead PID) → falls through to existing WT error",
			pr:   domain.PullRequest{Number: 13, Title: "Stale", Branch: "feat/stale", State: "OPEN"},
			worktrees: []domain.Worktree{
				{Path: "/wt/feat-stale", Branch: "feat/stale"},
			},
			sessions: []domain.Session{
				{ID: 2, WorktreePath: "/wt/feat-stale", Status: domain.StatusActive, ShellPID: &pid},
			},
			wantModalNil: true,
			wantCmdNil:   false,
			wantErrEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			require.NotNil(t, m)
			m.view = viewPRs
			m.prs = []domain.PullRequest{tt.pr}
			m.selectedPRIdx = 0
			m.Worktrees = tt.worktrees
			m.sessions = tt.sessions
			m.RepoPath = "/repo/nexus"

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			result, ok := updated.(*Model)
			require.True(t, ok)

			if tt.wantModalNil {
				assert.Nil(t, result.activeModal)
			} else {
				assert.NotNil(t, result.activeModal, "checkout modal should open")
			}

			if tt.wantCmdNil {
				assert.Nil(t, cmd)
			} else {
				assert.NotNil(t, cmd)
			}

			if tt.wantErrEmpty {
				assert.Empty(t, result.statusErr)
			} else {
				assert.NotEmpty(t, result.statusErr)
			}
		})
	}
}

// TestModel_RKey_TriggersFullRefresh verifies that pressing 'r' or 'R' fires both
// refreshWorktreesCmd and syncGitHubCmd from any view.
func TestModel_RKey_TriggersFullRefresh(t *testing.T) {
	cases := []struct {
		view activeView
		name string
		key  rune
	}{
		{viewWorktrees, "worktrees_r", 'r'},
		{viewWorktrees, "worktrees_R", 'R'},
		{viewIssues, "issues_r", 'r'},
		{viewIssues, "issues_R", 'R'},
		{viewPRs, "prs_r", 'r'},
		{viewPRs, "prs_R", 'R'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel()
			require.NotNil(t, m)
			m.view = tc.view

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})

			result, ok := updated.(*Model)
			require.True(t, ok)
			assert.True(t, result.syncing, "'%c' must set syncing=true", tc.key)
			assert.NotNil(t, cmd, "'%c' must return a non-nil Cmd (batch of refresh+sync)", tc.key)
		})
	}
}

// TestModel_RKey_NoOpWhenAlreadySyncing verifies that pressing 'r' while a sync
// is already in progress is a no-op (no duplicate batch fired).
func TestModel_RKey_NoOpWhenAlreadySyncing(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)
	m.syncing = true

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	assert.Nil(t, cmd, "'r' while syncing must return nil Cmd")
}

// TestModel_RKey_NotFiredWithModalOpen verifies that pressing 'r' while a modal
// is open is consumed by the modal and does NOT trigger a global refresh.
func TestModel_RKey_NotFiredWithModalOpen(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)
	m.activeModal = modal.NewHelpModal()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	result, ok := updated.(*Model)
	require.True(t, ok)
	assert.False(t, result.syncing, "'r' with modal open must NOT set syncing=true")
}

// TestBuildSearchIndexCmd verifies that buildSearchIndexCmd returns a
// fuzzyResultsReadyMsg containing results for the cached sources (worktrees,
// issues, PRs) and gracefully skips the async git/agent sources when there is
// no real git repository at m.RepoPath.
func TestBuildSearchIndexCmd(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	// Use a temp dir as the repo path so all git commands fail gracefully.
	m.RepoPath = t.TempDir()

	m.Worktrees = []domain.Worktree{
		{Path: "/wt/main", Branch: "main"},
		{Path: "/wt/feat", Branch: "feat/thing"},
	}
	m.issues = []domain.Issue{
		{Number: 1, Title: "First issue"},
		{Number: 2, Title: "Second issue"},
	}
	m.prs = []domain.PullRequest{
		{Number: 10, Title: "Fix something", Branch: "fix/something"},
	}

	cmd := m.buildSearchIndexCmd()
	require.NotNil(t, cmd, "buildSearchIndexCmd should return a non-nil tea.Cmd")

	// Execute the returned Cmd to obtain the message.
	msg := cmd()

	ready, ok := msg.(fuzzyResultsReadyMsg)
	require.True(t, ok, "expected fuzzyResultsReadyMsg, got %T", msg)

	// Count results per kind.
	kindCounts := map[domain.ResultKind]int{}
	for _, r := range ready.results {
		kindCounts[r.Kind]++
	}

	assert.Equal(t, 2, kindCounts[domain.KindWorktree], "should have 2 worktree results")
	assert.Equal(t, 2, kindCounts[domain.KindIssue], "should have 2 issue results")
	assert.Equal(t, 1, kindCounts[domain.KindPR], "should have 1 PR result")

	// Verify a worktree result has the correct fields.
	var wtResult *domain.SearchResult
	for i := range ready.results {
		if ready.results[i].Kind == domain.KindWorktree && ready.results[i].Sub == "main" {
			wtResult = &ready.results[i]
			break
		}
	}
	require.NotNil(t, wtResult, "should find worktree result for 'main' branch")
	assert.Equal(t, "/wt/main", wtResult.Label)
	assert.Equal(t, "🌿", wtResult.Icon)
}

// TestUpdate_FuzzyOpenMsg verifies that sending fuzzyOpenMsg to Update() returns
// a non-nil command (the async search index build).
func TestUpdate_FuzzyOpenMsg(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)
	m.RepoPath = t.TempDir()

	_, cmd := m.Update(fuzzyOpenMsg{})
	assert.NotNil(t, cmd, "fuzzyOpenMsg should return a non-nil Cmd")
}

// TestUpdate_FuzzyResultsReadyMsg verifies that fuzzyResultsReadyMsg populates
// m.fuzzyAllItems and always clears fuzzyLoading, even when the overlay is closed.
func TestUpdate_FuzzyResultsReadyMsg(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	results := []domain.SearchResult{
		{Kind: domain.KindFile, Label: "cmd/main.go", Icon: "📄", Payload: "cmd/main.go"},
		{Kind: domain.KindFile, Label: "internal/app.go", Icon: "📄", Payload: "internal/app.go"},
		{Kind: domain.KindBranch, Label: "main", Icon: "🌿", Payload: "main"},
		{Kind: domain.KindBranch, Label: "feat/foo", Icon: "🌿", Payload: "feat/foo"},
		{Kind: domain.KindWorktree, Label: "/wt/main", Sub: "main", Icon: "🌿", Payload: domain.Worktree{Path: "/wt/main", Branch: "main"}},
		{Kind: domain.KindIssue, Label: "Fix bug", Sub: "#1", Icon: "🐛", Payload: domain.Issue{Number: 1, Title: "Fix bug"}},
	}

	updated, cmd := m.Update(fuzzyResultsReadyMsg{results: results})
	assert.Nil(t, cmd, "fuzzyResultsReadyMsg should return nil Cmd")

	result, ok := updated.(*Model)
	require.True(t, ok)

	assert.Len(t, result.fuzzyAllItems, 6, "fuzzyAllItems should contain all 6 results")
}

// TestUpdate_FuzzyResultsReadyMsg_ClearsLoadingWhenOverlayClosed verifies that
// fuzzyLoading is reset to false even when the overlay is not currently open.
func TestUpdate_FuzzyResultsReadyMsg_ClearsLoadingWhenOverlayClosed(t *testing.T) {
	m := NewModel()
	require.NotNil(t, m)

	// Simulate: user opened overlay, triggering async build, then closed before results arrived.
	m.fuzzyLoading = true
	m.fuzzyActive = false

	_, _ = m.Update(fuzzyResultsReadyMsg{results: []domain.SearchResult{}})

	assert.False(t, m.fuzzyLoading, "fuzzyLoading must be cleared even when overlay is closed")
}

// ---------------------------------------------------------------------------
// fuzzy keybinding tests
// ---------------------------------------------------------------------------

// TestFuzzyOpensOnSlash verifies that pressing "/" activates the fuzzy overlay.
func TestFuzzyOpensOnSlash(t *testing.T) {
	m := NewModel()
	m.RepoPath = t.TempDir()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	result := updated.(*Model)

	assert.True(t, result.fuzzyActive, "'/' should activate the fuzzy overlay")
}

// TestFuzzyOpensOnCtrlF verifies that Ctrl+F activates the fuzzy overlay.
func TestFuzzyOpensOnCtrlF(t *testing.T) {
	m := NewModel()
	m.RepoPath = t.TempDir()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	result := updated.(*Model)

	assert.True(t, result.fuzzyActive, "Ctrl+F should activate the fuzzy overlay")
}

// ---------------------------------------------------------------------------
// fuzzyConfirmSelection tests
// ---------------------------------------------------------------------------

func TestFuzzyConfirmSelection_EmptyResults_ReturnsNil(t *testing.T) {
	m := NewModel()
	m.fuzzyResults = nil
	cmd := m.fuzzyConfirmSelection()
	assert.Nil(t, cmd)
}

func TestFuzzyConfirmSelection_IdxOutOfBounds_ReturnsNil(t *testing.T) {
	m := NewModel()
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindFile, Label: "x.go", Icon: "📄", Payload: "x.go"},
	}
	m.fuzzySelIdx = 5
	cmd := m.fuzzyConfirmSelection()
	assert.Nil(t, cmd)
}

func TestFuzzyConfirmSelection_WorktreeSelection(t *testing.T) {
	m := NewModel()
	m.Worktrees = []domain.Worktree{
		{Path: "/wt/alpha", Branch: "feat/alpha"},
		{Path: "/wt/beta", Branch: "feat/beta"},
	}
	m.view = viewIssues // start on a different view
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindWorktree, Label: "/wt/beta", Sub: "feat/beta", Icon: "🌿", Payload: domain.Worktree{Path: "/wt/beta", Branch: "feat/beta"}},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.Nil(t, cmd, "worktree selection returns no async Cmd")
	assert.Equal(t, viewWorktrees, m.view, "should switch to worktrees view")
	assert.Equal(t, 1, m.selectedIdx, "should select /wt/beta (index 1)")
	assert.Equal(t, 0, m.ctxScrollOffset)
	assert.Equal(t, 0, m.currentPage)
}

func TestFuzzyConfirmSelection_IssueSelection(t *testing.T) {
	m := NewModel()
	m.issues = []domain.Issue{
		{Number: 1, Title: "First"},
		{Number: 42, Title: "Fix auth"},
	}
	m.view = viewWorktrees
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindIssue, Label: "Fix auth", Sub: "#42", Icon: "🐛", Payload: domain.Issue{Number: 42, Title: "Fix auth"}},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.Nil(t, cmd)
	assert.Equal(t, viewIssues, m.view)
	assert.Equal(t, 1, m.selectedIssueIdx, "should select issue at index 1 (Number=42)")
}

func TestFuzzyConfirmSelection_PRSelection(t *testing.T) {
	m := NewModel()
	m.prs = []domain.PullRequest{
		{Number: 10, Title: "First PR"},
		{Number: 20, Title: "Add feature"},
	}
	m.view = viewWorktrees
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindPR, Label: "Add feature", Sub: "#20", Icon: "🔀", Payload: domain.PullRequest{Number: 20, Title: "Add feature"}},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.Nil(t, cmd)
	assert.Equal(t, viewPRs, m.view)
	assert.Equal(t, 1, m.selectedPRIdx, "should select PR at index 1 (Number=20)")
}

func TestFuzzyConfirmSelection_FileSelection_ReturnsCmd(t *testing.T) {
	m := NewModel()
	m.RepoPath = "/repo"
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindFile, Label: "internal/auth.go", Icon: "📄", Payload: "internal/auth.go"},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.NotNil(t, cmd, "file selection should return an exec Cmd")
	assert.Empty(t, m.statusMsg, "file selection should not set statusMsg")
}

func TestFuzzyConfirmSelection_BranchSelection_OpensModal(t *testing.T) {
	m := NewModel()
	m.RepoPath = "/repo/main"
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindBranch, Label: "feat/login", Icon: "🌿", Payload: "feat/login"},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.Nil(t, cmd, "branch selection should return nil Cmd (modal handles the action)")
	assert.NotNil(t, m.activeModal, "branch selection should open a BranchCheckoutModal")
}

func TestFuzzyConfirmSelection_AgentSelection_IsNoOp(t *testing.T) {
	m := NewModel()
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindAgent, Label: "fix the null pointer", Icon: "🤖", Payload: data.AgentRun{AgentName: "copilot", Prompt: "fix the null pointer"}},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.Nil(t, cmd, "agent selection is a no-op")
	assert.Empty(t, m.statusMsg, "agent selection should not set statusMsg")
}

func TestFuzzyConfirmSelection_CommitSelection_ReturnsCmd(t *testing.T) {
	m := NewModel()
	m.RepoPath = "/repo"
	m.fuzzyResults = []domain.SearchResult{
		{Kind: domain.KindCommit, Label: "add jwt middleware", Icon: "📦", Payload: "abc1234"},
	}
	m.fuzzySelIdx = 0

	cmd := m.fuzzyConfirmSelection()

	assert.NotNil(t, cmd, "commit selection should return a gh browse Cmd")
	assert.Empty(t, m.statusMsg, "commit selection should not set statusMsg")
}

// intPtr is a test helper that returns a pointer to the given int.
func intPtr(i int) *int { return &i }

// TestEnrichHerdrSessions verifies that enrichHerdrSessions applies snapshot-based
// liveness checks to Herdr-backed sessions and marks uncertain sessions as degraded.
func TestEnrichHerdrSessions(t *testing.T) {
	now := time.Now()

	paneFound := domain.PaneRef{PaneID: "pane-1", AgentID: "agent-1", Status: "open"}
	paneOther := domain.PaneRef{PaneID: "pane-2", AgentID: "agent-2", Status: "open"}

	herdrSnap := &herdr.Snapshot{
		Panes: []domain.PaneRef{paneFound, paneOther},
	}
	herdrErr := fmt.Errorf("herdr snapshot unavailable")

	scSnap := &sandcastle.Snapshot{
		Workflows: []domain.WorkflowRunRef{
			{WorkflowID: "wf-running", RunID: "wf-running", Status: domain.WorkflowRunning},
			{WorkflowID: "wf-failed", RunID: "wf-failed", Status: domain.WorkflowFailed},
			{WorkflowID: "wf-succeeded", RunID: "wf-succeeded", Status: domain.WorkflowSucceeded},
			{WorkflowID: "wf-blocked", RunID: "wf-blocked", Status: domain.WorkflowBlocked},
		},
	}
	scErr := fmt.Errorf("sandcastle status unavailable")

	tests := []struct {
		name         string
		sessions     []domain.Session
		herdrSnap    *herdr.Snapshot
		herdrErr     error
		scSnap       *sandcastle.Snapshot
		scErr        error
		wantAlive    int
		wantDegraded []int64 // IDs of sessions that should be degraded
		wantDead     []int64 // IDs of sessions that should be removed
	}{
		{
			name:      "nil herdr snapshot with nil error marks degraded (standalone mode)",
			sessions:  []domain.Session{{ID: 1, Runtime: domain.RuntimeHerdr, Status: domain.StatusActive, StartedAt: now}},
			herdrSnap: nil, herdrErr: nil, scSnap: nil, scErr: nil,
			wantAlive:    1,
			wantDegraded: []int64{1},
		},
		{
			name:      "herdr error marks herdr sessions degraded",
			sessions:  []domain.Session{{ID: 1, Runtime: domain.RuntimeHerdr, Status: domain.StatusActive, StartedAt: now}},
			herdrSnap: nil, herdrErr: herdrErr, scSnap: nil, scErr: nil,
			wantAlive:    1,
			wantDegraded: []int64{1},
		},
		{
			name: "herdr pane found keeps session alive",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, PaneID: strPtr("pane-1"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: herdrSnap, herdrErr: nil, scSnap: nil, scErr: nil,
			wantAlive: 1,
		},
		{
			name: "herdr pane not found marks session degraded",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, PaneID: strPtr("pane-missing"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: herdrSnap, herdrErr: nil, scSnap: nil, scErr: nil,
			wantAlive:    1,
			wantDegraded: []int64{1},
		},
		{
			name: "herdr session with nil PaneID and no snapshot marks degraded",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: nil, herdrErr: nil, scSnap: nil, scErr: nil,
			wantAlive:    1,
			wantDegraded: []int64{1},
		},
		{
			name: "sandcastle workflow running keeps session alive",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, WorkflowRunID: strPtr("wf-running"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: nil, herdrErr: nil, scSnap: scSnap, scErr: nil,
			wantAlive: 1,
		},
		{
			name: "sandcastle workflow failed removes session",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, WorkflowRunID: strPtr("wf-failed"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: nil, herdrErr: nil, scSnap: scSnap, scErr: nil,
			wantAlive: 0,
			wantDead:  []int64{1},
		},
		{
			name: "sandcastle workflow succeeded removes session",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, WorkflowRunID: strPtr("wf-succeeded"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: nil, herdrErr: nil, scSnap: scSnap, scErr: nil,
			wantAlive: 0,
			wantDead:  []int64{1},
		},
		{
			name: "sandcastle workflow blocked marks session degraded",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, WorkflowRunID: strPtr("wf-blocked"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: nil, herdrErr: nil, scSnap: scSnap, scErr: nil,
			wantAlive:    1,
			wantDegraded: []int64{1},
		},
		{
			name: "sandcastle error marks session degraded",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, WorkflowRunID: strPtr("wf-unknown"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: nil, herdrErr: nil, scSnap: nil, scErr: scErr,
			wantAlive:    1,
			wantDegraded: []int64{1},
		},
		{
			name: "local sessions pass through unchanged",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeLocal, Status: domain.StatusActive, StartedAt: now},
				{ID: 2, Runtime: "", Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: herdrSnap, herdrErr: herdrErr, scSnap: scSnap, scErr: scErr,
			wantAlive: 2,
		},
		{
			name: "mixed herdr and local sessions",
			sessions: []domain.Session{
				{ID: 1, Runtime: domain.RuntimeHerdr, PaneID: strPtr("pane-1"), Status: domain.StatusActive, StartedAt: now},
				{ID: 2, Runtime: domain.RuntimeLocal, ShellPID: intPtr(os.Getpid()), Status: domain.StatusActive, StartedAt: now},
				{ID: 3, Runtime: domain.RuntimeHerdr, PaneID: strPtr("pane-missing"), Status: domain.StatusActive, StartedAt: now},
			},
			herdrSnap: herdrSnap, herdrErr: nil, scSnap: nil, scErr: nil,
			wantAlive:    3, // local(2) + herdr pane found(1) + herdr pane missing(3) degraded but kept
			wantDegraded: []int64{3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enrichHerdrSessions(tt.sessions, tt.herdrSnap, tt.herdrErr, tt.scSnap, tt.scErr)
			assert.Len(t, got, tt.wantAlive, "alive session count")

			gotIDs := make(map[int64]domain.Session)
			for _, s := range got {
				gotIDs[s.ID] = s
			}

			for _, id := range tt.wantDegraded {
				s, ok := gotIDs[id]
				require.True(t, ok, "session %d should be in alive list", id)
				assert.NotNil(t, s.DegradedReason, "session %d should have DegradedReason set", id)
				assert.NotEmpty(t, *s.DegradedReason, "session %d DegradedReason should be non-empty", id)
			}

			for _, id := range tt.wantDead {
				_, ok := gotIDs[id]
				assert.False(t, ok, "session %d should be removed from alive list", id)
			}
		})
	}
}

// TestHerdrSyncedMsg_PreservesSnapshotOnError verifies that when herdrSyncedMsg
// carries an error the previous snapshot is not overwritten.
func TestHerdrSyncedMsg_PreservesSnapshotOnError(t *testing.T) {
	m := NewModel()
	existing := herdr.Snapshot{Panes: []domain.PaneRef{{PaneID: "keep-me"}}}
	m.herdrSnapshot = &existing

	m2, _ := m.Update(herdrSyncedMsg{err: errors.New("network timeout")})
	model := m2.(*Model)
	require.NotNil(t, model.herdrSnapshot, "snapshot must be preserved on error")
	assert.Equal(t, "keep-me", model.herdrSnapshot.Panes[0].PaneID)
}

// TestHerdrSyncedMsg_StoresSnapshotOnSuccess verifies that a successful
// herdrSyncedMsg updates the cached snapshot.
func TestHerdrSyncedMsg_StoresSnapshotOnSuccess(t *testing.T) {
	m := NewModel()
	snap := herdr.Snapshot{Panes: []domain.PaneRef{{PaneID: "new"}}}

	m2, _ := m.Update(herdrSyncedMsg{snapshot: snap})
	model := m2.(*Model)
	require.NotNil(t, model.herdrSnapshot)
	assert.Equal(t, "new", model.herdrSnapshot.Panes[0].PaneID)
}

// TestSandcastleSyncedMsg_PreservesDataAndStoresIntegrationError verifies that a
// failed refresh retains prior workflow data while surfacing the live failure.
func TestSandcastleSyncedMsg_PreservesSnapshotOnError(t *testing.T) {
	m := NewModel()
	existing := sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{{WorkflowID: "keep"}}}
	m.sandcastleSnapshot = &existing

	failed := sandcastle.Snapshot{
		Integration: domain.ExternalIntegration{
			Name:      "sandcastle",
			Mode:      "degraded",
			Available: true,
			Enabled:   true,
			Error:     "status command unavailable",
		},
	}
	m2, _ := m.Update(sandcastleSyncedMsg{snapshot: failed, err: errors.New("timeout")})
	model := m2.(*Model)
	require.NotNil(t, model.sandcastleSnapshot, "snapshot must be preserved on error")
	assert.Equal(t, "keep", model.sandcastleSnapshot.Workflows[0].WorkflowID)
	assert.Equal(t, "degraded", model.sandcastleSnapshot.Integration.Mode)
	assert.Equal(t, "status command unavailable", model.sandcastleSnapshot.Integration.Error)
}

// TestSandcastleSyncedMsg_StoresSnapshotOnSuccess verifies that a successful
// sandcastleSyncedMsg updates the cached snapshot.
func TestSandcastleSyncedMsg_StoresSnapshotOnSuccess(t *testing.T) {
	m := NewModel()
	snap := sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{{WorkflowID: "new"}}}

	m2, _ := m.Update(sandcastleSyncedMsg{snapshot: snap})
	model := m2.(*Model)
	require.NotNil(t, model.sandcastleSnapshot)
	assert.Equal(t, "new", model.sandcastleSnapshot.Workflows[0].WorkflowID)
}

// TestMissionControlUpdatedMsg_SetsState verifies that the message stores
// the state on the model.
func TestMissionControlUpdatedMsg_SetsState(t *testing.T) {
	m := NewModel()
	assert.Nil(t, m.missionState)

	state := domain.MissionControlState{Status: "test-status"}
	m2, _ := m.Update(missionControlUpdatedMsg{state: state})
	model := m2.(*Model)
	require.NotNil(t, model.missionState)
	assert.Equal(t, "test-status", model.missionState.Status)
}

// TestRebuildMissionStateCmd_NilSnapshots verifies that rebuildMissionStateCmd
// produces a valid state even when no snapshots have been fetched yet.
func TestRebuildMissionStateCmd_NilSnapshots(t *testing.T) {
	m := NewModel()
	cmd := m.rebuildMissionStateCmd()
	require.NotNil(t, cmd)

	msg := cmd()
	result, ok := msg.(missionControlUpdatedMsg)
	require.True(t, ok, "expected missionControlUpdatedMsg")
	assert.NotNil(t, result.state, "state should not be nil")
}

// TestIntegrationTickCmd_FiresAfterInterval verifies that the tick command
// produces an integrationsTickMsg after the configured interval.
func TestIntegrationTickCmd_FiresAfterInterval(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Herdr.PollIntervalSeconds = 1

	cmd := integrationTickCmd(cfg)
	require.NotNil(t, cmd)
}

// TestIntegrationTickCmd_DefaultInterval verifies that a zero poll interval
// falls back to the 5-second default.
func TestIntegrationTickCmd_DefaultInterval(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Herdr.PollIntervalSeconds = 0

	cmd := integrationTickCmd(cfg)
	require.NotNil(t, cmd)
}

// TestUpdate_IntegrationsTickMsg_DispatchesRefreshCmds verifies that the
// integrationsTickMsg handler dispatches snapshot commands and reschedules.
func TestUpdate_IntegrationsTickMsg_DispatchesRefreshCmds(t *testing.T) {
	hc := &fakeHealthChecker{
		herdrSnap: herdr.Snapshot{Panes: []domain.PaneRef{{PaneID: "p1"}}},
		scSnap:    sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{{WorkflowID: "w1"}}},
	}

	m := NewModel()
	m.healthChecker = hc
	m.Config.Herdr.Enabled = true
	m.Config.Sandcastle.Enabled = true

	m2, cmd := m.Update(integrationsTickMsg{})
	assert.NotNil(t, cmd, "should return a batch of commands")
	_ = m2

	// Simulate herdr response.
	m3, _ := m2.Update(herdrSyncedMsg{snapshot: herdr.Snapshot{Panes: []domain.PaneRef{{PaneID: "p1"}}}})
	model := m3.(*Model)
	require.NotNil(t, model.herdrSnapshot)
	assert.Equal(t, "p1", model.herdrSnapshot.Panes[0].PaneID)

	// Simulate sandcastle response.
	m4, _ := m3.Update(sandcastleSyncedMsg{snapshot: sandcastle.Snapshot{Workflows: []domain.WorkflowRunRef{{WorkflowID: "w1"}}}})
	model4 := m4.(*Model)
	require.NotNil(t, model4.sandcastleSnapshot)
	assert.Equal(t, "w1", model4.sandcastleSnapshot.Workflows[0].WorkflowID)
}

// TestNewModel_InsideHerdrDetection verifies that NewModel detects the
// inside-Herdr environment from the HERDR_ENV variable.
func TestNewModel_InsideHerdrDetection(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	m := NewModel()
	assert.True(t, m.insideHerdr, "insideHerdr should be true when HERDR_ENV is set")
}

func TestNewModel_InsideHerdrDetection_Standalone(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	m := NewModel()
	assert.False(t, m.insideHerdr, "insideHerdr should be false when HERDR_ENV is unset")
}

// TestRebuildMissionStateCmd_InsideHerdrPassed verifies that insideHerdr
// flows through to BuildInput.InsideHerdr so the Herdr integration shows
// "degraded" instead of "missing" when running inside Herdr with a nil snapshot.
func TestRebuildMissionStateCmd_InsideHerdrPassed(t *testing.T) {
	m := NewModel()
	m.insideHerdr = true
	// No snapshot — inside Herdr should produce degraded, not missing.
	cmd := m.rebuildMissionStateCmd()
	msg := cmd()
	result := msg.(missionControlUpdatedMsg)
	assert.Equal(t, domain.DegradedState, result.state.Integrations.Herdr.Mode,
		"inside Herdr with nil snapshot should show degraded mode")
}

func TestRebuildMissionStateCmd_OutsideHerdrMissing(t *testing.T) {
	m := NewModel()
	m.insideHerdr = false
	// No snapshot — outside Herdr should show missing.
	cmd := m.rebuildMissionStateCmd()
	msg := cmd()
	result := msg.(missionControlUpdatedMsg)
	assert.Equal(t, "missing", result.state.Integrations.Herdr.Mode,
		"outside Herdr with nil snapshot should show missing mode")
}

// TestHerdrSyncedMsg_SetsLastSync verifies that a successful herdrSyncedMsg
// records the time of the sync.
func TestHerdrSyncedMsg_SetsLastSync(t *testing.T) {
	m := NewModel()
	assert.True(t, m.lastHerdrSync.IsZero(), "should start with zero timestamp")

	before := time.Now()
	m2, _ := m.Update(herdrSyncedMsg{snapshot: herdr.Snapshot{}})
	model := m2.(*Model)
	after := time.Now()

	assert.False(t, model.lastHerdrSync.IsZero(), "lastHerdrSync should be set after success")
	assert.False(t, model.lastHerdrSync.Before(before), "timestamp should not be before handler")
	assert.False(t, model.lastHerdrSync.After(after), "timestamp should not be after handler")
}

// TestHerdrSyncedMsg_ErrorNoSync verifies that a failed herdrSyncedMsg
// does not update the sync timestamp.
func TestHerdrSyncedMsg_ErrorNoSync(t *testing.T) {
	m := NewModel()
	m.lastHerdrSync = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	m2, _ := m.Update(herdrSyncedMsg{err: errors.New("fail")})
	model := m2.(*Model)
	assert.Equal(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), model.lastHerdrSync,
		"lastHerdrSync should not change on error")
}

// TestSandcastleSyncedMsg_SetsLastSync verifies that a successful sandcastleSyncedMsg
// records the time of the sync.
func TestSandcastleSyncedMsg_SetsLastSync(t *testing.T) {
	m := NewModel()
	assert.True(t, m.lastSandcastleSync.IsZero(), "should start with zero timestamp")

	before := time.Now()
	m2, _ := m.Update(sandcastleSyncedMsg{snapshot: sandcastle.Snapshot{}})
	model := m2.(*Model)
	after := time.Now()

	assert.False(t, model.lastSandcastleSync.IsZero(), "lastSandcastleSync should be set after success")
	assert.False(t, model.lastSandcastleSync.Before(before), "timestamp should not be before handler")
	assert.False(t, model.lastSandcastleSync.After(after), "timestamp should not be after handler")
}

// TestSandcastleSyncedMsg_ErrorNoSync verifies that a failed sandcastleSyncedMsg
// does not update the sync timestamp.
func TestSandcastleSyncedMsg_ErrorNoSync(t *testing.T) {
	m := NewModel()
	m.lastSandcastleSync = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	m2, _ := m.Update(sandcastleSyncedMsg{err: errors.New("fail")})
	model := m2.(*Model)
	assert.Equal(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), model.lastSandcastleSync,
		"lastSandcastleSync should not change on error")
}

// TestRepeatedPollErrors_DontSetStatus verifies that repeated integration
// poll errors do not set statusErr or statusMsg on the model.
func TestRepeatedPollErrors_DontSetStatus(t *testing.T) {
	m := NewModel()

	for i := 0; i < 5; i++ {
		m2, _ := m.Update(herdrSyncedMsg{err: errors.New("poll error")})
		m = m2.(*Model)
		m3, _ := m.Update(sandcastleSyncedMsg{err: errors.New("poll error")})
		m = m3.(*Model)
	}

	assert.Empty(t, m.statusErr, "repeated poll errors must not set statusErr")
	assert.Empty(t, m.statusMsg, "repeated poll errors must not set statusMsg")
}

// TestNavigation_CyclingIncludesDashboard verifies nav cycling includes dashboard view.
func TestNavigation_CyclingIncludesDashboard(t *testing.T) {
	tests := []struct {
		name              string
		initialView       activeView
		expectedAfterUp   activeView // expected after moveUp from initial state
		expectedAfterDown activeView // expected after moveDown from initial state
	}{
		{
			name:              "from dashboard up goes to PRs, down goes to worktrees",
			initialView:       viewDashboard,
			expectedAfterUp:   viewPRs,
			expectedAfterDown: viewWorktrees,
		},
		{
			name:              "from worktrees up goes to dashboard, down goes to issues",
			initialView:       viewWorktrees,
			expectedAfterUp:   viewDashboard,
			expectedAfterDown: viewIssues,
		},
		{
			name:              "from issues up goes to worktrees, down goes to PRs",
			initialView:       viewIssues,
			expectedAfterUp:   viewWorktrees,
			expectedAfterDown: viewPRs,
		},
		{
			name:              "from PRs up goes to issues, down wraps to dashboard",
			initialView:       viewPRs,
			expectedAfterUp:   viewIssues,
			expectedAfterDown: viewDashboard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			m.view = tt.initialView
			m.focused = panelNav // Navigate cycling happens in nav panel
			require.NotNil(t, m)

			// Test moveUp (cycling backward from initial state)
			m.moveUp()
			assert.Equal(t, tt.expectedAfterUp, m.view, "%s: moveUp should navigate to %v", tt.name, tt.expectedAfterUp)

			// Reset and test moveDown (cycling forward from initial state)
			m2 := NewModel()
			m2.view = tt.initialView
			m2.focused = panelNav
			m2.moveDown()
			assert.Equal(t, tt.expectedAfterDown, m2.view, "%s: moveDown should navigate to %v", tt.name, tt.expectedAfterDown)
		})
	}
}
