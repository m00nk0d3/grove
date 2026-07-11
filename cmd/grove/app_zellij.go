// Package main implements the Groove Git Worktree Orchestrator with Zellij integration.

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/data"
	"github.com/m00nk0d3/grove/internal/domain"
)

// spawnZellijTabCmd spawns a new Zellij tab for worktree switching if zellij is available.
// If zellij binary is not found or layout file is missing/unreadable, falls back to plain shell session.
func (m *Model) spawnZellijTabCmd(worktreePath string) tea.Cmd {
	return func() tea.Msg {
		// 1. Check if zellij binary exists
		if _, err := exec.LookPath("zellij"); err != nil {
			// Fallback: just spawn plain shell session
			return m.spawnSessionCmd(worktreePath)
		}

		// 2. Load layout from external config file or use inline default
		layoutPath, err := m.getLayoutPath()
		if err != nil || !fileExists(layoutPath) {
			// Use inline default layout if file missing
			layoutPath = m.getDefaultLayout()
		} else if !m.validateLayoutPath(layoutPath) {
			// Fallback to inline default on permission failure
			layoutPath = m.getDefaultLayout()
		}

		// 3. Build spawn command with worktree path in tab name
		worktreeName := filepath.Base(worktreePath)
		spawnCmd := exec.Command("zellij", "action", "new-tab", "--layout", layoutPath, "--name", fmt.Sprintf("Grove-%s", worktreeName))

		// 4. Spawn the command asynchronously (don't block TUI)
		go func() {
			_, err = spawnCmd.CombinedOutput()
			if err != nil {
				// Log failure silently — worktree switch still succeeds
				m.statusErr = fmt.Sprintf(`Zellij tab spawn failed: %v

Tip: Ensure zellij is installed and ~/.config/grove/layouts/default.kdl exists`, err)
			}
		}()

			newSession := domain.Session{
				WorktreePath: worktreePath,
				Status:       domain.StatusActive,
				StartedAt:    time.Now().UTC().Truncate(time.Second),
			}
			m.sessions = append(m.sessions, newSession)
			return sessionSpawnedMsg{session: newSession}
	}
}

// getLayoutPath returns the path to external layout configuration file.
func (m *Model) getLayoutPath() (string, error) {
	// Default location for user layouts
	layoutDir := filepath.Join(os.Getenv("HOME"), ".config", "grove", "layouts")
	if !dirExists(layoutDir) {
		return "", fmt.Errorf("layout directory does not exist: %s", layoutDir)
	}
	return filepath.Join(layoutDir, "default.kdl"), nil
}

// getDefaultLayout returns inline default layout for fallback use.
func (m *Model) getDefaultLayout() string {
	return `layout {
    tab name="Grove (%s)" focus=true {
        pane split_direction="vertical" {
            // Left: Workspace shell or neovim
            pane command="bash" name="[Workspace]" size="70%"

            // Right: Grove CLI context
            pane command="grove -c" name="[Grove Context]" size="30%"
        }
    }
}`
}

// validateLayoutPath checks that the layout file is readable by owner only (security hardening).
func (m *Model) validateLayoutPath(layoutPath string) bool {
	info, err := os.Stat(layoutPath)
	if err != nil {
		return false // Can't stat file, fallback to inline default
	}
	// Regular file AND no world-readable bits to prevent unauthorized access
	if info.Mode().IsRegular() == false {
		m.statusErr = fmt.Sprintf("Layout file is not a regular file: %s", layoutPath)
		return false
	}
	if info.Mode().Perm()&0444 != 0 {
		log.Printf("Warning: Layout file is world-readable: %s", layoutPath)
		return false
	}
	return true
}

