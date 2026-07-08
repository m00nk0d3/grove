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

// BuildHerdrCommand constructs the command to launch herdr with a working directory and agent command.
// It uses "sh -c" to wrap the agent command for shell-safety as required by the PRD.
func BuildHerdrCommand(worktreePath, agentCmd string) *exec.Cmd {
	var args []string
	if agentCmd != "" {
		// Wrap agentCmd in single quotes to ensure sh -c treats it as a single command string.
		// We escape existing single quotes by replacing them with '\'' (close quote, escaped quote, open quote).
		safeAgentCmd := fmt.Sprintf("'%s'", strings.ReplaceAll(agentCmd, "'", `'\''`))
		args = []string{"new-window", "--working-directory", worktreePath, "--", "sh", "-c", safeAgentCmd}
	} else {
		args = []string{"new-window", "--working-directory", worktreePath}
	}
	return exec.Command("herdr", args...)
}
