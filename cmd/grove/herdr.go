package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsHerdrInstalled returns true if the herdr binary is found on the system's PATH.
func IsHerdrInstalled() bool {
	_, err := exec.LookPath("herdr")
	return err == nil
}

// BuildHerdrWorkspaceCmd constructs the command to create a new herdr workspace.
// Matches PRD: herdr workspace create --cwd <path> --label <label>
func BuildHerdrWorkspaceCmd(path, label string) *exec.Cmd {
	if label == "" {
		label = "grove"
	}
	args := []string{"workspace", "create", "--cwd", path, "--label", label}
	return exec.Command("herdr", args...)
}

// BuildHerdrTabCmd constructs the command to create a new herdr tab.
// Matches PRD: herdr tab create --workspace <workspace_id> --cwd <path> --label <label>
func BuildHerdrTabCmd(path, workspaceID, label string) *exec.Cmd {
	if label == "" {
		label = "grove"
	}
	args := []string{"tab", "create", "--workspace", workspaceID, "--cwd", path, "--label", label}
	return exec.Command("herdr", args...)
}

// BuildHerdrPaneCmd constructs the command to run a command in a herdr pane.
// Matches PRD: herdr pane run <pane_id> <command>
// The command is wrapped in a shell-safe manner.
func BuildHerdrPaneCmd(paneID, agentCmd string) *exec.Cmd {
	if agentCmd == "" {
		return nil
	}
	// Wrap agentCmd in single quotes to ensure sh -c treats it as a single command string.
	// We escape existing single quotes by replacing them with '\'' (close quote, escaped quote, open quote).
	safeAgentCmd := fmt.Sprintf("'%s'", strings.ReplaceAll(agentCmd, "'", `'\''`))
	args := []string{"pane", "run", paneID, "sh", "-c", safeAgentCmd}
	return exec.Command("herdr", args...)
}
