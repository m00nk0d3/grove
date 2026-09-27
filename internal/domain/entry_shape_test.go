// Package domain provides the core types for Grove's data model.
package domain_test

import (
	"strings"
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
)

// ---------------------------------------------------------------------------
// Shape Workflow Tests - Issue #231
// Tests formatShapedIssue function for transforming bug entries into structured GitHub issues
// ---------------------------------------------------------------------------

func TestFormatShapedIssue_EmptyEntry_ReturnsMinimalStructure(t *testing.T) {
	entry := domain.LabEntry{
		ID:    "test-id",
		Title: "", // Empty title
		Kind:  "bug",
		Content: "", // Empty content
		Created: "2024-01-01T00:00:00Z",
	}

	result := domain.FormatShapedIssue(entry)

	if result == "" {
		t.Error("Expected shaped issue to have structure, got empty string")
	}

	// Should include title placeholder since content is empty
	if !contains(result, "Title") {
		t.Error("Expected shaped issue to mention title field")
	}
}

func TestFormatShapedIssue_SingleLineContent_UsesAsBody(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Fix Null Pointer Exception",
		Kind:    "bug",
		Content: "NullPointerException at line 42 when calling process()",
		Created: "2024-01-01T00:00:00Z",
	}

	result := domain.FormatShapedIssue(entry)

	if !contains(result, "Fix Null Pointer Exception") {
		t.Error("Expected shaped issue to include title from entry")
	}

	if !contains(result, "NullPointerException at line 42 when calling process()") {
		t.Error("Expected shaped issue to include content as body")
	}
}

func TestFormatShapedIssue_MultipleLinesContent_IndentsSubsequentLines(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Fix Null Pointer Exception",
		Kind:    "bug",
		Content: "NullPointerException at line 42\nwhen calling process()\n\nAdditional context here",
		Created: "2024-01-01T00:00:00Z",
	}

	result := domain.FormatShapedIssue(entry)

	if !contains(result, "Fix Null Pointer Exception") {
		t.Error("Expected shaped issue to include title")
	}

	// Subsequent lines should be indented
	lines := splitLines(result)
	if len(lines) < 2 {
		t.Fatal("Expected at least two lines in result")
	}

	// First line is "### Title: ..."
	if !startsWith(lines[0], "### Title:") {
		t.Errorf("First line should start with '### Title:', got %q", lines[0])
	}

	// Second+ lines should have leading spaces (indentation)
	for i, line := range lines {
		if i > 0 && !startsWith(line, "    ") && !startsWith(line, "\t") && len(strings.TrimSpace(line)) > 0 {
			t.Errorf("Line %d should be indented with 4 spaces or tab, got %q", i+1, line)
		}
	}
}

func TestFormatShapedIssue_NoTitleUsesFirstContentLine(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "", // Empty title
		Kind:    "bug",
		Content: "This is the first line of the bug report",
		Created: "2024-01-01T00:00:00Z",
	}

	result := domain.FormatShapedIssue(entry)

	// Should use first content line as title
	if !contains(result, "This is the first line of the bug report") {
		t.Error("Expected shaped issue to use first content line as title when title is empty")
	}
}

func TestFormatShapedIssue_BlankLineAfterTitle_Skipped(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Bug Title Here",
		Kind:    "bug",
		Content: "Actual bug content here\n\nMore content after blank line",
		Created: "2024-01-01T00:00:00Z",
	}

	result := domain.FormatShapedIssue(entry)

	lines := splitLines(result)
	// Line 0 should be title section
	// Line 1+ should start with content (blank line after title should be skipped)
	if len(lines) >= 3 {
		// Check that there's no completely empty line between title and content
		for i, line := range lines {
			if strings.TrimSpace(line) == "" && i > 0 && !startsWith(lines[i-1], "### Title:") {
				t.Errorf("Found unexpected blank line at position %d", i)
			}
		}
	}
}

func TestFormatShapedIssue_TitleAndContentBothNonEmpty(t *testing.T) {
	entry := domain.LabEntry{
		ID:      "test-id",
		Title:   "Fix Memory Leak in Cache Module",
		Kind:    "bug",
		Content: "Memory leak detected when cache grows beyond 10MB threshold\n\nRepro steps:\n1. Create cache instance\n2. Add items until memory usage > 9MB\n3. Observe memory not being released\n4. Application eventually OOMs",
		Created: "2024-01-01T00:00:00Z",
	}

	result := domain.FormatShapedIssue(entry)

	if !contains(result, "Fix Memory Leak in Cache Module") {
		t.Error("Expected shaped issue to include title")
	}

	if !contains(result, "Memory leak detected when cache grows beyond 10MB threshold") {
		t.Error("Expected shaped issue to include content")
	}

	if !contains(result, "Repro steps:") {
		t.Error("Expected shaped issue to preserve repro steps section")
	}
}

// Helper functions for testing

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if result == nil || len(result) == 0 {
			result = append(result, line)
		} else if lastLineEmpty := result[len(result)-1] == ""; lastLineEmpty {
			result = append(result, line)
		} else if i > 0 && strings.TrimSpace(lines[i-1]) != "" {
			// Non-empty previous line, include current non-empty line
			if strings.TrimSpace(line) != "" {
				result = append(result, line)
			}
		}
	}
	return result
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
