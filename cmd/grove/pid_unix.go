//go:build !windows

package main

import "syscall"

// pidAlive reports whether the process with the given PID is currently running.
// On Unix, it sends signal 0, which does not kill the process but returns an
// error if the process does not exist. EPERM means the process exists but we
// cannot signal it (still alive).
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
