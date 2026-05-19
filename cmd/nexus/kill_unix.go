//go:build !windows

package main

import (
	"os"
	"syscall"
	"time"
)

// gracefulKillPID sends SIGTERM to pid and waits up to 3 seconds for the process
// to exit. If it is still alive after the timeout, SIGKILL is sent as a fallback.
// If the process is not found (already exited), the call is a no-op.
func gracefulKillPID(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
}
