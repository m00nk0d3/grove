package main

import (
	"path/filepath"
	"strings"
)

// pathsEqual reports whether two filesystem paths refer to the same location.
// It normalises both paths by converting backslashes to forward slashes before
// calling filepath.Clean, then compares case-insensitively. This ensures that
// mixed-separator paths from the Copilot session-store (which may store Windows
// paths with backslashes) match paths returned by git worktree list on any OS.
func pathsEqual(a, b string) bool {
	normalize := func(p string) string {
		return filepath.Clean(strings.ReplaceAll(p, `\`, `/`))
	}
	return strings.EqualFold(normalize(a), normalize(b))
}
