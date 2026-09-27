// Package modal provides TUI modals for Grove.
package modal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m00nk0d3/grove/internal/domain"
)

// ShapeIssueModal handles the single-step approval: shaped preview → approve/reject
type ShapeIssueModal struct {
	title      string
	entry      domain.LabEntry // Original bug entry
	repoPath   string
	themedView string // Formatted shaped issue for display
	confirm    bool   // true if user approved
}

// NewShapeIssueModal creates a new ShapeIssueModal for approval.
func NewShapeIssueModal(msg ShapeIssueInitMsg) *ShapeIssueModal {
	if msg.Entry.Kind != "bug" {
		// Only shape bug entries; idea entries skip this flow
		return nil
	}

	shaped := formatShapedIssue(msg.Entry) // See formatShapedIssue() below
	return &ShapeIssueModal{
		title:      "SHAPE BUG REPORT",
		entry:      msg.Entry,
		repoPath:   msg.RepoPath,
		themedView: shaped,
		confirm:    false,
	}
}

// formatShapedIssue transforms a LabEntry into a structured GitHub issue body.
func formatShapedIssue(entry domain.LabEntry) string {
	// Use entry.Title if provided; otherwise extract first non-empty line from content
	var title string
	if len(entry.Title) > 0 {
		title = entry.Title
	} else {
		// Extract first non-empty line from content as title
		lines := strings.Split(strings.TrimSpace(entry.Content), "\n")
		for _, line := range lines {
			if len(line) > 0 {
				title = line
				break
			}
		}
	}

	lines := strings.Split(strings.TrimSpace(entry.Content), "\n")
	var body strings.Builder

	for i, line := range lines {
		// If no title was extracted, skip first non-empty line (use as title)
		if len(title) == 0 && i == 0 && len(line) > 0 {
			continue
		}
		// Skip blank line after title if present
		if len(strings.TrimSpace(line)) == 0 && body.Len() >= 4 {
			continue
		}
		// Only add content if it's non-empty or if we already have body content
		// First non-empty line becomes body without indentation
		if len(line) > 0 {
			if body.Len() == 0 {
				body.WriteString(line)
			} else {
				body.WriteString(fmt.Sprintf("%s%s", strings.Repeat(" ", 4), line))
			}
			body.WriteString("\n")
		}
	}

	return fmt.Sprintf(
		"### Title: %s\n\n%s",
		title,
		strings.TrimSpace(body.String()),
	)
}

// View renders the shaped issue preview with approve/reject actions.
func (m *ShapeIssueModal) View() string {
	// Apply basic styling to themedView
	styled := formatShapedIssue(m.entry)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0f0")).Render("SHAPE BUG REPORT\n"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("### Title: %s\n", m.entry.Title))
	b.WriteString(styled)

	hints := []string{
		"Preview shaped issue    [↑↓] Navigate",
		"Approve creation       [y/Y/Enter]",
		"Cancel                 [n/N/Esc/Q]",
	}
	for _, hint := range hints {
		b.WriteString(fmt.Sprintf("  %s\n", hint))
	}

	return b.String()
}

// Title returns the modal title.
func (m *ShapeIssueModal) Title() string {
	return m.title
}

// Init initializes the modal.
func (m *ShapeIssueModal) Init() tea.Cmd {
	return nil
}

// Update handles key input for approval flow.
func (m *ShapeIssueModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			return m, m.SubmitCmd() // Approve - SubmitCmd will be scheduled
		case "n", "N", "esc", "q":
			return m, nil // Cancel
		}
	}
	return m, nil
}

// SubmitCmd creates the GitHub issue and returns confirmation message.
func (m *ShapeIssueModal) SubmitCmd() tea.Cmd {
	return func() tea.Msg {
		issueNumber, url, err := m.createGitHubIssue()
		if err != nil {
			return ShapeIssueCreationFailedMsg{Err: err}
		}

		entry := &domain.LabEntry{
			ID:      m.entry.ID,
			Title:   m.entry.Title,
			Kind:    m.entry.Kind,
			Content: m.entry.Content,
			Created: m.entry.Created,
		}

		return ShapeIssueConfirmedMsg{
			IssueNumber: issueNumber,
			URL:         url,
			Entry:       entry,
		}
	}
}

// createGitHubIssue uses GitHub REST API to create the issue.
func (m *ShapeIssueModal) createGitHubIssue() (int, string, error) {
	ghToken := os.Getenv("GH_TOKEN")
	if ghToken == "" {
		return 0, "", fmt.Errorf("no GH_TOKEN environment variable set")
	}

	// Extract owner and repo from path (e.g., "/home/user/repo")
	parts := strings.Split(m.repoPath, "/")
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("cannot parse repository path: %s", m.repoPath)
	}
	lastTwo := parts[len(parts)-2:] // Assumes format: /owner/repo
	owner := lastTwo[0]
	name := lastTwo[1]

	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/issues",
		owner, name,
	)

	requestBody := IssueCreateRequest{
		Title:  m.entry.Title,
		Body:   formatShapedIssue(m.entry),
		Labels: []string{"bug"},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return 0, "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return 0, "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+ghToken)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return 0, "", fmt.Errorf(
			"issue creation failed with status %d: %s",
			resp.StatusCode, string(body),
		)
	}

	var response IssueCreateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, "", fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Number, response.URL, nil
}

// IssueCreateRequest represents the GitHub API request body for creating an issue.
type IssueCreateRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// IssueCreateResponse represents the GitHub API response for a created issue.
type IssueCreateResponse struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
}
