package exec

import (
	"fmt"
	osexec "os/exec"
	"strconv"
	"strings"

	"github.com/m00nk0d3/nexus/internal/domain"
)

const (
	worktreePrefix   = "worktree "
	headPrefix       = "HEAD "
	branchPrefix     = "branch "
	branchHeadPrefix = "refs/heads/"
	detachedMarker   = "detached"
	cleanMarker      = "clean"
	lockedMarker     = "locked"
	lockedWithReason = "locked "
)

// GitCommand represents a git command executor
type GitCommand struct {
	repoPath string
	runner   commandRunner
}

type commandRunner func(repoPath string, args ...string) (string, error)

// NewGitCommand creates a new git command executor
func NewGitCommand(repoPath string) *GitCommand {
	return NewGitCommandWithRunner(repoPath, runGitCommand)
}

// NewGitCommandWithRunner creates a git command executor with an injected runner.
func NewGitCommandWithRunner(repoPath string, runner commandRunner) *GitCommand {
	return &GitCommand{
		repoPath: repoPath,
		runner:   runner,
	}
}

// ListWorktrees returns worktrees from `git worktree list --porcelain`.
// IsClean is determined by running `git status --porcelain` per worktree,
// since the porcelain format does not reliably emit a "clean" marker.
func (g *GitCommand) ListWorktrees() ([]domain.Worktree, error) {
	output, err := g.run("list worktrees", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	worktrees, err := parseWorktreeListPorcelain(output)
	if err != nil {
		return nil, fmt.Errorf("parse worktree porcelain: %w", err)
	}

	for i := range worktrees {
		clean, err := g.isWorktreeClean(worktrees[i].Path)
		if err != nil {
			return nil, err
		}
		worktrees[i].IsClean = clean
	}

	return worktrees, nil
}

func (g *GitCommand) isWorktreeClean(path string) (bool, error) {
	output, err := g.runner(path, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("check worktree status: %w", err)
	}
	return strings.TrimSpace(output) == "", nil
}

// AddWorktreeNewBranch fetches the latest state of baseBranch from origin, then
// creates a new worktree and branch rooted at origin/<baseBranch> so the
// worktree always starts from the most recent remote commit.
func (g *GitCommand) AddWorktreeNewBranch(path, branchName, baseBranch string) error {
	if err := g.FetchRemoteBranch(baseBranch); err != nil {
		return err
	}
	return g.runNoOutput("add worktree new branch", "worktree", "add", "-b", branchName, path, "origin/"+baseBranch)
}

// AddWorktree adds a new worktree at path for branch.
func (g *GitCommand) AddWorktree(path, branch string) error {
	return g.runNoOutput("add worktree", "worktree", "add", path, branch)
}

// FetchRemoteBranch fetches the named branch from origin.
func (g *GitCommand) FetchRemoteBranch(branch string) error {
	return g.runNoOutput("fetch remote branch", "fetch", "origin", branch)
}

// CheckoutPRWorktree fetches a remote branch and creates a new worktree tracking it.
// Uses -B so it works even if a local branch by that name already exists.
func (g *GitCommand) CheckoutPRWorktree(path, branch string) error {
	if err := g.FetchRemoteBranch(branch); err != nil {
		return err
	}
	return g.runNoOutput("checkout pr worktree", "worktree", "add", "-B", branch, path, "origin/"+branch)
}

// RemoveWorktree removes the worktree at path.
func (g *GitCommand) RemoveWorktree(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)

	return g.runNoOutput("remove worktree", args...)
}

// ListModifiedFiles returns the list of modified (unstaged) and untracked files
// in the worktree at path using git ls-files --modified --others --exclude-standard.
// Note: staged-only files (added with git add but not yet modified) are not included.
func (g *GitCommand) ListModifiedFiles(path string) ([]string, error) {
	output, err := g.runner(path, "ls-files", "--modified", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("list modified files: %w", err)
	}

	var files []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// PruneWorktrees prunes stale worktree metadata.
func (g *GitCommand) PruneWorktrees() error {
	return g.runNoOutput("prune worktrees", "worktree", "prune")
}

// LockWorktree locks the worktree at path, optionally with a reason.
func (g *GitCommand) LockWorktree(path, reason string) error {
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	args = append(args, path)

	return g.runNoOutput("lock worktree", args...)
}

// UnlockWorktree unlocks the worktree at path.
func (g *GitCommand) UnlockWorktree(path string) error {
	return g.runNoOutput("unlock worktree", "worktree", "unlock", path)
}

// GitStatus returns the short git status for the worktree at path.
func (g *GitCommand) GitStatus(path string) (string, error) {
	output, err := g.runner(path, "status", "--short")
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GitLog returns the last n commits for the worktree at path in oneline format.
func (g *GitCommand) GitLog(path string, n int) (string, error) {
	output, err := g.runner(path, "log", "--oneline", "-n", strconv.Itoa(n))
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GitDiffNameOnly returns the list of changed files for the worktree at path.
func (g *GitCommand) GitDiffNameOnly(path string) ([]string, error) {
	output, err := g.runner(path, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git diff name-only: %w", err)
	}
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return []string{}, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// GitDiffStat returns the diff stat for the worktree at path.
func (g *GitCommand) GitDiffStat(path string) (string, error) {
	output, err := g.runner(path, "diff", "--stat", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git diff stat: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// GitBranch returns the current branch name for the worktree at path.
func (g *GitCommand) GitBranch(path string) (string, error) {
	output, err := g.runner(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git branch: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func (g *GitCommand) run(op string, args ...string) (string, error) {
	output, err := g.runner(g.repoPath, args...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return output, nil
}

func (g *GitCommand) runNoOutput(op string, args ...string) error {
	_, err := g.run(op, args...)
	return err
}

func runGitCommand(repoPath string, args ...string) (string, error) {
	cmd := osexec.Command("git", args...)
	cmd.Dir = repoPath

	out, err := cmd.CombinedOutput()
	output := string(out)
	if err != nil {
		trimmedOutput := strings.TrimSpace(output)
		if trimmedOutput != "" {
			return "", fmt.Errorf("run git %s: %w; output: %s", strings.Join(args, " "), err, trimmedOutput)
		}

		return "", fmt.Errorf("run git %s: %w", strings.Join(args, " "), err)
	}

	return output, nil
}

func parseWorktreeListPorcelain(porcelain string) ([]domain.Worktree, error) {
	var (
		worktrees []domain.Worktree
		current   *domain.Worktree
	)

	appendCurrent := func() {
		if current != nil {
			worktrees = append(worktrees, *current)
		}
	}

	lines := strings.Split(porcelain, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if path, ok := strings.CutPrefix(line, worktreePrefix); ok {
			appendCurrent()
			current = &domain.Worktree{
				Path: path,
			}
			continue
		}

		if current == nil {
			return nil, fmt.Errorf("metadata before worktree: %q", line)
		}

		switch {
		case strings.HasPrefix(line, headPrefix):
			current.CommitSHA = strings.TrimPrefix(line, headPrefix)
		case strings.HasPrefix(line, branchPrefix):
			branchRef := strings.TrimPrefix(line, branchPrefix)
			current.Branch = strings.TrimPrefix(branchRef, branchHeadPrefix)
		case line == detachedMarker:
			if current.Branch == "" {
				current.Branch = detachedMarker
			}
		case line == cleanMarker:
			current.IsClean = true
		case line == lockedMarker || strings.HasPrefix(line, lockedWithReason):
			current.IsLocked = true
		}
	}

	appendCurrent()

	return worktrees, nil
}
