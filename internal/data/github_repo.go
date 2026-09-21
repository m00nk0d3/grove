package data

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/m00nk0d3/grove/internal/exec"
)

// NormalizeRepoPath returns a canonical form of a repository path for use as a
// cache key. Grove derives the repository path from both os.Getwd() and
// `git worktree list`, which disagree on path separators on Windows; without
// normalization the same repository is cached twice under two spellings.
func NormalizeRepoPath(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(repoPath))
}

// GitHubRepository persists and retrieves GitHub PR and Issue data from SQLite.
// All reads and writes are scoped to repoPath so that nexus instances run from
// different repositories never mix their cached issues or PRs.
type GitHubRepository struct {
	db       *DB
	repoPath string
}

// NewGitHubRepository creates a new GitHubRepository backed by the given DB,
// scoped to repoPath (typically the result of os.Getwd() at startup).
func NewGitHubRepository(db *DB, repoPath string) *GitHubRepository {
	return &GitHubRepository{db: db, repoPath: NormalizeRepoPath(repoPath)}
}

// UpsertPRs inserts or replaces all provided pull requests in the cache.
func (r *GitHubRepository) UpsertPRs(prs []domain.PullRequest) error {
	for _, pr := range prs {
		labels, err := json.Marshal(pr.Labels)
		if err != nil {
			return fmt.Errorf("upsert prs: marshal labels for pr %d: %w", pr.Number, err)
		}
		assignees, err := json.Marshal(pr.Assignees)
		if err != nil {
			return fmt.Errorf("upsert prs: marshal assignees for pr %d: %w", pr.Number, err)
		}
		comments, err := json.Marshal(pr.Comments)
		if err != nil {
			return fmt.Errorf("upsert prs: marshal comments for pr %d: %w", pr.Number, err)
		}
		reviews, err := json.Marshal(pr.Reviews)
		if err != nil {
			return fmt.Errorf("upsert prs: marshal reviews for pr %d: %w", pr.Number, err)
		}

		_, err = r.db.Conn.Exec(`
			INSERT INTO github_prs (
				number, repo_path, title, body, branch, author, state, review_decision,
				is_draft, labels, assignees, comments, reviews, unresolved_threads,
				checks_failing, merge_conflict, is_mine, review_requested, synced_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(number, repo_path) DO UPDATE SET
				title              = excluded.title,
				body               = excluded.body,
				branch             = excluded.branch,
				author             = excluded.author,
				state              = excluded.state,
				review_decision    = excluded.review_decision,
				is_draft           = excluded.is_draft,
				labels             = excluded.labels,
				assignees          = excluded.assignees,
				comments           = excluded.comments,
				reviews            = excluded.reviews,
				unresolved_threads = excluded.unresolved_threads,
				checks_failing     = excluded.checks_failing,
				merge_conflict     = excluded.merge_conflict,
				is_mine            = excluded.is_mine,
				review_requested   = excluded.review_requested,
				synced_at          = CURRENT_TIMESTAMP
		`, pr.Number, r.repoPath, pr.Title, pr.Body, pr.Branch, pr.Author, pr.State,
			pr.ReviewDecision, pr.IsDraft, string(labels), string(assignees),
			string(comments), string(reviews), pr.UnresolvedThreads, pr.ChecksFailing,
			pr.MergeConflict, pr.IsMine, pr.ReviewRequested)
		if err != nil {
			return fmt.Errorf("upsert prs: %w", err)
		}
	}
	return nil
}

