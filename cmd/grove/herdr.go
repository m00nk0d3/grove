package main

import (
	"fmt"
	"os/exec"
	"runtime"
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

// BuildHerdrCommand constructs the command to create a new herdr workspace or tab.
func BuildHerdrCommand(path, workspaceID, label string) *exec.Cmd {
	if workspaceID != "" {
		return BuildHerdrTabCmd(path, workspaceID, label)
	}
	return BuildHerdrWorkspaceCmd(path, label)
}

// BuildHerdrPaneCmd constructs the command to run a command in a herdr pane.
// Matches PRD: herdr pane run <pane_id> <command>
// The command is wrapped in a shell-safe manner.
func BuildHerdrPaneCmd(paneID, agentCmd string) *exec.Cmd {
	if agentCmd == "" {
		return nil
	}
	var shell, args []string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		// Use /K to keep the shell open if needed, or just run it. 
		// Wrapping in quotes for shell safety.
		escapedCmd := strings.ReplaceAll(agentCmd, "\"", `\"`)
		args = []string{"/K", fmt.Sprintf("\"%s\"", escapedCmd)}
	} else {
		shell = "sh"
		// Wrap agentCmd in single quotes to ensure sh -c treats it as a single command string.
		// We escape existing single quotes by replacing them with '\'' (close quote, escaped quote, open quote).
		safeAgentCmd := fmt.Sprintf("'%s'", strings.ReplaceAll(agentCmd, "'", `'\''`))
		args = []string{"-c", safeAgentCmd}
	}
	return exec.Command("herdr", append([]string{"pane", "run", paneID}, shell, args...)...)
}
