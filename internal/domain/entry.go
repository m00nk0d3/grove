// Package domain provides the core types for Grove's data model.
package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// LabEntry represents a captured idea or bug in the Lab view.
type LabEntry struct {
	ID          string `json:"id"`                     // Unique entry ID (slugified)
	Title       string `json:"title"`                  // Entry title
	Kind        string `json:"kind"`                   // "idea" or "bug"
	Content     string `json:"content"`                // Content/description of the idea/bug
	Created     string `json:"created"`                // Unix timestamp in RFC3339 format
	LinkedIssue *int   `json:"linked_issue,omitempty"` // GitHub issue number (optional, set after shape)
}

// slugify converts a string to a URL-safe, lowercase slug.
// It removes non-alphanumeric characters (except hyphens and underscores),
// replaces spaces with hyphens, and collapses multiple hyphens/underscores.
func slugify(s string) string {
	s = strings.ToLower(s)
	// Remove anything that's not a-z, 0-9, _, or -
	s = regexp.MustCompile(`[^a-z0-9_-]`).ReplaceAllString(s, "")
	// Replace multiple hyphens/underscores with single one
	s = regexp.MustCompile(`[-_]+`).ReplaceAllString(s, "-")
	return strings.TrimSpace(s)
}

// CurrentTimestamp returns current Unix timestamp for generating lab entry IDs.
func CurrentTimestamp() int64 {
	return time.Now().Unix()
}

// Slugify is the exported slugify function for public use.
func Slugify(s string) string {
	return slugify(s)
}

// MarshalJSON implements json.Marshaler for LabEntry.
func (e LabEntry) MarshalJSON() ([]byte, error) {
	type alias LabEntry
	return json.Marshal(&struct {
		*alias
		Created *string `json:"created,omitempty"` // Omit created if empty
	}{
		alias:   (*alias)(&e),
		Created: &e.Created,
	})
}

// UnmarshalJSON implements json.Unmarshaler for LabEntry.
func (e *LabEntry) UnmarshalJSON(data []byte) error {
	type alias LabEntry
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*e = LabEntry(a)
	return nil
}

// SetIssueNumber sets the GitHub issue number for a shaped lab entry.
func (e *LabEntry) SetIssueNumber(issueNum int) {
	if e.LinkedIssue == nil {
		e.LinkedIssue = new(int)
	}
	*e.LinkedIssue = issueNum
}

// GetIssueNumber returns the linked GitHub issue number, or nil if not set.
func (e *LabEntry) GetIssueNumber() *int {
	return e.LinkedIssue
}

// FormatShapedIssue transforms a LabEntry into a structured GitHub issue body.
// This is used by the shape workflow to convert bug entries into GitHub issues.
func FormatShapedIssue(entry LabEntry) string {
	// Use entry.Title if provided; otherwise extract first non-empty line from content as title
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

// LabFilter controls which entries are shown in the Lab view.
type LabFilter string

const (
	LabFilterAll  LabFilter = "all"
	LabFilterIdea LabFilter = "idea"
	LabFilterBug  LabFilter = "bug"
)
