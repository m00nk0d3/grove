package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// GitHubWriter creates issues and places them on project boards through the
// gh CLI. It is Grove's only path for writing to GitHub; see ADR-0001 and
// docs/LAB_DESIGN.md.
type GitHubWriter struct {
	repoPath string
	runner   commandRunner
}

// NewGitHubWriter creates a GitHubWriter using the real gh CLI.
func NewGitHubWriter(repoPath string) *GitHubWriter {
	return NewGitHubWriterWithRunner(repoPath, runGhCommand)
}

// NewGitHubWriterWithRunner creates a GitHubWriter with an injected runner for
// testing.
func NewGitHubWriterWithRunner(repoPath string, runner commandRunner) *GitHubWriter {
	return &GitHubWriter{repoPath: repoPath, runner: runner}
}

// Repo returns the repository gh resolves for the working directory, as
// "owner/name".
func (w *GitHubWriter) Repo() (string, error) {
	out, err := w.runner(w.repoPath, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", fmt.Errorf("resolve the GitHub repository: %w", err)
	}
	repo := strings.TrimSpace(out)
	if !strings.Contains(repo, "/") {
		return "", fmt.Errorf("resolve the GitHub repository: gh returned %q", repo)
	}
	return repo, nil
}

var issueURLPattern = regexp.MustCompile(`https://\S+/issues/(\d+)`)

// CreateIssue creates an issue in repo and returns its number and URL.
func (w *GitHubWriter) CreateIssue(repo, title, body string, labels []string) (int, string, error) {
	if strings.TrimSpace(title) == "" {
		return 0, "", errors.New("create issue: the title is empty")
	}
	bodyFile, err := os.CreateTemp("", "grove-issue-*.md")
	if err != nil {
		return 0, "", fmt.Errorf("create issue: %w", err)
	}
	defer os.Remove(bodyFile.Name())
	_, writeErr := bodyFile.WriteString(body)
	closeErr := bodyFile.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return 0, "", fmt.Errorf("create issue: write the body: %w", err)
	}

	args := []string{"issue", "create", "--repo", repo, "--title", title, "--body-file", bodyFile.Name()}
	for _, label := range labels {
		args = append(args, "--label", label)
	}
	out, err := w.runner(w.repoPath, args...)
	if err != nil {
		return 0, "", fmt.Errorf("create issue: %w", err)
	}
	match := issueURLPattern.FindStringSubmatch(out)
	if match == nil {
		return 0, "", fmt.Errorf("create issue: gh did not report the new issue's URL: %s", strings.TrimSpace(out))
	}
	number, _ := strconv.Atoi(match[1])
	return number, match[0], nil
}

// LabProject is a GitHub project board.
type LabProject struct {
	ID     string
	Owner  string
	Number int
	Title  string
}

// Ref returns the board as "owner/number", the form of the lab.project
// setting.
func (p LabProject) Ref() string { return fmt.Sprintf("%s/%d", p.Owner, p.Number) }

// ParseProjectRef splits an "owner/number" board reference.
func ParseProjectRef(ref string) (owner string, number int, err error) {
	owner, num, ok := strings.Cut(strings.TrimSpace(ref), "/")
	n, convErr := strconv.Atoi(num)
	if !ok || owner == "" || convErr != nil || n <= 0 {
		return "", 0, fmt.Errorf("project board %q is not in owner/number form", ref)
	}
	return owner, n, nil
}

const linkedProjectsQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    projectsV2(first: 20) {
      nodes {
        id
        number
        title
        owner { ... on User { login } ... on Organization { login } }
      }
    }
  }
}`

// LinkedProjects returns the project boards linked to repo.
func (w *GitHubWriter) LinkedProjects(repo string) ([]LabProject, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("list project boards: %q is not owner/name", repo)
	}
	out, err := w.runner(w.repoPath, "api", "graphql", "-f", "query="+linkedProjectsQuery, "-F", "owner="+owner, "-F", "name="+name)
	if err != nil {
		return nil, fmt.Errorf("list project boards: %w", err)
	}
	var resp struct {
		Data struct {
			Repository struct {
				ProjectsV2 struct {
					Nodes []struct {
						ID     string `json:"id"`
						Number int    `json:"number"`
						Title  string `json:"title"`
						Owner  struct {
							Login string `json:"login"`
						} `json:"owner"`
					} `json:"nodes"`
				} `json:"projectsV2"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("list project boards: %w", err)
	}
	var projects []LabProject
	for _, n := range resp.Data.Repository.ProjectsV2.Nodes {
		projects = append(projects, LabProject{ID: n.ID, Owner: n.Owner.Login, Number: n.Number, Title: n.Title})
	}
	return projects, nil
}

// Project looks up a board by owner and number.
func (w *GitHubWriter) Project(owner string, number int) (LabProject, error) {
	out, err := w.runner(w.repoPath, "project", "view", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return LabProject{}, fmt.Errorf("find project board %s/%d: %w", owner, number, err)
	}
	var resp struct {
		ID     string `json:"id"`
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return LabProject{}, fmt.Errorf("find project board %s/%d: %w", owner, number, err)
	}
	return LabProject{ID: resp.ID, Owner: owner, Number: number, Title: resp.Title}, nil
}

