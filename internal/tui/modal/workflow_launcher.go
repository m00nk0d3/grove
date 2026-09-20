package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

type workflowOption struct {
	kind        string
	key         string
	label       string
	description string
}

// WorkflowLauncherModal presents context-aware Grove workflow actions.
type WorkflowLauncherModal struct {
	options     []workflowOption
	selectedIdx int
	issueNumber *int
	prNumber    *int
}

// NewIssueWorkflowLauncherModal creates the workflow launcher for an issue.
func NewIssueWorkflowLauncherModal(issue domain.Issue) *WorkflowLauncherModal {
	number := issue.Number
	return &WorkflowLauncherModal{
		options: []workflowOption{{
			kind:        WorkflowKindImplement,
			key:         "i",
			label:       "Implement issue",
			description: fmt.Sprintf("Run imp for issue #%d", issue.Number),
		}},
		issueNumber: &number,
	}
}

// NewPRWorkflowLauncherModal creates the workflow launcher for a pull request.
func NewPRWorkflowLauncherModal(pr domain.PullRequest) *WorkflowLauncherModal {
	number := pr.Number
	return &WorkflowLauncherModal{
		options: []workflowOption{
			{kind: WorkflowKindReview, key: "r", label: "Review pull request", description: "Run an independent PR review"},
			{kind: WorkflowKindCI, key: "c", label: "Repair CI", description: "Diagnose and fix failed checks"},
			{kind: WorkflowKindResolve, key: "x", label: "Resolve conflicts", description: "Merge the base branch and resolve conflicts"},
		},
		prNumber: &number,
	}
}

// NewMaintenanceWorkflowLauncherModal creates repository-level workflow actions.
func NewMaintenanceWorkflowLauncherModal() *WorkflowLauncherModal {
	return &WorkflowLauncherModal{
		options: []workflowOption{{
			kind:        WorkflowKindClean,
			key:         "c",
			label:       "Clean merged work",
			description: "Remove eligible merged worktrees and branches",
		}},
	}
}

func (m *WorkflowLauncherModal) Init() tea.Cmd { return nil }

func (m *WorkflowLauncherModal) Title() string { return "RUN WORKFLOW" }

func (m *WorkflowLauncherModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyEsc {
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	switch key.String() {
	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "down", "j":
		if m.selectedIdx < len(m.options)-1 {
			m.selectedIdx++
		}
	case "enter":
		return m.launchSelected()
	default:
		for i, option := range m.options {
			if key.String() == option.key {
				m.selectedIdx = i
				return m.launchSelected()
			}
		}
	}
	return m, nil
}

func (m *WorkflowLauncherModal) launchSelected() (tea.Model, tea.Cmd) {
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.options) {
		return m, nil
	}
	option := m.options[m.selectedIdx]
	issueNumber := m.issueNumber
	prNumber := m.prNumber
	return m, func() tea.Msg {
		return WorkflowLaunchMsg{
			Kind:        option.kind,
			IssueNumber: issueNumber,
			PRNumber:    prNumber,
		}
	}
}

func (m *WorkflowLauncherModal) View() string {
	var b strings.Builder
	b.WriteString("Choose a Grove-managed workflow:\n\n")
	for i, option := range m.options {
		cursor := "  "
		if i == m.selectedIdx {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s[%s] %-22s %s\n", cursor, option.key, option.label, option.description))
	}
	b.WriteString("\n↑/↓ navigate  •  Enter run  •  Esc cancel")
	return b.String()
}
