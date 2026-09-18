package domain

import "time"

type Config struct {
	GitHub     GitHubConfig     `toml:"github"`
	Appearance AppearanceConfig `toml:"appearance"`
	AIAgents   AIAgentsConfig   `toml:"ai_agents"`
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

type AIAgentsConfig struct {
	CopilotEnabled bool   `toml:"copilot_enabled"`
	ClaudeEnabled  bool   `toml:"claude_enabled"`
	AiderEnabled   bool   `toml:"aider_enabled"`
	ClaudeBinary   string `toml:"claude_binary"`
	AiderBinary    string `toml:"aider_binary"`
}

type HerdrConfig struct {
	Enabled             bool   `toml:"enabled"`
	Binary              string `toml:"binary"`
	PollIntervalSeconds int    `toml:"poll_interval_seconds"`
	PreferWorktreeAPI   bool   `toml:"prefer_worktree_api"`
}

type SandcastleConfig struct {
	Enabled             bool   `toml:"enabled"`
	Binary              string `toml:"binary"`
	PollIntervalSeconds int    `toml:"poll_interval_seconds"`
	DefaultAgent        string `toml:"default_agent"`
}

type WorktreesConfig struct {
	BaseBranch   string `toml:"base_branch"`
	WorktreeRoot string `toml:"worktree_root"`
}

func DefaultConfig() *Config {
	return &Config{
		Appearance: AppearanceConfig{Theme: "digital-noir"},
		Worktrees:  WorktreesConfig{BaseBranch: "main", WorktreeRoot: "../worktrees"},
		GitHub:     GitHubConfig{AutoSync: true, SyncIntervalMinutes: 5},
		AIAgents:   AIAgentsConfig{CopilotEnabled: true, ClaudeEnabled: true},
		Herdr: HerdrConfig{
			Enabled:             true,
			Binary:              "herdr",
			PollIntervalSeconds: 5,
			PreferWorktreeAPI:   true,
		},
		Sandcastle: SandcastleConfig{
			Enabled:             true,
			Binary:              "sandcastle",
			PollIntervalSeconds: 5,
			DefaultAgent:        "pi",
		},
	}
}
