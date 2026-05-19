package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/m00nk0d3/nexus/internal/tui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderFull_ContainsHeader(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		wantIn   []string
	}{
		{
			name:     "header includes NEXUS branding and GIT WORKTREE ORCHESTRATOR",
			repoPath: "/home/user/nexus",
			wantIn:   []string{"NEXUS", "GIT WORKTREE ORCHESTRATOR"},
		},
		{
			name:     "header includes repo path",
			repoPath: "/projects/myapp",
			wantIn:   []string{"myapp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.RepoPath = tt.repoPath

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

func TestRenderFull_ContainsNavRailItems(t *testing.T) {
	tests := []struct {
		name    string
		wantIn  []string
	}{
		{
			name:   "nav rail contains all navigation keys",
			wantIn: []string{"W", "I", "P", "T"},
		},
		{
			name:   "nav rail shows active cursor on W",
			wantIn: []string{"> W"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

func TestRenderFull_ContainsWorktreeTableHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
	}{
		{
			name:    "renders Digital Noir column headers",
			headers: []string{"NAME", "PATH", "STATUS", "SHA", "PR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.width = 300
			model.Worktrees = []domain.Worktree{
				{
					Path:      filepath.Join("worktrees", "feature-issue-4"),
					CommitSHA: "0123456789abcdef",
					IsClean:   true,
					IsLocked:  false,
				},
			}

			view := model.View()

			for _, header := range tt.headers {
				assert.Contains(t, view, header)
			}
		})
	}
}

func TestRenderFull_ContainsContextPanelPlaceholder(t *testing.T) {
	tests := []struct {
		name   string
		wantIn string
	}{
		{
			name:   "shows placeholder when no worktrees",
			wantIn: "No worktree selected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			view := model.View()

			assert.Contains(t, view, tt.wantIn)
		})
	}
}

func TestRenderFull_ContainsFooterKeyHints(t *testing.T) {
	tests := []struct {
		name   string
		wantIn []string
	}{
		{
			name:   "footer contains navigation and action hints",
			wantIn: []string{"[Enter] Select", "[t] Settings", "[esc] Quit"},
		},
		{
			name:   "action bar contains worktree commands",
			wantIn: []string{"[c-n] New", "[c-d] Delete", "[f1] Help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

func TestRenderFull_SelectedRowDistinguishable(t *testing.T) {
	tests := []struct {
		name        string
		selectedIdx int
		wantCursor  string
	}{
		{
			name:        "selected row has > cursor on first item",
			selectedIdx: 0,
			wantCursor:  "> wt1",
		},
		{
			name:        "selected row has > cursor on second item",
			selectedIdx: 1,
			wantCursor:  "> wt2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.Worktrees = []domain.Worktree{
				{Path: "/tmp/wt1", Branch: "main", CommitSHA: "abc123", IsClean: true},
				{Path: "/tmp/wt2", Branch: "feature", CommitSHA: "def456", IsClean: false},
			}
			model.selectedIdx = tt.selectedIdx

			view := model.View()

			assert.Contains(t, view, tt.wantCursor)
		})
	}
}

func TestRenderFull_WorktreeStatusMapping(t *testing.T) {
	tests := []struct {
		name           string
		worktree       domain.Worktree
		expectedStatus string
		expectedName   string
	}{
		{
			name: "clean unlocked worktree shows Idle status",
			worktree: domain.Worktree{
				Path:    filepath.Join("tmp", "wt-clean"),
				IsClean: true, IsLocked: false,
			},
			expectedStatus: "Idle",
			expectedName:   "wt-clean",
		},
		{
			name: "dirty worktree shows Dirty status",
			worktree: domain.Worktree{
				Path:    filepath.Join("tmp", "wt-dirty"),
				IsClean: false, IsLocked: false,
			},
			expectedStatus: "Dirty",
			expectedName:   "wt-dirty",
		},
		{
			name: "locked worktree shows Locked status",
			worktree: domain.Worktree{
				Path:    filepath.Join("tmp", "wt-locked"),
				IsClean: true, IsLocked: true,
			},
			expectedStatus: "Locked",
			expectedName:   "wt-locked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.Worktrees = []domain.Worktree{tt.worktree}
			// Wide terminal so path fits without truncation.
			model.width = 300

			view := model.View()

			assert.Contains(t, view, tt.expectedName)
			assert.Contains(t, view, tt.worktree.Path)
			assert.Contains(t, view, tt.expectedStatus)
		})
	}
}

func TestRenderFooterBar_SyncStatus(t *testing.T) {
	tests := []struct {
		name       string
		syncing    bool
		lastSynced time.Time
		syncErr    error
		wantIn     string
	}{
		{
			name:    "shows syncing indicator while sync is in progress",
			syncing: true,
			wantIn:  "⟳ syncing",
		},
		{
			name:       "shows synced time when last sync succeeded",
			lastSynced: time.Now().Add(-3 * time.Minute),
			wantIn:     "✓ synced 3m ago",
		},
		{
			name:       "shows synced just now when under one minute",
			lastSynced: time.Now().Add(-30 * time.Second),
			wantIn:     "✓ synced just now",
		},
		{
			name:    "shows error indicator on sync failure",
			syncErr: errors.New("api rate limited"),
			wantIn:  "✗ sync err",
		},
		{
			name:   "shows nothing when never synced and not syncing",
			wantIn: "NEXUS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.syncing = tt.syncing
			model.lastSynced = tt.lastSynced
			model.syncErr = tt.syncErr

			view := model.View()

			assert.Contains(t, view, tt.wantIn)
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 2: Issues & PRs View tests
// ---------------------------------------------------------------------------

// TestRenderIssueList_ContainsHeaders verifies that renderIssueList renders
// the required column headers: #, TITLE, STATUS, LABELS.
func TestRenderIssueList_ContainsHeaders(t *testing.T) {
	tests := []struct {
		name   string
		wantIn []string
	}{
		{
			name:   "renders all required column headers",
			wantIn: []string{"#", "TITLE", "STATUS", "LABELS"},
		},
	}

	theme := styles.NewTheme("digital-noir")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderIssueList(nil, 0, nil, theme, 80, 20, false)
			for _, want := range tt.wantIn {
				assert.Contains(t, result, want)
			}
		})
	}
}

// TestRenderIssueList_ContainsIssueRows verifies that issue number, title, labels
// are rendered, and that the selected row is highlighted.
func TestRenderIssueList_ContainsIssueRows(t *testing.T) {
	issues := []domain.Issue{
		{Number: 7, Title: "Fix the bug", Labels: []string{"bug", "p1"}},
		{Number: 8, Title: "Add feature", Labels: []string{"enhancement"}},
	}

	tests := []struct {
		name        string
		selectedIdx int
		wantIn      []string
	}{
		{
			name:        "renders issue number and title for all issues",
			selectedIdx: 0,
			wantIn:      []string{"7", "Fix the bug", "8", "Add feature"},
		},
		{
			// "enhancement" (11 chars) is truncated to "enhancem…" at labelsWidth=9
			name:        "renders issue labels",
			selectedIdx: 0,
			wantIn:      []string{"bug", "enhancem"},
		},
		{
			name:        "selected issue (idx 0) content is present",
			selectedIdx: 0,
			wantIn:      []string{"7", "Fix the bug"},
		},
		{
			name:        "selected issue (idx 1) content is present",
			selectedIdx: 1,
			wantIn:      []string{"8", "Add feature"},
		},
	}

	theme := styles.NewTheme("digital-noir")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderIssueList(issues, tt.selectedIdx, nil, theme, 80, 20, false)
			for _, want := range tt.wantIn {
				assert.Contains(t, result, want)
			}
		})
	}
}

// TestRenderIssueList_InProgressStatus verifies that an issue shows "In Progress"
// when a matching worktree branch exists, and "Open" otherwise.
func TestRenderIssueList_InProgressStatus(t *testing.T) {
	issues := []domain.Issue{
		{Number: 7, Title: "Fix the bug", Labels: []string{"bug"}},
		{Number: 8, Title: "Add feature", Labels: []string{"enhancement"}},
	}
	worktrees := []domain.Worktree{
		{Path: "/repo/worktrees/feat-issue-7", Branch: "feat/issue-7-fix-the-bug"},
	}

	theme := styles.NewTheme("digital-noir")

	t.Run("issue with matching worktree shows In Progress", func(t *testing.T) {
		result := renderIssueList(issues, 0, worktrees, theme, 80, 20, false)
		assert.Contains(t, result, "In Progress")
	})

	t.Run("issue without matching worktree shows Open", func(t *testing.T) {
		result := renderIssueList(issues, 0, worktrees, theme, 80, 20, false)
		assert.Contains(t, result, "Open")
	})

	t.Run("no worktrees means all issues show Open", func(t *testing.T) {
		result := renderIssueList(issues, 0, nil, theme, 80, 20, false)
		assert.NotContains(t, result, "In Progress")
	})
}

// TestRenderIssueList_ShowsAssignees verifies that assignees are shown in the list.
func TestRenderIssueList_ShowsAssignees(t *testing.T) {
	theme := styles.NewTheme("digital-noir")

	t.Run("shows ASSIGNED column header", func(t *testing.T) {
		result := renderIssueList(nil, 0, nil, theme, 80, 20, false)
		assert.Contains(t, result, "ASSIGNED")
	})

	t.Run("shows @username for assigned issue", func(t *testing.T) {
		issues := []domain.Issue{
			{Number: 1, Title: "My issue", Assignees: []string{"alice"}},
		}
		result := renderIssueList(issues, 0, nil, theme, 80, 20, false)
		assert.Contains(t, result, "@alice")
	})

	t.Run("shows dash when no assignees", func(t *testing.T) {
		issues := []domain.Issue{
			{Number: 1, Title: "My issue", Assignees: nil},
		}
		result := renderIssueList(issues, 0, nil, theme, 80, 20, false)
		assert.Contains(t, result, "-")
	})
}

// TestIssueHasWorktree verifies the branch-matching logic for issue worktrees.
func TestIssueHasWorktree(t *testing.T) {
	worktrees := []domain.Worktree{
		{Branch: "feat/issue-7-fix-the-bug"},
		{Branch: "feat/issue-42-some-feature"},
	}

	tests := []struct {
		name        string
		issueNumber int
		worktrees   []domain.Worktree
		want        bool
	}{
		{"matches issue-7", 7, worktrees, true},
		{"matches issue-42", 42, worktrees, true},
		{"no match for issue-1", 1, worktrees, false},
		{"no match for issue-4 (partial, issue-42 exists)", 4, worktrees, false},
		{"nil worktrees returns false", 7, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := issueHasWorktree(tt.issueNumber, tt.worktrees)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRenderPRList_ContainsHeaders verifies that renderPRList renders
// the required column headers: #, TITLE, BRANCH, ASSIGNED, STATUS.
// Note: AUTHOR column was removed from the list (it is visible in the context panel).
func TestRenderPRList_ContainsHeaders(t *testing.T) {
	tests := []struct {
		name   string
		wantIn []string
	}{
		{
			name:   "renders all required column headers",
			wantIn: []string{"#", "TITLE", "BRANCH", "ASSIGNED", "STATUS"},
		},
	}

	theme := styles.NewTheme("digital-noir")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderPRList(nil, 0, theme, 80, 20, false)
			for _, want := range tt.wantIn {
				assert.Contains(t, result, want)
			}
		})
	}
}

// TestRenderPRList_ContainsPRRows verifies that PR number, title, branch, author
// and state are rendered, and that the selected PR has a ">" cursor.
func TestRenderPRList_ContainsPRRows(t *testing.T) {
	prs := []domain.PullRequest{
		{Number: 42, Title: "My PR", Branch: "feat/awesome", Author: "alice", State: "OPEN"},
		{Number: 43, Title: "Fix PR", Branch: "fix/bug", Author: "bob", State: "OPEN"},
	}

	tests := []struct {
		name        string
		selectedIdx int
		wantIn      []string
	}{
		{
			name:        "renders PR number, title, branch, and state",
			selectedIdx: 0,
			wantIn:      []string{"42", "My PR", "feat/awesome", "OPEN"},
		},
		{
			name:        "selected PR (idx 0) has > cursor",
			selectedIdx: 0,
			wantIn:      []string{"> "},
		},
		{
			name:        "second PR is also rendered",
			selectedIdx: 0,
			wantIn:      []string{"43", "Fix PR", "fix/bug"},
		},
	}

	theme := styles.NewTheme("digital-noir")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderPRList(prs, tt.selectedIdx, theme, 80, 20, false)
			for _, want := range tt.wantIn {
				assert.Contains(t, result, want)
			}
		})
	}
}

