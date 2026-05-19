//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"syscall"
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
