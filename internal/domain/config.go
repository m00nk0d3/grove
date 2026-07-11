package domain

import "time"

type Config struct {
	GitHub      GitHubConfig     `toml:"github"`
	Appearance  AppearanceConfig `toml:"appearance"`
	AIAgents    AIAgentsConfig   `toml:"ai_agents"`
	Worktrees   WorktreesConfig  `toml:"worktrees"`
	Zellij      ZellijConfig     `toml:"zellij"`
	LogFilePath string           `toml:"log_file_path"`
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

type WorktreesConfig struct {
	BaseBranch   string `toml:"base_branch"`
	WorktreeRoot string `toml:"worktree_root"`
}

// ZellijConfig holds configuration for Zellij tab integration.
// This is opt-in by default to avoid state explosion issues.
type ZellijConfig struct {
	Enabled            bool `toml:"enabled"`
	MaxTabs            int  `toml:"max_tabs"`
	CleanupIdleMinutes int  `toml:"cleanup_idle_minutes"`
}

func DefaultConfig() *Config {
	return &Config{
		Appearance:  AppearanceConfig{Theme: "digital-noir"},
		Worktrees:   WorktreesConfig{BaseBranch: "main", WorktreeRoot: "../worktrees"},
		GitHub:      GitHubConfig{AutoSync: true, SyncIntervalMinutes: 5},
		AIAgents:    AIAgentsConfig{CopilotEnabled: true, ClaudeEnabled: true},
		Zellij:      ZellijConfig{Enabled: false, MaxTabs: 10, CleanupIdleMinutes: 30}, // Opt-in by default
		LogFilePath: "",
	}
}
