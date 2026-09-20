package domain

import (
	"fmt"
	"time"
)

// PullRequestActivity represents a GitHub PR conversation comment or review.
type PullRequestActivity struct {
	Author    string
	Body      string
	State     string
	URL       string
	CreatedAt time.Time
}

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Number            int
	Title             string
	Body              string
	Branch            string
	Author            string
	State             string // "OPEN", "MERGED", "CLOSED"
	ReviewDecision    string // "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED", or ""
	Labels            []string
	IsDraft           bool
	Assignees         []string
	Comments          []PullRequestActivity
	Reviews           []PullRequestActivity
	UnresolvedThreads int
	ChecksFailing     bool
	MergeConflict     bool
	IsMine            bool
	ReviewRequested   bool
}

// AttentionReasons describes why the current viewer should act on this PR.
func (pr PullRequest) AttentionReasons() []string {
	var reasons []string
	if pr.ReviewRequested {
		reasons = append(reasons, "review requested")
	}
	if !pr.IsMine {
		return reasons
	}
	if pr.ReviewDecision == "CHANGES_REQUESTED" {
		reasons = append(reasons, "changes requested")
	}
	if pr.UnresolvedThreads > 0 {
		reasons = append(reasons, pluralizePRCount(pr.UnresolvedThreads, "unresolved thread"))
	}
	if pr.ChecksFailing {
		reasons = append(reasons, "checks failing")
	}
	if pr.MergeConflict {
		reasons = append(reasons, "merge conflict")
	}
	return reasons
}

// NeedsAttention reports whether the current viewer has an actionable PR task.
func (pr PullRequest) NeedsAttention() bool {
	return len(pr.AttentionReasons()) > 0
}

func pluralizePRCount(count int, label string) string {
	if count == 1 {
		return "1 " + label
	}
	return fmt.Sprintf("%d %ss", count, label)
}
