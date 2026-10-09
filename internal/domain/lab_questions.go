package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// LabQuestionKind is how a question card is answered.
type LabQuestionKind string

const (
	LabQuestionChoice LabQuestionKind = "choice" // pick one option
	LabQuestionMulti  LabQuestionKind = "multi"  // pick any options
	LabQuestionText   LabQuestionKind = "text"   // a free answer
)

// LabQuestion is a question card the session's agent writes to
// questions/NNN.json. See docs/LAB_DESIGN.md, "Question protocol".
type LabQuestion struct {
	ID       int             `json:"id"`
	Kind     LabQuestionKind `json:"kind"`
	Question string          `json:"question"`
	Context  string          `json:"context"`
	Options  []string        `json:"options,omitempty"`
	// Recommended holds the recommended option indices: one for a choice
	// card, any number for a multi card.
	Recommended []int `json:"-"`
	// RecommendedText is the suggested answer of a text card.
	RecommendedText string `json:"-"`
	Why             string `json:"why"`
}

// LabAnswerRevision is an earlier answer to a question, kept when the user
// revises it.
type LabAnswerRevision struct {
	Choices    []int     `json:"choices,omitempty"`
	Text       string    `json:"text,omitempty"`
	AnsweredAt time.Time `json:"answered_at"`
}

// LabAnswer is Grove's answer to a question, written to
// questions/NNN.answer.json.
type LabAnswer struct {
	ID         int                 `json:"id"`
	Choices    []int               `json:"choices,omitempty"`
	Text       string              `json:"text,omitempty"`
	AnsweredAt time.Time           `json:"answered_at"`
	Revisions  []LabAnswerRevision `json:"revisions,omitempty"`
}

// LabReceipt records that the session runtime delivered an answer or a
// request. Via is "pane" for a question the agent moved past after the user
// answered it in the pane.
type LabReceipt struct {
	Revision int       `json:"revision"`
	Via      string    `json:"via"`
	SentAt   time.Time `json:"sent_at"`
}

// LabReceiptViaPane marks a question answered in the agent's pane.
const LabReceiptViaPane = "pane"

// LabQuestionRecord is a question file and everything written about it.
type LabQuestionRecord struct {
	Number int
	// Question is the parsed card; nil when the card is invalid.
	Question *LabQuestion
	// Err says why the card is invalid.
	Err     string
	Answer  *LabAnswer
	Receipt *LabReceipt
}

// Pending reports whether the card is valid and waiting for an answer.
func (r LabQuestionRecord) Pending() bool {
	return r.Question != nil && r.Answer == nil && r.Receipt == nil
}

// AnsweredInPane reports whether the agent moved past the card after the user
// answered it in the pane, so Grove never recorded the answer.
func (r LabQuestionRecord) AnsweredInPane() bool {
	return r.Answer == nil && r.Receipt != nil && r.Receipt.Via == LabReceiptViaPane
}

// LabPendingQuestions returns the cards waiting for an answer, in order.
func LabPendingQuestions(records []LabQuestionRecord) []LabQuestionRecord {
	var out []LabQuestionRecord
	for _, r := range records {
		if r.Pending() {
			out = append(out, r)
		}
	}
	return out
}

// LabRequestKind is the kind of a message from Grove to a session's agent.
type LabRequestKind string

const (
	LabRequestReply           LabRequestKind = "reply"
	LabRequestChange          LabRequestKind = "change"
	LabRequestFinishInterview LabRequestKind = "finish_interview"
	LabRequestPermission      LabRequestKind = "permission"
)

