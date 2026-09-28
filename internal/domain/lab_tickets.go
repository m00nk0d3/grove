package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// LabTicket is one tracer-bullet ticket drafted by a grilling session.
type LabTicket struct {
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	BlockedBy []string `json:"blocked_by"`
}

// ParseLabTickets reads a drafted tickets.json and returns its tickets in
// dependency order: every ticket after the tickets that block it, and
// otherwise in key order. It rejects a draft that could not be published as
// written: a missing key or title, a repeated key, a blocker that is not a
// ticket, a ticket that blocks itself, or a cycle.
func ParseLabTickets(raw string) ([]LabTicket, error) {
	var doc struct {
		Tickets []LabTicket `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("tickets.json is not valid: %w", err)
	}
	if len(doc.Tickets) == 0 {
		return nil, errors.New("tickets.json has no tickets")
	}
	byKey := make(map[string]LabTicket, len(doc.Tickets))
	for _, t := range doc.Tickets {
		t.Key = strings.TrimSpace(t.Key)
		switch {
		case t.Key == "":
			return nil, errors.New("tickets.json has a ticket without a key")
		case strings.TrimSpace(t.Title) == "":
			return nil, fmt.Errorf("ticket %s has no title", t.Key)
		}
		if _, dup := byKey[t.Key]; dup {
			return nil, fmt.Errorf("ticket key %s is used twice", t.Key)
		}
		byKey[t.Key] = t
	}
	for _, t := range byKey {
		for _, b := range t.BlockedBy {
			if b == t.Key {
				return nil, fmt.Errorf("ticket %s blocks itself", t.Key)
			}
			if _, ok := byKey[b]; !ok {
				return nil, fmt.Errorf("ticket %s is blocked by %s, which is not a ticket", t.Key, b)
			}
		}
	}

	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([]LabTicket, 0, len(keys))
	placed := make(map[string]bool, len(keys))
	for len(ordered) < len(keys) {
		progressed := false
		for _, k := range keys {
			if placed[k] {
				continue
			}
			ready := true
			for _, b := range byKey[k].BlockedBy {
				if !placed[b] {
					ready = false
				}
			}
			if ready {
				ordered = append(ordered, byKey[k])
				placed[k] = true
				progressed = true
			}
		}
		if !progressed {
			var stuck []string
			for _, k := range keys {
				if !placed[k] {
					stuck = append(stuck, k)
				}
			}
			return nil, fmt.Errorf("tickets %s block each other in a cycle", strings.Join(stuck, ", "))
		}
	}
	return ordered, nil
}

// Artifacts a session writes for Grove rather than for the repository.
const (
	LabSpecArtifact    = "spec.md"
	LabTicketsArtifact = "tickets.json"
	LabIssueArtifact   = "issue.md"
)

// IsLabRepositoryDocument reports whether an artifact is a repository
// document, such as CONTEXT.md or a decision record, drafted at its
// repository-relative path and copied into the checkout when approved.
func IsLabRepositoryDocument(path string) bool {
	switch path {
	case LabSpecArtifact, LabTicketsArtifact, LabIssueArtifact:
		return false
	}
	return true
}
