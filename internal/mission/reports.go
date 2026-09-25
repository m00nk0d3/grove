package mission

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/m00nk0d3/grove/internal/domain"
)

// ReportsDir is the directory, inside a repository's git common directory,
// where the Sandcastle runtime keeps workflow checkpoints and reports.
const ReportsDir = "agent-flow"

// maxReportBytes bounds how much of one report is read. Reports run to tens of
// kilobytes; the bound only stops a runaway file from being loaded whole.
const maxReportBytes = 1 << 20

// reportSpec names one report a workflow kind can write. pattern is a glob
// relative to the reports directory; a pattern that matches several files,
// such as one review per reviewed head, yields one report per file, newest
// first.
type reportSpec struct {
	title   string
	pattern string
}

// reportSpecs returns the reports a workflow of this kind writes for its
// issue or pull request. Kinds that write none return nil.
func reportSpecs(workflow domain.WorkflowRunRef) []reportSpec {
	switch workflow.Kind {
	case "imp":
		if workflow.IssueNumber == nil {
			return nil
		}
		n := *workflow.IssueNumber
		return []reportSpec{
			{title: "Implementation report", pattern: fmt.Sprintf("issue-%d-implementation-report.md", n)},
			{title: "Review verdict", pattern: fmt.Sprintf("issue-%d-review-verdict.md", n)},
		}
	case "review":
		if workflow.PRNumber == nil {
			return nil
		}
		return []reportSpec{
			{title: "Pull request review", pattern: fmt.Sprintf("pr-%d-*-review.md", *workflow.PRNumber)},
		}
	case "ci":
		if workflow.PRNumber == nil {
			return nil
		}
		return []reportSpec{
			{title: "CI failures", pattern: filepath.Join(fmt.Sprintf("ci-pr-%d", *workflow.PRNumber), "failures.md")},
		}
	}
	return nil
}

// WritesReports reports whether a workflow of this kind produces any report,
// so a caller can tell "none yet" apart from "none ever".
func WritesReports(workflow domain.WorkflowRunRef) bool {
	return len(reportSpecs(workflow)) > 0
}

// FindReports reads the reports a workflow has written so far from
// reportsDir, in the order the workflow writes them. A report that does not
// exist yet is left out rather than treated as an error.
func FindReports(reportsDir string, workflow domain.WorkflowRunRef) ([]domain.WorkflowReport, error) {
	var reports []domain.WorkflowReport
	for _, spec := range reportSpecs(workflow) {
		matches, err := filepath.Glob(filepath.Join(reportsDir, spec.pattern))
		if err != nil {
			return nil, fmt.Errorf("find %s: %w", strings.ToLower(spec.title), err)
		}
		var found []domain.WorkflowReport
		for _, path := range matches {
			report, err := readReport(path, spec.title)
			if err != nil {
				return nil, err
			}
			found = append(found, report)
		}
		sort.SliceStable(found, func(i, j int) bool { return found[i].ModTime.After(found[j].ModTime) })
		if len(found) > 1 {
			for i := range found {
				found[i].Title = fmt.Sprintf("%s %d/%d", spec.title, len(found)-i, len(found))
			}
		}
		reports = append(reports, found...)
	}
	return reports, nil
}

func readReport(path, title string) (domain.WorkflowReport, error) {
	f, err := os.Open(path)
	if err != nil {
		return domain.WorkflowReport{}, fmt.Errorf("open report %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return domain.WorkflowReport{}, fmt.Errorf("stat report %s: %w", filepath.Base(path), err)
	}
	buf := make([]byte, min(info.Size(), maxReportBytes))
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return domain.WorkflowReport{}, fmt.Errorf("read report %s: %w", filepath.Base(path), err)
	}
	body := string(buf[:n])
	if info.Size() > maxReportBytes {
		body += "\n\n> Report truncated: only the first 1 MiB is shown.\n"
	}
	return domain.WorkflowReport{
		Title:   title,
		Path:    path,
		ModTime: info.ModTime(),
		Body:    body,
	}, nil
}
