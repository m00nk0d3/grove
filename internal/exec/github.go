package exec

import (
	"encoding/json"
	"fmt"
	osexec "os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
)

// parentRefRe matches common "this issue is a sub-issue of #N" patterns in issue bodies.
// Examples: "Part of #61", "Tracked by #61", "Parent: #61", "Sub-issue of #61".
var parentRefRe = regexp.MustCompile(`(?im)(?:part\s+of|tracked?\s+by|parent:?|sub.?issue\s+of)\s+#(\d+)`)

// EnrichHierarchyFromBodies scans each issue's body text and sets ParentNumber /
// SubIssueNumbers when body-based parent-reference patterns are found. It only
// writes fields that are not already set (so GraphQL data wins over body parsing).
func EnrichHierarchyFromBodies(issues []domain.Issue) {
	childToParent := make(map[int]int)

	for _, iss := range issues {
		if iss.ParentNumber != nil {
			continue // already set by GraphQL enrichment
		}
		m := parentRefRe.FindStringSubmatch(iss.Body)
		if m == nil {
			continue
		}
		parentNum, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		childToParent[iss.Number] = parentNum
	}

	// Build a number→slice-index map for quick lookups.
	byNum := make(map[int]int, len(issues))
	for i, iss := range issues {
		byNum[iss.Number] = i
	}

	for i := range issues {
		n := issues[i].Number
		if p, ok := childToParent[n]; ok {
			pCopy := p
			issues[i].ParentNumber = &pCopy
		}
	}

	// Derive SubIssueNumbers for parent issues from the reverse map.
	// Only add children that actually exist in the slice.
	parentChildren := make(map[int][]int)
	for child, parent := range childToParent {
		if _, ok := byNum[child]; ok {
			parentChildren[parent] = append(parentChildren[parent], child)
		}
	}
	for i := range issues {
		n := issues[i].Number
		if len(issues[i].SubIssueNumbers) > 0 {
			continue // already set
		}
		if children, ok := parentChildren[n]; ok && len(children) > 0 {
			issues[i].SubIssueNumbers = children
		}
	}
}

// IssueCommand wraps the gh CLI for GitHub issue operations.
type IssueCommand struct {
	repoPath string
	runner   commandRunner
}

// NewIssueCommand creates a new IssueCommand using the real gh CLI.
func NewIssueCommand(repoPath string) *IssueCommand {
	return NewIssueCommandWithRunner(repoPath, runGhCommand)
}

// NewIssueCommandWithRunner creates an IssueCommand with an injected runner for testing.
func NewIssueCommandWithRunner(repoPath string, runner commandRunner) *IssueCommand {
	return &IssueCommand{repoPath: repoPath, runner: runner}
}

// ListOpenIssues returns all open GitHub issues via `gh issue list`.
func (c *IssueCommand) ListOpenIssues() ([]domain.Issue, error) {
	output, err := c.runner(c.repoPath, "issue", "list", "--json", "number,title,body,labels,assignees,state", "--state", "open", "--limit", "100")
	if err != nil {
		return nil, fmt.Errorf("list open issues: %w", err)
	}

	issues, err := parseIssueList(output)
	if err != nil {
		return nil, err
	}

	return issues, nil
}

// ghLabel is the JSON shape for a label returned by gh.
type ghLabel struct {
	Name string `json:"name"`
}

// ghAssignee is the JSON shape for an assignee returned by gh.
type ghAssignee struct {
	Login string `json:"login"`
}

// ghIssue is the JSON shape returned by `gh issue list --json number,title,body,labels,assignees,state`.
type ghIssue struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	State     string       `json:"state"`
	Labels    []ghLabel    `json:"labels"`
	Assignees []ghAssignee `json:"assignees"`
}

