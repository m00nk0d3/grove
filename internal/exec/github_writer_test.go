package exec

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGh answers gh commands by their leading words and records every call.
// Nothing reaches GitHub.
type fakeGh struct {
	replies map[string]string
	errs    map[string]error
	calls   [][]string
	bodies  []string
}

func newFakeGh() *fakeGh {
	return &fakeGh{replies: map[string]string{}, errs: map[string]error{}}
}

func (f *fakeGh) run(_ string, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	for i, arg := range args {
		if arg == "--body-file" && i+1 < len(args) {
			body, err := os.ReadFile(args[i+1])
			if err != nil {
				return "", err
			}
			f.bodies = append(f.bodies, string(body))
		}
	}
	key := strings.Join(args[:min(2, len(args))], " ")
	if err, ok := f.errs[key]; ok {
		return "", err
	}
	return f.replies[key], nil
}

func (f *fakeGh) writer() *GitHubWriter { return NewGitHubWriterWithRunner("/repo", f.run) }

func TestGitHubWriter_Repo(t *testing.T) {
	gh := newFakeGh()
	gh.replies["repo view"] = "m00nk0d3/grove\n"
	repo, err := gh.writer().Repo()
	require.NoError(t, err)
	assert.Equal(t, "m00nk0d3/grove", repo)
}

func TestGitHubWriter_CreateIssue(t *testing.T) {
	gh := newFakeGh()
	gh.replies["issue create"] = "Creating issue in m00nk0d3/grove\n\nhttps://github.com/m00nk0d3/grove/issues/251\n"

	number, url, err := gh.writer().CreateIssue("m00nk0d3/grove", "Sync stalls", "## Summary\nIt stalls.", []string{"bug"})
	require.NoError(t, err)
	assert.Equal(t, 251, number)
	assert.Equal(t, "https://github.com/m00nk0d3/grove/issues/251", url)

	args := strings.Join(gh.calls[0], " ")
	assert.Contains(t, args, "issue create --repo m00nk0d3/grove --title Sync stalls --body-file ")
	assert.Contains(t, args, "--label bug")
	assert.Equal(t, []string{"## Summary\nIt stalls."}, gh.bodies, "the body is passed through a file, never the command line")
}

func TestGitHubWriter_CreateIssueErrors(t *testing.T) {
	gh := newFakeGh()
	_, _, err := gh.writer().CreateIssue("o/r", "  ", "body", nil)
	assert.ErrorContains(t, err, "title is empty")
	assert.Empty(t, gh.calls, "nothing is sent without a title")

	gh.replies["issue create"] = "something unexpected"
	_, _, err = gh.writer().CreateIssue("o/r", "Title", "body", nil)
	assert.ErrorContains(t, err, "did not report the new issue's URL")

	gh.errs["issue create"] = errors.New("label not found")
	_, _, err = gh.writer().CreateIssue("o/r", "Title", "body", []string{"bug"})
	assert.ErrorContains(t, err, "label not found")
}

func TestGitHubWriter_LinkedProjects(t *testing.T) {
	gh := newFakeGh()
	gh.replies["api graphql"] = `{"data":{"repository":{"projectsV2":{"nodes":[
		{"id":"PVT_1","number":3,"title":"Roadmap","owner":{"login":"m00nk0d3"}},
		{"id":"PVT_2","number":7,"title":"Bugs","owner":{"login":"acme"}}]}}}}`

	projects, err := gh.writer().LinkedProjects("m00nk0d3/grove")
	require.NoError(t, err)
	assert.Equal(t, []LabProject{
		{ID: "PVT_1", Owner: "m00nk0d3", Number: 3, Title: "Roadmap"},
		{ID: "PVT_2", Owner: "acme", Number: 7, Title: "Bugs"},
	}, projects)
	assert.Contains(t, gh.calls[0], "owner=m00nk0d3")
	assert.Contains(t, gh.calls[0], "name=grove")
}

func TestGitHubWriter_Project(t *testing.T) {
	gh := newFakeGh()
	gh.replies["project view"] = `{"id":"PVT_1","number":3,"title":"Roadmap"}`
	p, err := gh.writer().Project("m00nk0d3", 3)
	require.NoError(t, err)
	assert.Equal(t, LabProject{ID: "PVT_1", Owner: "m00nk0d3", Number: 3, Title: "Roadmap"}, p)
	assert.Equal(t, "m00nk0d3/3", p.Ref())
}

func TestGitHubWriter_PlaceOnBoardSetsStatus(t *testing.T) {
	gh := newFakeGh()
	gh.replies["project item-add"] = `{"id":"PVTI_9"}`
	gh.replies["project field-list"] = `{"fields":[
		{"id":"PVTF_title","name":"Title"},
		{"id":"PVTSSF_status","name":"Status","options":[{"id":"opt_todo","name":"Todo"},{"id":"opt_backlog","name":"Backlog"}]}]}`
	p := LabProject{ID: "PVT_1", Owner: "m00nk0d3", Number: 3}

	require.NoError(t, gh.writer().PlaceOnBoard(p, "https://github.com/m00nk0d3/grove/issues/251", "Backlog"))
	require.Len(t, gh.calls, 3)
	assert.Equal(t, []string{"project", "item-add", "3", "--owner", "m00nk0d3", "--url", "https://github.com/m00nk0d3/grove/issues/251", "--format", "json"}, gh.calls[0])
	assert.Equal(t, []string{"project", "item-edit", "--id", "PVTI_9", "--project-id", "PVT_1",
		"--field-id", "PVTSSF_status", "--single-select-option-id", "opt_backlog"}, gh.calls[2])
}

func TestGitHubWriter_PlaceOnBoardWithoutBacklog(t *testing.T) {
	gh := newFakeGh()
	gh.replies["project item-add"] = `{"id":"PVTI_9"}`
	gh.replies["project field-list"] = `{"fields":[{"id":"S","name":"Status","options":[{"id":"o","name":"Todo"}]}]}`
	err := gh.writer().PlaceOnBoard(LabProject{Owner: "o", Number: 1}, "u", "Backlog")
	assert.ErrorContains(t, err, `has no "Backlog" status`)
	assert.Len(t, gh.calls, 2, "no status is set when the option is missing")
}

func TestParseProjectRef(t *testing.T) {
	owner, number, err := ParseProjectRef(" m00nk0d3/3 ")
	require.NoError(t, err)
	assert.Equal(t, "m00nk0d3", owner)
	assert.Equal(t, 3, number)
	for _, bad := range []string{"", "m00nk0d3", "m00nk0d3/x", "/3", "o/0"} {
		_, _, err := ParseProjectRef(bad)
		assert.Error(t, err, bad)
	}
}
