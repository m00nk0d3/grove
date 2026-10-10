package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/tui/modal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubIssueCloseWriter installs a fake gh runner for close operations and
// restores the real writer when the test ends.
func stubIssueCloseWriter(t *testing.T, run func(string, ...string) (string, error)) {
	t.Helper()
	orig := newIssueCloseWriter
	newIssueCloseWriter = func(repoPath string) *internalexec.GitHubWriter {
		return internalexec.NewGitHubWriterWithRunner(repoPath, run)
	}
	t.Cleanup(func() { newIssueCloseWriter = orig })
}

// fakeCloseRunner answers repo resolution and permission queries from maps.
func fakeCloseRunner(repo string, permissions string, errs map[string]error) func(string, ...string) (string, error) {
	return func(_ string, args ...string) (string, error) {
		key := strings.Join(args[:min(2, len(args))], " ")
		if err, ok := errs[key]; ok {
			return "", err
		}
		switch key {
		case "repo view":
			return repo + "\n", nil
		case "api " + "repos/" + repo:
			return permissions, nil
		}
		return "", nil
	}
}

func closeTestModel() *Model {
	m := NewModel()
	m.statusErr = ""
	m.view = viewIssues
	m.issues = []domain.Issue{{Number: 7, Title: "Sync stalls", State: "OPEN"}}
	m.selectedIssueIdx = 0
	return m
}

func TestContextActionsFor_Issues_IncludesCloseIssue(t *testing.T) {
	issues := []domain.Issue{{Number: 7, Title: "Sync stalls", State: "OPEN"}}

	actions := contextActionsFor(viewIssues, nil, 0, issues, 0, nil, 0, nil, labView{})
	require.Len(t, actions, 4)
	assert.Equal(t, modal.ContextActionIssueClose, actions[3].action)
	assert.Equal(t, "Close issue", actions[3].label)
}

func TestContextActionsFor_ClosedIssue_MarksCloseDisabled(t *testing.T) {
	issues := []domain.Issue{{Number: 7, Title: "Old", State: "CLOSED"}}

	actions := contextActionsFor(viewIssues, nil, 0, issues, 0, nil, 0, nil, labView{})
	require.Len(t, actions, 4)
	assert.Equal(t, modal.ContextActionIssueClose, actions[3].action)
	assert.Equal(t, "Close issue (already closed)", actions[3].label)
}

func TestHandleContextAction_CloseIssue_ChecksPermissionThenOpensModal(t *testing.T) {
	stubIssueCloseWriter(t, fakeCloseRunner("o/r",
		`{"admin":false,"maintain":false,"push":true,"triage":false,"pull":true}`, nil))
	m := closeTestModel()

	updated, cmd := m.handleContextAction(modal.ContextActionIssueClose)
	require.NotNil(t, cmd)
	assert.Equal(t, "Checking close permission for #7…", updated.(*Model).statusMsg)

	msg, ok := cmd().(issueClosePermissionMsg)
	require.True(t, ok)
	assert.True(t, msg.canClose)

	updated, cmd = updated.Update(msg)
	require.Nil(t, cmd)
	model := updated.(*Model)
	assert.Empty(t, model.statusMsg)
	issueModal, ok := model.activeModal.(*modal.IssueCloseModal)
	require.True(t, ok, "expected *IssueCloseModal, got %T", model.activeModal)
	assert.True(t, issueModal.CanConfirm())
}

func TestHandleContextAction_CloseIssue_NonCollaboratorOpensDisabledModal(t *testing.T) {
	stubIssueCloseWriter(t, fakeCloseRunner("o/r",
		`{"admin":false,"maintain":false,"push":false,"triage":false,"pull":true}`, nil))
	m := closeTestModel()

	_, cmd := m.handleContextAction(modal.ContextActionIssueClose)
	msg, ok := cmd().(issueClosePermissionMsg)
	require.True(t, ok)
	assert.False(t, msg.canClose)

	updated, _ := m.Update(msg)
	issueModal, ok := updated.(*Model).activeModal.(*modal.IssueCloseModal)
	require.True(t, ok)
	assert.False(t, issueModal.CanConfirm())
	assert.Contains(t, issueModal.View(), "permission")
}

func TestHandleContextAction_CloseIssue_PermissionErrorShowsStatus(t *testing.T) {
	stubIssueCloseWriter(t, func(string, ...string) (string, error) {
		return "", errors.New("HTTP 401: Requires authentication")
	})
	m := closeTestModel()

	_, cmd := m.handleContextAction(modal.ContextActionIssueClose)
	msg, ok := cmd().(issueClosePermissionMsg)
	require.True(t, ok)
	require.Error(t, msg.err)

	updated, _ := m.Update(msg)
	model := updated.(*Model)
	assert.Nil(t, model.activeModal)
	assert.Contains(t, model.statusErr, "Cannot verify close permission")
}