func parseIssueList(raw string) ([]domain.Issue, error) {
	var gh []ghIssue
	if err := json.Unmarshal([]byte(raw), &gh); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}

	issues := make([]domain.Issue, 0, len(gh))
	for _, g := range gh {
		labels := make([]string, len(g.Labels))
		for i, l := range g.Labels {
			labels[i] = l.Name
		}
		var assignees []string
		if len(g.Assignees) > 0 {
			assignees = make([]string, len(g.Assignees))
			for i, a := range g.Assignees {
				assignees[i] = a.Login
			}
		}
		issues = append(issues, domain.Issue{
			Number:    g.Number,
			Title:     g.Title,
			Body:      g.Body,
			State:     g.State,
			Labels:    labels,
			Assignees: assignees,
		})
	}

	return issues, nil
}

// GetRepoOwnerAndName returns the GitHub repository owner login and repo name via gh CLI.
func (c *IssueCommand) GetRepoOwnerAndName() (string, string, error) {
	output, err := c.runner(c.repoPath, "repo", "view", "--json", "owner,name")
	if err != nil {
		return "", "", fmt.Errorf("get repo owner and name: %w", err)
	}
	var result struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return "", "", fmt.Errorf("parse repo owner and name: %w", err)
	}
	return result.Owner.Login, result.Name, nil
}

// projectStatusRank orders Projects v2 status values by how much active work they
// imply. An issue can appear on several boards whose vocabularies differ, so the
// rank decides which board's value is reported. Unrecognised values (including
// terminal ones such as "Done") rank lowest and are only used when nothing else
// is available.
func projectStatusRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in progress", "in-progress", "doing", "started", "in development":
		return 5
	case "in review", "review", "in qa", "qa":
		return 4
	case "blocked", "on hold":
		return 3
	case "ready", "todo", "to do", "next":
		return 2
	case "backlog", "triage", "icebox":
		return 1
	default:
		return 0
	}
}

// FetchIssueProjectStatus fetches the GitHub Projects v2 "Status" field for open
// issues in a single bulk GraphQL call. Values are returned with the board's own
// wording (for example "In progress" or "Backlog") rather than normalised, so the
// UI shows what the board actually says.
//
// Returns nil, nil on any API error — project status is optional enrichment and
// reading it requires the read:project token scope, which may not be granted.
func (c *IssueCommand) FetchIssueProjectStatus(owner, repo string) (map[int]string, error) {
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			issues(states: OPEN, first: 100, orderBy: {field: UPDATED_AT, direction: DESC}) {
				nodes {
					number
					projectItems(first: 5) {
						nodes {
							fieldValueByName(name: "Status") {
								... on ProjectV2ItemFieldSingleSelectValue { name }
							}
						}
					}
				}
			}
		}
	}`
	output, err := c.runner(c.repoPath, "api", "graphql",
		"-F", "owner="+owner,
		"-F", "repo="+repo,
		"-f", "query="+query,
	)
	if err != nil {
		return nil, nil // graceful fallback
	}
	var resp struct {
		Data struct {
			Repository struct {
				Issues struct {
					Nodes []struct {
						Number       int `json:"number"`
						ProjectItems struct {
							Nodes []struct {
								FieldValueByName struct {
									Name string `json:"name"`
								} `json:"fieldValueByName"`
							} `json:"nodes"`
						} `json:"projectItems"`
					} `json:"nodes"`
				} `json:"issues"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, nil // graceful fallback
	}
	result := make(map[int]string)
	for _, issue := range resp.Data.Repository.Issues.Nodes {
		best := ""
		for _, item := range issue.ProjectItems.Nodes {
			name := item.FieldValueByName.Name
			if name == "" {
				continue
			}
			if best == "" || projectStatusRank(name) > projectStatusRank(best) {
				best = name
			}
		}
		if best != "" {
			result[issue.Number] = best
		}
	}
	return result, nil
}

