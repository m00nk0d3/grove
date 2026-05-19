//go:build !windows

package main

import "os/exec"

// setWindowsCmdLine is a no-op on non-Windows platforms.
func setWindowsCmdLine(_ *exec.Cmd, _ string) {}
