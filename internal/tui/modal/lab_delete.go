package modal

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/m00nk0d3/grove/internal/domain"
)

// LabDeleteConfirmedMsg is sent when the user confirms deleting a Lab entry.
type LabDeleteConfirmedMsg struct {
	ID string
}

// LabDeleteModal confirms deleting a Lab entry and its stored artifacts.
type LabDeleteModal struct {
	entry domain.LabEntry
}

// NewLabDeleteModal creates the confirmation for entry.
func NewLabDeleteModal(entry domain.LabEntry) *LabDeleteModal {
	return &LabDeleteModal{entry: entry}
}

// Init satisfies tea.Model.
func (m *LabDeleteModal) Init() tea.Cmd { return nil }

// Title returns the modal title.
func (m *LabDeleteModal) Title() string { return "DELETE LAB ENTRY" }

// Update handles y/n/Esc.
func (m *LabDeleteModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y":
		id := m.entry.ID
		return m, func() tea.Msg { return LabDeleteConfirmedMsg{ID: id} }
	case "n", "N", "esc":
		return m, func() tea.Msg { return ModalCancelledMsg{} }
	}
	return m, nil
}

// View renders the confirmation.
func (m *LabDeleteModal) View() string {
	note := "Its captured text and any drafted artifacts will be removed."
	if !m.entry.Issues.Empty() {
		note += "\nIssues already published on GitHub are not affected."
	}
	return fmt.Sprintf("Delete %q?\n\n%s\n\n[y] delete  [n / Esc] cancel", m.entry.Title(), note)
}
