package domain

import (
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	GitHub     GitHubConfig     `toml:"github"`
	Appearance AppearanceConfig `toml:"appearance"`
	Herdr      HerdrConfig      `toml:"herdr"`
	Sandcastle SandcastleConfig `toml:"sandcastle"`
	Worktrees  WorktreesConfig  `toml:"worktrees"`
}

type GitHubConfig struct {
	AutoSync            bool `toml:"auto_sync"`
	SyncIntervalMinutes int  `toml:"sync_interval_minutes"`
}

// SyncInterval returns the configured sync interval as a time.Duration.
// Defaults to 5 minutes when SyncIntervalMinutes is zero or negative.
func (c GitHubConfig) SyncInterval() time.Duration {
	if c.SyncIntervalMinutes <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(c.SyncIntervalMinutes) * time.Minute
}

type AppearanceConfig struct {
	Theme string `toml:"theme"`
}

type HerdrConfig struct {
	Enabled bool `toml:"enabled"`
	// PollIntervalSeconds is how often Herdr and Sandcastle status are checked.
	PollIntervalSeconds int  `toml:"poll_interval_seconds"`
	PreferWorktreeAPI   bool `toml:"prefer_worktree_api"`
}

type SandcastleConfig struct {
	Enabled      bool   `toml:"enabled"`
	Binary       string `toml:"binary"`
	DefaultAgent string `toml:"default_agent"`
}

type WorktreesConfig struct {
	BaseBranch   string `toml:"base_branch"`
	WorktreeRoot string `toml:"worktree_root"`
}

func DefaultConfig() *Config {
	return &Config{
		Appearance: AppearanceConfig{Theme: "digital-noir"},
		Worktrees:  WorktreesConfig{BaseBranch: "main", WorktreeRoot: DefaultWorktreeRoot},
		GitHub:     GitHubConfig{AutoSync: true, SyncIntervalMinutes: 5},
		Herdr: HerdrConfig{
			Enabled:             true,
			PollIntervalSeconds: 5,
			PreferWorktreeAPI:   true,
		},
		Sandcastle: SandcastleConfig{
			Enabled:      true,
			Binary:       "grove-sandcastle",
			DefaultAgent: "opencode",
		},
	}
}

// DefaultWorktreeRoot is where worktrees are created when no root is
// configured, relative to the repository root.
const DefaultWorktreeRoot = "../worktrees"

// WorktreePath returns where the worktree named slug is created for the
// repository at repoPath: <worktree_root>/<repository name>/<slug>. A relative
// root is resolved against the repository root, and an empty root means
// DefaultWorktreeRoot. The repository name keeps the worktrees of several
// repositories that share a root apart.
func (c WorktreesConfig) WorktreePath(repoPath, slug string) string {
	root := strings.TrimSpace(c.WorktreeRoot)
	if root == "" {
		root = DefaultWorktreeRoot
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(repoPath, root)
	}
	return filepath.Join(filepath.Clean(root), filepath.Base(repoPath), slug)
}
