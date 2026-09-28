package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// LabKind is the kind of a Lab entry.
type LabKind string

const (
	LabKindIdea LabKind = "idea"
	LabKindBug  LabKind = "bug"
)

// Valid reports whether k is a known kind.
func (k LabKind) Valid() bool {
	return k == LabKindIdea || k == LabKindBug
}

// LabStatus is a Lab entry's position in its lifecycle. See docs/LAB_DESIGN.md.
type LabStatus string

const (
	LabStatusDraft     LabStatus = "draft"
	LabStatusGrilling  LabStatus = "grilling"
	LabStatusSpecced   LabStatus = "specced"
	LabStatusTicketed  LabStatus = "ticketed"
	LabStatusShaping   LabStatus = "shaping"
	LabStatusPublished LabStatus = "published"
)

// LabMode is the build path an entry takes once a run starts.
type LabMode string

const (
	LabModeGrill LabMode = "grill"
	LabModeShape LabMode = "shape"
)

// LabIssues records the GitHub issues an entry has published. A grilled entry
// publishes an epic and its tickets; a shaped bug publishes a single issue.
type LabIssues struct {
	Epic    *int  `json:"epic,omitempty"`
	Tickets []int `json:"tickets,omitempty"`
	Issue   *int  `json:"issue,omitempty"`
}

// Empty reports whether no issue has been published.
func (i LabIssues) Empty() bool {
	return i.Epic == nil && i.Issue == nil && len(i.Tickets) == 0
}

// LabEntry is an idea or bug captured in the Lab.
type LabEntry struct {
	ID       string    `json:"id"`
	Kind     LabKind   `json:"kind"`
	Text     string    `json:"text"`
	Status   LabStatus `json:"status"`
	Mode     LabMode   `json:"mode,omitempty"`
	Archived bool      `json:"archived,omitempty"`
	Issues   LabIssues `json:"issues,omitempty"`
	Runs     []string  `json:"runs,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

// NewLabEntry returns a draft entry with a fresh ID. The text is normalised
// with NormalizeLabText.
func NewLabEntry(kind LabKind, text string, now time.Time) LabEntry {
	now = now.UTC()
	return LabEntry{
		ID:      newLabID(now),
		Kind:    kind,
		Text:    NormalizeLabText(text),
		Status:  LabStatusDraft,
		Created: now,
		Updated: now,
	}
}

// newLabID returns an ID that sorts by capture time and is unique across
// Grove instances capturing in the same second.
func newLabID(now time.Time) string {
	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("%s-%06d", now.Format("20060102-150405"), now.Nanosecond()/1000)
	}
	return now.Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:])
}

// NormalizeLabText converts line endings to \n and trims surrounding blank
// space.
func NormalizeLabText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

// Title returns the first non-blank line of the entry's text.
func (e LabEntry) Title() string {
	for _, line := range strings.Split(e.Text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// Body returns the text after the title line, with surrounding blank space
// trimmed.
func (e LabEntry) Body() string {
	text := strings.TrimSpace(e.Text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[i+1:])
	}
	return ""
}

// Editable reports whether the entry's text may still change. Once a run has
// started, the text is the run's input and stays fixed.
func (e LabEntry) Editable() bool {
	return e.Status == LabStatusDraft
}

// Deletable reports whether the entry may be deleted: a draft or an archived
// entry. Whether a run is live is checked separately by the caller.
func (e LabEntry) Deletable() bool {
	return e.Status == LabStatusDraft || e.Archived
}

// LabFilter restricts the Lab list to one kind of entry.
type LabFilter string

const (
	LabFilterAll  LabFilter = "all"
	LabFilterIdea LabFilter = "idea"
	LabFilterBug  LabFilter = "bug"
)

// Matches reports whether e is shown under the filter.
func (f LabFilter) Matches(e LabEntry) bool {
	switch f {
	case LabFilterIdea:
		return e.Kind == LabKindIdea
	case LabFilterBug:
		return e.Kind == LabKindBug
	default:
		return true
	}
}
