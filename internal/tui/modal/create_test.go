package modal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testIssues = []domain.Issue{
	{Number: 5, Title: "[Phase 1] Implement - Create/delete modals", Labels: []string{"phase-1"}},
	{Number: 6, Title: "[Phase 1] Implement - Switch worktree", Labels: []string{"phase-1"}},
}

func TestNewCreateModal_StartsAtIssueListStep(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")

	require.NotNil(t, m)
	assert.Equal(t, stepIssues, m.step)
	assert.Equal(t, 0, m.issueIdx)
	assert.Equal(t, 0, m.typeIdx)
}

func TestNewCreateModalForIssue_StartsAtTypeStep(t *testing.T) {
	m := NewCreateModalForIssue(testIssues[1], "/home/user/repo")

	require.NotNil(t, m)
	assert.Equal(t, stepType, m.step)
	assert.Equal(t, 0, m.issueIdx)
	assert.Equal(t, testIssues[1].Number, m.SelectedIssue().Number)
}

func TestCreateModal_View_IssueStep_ShowsIssues(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	view := m.View()

	assert.Contains(t, view, "#5")
	assert.Contains(t, view, "[Phase 1] Implement - Create/delete modals")
	assert.Contains(t, view, "#6")
}

func TestCreateModal_View_IssueStep_ShowsCursor(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	view := m.View()

	assert.Contains(t, view, ">")
}

func TestCreateModal_DownKey_MovesIssueSelection(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(*CreateModal)

	assert.Equal(t, 1, m.issueIdx)
}

func TestCreateModal_DownKey_DoesNotExceedIssueList(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.issueIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(*CreateModal)

	assert.Equal(t, 1, m.issueIdx)
}

func TestCreateModal_UpKey_MovesIssueSelectionUp(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.issueIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(*CreateModal)

	assert.Equal(t, 0, m.issueIdx)
}

func TestCreateModal_Enter_AdvancesFromIssuesToTypeStep(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepType, m.step)
}

func TestCreateModal_View_TypeStep_ShowsTypes(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.step = stepType

	view := m.View()

	for _, bt := range BranchTypes {
		assert.Contains(t, view, bt)
	}
}

func TestCreateModal_Enter_AdvancesFromTypeToSlugStep(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.step = stepType

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepSlug, m.step)
}

func TestCreateModal_SlugStep_AutoPopulatesSlugFromTitle(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.step = stepType

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, "phase-1-implement-create-delete", m.slugInput.Value())
}

func TestCreateModal_Enter_AdvancesFromSlugToConfirmStep(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.step = stepSlug
	m.slugInput.SetValue("create-delete-modals")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepConfirm, m.step)
}

func TestCreateModal_BranchName_FollowsConvention(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.issueIdx = 0
	m.typeIdx = 0 // feat
	m.slugInput.SetValue("create-delete-modals")

	assert.Equal(t, "feat/issue-5-create-delete-modals", m.BranchName())
}

func TestCreateModal_WorktreePath_IsParentWorktreesDir(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.issueIdx = 0
	m.typeIdx = 0
	m.slugInput.SetValue("create-delete-modals")

	path := m.WorktreePath()

	assert.True(t, strings.HasSuffix(path, "feat-issue-5-create-delete-modals"))
	assert.Contains(t, path, "worktrees")
}

func TestCreateModal_WorktreePath_IsScopedToRepoName(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		wantPath string
	}{
		{
			name:     "nexus repo scoped correctly",
			repoPath: "/home/user/nexus",
			wantPath: "/home/user/worktrees/nexus/feat-issue-5-create-delete-modals",
		},
		{
			name:     "nova repo scoped correctly — no collision with nexus",
			repoPath: "/home/user/nova",
			wantPath: "/home/user/worktrees/nova/feat-issue-5-create-delete-modals",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCreateModal(testIssues, tt.repoPath)
			m.issueIdx = 0
			m.typeIdx = 0 // feat
			m.slugInput.SetValue("create-delete-modals")

			assert.Equal(t, tt.wantPath, m.WorktreePath())
		})
	}
}

