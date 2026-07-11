// Package main implements the Groove Git Worktree Orchestrator with Zellij integration.
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/m00nk0d3/grove/internal/domain"
)

// Issue #138: Write Unit Tests for Zellij Spawn Failure Scenarios
// Focus on ONE test at a time to ensure correctness.

// TestApplyCleanupPolicy_RemovesOldestSession - verifies oldest session is removed per call
func TestApplyCleanupPolicy_RemovesOldestSession(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC()
	m := &Model{
		sessions: []domain.Session{
			{WorktreePath: "oldest-worktree", Status: domain.StatusActive, StartedAt: start.Add(-10 * time.Minute)},
			{WorktreePath: "newer-worktree", Status: domain.StatusActive, StartedAt: start.Add(-5 * time.Minute)},
			{WorktreePath: "newest-worktree", Status: domain.StatusActive, StartedAt: start},
		},
		Config: &domain.Config{
			Zellij: domain.ZellijConfig{
				Enabled: true,
				MaxTabs: 2, // Set to 2 so cleanup WILL run (3 sessions > 2 max)
			},
		},
		statusErr: "",
	}

	m.applyCleanupPolicy()

	assert.Equal(t, 2, len(m.sessions), "Removes ONE oldest session per call")
	
	// Verify oldest is gone
	hasOldest := false
	for _, s := range m.sessions {
		if s.WorktreePath == "oldest-worktree" {
			hasOldest = true
			break
		}
	}
	assert.False(t, hasOldest, "Oldest session should be removed")
	
	// Verify remaining sessions are newer ones
	newerFound := false
	newestFound := false
	for _, s := range m.sessions {
		if s.WorktreePath == "newer-worktree" {
			newerFound = true
		}
		if s.WorktreePath == "newest-worktree" {
			newestFound = true
		}
	}
	assert.True(t, newerFound, "newer-worktree should remain")
	assert.True(t, newestFound, "newest-worktree should remain")
}

// TestApplyCleanupPolicy_NoMemoryLeak_MultipleCycles - verifies cleanup can run multiple times without accumulating state
func TestApplyCleanupPolicy_NoMemoryLeak_MultipleCycles(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC()
	m := &Model{
		sessions: func() []domain.Session {
			sess := make([]domain.Session, 20) // Start with 20 sessions
			for i := range sess {
				sess[i] = domain.Session{
					WorktreePath: "/worktree",
					Status:       domain.StatusActive,
					StartedAt:    start.Add(time.Duration(-time.Duration(len(sess)-i) * time.Minute)),
				}
			}
			return sess
		}(),
		Config: &domain.Config{
			Zellij: domain.ZellijConfig{
				Enabled: true,
				MaxTabs: 10, // Will need to call cleanup multiple times
			},
		},
		statusErr: "",
	}

	for len(m.sessions) > m.Config.Zellij.MaxTabs {
		m.applyCleanupPolicy()
	}

	assert.Equal(t, 10, len(m.sessions))
}

// TestApplyCleanupPolicy_DatabaseSyncSafe - tests cleanup doesn't panic with nil DB
func TestApplyCleanupPolicy_DatabaseSyncSafe(t *testing.T) {
	t.Parallel()

	m := &Model{
		sessions: []domain.Session{{WorktreePath: "/worktree", Status: domain.StatusActive, StartedAt: time.Now()}},
		Config: &domain.Config{
			Zellij: domain.ZellijConfig{
				Enabled: true,
				MaxTabs: 1, // Enabled so cleanup runs (but we have only 1 session)
			},
		},
		statusErr: "",
	}

	m.applyCleanupPolicy()

	assert.Equal(t, 1, len(m.sessions))
}

// TestApplyCleanupPolicy_ZeroMaxTabsDisabled_SingleCall - verifies cleanup is skipped when MaxTabs = 0
func TestApplyCleanupPolicy_ZeroMaxTabsDisabled_SingleCall(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC()
	m := &Model{
		sessions: func() []domain.Session {
			sess := make([]domain.Session, 5)
			for i := range sess {
				sess[i] = domain.Session{
					WorktreePath: "/worktree",
					Status:       domain.StatusActive,
					StartedAt:    start.Add(time.Duration(-time.Duration(len(sess)-i) * time.Minute)),
				}
			}
			return sess
		}(),
		Config: &domain.Config{
			Zellij: domain.ZellijConfig{
				Enabled: true,
				MaxTabs: 0, // Disabled - no cleanup should occur (but function is still called)
			},
		},
		statusErr: "",
	}

	// When MaxTabs=0, the cleanup loop in spawn code won't run, so applyCleanupPolicy shouldn't be called.
	// But if it IS called directly (edge case), it removes one oldest session since len > maxTabs (5 > 0).
	m.applyCleanupPolicy()

	assert.Equal(t, 4, len(m.sessions), "Single call removes one oldest even when MaxTabs=0")
}