// fileExists checks if a file exists (non-blocking).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// dirExists checks if a directory exists (non-blocking).
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// spawnZellijTabCleanupPolicy implements the tab cleanup policy.
// It spawns a Zellij tab but enforces max_tabs limit by cleaning up oldest tabs first.
// Returns sessionSpawnedMsg with the new session.
func (m *Model) spawnZellijTabCleanupPolicy(worktreePath string) sessionSpawnedMsg {
	// Check if zellij is available
	if _, err := exec.LookPath("zellij"); err != nil {
		// Fallback: return shell session directly (no async tracking needed for fallback)
		// Fallback: create shell session and add to tracking list
		newSession := domain.Session{
			WorktreePath: worktreePath,
			Status:       domain.StatusActive,
			StartedAt:    time.Now().UTC().Truncate(time.Second),
		}
		m.sessions = append(m.sessions, newSession)
		return sessionSpawnedMsg{session: newSession}
	}

	// Check layout file and get path (inline default if missing/unreadable)
	layoutPath, err := m.getLayoutPath()
	if err != nil || !fileExists(layoutPath) {
		layoutPath = m.getDefaultLayout()
	} else if !m.validateLayoutPath(layoutPath) {
		layoutPath = m.getDefaultLayout()
	}

	// Build spawn command
	worktreeName := filepath.Base(worktreePath)
	spawnCmd := exec.Command("zellij", "action", "new-tab", "--layout", layoutPath, "--name", fmt.Sprintf("Grove-%s", worktreeName))

	// Spawn asynchronously (don't block TUI)
	go func() {
		_, err = spawnCmd.CombinedOutput()
		if err != nil {
			// Log failure silently — worktree switch still succeeds
			m.statusErr = fmt.Sprintf(`Zellij tab spawn failed: %v

Tip: Ensure zellij is installed and ~/.config/grove/layouts/default.kdl exists`, err)
		}
	}()

	// Add new session to track it
	session := domain.Session{
		WorktreePath: worktreePath,
		Status:       domain.StatusActive,
		StartedAt:    time.Now().UTC().Truncate(time.Second),
	}
	m.sessions = append(m.sessions, session)

	// Apply cleanup policy if Zellij integration is enabled - enforce max_tabs AFTER adding new session
	if m.Config.Zellij.Enabled && m.Config.Zellij.MaxTabs > 0 {
		for len(m.sessions) > m.Config.Zellij.MaxTabs {
			m.applyCleanupPolicy()
			}
	}

	return sessionSpawnedMsg{
		session: session,
	}
}

// applyCleanupPolicy removes the oldest single session when exceeding max tabs.
func (m *Model) applyCleanupPolicy() {
	maxTabs := int(m.Config.Zellij.MaxTabs)
	if len(m.sessions) <= maxTabs {
		return
	}
	
	// Find the oldest session by StartedAt
	oldestIdx := 0
	for i := 1; i < len(m.sessions); i++ {
		if m.sessions[i].StartedAt.Before(m.sessions[oldestIdx].StartedAt) {
			oldestIdx = i
		}
	}
	oldest := m.sessions[oldestIdx]
	
	// Remove from DB if needed
	if m.db != nil && oldest.ID > 0 {
		data.DeleteSession(m.db, oldest.ID)
	}
	
	// Log cleanup event
	m.logCleanupEvent(oldest, maxTabs)
	
	// Remove oldest session
	m.sessions = slices.Delete(m.sessions, oldestIdx, oldestIdx+1)
}

// logCleanupEvent logs a cleanup event to the configured log file.
func (m *Model) logCleanupEvent(session domain.Session, maxTabs int) {
	if m.Config.LogFilePath == "" {
		return // No log path configured
	}

	logDir := filepath.Dir(m.Config.LogFilePath)
	os.MkdirAll(logDir, 0755)

	// Create log file if it doesn't exist
	file, err := os.OpenFile(m.Config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file for cleanup event: %v", err)
		return
	}
	defer file.Close()

	// Log the cleanup event
	msg := fmt.Sprintf("%s: Cleanup enforced max_tabs=%d. Removed session for worktree: %s (started at: %s)",
		time.Now().Format(time.RFC3339), maxTabs, session.WorktreePath, session.StartedAt.Format(time.RFC3339))

	if _, err := file.WriteString(msg + "\n"); err != nil {
		log.Printf("Failed to write cleanup event to log: %v", err)
	}
}
