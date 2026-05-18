package modal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/nexus/internal/domain"
)

type createStep int

const (
	stepIssues    createStep = iota
	stepType
	stepBaseBranch // only shown when len(m.baseBranches) > 0
	stepSlug
	stepConfirm
)

// BranchTypes are the valid conventional commit type prefixes for branch names.
var BranchTypes = []string{"feat", "fix", "chore", "docs", "test", "refactor"}

// CreateModal is a multi-step Bubbletea model for creating a new worktree from a GitHub issue.
// Steps: issue picker → type picker → [base branch picker] → slug editor → confirm.
type CreateModal struct {
	step           createStep
	issues         []domain.Issue
	issueIdx       int
	typeIdx        int
	baseBranchIdx  int
	baseBranches   []string // "main" + any parent branches; empty means skip base branch step
	slugInput      textinput.Model
	repoPath       string
}

// NewCreateModal creates a new CreateModal with the given issues and repo path.
// Optional parentBranches are offered as additional base branch options (beyond "main").
func NewCreateModal(issues []domain.Issue, repoPath string, parentBranches ...string) *CreateModal {
	ti := textinput.New()
	ti.Placeholder = "slug"
	ti.CharLimit = 60

	var bases []string
	if len(parentBranches) > 0 {
		bases = make([]string, 0, 1+len(parentBranches))
		bases = append(bases, "main")
		bases = append(bases, parentBranches...)
	}

	return &CreateModal{
		step:         stepIssues,
		issues:       issues,
		repoPath:     repoPath,
		slugInput:    ti,
		baseBranches: bases,
	}
}

// Init satisfies tea.Model.
func (m *CreateModal) Init() tea.Cmd { return nil }

// Update handles Bubbletea messages, driving the multi-step state machine.
func (m *CreateModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		if m.step == stepSlug {
			var cmd tea.Cmd
			m.slugInput, cmd = m.slugInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Esc always cancels regardless of step.
	if keyMsg.Type == tea.KeyEsc {
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}

	// On the slug step, route enter to advance and everything else to the text input.
	if m.step == stepSlug {
		if keyMsg.Type == tea.KeyEnter {
			m.slugInput.Blur()
			m.step = stepConfirm
			return m, nil
		}
		var cmd tea.Cmd
		m.slugInput, cmd = m.slugInput.Update(msg)
		return m, cmd
	}

	switch keyMsg.String() {
	case "up", "k":
		m.moveUp()
	case "down", "j":
		m.moveDown()
	case "enter":
		return m.advance()
	}

	return m, nil
}

func (m *CreateModal) moveUp() {
	switch m.step {
	case stepIssues:
		if m.issueIdx > 0 {
			m.issueIdx--
		}
	case stepType:
		if m.typeIdx > 0 {
			m.typeIdx--
		}
	case stepBaseBranch:
		if m.baseBranchIdx > 0 {
			m.baseBranchIdx--
		}
	}
}

func (m *CreateModal) moveDown() {
	switch m.step {
	case stepIssues:
		if m.issueIdx < len(m.issues)-1 {
			m.issueIdx++
		}
	case stepType:
		if m.typeIdx < len(BranchTypes)-1 {
			m.typeIdx++
		}
	case stepBaseBranch:
		if m.baseBranchIdx < len(m.baseBranches)-1 {
			m.baseBranchIdx++
		}
	}
}

func (m *CreateModal) advance() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepIssues:
		if len(m.issues) == 0 {
			return m, nil
		}
		m.step = stepType

	case stepType:
		if len(m.baseBranches) > 0 {
			m.step = stepBaseBranch
		} else {
			slug := domain.SlugFromTitle(m.SelectedIssue().Title)
			m.slugInput.SetValue(slug)
			m.slugInput.Focus()
			m.step = stepSlug
		}

	case stepBaseBranch:
		slug := domain.SlugFromTitle(m.SelectedIssue().Title)
		m.slugInput.SetValue(slug)
		m.slugInput.Focus()
		m.step = stepSlug

	case stepConfirm:
		branch := m.BranchName()
		path := m.WorktreePath()
		base := m.BaseBranch()
		return m, func() tea.Msg {
			return WorktreeCreateConfirmedMsg{Branch: branch, Path: path, BaseBranch: base}
		}
	}

	return m, nil
}

