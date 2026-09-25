package domain

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGitHubConfig_SyncInterval(t *testing.T) {
	tests := []struct {
		name    string
		minutes int
		wantDur time.Duration
	}{
		{
			name:    "zero falls back to 5 minutes",
			minutes: 0,
			wantDur: 5 * time.Minute,
		},
		{
			name:    "negative falls back to 5 minutes",
			minutes: -1,
			wantDur: 5 * time.Minute,
		},
		{
			name:    "positive value is used as-is",
			minutes: 10,
			wantDur: 10 * time.Minute,
		},
		{
			name:    "one minute is the minimum valid positive value",
			minutes: 1,
			wantDur: 1 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GitHubConfig{SyncIntervalMinutes: tt.minutes}
			assert.Equal(t, tt.wantDur, cfg.SyncInterval())
		})
	}
}

func TestWorktreesConfig_WorktreePath(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "src", "grove")
	absRoot := filepath.Join(base, "wt")
	beside := filepath.Join(base, "src")
	tests := []struct {
		name string
		root string
		want string
	}{
		{name: "empty root uses the default beside the repository", root: "", want: filepath.Join(beside, "worktrees", "grove", "feat-x")},
		{name: "default root matches the historical layout", root: DefaultWorktreeRoot, want: filepath.Join(beside, "worktrees", "grove", "feat-x")},
		{name: "relative root resolves against the repository", root: ".worktrees", want: filepath.Join(repo, ".worktrees", "grove", "feat-x")},
		{name: "absolute root is used as is", root: absRoot, want: filepath.Join(absRoot, "grove", "feat-x")},
		{name: "surrounding whitespace is ignored", root: "  ../trees  ", want: filepath.Join(beside, "trees", "grove", "feat-x")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, WorktreesConfig{WorktreeRoot: tt.root}.WorktreePath(repo, "feat-x"))
		})
	}
}
