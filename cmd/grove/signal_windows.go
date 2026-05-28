//go:build windows

package main

import tea "github.com/charmbracelet/bubbletea"

// installSIGTSTP is a no-op on Windows — SIGTSTP does not exist on this platform.
func installSIGTSTP(_ *tea.Program) {}