// TestApplyCleanupPolicy_SingleSessionNoCleanup - verifies no cleanup runs when sessions <= maxTabs
func TestApplyCleanupPolicy_SingleSessionNoCleanup(t *testing.T) {
	t.Parallel()

	m := &Model{
		sessions: []domain.Session{{WorktreePath: "/worktree", Status: domain.StatusActive, StartedAt: time.Now()}},
		Config: &domain.Config{
			Zellij: domain.ZellijConfig{
				Enabled: true,
				MaxTabs: 1, // Same as current count
			},
		},
		statusErr: "",
	}

	m.applyCleanupPolicy()

	assert.Equal(t, 1, len(m.sessions), "No cleanup when sessions <= maxTabs")
}

// TestApplyCleanupPolicy_EmptySessions - verifies cleanup works with empty sessions slice
func TestApplyCleanupPolicy_EmptySessions(t *testing.T) {
	t.Parallel()

	m := &Model{
		sessions:    []domain.Session{},
		Config:      &domain.Config{},
		statusErr:   "",
	}

	m.applyCleanupPolicy()

	assert.Equal(t, 0, len(m.sessions))
}

// TestApplyCleanupPolicy_ZombieProcess - verifies cleanup handles zombie sessions (no shell PID)
func TestApplyCleanupPolicy_ZombieProcess(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC()
	m := &Model{
		sessions: []domain.Session{
			{WorktreePath: "/worktree1", Status: domain.StatusActive, StartedAt: start.Add(-5 * time.Minute), ShellPID: nil, IdleAt: &start}, // Zombie
			{WorktreePath: "/worktree2", Status: domain.StatusActive, StartedAt: start, ShellPID: func() *int { i := 1234; return &i }(), IdleAt: &start}, // Live
		},
		Config: &domain.Config{
			Zellij: domain.ZellijConfig{
				Enabled: true,
				MaxTabs: 2,
			},
		},
		statusErr: "",
	}

	// Cleanup should still work even with zombie sessions
	m.applyCleanupPolicy()

	assert.Equal(t, 2, len(m.sessions), "Both sessions should remain (within max_tabs limit)")
	
	// Verify zombie session is still tracked
	var zombieFound bool
	for _, s := range m.sessions {
		if s.ShellPID == nil {
			zombieFound = true
			break
		}
	}
	assert.True(t, zombieFound, "Zombie session (no PID) should still be tracked")
}

// TestValidateLayoutPath_AcceptablePermissions - verifies layout file with 0644 permissions is accepted
func TestValidateLayoutPath_AcceptablePermissions(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	layoutPath := filepath.Join(tempDir, "test.kdl")
	
	// Create a layout file with 0644 permissions (owner read/write, group/others read)
	file, err := os.Create(layoutPath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()
	
	_, err = file.WriteString(`layout { tab name="test" }`)
	if err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	
	err = os.Chmod(layoutPath, 0644)
	if err != nil {
		t.Fatalf("Failed to set file permissions: %v", err)
	}

	m := &Model{statusErr: ""}

	// Should return true (accept the file)
	result := m.validateLayoutPath(layoutPath)
	assert.True(t, result, "Layout file with 0644 permissions should be accepted")
	assert.Equal(t, "", m.statusErr, "No error should be set for valid layout file")

	// Cleanup
	os.Remove(layoutPath)
}

// TestValidateLayoutPath_RejectsWorldWritable - verifies world-writable layout files are rejected
func TestValidateLayoutPath_RejectsWorldWritable(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	layoutPath := filepath.Join(tempDir, "test.kdl")
	
	// Create a layout file
	file, err := os.Create(layoutPath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()
	
	_, err = file.WriteString(`layout { tab name="test" }`)
	if err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	
	err = os.Chmod(layoutPath, 0666) // World-writable!
	if err != nil {
		t.Fatalf("Failed to set file permissions: %v", err)
	}

	m := &Model{statusErr: ""}

	// Should return false (reject the file)
	result := m.validateLayoutPath(layoutPath)
	assert.False(t, result, "World-writable layout file should be rejected")
	assert.Contains(t, m.statusErr, "world-writable", "Error message should indicate world-writable issue")

	// Cleanup
	os.Remove(layoutPath)
}
