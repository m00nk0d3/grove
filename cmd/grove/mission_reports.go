package main

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/mission"
	"github.com/m00nk0d3/grove/internal/tui/modal"
)

// gitCommonDir resolves the shared git directory of the repository holding
// dir. Tests replace it to avoid running git.
var gitCommonDir = func(dir string) (string, error) {
	return internalexec.NewGitCommand(dir).CommonDir()
}

// loadMissionReportsCmd reads the reports a workflow run has written. They
// live in the repository's git common directory, which every worktree shares,
// so the run's own worktree is used when it still exists and the repository
// Grove is open on otherwise: an implementation run's worktree is often gone
// by the time its report is read.
func loadMissionReportsCmd(msg modal.MissionReportsRequestedMsg, repoPath string) tea.Cmd {
	return func() tea.Msg {
		loaded := modal.MissionReportsLoadedMsg{
			RunID:     msg.RunID,
			Supported: mission.WritesReports(msg.Workflow),
		}
		if !loaded.Supported {
			return loaded
		}
		dir := repoPath
		if wt := msg.Workflow.WorktreePath; wt != "" {
			if info, err := os.Stat(wt); err == nil && info.IsDir() {
				dir = wt
			}
		}
		commonDir, err := gitCommonDir(dir)
		if err != nil {
			loaded.Err = err
			return loaded
		}
		loaded.Reports, loaded.Err = mission.FindReports(filepath.Join(commonDir, mission.ReportsDir), msg.Workflow)
		return loaded
	}
}
