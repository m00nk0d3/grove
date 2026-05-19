//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

// focusSessionWindow brings the terminal window owned by pid to the foreground
// using PowerShell and the Win32 SetForegroundWindow API. The function is
// best-effort: if no window handle is found for the given PID, a descriptive
// error is returned.
func focusSessionWindow(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("no trackable PID for this session")
	}

	// Inject a minimal P/Invoke shim via Add-Type, then look up the main
	// window handle for the PID and call SetForegroundWindow.
	psScript := `Add-Type @"
using System;
using System.Runtime.InteropServices;
public class NexusFocus {
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
}
"@
$p = Get-Process -Id ` + strconv.Itoa(pid) + ` -ErrorAction SilentlyContinue
if ($p -and $p.MainWindowHandle -ne 0) {
    [NexusFocus]::SetForegroundWindow($p.MainWindowHandle) | Out-Null
    exit 0
}
exit 1
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("focus window (pid %d): %w", pid, err)
	}
	return nil
}
