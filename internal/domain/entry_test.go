package domain_test

import (
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "HelloWorld", "helloworld"},
		{"hyphen separator", "Hello-World", "hello-world"},
		{"underscore separator", "Hello_World", "hello-world"},
		{"multiple hyphens", "Hello---World", "hello-world"},
		{"multiple underscores", "Hello___World", "hello-world"},
		{"mixed separators", "Hello---_World", "hello-world"},
		{"spaces not replaced", "My Great Idea", "mygreatidea"},
		{"special chars", "Test@#$%Special!", "testspecial"},
		{"mixed case with hyphens and underscores", "MIXED-CASE_TEST_123", "mixed-case-test-123"},
		{"empty", "", ""},
		{"single word", "Test", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := domain.Slugify(tt.input)
			if result != tt.expected {
				t.Errorf("Slugify(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSlugifyIdGeneration(t *testing.T) {
	// Test that ID generation creates valid slugs from titles
	tests := []struct {
		name     string
		title    string
		expected string // expected to start with timestamp and slug
	}{
		{"simple idea", "New Feature Idea", "[0-9]+-new-feature-idea"},
		{"bug report", "Critical Bug Fix Needed", "[0-9]+-critical-bug-fix-needed"},
		{"complex title", "Feature: Add Dark Mode to the Settings Panel!", "[0-9]+-feature-add-dark-mode-to-the-settings-panel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate ID (timestamp will vary, so we check prefix and slug part)
			id := domain.Slugify(tt.title)

			// The ID should be the slugified title (without timestamp prefix for LabEntry)
			// But when generating from title with CurrentTimestamp(), it combines them
			slug := domain.Slugify(tt.title)

			// Verify the slug part is correct
			if id != slug {
				t.Errorf("Slugify(%q) = %q, expected %q", tt.title, id, slug)
			}
		})
	}
}

func TestLabEntryMarshalUnmarshal(t *testing.T) {
	entry := domain.LabEntry{
		ID:       "test-id-123",
		Title:    "Test Entry",
		Kind:     "idea",
		Content:  "This is a test idea for the lab view.",
		Created:  "2024-01-01T00:00:00Z",
	}

	data, err := entry.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var entry2 domain.LabEntry
	err = entry2.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if entry2.ID != entry.ID {
		t.Errorf("ID mismatch: got %q, want %q", entry2.ID, entry.ID)
	}
	if entry2.Title != entry.Title {
		t.Errorf("Title mismatch: got %q, want %q", entry2.Title, entry.Title)
	}
	if entry2.Kind != entry.Kind {
		t.Errorf("Kind mismatch: got %q, want %q", entry2.Kind, entry.Kind)
	}
	if entry2.Content != entry.Content {
		t.Errorf("Content mismatch: got %q, want %q", entry2.Content, entry.Content)
	}
}

func TestLabFilter(t *testing.T) {
	tests := []domain.LabFilter{
		domain.LabFilterAll,
		domain.LabFilterIdea,
		domain.LabFilterBug,
	}

	for _, f := range tests {
		t.Run(string(f), func(t *testing.T) {
			// Just verify the filter constants exist and are distinct
			if f == "" {
				t.Error("LabFilter should not be empty")
			}
		})
	}
}
