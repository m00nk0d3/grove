//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// setWindowsCmdLine overrides the raw Windows command line for the new-terminal
// command so that cmd.exe receives an unescaped string.
//
// Go's exec package escapes inner double-quotes in arguments (e.g. "path" →
// \"path\"), but cmd.exe does not honour that C-runtime escaping convention.
// Setting SysProcAttr.CmdLine bypasses Go's argument joiner entirely and lets
// cmd.exe parse the string directly, which correctly handles the quoted path in
// the "cd /d" sub-command.
func setWindowsCmdLine(c *exec.Cmd, path string) {
	c.SysProcAttr = &syscall.SysProcAttr{
		// "" is the window title for `start`; the empty string avoids the
		// ambiguity where start would otherwise treat the first quoted arg as
		// the window title instead of the command.
		CmdLine: fmt.Sprintf(`cmd /C start "" cmd /K cd /d "%s"`, path),
	}
}

// spawnTerminalWindow opens a terminal at path, preferring a new tab in the
// current terminal emulator when possible, falling back to a new standalone
// cmd.exe window. Returns the PID of the spawned shell process, or 0 when no
// trackable long-lived PID is available.
func spawnTerminalWindow(path string) (int, error) {
	// Windows Terminal: use a PID-file trick so we get the real PowerShell PID
	// (which stays alive as long as the tab is open) instead of the wt.exe
	// launcher PID (which exits immediately after creating the tab).
	if os.Getenv("WT_SESSION") != "" {
		pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("nexus-session-%d.pid", time.Now().UnixNano()))
		// The PowerShell session writes its own PID to the temp file on start,
		// then stays interactive. Deleting the file is handled by the caller.
		psCmd := fmt.Sprintf(
			`[System.Diagnostics.Process]::GetCurrentProcess().Id | Set-Content -LiteralPath '%s'`,
			pidFile,
		)
		cmd := exec.Command("wt", "-w", "0", "new-tab", "--startingDirectory", path,
			"powershell", "-NoExit", "-Command", psCmd)
		if err := cmd.Start(); err == nil {
			pid := pollPIDFile(pidFile, 3*time.Second)
			os.Remove(pidFile)
			return pid, nil
		}
		// Fall through to standalone window on error.
	}

	// Other tab-capable emulators (tmux, zellij under Windows/WSL) — use
	// Unix-style PID-file injection via buildNewTabWithCmdCmd.
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("nexus-session-%d.pid", time.Now().UnixNano()))
	if tabCmd, ok := buildNewTabWithCmdCmd(path, "", pidFile, "windows"); ok {
		if err := tabCmd.Start(); err == nil {
			pid := pollPIDFile(pidFile, 3*time.Second)
			os.Remove(pidFile)
			return pid, nil
		}
		// Fall through to new standalone window on error.
	}
	os.Remove(pidFile)

	// Fallback: open a standalone cmd.exe window via PowerShell.
	// PowerShell Start-Process -PassThru returns a process object whose .Id is
	// the actual cmd.exe /K shell — the one that stays alive while the user has
	// the window open.
	fallbackPsCmd := fmt.Sprintf(
		`(Start-Process -FilePath 'cmd' -ArgumentList '/K','cd /d "%s"' -PassThru).Id`,
		path,
	)
	out, err := exec.Command(
		"powershell", "-NoProfile", "-NonInteractive", "-Command", fallbackPsCmd,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("spawn terminal: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse terminal pid %q: %w", strings.TrimSpace(string(out)), err)
	}
	return pid, nil
}

// spawnAgentInTerminalWindow launches agentCmd at path in a new tab of the
// current terminal emulator when possible, falling back to a new cmd.exe window.
// The TUI is not suspended — nexus keeps running in the original terminal.
func spawnAgentInTerminalWindow(path, agentCmd string) (int, error) {
	// Windows Terminal: use the PID-file trick to capture the real PowerShell PID
	// (wt.exe exits immediately after launching the tab, so tabCmd.Process.Pid
	// would be dead by the time the health-check poller runs).
	if os.Getenv("WT_SESSION") != "" {
		pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("nexus-agent-%d.pid", time.Now().UnixNano()))
		psCmd := fmt.Sprintf(
			`[System.Diagnostics.Process]::GetCurrentProcess().Id | Set-Content -LiteralPath '%s'; %s`,
			pidFile, agentCmd,
		)
		cmd := exec.Command("wt", "-w", "0", "new-tab", "--startingDirectory", path,
			"powershell", "-NoExit", "-Command", psCmd)
		if err := cmd.Start(); err == nil {
			pid := pollPIDFile(pidFile, 3*time.Second)
			os.Remove(pidFile)
			if pid > 0 {
				return pid, nil
			}
		}
		os.Remove(pidFile)
		// Fall through to standalone window on error.
	}

	// Prefer a new tab when the running terminal supports it.
	if tabCmd, ok := buildNewTabWithCmdCmd(path, agentCmd, "", "windows"); ok {
		if err := tabCmd.Start(); err == nil {
			return tabCmd.Process.Pid, nil
		}
		// Fall through to new window on error.
	}

	// Fall back: open a new standalone cmd.exe window via PowerShell.
	psCmd := fmt.Sprintf(
		`(Start-Process -FilePath 'cmd' -ArgumentList '/K','cd /d "%s" & %s' -PassThru).Id`,
		path, agentCmd,
	)
	out, err := exec.Command(
		"powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("spawn agent terminal: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse agent terminal pid %q: %w", strings.TrimSpace(string(out)), err)
	}
	return pid, nil
}
