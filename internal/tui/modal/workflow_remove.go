package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

// WorkflowRemoveModal confirms removal of Sandcastle workflow state and,
// for active workflows, termination of the workflow process.
type WorkflowRemoveModal struct {
	workflow domain.WorkflowRunRef
	stop     bool
}

func NewWorkflowRemoveModal(workflow domain.WorkflowRunRef) *WorkflowRemoveModal {
	status := strings.ToLower(workflow.Status)
	return &WorkflowRemoveModal{
		workflow: workflow,
		stop: status == domain.WorkflowQueued ||
			status == domain.WorkflowRunning ||
			status == domain.WorkflowBlocked,
	}
}

func (m *WorkflowRemoveModal) Init() tea.Cmd { return nil }

func (m *WorkflowRemoveModal) Title() string {
	if m.stop {
		return "STOP WORKFLOW"
	}
	return "REMOVE WORKFLOW"
}

func (m *WorkflowRemoveModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		runID := m.workflow.RunID
		if runID == "" {
			runID = m.workflow.WorkflowID
		}
		return m, func() tea.Msg {
			return WorkflowRemoveConfirmedMsg{RunID: runID, Stop: m.stop}
		}
	case "n", "N", "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}
	return m, nil
}

func (m *WorkflowRemoveModal) View() string {
	title := m.workflow.Title
	if title == "" {
		title = m.workflow.WorkflowID
	}
	if m.stop {
		return fmt.Sprintf(
			"Stop and remove %q?\n\nThe Sandcastle workflow process will be terminated.\nThe worktree and Git branch will be preserved.\n\n[y] stop and remove  [n / Esc] cancel",
			title,
		)
	}
	return fmt.Sprintf(
		"Remove %q from workflow history?\n\nThe worktree and Git branch will be preserved.\n\n[y] remove  [n / Esc] cancel",
		title,
	)
}
