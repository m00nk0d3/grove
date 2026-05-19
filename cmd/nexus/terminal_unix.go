//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// setWindowsCmdLine is a no-op on non-Windows platforms.
func setWindowsCmdLine(_ *exec.Cmd, _ string) {}

// spawnTerminalWindow opens a new terminal window at path and returns the PID
// of the spawned process. On Linux the terminal emulator stays alive, so the
// PID is reliable. On macOS, open -a Terminal exits quickly (same trade-off as
// Windows) but we keep the existing behaviour for now.
func spawnTerminalWindow(path string) (int, error) {
	cmd := buildNewTerminalCmd(path, runtime.GOOS)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

// spawnAgentInTerminalWindow launches agentCmd at path, preferring a new tab
// in the current terminal emulator when possible, falling back to a new window.
// The TUI is not suspended — nexus keeps running in the original terminal.
func spawnAgentInTerminalWindow(path, agentCmd string) (int, error) {
	// Prefer a new tab when the running terminal supports it.
	if tabCmd, ok := buildNewTabWithCmdCmd(path, agentCmd, runtime.GOOS); ok {
		if err := tabCmd.Start(); err == nil {
			return tabCmd.Process.Pid, nil
		}
		// Fall through to new window on error (e.g. kitty remote-control disabled).
	}

	cmd := buildNewTerminalWithCmdCmd(path, agentCmd, runtime.GOOS)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}
