// Package main implements the Groove Git Worktree Orchestrator with Zellij integration.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

// spawnZellijTabCmd spawns a new Zellij tab for worktree switching if zellij is available.
// If zellij binary is not found or layout file is missing, falls back to plain shell session.
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
		}

		// 3. Build spawn command with worktree path in tab name
		worktreeName := filepath.Base(worktreePath)
		spawnCmd := exec.Command("zellij", "action", "new-tab", "--layout", layoutPath, "--name", fmt.Sprintf("Grove-%s", worktreeName))

		// 4. Spawn the command asynchronously (don't block TUI)
		go func() {
			_, err = spawnCmd.CombinedOutput()
			if err != nil {
				// Log failure silently — worktree switch still succeeds
				m.statusErr = fmt.Sprintf("Zellij tab spawn failed: %v (fallback to shell)", err)
			}
		}()

		return sessionSpawnedMsg{
			session: domain.Session{
				WorktreePath: worktreePath,
				Status:       domain.StatusActive,
				StartedAt:    time.Now().UTC().Truncate(time.Second),
			},
		}
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
}
`
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
