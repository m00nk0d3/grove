//go:build windows

package main

import "os"

// gracefulKillPID immediately terminates the process with the given PID.
// Windows does not expose a SIGTERM equivalent for console processes;
// TerminateProcess (via os.Process.Kill) is the appropriate forceful stop.
func gracefulKillPID(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Kill()
}
