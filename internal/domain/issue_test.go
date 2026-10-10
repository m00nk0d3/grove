package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlugFromTitle(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "simple lowercase words",
			title:    "Add feature X",
			expected: "add-feature-x",
		},
		{
			name:     "project-style title with brackets and slashes",
			title:    "[Phase 1] Implement - Create/delete modals",
			expected: "phase-1-implement-create-delete",
		},
		{
			name:     "more than 5 words gets truncated",
			title:    "one two three four five six seven",
			expected: "one-two-three-four-five",
		},
		{
			name:     "exactly 5 words stays the same",
			title:    "fix bug in the renderer",
			expected: "fix-bug-in-the-renderer",
		},
		{
			name:     "empty string returns empty",
			title:    "",
			expected: "",
		},
		{
			name:     "consecutive special chars collapse to single hyphen",
			title:    "Fix  bug   in renderer",
			expected: "fix-bug-in-renderer",
		},
		{
			name:     "leading and trailing special chars are trimmed",
			title:    "---Fix bug---",
			expected: "fix-bug",
		},
		{
			name:     "only special chars returns empty",
			title:    "---!!!---",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SlugFromTitle(tt.title)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestIssue_IsClosed(t *testing.T) {
	assert.True(t, Issue{State: "CLOSED"}.IsClosed())
	assert.True(t, Issue{State: "closed"}.IsClosed(), "state comparison is case-insensitive")
	assert.False(t, Issue{State: "OPEN"}.IsClosed())
	assert.False(t, Issue{}.IsClosed(), "an empty state is not closed")
}
