import assert from "node:assert/strict";
import test from "node:test";
import {
  buildCiFixPrompt,
  extractActionsRunIds,
  parseCiArgs,
  parseFailedChecks,
  validateFixablePullRequest,
} from "./ci-fix.js";

const pullRequest = {
  number: 42,
  title: "Repair CI",
  url: "https://github.com/owner/repo/pull/42",
  headRefName: "feature/ci",
  headRefOid: "abc123",
  isCrossRepository: false,
  state: "OPEN",
  headRepository: { nameWithOwner: "owner/repo" },
};

const checks = [
  {
    bucket: "fail",
    link: "https://github.com/owner/repo/actions/runs/123/job/456",
    name: "test",
    state: "FAILURE",
    workflow: "CI",
  },
  {
    bucket: "pass",
    link: "https://github.com/owner/repo/actions/runs/124/job/457",
    name: "lint",
    state: "SUCCESS",
    workflow: "CI",
  },
];

test("parseCiArgs accepts exactly one PR number", () => {
  assert.equal(parseCiArgs(["42"]), "42");
  assert.throws(() => parseCiArgs([]), /Usage: ci/);
  assert.throws(() => parseCiArgs(["owner/repo", "42"]), /Usage: ci/);
  assert.throws(() => parseCiArgs(["../42"]), /Usage: ci/);
});

test("validateFixablePullRequest rejects merged and fork PRs", () => {
  assert.doesNotThrow(() =>
    validateFixablePullRequest(pullRequest, "owner/repo"),
  );
  assert.throws(
    () =>
      validateFixablePullRequest(
        { ...pullRequest, state: "MERGED" },
        "owner/repo",
      ),
    /is merged/,
  );
  assert.throws(
    () =>
      validateFixablePullRequest(
        { ...pullRequest, isCrossRepository: true },
        "owner/repo",
      ),
    /comes from a fork/,
  );
});

test("parseFailedChecks returns only failed checks and validates input", () => {
  assert.deepEqual(parseFailedChecks(JSON.stringify(checks)), [checks[0]]);
  assert.throws(() => parseFailedChecks("{}"), /invalid CI check data/);
  assert.throws(
    () => parseFailedChecks('[{"bucket":"fail"}]'),
    /invalid CI check entry/,
  );
});

test("extractActionsRunIds deduplicates GitHub Actions run links", () => {
  assert.deepEqual(
    extractActionsRunIds([checks[0], { ...checks[0], name: "test again" }]),
    ["123"],
  );
  assert.deepEqual(
    extractActionsRunIds([
      { ...checks[0], link: "https://ci.example.com/build/123" },
    ]),
    [],
  );
});

test("CI prompt requires root-cause fixes without delivery", () => {
  const prompt = buildCiFixPrompt(pullRequest, [checks[0]], "/tmp/failures.md");
  assert.match(prompt, /CI \/ test: FAILURE/);
  assert.match(prompt, /root cause/);
  assert.match(prompt, /Do not commit, push/);
  assert.match(prompt, /external, flaky/);
});
