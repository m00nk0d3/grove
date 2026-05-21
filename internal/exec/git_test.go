package exec

import (
	"errors"
	"testing"

	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitCommand(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		expected string
	}{
		{
			name:     "creates git command executor",
			repoPath: "/home/user/repo",
			expected: "/home/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewGitCommand(tt.repoPath)
			assert.NotNil(t, cmd)
			assert.Equal(t, tt.expected, cmd.repoPath)
		})
	}
}

func TestParseWorktreeListPorcelain(t *testing.T) {
	tests := []struct {
		name        string
		porcelain   string
		expected    []domain.Worktree
		expectError string
	}{
		{
			name: "minimal valid porcelain parse into one worktree",
			porcelain: "worktree /repo/main\n" +
				"HEAD 1111111111111111111111111111111111111111\n" +
				"branch refs/heads/main\n",
			expected: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "1111111111111111111111111111111111111111",
					Branch:    "main",
				},
			},
		},
		{
			name: "multiple worktrees with detached fallback locked marker and cleanliness marker",
			porcelain: "worktree /repo/main\n" +
				"HEAD aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
				"branch refs/heads/main\n" +
				"clean\n" +
				"\n" +
				"worktree /repo/feature\n" +
				"HEAD bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
				"detached\n" +
				"locked reason\n",
			expected: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Branch:    "main",
					IsClean:   true,
				},
				{
					Path:      "/repo/feature",
					CommitSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					Branch:    "detached",
					IsLocked:  true,
				},
			},
		},
		{
			name: "metadata line before worktree returns error",
			porcelain: "HEAD cccccccccccccccccccccccccccccccccccccccc\n" +
				"worktree /repo/main\n",
			expectError: "metadata before worktree",
		},
		{
			name: "unknown metadata ignored",
			porcelain: "worktree /repo/main\n" +
				"HEAD dddddddddddddddddddddddddddddddddddddddd\n" +
				"branch refs/heads/main\n" +
				"custom-field value\n",
			expected: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "dddddddddddddddddddddddddddddddddddddddd",
					Branch:    "main",
				},
			},
		},
		{
			name: "non heads branch ref is preserved as-is",
			porcelain: "worktree /repo/remote\n" +
				"HEAD eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n" +
				"branch refs/remotes/origin/main\n",
			expected: []domain.Worktree{
				{
					Path:      "/repo/remote",
					CommitSHA: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
					Branch:    "refs/remotes/origin/main",
				},
			},
		},
		{
			name: "final worktree is appended without trailing newline",
			porcelain: "worktree /repo/main\n" +
				"HEAD ffffffffffffffffffffffffffffffffffffffff\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /repo/feature\n" +
				"HEAD 9999999999999999999999999999999999999999\n" +
				"branch refs/heads/feature",
			expected: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "ffffffffffffffffffffffffffffffffffffffff",
					Branch:    "main",
				},
				{
					Path:      "/repo/feature",
					CommitSHA: "9999999999999999999999999999999999999999",
					Branch:    "feature",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := parseWorktreeListPorcelain(tt.porcelain)

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGitCommand_ListWorktrees(t *testing.T) {
	tests := []struct {
		name          string
		repoPath      string
		listOutput    string
		listErr       error
		statusOutputs map[string]string // worktree path -> git status --porcelain output
		statusErr     error
		want          []domain.Worktree
		expectErr     string
	}{
		{
			name:     "clean worktree is marked IsClean true",
			repoPath: "/repo/main",
			listOutput: "worktree /repo/main\n" +
				"HEAD eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n" +
				"branch refs/heads/main\n",
			statusOutputs: map[string]string{"/repo/main": ""},
			want: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
					Branch:    "main",
					IsClean:   true,
				},
			},
		},
		{
			name:     "dirty worktree is marked IsClean false",
			repoPath: "/repo/main",
			listOutput: "worktree /repo/main\n" +
				"HEAD eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n" +
				"branch refs/heads/main\n",
			statusOutputs: map[string]string{"/repo/main": " M internal/exec/git.go\n"},
			want: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
					Branch:    "main",
					IsClean:   false,
				},
			},
		},
		{
			name:     "multiple worktrees each get individual status checks",
			repoPath: "/repo/main",
			listOutput: "worktree /repo/main\n" +
				"HEAD aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
				"branch refs/heads/main\n" +
				"\n" +
				"worktree /repo/feature\n" +
				"HEAD bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
				"branch refs/heads/feature\n",
			statusOutputs: map[string]string{
				"/repo/main":    "",
				"/repo/feature": "?? untracked.go\n",
			},
			want: []domain.Worktree{
				{
					Path:      "/repo/main",
					CommitSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Branch:    "main",
					IsClean:   true,
				},
				{
					Path:      "/repo/feature",
					CommitSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					Branch:    "feature",
					IsClean:   false,
				},
			},
		},
		{
			name:      "returns list runner error",
			repoPath:  "/repo/main",
			listErr:   errors.New("git failed"),
			expectErr: "git failed",
		},
		{
			name:     "returns parse error with context",
			repoPath: "/repo/main",
			listOutput: "HEAD badbadbadbadbadbadbadbadbadbadbadbadbadb\n" +
				"worktree /repo/main\n",
			expectErr: "parse worktree porcelain: metadata before worktree",
		},
		{
			name:     "returns status runner error",
			repoPath: "/repo/main",
			listOutput: "worktree /repo/main\n" +
				"HEAD eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee\n" +
				"branch refs/heads/main\n",
			statusErr: errors.New("permission denied"),
			expectErr: "check worktree status: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(repoPath string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == "worktree" {
					return tt.listOutput, tt.listErr
				}
				// status --porcelain call
				if tt.statusErr != nil {
					return "", tt.statusErr
				}
				if tt.statusOutputs != nil {
					return tt.statusOutputs[repoPath], nil
				}
				return "", nil
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)

			actual, err := cmd.ListWorktrees()

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestGitCommand_AddWorktree(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		branch    string
		runErr    error
		wantArgs  []string
		expectErr string
	}{
		{
			name:     "invokes git worktree add with path and branch",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			branch:   "feature-branch",
			wantArgs: []string{"worktree", "add", "/repo/feature", "feature-branch"},
		},
		{
			name:      "returns runner error",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			branch:    "feature-branch",
			runErr:    errors.New("git failed"),
			wantArgs:  []string{"worktree", "add", "/repo/feature", "feature-branch"},
			expectErr: "git failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			var calledRepoPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				called = true
				calledRepoPath = repoPath
				calledArgs = append([]string{}, args...)
				return "", tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)

			err := cmd.AddWorktree(tt.path, tt.branch)

			require.True(t, called, "expected command runner to be invoked")
			assert.Equal(t, tt.repoPath, calledRepoPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGitCommand_AddWorktreeNewBranch(t *testing.T) {
	tests := []struct {
		name        string
		repoPath    string
		path        string
		branchName  string
		baseBranch  string
		fetchErr    error
		worktreeErr error
		wantCallSeq [][]string
		expectErr   string
	}{
		{
			name:       "fetches base branch then creates worktree from origin/<base>",
			repoPath:   "/repo/main",
			path:       "/repo/worktrees/feat-issue-5-modals",
			branchName: "feat/issue-5-modals",
			baseBranch: "main",
			wantCallSeq: [][]string{
				{"fetch", "origin", "main"},
				{"worktree", "add", "-b", "feat/issue-5-modals", "/repo/worktrees/feat-issue-5-modals", "origin/main"},
			},
		},
		{
			name:       "fetches feature base branch then creates worktree from origin/<base>",
			repoPath:   "/repo/main",
			path:       "/repo/worktrees/feat-issue-63",
			branchName: "feat/issue-63-my-feature",
			baseBranch: "feat/issue-61-base",
			wantCallSeq: [][]string{
				{"fetch", "origin", "feat/issue-61-base"},
				{"worktree", "add", "-b", "feat/issue-63-my-feature", "/repo/worktrees/feat-issue-63", "origin/feat/issue-61-base"},
			},
		},
		{
			name:       "falls back to local branch ref when fetch fails (offline)",
			repoPath:   "/repo/main",
			path:       "/repo/worktrees/feat-issue-5-modals",
			branchName: "feat/issue-5-modals",
			baseBranch: "main",
			fetchErr:   errors.New("network error"),
			wantCallSeq: [][]string{
				{"fetch", "origin", "main"},
				{"worktree", "add", "-b", "feat/issue-5-modals", "/repo/worktrees/feat-issue-5-modals", "main"},
			},
		},
		{
			name:        "returns error when worktree add fails",
			repoPath:    "/repo/main",
			path:        "/repo/worktrees/feat-issue-5-modals",
			branchName:  "feat/issue-5-modals",
			baseBranch:  "main",
			worktreeErr: errors.New("branch already exists"),
			expectErr:   "branch already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callSeq [][]string

			runner := func(repoPath string, args ...string) (string, error) {
				call := append([]string{}, args...)
				callSeq = append(callSeq, call)
				// First call is fetch, second is worktree add.
				if len(callSeq) == 1 && tt.fetchErr != nil {
					return "", tt.fetchErr
				}
				if len(callSeq) == 2 && tt.worktreeErr != nil {
					return "", tt.worktreeErr
				}
				return "", nil
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			err := cmd.AddWorktreeNewBranch(tt.path, tt.branchName, tt.baseBranch)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCallSeq, callSeq)
		})
	}
}

func TestGitCommand_DefaultBranch(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		runErr    error
		wantBranch string
	}{
		{
			name:       "returns main when origin/HEAD points to main",
			output:     "origin/main\n",
			wantBranch: "main",
		},
		{
			name:       "returns master when origin/HEAD points to master",
			output:     "origin/master\n",
			wantBranch: "master",
		},
		{
			name:       "falls back to main when symbolic-ref fails (no remote)",
			runErr:     errors.New("not a symbolic ref"),
			wantBranch: "main",
		},
		{
			name:       "handles ref without slash prefix gracefully",
			output:     "develop\n",
			wantBranch: "develop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(_ string, _ ...string) (string, error) {
				return tt.output, tt.runErr
			}
			cmd := NewGitCommandWithRunner("/repo", runner)
			assert.Equal(t, tt.wantBranch, cmd.DefaultBranch())
		})
	}
}

