package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
	internalexec "github.com/m00nk0d3/grove/internal/exec"
	"github.com/m00nk0d3/grove/internal/tui/modal"
)

// issueClosePermissionMsg carries the result of the background collaborator
// check that precedes the close-issue dialog.
type issueClosePermissionMsg struct {
	issue    domain.Issue
	repo     string
	canClose bool
	err      error
}

// issueCloseDoneMsg carries the result of the background close attempt.
type issueCloseDoneMsg struct {
	issue domain.Issue
	err   error
}

// newIssueCloseWriter builds the gh writer for close operations. It is a
// variable so tests can stub GitHub without touching the CLI.
var newIssueCloseWriter = func(repoPath string) *internalexec.GitHubWriter {
	return internalexec.NewGitHubWriter(repoPath)
}

// checkIssueClosePermissionCmd resolves the repository and checks whether the
// viewer may close issues, without blocking the UI.
func (m *Model) checkIssueClosePermissionCmd(issue domain.Issue) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		w := newIssueCloseWriter(repoPath)
		repo, err := w.Repo()
		if err != nil {
			return issueClosePermissionMsg{issue: issue, err: err}
		}
		canClose, err := w.ViewerCanClose(repo)
		return issueClosePermissionMsg{issue: issue, repo: repo, canClose: canClose, err: err}
	}
}

// closeIssueCmd closes the confirmed issue with retry, then reports back.
func (m *Model) closeIssueCmd(confirmed modal.IssueCloseConfirmedMsg, issue domain.Issue) tea.Cmd {
	repoPath := m.RepoPath
	return func() tea.Msg {
		w := newIssueCloseWriter(repoPath)
		repo, err := w.Repo()
		if err != nil {
			return issueCloseDoneMsg{issue: issue, err: err}
		}
		if err := w.CloseIssue(repo, confirmed.Number, confirmed.Reason, confirmed.Comment); err != nil {
			return issueCloseDoneMsg{issue: issue, err: err}
		}
		return issueCloseDoneMsg{issue: issue}
	}
}

// openIssueCloseFlow starts the close flow for the selected issue. Already
// closed issues open the dialog directly in its disabled state; otherwise the
// collaborator check runs first and the dialog opens when it answers.
func (m *Model) openIssueCloseFlow() (tea.Model, tea.Cmd) {
	issue, ok := m.selectedIssue()
	if !ok {
		m.statusErr = "No issue selected — select one first"
		return m, clearErrorCmd()
	}
	if issue.IsClosed() {
		m.activeModal = modal.NewIssueCloseModal(issue, false)
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("Checking close permission for #%d…", issue.Number)
	return m, m.checkIssueClosePermissionCmd(issue)
}

// handleIssueClosePermission opens the dialog once the collaborator check
// answers, or shows why it cannot.
func (m *Model) handleIssueClosePermission(msg issueClosePermissionMsg) (tea.Model, tea.Cmd) {
	m.statusMsg = ""
	if msg.err != nil {
		m.statusErr = fmt.Sprintf("Cannot verify close permission: %v", msg.err)
		return m, clearErrorCmd()
	}
	if m.activeModal != nil {
		// The user moved on while the check ran; never clobber their modal.
		return m, nil
	}
	m.activeModal = modal.NewIssueCloseModal(msg.issue, msg.canClose)
	return m, nil
}

// handleIssueCloseConfirmed clears the dialog and closes the issue in the
// background.
func (m *Model) handleIssueCloseConfirmed(msg modal.IssueCloseConfirmedMsg) (tea.Model, tea.Cmd) {
	m.activeModal = nil
	m.statusMsg = fmt.Sprintf("Closing #%d…", msg.Number)
	return m, m.closeIssueCmd(msg, m.issueByNumber(msg.Number))
}

// handleIssueCloseDone toasts success and refreshes, or reopens the dialog
// with the failure inline so the user can retry or cancel.
func (m *Model) handleIssueCloseDone(msg issueCloseDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.statusMsg = ""
		reopened := modal.NewIssueCloseModal(msg.issue, true)
		reopened.SetError(closeIssueFailureText(msg.issue.Number, msg.err))
		m.activeModal = reopened
		return m, nil
	}
	m.activeModal = nil
	m.statusMsg = fmt.Sprintf("Closed #%d", msg.issue.Number)
	return m, tea.Batch(m.syncGitHubCmd(true), clearMsgCmd())
}

// closeIssueFailureText renders granular close errors: rate limits and
// permission denials name themselves instead of hiding behind a generic
// failure.
func closeIssueFailureText(number int, err error) string {
	switch {
	case internalexec.IsRateLimitError(err):
		return fmt.Sprintf("GitHub rate limit hit — try again later. #%d is still open; press y to retry.", number)
	case internalexec.IsPermissionError(err):
		return fmt.Sprintf("Permission denied — only collaborators can close issues. #%d is still open.", number)
	default:
		return fmt.Sprintf("Could not close #%d: %v. Press y to retry.", number, err)
	}
}

// issueByNumber finds the selected issue by number, falling back to a bare
// issue when the list moved on.
func (m *Model) issueByNumber(number int) domain.Issue {
	for _, issue := range m.issues {
		if issue.Number == number {
			return issue
		}
	}
	return domain.Issue{Number: number}
}