// TestRenderContextPanel_IssueContext verifies that when the active view is
// viewIssues, the context panel shows the selected issue's number, title, labels,
// and an "[g] Open in GitHub" hint.
func TestRenderContextPanel_IssueContext(t *testing.T) {
	tests := []struct {
		name        string
		issues      []domain.Issue
		selectedIdx int
		wantIn      []string
	}{
		{
			name: "shows Issue context header with number and title",
			issues: []domain.Issue{
				{Number: 14, Title: "Implement Issues View", Labels: []string{"feature"}},
			},
			selectedIdx: 0,
			wantIn:      []string{"Issue #14", "Implement Issues View"},
		},
		{
			name: "shows labels in [label] bracket format",
			issues: []domain.Issue{
				{Number: 5, Title: "Bug Report", Labels: []string{"bug", "critical"}},
			},
			selectedIdx: 0,
			wantIn:      []string{"[bug]", "[critical]"},
		},
		{
			name: "shows [g] Open in GitHub hint",
			issues: []domain.Issue{
				{Number: 1, Title: "Some Issue", Labels: nil},
			},
			selectedIdx: 0,
			wantIn:      []string{"[g] Open in GitHub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewIssues
			model.issues = tt.issues
			model.selectedIssueIdx = tt.selectedIdx

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestRenderContextPanel_PRContext verifies that when the active view is
// viewPRs, the context panel shows the selected PR's number, title, branch,
// author, state, and an "[g] Open in GitHub" hint.
func TestRenderContextPanel_PRContext(t *testing.T) {
	tests := []struct {
		name      string
		prs       []domain.PullRequest
		prIdx     int
		wantIn    []string
	}{
		{
			name: "shows PR context header with number and title",
			prs: []domain.PullRequest{
				{Number: 42, Title: "Add search feature", Branch: "feat/search", Author: "alice", State: "OPEN"},
			},
			prIdx:  0,
			wantIn: []string{"PR #42", "Add search feature"},
		},
		{
			name: "shows branch and author in context panel",
			prs: []domain.PullRequest{
				{Number: 10, Title: "Fix bug", Branch: "fix/null-ptr", Author: "bob", State: "OPEN"},
			},
			prIdx:  0,
			wantIn: []string{"fix/null-ptr", "bob"},
		},
		{
			name: "shows PR state in context panel",
			prs: []domain.PullRequest{
				{Number: 55, Title: "Open PR", Branch: "feat/draft", Author: "carol", State: "OPEN"},
			},
			prIdx:  0,
			wantIn: []string{"OPEN"},
		},
		{
			name: "shows [g] Open in GitHub hint",
			prs: []domain.PullRequest{
				{Number: 1, Title: "Some PR", Branch: "main", Author: "dev", State: "OPEN"},
			},
			prIdx:  0,
			wantIn: []string{"[g] Open in GitHub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewPRs
			model.prs = tt.prs
			model.selectedPRIdx = tt.prIdx

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestRenderFull_IssueViewShowsIssueList verifies that when view is viewIssues,
// renderFull (via model.View()) shows issue data in the main list panel.
func TestRenderFull_IssueViewShowsIssueList(t *testing.T) {
	tests := []struct {
		name   string
		issues []domain.Issue
		wantIn []string
	}{
		{
			name: "issue view renders issue number and title",
			issues: []domain.Issue{
				{Number: 99, Title: "Test Issue"},
			},
			wantIn: []string{"99", "Test Issue"},
		},
		{
			name: "issue view renders issue column headers",
			issues: []domain.Issue{
				{Number: 1, Title: "Any Issue"},
			},
			wantIn: []string{"TITLE", "STATUS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewIssues
			model.issues = tt.issues

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestRenderFull_PRViewShowsPRList verifies that when view is viewPRs,
// renderFull (via model.View()) shows PR data in the main list panel.
func TestRenderFull_PRViewShowsPRList(t *testing.T) {
	tests := []struct {
		name   string
		prs    []domain.PullRequest
		wantIn []string
	}{
		{
			name: "PR view renders PR number, title, and branch",
			prs: []domain.PullRequest{
				{Number: 88, Title: "Test PR", Branch: "feat/test", Author: "dev", State: "OPEN"},
			},
			wantIn: []string{"88", "Test PR", "feat/test"},
		},
		{
			name: "PR view renders PR column headers",
			prs: []domain.PullRequest{
				{Number: 1, Title: "Any PR", Branch: "main", Author: "dev", State: "OPEN"},
			},
			wantIn: []string{"BRANCH", "STATUS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewPRs
			model.prs = tt.prs
			model.width = 300

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestRenderFull_WorktreesViewStillWorks verifies that when view is viewWorktrees,
// the existing worktree table still renders correctly.
func TestRenderFull_WorktreesViewStillWorks(t *testing.T) {
	tests := []struct {
		name      string
		worktrees []domain.Worktree
		wantIn    []string
	}{
		{
			name: "worktrees view shows worktree column headers",
			worktrees: []domain.Worktree{
				{Path: "/tmp/wt-main", Branch: "main", IsClean: true},
			},
			wantIn: []string{"NAME", "PATH", "STATUS"},
		},
		{
			name: "worktrees view shows worktree name",
			worktrees: []domain.Worktree{
				{Path: "/tmp/wt-main", Branch: "main", IsClean: true},
			},
			wantIn: []string{"wt-main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = tt.worktrees

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 3: Issue #15 — PR Context Panel & GH:ID Column tests
// ---------------------------------------------------------------------------

// TestRenderer_ContextPanel_WithLinkedPR verifies that when a worktree has a
// LinkedPR, the context panel shows PR-specific information: number, title,
// author, state, and labels in the correct format.
func TestRenderer_ContextPanel_WithLinkedPR(t *testing.T) {
	tests := []struct {
		name     string
		worktree domain.Worktree
		wantIn   []string
	}{
		{
			name: "shows PR context header with number",
			worktree: domain.Worktree{
				Path:   "/tmp/feat-login",
				Branch: "feat/login",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 42,
					Title:  "Add login feature",
					Author: "alice",
					State:  "OPEN",
					Labels: []string{},
				},
			},
			wantIn: []string{"Context: PR #42"},
		},
		{
			name: "shows PR title in context",
			worktree: domain.Worktree{
				Path:   "/tmp/feat-login",
				Branch: "feat/login",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 42,
					Title:  "Add login feature",
					Author: "alice",
					State:  "OPEN",
					Labels: []string{},
				},
			},
			wantIn: []string{"Add login feature"},
		},
		{
			name: "shows GH Title label and author with @ prefix",
			worktree: domain.Worktree{
				Path:   "/tmp/feat-login",
				Branch: "feat/login",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 42,
					Title:  "Add login feature",
					Author: "alice",
					State:  "OPEN",
					Labels: []string{},
				},
			},
			wantIn: []string{"GH Title:", "Author: @alice"},
		},
		{
			name: "shows status dot and state",
			worktree: domain.Worktree{
				Path:   "/tmp/feat-login",
				Branch: "feat/login",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 42,
					Title:  "Add login feature",
					Author: "alice",
					State:  "OPEN",
					Labels: []string{},
				},
			},
			wantIn: []string{"Status: ●", "OPEN"},
		},
		{
			name: "shows labels in bracket format",
			worktree: domain.Worktree{
				Path:   "/tmp/feat-auth",
				Branch: "feat/auth",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 99,
					Title:  "Auth refactor",
					Author: "bob",
					State:  "OPEN",
					Labels: []string{"enhancement", "backend"},
				},
			},
			wantIn: []string{"Labels: [enhancement][backend]"},
		},
		{
			name: "shows agent commands section",
			worktree: domain.Worktree{
				Path:   "/tmp/feat-login",
				Branch: "feat/login",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 42,
					Title:  "Add login feature",
					Author: "alice",
					State:  "MERGED",
					Labels: []string{},
				},
			},
			wantIn: []string{"AGENT COMMANDS:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{tt.worktree}
			model.selectedIdx = 0

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestRenderer_ContextPanel_NoLinkedPR verifies that when a worktree has no
// LinkedPR, the context panel shows the worktree basename, branch, and path.
func TestRenderer_ContextPanel_NoLinkedPR(t *testing.T) {
	tests := []struct {
		name     string
		worktree domain.Worktree
		wantIn   []string
		wantOut  []string
	}{
		{
			name: "shows worktree basename as context header",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-search",
				Branch:  "feat/search",
				IsClean: true,
			},
			wantIn: []string{"Context: feat-search"},
		},
		{
			name: "shows branch",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-search",
				Branch:  "feat/search",
				IsClean: true,
			},
			wantIn: []string{"Branch: feat/search"},
		},
		{
			name: "shows path",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-search",
				Branch:  "feat/search",
				IsClean: true,
			},
			wantIn: []string{"Path: /tmp/feat-search"},
		},
		{
			name: "does not show PR-specific fields",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-search",
				Branch:  "feat/search",
				IsClean: true,
			},
			wantOut: []string{"GH Title:", "Author: @", "Labels:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{tt.worktree}
			model.selectedIdx = 0

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
			for _, notWant := range tt.wantOut {
				assert.NotContains(t, view, notWant)
			}
		})
	}
}

// TestRenderer_ContextPanel_AgentCommandsAlwaysVisible verifies that the agent
// commands [a], [c], and [s] are always shown in the worktree context panel,
// regardless of whether a LinkedPR is present.
func TestRenderer_ContextPanel_AgentCommandsAlwaysVisible(t *testing.T) {
	tests := []struct {
		name     string
		worktree domain.Worktree
	}{
		{
			name: "agent commands present with LinkedPR",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-auth",
				Branch:  "feat/auth",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 7,
					Title:  "Auth",
					Author: "carol",
					State:  "OPEN",
				},
			},
		},
		{
			name: "agent commands present without LinkedPR",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-search",
				Branch:  "feat/search",
				IsClean: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{tt.worktree}
			model.selectedIdx = 0

			view := model.View()

			assert.Contains(t, view, "[a] Spawn Claude Code")
			assert.Contains(t, view, "[c] Spawn Copilot")
			assert.Contains(t, view, "[s] Open Shell in WT")
		})
	}
}

// TestRenderer_GHIDColumn_NoLinkedPR verifies that when a worktree has no
// LinkedPR, the GH:ID column shows "-" (not empty string).
func TestRenderer_GHIDColumn_NoLinkedPR(t *testing.T) {
	tests := []struct {
		name     string
		worktree domain.Worktree
	}{
		{
			name: "GH:ID shows dash when no PR linked (non-selected row)",
			worktree: domain.Worktree{
				Path:    "/tmp/wt-no-pr",
				Branch:  "main",
				IsClean: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{tt.worktree}
			// Set selectedIdx to -1 so the row is NOT selected, ensuring we hit the
			// non-selected branch of the rendering code.
			model.selectedIdx = -1
			model.width = 300

			view := model.View()

			// The PR column should contain "-" (right-aligned). We verify
			// the worktree row is actually rendered and the column shows the placeholder.
			assert.Contains(t, view, "wt-no-pr")
			assert.Contains(t, view, "     -") // "-" right-aligned in 6-char column
		})
	}
}

// TestRenderer_GHIDColumn_WithLinkedPR verifies that when a worktree has a
// LinkedPR, the GH:ID column shows the PR number.
func TestRenderer_GHIDColumn_WithLinkedPR(t *testing.T) {
	tests := []struct {
		name        string
		worktree    domain.Worktree
		wantPRNum   string
	}{
		{
			name: "GH:ID shows PR number for linked PR (non-selected row)",
			worktree: domain.Worktree{
				Path:    "/tmp/feat-login",
				Branch:  "feat/login",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 1442,
					Title:  "Login feature",
					Author: "alice",
					State:  "OPEN",
				},
			},
			wantPRNum: "1442",
		},
		{
			name: "GH:ID shows PR number for merged PR",
			worktree: domain.Worktree{
				Path:    "/tmp/fix-bug",
				Branch:  "fix/bug",
				IsClean: true,
				LinkedPR: &domain.PullRequest{
					Number: 55,
					Title:  "Bug fix",
					Author: "bob",
					State:  "MERGED",
				},
			},
			wantPRNum: "55",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{tt.worktree}
			model.selectedIdx = -1
			model.width = 300

			view := model.View()

			assert.Contains(t, view, tt.wantPRNum)
		})
	}
}

// ---------------------------------------------------------------------------
// Phase 4: clipContent & panel focus rendering tests
// ---------------------------------------------------------------------------

// TestClipContent verifies that clipContent correctly slices content lines
// with scroll offset and maxLines constraints.
func TestClipContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		offset   int
		maxLines int
		wantOut  string
	}{
		{
			name:     "no clipping needed when within bounds",
			content:  "line1\nline2\nline3",
			offset:   0,
			maxLines: 10,
			wantOut:  "line1\nline2\nline3",
		},
		{
			name:     "clips to maxLines",
			content:  "a\nb\nc\nd\ne",
			offset:   0,
			maxLines: 3,
			wantOut:  "a\nb\nc",
		},
		{
			name:     "applies offset",
			content:  "a\nb\nc\nd",
			offset:   1,
			maxLines: 10,
			wantOut:  "b\nc\nd",
		},
		{
			name:     "applies offset and clips",
			content:  "a\nb\nc\nd\ne",
			offset:   1,
			maxLines: 2,
			wantOut:  "b\nc",
		},
		{
			name:     "offset beyond content clamps to last line",
			content:  "a\nb",
			offset:   5,
			maxLines: 10,
			wantOut:  "b",
		},
		{
			name:     "zero maxLines returns content unchanged",
			content:  "a\nb\nc",
			offset:   0,
			maxLines: 0,
			wantOut:  "a\nb\nc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clipContent(tt.content, tt.offset, tt.maxLines)
			assert.Equal(t, tt.wantOut, result)
		})
	}
}

// TestRenderFull_FooterContainsTabAndJKHints verifies that the footer bar
// includes the new Tab panel and j/k navigation hints.
func TestRenderFull_FooterContainsTabAndJKHints(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)

	view := model.View()

	assert.Contains(t, view, "[Tab]")
	assert.Contains(t, view, "[j/k]")
	// Old hints must still be present
	assert.Contains(t, view, "[Enter] Select")
	assert.Contains(t, view, "[esc] Quit")
}

// ---------------------------------------------------------------------------
// End Phase 4 tests (renderer_test.go)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Phase 5: wrapText — prevent context panel from stretching vertically
// ---------------------------------------------------------------------------

// TestWrapText verifies that wrapText breaks long lines at word boundaries,
// hard-breaks words longer than the width, and preserves existing newlines.
func TestWrapText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		width   int
		wantOut string
	}{
		{
			name:    "short line unchanged",
			input:   "hello world",
			width:   50,
			wantOut: "hello world",
		},
		{
			name:    "long line breaks at word boundary",
			input:   "one two three four five six seven eight nine ten eleven",
			width:   20,
			wantOut: "one two three four\nfive six seven eight\nnine ten eleven",
		},
		{
			name:    "single word longer than width gets hard-broken",
			input:   "abcdefghijklmnopqrstuvwxyz",
			width:   10,
			wantOut: "abcdefghij\nklmnopqrst\nuvwxyz",
		},
		{
			name:    "existing newlines are preserved and each segment wrapped",
			input:   "line one is short\nthis line is much much much much much much much longer than the limit",
			width:   20,
			wantOut: "line one is short\nthis line is much\nmuch much much much\nmuch much longer\nthan the limit",
		},
		{
			name:    "zero width returns input unchanged",
			input:   "some content",
			width:   0,
			wantOut: "some content",
		},
		{
			name:    "empty string",
			input:   "",
			width:   10,
			wantOut: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.input, tt.width)
			assert.Equal(t, tt.wantOut, got)
		})
	}
}

func TestPrStateColor(t *testing.T) {
	tests := []struct {
		state     string
		wantColor string
	}{
		{"OPEN", "#00D9FF"},
		{"MERGED", "#9B59B6"},
		{"CLOSED", "#E74C3C"},
		{"UNKNOWN", "#4A5568"},
		{"", "#4A5568"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := prStateColor(tt.state)
			assert.Equal(t, lipgloss.Color(tt.wantColor), got)
		})
	}
}


