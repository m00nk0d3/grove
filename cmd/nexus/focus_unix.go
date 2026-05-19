//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// focusSessionWindow attempts to bring the terminal window that owns pid to the
// foreground. The call is best-effort: on platforms or configurations where no
// suitable tooling is available a descriptive error is returned but no panic
// occurs.
func focusSessionWindow(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("no trackable PID for this session")
	}
	switch runtime.GOOS {
	case "darwin":
		return focusWindowDarwin()
	default:
		return focusWindowLinux(pid)
	}
}

// focusWindowDarwin activates the terminal application that is currently
// running nexus. Because macOS does not easily let us map a shell PID to a
// specific window without scripting bridges, we activate the whole app which
// brings its frontmost window forward — good enough for most single-window
// workflows. In multi-window setups this may bring the wrong window to front;
// the PID parameter is intentionally not used here for this reason.
func focusWindowDarwin() error {
	var appName string
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		appName = "iTerm2"
	default:
		// Terminal.app, Ghostty, and most others expose an "activate" AppleScript
		// verb via the generic "Terminal" identifier.
		appName = "Terminal"
	}
	script := fmt.Sprintf(`tell application %q to activate`, appName)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return fmt.Errorf("osascript activate %q: %w", appName, err)
	}
	return nil
}

// focusWindowLinux tries xdotool first (most reliable), then falls back to
// wmctrl. Both tools accept a PID and raise the matching window.
func focusWindowLinux(pid int) error {
	pidStr := strconv.Itoa(pid)

	// xdotool search --pid <pid> windowactivate
	if path, err := exec.LookPath("xdotool"); err == nil {
		if err := exec.Command(path, "search", "--pid", pidStr, "windowactivate").Run(); err == nil {
			return nil
		}
	}

	// wmctrl -lp lists windows with PIDs; parse to find the window ID.
	if _, err := exec.LookPath("wmctrl"); err == nil {
		out, err := exec.Command("wmctrl", "-lp").Output()
		if err == nil {
			if winID := wmctrlFindWindowByPID(string(out), pid); winID != "" {
				if err := exec.Command("wmctrl", "-i", "-a", winID).Run(); err == nil {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("could not focus window for PID %d: install xdotool or wmctrl to enable window focus", pid)
}

// wmctrlFindWindowByPID parses the output of "wmctrl -lp" and returns the
// hex window ID for the first line whose PID column matches pid.
// wmctrl -lp output format: <window_id> <desktop> <pid> <host> <title>
func wmctrlFindWindowByPID(output string, pid int) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if p, err := strconv.Atoi(fields[2]); err == nil && p == pid {
			return fields[0]
		}
	}
	return ""
}
