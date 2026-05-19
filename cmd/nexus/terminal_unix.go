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