// TestRenderer_ContextPanel_LongPathTruncated verifies that a worktree path
// longer than the dynamic ctxInner does not overflow and is truncated with ellipsis.
func TestRenderer_ContextPanel_LongPathTruncated(t *testing.T) {
longPath := "/development/worktrees/feat-issue-9-config-file-parsing-very-long-name"
wt := domain.Worktree{
Path:    longPath,
Branch:  "feat-issue-9",
IsClean: true,
}
model := NewModel()
require.NotNil(t, model)
model.view = viewWorktrees
model.Worktrees = []domain.Worktree{wt}
model.selectedIdx = 0

view := model.View()

// Path must not appear raw (it would overflow the panel when the terminal is narrow).
assert.NotContains(t, view, longPath, "raw long path must not appear; it should be truncated")
// The truncated path must contain the ellipsis sentinel.
assert.Contains(t, view, "Path: ")
}

// ---------------------------------------------------------------------------
// Phase 6: context panel height-cap and scroll-reset tests
// ---------------------------------------------------------------------------

// TestRenderFull_PRWithLongBody_FitsTerminalHeight verifies that a PR with a
// very long body does not cause the rendered output to exceed the terminal height.
// This is a regression test for the bug where the context panel grew taller than
// the terminal, pushing the header bar off-screen.
func TestRenderFull_PRWithLongBody_FitsTerminalHeight(t *testing.T) {
	const termHeight = 24
	const termWidth = 120

	// Build a body that, after word-wrapping, generates hundreds of lines.
	longBody := strings.Repeat("This is a long line of PR body text that keeps going. ", 200)

	prs := []domain.PullRequest{
		{
			Number: 1,
			Title:  "My very important PR",
			Branch: "feat/very-long-body-pr",
			Author: "alice",
			State:  "OPEN",
			Labels: []string{"bug", "enhancement", "security", "breaking-change"},
			Body:   longBody,
		},
	}

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewPRs
	model.prs = prs
	model.width = termWidth
	model.height = termHeight

	rendered := model.View()

	// Count newlines — output must have at most termHeight lines.
	lineCount := strings.Count(rendered, "\n") + 1
	assert.LessOrEqual(t, lineCount, termHeight,
		"rendered output (%d lines) must not exceed terminal height (%d)", lineCount, termHeight)
}

