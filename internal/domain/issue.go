package domain

import (
	"regexp"
	"strings"
)

// Issue represents a GitHub issue.
type Issue struct {
	Number          int
	Title           string
	Body            string
	Labels          []string
	Assignees       []string
	ParentNumber    *int // nil if this is a top-level issue
	SubIssueNumbers []int // empty if no sub-issues
}

var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

// SlugFromTitle returns a URL-safe hyphenated slug from an issue title.
// It lowercases, strips non-alphanumeric characters, and limits to 5 words.
func SlugFromTitle(title string) string {
	s := strings.ToLower(title)
	s = nonAlnumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}

	parts := strings.Split(s, "-")
	if len(parts) > 5 {
		parts = parts[:5]
	}

	return strings.Join(parts, "-")
}
