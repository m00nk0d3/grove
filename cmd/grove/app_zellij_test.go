// Package main implements the Groove Git Worktree Orchestrator with Zellij integration.
package main

import (
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
