// Package domain provides the core types for Grove's data model.
package domain

import (
	"regexp"
	"strings"
	"time"
)

// LabEntry represents a captured idea or bug in the Lab view.
type LabEntry struct {
	ID       string `json:"id"`           // Unique entry ID (slugified)
	Title    string `json:"title"`        // Entry title
	Kind     string `json:"kind"`         // "idea" or "bug"
	Content  string `json:"content"`      // Content/description of the idea/bug
	Created  string `json:"created"`      // Unix timestamp in RFC3339 format
}

// slugify converts a string to a URL-safe, lowercase slug.
// It removes non-alphanumeric characters (except hyphens and underscores),
// replaces spaces with hyphens, and collapses multiple hyphens/underscores.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9_\-\u00C0-\u024F]`).ReplaceAllString(s, "")
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
