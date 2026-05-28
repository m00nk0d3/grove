//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// setWindowsCmdLine is a no-op on non-Windows platforms.
func setWindowsCmdLine(_ *exec.Cmd, _ string) {}

// spawnTerminalWindow opens a terminal at path, preferring a new tab in the
// current terminal emulator when possible, falling back to a new standalone
// window. The spawned shell writes its PID to a temp file so nexus can track
// whether the tab is still open. Returns the shell PID, or 0 if the PID could
// not be determined within 3 seconds.
func spawnTerminalWindow(path string) (int, error) {
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("grove-session-%d.pid", time.Now().UnixNano()))

	if tabCmd, ok := buildNewTabWithCmdCmd(path, "", pidFile, runtime.GOOS); ok {
		if err := tabCmd.Start(); err == nil {
			pid := pollPIDFile(pidFile, 3*time.Second)
			os.Remove(pidFile)
			return pid, nil
		}
		// Fall through to new window on error.
	}

	cmd := buildNewTerminalCmd(path, pidFile, runtime.GOOS)
	if err := cmd.Start(); err != nil {
		os.Remove(pidFile)
		return 0, err
	}
	pid := pollPIDFile(pidFile, 3*time.Second)
	os.Remove(pidFile)
	if pid != 0 {
		return pid, nil
	}
	// PID file not populated (e.g. plain `open -a Terminal` without osascript
	// fallback) — return the launcher PID as best-effort; it may be 0 for
	// emulators whose launcher process exits immediately.
	return cmd.Process.Pid, nil
}

// spawnAgentInTerminalWindow launches agentCmd at path, preferring a new tab
// in the current terminal emulator when possible, falling back to a new window.
// The TUI is not suspended — grove keeps running in the original terminal.
// Agent processes are tracked via AgentPID, not ShellPID, so no PID file is used.
func spawnAgentInTerminalWindow(path, agentCmd string) (int, error) {
	if tabCmd, ok := buildNewTabWithCmdCmd(path, agentCmd, "", runtime.GOOS); ok {
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