// GetPRs returns all cached pull requests for this repository.
func (r *GitHubRepository) GetPRs() ([]domain.PullRequest, error) {
	rows, err := r.db.Conn.Query(`
		SELECT number, title, body, branch, author, state, review_decision,
		       is_draft, labels, assignees, comments, reviews, unresolved_threads,
		       checks_failing, merge_conflict, is_mine, review_requested
		FROM github_prs
		WHERE repo_path = ?
		ORDER BY number
	`, r.repoPath)
	if err != nil {
		return nil, fmt.Errorf("get prs: %w", err)
	}
	defer rows.Close()

	var prs []domain.PullRequest
	for rows.Next() {
		var pr domain.PullRequest
		var labelsJSON string
		var assigneesJSON string
		var commentsJSON string
		var reviewsJSON string
		var isDraftInt int
		var checksFailingInt int
		var mergeConflictInt int
		var isMineInt int
		var reviewRequestedInt int

		if err := rows.Scan(
			&pr.Number, &pr.Title, &pr.Body, &pr.Branch, &pr.Author, &pr.State,
			&pr.ReviewDecision, &isDraftInt, &labelsJSON, &assigneesJSON,
			&commentsJSON, &reviewsJSON, &pr.UnresolvedThreads, &checksFailingInt,
			&mergeConflictInt, &isMineInt, &reviewRequestedInt,
		); err != nil {
			return nil, fmt.Errorf("get prs: scan row: %w", err)
		}

		pr.IsDraft = isDraftInt != 0
		pr.ChecksFailing = checksFailingInt != 0
		pr.MergeConflict = mergeConflictInt != 0
		pr.IsMine = isMineInt != 0
		pr.ReviewRequested = reviewRequestedInt != 0

		if err := json.Unmarshal([]byte(labelsJSON), &pr.Labels); err != nil {
			return nil, fmt.Errorf("get prs: parse labels for pr %d: %w", pr.Number, err)
		}
		if err := json.Unmarshal([]byte(assigneesJSON), &pr.Assignees); err != nil {
			return nil, fmt.Errorf("get prs: parse assignees for pr %d: %w", pr.Number, err)
		}
		if err := json.Unmarshal([]byte(commentsJSON), &pr.Comments); err != nil {
			return nil, fmt.Errorf("get prs: parse comments for pr %d: %w", pr.Number, err)
		}
		if err := json.Unmarshal([]byte(reviewsJSON), &pr.Reviews); err != nil {
			return nil, fmt.Errorf("get prs: parse reviews for pr %d: %w", pr.Number, err)
		}

		prs = append(prs, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get prs: rows error: %w", err)
	}

	if prs == nil {
		prs = []domain.PullRequest{}
	}
	return prs, nil
}

// UpsertIssues inserts or replaces all provided issues in the cache.
func (r *GitHubRepository) UpsertIssues(issues []domain.Issue) error {
	for _, issue := range issues {
		labels, err := json.Marshal(issue.Labels)
		if err != nil {
			return fmt.Errorf("upsert issues: marshal labels for issue %d: %w", issue.Number, err)
		}

		assigneeLogins := issue.Assignees
		if assigneeLogins == nil {
			assigneeLogins = []string{}
		}
		assignees, err := json.Marshal(assigneeLogins)
		if err != nil {
			return fmt.Errorf("upsert issues: marshal assignees for issue %d: %w", issue.Number, err)
		}

		subNums := issue.SubIssueNumbers
		if subNums == nil {
			subNums = []int{}
		}
		subNumsJSON, err := json.Marshal(subNums)
		if err != nil {
			return fmt.Errorf("upsert issues: marshal sub_issue_numbers for issue %d: %w", issue.Number, err)
		}

		var parentNum interface{}
		if issue.ParentNumber != nil {
			parentNum = *issue.ParentNumber
		}

		_, err = r.db.Conn.Exec(`
			INSERT INTO github_issues (number, repo_path, title, body, state, project_status, labels, assignees, parent_number, sub_issue_numbers, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(number, repo_path) DO UPDATE SET
				title              = excluded.title,
				body               = excluded.body,
				state              = excluded.state,
				project_status     = excluded.project_status,
				labels             = excluded.labels,
				assignees          = excluded.assignees,
				parent_number      = excluded.parent_number,
				sub_issue_numbers  = excluded.sub_issue_numbers,
				synced_at          = CURRENT_TIMESTAMP
		`, issue.Number, r.repoPath, issue.Title, issue.Body, issue.State, issue.ProjectStatus, string(labels), string(assignees), parentNum, string(subNumsJSON))
		if err != nil {
			return fmt.Errorf("upsert issues: %w", err)
		}
	}
	return nil
}

// GetIssues returns all cached issues for this repository.
func (r *GitHubRepository) GetIssues() ([]domain.Issue, error) {
	rows, err := r.db.Conn.Query(`
		SELECT number, title, body, state, project_status, labels, assignees, parent_number, sub_issue_numbers
		FROM github_issues
		WHERE repo_path = ?
		ORDER BY number
	`, r.repoPath)
	if err != nil {
		return nil, fmt.Errorf("get issues: %w", err)
	}
	defer rows.Close()

	var issues []domain.Issue
	for rows.Next() {
		var issue domain.Issue
		var body, state, projectStatus, assigneesJSON sql.NullString
		var labelsJSON string
		var parentNum sql.NullInt64
		var subNumsJSON string

		if err := rows.Scan(&issue.Number, &issue.Title, &body, &state, &projectStatus, &labelsJSON, &assigneesJSON, &parentNum, &subNumsJSON); err != nil {
			return nil, fmt.Errorf("get issues: scan row: %w", err)
		}

		issue.Body = body.String
		issue.State = state.String
		issue.ProjectStatus = projectStatus.String

		if err := json.Unmarshal([]byte(labelsJSON), &issue.Labels); err != nil {
			return nil, fmt.Errorf("get issues: parse labels for issue %d: %w", issue.Number, err)
		}

		if assigneesJSON.Valid && assigneesJSON.String != "" {
			var logins []string
			if err := json.Unmarshal([]byte(assigneesJSON.String), &logins); err != nil {
				return nil, fmt.Errorf("get issues: parse assignees for issue %d: %w", issue.Number, err)
			}
			if len(logins) > 0 {
				issue.Assignees = logins
			}
		}

		if parentNum.Valid {
			n := int(parentNum.Int64)
			issue.ParentNumber = &n
		}

		var subNums []int
		if err := json.Unmarshal([]byte(subNumsJSON), &subNums); err == nil && len(subNums) > 0 {
			issue.SubIssueNumbers = subNums
		}

		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get issues: rows error: %w", err)
	}

	if issues == nil {
		issues = []domain.Issue{}
	}
	return issues, nil
}

// SyncPRs fetches open PRs from GitHub via the client and upserts them into the cache.
// On CLI error, the existing cached data is preserved and the error is returned.
func (r *GitHubRepository) SyncPRs(client *exec.PRCommand) error {
	prs, err := client.ListOpenPRs()
	if err != nil {
		return fmt.Errorf("sync prs: %w", err)
	}
	return r.UpsertPRs(prs)
}

// SyncIssues fetches open issues from GitHub via the client and upserts them into the cache.
// On CLI error, the existing cached data is preserved and the error is returned.
func (r *GitHubRepository) SyncIssues(client *exec.IssueCommand) error {
	issues, err := client.ListOpenIssues()
	if err != nil {
		return fmt.Errorf("sync issues: %w", err)
	}
	return r.UpsertIssues(issues)
}
