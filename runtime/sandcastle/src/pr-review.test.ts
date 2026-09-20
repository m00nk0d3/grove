import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {
  canSubmitReviewDecision,
  parseReviewCliArgs,
} from "./pr-review.js";
import {
  formatPullRequestVerdict,
  parseReviewPostAction,
  readPullRequestVerdict,
} from "./pull-request-review.js";

test("parseReviewCliArgs accepts local and explicit repository forms", () => {
  assert.deepEqual(parseReviewCliArgs(["124"]), {
    requestedRepo: null,
    prNumber: "124",
  });
  assert.deepEqual(parseReviewCliArgs(["owner/repo", "124"]), {
    requestedRepo: "owner/repo",
    prNumber: "124",
  });
});

test("parseReviewCliArgs rejects malformed input", () => {
  for (const args of [[], ["0"], ["abc"], ["owner/repo", "1; rm"]]) {
    assert.throws(() => parseReviewCliArgs(args), /Usage: review/);
  }
});

test("pull request verdicts are validated and formatted", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".pr-review-verdict-"));
  const verdictPath = path.join(root, "verdict.json");
  try {
    fs.writeFileSync(
      verdictPath,
      JSON.stringify({
        recommendation: "request_changes",
        summary: "A blocking nil dereference remains.",
        linkedIssues: ["#100 requires safe empty input handling."],
        acceptanceCriteria: ["Empty input must not panic: not satisfied."],
        reviewedAreas: ["Request parsing and validation."],
        findings: ["HIGH src/api.go:42 can dereference nil input."],
        strengths: ["Focused change with a regression test."],
        validation: ["go test ./...: passed."],
        residualRisks: ["External API integration was not exercised."],
      }),
    );
    const verdict = readPullRequestVerdict(verdictPath);
    const report = formatPullRequestVerdict(verdict, "124");
    assert.equal(verdict.recommendation, "request_changes");
    assert.match(report, /⛔ REQUEST CHANGES/);
    assert.match(report, /src\/api\.go:42/);
    assert.match(report, /## 🎯 Acceptance Criteria/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("structured review list entries are normalized without losing evidence", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".pr-review-structured-"));
  const verdictPath = path.join(root, "verdict.json");
  try {
    fs.writeFileSync(
      verdictPath,
      JSON.stringify({
        recommendation: "approve",
        summary: "Acceptance criteria are met.",
        linkedIssues: ["owner/repo#100"],
        acceptanceCriteria: [
          { criterion: "Empty input is safe", satisfied: true },
        ],
        reviewedAreas: [
          { area: "Error handling", evidence: "Errors preserve stderr." },
        ],
        findings: [
          {
            severity: "minor",
            file: "README.md",
            line: null,
            defect: "The PR body is stale.",
          },
        ],
        strengths: ["Focused implementation."],
        validation: [{ command: "go test ./...", result: "passed" }],
        residualRisks: [],
      }),
    );
    const verdict = readPullRequestVerdict(verdictPath);
    assert.equal(
      verdict.acceptanceCriteria[0],
      "Criterion: Empty input is safe; Satisfied: true",
    );
    assert.equal(
      verdict.validation[0],
      "Command: go test ./...; Result: passed",
    );
    assert.match(verdict.findings[0], /Severity: minor/);
    assert.doesNotMatch(verdict.findings[0], /Line:/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("review posting requires an explicit recognized action", () => {
  assert.equal(parseReviewPostAction("approve"), "approve");
  assert.equal(parseReviewPostAction("r"), "request_changes");
  assert.equal(parseReviewPostAction("comment"), "comment");
  assert.equal(parseReviewPostAction(""), "none");
  assert.equal(parseReviewPostAction("maybe"), "none");
});

test("self-authored PRs allow comments but not review decisions", () => {
  assert.equal(canSubmitReviewDecision("Owner", "owner"), false);
  assert.equal(canSubmitReviewDecision("contributor", "owner"), true);
  assert.equal(parseReviewPostAction("approve", false), "none");
  assert.equal(parseReviewPostAction("request changes", false), "none");
  assert.equal(parseReviewPostAction("comment", false), "comment");
});