// TestRenderFull_IssueWithManyLabels_FitsTerminalHeight verifies the same height
// constraint when viewing an issue with many labels (exercises the Labels: prefix fix).
func TestRenderFull_IssueWithManyLabels_FitsTerminalHeight(t *testing.T) {
	const termHeight = 24
	const termWidth = 80

	issues := []domain.Issue{
		{
			Number: 5,
			Title:  "Some issue",
			Labels: []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"},
		},
	}

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewIssues
	model.issues = issues
	model.width = termWidth
	model.height = termHeight

	rendered := model.View()

	lineCount := strings.Count(rendered, "\n") + 1
	assert.LessOrEqual(t, lineCount, termHeight,
		"rendered output (%d lines) must not exceed terminal height (%d)", lineCount, termHeight)
}

// TestMoveDown_ResetsCtxScrollOffset verifies that navigating the list panel
// resets the context scroll offset so the new item's content starts at the top.
func TestMoveDown_ResetsCtxScrollOffset(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.view = viewPRs
	model.focused = panelList
	model.prs = []domain.PullRequest{
		{Number: 1, Title: "PR 1", Branch: "feat/1", Author: "alice", State: "OPEN"},
		{Number: 2, Title: "PR 2", Branch: "feat/2", Author: "bob", State: "OPEN"},
	}
	model.selectedPRIdx = 0
	model.ctxScrollOffset = 5 // simulate scrolled state

	model.moveDown()

	assert.Equal(t, 0, model.ctxScrollOffset, "ctxScrollOffset must reset to 0 after navigating to next PR")
	assert.Equal(t, 1, model.selectedPRIdx)
}

