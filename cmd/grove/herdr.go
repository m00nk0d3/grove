package main

import (
	"os/exec"
)

// IsHerdrInstalled returns true if the herdr binary is found on the system's PATH.
func IsHerdrInstalled() bool {
	_, err := exec.LookPath("herdr")
	return err == nil
}

// BuildHerdrCommand constructs the command to launch herdr with a working directory and agent command.
// It uses "sh -c" to wrap the agent command for shell-safety as required by the PRD.
func BuildHerdrCommand(worktreePath, agentCmd string) *exec.Cmd {
	// Construct: herdr new-window --working-directory <path> -- sh -c "<agentCmd>"
	// If agentCmd is empty, we just want to open a new window.
	var args []string
	if agentCmd != "" {
		args = []string{"new-window", "--working-directory", worktreePath, "--", "sh", "-c", agentCmd}
	} else {
		args = []string{"new-window", "--working-directory", worktreePath}
	}
	return exec.Command("herdr", args...)
}
