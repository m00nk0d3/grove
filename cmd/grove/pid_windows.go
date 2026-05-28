//go:build windows

package main

import "golang.org/x/sys/windows"

// pidAlive reports whether the process with the given PID is currently running.
// On Windows, OpenProcess can succeed even for processes that have already exited
// if their object handle is still held open elsewhere (zombie-like). We follow up
// with GetExitCodeProcess: exit code STILL_ACTIVE (259) means the process is
// genuinely running; any other code means it has terminated.
func pidAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE / STATUS_PENDING
	return code == stillActive
}
