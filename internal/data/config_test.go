package data

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validTOML is a well-formed config file covering all sections.
const validTOML = `
[github]
auto_sync = false
sync_interval_minutes = 10

[appearance]
theme = "dark-mode"

[ai_agents]
copilot_enabled = true
claude_enabled  = false
aider_enabled   = true
claude_binary   = "/usr/local/bin/claude"

[worktrees]
base_branch   = "develop"
worktree_root = "/tmp/wt"
`

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns path to pass to LoadConfig
		wantErr   bool
		wantCheck func(t *testing.T, cfg *domain.Config)
	}{
		{
			name: "valid TOML returns parsed config",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.toml")
				require.NoError(t, os.WriteFile(path, []byte(validTOML), 0o644))
				return path
			},
			wantErr: false,
			wantCheck: func(t *testing.T, cfg *domain.Config) {
				t.Helper()
				assert.Equal(t, false, cfg.GitHub.AutoSync)
				assert.Equal(t, 10, cfg.GitHub.SyncIntervalMinutes)
				assert.Equal(t, "dark-mode", cfg.Appearance.Theme)
				assert.Equal(t, true, cfg.AIAgents.CopilotEnabled)
				assert.Equal(t, false, cfg.AIAgents.ClaudeEnabled)
				assert.Equal(t, true, cfg.AIAgents.AiderEnabled)
				assert.Equal(t, "/usr/local/bin/claude", cfg.AIAgents.ClaudeBinary)
				assert.Equal(t, "develop", cfg.Worktrees.BaseBranch)
				assert.Equal(t, "/tmp/wt", cfg.Worktrees.WorktreeRoot)
			},
		},
		{
			name: "missing file returns default config",
			setup: func(t *testing.T) string {
				t.Helper()
				// Return a path that does not exist.
				return filepath.Join(t.TempDir(), "nonexistent", "config.toml")
			},
			wantErr: false,
			wantCheck: func(t *testing.T, cfg *domain.Config) {
				t.Helper()
				defaults := domain.DefaultConfig()
				assert.Equal(t, defaults.Appearance.Theme, cfg.Appearance.Theme)
				assert.Equal(t, defaults.Worktrees.BaseBranch, cfg.Worktrees.BaseBranch)
				assert.Equal(t, defaults.Worktrees.WorktreeRoot, cfg.Worktrees.WorktreeRoot)
				assert.Equal(t, defaults.GitHub.AutoSync, cfg.GitHub.AutoSync)
				assert.Equal(t, defaults.GitHub.SyncIntervalMinutes, cfg.GitHub.SyncIntervalMinutes)
			},
		},
		{
			name: "partial TOML without ai_agents section inherits defaults",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.toml")
				// No [ai_agents] section — CopilotEnabled must still be true (default).
				partial := "[appearance]\ntheme = \"matrix\"\n"
				require.NoError(t, os.WriteFile(path, []byte(partial), 0o644))
				return path
			},
			wantErr: false,
			wantCheck: func(t *testing.T, cfg *domain.Config) {
				t.Helper()
				assert.Equal(t, "matrix", cfg.Appearance.Theme)
				assert.Equal(t, true, cfg.AIAgents.CopilotEnabled, "CopilotEnabled must default to true when not specified in config")
				assert.Equal(t, true, cfg.AIAgents.ClaudeEnabled, "ClaudeEnabled must default to true when not specified in config")
			},
		},
		{
			name: "invalid TOML returns error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "config.toml")
				require.NoError(t, os.WriteFile(path, []byte("[[[[not valid toml"), 0o644))
				return path
			},
			wantErr: true,
			wantCheck: func(t *testing.T, cfg *domain.Config) {
				// cfg should be nil on error — nothing to check.
				assert.Nil(t, cfg)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			cfg, err := LoadConfig(path)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
			}
			tt.wantCheck(t, cfg)
		})
	}
}

func TestSaveConfig(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (cfg *domain.Config, path string)
	}{
		{
			name: "SaveConfig creates file and round-trips via LoadConfig",
			setup: func(t *testing.T) (*domain.Config, string) {
				t.Helper()
				dir := t.TempDir()
				cfg := &domain.Config{
					GitHub: domain.GitHubConfig{
						AutoSync:            true,
						SyncIntervalMinutes: 15,
					},
					Appearance: domain.AppearanceConfig{Theme: "digital-noir"},
					AIAgents: domain.AIAgentsConfig{
						CopilotEnabled: true,
						ClaudeEnabled:  true,
						AiderEnabled:   false,
						ClaudeBinary:   "claude",
					},
					Worktrees: domain.WorktreesConfig{
						BaseBranch:   "main",
						WorktreeRoot: "../worktrees",
					},
				}
				return cfg, filepath.Join(dir, "config.toml")
			},
		},
		{
			name: "SaveConfig creates directory if it does not exist",
			setup: func(t *testing.T) (*domain.Config, string) {
				t.Helper()
				// Nest two levels deep — neither exists yet.
				dir := filepath.Join(t.TempDir(), "new-parent", ".grove")
				cfg := domain.DefaultConfig()
				return cfg, filepath.Join(dir, "config.toml")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, path := tt.setup(t)

			err := SaveConfig(cfg, path)
			require.NoError(t, err)

			// File must exist after SaveConfig.
			_, statErr := os.Stat(path)
			require.NoError(t, statErr, "expected file to exist at %s", path)

			// Atomic write: the .tmp file must NOT remain after a successful save.
			tmpPath := path + ".tmp"
			_, tmpStatErr := os.Stat(tmpPath)
			assert.True(t, os.IsNotExist(tmpStatErr), "expected .tmp file to be removed after atomic rename, but it exists at %s", tmpPath)

			// Round-trip: reload and verify a representative field.
			loaded, err := LoadConfig(path)
			require.NoError(t, err)
			require.NotNil(t, loaded)
			assert.Equal(t, cfg.Appearance.Theme, loaded.Appearance.Theme)
			assert.Equal(t, cfg.Worktrees.BaseBranch, loaded.Worktrees.BaseBranch)
			assert.Equal(t, cfg.GitHub.SyncIntervalMinutes, loaded.GitHub.SyncIntervalMinutes)
			assert.Equal(t, cfg.AIAgents.CopilotEnabled, loaded.AIAgents.CopilotEnabled)
			assert.Equal(t, cfg.AIAgents.ClaudeBinary, loaded.AIAgents.ClaudeBinary)
		})
	}
}