// SelectedIssue returns the currently selected issue.
func (m *CreateModal) SelectedIssue() domain.Issue {
	if len(m.issues) == 0 {
		return domain.Issue{}
	}
	return m.issues[m.issueIdx]
}

// SelectedType returns the currently selected branch type prefix.
func (m *CreateModal) SelectedType() string {
	return BranchTypes[m.typeIdx]
}

// BaseBranch returns the selected base branch, or empty string when no parent branches were provided.
func (m *CreateModal) BaseBranch() string {
	if len(m.baseBranches) == 0 {
		return ""
	}
	return m.baseBranches[m.baseBranchIdx]
}

// BranchName returns the branch name following the <type>/issue-<N>-<slug> convention.
func (m *CreateModal) BranchName() string {
	issue := m.SelectedIssue()
	return fmt.Sprintf("%s/issue-%d-%s", m.SelectedType(), issue.Number, m.slugInput.Value())
}

// WorktreePath returns the filesystem path for the new worktree: ../worktrees/<type>-issue-<N>-<slug>.
func (m *CreateModal) WorktreePath() string {
	slug := strings.ReplaceAll(m.BranchName(), "/", "-")
	return filepath.Join(filepath.Dir(m.repoPath), "worktrees", slug)
}

// Title returns the modal title for themed overlay rendering.
func (m *CreateModal) Title() string { return "New Worktree" }

// View renders the current step of the modal.
func (m *CreateModal) View() string {
	switch m.step {
	case stepIssues:
		return m.viewIssueList()
	case stepType:
		return m.viewTypePicker()
	case stepBaseBranch:
		return m.viewBaseBranchPicker()
	case stepSlug:
		return m.viewSlugEditor()
	case stepConfirm:
		return m.viewConfirm()
	}
	return ""
}

func (m *CreateModal) viewIssueList() string {
	var b strings.Builder
	b.WriteString("Select issue:\n\n")

	for i, issue := range m.issues {
		cursor := "  "
		if i == m.issueIdx {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s#%d %s\n", cursor, issue.Number, issue.Title))
	}

	if len(m.issues) == 0 {
		b.WriteString("  (no open issues)\n")
	}

	b.WriteString("\n↑/↓ navigate  •  Enter select  •  Esc cancel")
	return b.String()
}

func (m *CreateModal) viewTypePicker() string {
	issue := m.SelectedIssue()
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Issue: #%d %s\n\n", issue.Number, issue.Title))
	b.WriteString("Select branch type:\n\n")

	for i, t := range BranchTypes {
		cursor := "  "
		if i == m.typeIdx {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, t))
	}

	b.WriteString("\n↑/↓ navigate  •  Enter select  •  Esc cancel")
	return b.String()
}

func (m *CreateModal) viewBaseBranchPicker() string {
	issue := m.SelectedIssue()
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Issue: #%d %s\n", issue.Number, issue.Title))
	b.WriteString(fmt.Sprintf("Type:  %s\n\n", m.SelectedType()))
	b.WriteString("Select base branch:\n\n")

	for i, br := range m.baseBranches {
		cursor := "  "
		if i == m.baseBranchIdx {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, br))
	}

	b.WriteString("\n↑/↓ navigate  •  Enter select  •  Esc cancel")
	return b.String()
}

func (m *CreateModal) viewSlugEditor() string {
	issue := m.SelectedIssue()
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Issue: #%d %s\n", issue.Number, issue.Title))
	b.WriteString(fmt.Sprintf("Type:  %s\n\n", m.SelectedType()))
	b.WriteString("Edit slug:\n")
	b.WriteString(m.slugInput.View())
	b.WriteString(fmt.Sprintf("\n\nBranch preview: %s/issue-%d-%s\n", m.SelectedType(), issue.Number, m.slugInput.Value()))
	b.WriteString("\nEnter confirm  •  Esc cancel")
	return b.String()
}

func (m *CreateModal) viewConfirm() string {
	var b strings.Builder
	b.WriteString("Create worktree:\n\n")
	b.WriteString(fmt.Sprintf("  Branch:  %s\n", m.BranchName()))
	b.WriteString(fmt.Sprintf("  Path:    %s\n", m.WorktreePath()))
	if base := m.BaseBranch(); base != "" {
		b.WriteString(fmt.Sprintf("  Base:    %s\n", base))
	}
	b.WriteString("\nEnter confirm  •  Esc cancel")
	return b.String()
}
