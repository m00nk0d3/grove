//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

// installSIGTSTP installs a SIGTSTP (Ctrl+Z) signal handler that cleanly
// suspends the Bubbletea program by sending tea.SuspendMsg. The TUI is fully
// restored when the process is resumed with fg.
func installSIGTSTP(p *tea.Program) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTSTP)
	go func() {
		for range sigCh {
			p.Send(tea.SuspendMsg{})
		}
	}()
}
