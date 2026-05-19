package domain

// Worktree represents a git worktree managed by Nexus
type Worktree struct {
	Path      string
	Branch    string
	CommitSHA string
	IsClean   bool
	IsLocked  bool
	LinkedPR  *PullRequest
}