// LabRequest is a message from Grove to a session's agent, written to
// requests/NNN.json.
type LabRequest struct {
	ID        int            `json:"id"`
	Kind      LabRequestKind `json:"kind"`
	Text      string         `json:"text,omitempty"`
	Allow     *bool          `json:"allow,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// LabSessionPhase is what a live session is doing or waiting for.
type LabSessionPhase string

const (
	LabPhaseStarting   LabSessionPhase = "starting"
	LabPhaseWorking    LabSessionPhase = "working"
	LabPhaseQuestion   LabSessionPhase = "question"
	LabPhaseFallback   LabSessionPhase = "fallback"
	LabPhasePermission LabSessionPhase = "permission"
	LabPhaseReview     LabSessionPhase = "review"
)

// LabSession is the live session state the runtime writes to session.json.
type LabSession struct {
	Phase   LabSessionPhase `json:"phase"`
	Stage   string          `json:"stage"`
	PaneID  string          `json:"pane_id"`
	Agent   string          `json:"agent"`
	Pending []int           `json:"pending"`
	Output  string          `json:"output,omitempty"`
	Counts  struct {
		Repairs   int `json:"repairs"`
		Fallbacks int `json:"fallbacks"`
	} `json:"counts"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ParseLabQuestion validates a question card read from questions/NNN.json,
// where id is NNN. It accepts exactly the cards the session runtime accepts
// (runtime/sandcastle/src/lab-protocol.ts); both are tested against
// runtime/sandcastle/testdata/lab-question-cards.json.
func ParseLabQuestion(raw []byte, id int) (*LabQuestion, error) {
	if !json.Valid(raw) {
		var probe any
		return nil, fmt.Errorf("the file is not valid JSON (%v)", json.Unmarshal(raw, &probe))
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, fmt.Errorf("the file must hold one JSON object")
	}
	var gotID float64
	if json.Unmarshal(fields["id"], &gotID) != nil || gotID != float64(id) {
		return nil, fmt.Errorf(`"id" must be %d, the number in the file name`, id)
	}
	q := &LabQuestion{ID: id}
	var kind string
	_ = json.Unmarshal(fields["kind"], &kind)
	q.Kind = LabQuestionKind(kind)
	if q.Kind != LabQuestionChoice && q.Kind != LabQuestionMulti && q.Kind != LabQuestionText {
		return nil, fmt.Errorf(`"kind" must be "choice", "multi", or "text"`)
	}
	for _, f := range []struct {
		name string
		dst  *string
	}{{"question", &q.Question}, {"context", &q.Context}, {"why", &q.Why}} {
		var s string
		if json.Unmarshal(fields[f.name], &s) != nil || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf(`"%s" must be a non-empty string`, f.name)
		}
		*f.dst = strings.TrimSpace(s)
	}

	if q.Kind == LabQuestionText {
		if _, ok := fields["options"]; ok {
			return nil, fmt.Errorf(`a "text" question has no "options"`)
		}
		var s string
		if json.Unmarshal(fields["recommended"], &s) != nil || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf(`"recommended" must be a suggested answer string for a "text" question`)
		}
		q.RecommendedText = strings.TrimSpace(s)
		return q, nil
	}

	var options []string
	if json.Unmarshal(fields["options"], &options) != nil || len(options) < 2 || len(options) > 4 {
		return nil, fmt.Errorf(`"options" must be a list of 2 to 4 strings for a "%s" question`, q.Kind)
	}
	for i, o := range options {
		if strings.TrimSpace(o) == "" {
			return nil, fmt.Errorf(`every entry in "options" must be a non-empty string`)
		}
		options[i] = strings.TrimSpace(o)
	}
	q.Options = options
	index := func(v float64) (int, bool) {
		if v != math.Trunc(v) || v < 0 || int(v) >= len(options) {
			return 0, false
		}
		return int(v), true
	}

	if q.Kind == LabQuestionChoice {
		var v float64
		i, ok := 0, json.Unmarshal(fields["recommended"], &v) == nil
		if ok {
			i, ok = index(v)
		}
		if !ok {
			return nil, fmt.Errorf(`"recommended" is %s but must be an option index from 0 to %d`, strings.TrimSpace(string(fields["recommended"])), len(options)-1)
		}
		q.Recommended = []int{i}
		return q, nil
	}

	var values []float64
	if json.Unmarshal(fields["recommended"], &values) != nil || len(values) == 0 {
		return nil, fmt.Errorf(`"recommended" must be a list of option indices from 0 to %d for a "multi" question`, len(options)-1)
	}
	seen := make(map[int]bool, len(values))
	for _, v := range values {
		i, ok := index(v)
		if !ok {
			return nil, fmt.Errorf(`"recommended" must be a list of option indices from 0 to %d for a "multi" question`, len(options)-1)
		}
		if seen[i] {
			return nil, fmt.Errorf(`"recommended" lists the same option twice`)
		}
		seen[i] = true
		q.Recommended = append(q.Recommended, i)
	}
	return q, nil
}