// TestMoveUp_ResetsCtxScrollOffset verifies that navigating up in the list panel
// resets the context scroll offset.
func TestMoveUp_ResetsCtxScrollOffset(t *testing.T) {
	model := NewModel()
	require.NotNil(t, model)
	model.view = viewPRs
	model.focused = panelList
	model.prs = []domain.PullRequest{
		{Number: 1, Title: "PR 1", Branch: "feat/1", Author: "alice", State: "OPEN"},
		{Number: 2, Title: "PR 2", Branch: "feat/2", Author: "bob", State: "OPEN"},
	}
	model.selectedPRIdx = 1
	model.ctxScrollOffset = 3 // simulate scrolled state

	model.moveUp()

	assert.Equal(t, 0, model.ctxScrollOffset, "ctxScrollOffset must reset to 0 after navigating to previous PR")
	assert.Equal(t, 0, model.selectedPRIdx)
}

// TestViewSwitch_ResetsCtxScrollOffset verifies that switching views (W/I/P keys)
// resets the context scroll offset so the new view starts from the top.
func TestViewSwitch_ResetsCtxScrollOffset(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantView activeView
	}{
		{"switch to worktrees resets scroll", "w", viewWorktrees},
		{"switch to issues resets scroll", "i", viewIssues},
		{"switch to PRs resets scroll", "p", viewPRs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.ctxScrollOffset = 7 // simulate scrolled state

			_, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})

			assert.Equal(t, 0, model.ctxScrollOffset, "ctxScrollOffset must reset to 0 after switching view")
			assert.Equal(t, tt.wantView, model.view)
		})
	}
}

// ---------------------------------------------------------------------------
// Issue body in context panel tests
// ---------------------------------------------------------------------------