func TestGitCommand_RemoveWorktree(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		force     bool
		runErr    error
		wantArgs  []string
		expectErr string
	}{
		{
			name:     "invokes git worktree remove without force when force false",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			force:    false,
			wantArgs: []string{"worktree", "remove", "/repo/feature"},
		},
		{
			name:     "invokes git worktree remove with force when force true",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			force:    true,
			wantArgs: []string{"worktree", "remove", "--force", "/repo/feature"},
		},
		{
			name:      "returns runner error",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			force:     true,
			runErr:    errors.New("git failed"),
			wantArgs:  []string{"worktree", "remove", "--force", "/repo/feature"},
			expectErr: "git failed",
		},
		{
			name:     "places force flag before dash-prefixed path",
			repoPath: "/repo/main",
			path:     "--path-like-argument",
			force:    true,
			wantArgs: []string{"worktree", "remove", "--force", "--path-like-argument"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			var calledRepoPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				called = true
				calledRepoPath = repoPath
				calledArgs = append([]string{}, args...)
				return "", tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)

			err := cmd.RemoveWorktree(tt.path, tt.force)

			require.True(t, called, "expected command runner to be invoked")
			assert.Equal(t, tt.repoPath, calledRepoPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGitCommand_PruneWorktrees(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		runErr    error
		wantArgs  []string
		expectErr string
	}{
		{
			name:     "invokes git worktree prune",
			repoPath: "/repo/main",
			wantArgs: []string{"worktree", "prune"},
		},
		{
			name:      "returns runner error",
			repoPath:  "/repo/main",
			runErr:    errors.New("git failed"),
			wantArgs:  []string{"worktree", "prune"},
			expectErr: "git failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			var calledRepoPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				called = true
				calledRepoPath = repoPath
				calledArgs = append([]string{}, args...)
				return "", tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)

			err := cmd.PruneWorktrees()

			require.True(t, called, "expected command runner to be invoked")
			assert.Equal(t, tt.repoPath, calledRepoPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGitCommand_LockWorktree(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		reason    string
		runErr    error
		wantArgs  []string
		expectErr string
	}{
		{
			name:     "invokes git worktree lock without reason when empty",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			reason:   "",
			wantArgs: []string{"worktree", "lock", "/repo/feature"},
		},
		{
			name:     "invokes git worktree lock with reason when provided",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			reason:   "active development",
			wantArgs: []string{"worktree", "lock", "--reason", "active development", "/repo/feature"},
		},
		{
			name:      "returns runner error",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			reason:    "active development",
			runErr:    errors.New("git failed"),
			wantArgs:  []string{"worktree", "lock", "--reason", "active development", "/repo/feature"},
			expectErr: "git failed",
		},
		{
			name:     "keeps reason as single argument and path last",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			reason:   "budget variance analysis",
			wantArgs: []string{"worktree", "lock", "--reason", "budget variance analysis", "/repo/feature"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			var calledRepoPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				called = true
				calledRepoPath = repoPath
				calledArgs = append([]string{}, args...)
				return "", tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)

			err := cmd.LockWorktree(tt.path, tt.reason)

			require.True(t, called, "expected command runner to be invoked")
			assert.Equal(t, tt.repoPath, calledRepoPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGitCommand_UnlockWorktree(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		runErr    error
		wantArgs  []string
		expectErr string
	}{
		{
			name:     "invokes git worktree unlock with path",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			wantArgs: []string{"worktree", "unlock", "/repo/feature"},
		},
		{
			name:      "returns runner error",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runErr:    errors.New("git failed"),
			wantArgs:  []string{"worktree", "unlock", "/repo/feature"},
			expectErr: "git failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			var calledRepoPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				called = true
				calledRepoPath = repoPath
				calledArgs = append([]string{}, args...)
				return "", tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)

			err := cmd.UnlockWorktree(tt.path)

			require.True(t, called, "expected command runner to be invoked")
			assert.Equal(t, tt.repoPath, calledRepoPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGitCommand_GitStatus(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		runOutput string
		runErr    error
		wantArgs  []string
		wantOut   string
		expectErr string
	}{
		{
			name:      "returns trimmed short status",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runOutput: " M internal/exec/git.go\n?? new_file.go\n",
			wantArgs:  []string{"status", "--short"},
			wantOut:   "M internal/exec/git.go\n?? new_file.go",
		},
		{
			name:     "returns empty string for clean worktree",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			wantArgs: []string{"status", "--short"},
			wantOut:  "",
		},
		{
			name:      "returns runner error with context",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runErr:    errors.New("permission denied"),
			wantArgs:  []string{"status", "--short"},
			expectErr: "git status: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				calledPath = repoPath
				calledArgs = append([]string{}, args...)
				return tt.runOutput, tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			out, err := cmd.GitStatus(tt.path)

			assert.Equal(t, tt.path, calledPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestGitCommand_GitLog(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		n         int
		runOutput string
		runErr    error
		wantArgs  []string
		wantOut   string
		expectErr string
	}{
		{
			name:      "returns oneline log with n commits",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			n:         10,
			runOutput: "abc1234 feat: add context builder\ndef5678 fix: handle empty repo\n",
			wantArgs:  []string{"log", "--oneline", "-n", "10"},
			wantOut:   "abc1234 feat: add context builder\ndef5678 fix: handle empty repo",
		},
		{
			name:     "passes correct n to git log",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			n:        5,
			wantArgs: []string{"log", "--oneline", "-n", "5"},
		},
		{
			name:      "returns runner error with context",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			n:         10,
			runErr:    errors.New("not a git repository"),
			wantArgs:  []string{"log", "--oneline", "-n", "10"},
			expectErr: "git log: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				calledPath = repoPath
				calledArgs = append([]string{}, args...)
				return tt.runOutput, tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			out, err := cmd.GitLog(tt.path, tt.n)

			assert.Equal(t, tt.path, calledPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestGitCommand_GitDiffNameOnly(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		runOutput string
		runErr    error
		wantArgs  []string
		wantFiles []string
		expectErr string
	}{
		{
			name:      "returns list of changed files",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runOutput: "internal/exec/git.go\ninternal/domain/context.go\n",
			wantArgs:  []string{"diff", "--name-only", "HEAD"},
			wantFiles: []string{"internal/exec/git.go", "internal/domain/context.go"},
		},
		{
			name:      "returns empty slice when no files changed",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runOutput: "",
			wantArgs:  []string{"diff", "--name-only", "HEAD"},
			wantFiles: []string{},
		},
		{
			name:      "returns runner error with context",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runErr:    errors.New("fatal: not a git repository"),
			wantArgs:  []string{"diff", "--name-only", "HEAD"},
			expectErr: "git diff name-only: fatal: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				calledPath = repoPath
				calledArgs = append([]string{}, args...)
				return tt.runOutput, tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			files, err := cmd.GitDiffNameOnly(tt.path)

			assert.Equal(t, tt.path, calledPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantFiles, files)
		})
	}
}

func TestGitCommand_GitDiffStat(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		runOutput string
		runErr    error
		wantArgs  []string
		wantOut   string
		expectErr string
	}{
		{
			name:      "returns trimmed diff stat",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runOutput: " internal/exec/git.go | 42 ++++++\n 1 file changed, 42 insertions(+)\n",
			wantArgs:  []string{"diff", "--stat", "HEAD"},
			wantOut:   "internal/exec/git.go | 42 ++++++\n 1 file changed, 42 insertions(+)",
		},
		{
			name:     "returns empty string for no diff",
			repoPath: "/repo/main",
			path:     "/repo/feature",
			wantArgs: []string{"diff", "--stat", "HEAD"},
			wantOut:  "",
		},
		{
			name:      "returns runner error with context",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runErr:    errors.New("fatal: ambiguous argument"),
			wantArgs:  []string{"diff", "--stat", "HEAD"},
			expectErr: "git diff stat: fatal: ambiguous argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				calledPath = repoPath
				calledArgs = append([]string{}, args...)
				return tt.runOutput, tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			out, err := cmd.GitDiffStat(tt.path)

			assert.Equal(t, tt.path, calledPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestGitCommand_GitBranch(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		path      string
		runOutput string
		runErr    error
		wantArgs  []string
		wantOut   string
		expectErr string
	}{
		{
			name:      "returns trimmed branch name",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runOutput: "feat/issue-19\n",
			wantArgs:  []string{"rev-parse", "--abbrev-ref", "HEAD"},
			wantOut:   "feat/issue-19",
		},
		{
			name:      "returns HEAD for detached head state",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runOutput: "HEAD\n",
			wantArgs:  []string{"rev-parse", "--abbrev-ref", "HEAD"},
			wantOut:   "HEAD",
		},
		{
			name:      "returns runner error with context",
			repoPath:  "/repo/main",
			path:      "/repo/feature",
			runErr:    errors.New("not a git repository"),
			wantArgs:  []string{"rev-parse", "--abbrev-ref", "HEAD"},
			expectErr: "git branch: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				calledPath = repoPath
				calledArgs = append([]string{}, args...)
				return tt.runOutput, tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			out, err := cmd.GitBranch(tt.path)

			assert.Equal(t, tt.path, calledPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestRunGitCommand_IncludesOutputOnFailure(t *testing.T) {
	_, err := runGitCommand(t.TempDir(), "not-a-real-git-subcommand")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "run git not-a-real-git-subcommand")
	assert.Contains(t, err.Error(), "output:")
}

func TestGitCommand_FetchRemoteBranch(t *testing.T) {
	tests := []struct {
		name      string
		repoPath  string
		branch    string
		runErr    error
		wantArgs  []string
		expectErr string
	}{
		{
			name:     "fetches branch from origin",
			repoPath: "/repo/main",
			branch:   "feat/issue-42-my-feature",
			wantArgs: []string{"fetch", "origin", "feat/issue-42-my-feature"},
		},
		{
			name:      "propagates runner error",
			repoPath:  "/repo/main",
			branch:    "main",
			runErr:    errors.New("network error"),
			wantArgs:  []string{"fetch", "origin", "main"},
			expectErr: "fetch remote branch:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			var calledRepoPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				called = true
				calledRepoPath = repoPath
				calledArgs = append([]string{}, args...)
				return "", tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			err := cmd.FetchRemoteBranch(tt.branch)

			require.True(t, called, "expected command runner to be invoked")
			assert.Equal(t, tt.repoPath, calledRepoPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestGitCommand_CheckoutPRWorktree(t *testing.T) {
	tests := []struct {
		name        string
		repoPath    string
		path        string
		branch      string
		fetchErr    error
		worktreeErr error
		wantCallSeq [][]string
		expectErr   string
	}{
		{
			name:     "fetches then creates worktree",
			repoPath: "/repo/main",
			path:     "/worktrees/feat-issue-42",
			branch:   "feat/issue-42-my-feature",
			wantCallSeq: [][]string{
				{"fetch", "origin", "feat/issue-42-my-feature"},
				{"worktree", "add", "-B", "feat/issue-42-my-feature", "/worktrees/feat-issue-42", "origin/feat/issue-42-my-feature"},
			},
		},
		{
			name:      "returns error when fetch fails",
			repoPath:  "/repo/main",
			path:      "/worktrees/feat-issue-42",
			branch:    "feat/issue-42-my-feature",
			fetchErr:  errors.New("network error"),
			expectErr: "fetch remote branch:",
		},
		{
			name:        "returns error when worktree add fails",
			repoPath:    "/repo/main",
			path:        "/worktrees/feat-issue-42",
			branch:      "feat/issue-42-my-feature",
			worktreeErr: errors.New("worktree already exists"),
			expectErr:   "checkout pr worktree:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var callSeq [][]string

			runner := func(repoPath string, args ...string) (string, error) {
				call := append([]string{}, args...)
				callSeq = append(callSeq, call)
				// First call is fetch, second is worktree add.
				if len(callSeq) == 1 && tt.fetchErr != nil {
					return "", tt.fetchErr
				}
				if len(callSeq) == 2 && tt.worktreeErr != nil {
					return "", tt.worktreeErr
				}
				return "", nil
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			err := cmd.CheckoutPRWorktree(tt.path, tt.branch)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantCallSeq, callSeq)
		})
	}
}

func TestGitCommand_ListModifiedFiles(t *testing.T) {
	tests := []struct {
		name       string
		repoPath   string
		targetPath string
		output     string
		runErr     error
		wantFiles  []string
		wantArgs   []string
		expectErr  string
	}{
		{
			name:       "invokes correct git ls-files args",
			repoPath:   "/repo/main",
			targetPath: "/repo/feature",
			output:     "",
			wantArgs:   []string{"ls-files", "--modified", "--others", "--exclude-standard"},
			wantFiles:  nil,
		},
		{
			name:       "parses single modified file",
			repoPath:   "/repo/main",
			targetPath: "/repo/feature",
			output:     "main.go\n",
			wantArgs:   []string{"ls-files", "--modified", "--others", "--exclude-standard"},
			wantFiles:  []string{"main.go"},
		},
		{
			name:       "parses multiple files and trims blank lines",
			repoPath:   "/repo/main",
			targetPath: "/repo/feature",
			output:     "main.go\ngo.mod\nREADME.md\n",
			wantArgs:   []string{"ls-files", "--modified", "--others", "--exclude-standard"},
			wantFiles:  []string{"main.go", "go.mod", "README.md"},
		},
		{
			name:       "returns nil slice for empty output",
			repoPath:   "/repo/main",
			targetPath: "/repo/feature",
			output:     "\n\n",
			wantArgs:   []string{"ls-files", "--modified", "--others", "--exclude-standard"},
			wantFiles:  nil,
		},
		{
			name:       "propagates runner error",
			repoPath:   "/repo/main",
			targetPath: "/repo/feature",
			runErr:     errors.New("git failed"),
			wantArgs:   []string{"ls-files", "--modified", "--others", "--exclude-standard"},
			expectErr:  "list modified files",
		},
		{
			name:       "uses target path not repo path as runner arg",
			repoPath:   "/repo/main",
			targetPath: "/repo/feature-branch",
			output:     "cmd/nexus/app.go\n",
			wantArgs:   []string{"ls-files", "--modified", "--others", "--exclude-standard"},
			wantFiles:  []string{"cmd/nexus/app.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calledPath string
			var calledArgs []string

			runner := func(repoPath string, args ...string) (string, error) {
				calledPath = repoPath
				calledArgs = append([]string{}, args...)
				return tt.output, tt.runErr
			}

			cmd := NewGitCommandWithRunner(tt.repoPath, runner)
			files, err := cmd.ListModifiedFiles(tt.targetPath)

			assert.Equal(t, tt.targetPath, calledPath)
			assert.Equal(t, tt.wantArgs, calledArgs)

			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantFiles, files)
		})
	}
}
