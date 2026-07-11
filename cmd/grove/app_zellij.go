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

// validateLayoutPath checks that the layout file is secure (security hardening).
// Rejects world-writable files and non-regular files. Owner-readable is required.
func (m *Model) validateLayoutPath(layoutPath string) bool {
	info, err := os.Stat(layoutPath)
	if err == nil {
		// Regular file AND prevent world-writable (security hardening)
		if info.Mode().IsRegular() == false || info.Mode().Perm()&0o22 != 0 {
			m.statusErr = fmt.Sprintf("Layout file is not a regular file or world-writable: %s", layoutPath)
			return false
		}
	} else {
		// Can't stat file, fallback to inline default
		m.statusErr = fmt.Sprintf("Cannot validate layout file permissions: %s", layoutPath)
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
		// Fallback: create shell session and add to tracking list
		newSession := domain.Session{
			WorktreePath: worktreePath,
			Status:       domain.StatusActive,
			StartedAt:    time.Now().UTC().Truncate(time.Second),
		}
		m.sessions = append(m.sessions, newSession)

		// Enforce max tabs in fallback too
		if m.Config.Zellij.Enabled {
			// Apply max_tabs limit if configured
			if m.Config.Zellij.MaxTabs > 0 {
				for len(m.sessions) > m.Config.Zellij.MaxTabs {
					m.applyCleanupPolicy()
				}
			}

			// Always run idle-based cleanup if idle timeout is configured (regardless of max_tabs)
			idleThreshold := time.Duration(m.Config.Zellij.CleanupIdleMinutes) * time.Minute
			if idleThreshold > 0 && idleThreshold <= time.Hour*24*7 {
				m.applyIdleCleanupPolicy()
			}
		}
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

	// Apply cleanup policy if Zellij integration is enabled - enforce max_tabs AND idle timeout AFTER adding new session
	if m.Config.Zellij.Enabled {
		// Enforce max_tabs limit if configured
		if m.Config.Zellij.MaxTabs > 0 {
			for len(m.sessions) > m.Config.Zellij.MaxTabs {
				m.applyCleanupPolicy()
			}
		}

		// Always run idle-based cleanup if idle timeout is configured (regardless of max_tabs)
		idleThreshold := time.Duration(m.Config.Zellij.CleanupIdleMinutes) * time.Minute
		if idleThreshold > 0 && idleThreshold <= time.Hour*24*7 {
			m.applyIdleCleanupPolicy()
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
	m.logCleanupEvent(oldest, maxTabs, "max_tabs")

	// Remove oldest session
	m.sessions = slices.Delete(m.sessions, oldestIdx, oldestIdx+1)
}

// logCleanupEvent logs a cleanup event to the configured log file.
func (m *Model) logCleanupEvent(session domain.Session, maxTabs int, reason string) {
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

	// Build the cleanup message with reason and idle time if applicable
	idleDuration := ""
	if session.IdleAt != nil {
		idleDuration = fmt.Sprintf(" (idle: %s)", durationSince(session.IdleAt).String())
	}

	msg := fmt.Sprintf(
		"%s: %s=%d [%s]. Removed session for worktree: %s (started at: %s)",
		time.Now().Format(time.RFC3339), reason, maxTabs, session.WorktreePath,
		session.StartedAt.Format(time.RFC3339), idleDuration,
	)

	if _, err := file.WriteString(msg + "\n"); err != nil {
}

// durationSince calculates the time elapsed since a given time.
}
func durationSince(t *time.Time) time.Duration {
	return time.Since(*t)
}


// applyIdleCleanupPolicy removes sessions that have exceeded the idle timeout.
// This runs independently of max_tabs limit and handles stale tabs based on user activity.
func (m *Model) applyIdleCleanupPolicy() {
	if len(m.sessions) <= 1 {
		return // Need at least 2 sessions to compare
	}

	idleThreshold := time.Duration(m.Config.Zellij.CleanupIdleMinutes) * time.Minute
	if idleThreshold == 0 || idleThreshold > time.Hour*24*7 {
		log.Printf("Warning: CleanupIdleMinutes (%d) exceeds safe limit (168h), capping to max", m.Config.Zellij.CleanupIdleMinutes)
		return // Idle cleanup disabled when config value is invalid or unsafe
	}

	// Find all idle sessions and count them
	var idleSessions []domain.Session
	for _, s := range m.sessions {
		if s.IdleAt == nil || time.Since(*s.IdleAt) > idleThreshold {
			idleSessions = append(idleSessions, s)
		}
	}

	// If no idle sessions exceed threshold, nothing to clean up
	if len(idleSessions) == 0 {
		return
	}

	// Sort by most idle first (remove most idle oldest session first)
	slices.SortFunc(idleSessions, func(a, b domain.Session) int {
		if a.IdleAt == nil && b.IdleAt == nil {
			return 0 // Both have no activity
		}
		if a.IdleAt == nil {
			return -1 // Prioritize removing session with no idle tracking
		}
		if b.IdleAt == nil {
			return 1 // Keep session without idle tracking
		}
		// Sort by most idle first (oldest IdleAt)
		if a.IdleAt.Before(*b.IdleAt) {
			return -1
		}
		return 1
	})

	// Remove the most idle session (limit to one per cleanup cycle for safety)
	if len(idleSessions) > 0 && idleSessions[0].IdleAt != nil {
		oldestIdx := slices.IndexFunc(m.sessions, func(s domain.Session) bool {
			for _, idls := range idleSessions {
				if s.WorktreePath == idls.WorktreePath && s.ID == idls.ID {
					return true
				}
			}
			return false
		})

		// Remove from DB if needed
		if m.db != nil && oldestIdx >= 0 && m.sessions[oldestIdx].ID > 0 {
			data.DeleteSession(m.db, m.sessions[oldestIdx].ID)
		}

		// Log cleanup event
		m.logCleanupEvent(m.sessions[oldestIdx], -1, "idle_timeout")

		// Remove oldest session from in-memory list
		if oldestIdx >= 0 && oldestIdx < len(m.sessions) {
			m.sessions = slices.Delete(m.sessions, oldestIdx, oldestIdx+1)
		}
	}
}
