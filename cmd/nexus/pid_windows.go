//go:build windows

package main

import "golang.org/x/sys/windows"

// pidAlive reports whether the process with the given PID is currently running.
// On Windows, it attempts to open the process with PROCESS_QUERY_LIMITED_INFORMATION.
// A successful open means the process exists; any error means it is gone.
func pidAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}