func TestHandleContextAction_CloseIssue_AlreadyClosedOpensDisabledModal(t *testing.T) {
	m := closeTestModel()
	m.issues[0].State = "CLOSED"

	updated, cmd := m.handleContextAction(modal.ContextActionIssueClose)
	assert.Nil(t, cmd, "no permission check for an already closed issue")
	issueModal, ok := updated.(*Model).activeModal.(*modal.IssueCloseModal)
	require.True(t, ok)
	assert.False(t, issueModal.CanConfirm())
	assert.Contains(t, issueModal.View(), "already closed")
}

func TestHandleContextAction_CloseIssue_NoSelection(t *testing.T) {
	m := closeTestModel()
	m.issues = nil

	updated, _ := m.handleContextAction(modal.ContextActionIssueClose)
	assert.Nil(t, updated.(*Model).activeModal)
	assert.Contains(t, updated.(*Model).statusErr, "No issue selected")
}

func TestIssueCloseFlow_Confirm_ClosesAndToasts(t *testing.T) {
	var closed []string
	stubIssueCloseWriter(t, func(_ string, args ...string) (string, error) {
		key := strings.Join(args[:min(2, len(args))], " ")
		switch key {
		case "repo view":
			return "o/r\n", nil
		case "issue close":
			closed = append(closed, strings.Join(args, " "))
			return "", nil
		}
		return "", nil
	})
	m := closeTestModel()

	updated, cmd := m.handleIssueCloseConfirmed(modal.IssueCloseConfirmedMsg{
		Number: 7, Reason: "completed", Comment: "shipped",
	})
	model := updated.(*Model)
	assert.Nil(t, model.activeModal)
	assert.Equal(t, "Closing #7…", model.statusMsg)
	require.NotNil(t, cmd)

	msg, ok := cmd().(issueCloseDoneMsg)
	require.True(t, ok)
	require.NoError(t, msg.err)
	require.Len(t, closed, 1)
	assert.Contains(t, closed[0], "issue close 7 --repo o/r --reason completed --comment shipped")

	updated, cmd = model.Update(msg)
	model = updated.(*Model)
	assert.Equal(t, "Closed #7", model.statusMsg)
	assert.NotNil(t, cmd, "success refreshes GitHub in the background")
}

func TestIssueCloseFlow_Failure_ReopensModalWithInlineError(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		wantErr string
	}{
		{name: "rate limit", err: errors.New("API rate limit exceeded (HTTP 429)"), wantErr: "rate limit"},
		{name: "permission", err: errors.New("HTTP 403: Resource not accessible"), wantErr: "Permission denied"},
		{name: "network", err: errors.New("connection reset by peer"), wantErr: "Could not close #7"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := closeTestModel()
			updated, cmd := m.handleIssueCloseDone(issueCloseDoneMsg{
				issue: domain.Issue{Number: 7, Title: "Sync stalls", State: "OPEN"},
				err:   tt.err,
			})
			assert.Nil(t, cmd)
			model := updated.(*Model)
			assert.Empty(t, model.statusMsg)
			issueModal, ok := model.activeModal.(*modal.IssueCloseModal)
			require.True(t, ok, "failure reopens the dialog instead of closing it")
			assert.Contains(t, issueModal.View(), tt.wantErr)
		})
	}
}

func TestIssueCloseFlow_EndToEnd_KeyToConfirm(t *testing.T) {
	stubIssueCloseWriter(t, fakeCloseRunner("o/r",
		`{"admin":false,"maintain":false,"push":true,"triage":false,"pull":true}`, nil))
	m := closeTestModel()

	_, cmd := m.handleContextAction(modal.ContextActionIssueClose)
	updated, _ := m.Update(cmd())
	model := updated.(*Model)
	require.IsType(t, &modal.IssueCloseModal{}, model.activeModal)

	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	require.NotNil(t, cmd)
	confirmed, ok := cmd().(modal.IssueCloseConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, 7, confirmed.Number)
	assert.Equal(t, "completed", confirmed.Reason)
}

func TestCloseIssueFailureText(t *testing.T) {
	assert.Contains(t, closeIssueFailureText(7, errors.New("HTTP 429: too many")), "rate limit")
	assert.Contains(t, closeIssueFailureText(7, errors.New("HTTP 403: denied")), "Permission denied")
	assert.Contains(t, closeIssueFailureText(7, errors.New("boom")), "Could not close #7")
}

func TestIssueByNumber(t *testing.T) {
	m := closeTestModel()
	assert.Equal(t, 7, m.issueByNumber(7).Number)
	assert.Equal(t, "Sync stalls", m.issueByNumber(7).Title)
	assert.Equal(t, 99, m.issueByNumber(99).Number, "unknown numbers fall back to a bare issue")
}
