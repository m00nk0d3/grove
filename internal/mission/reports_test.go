package mission

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(n int) *int { return &n }

func writeReport(t *testing.T, dir, name, body string, modTime time.Time) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}

func TestFindReports_ImplementationWorkflow(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	writeReport(t, dir, "issue-42-implementation-report.md", "# Implementation Report", now.Add(-time.Hour))
	writeReport(t, dir, "issue-42-review-verdict.md", "# Review Cycle 1: APPROVED", now)
	// Another issue's report and the checkpoint must not be picked up.
	writeReport(t, dir, "issue-420-implementation-report.md", "other", now)
	writeReport(t, dir, "issue-42.json", "{}", now)

	reports, err := FindReports(dir, domain.WorkflowRunRef{Kind: "imp", IssueNumber: intPtr(42)})

	require.NoError(t, err)
	require.Len(t, reports, 2)
	assert.Equal(t, "Implementation report", reports[0].Title, "reports follow the order the workflow writes them")
	assert.Equal(t, "# Implementation Report", reports[0].Body)
	assert.Equal(t, filepath.Join(dir, "issue-42-implementation-report.md"), reports[0].Path)
	assert.True(t, reports[0].ModTime.Equal(now.Add(-time.Hour)))
	assert.Equal(t, "Review verdict", reports[1].Title)
}

func TestFindReports_LeavesOutReportsNotWrittenYet(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "issue-7-implementation-report.md", "draft", time.Now())

	reports, err := FindReports(dir, domain.WorkflowRunRef{Kind: "imp", IssueNumber: intPtr(7)})

	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "Implementation report", reports[0].Title)
}

func TestFindReports_ReviewWorkflowListsEveryReviewedHeadNewestFirst(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	writeReport(t, dir, "pr-9-aaaaaaaa-review.md", "first head", now.Add(-2*time.Hour))
	writeReport(t, dir, "pr-9-bbbbbbbb-review.md", "second head", now)
	writeReport(t, dir, "pr-9-bbbbbbbb-verdict.json", "{}", now)
	writeReport(t, dir, "pr-90-cccccccc-review.md", "other pull request", now)

	reports, err := FindReports(dir, domain.WorkflowRunRef{Kind: "review", PRNumber: intPtr(9)})

	require.NoError(t, err)
	require.Len(t, reports, 2)
	assert.Equal(t, "second head", reports[0].Body)
	assert.Equal(t, "Pull request review 2/2", reports[0].Title)
	assert.Equal(t, "first head", reports[1].Body)
	assert.Equal(t, "Pull request review 1/2", reports[1].Title)
}

func TestFindReports_CIWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, filepath.Join("ci-pr-5", "failures.md"), "# Failed CI checks", time.Now())

	reports, err := FindReports(dir, domain.WorkflowRunRef{Kind: "ci", PRNumber: intPtr(5)})

	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, "CI failures", reports[0].Title)
}

func TestFindReports_KindsWithoutReports(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "issue-3-implementation-report.md", "x", time.Now())

	for _, workflow := range []domain.WorkflowRunRef{
		{Kind: "address", PRNumber: intPtr(3)},
		{Kind: "resolve", PRNumber: intPtr(3)},
		{Kind: "clean"},
		{Kind: "imp"}, // no issue number to key the report on
	} {
		reports, err := FindReports(dir, workflow)
		require.NoError(t, err)
		assert.Empty(t, reports, "kind %q", workflow.Kind)
		assert.False(t, WritesReports(workflow), "kind %q", workflow.Kind)
	}
	assert.True(t, WritesReports(domain.WorkflowRunRef{Kind: "imp", IssueNumber: intPtr(3)}))
}

func TestFindReports_MissingDirectoryIsNotAnError(t *testing.T) {
	reports, err := FindReports(filepath.Join(t.TempDir(), "absent"), domain.WorkflowRunRef{Kind: "imp", IssueNumber: intPtr(1)})

	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestFindReports_TruncatesAnOversizedReport(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, dir, "issue-1-implementation-report.md", strings.Repeat("x", maxReportBytes+10), time.Now())

	reports, err := FindReports(dir, domain.WorkflowRunRef{Kind: "imp", IssueNumber: intPtr(1)})

	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.True(t, strings.HasPrefix(reports[0].Body, strings.Repeat("x", maxReportBytes)))
	assert.Contains(t, reports[0].Body, "Report truncated")
}
