package domain

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Number         int
	Title          string
	Body           string
	Branch         string
	Author         string
	State          string // "OPEN", "MERGED", "CLOSED"
	ReviewDecision string // "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED", or ""
	Labels         []string
	IsDraft        bool
	Assignees      []string
}