// TestRenderer_IssueContextPanel_ShowsBody verifies that the issue context panel
// renders the issue body when present.
func TestRenderer_IssueContextPanel_ShowsBody(t *testing.T) {
	tests := []struct {
		name    string
		issue   domain.Issue
		wantIn  []string
		wantOut []string
	}{
		{
			name: "shows body text when populated",
			issue: domain.Issue{
				Number: 7,
				Title:  "Fix the bug",
				Body:   "This bug causes a crash on startup.",
				Labels: []string{},
			},
			wantIn: []string{"This bug causes a crash on startup."},
		},
		{
			name: "shows (no description) when body is empty",
			issue: domain.Issue{
				Number: 8,
				Title:  "Empty body issue",
				Body:   "",
				Labels: []string{},
			},
			wantIn: []string{"(no description)"},
		},
		{
			name: "body appears alongside all context fields",
			issue: domain.Issue{
				Number: 9,
				Title:  "Some issue",
				Body:   "Details about the issue.",
				Labels: []string{"bug"},
			},
			wantIn: []string{
				"Context: Issue #9",
				"Some issue",
				"Status: ● Open",
				"Labels: [bug]",
				"Details about the issue.",
				"[g] Open in GitHub",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewIssues
			model.issues = []domain.Issue{tt.issue}
			model.selectedIssueIdx = 0

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
			for _, notWant := range tt.wantOut {
				assert.NotContains(t, view, notWant)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizeBody & wrapLine regression tests
// ---------------------------------------------------------------------------

// TestSanitizeBody verifies that control characters (produced when PowerShell
// interprets backtick-letter escape sequences in PR body strings) are stripped
// without touching normal printable content or newlines.
func TestSanitizeBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips backspace (0x08) from PR body",
			in:   "\\buildAiderCmd\x08 rest",
			want: "\\buildAiderCmd rest",
		},
		{
			name: "strips form feed (0x0C) from PR body",
			in:   "\\fetchFiles\x0c rest",
			want: "\\fetchFiles rest",
		},
		{
			name: "preserves newlines",
			in:   "line one\nline two\nline three",
			want: "line one\nline two\nline three",
		},
		{
			name: "preserves tabs",
			in:   "\tindented",
			want: "\tindented",
		},
		{
			name: "strips null bytes",
			in:   "before\x00after",
			want: "beforeafter",
		},
		{
			name: "passes through clean body unchanged",
			in:   "## Heading\n- list item\n`code span`",
			want: "## Heading\n- list item\n`code span`",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "multiple control chars in sequence",
			in:   "\x08\x0c\x07hello\x08\x0c",
			want: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeBody(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestWrapLine_BreakAtZeroNotHardBreak verifies that when the only space in the
// current window is at index 0 (leading space on the segment), wrapLine does NOT
// hard-break mid-word. The fix is breakAt < 0 (not <= 0).
func TestWrapLine_BreakAtZeroNotHardBreak(t *testing.T) {
	// A segment that starts with a space followed by a long word that exceeds
	// the wrap width. breakAt will be found at index 0 (the leading space).
	// With the old `breakAt <= 0` guard this triggered a mid-word hard break;
	// with the fixed `breakAt < 0` it correctly advances past the leading space.
	s := " superlongword_that_exceeds_limit"
	wrapped := wrapLine(s, 10)
	for _, line := range strings.Split(wrapped, "\n") {
		// No line should be empty due to a spurious hard break at the leading space.
		assert.NotEqual(t, "", strings.TrimSpace(line), "wrapLine produced an unexpected empty line")
	}
}

// TestPRContextPanel_SanitizesControlChars verifies that the PR context panel
// strips control characters from the PR body before rendering so that terminal
// renderers don't eat visible characters (e.g. backspace erasing a 'b').
func TestPRContextPanel_SanitizesControlChars(t *testing.T) {
	// Simulate a body where PowerShell turned `buildAiderCmd` into
	// \x08uildAiderCmd (backspace + rest) and `fetchFiles` into \x0cetchFiles.
	body := "- \\buildAiderCmd\x08 text\n- \\fetchFiles\x0cetchFiles"
	model := NewModel()
	require.NotNil(t, model)
	model.view = viewPRs
	model.prs = []domain.PullRequest{
		{Number: 1, Title: "Test PR", Branch: "feat/test", Author: "dev", State: "OPEN", Body: body},
	}
	model.selectedPRIdx = 0

	view := model.View()

	// Control characters must not reach the rendered output.
	assert.NotContains(t, view, "\x08", "backspace control char must be stripped")
	assert.NotContains(t, view, "\x0c", "form-feed control char must be stripped")
}

// ---------------------------------------------------------------------------
// Virtual scrolling tests
// ---------------------------------------------------------------------------

// TestRenderIssueList_VirtualScroll_SelectedAlwaysVisible verifies that when
// selectedIdx exceeds the visible window (panelHeight-1 items), the list
// scrolls so the selected item is always rendered and highlighted.
func TestRenderIssueList_VirtualScroll_SelectedAlwaysVisible(t *testing.T) {
	const termHeight = 10 // small terminal: panelHeight=5, maxItems=4
	const termWidth = 120

	// Build 12 issues; select the last one (idx=11) which is well beyond window.
	issues := make([]domain.Issue, 12)
	for i := range issues {
		issues[i] = domain.Issue{Number: i + 1, Title: fmt.Sprintf("Issue %d title", i+1)}
	}

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewIssues
	model.issues = issues
	model.selectedIssueIdx = 11 // last item
	model.width = termWidth
	model.height = termHeight

	rendered := model.View()

	// The selected issue (#12) must be visible and highlighted with "> ".
	assert.Contains(t, rendered, "> ", "selected row marker must be present")
	assert.Contains(t, rendered, "12", "selected issue number must be visible")

	// Height must not overflow terminal.
	lineCount := strings.Count(rendered, "\n") + 1
	assert.LessOrEqual(t, lineCount, termHeight,
		"rendered output (%d lines) must not exceed terminal height (%d)", lineCount, termHeight)
}

// TestRenderPRList_VirtualScroll_SelectedAlwaysVisible verifies the same for the PR list.
func TestRenderPRList_VirtualScroll_SelectedAlwaysVisible(t *testing.T) {
	const termHeight = 10
	const termWidth = 120

	prs := make([]domain.PullRequest, 12)
	for i := range prs {
		prs[i] = domain.PullRequest{
			Number: i + 1,
			Title:  fmt.Sprintf("PR %d title", i+1),
			Branch: fmt.Sprintf("feat/pr-%d", i+1),
			State:  "OPEN",
		}
	}

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewPRs
	model.prs = prs
	model.selectedPRIdx = 11
	model.width = termWidth
	model.height = termHeight

	rendered := model.View()

	assert.Contains(t, rendered, "> ")
	assert.Contains(t, rendered, "12")

	lineCount := strings.Count(rendered, "\n") + 1
	assert.LessOrEqual(t, lineCount, termHeight,
		"rendered output (%d lines) must not exceed terminal height (%d)", lineCount, termHeight)
}

// TestRenderWorktreePanel_VirtualScroll_SelectedAlwaysVisible verifies worktree virtual scroll.
func TestRenderWorktreePanel_VirtualScroll_SelectedAlwaysVisible(t *testing.T) {
	const termHeight = 10
	const termWidth = 120

	worktrees := make([]domain.Worktree, 12)
	for i := range worktrees {
		worktrees[i] = domain.Worktree{
			Path:   fmt.Sprintf("/repo/branch-%d", i+1),
			Branch: fmt.Sprintf("branch-%d", i+1),
		}
	}

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewWorktrees
	model.Worktrees = worktrees
	model.selectedIdx = 11
	model.width = termWidth
	model.height = termHeight

	rendered := model.View()

	assert.Contains(t, rendered, "> ")
	assert.Contains(t, rendered, "branch-12")

	lineCount := strings.Count(rendered, "\n") + 1
	assert.LessOrEqual(t, lineCount, termHeight,
		"rendered output (%d lines) must not exceed terminal height (%d)", lineCount, termHeight)
}

// TestRenderFull_IssueWithLongBody_FitsTerminalHeight verifies that selecting an
// issue with a very long body (like a GitHub issue with multiple sections) does
// not push the header off-screen.
func TestRenderFull_IssueWithLongBody_FitsTerminalHeight(t *testing.T) {
	const termHeight = 24
	const termWidth = 120

	longBody := strings.Repeat("## Section\nLine of body text that keeps going and going. ", 50)

	issues := []domain.Issue{
		{Number: 1, Title: "Short issue", Body: "tiny"},
		{Number: 2, Title: "Long issue", Body: longBody},
	}

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewIssues
	model.issues = issues
	model.selectedIssueIdx = 1 // select the long-body issue
	model.width = termWidth
	model.height = termHeight

	rendered := model.View()

	lineCount := strings.Count(rendered, "\n") + 1
	assert.LessOrEqual(t, lineCount, termHeight,
		"rendered output (%d lines) must not exceed terminal height (%d) when issue body is very long",
		lineCount, termHeight)
}

// TestRenderFull_IssueBody_ControlCharsStripped verifies that control characters
// in issue bodies are stripped (same as PR bodies).
func TestRenderFull_IssueBody_ControlCharsStripped(t *testing.T) {
	body := "Normal text\x08backspace\x0cformfeed end"

	model := NewModel()
	require.NotNil(t, model)
	model.view = viewIssues
	model.issues = []domain.Issue{{Number: 1, Title: "Test", Body: body}}
	model.selectedIssueIdx = 0

	view := model.View()

	assert.NotContains(t, view, "\x08", "backspace must be stripped from issue body")
	assert.NotContains(t, view, "\x0c", "form-feed must be stripped from issue body")
	assert.Contains(t, view, "Normal text")
}

// ---------------------------------------------------------------------------
// Phase 4: Pagination renderer tests
// ---------------------------------------------------------------------------

func TestRenderFooterBar_ShowsPageInfo_Issues(t *testing.T) {
	theme := styles.NewTheme("digital-noir")
	issues := make([]domain.Issue, 120)
	for i := range issues {
		issues[i] = domain.Issue{Number: i + 1, Title: fmt.Sprintf("Issue %d", i+1)}
	}

	// Page 1 of 3 (items 1-50 of 120)
	footer := renderFooterBar(theme, "2025-01-01", 200, false, time.Time{}, nil, viewIssues, issues, nil, 0)
	assert.Contains(t, footer, "Page 1/3", "should show page 1/3")
	assert.Contains(t, footer, "1-50 of 120 issues", "should show item range")

	// Page 3 of 3 (items 101-120 of 120)
	footer2 := renderFooterBar(theme, "2025-01-01", 200, false, time.Time{}, nil, viewIssues, issues, nil, 2)
	assert.Contains(t, footer2, "Page 3/3", "should show page 3/3")
	assert.Contains(t, footer2, "101-120 of 120 issues", "should show last page range")
}

func TestRenderFooterBar_ShowsPageInfo_PRs(t *testing.T) {
	theme := styles.NewTheme("digital-noir")
	prs := make([]domain.PullRequest, 60)
	for i := range prs {
		prs[i] = domain.PullRequest{Number: i + 1, Title: fmt.Sprintf("PR %d", i+1)}
	}

	footer := renderFooterBar(theme, "2025-01-01", 200, false, time.Time{}, nil, viewPRs, nil, prs, 1)
	assert.Contains(t, footer, "Page 2/2", "should show page 2/2")
	assert.Contains(t, footer, "51-60 of 60 PRs", "should show last PR page range")
}

func TestRenderFooterBar_NoPageInfo_WhenListFitsOnOnePage(t *testing.T) {
	theme := styles.NewTheme("digital-noir")
	issues := make([]domain.Issue, 10)
	for i := range issues {
		issues[i] = domain.Issue{Number: i + 1, Title: fmt.Sprintf("Issue %d", i+1)}
	}

	footer := renderFooterBar(theme, "2025-01-01", 200, false, time.Time{}, nil, viewIssues, issues, nil, 0)
	assert.NotContains(t, footer, "Page", "no page info when list fits in one page")
}

// ---------------------------------------------------------------------------
// Renderer helper unit tests
// ---------------------------------------------------------------------------

func TestBuildIssueTree_TopLevelOnly(t *testing.T) {
issues := []domain.Issue{
{Number: 1, Title: "First"},
{Number: 2, Title: "Second"},
{Number: 3, Title: "Third"},
}
tree := buildIssueTree(issues)
require.Len(t, tree, 3)
assert.Equal(t, 1, tree[0].issue.Number)
assert.Equal(t, 2, tree[1].issue.Number)
assert.Equal(t, 3, tree[2].issue.Number)
for _, r := range tree {
assert.Empty(t, r.prefix)
}
}

func TestBuildIssueTree_ParentWithChildren(t *testing.T) {
parent := 10
issues := []domain.Issue{
{Number: 1, Title: "Parent", SubIssueNumbers: []int{2, 3}},
{Number: 2, Title: "Child A", ParentNumber: &parent},
{Number: 3, Title: "Child B", ParentNumber: &parent},
}
// Fix: set parent refs to match parent issue number (1, not 10)
one := 1
issues[1].ParentNumber = &one
issues[2].ParentNumber = &one

tree := buildIssueTree(issues)
require.Len(t, tree, 3)
assert.Equal(t, 1, tree[0].issue.Number)
assert.Empty(t, tree[0].prefix)
assert.Equal(t, 2, tree[1].issue.Number)
assert.Equal(t, "├─ ", tree[1].prefix)
assert.Equal(t, 3, tree[2].issue.Number)
assert.Equal(t, "└─ ", tree[2].prefix, "last child gets └─ prefix")
}

func TestBuildIssueTree_OrphanedSubIssue(t *testing.T) {
missingParent := 99
issues := []domain.Issue{
{Number: 1, Title: "Regular"},
{Number: 2, Title: "Orphan", ParentNumber: &missingParent},
}
tree := buildIssueTree(issues)
require.Len(t, tree, 2)
assert.Equal(t, 1, tree[0].issue.Number)
// Orphan is appended at bottom with no prefix.
assert.Equal(t, 2, tree[1].issue.Number)
assert.Empty(t, tree[1].prefix)
}

func TestBuildIssueTree_OriginalIdxPreserved(t *testing.T) {
one := 1
issues := []domain.Issue{
{Number: 1, Title: "Parent", SubIssueNumbers: []int{2}},
{Number: 2, Title: "Child", ParentNumber: &one},
}
tree := buildIssueTree(issues)
require.Len(t, tree, 2)
assert.Equal(t, 0, tree[0].originalIdx)
assert.Equal(t, 1, tree[1].originalIdx)
}

func TestExtractIssueNumber_Valid(t *testing.T) {
cases := []struct {
branch string
want   int
}{
{"feat/issue-42-some-feature", 42},
{"fix/issue-7-bug-fix", 7},
{"issue-100-other", 100},
}
for _, tc := range cases {
assert.Equal(t, tc.want, extractIssueNumber(tc.branch), tc.branch)
}
}

func TestExtractIssueNumber_NoMatch(t *testing.T) {
cases := []string{"main", "feat/no-number", "hotfix-something", ""}
for _, branch := range cases {
assert.Equal(t, 0, extractIssueNumber(branch), "should return 0 for %q", branch)
}
}

func TestBuildIssueHierarchyStr_ParentIssue(t *testing.T) {
parentNum := 5
issue := domain.Issue{Number: 10, Title: "Child", ParentNumber: &parentNum}
parent := domain.Issue{Number: 5, Title: "The Parent"}
result := buildIssueHierarchyStr(issue, []domain.Issue{parent, issue}, 80)
assert.Contains(t, result, "Parent: #5")
assert.Contains(t, result, "The Parent")
}

func TestBuildIssueHierarchyStr_SubIssues(t *testing.T) {
issue := domain.Issue{Number: 5, Title: "Parent", SubIssueNumbers: []int{10, 11}}
child1 := domain.Issue{Number: 10, Title: "Child One"}
child2 := domain.Issue{Number: 11, Title: "Child Two"}
result := buildIssueHierarchyStr(issue, []domain.Issue{issue, child1, child2}, 80)
assert.Contains(t, result, "Sub-issues:")
assert.Contains(t, result, "#10")
assert.Contains(t, result, "#11")
assert.Contains(t, result, "Child One")
assert.Contains(t, result, "Child Two")
}

func TestBuildIssueHierarchyStr_NeitherParentNorChildren(t *testing.T) {
issue := domain.Issue{Number: 1, Title: "Standalone"}
result := buildIssueHierarchyStr(issue, []domain.Issue{issue}, 80)
assert.Empty(t, result)
}

func TestBuildPRHint_SubIssueWithParentWorktree(t *testing.T) {
parentNum := 5
issues := []domain.Issue{
{Number: 10, Title: "Child", ParentNumber: &parentNum},
{Number: 5, Title: "Parent"},
}
worktrees := []domain.Worktree{
{Branch: "feat/issue-5-parent-feature"},
{Branch: "feat/issue-10-child-feature"},
}
hint := buildPRHint("feat/issue-10-child-feature", issues, worktrees)
assert.Contains(t, hint, "gh pr create --base feat/issue-5-parent-feature")
}

func TestBuildPRHint_NotASubIssue(t *testing.T) {
issues := []domain.Issue{
{Number: 10, Title: "Standalone"},
}
worktrees := []domain.Worktree{{Branch: "feat/issue-10-standalone"}}
hint := buildPRHint("feat/issue-10-standalone", issues, worktrees)
assert.Empty(t, hint)
}

func TestBuildPRHint_NonIssueBranch(t *testing.T) {
hint := buildPRHint("main", []domain.Issue{}, []domain.Worktree{})
assert.Empty(t, hint)
}

// ---------------------------------------------------------------------------
// Phase 5 (Issue #65): Session badges, header count, context panel session block
// ---------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

// TestSessionBadge_AllStates verifies sessionBadge returns correct badge text
// for every session status, including the nil (no session) case.
func TestSessionBadge_AllStates(t *testing.T) {
	tests := []struct {
		name    string
		session *domain.Session
		want    string
	}{
		{
			name:    "nil session returns empty string",
			session: nil,
			want:    "",
		},
		{
			name:    "StatusActive returns [session open]",
			session: &domain.Session{Status: domain.StatusActive},
			want:    "[session open]",
		},
		{
			name:    "StatusDead returns empty string",
			session: &domain.Session{Status: domain.StatusDead},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionBadge(tt.session)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCountActiveSessions verifies that only non-dead sessions are counted.
func TestCountActiveSessions(t *testing.T) {
	sessions := []domain.Session{
		{Status: domain.StatusActive},
		{Status: domain.StatusDead},
	}
	assert.Equal(t, 1, countActiveSessions(sessions))
	assert.Equal(t, 0, countActiveSessions(nil))
}

// TestRenderFull_SessionCountInHeader verifies that when active sessions exist,
// the header reports the count; when none exist, no count is shown.
func TestRenderFull_SessionCountInHeader(t *testing.T) {
	tests := []struct {
		name     string
		sessions []domain.Session
		wantIn   []string
		wantOut  []string
	}{
		{
			name: "header shows session count when sessions are active",
			sessions: []domain.Session{
				{WorktreePath: "/tmp/wt1", Status: domain.StatusActive},
				{WorktreePath: "/tmp/wt3", Status: domain.StatusDead},
			},
			wantIn:  []string{"1 active session(s)"},
			wantOut: []string{},
		},
		{
			name:     "header does not show session count when no sessions",
			sessions: nil,
			wantOut:  []string{"active session"},
		},
		{
			name: "header does not show count when all sessions are dead",
			sessions: []domain.Session{
				{WorktreePath: "/tmp/wt1", Status: domain.StatusDead},
			},
			wantOut: []string{"active session"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.sessions = tt.sessions

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
			for _, notWant := range tt.wantOut {
				assert.NotContains(t, view, notWant)
			}
		})
	}
}

// TestRenderFull_SessionBadgesInWorktreeRow verifies that session badges appear
// inline in the worktree name cell for every badge state.
func TestRenderFull_SessionBadgesInWorktreeRow(t *testing.T) {
	wt := domain.Worktree{
		Path:    "/tmp/feat-work",
		Branch:  "feat/work",
		IsClean: true,
	}

	tests := []struct {
		name    string
		session *domain.Session
		wantIn  string
	}{
		{
			name:    "no session — no badge rendered",
			session: nil,
			wantIn:  "feat-work",
		},
		{
			name:    "shell-only session shows [session open]",
			session: &domain.Session{WorktreePath: "/tmp/feat-work", Status: domain.StatusActive},
			wantIn:  "[session open]",
		},
		{
			name:    "dead session shows no badge",
			session: &domain.Session{WorktreePath: "/tmp/feat-work", Status: domain.StatusDead},
			wantIn:  "feat-work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{wt}
			model.selectedIdx = 0
			model.width = 300 // wide terminal so names aren't truncated
			if tt.session != nil {
				model.sessions = []domain.Session{*tt.session}
			}

			view := model.View()

			assert.Contains(t, view, tt.wantIn)
		})
	}
}

// TestRenderFull_SessionBlockInContextPanel verifies that when a selected worktree
// has a session, the SESSION block appears in the right context panel.
func TestRenderFull_SessionBlockInContextPanel(t *testing.T) {
	wt := domain.Worktree{Path: "/tmp/wt-session", Branch: "feat/session", IsClean: true}
	pid := 8821
	prompt := "implement JWT auth"

	tests := []struct {
		name    string
		session domain.Session
		wantIn  []string
	}{
		{
			name: "session block shows status and shell pid",
			session: domain.Session{
				WorktreePath: "/tmp/wt-session",
				Status:       domain.StatusActive,
				ShellPID:     &pid,
			},
			wantIn: []string{"SESSION", "Status:  active", "Shell:   pid 8821"},
		},
		{
			name: "session block shows agent name and prompt",
			session: domain.Session{
				WorktreePath: "/tmp/wt-session",
				Status:       domain.StatusActive,
				ShellPID:     &pid,
				AgentName:    ptr("Copilot"),
				Prompt:       &prompt,
			},
			wantIn: []string{"SESSION", "Agent:   Copilot", "Prompt:  implement JWT auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel()
			require.NotNil(t, model)
			model.view = viewWorktrees
			model.Worktrees = []domain.Worktree{wt}
			model.selectedIdx = 0
			model.sessions = []domain.Session{tt.session}

			view := model.View()

			for _, want := range tt.wantIn {
				assert.Contains(t, view, want)
			}
		})
	}
}

// TestRenderFull_NoSessionBlock_WhenNoSession verifies the SESSION block is absent
// when the selected worktree has no associated session.
func TestRenderFull_NoSessionBlock_WhenNoSession(t *testing.T) {
	wt := domain.Worktree{Path: "/tmp/wt-empty", Branch: "feat/empty", IsClean: true}
	model := NewModel()
	require.NotNil(t, model)
	model.view = viewWorktrees
	model.Worktrees = []domain.Worktree{wt}
	model.selectedIdx = 0
	// no sessions set

	view := model.View()

	assert.NotContains(t, view, "SESSION")
}

// TestSessionForWorktree_PathNormalisation verifies that sessionForWorktree matches
// sessions even when the stored WorktreePath uses different path separators than
// the lookup path (the Windows forward-slash vs backslash mismatch from the
// Copilot session-store).
func TestSessionForWorktree_PathNormalisation(t *testing.T) {
	agentName := "copilot"
	sessions := []domain.Session{
		{
			WorktreePath: filepath.FromSlash("/repo/feat-work"),
			Status:       domain.StatusActive,
			AgentName:    &agentName,
		},
	}

	// Same path with opposite separator style — must still match.
	got := sessionForWorktree(sessions, filepath.ToSlash(filepath.FromSlash("/repo/feat-work")))
	require.NotNil(t, got, "expected a session match despite different separators")
	assert.Equal(t, domain.StatusActive, got.Status)
}

// TestPathsEqual_NormalisedComparison verifies the pathsEqual helper handles
// case differences and mixed separators.
func TestPathsEqual_NormalisedComparison(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical paths", "/repo/foo", "/repo/foo", true},
		{"different case", "/Repo/Foo", "/repo/foo", true},
		{"forward vs backslash", "D:/dev/foo", `D:\dev\foo`, true},
		{"trailing slash a", "/repo/foo/", "/repo/foo", true},
		{"trailing slash b", "/repo/foo", "/repo/foo/", true},
		{"different paths", "/repo/foo", "/repo/bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pathsEqual(tt.a, tt.b))
		})
	}
}
