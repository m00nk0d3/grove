package domain

// Repository represents a Git repository within Grove.
type Repository struct {
	Path      string            `json:"path"`                    // Path to the Git repository root
	Labs      []LabEntry        `json:"labs"`                    // Per-repository lab entries
	Worktrees []Worktree        `json:"worktrees"`               // Worktrees tracked in this repo
	Issues    []Issue           `json:"issues"`                  // Synced GitHub issues
	PRs       []PullRequest     `json:"prs"`                     // Synced GitHub pull requests
}

// NewRepository creates a new Repository instance with empty Labs slice.
func NewRepository(path string) Repository {
	return Repository{
		Path:    path,
		Labs:    nil, // Will be initialized on first use or persistence load
		Worktrees: []Worktree{},
		Issues:    []Issue{},
		PRs:       []PullRequest{},
	}
}
