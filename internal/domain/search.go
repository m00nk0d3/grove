package domain

// ResultKind identifies the kind of search result.
type ResultKind string

const (
	KindWorktree ResultKind = "worktree"
	KindIssue    ResultKind = "issue"
	KindPR       ResultKind = "pr"
	KindFile     ResultKind = "file"
	KindBranch   ResultKind = "branch"
	KindWorkflow ResultKind = "workflow"
	KindCommit   ResultKind = "commit"
)

// SearchResult is a unified result type returned by the fuzzy finder across all sources.
type SearchResult struct {
	Kind    ResultKind // worktree | issue | pr | file | branch | workflow | commit
	Label   string     // primary display text
	Sub     string     // secondary info (issue number, branch, etc.)
	Icon    string     // emoji prefix
	Payload any        // domain.Worktree | domain.Issue | domain.PullRequest | domain.WorkflowRunRef | string
}