// PlaceOnBoard adds an issue to a board and sets its Status field. Adding an
// issue that is already on the board returns its existing item, so placing it
// again is safe.
func (w *GitHubWriter) PlaceOnBoard(p LabProject, issueURL, status string) error {
	num := strconv.Itoa(p.Number)
	out, err := w.runner(w.repoPath, "project", "item-add", num, "--owner", p.Owner, "--url", issueURL, "--format", "json")
	if err != nil {
		return fmt.Errorf("add the issue to %s: %w", p.Ref(), err)
	}
	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &item); err != nil || item.ID == "" {
		return fmt.Errorf("add the issue to %s: gh returned no item", p.Ref())
	}

	out, err = w.runner(w.repoPath, "project", "field-list", num, "--owner", p.Owner, "--format", "json")
	if err != nil {
		return fmt.Errorf("read the fields of %s: %w", p.Ref(), err)
	}
	var fields struct {
		Fields []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Options []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"options"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		return fmt.Errorf("read the fields of %s: %w", p.Ref(), err)
	}
	var fieldID, optionID string
	for _, f := range fields.Fields {
		if !strings.EqualFold(f.Name, "Status") {
			continue
		}
		fieldID = f.ID
		for _, o := range f.Options {
			if strings.EqualFold(o.Name, status) {
				optionID = o.ID
			}
		}
	}
	if fieldID == "" {
		return fmt.Errorf("%s has no Status field", p.Ref())
	}
	if optionID == "" {
		return fmt.Errorf("%s has no %q status", p.Ref(), status)
	}

	if _, err := w.runner(w.repoPath, "project", "item-edit", "--id", item.ID, "--project-id", p.ID,
		"--field-id", fieldID, "--single-select-option-id", optionID); err != nil {
		return fmt.Errorf("set the issue's status on %s: %w", p.Ref(), err)
	}
	return nil
}

// labLabelColors are the colors Grove gives the labels it creates. A label
// that already exists is left as it is.
var labLabelColors = map[string]string{
	"bug":             "d73a4a",
	"epic":            "5319e7",
	"ready-for-agent": "0e8a16",
}

// EnsureLabels creates those of labels that repo does not have yet. Existing
// labels, and their colors and descriptions, are never changed.
func (w *GitHubWriter) EnsureLabels(repo string, labels []string) error {
	out, err := w.runner(w.repoPath, "label", "list", "--repo", repo, "--json", "name", "--limit", "1000")
	if err != nil {
		return fmt.Errorf("list labels: %w", err)
	}
	var existing []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &existing); err != nil {
		return fmt.Errorf("list labels: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, l := range existing {
		have[strings.ToLower(l.Name)] = true
	}
	for _, label := range labels {
		if have[strings.ToLower(label)] {
			continue
		}
		args := []string{"label", "create", label, "--repo", repo}
		if color, ok := labLabelColors[label]; ok {
			args = append(args, "--color", color)
		}
		if _, err := w.runner(w.repoPath, args...); err != nil {
			return fmt.Errorf("create label %s: %w", label, err)
		}
	}
	return nil
}

// issueDatabaseID returns the numeric ID the REST API uses for an issue, which
// is not its number.
func (w *GitHubWriter) issueDatabaseID(repo string, number int) (int64, error) {
	out, err := w.runner(w.repoPath, "api", fmt.Sprintf("repos/%s/issues/%d", repo, number), "--jq", ".id")
	if err != nil {
		return 0, fmt.Errorf("look up issue #%d: %w", number, err)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("look up issue #%d: gh returned %q", number, strings.TrimSpace(out))
	}
	return id, nil
}

// AddSubIssue makes issue child a sub-issue of parent, through GitHub's native
// sub-issue relationship.
func (w *GitHubWriter) AddSubIssue(repo string, parent, child int) error {
	id, err := w.issueDatabaseID(repo, child)
	if err != nil {
		return err
	}
	if _, err := w.runner(w.repoPath, "api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/sub_issues", repo, parent),
		"-F", fmt.Sprintf("sub_issue_id=%d", id)); err != nil {
		return fmt.Errorf("make #%d a sub-issue of #%d: %w", child, parent, err)
	}
	return nil
}

// AddBlockedBy records that issue is blocked by blocker, through GitHub's
// native issue dependencies.
func (w *GitHubWriter) AddBlockedBy(repo string, issue, blocker int) error {
	id, err := w.issueDatabaseID(repo, blocker)
	if err != nil {
		return err
	}
	if _, err := w.runner(w.repoPath, "api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/dependencies/blocked_by", repo, issue),
		"-F", fmt.Sprintf("issue_id=%d", id)); err != nil {
		return fmt.Errorf("mark #%d blocked by #%d: %w", issue, blocker, err)
	}
	return nil
}