// FetchIssueHierarchy fetches sub-issue relationships for open issues in a single
// bulk GraphQL call. Returns map[parentNumber][]childNumbers.
// Returns nil, nil on any API error (graceful fallback — hierarchy data is optional).
func (c *IssueCommand) FetchIssueHierarchy(_ []int, owner, repo string) (map[int][]int, error) {
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			issues(states: OPEN, first: 100) {
				nodes {
					number
					subIssues(first: 30) {
						nodes { number }
					}
				}
			}
		}
	}`
	output, err := c.runner(c.repoPath, "api", "graphql",
		"-H", "GraphQL-Features: sub_issues",
		"-F", "owner="+owner,
		"-F", "repo="+repo,
		"-f", "query="+query,
	)
	if err != nil {
		return nil, nil // graceful fallback
	}
	var resp struct {
		Data struct {
			Repository struct {
				Issues struct {
					Nodes []struct {
						Number    int `json:"number"`
						SubIssues struct {
							Nodes []struct {
								Number int `json:"number"`
							} `json:"nodes"`
						} `json:"subIssues"`
					} `json:"nodes"`
				} `json:"issues"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return nil, nil // graceful fallback
	}
	result := make(map[int][]int)
	for _, issue := range resp.Data.Repository.Issues.Nodes {
		if len(issue.SubIssues.Nodes) > 0 {
			children := make([]int, 0, len(issue.SubIssues.Nodes))
			for _, node := range issue.SubIssues.Nodes {
				children = append(children, node.Number)
			}
			result[issue.Number] = children
		}
	}
	return result, nil
}

// PRCommand wraps the gh CLI for GitHub pull request operations.
type PRCommand struct {
	repoPath string
	runner   commandRunner
}

// NewPRCommand creates a new PRCommand using the real gh CLI.
func NewPRCommand(repoPath string) *PRCommand {
	return NewPRCommandWithRunner(repoPath, runGhCommand)
}

// NewPRCommandWithRunner creates a PRCommand with an injected runner for testing.
func NewPRCommandWithRunner(repoPath string, runner commandRunner) *PRCommand {
	return &PRCommand{repoPath: repoPath, runner: runner}
}

const prFields = "number,title,body,headRefName,author,state,labels,isDraft,assignees,reviewDecision,statusCheckRollup,mergeable"

// ListOpenPRs returns all open pull requests via `gh pr list`.
func (c *PRCommand) ListOpenPRs() ([]domain.PullRequest, error) {
	output, err := c.runner(c.repoPath, "pr", "list", "--json", prFields, "--state", "open", "--limit", "100")
	if err != nil {
		return nil, fmt.Errorf("list open prs: %w", err)
	}

	prs, err := parsePRList(output)
	if err != nil {
		return nil, err
	}

	return prs, nil
}