func TestCreateModal_Enter_AtConfirmStep_EmitsCreateMsg(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.step = stepConfirm
	m.issueIdx = 0
	m.typeIdx = 0
	m.slugInput.SetValue("create-delete-modals")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	confirmed, ok := msg.(WorktreeCreateConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, "feat/issue-5-create-delete-modals", confirmed.Branch)
	assert.Contains(t, confirmed.Path, "feat-issue-5-create-delete-modals")
}

func TestCreateModal_Esc_AtAnyStep_EmitsCancelMsg(t *testing.T) {
	steps := []createStep{stepIssues, stepType, stepParentRequired, stepSlug, stepConfirm}

	for _, step := range steps {
		m := NewCreateModal(testIssues, "/home/user/repo")
		m.step = step

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		require.NotNil(t, cmd)

		msg := cmd()
		_, ok := msg.(ModalCancelledMsg)
		assert.True(t, ok, "expected ModalCancelledMsg at step %d", step)
	}
}

func TestCreateModal_View_ConfirmStep_ShowsBranchAndPath(t *testing.T) {
	m := NewCreateModal(testIssues, "/home/user/repo")
	m.step = stepConfirm
	m.issueIdx = 0
	m.typeIdx = 0
	m.slugInput.SetValue("create-delete-modals")

	view := m.View()

	assert.Contains(t, view, "feat/issue-5-create-delete-modals")
}

func TestCreateModal_EmptyIssueList_EnterDoesNotAdvance(t *testing.T) {
	m := NewCreateModal([]domain.Issue{}, "/home/user/repo")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepIssues, m.step)
}

// Sub-issue tests — parent #10 with and without a worktree branch.

var parentNum10 = 10

var subIssue = domain.Issue{Number: 42, Title: "Sub thing", ParentNumber: &parentNum10}

func TestCreateModal_SubIssue_ParentHasNoWorktree_ShowsParentRequired(t *testing.T) {
	// No parentBranches provided → parent has no worktree.
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo")
	m.step = stepType

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepParentRequired, m.step)
	assert.Equal(t, 10, m.parentIssueNum)
	assert.Equal(t, 0, m.warnChoiceIdx)
}

func TestCreateModal_SubIssue_ParentHasWorktree_AutoSelectsParentBranch(t *testing.T) {
	parentBranch := "feat/issue-10-some-feature"
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo", parentBranch)
	m.step = stepType

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepBaseBranch, m.step)
	// baseBranches = ["main", parentBranch], parent is index 1.
	assert.Equal(t, 1, m.baseBranchIdx)
	assert.Equal(t, parentBranch, m.BaseBranch())
}

func TestCreateModal_ParentRequired_View_ShowsWarning(t *testing.T) {
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo")
	m.step = stepParentRequired
	m.parentIssueNum = 10

	view := m.View()

	assert.Contains(t, view, "#10")
	assert.Contains(t, view, "no worktree")
	assert.Contains(t, view, "Create parent")
	assert.Contains(t, view, "Continue anyway")
}

func TestCreateModal_ParentRequired_CreateFirst_EmitsParentRequiredMsg(t *testing.T) {
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo")
	m.step = stepParentRequired
	m.parentIssueNum = 10
	m.warnChoiceIdx = 0 // "Create parent first"

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	reqMsg, ok := msg.(ParentWorktreeRequiredMsg)
	require.True(t, ok)
	assert.Equal(t, 10, reqMsg.ParentNumber)
}

func TestCreateModal_ParentRequired_ContinueWithMain_GoesToSlug(t *testing.T) {
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo")
	m.step = stepParentRequired
	m.parentIssueNum = 10
	m.warnChoiceIdx = 1 // "Continue anyway"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*CreateModal)

	assert.Equal(t, stepSlug, m.step)
	assert.Equal(t, "", m.BaseBranch()) // no baseBranches → empty
}

func TestCreateModal_ParentRequired_DownKey_MovesToContinue(t *testing.T) {
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo")
	m.step = stepParentRequired
	m.warnChoiceIdx = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(*CreateModal)

	assert.Equal(t, 1, m.warnChoiceIdx)
}

func TestCreateModal_ParentRequired_UpKey_MovesToCreateFirst(t *testing.T) {
	m := NewCreateModal([]domain.Issue{subIssue}, "/home/user/repo")
	m.step = stepParentRequired
	m.warnChoiceIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(*CreateModal)

	assert.Equal(t, 0, m.warnChoiceIdx)
}
