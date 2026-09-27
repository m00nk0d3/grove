package modal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/tui/styles"
	"github.com/m00nk0d3/grove/internal/data"
)

// ComposeModal handles the multi-step compose workflow: kind selector → content editor → save.
type ComposeModal struct {
	title      string
	content    string
	width      int
	height     int
	theme      *styles.Theme
	initMsg    ComposeInitMsg // Initial message for empty entry
	cancel     bool            // true if cancel mode (n key)
	step       composeStep     // current step: kind | content | confirm
	kindIdx    int             // selected kind index
}

type composeStep int

const (
	composeKind composeStep = iota
	composeContent
	composeConfirm
)

type ComposeInitMsg struct {
	Kind      string // "" means empty, pre-select first kind
	Title     string
	Content   string
	Created   string
	RepoPath  string
}

const (
	stepKindLabel    = "KIND"
	stepContentLabel = "CONTENT"
)

// NewComposeModal creates a new ComposeModal in kind-selection step.
func NewComposeModal(msg ComposeInitMsg, isEditMode bool) *ComposeModal {
	if msg.Created == "" {
		msg.Created = fmt.Sprintf("%d", time.Now().Unix())
	}
	step := composeKind
	if len(msg.Kind) > 0 {
		step = composeContent
	}
	return &ComposeModal{
		title:  "COMPOSE NEW ENTRY",
		content: "",
		width:  0,
		height: 0,
		theme:  nil,
		initMsg: msg,
		cancel: false,
		step:   step,
		kindIdx: 0,
	}
}

// CurrentTimestamp returns current Unix timestamp for generating lab entry IDs.
func CurrentTimestamp() int64 {
	return time.Now().Unix()
}

// SetWidth sets the modal width.
func (m *ComposeModal) SetWidth(w int) {
	m.width = w
}

// SetHeight sets the modal height.
func (m *ComposeModal) SetHeight(h int) {
	m.height = h
}

// SetTheme injects the current visual theme for styled rendering.
func (m *ComposeModal) SetTheme(t styles.Theme) {
	m.theme = &t
}

// Init satisfies tea.Model.
func (m *ComposeModal) Init() tea.Cmd {
	return nil
}

// Title returns the modal title for themed rendering.
func (m *ComposeModal) Title() string {
	return m.title
}

// View renders the compose modal content.
func (m *ComposeModal) View() string {
	if m.height <= 0 {
		m.height = 24
	}
	if m.width <= 0 {
		m.width = 80
	}

	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")

	stepLabel := ""
	if m.step == composeKind {
		stepLabel = fmt.Sprintf("%-12s ", stepKindLabel)
	} else if m.step == composeContent {
		stepLabel = fmt.Sprintf("%-12s ", stepContentLabel)
	}
	b.WriteString(stepLabel)

	var content string
	switch m.step {
	case composeKind:
		content = renderKindSelector(m, m.width)
	case composeContent:
		content = renderContentEditor(m, m.width)
	case composeConfirm:
		content = renderConfirmSave(m, m.width)
	default:
		content = "Initializing…"
	}

	b.WriteString(content)

	hints := []string{
		"Select kind   [↑↓] Navigate   [n] Cancel",
		"Enter content [↑↓] Browse   [Enter] Confirm",
		"Preview       [a] Accept   [e] Edit   [n] Cancel",
	}
	for _, hint := range hints {
		b.WriteString(fmt.Sprintf("  %s\n", hint))
	}

	b.WriteString("\n[ESC/q] Quit")

	return b.String()
}

// Update handles input for the compose modal.
func (m *ComposeModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			m.cancel = true
			return m, nil

		case tea.KeyEnter:
			if m.step == composeKind {
				m.step = composeContent
			} else if m.step == composeContent {
				m.step = composeConfirm
			} else if m.step == composeConfirm {
				return m, m.EntrySaveCmd()
			}

		case tea.KeyTab:
			if m.step == composeKind {
				m.step = composeContent
			} else if m.step == composeContent {
				m.step = composeConfirm
			} else if m.step == composeConfirm {
				m.step = composeKind
			}

		case tea.KeyUp:
			if m.step == composeKind {
				m.kindIdx--
				if m.kindIdx < 0 {
					m.kindIdx = len(kinds) - 1
				}
			}

		case tea.KeyDown:
			if m.step == composeKind {
				m.kindIdx++
				if m.kindIdx >= len(kinds) {
					m.kindIdx = 0
				}
			}

		case tea.KeyCtrlN:
			m.cancel = true
			return m, nil
		}

		return m, nil
	}
	return m, nil
}

// renderKindSelector renders the kind selection screen with 2 options.
func renderKindSelector(m *ComposeModal, width int) string {
	kinds := []string{"idea", "bug"}
	bold := lipgloss.NewStyle().Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#888"))

	var b strings.Builder
	for i, kind := range kinds {
		cursor := bold.Render("▶")
		if m.kindIdx != i {
			cursor = "  "
		}
		line := fmt.Sprintf("%s %s", cursor, bold.Render(kind))
		b.WriteString(muted.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}

// renderContentEditor renders the content entry screen.
func renderContentEditor(m *ComposeModal, width int) string {
	if m.content == "" {
		m.content = "(enter idea or bug details here...)"
	}
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render("Title: ")
	contentStyle := lipgloss.NewStyle().MarginLeft(10)
	return fmt.Sprintf("%s%s\n\n%s", titleStyle, m.initMsg.Title, contentStyle.Render(m.content))
}

// renderConfirmSave renders the save confirmation screen.
func renderConfirmSave(m *ComposeModal, width int) string {
	status := "ACCEPT"
	if m.cancel {
		status = "CANCELLED"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0f0"))
	return fmt.Sprintf("%s %s", titleStyle.Render("Entry Ready:"), status)
}

// EntrySaveCmd saves the composed entry to disk and returns the saved message or error.
func (m *ComposeModal) EntrySaveCmd() tea.Cmd {
	return func() tea.Msg {
		// Build entry record with slugified ID
		ts := fmt.Sprintf("%d", domain.CurrentTimestamp())
		slug := fmt.Sprintf("%s-%s", ts, domain.Slugify(m.initMsg.Title))
		entry := domain.LabEntry{
			ID:       slug,
			Title:    m.initMsg.Title,
			Kind:     m.initMsg.Kind,
			Content:  m.content,
			Created:  time.Now().UTC().Format(time.RFC3339),
		}

		// Save to disk using the repository path from init message
		path := data.LabsPath(m.initMsg.RepoPath)

		dataBytes, err := json.MarshalIndent([]interface{}{entry}, "", "  ")
		if err != nil {
			return EntrySavedErrMsg{Error: fmt.Errorf("failed to marshal entry: %w", err)}
		}

		err = os.WriteFile(path, dataBytes, 0o644)
		if err != nil {
			return EntrySavedErrMsg{Error: fmt.Errorf("failed to write file: %w", err)}
		}

		return EntrySavedMsg{Entry: entry}
	}
}

// kinds is the ordered list of available entry kinds.
var kinds = []string{"idea", "bug"}
