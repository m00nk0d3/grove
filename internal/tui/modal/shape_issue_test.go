// Package modal provides TUI modals for Grove.
package modal

import (
	"os"
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// ShapeIssueModal unit tests - Issue #231
// ---------------------------------------------------------------------------

func TestShapeIssueModal_New_ReturnsNil_WhenNotBug(t *testing.T) {
	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Test Idea",
			Kind:    "idea", // Not a bug
			Content: "This is an idea",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)

	assert.Nil(t, modal, "Expected nil for non-bug entries")
}

func TestShapeIssueModal_New_ReturnsValid_WhenBug(t *testing.T) {
	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Test Bug Report",
			Kind:    "bug", // Is a bug
			Content: "This is a bug report content",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)

	assert.NotNil(t, modal, "Expected non-nil for bug entries")
	assert.Equal(t, "SHAPE BUG REPORT", modal.Title())
}

func TestShapeIssueModal_View_ShowsShapedPreview(t *testing.T) {
	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Memory Leak Bug",
			Kind:    "bug",
			Content: "Memory leak in cache module\n\nSteps to reproduce:\n1. Add items\n2. Wait for memory threshold",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)
	view := modal.View()

	assert.Contains(t, view, "SHAPE BUG REPORT")
	assert.Contains(t, view, "Memory Leak Bug")
	assert.Contains(t, view, "Steps to reproduce")
}

func TestShapeIssueModal_Title_ReturnsCorrectTitle(t *testing.T) {
	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Test Title",
			Kind:    "bug",
			Content: "content",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)
	assert.Equal(t, "SHAPE BUG REPORT", modal.Title())
}

func TestShapeIssueModal_Init_ReturnsNil(t *testing.T) {
	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Test Bug",
			Kind:    "bug",
			Content: "content",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)
	cmd := modal.Init()

	assert.Nil(t, cmd, "Expected nil Init command")
}

func TestFormatShapedIssue_EmptyContent_ReturnsTitleOnly(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Test Bug",
		Kind:    "bug",
		Content: "",
		Created: "2024-01-01T00:00:00Z",
	}

	result := formatShapedIssue(entry)
	assert.Contains(t, result, "### Title: Test Bug")
	// Empty content means no body section, just title with trailing newline
	assert.Contains(t, result, "\n\n") // Always has two newlines as structure
}

func TestFormatShapedIssue_SingleLineContent_ReturnsTitleWithBody(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Test Bug",
		Kind:    "bug",
		Content: "This is the bug description",
		Created: "2024-01-01T00:00:00Z",
	}

	result := formatShapedIssue(entry)
	assert.Contains(t, result, "### Title: Test Bug")
	assert.Contains(t, result, "This is the bug description")
}

func TestFormatShapedIssue_MultiLineContent_PreservesFormatting(t *testing.T) {
	content := "Memory leak in cache module\n\nSteps to reproduce:\n1. Add items\n2. Wait for memory threshold"
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Memory Leak Bug",
		Kind:    "bug",
		Content: content,
		Created: "2024-01-01T00:00:00Z",
	}

	result := formatShapedIssue(entry)
	assert.Contains(t, result, "### Title: Memory Leak Bug")
	assert.Contains(t, result, "Steps to reproduce")
}

func TestShapeIssueModal_SubmitCmd_WithoutGH_TOKEN_Fails(t *testing.T) {
	// Save original token
	origToken := os.Getenv("GH_TOKEN")
	defer func() {
		if origToken != "" {
			os.Setenv("GH_TOKEN", origToken)
		} else {
			os.Unsetenv("GH_TOKEN")
		}
	}()

	// Ensure no token is set
	os.Unsetenv("GH_TOKEN")

	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Test Bug",
			Kind:    "bug",
			Content: "content",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)
	cmd := modal.SubmitCmd()
	receivedMsg := cmd()

	// Without GH_TOKEN, submit should fail with token error
	failed, ok := receivedMsg.(ShapeIssueCreationFailedMsg)
	assert.True(t, ok, "Expected ShapeIssueCreationFailedMsg when no GH_TOKEN")
	assert.Contains(t, failed.Err.Error(), "no GH_TOKEN environment variable set")
}

func TestShapeIssueModal_View_StylizedTitle(t *testing.T) {
	msg := ShapeIssueInitMsg{
		Entry: domain.LabEntry{
			ID:      "test-id",
			Title:   "Test Bug",
			Kind:    "bug",
			Content: "description",
			Created: "2024-01-01T00:00:00Z",
		},
		RepoPath: "/home/user/test-repo",
	}

	modal := NewShapeIssueModal(msg)
	view := modal.View()

	assert.Contains(t, view, "SHAPE BUG REPORT")
	assert.Contains(t, view, "Test Bug")
	assert.Contains(t, view, "[y/Y/Enter]") // Approve hint
	assert.Contains(t, view, "[n/N/Esc/Q]") // Cancel hint
}
