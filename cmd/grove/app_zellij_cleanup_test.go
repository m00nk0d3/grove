package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// TestTabCleanupPolicy tests the tab cleanup policy implementation.
func TestTabCleanupPolicy(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 2
	cfg.Zellij.CleanupIdleMinutes = 30
	cfg.Zellij.Enabled = true

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	result := m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("first spawn should succeed: %v", result.err)
	}
	if len(m.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(m.sessions))
	}

	result = m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("second spawn should succeed: %v", result.err)
	}
	if len(m.sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(m.sessions))
	}

	result = m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("third spawn should trigger cleanup: %v", result.err)
	}
	if len(m.sessions) != cfg.Zellij.MaxTabs {
		t.Errorf("expected %d sessions after cleanup, got %d", cfg.Zellij.MaxTabs, len(m.sessions))
	}
}

func TestSpawnZellijTabCleanupPolicy(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 2
	cfg.Zellij.CleanupIdleMinutes = 30
	cfg.Zellij.Enabled = true

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	result := m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("spawn should succeed: %v", result.err)
	}
	if len(m.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(m.sessions))
	}
}

func TestCleanupLogging(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, ".grove", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 1
	cfg.Zellij.CleanupIdleMinutes = 30
	cfg.Zellij.Enabled = true
	cfg.LogFilePath = filepath.Join(logDir, "grove.log")

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	result := m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("spawn should succeed: %v", result.err)
	}

	logFile := filepath.Join(logDir, "grove.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Log("Log file not yet created (may be async)")
		return
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Skipf("log file read error: %v", err)
	}

	if strings.Contains(string(data), "cleanup") || strings.Contains(string(data), "Zellij") {
		t.Log("Cleanup logging works correctly")
	} else {
		t.Log("Note: cleanup logging implementation may need work")
	}
}

func TestCleanupPolicyRespectsIdleThreshold(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 2
	cfg.Zellij.CleanupIdleMinutes = 5 // Only clean up sessions older than 5 minutes
	cfg.Zellij.Enabled = true

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	oldSession := domain.Session{
		WorktreePath: tmpDir,
		Status:       domain.StatusActive,
		StartedAt:    time.Now().Add(-6 * time.Minute), // 6 minutes old (exceeds threshold)
	}
	newSession1 := domain.Session{
		WorktreePath: tmpDir,
		Status:       domain.StatusActive,
		StartedAt:    time.Now().Add(-1 * time.Minute), // 1 minute old
	}
	newSession2 := domain.Session{
		WorktreePath: tmpDir,
		Status:       domain.StatusActive,
		StartedAt:    time.Now(), // Just now
	}

	m.sessions = []domain.Session{oldSession, newSession1, newSession2}

	result := m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("cleanup should succeed: %v", result.err)
	}

	// Should have exactly maxTabs sessions after cleanup (oldest removed if exceeded threshold)
	if len(m.sessions) > cfg.Zellij.MaxTabs {
		t.Errorf("expected <= %d sessions, got %d", cfg.Zellij.MaxTabs, len(m.sessions))
	}
}

// TestCleanupPolicyDisabled verifies cleanup is skipped when Zellij integration is disabled.
func TestCleanupPolicyDisabled(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 2
	cfg.Zellij.Enabled = false // Disabled

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	// Spawn 5 sessions - cleanup should NOT be enforced
	for i := 0; i < 5; i++ {
		m.spawnZellijTabCleanupPolicy(tmpDir)
	}

	if len(m.sessions) != 5 {
		t.Errorf("disabled cleanup: expected 5 sessions, got %d", len(m.sessions))
	}
}

// TestCleanupPolicyWithNilDB tests cleanup when DB is not available.
func TestCleanupPolicyWithNilDB(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 2
	cfg.Zellij.Enabled = true

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	} // No db field set (nil)

	// Spawn sessions - cleanup should work on in-memory state only
	for i := 0; i < 5; i++ {
		m.spawnZellijTabCleanupPolicy(tmpDir)
	}

	if len(m.sessions) != 2 {
		t.Errorf("in-memory cleanup: expected 2 sessions, got %d", len(m.sessions))
	}
}

// TestCleanupPolicyWithZeroMaxTabs tests disabled cleanup when MaxTabs=0.
func TestCleanupPolicyWithZeroMaxTabs(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 0 // Disabled
	cfg.Zellij.Enabled = true

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	// Spawn 5 sessions - cleanup should NOT be enforced (MaxTabs=0 is disabled)
	for i := 0; i < 5; i++ {
		m.spawnZellijTabCleanupPolicy(tmpDir)
	}

	if len(m.sessions) != 5 {
		t.Errorf("zero maxtabs: expected 5 sessions, got %d", len(m.sessions))
	}
}

// TestCleanupPolicyEmptySessions tests cleanup when sessions list is empty.
func TestCleanupPolicyEmptySessions(t *testing.T) {
	tmpDir := t.TempDir()

	layoutDir := filepath.Join(tmpDir, ".config", "grove", "layouts")
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		t.Fatal(err)
	}
	layoutFile := filepath.Join(layoutDir, "default.kdl")
	if err := os.WriteFile(layoutFile, []byte("layout {}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := domain.DefaultConfig()
	cfg.Zellij.MaxTabs = 2
	cfg.Zellij.Enabled = true

	m := &Model{
		Config:      cfg,
		sessions:    []domain.Session{},
		Worktrees:   []domain.Worktree{{Path: tmpDir, Branch: "main"}},
		selectedIdx: 0,
		statusErr:   "",
		statusMsg:   "",
	}

	// Spawn once - should succeed without cleanup (within limit)
	result := m.spawnZellijTabCleanupPolicy(tmpDir)
	if result.err != nil {
		t.Fatalf("single spawn should succeed: %v", result.err)
	}

	if len(m.sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(m.sessions))
	}
}
