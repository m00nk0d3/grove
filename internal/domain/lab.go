package domain

import (
	"crypto/rand"
	"crypto/sha256"
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
	// Published records, for an epic, each ticket's issue and links by ticket
	// key as they are made, so an interrupted publication resumes where it
	// stopped instead of creating or linking anything twice.
	Published map[string]LabPublishedTicket `json:"published,omitempty"`
	// BugLinked records that the issue of a bug escalated to a grill has been
	// made a sub-issue of the epic that grew out of it.
	BugLinked bool `json:"bug_linked,omitempty"`
}

// LabPublishedTicket is the publication progress of one ticket.
type LabPublishedTicket struct {
	Number   int  `json:"number"`
	SubIssue bool `json:"sub_issue,omitempty"`
	// BlockedBy lists the keys of the blockers already linked.
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// Empty reports whether no issue has been published.
func (i LabIssues) Empty() bool {
	return i.Epic == nil && i.Issue == nil && len(i.Tickets) == 0
}

// LabReviewState is the user's decision on a drafted artifact.
type LabReviewState string

const (
	LabReviewDraft     LabReviewState = "draft"
	LabReviewApproved  LabReviewState = "approved"
	LabReviewDiscarded LabReviewState = "discarded"
)

// LabReview records a decision on one artifact and the content it was made
// on, so a later revision of the file is reviewed again.
type LabReview struct {
	State LabReviewState `json:"state"`
	Hash  string         `json:"hash"`
}

// LabContentHash identifies an artifact's content for review.
func LabContentHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
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
	// Reviews holds the user's decisions on drafted artifacts, by artifact
	// path.
	Reviews map[string]LabReview `json:"reviews,omitempty"`
	Created time.Time            `json:"created"`
	Updated time.Time            `json:"updated"`
}

// ReviewOf returns the decision on an artifact as it is now. A decision made
// on different content no longer applies: the artifact is a draft again.
func (e LabEntry) ReviewOf(a LabArtifact) LabReviewState {
	r, ok := e.Reviews[a.Path]
	if !ok || r.Hash != LabContentHash(a.Body) {
		return LabReviewDraft
	}
	return r.State
}

// SetReview records a decision on an artifact's current content.
func (e *LabEntry) SetReview(a LabArtifact, state LabReviewState) {
	if e.Reviews == nil {
		e.Reviews = make(map[string]LabReview)
	}
	e.Reviews[a.Path] = LabReview{State: state, Hash: LabContentHash(a.Body)}
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

// LabStages returns the steps of the run an entry takes: a grill for ideas and
// escalated bugs, a shape for bugs. The entry's mode decides once a run has
// started; before that, its kind does.
func (e LabEntry) LabStages() []string {
	mode := e.Mode
	if mode == "" {
		mode = LabModeGrill
		if e.Kind == LabKindBug {
			mode = LabModeShape
		}
	}
	if mode == LabModeShape {
		return []string{"Shape", "Review", "Publish"}
	}
	return []string{"Scout", "Interview", "Spec", "Tickets", "Publish"}
}

// Stage returns the 1-based step of LabStages the entry has reached, or 0 for
// a draft.
func (e LabEntry) Stage() int {
	switch e.Status {
	case LabStatusShaping:
		return 1
	case LabStatusGrilling:
		// Grilling covers scouting and the interview; the interview is
		// where it waits on the user.
		return 2
	case LabStatusSpecced:
		return 3
	case LabStatusTicketed:
		return 4
	case LabStatusPublished:
		return len(e.LabStages())
	default:
		return 0
	}
}

// LabArtifact is a file an agent drafted for an entry, such as CONTEXT.md, an
// ADR, the spec, or the tickets. Path is relative to the entry's artifacts
// directory and uses forward slashes.
type LabArtifact struct {
	Path    string
	Body    string
	ModTime time.Time
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

// ParseIssueDraft splits a drafted issue into its title, the text of its first
// "# " heading, and its body, everything after it. A draft without a heading
// has no title and is all body.
func ParseIssueDraft(draft string) (title, body string) {
	draft = strings.ReplaceAll(draft, "\r\n", "\n")
	lines := strings.Split(draft, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(trimmed[2:]), strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		}
		break
	}
	return "", strings.TrimSpace(draft)
}