// EnrichViewerAttention annotates PRs with viewer ownership, review requests,
// and unresolved review-thread counts.
func (c *PRCommand) EnrichViewerAttention(prs []domain.PullRequest) error {
	if len(prs) == 0 {
		return nil
	}

	repoOutput, err := c.runner(c.repoPath, "repo", "view", "--json", "owner,name")
	if err != nil {
		return fmt.Errorf("get repository identity for PR attention: %w", err)
	}
	var repository struct {
		Owner ghAuthor `json:"owner"`
		Name  string   `json:"name"`
	}
	if err := json.Unmarshal([]byte(repoOutput), &repository); err != nil {
		return fmt.Errorf("parse repository identity for PR attention: %w", err)
	}

	query := `query($owner: String!, $name: String!) {
		viewer { login }
		repository(owner: $owner, name: $name) {
			pullRequests(states: OPEN, first: 100) {
				nodes {
					number
					comments(last: 3) {
						nodes { author { login } body createdAt url }
					}
					latestReviews(first: 10) {
						nodes { author { login } body state submittedAt url }
					}
					reviewThreads(first: 100) {
						nodes { isResolved }
					}
				}
			}
		}
	}`
	output, err := c.runner(
		c.repoPath,
		"api", "graphql",
		"-F", "owner="+repository.Owner.Login,
		"-F", "name="+repository.Name,
		"-f", "query="+query,
	)
	if err != nil {
		return fmt.Errorf("fetch PR attention metadata: %w", err)
	}
	var response struct {
		Data struct {
			Viewer     ghAuthor `json:"viewer"`
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						Number   int `json:"number"`
						Comments struct {
							Nodes []ghPRActivity `json:"nodes"`
						} `json:"comments"`
						LatestReviews struct {
							Nodes []ghPRActivity `json:"nodes"`
						} `json:"latestReviews"`
						ReviewThreads struct {
							Nodes []struct {
								IsResolved bool `json:"isResolved"`
							} `json:"nodes"`
						} `json:"reviewThreads"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		return fmt.Errorf("parse PR attention metadata: %w", err)
	}
	if len(response.Errors) > 0 {
		return fmt.Errorf("fetch PR attention metadata: %s", response.Errors[0].Message)
	}

	unresolvedByNumber := make(map[int]int, len(response.Data.Repository.PullRequests.Nodes))
	metadataByNumber := make(map[int]struct {
		comments []domain.PullRequestActivity
		reviews  []domain.PullRequestActivity
	}, len(response.Data.Repository.PullRequests.Nodes))
	for _, node := range response.Data.Repository.PullRequests.Nodes {
		for _, thread := range node.ReviewThreads.Nodes {
			if !thread.IsResolved {
				unresolvedByNumber[node.Number]++
			}
		}
		comments := make([]domain.PullRequestActivity, 0, len(node.Comments.Nodes))
		for _, comment := range node.Comments.Nodes {
			comments = append(comments, ghActivityToDomain(comment, comment.CreatedAt))
		}
		reviews := make([]domain.PullRequestActivity, 0, len(node.LatestReviews.Nodes))
		for _, review := range node.LatestReviews.Nodes {
			reviews = append(reviews, ghActivityToDomain(review, review.SubmittedAt))
		}
		metadataByNumber[node.Number] = struct {
			comments []domain.PullRequestActivity
			reviews  []domain.PullRequestActivity
		}{comments: comments, reviews: reviews}
	}
	for i := range prs {
		prs[i].IsMine = strings.EqualFold(prs[i].Author, response.Data.Viewer.Login)
		prs[i].UnresolvedThreads = unresolvedByNumber[prs[i].Number]
		if metadata, ok := metadataByNumber[prs[i].Number]; ok {
			prs[i].Comments = metadata.comments
			prs[i].Reviews = metadata.reviews
		}
	}

	requestedOutput, err := c.runner(
		c.repoPath,
		"pr", "list",
		"--json", "number",
		"--state", "open",
		"--search", "review-requested:@me",
		"--limit", "100",
	)
	if err != nil {
		return fmt.Errorf("list viewer review requests: %w", err)
	}
	var requested []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(requestedOutput), &requested); err != nil {
		return fmt.Errorf("parse viewer review requests: %w", err)
	}
	requestedByNumber := make(map[int]bool, len(requested))
	for _, pr := range requested {
		requestedByNumber[pr.Number] = true
	}

	for i := range prs {
		prs[i].ReviewRequested = requestedByNumber[prs[i].Number]
	}
	return nil
}

// GetPR returns a single pull request by number via `gh pr view`.
func (c *PRCommand) GetPR(number int) (*domain.PullRequest, error) {
	output, err := c.runner(c.repoPath, "pr", "view", fmt.Sprintf("%d", number), "--json", prFields)
	if err != nil {
		return nil, fmt.Errorf("get pr: %w", err)
	}

	pr, err := parsePR(output)
	if err != nil {
		return nil, err
	}

	return pr, nil
}

// ghAuthor is the JSON shape for the author object returned by gh.
type ghAuthor struct {
	Login string `json:"login"`
}

type ghPRActivity struct {
	Author      ghAuthor `json:"author"`
	Body        string   `json:"body"`
	State       string   `json:"state"`
	URL         string   `json:"url"`
	CreatedAt   string   `json:"createdAt"`
	SubmittedAt string   `json:"submittedAt"`
}

type ghStatusCheck struct {
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// ghPR is the JSON shape returned by `gh pr list/view --json ...`.
type ghPR struct {
	Number            int             `json:"number"`
	Title             string          `json:"title"`
	Body              string          `json:"body"`
	HeadRefName       string          `json:"headRefName"`
	Author            ghAuthor        `json:"author"`
	State             string          `json:"state"`
	ReviewDecision    string          `json:"reviewDecision"`
	Labels            []ghLabel       `json:"labels"`
	IsDraft           bool            `json:"isDraft"`
	Assignees         []ghAuthor      `json:"assignees"`
	Comments          []ghPRActivity  `json:"comments"`
	LatestReviews     []ghPRActivity  `json:"latestReviews"`
	StatusCheckRollup []ghStatusCheck `json:"statusCheckRollup"`
	Mergeable         string          `json:"mergeable"`
}

func ghPRToDomain(g ghPR) domain.PullRequest {
	labels := make([]string, len(g.Labels))
	for i, l := range g.Labels {
		labels[i] = l.Name
	}
	var assignees []string
	if len(g.Assignees) > 0 {
		assignees = make([]string, len(g.Assignees))
		for i, a := range g.Assignees {
			assignees[i] = a.Login
		}
	}
	var comments []domain.PullRequestActivity
	if len(g.Comments) > 0 {
		comments = make([]domain.PullRequestActivity, 0, len(g.Comments))
		for _, comment := range g.Comments {
			comments = append(comments, ghActivityToDomain(comment, comment.CreatedAt))
		}
	}
	var reviews []domain.PullRequestActivity
	if len(g.LatestReviews) > 0 {
		reviews = make([]domain.PullRequestActivity, 0, len(g.LatestReviews))
		for _, review := range g.LatestReviews {
			reviews = append(reviews, ghActivityToDomain(review, review.SubmittedAt))
		}
	}
	checksFailing := false
	for _, check := range g.StatusCheckRollup {
		switch strings.ToUpper(firstNonEmpty(check.Conclusion, check.State)) {
		case "ACTION_REQUIRED", "CANCELLED", "ERROR", "FAILURE", "STARTUP_FAILURE", "TIMED_OUT":
			checksFailing = true
		}
	}
	return domain.PullRequest{
		Number:         g.Number,
		Title:          g.Title,
		Body:           g.Body,
		Branch:         g.HeadRefName,
		Author:         g.Author.Login,
		State:          g.State,
		ReviewDecision: g.ReviewDecision,
		Labels:         labels,
		IsDraft:        g.IsDraft,
		Assignees:      assignees,
		Comments:       comments,
		Reviews:        reviews,
		ChecksFailing:  checksFailing,
		MergeConflict:  strings.EqualFold(g.Mergeable, "CONFLICTING"),
	}
}

func ghActivityToDomain(activity ghPRActivity, timestamp string) domain.PullRequestActivity {
	createdAt, _ := time.Parse(time.RFC3339, timestamp)
	return domain.PullRequestActivity{
		Author:    activity.Author.Login,
		Body:      activity.Body,
		State:     activity.State,
		URL:       activity.URL,
		CreatedAt: createdAt,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parsePRList(raw string) ([]domain.PullRequest, error) {
	var gh []ghPR
	if err := json.Unmarshal([]byte(raw), &gh); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}

	prs := make([]domain.PullRequest, 0, len(gh))
	for _, g := range gh {
		prs = append(prs, ghPRToDomain(g))
	}

	return prs, nil
}

func parsePR(raw string) (*domain.PullRequest, error) {
	var g ghPR
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return nil, fmt.Errorf("parse pr: %w", err)
	}

	pr := ghPRToDomain(g)
	return &pr, nil
}

func runGhCommand(repoPath string, args ...string) (string, error) {
	cmd := osexec.Command("gh", args...)
	cmd.Dir = repoPath

	out, err := cmd.CombinedOutput()
	output := string(out)
	if err != nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			return "", fmt.Errorf("run gh %s: %w; output: %s", strings.Join(args, " "), err, trimmed)
		}
		return "", fmt.Errorf("run gh %s: %w", strings.Join(args, " "), err)
	}

	return output, nil
}
