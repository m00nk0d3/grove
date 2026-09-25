import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import {
  buildCiFixPrompt,
  classifyWorktreeDrift,
  extractActionsRunIds,
  parseCiArgs,
  parseFailedChecks,
  syncWorktreeToPullRequestHead,
  validateFixablePullRequest,
} from "./ci-fix.js";

const pullRequest = {
  number: 42,
  title: "Repair CI",
  url: "https://github.com/owner/repo/pull/42",
  headRefName: "feature/ci",
  headRefOid: "abc123",
  baseRefName: "main",
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
  assert.deepEqual(parseCiArgs(["42"]), { prNumber: "42", resume: false });
  assert.throws(() => parseCiArgs([]), /Usage: ci/);
  assert.throws(() => parseCiArgs(["owner/repo", "42"]), /Usage: ci/);
  assert.throws(() => parseCiArgs(["../42"]), /Usage: ci/);
});

// The dirty-worktree refusal tells the user to rerun with --continue, so ci
// has to accept it, before or after the number.
test("parseCiArgs accepts --continue to resume an earlier run", () => {
  assert.deepEqual(parseCiArgs(["42", "--continue"]), { prNumber: "42", resume: true });
  assert.deepEqual(parseCiArgs(["--continue", "42"]), { prNumber: "42", resume: true });
  assert.throws(() => parseCiArgs(["--continue"]), /Usage: ci <pr_number> \[--continue\]/);
});

test("a resumed CI prompt tells the agent to build on the earlier changes", () => {
  const fresh = buildCiFixPrompt("persona", pullRequest, [checks[0]], "/tmp/failures.md");
  const resumed = buildCiFixPrompt("persona", pullRequest, [checks[0]], "/tmp/failures.md", true);
  assert.doesNotMatch(fresh, /Work already in progress/);
  assert.match(resumed, /Work already in progress/);
  assert.match(resumed, /git diff/);
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
  const prompt = buildCiFixPrompt("persona", pullRequest, [checks[0]], "/tmp/failures.md");
  assert.match(prompt, /CI \/ test: FAILURE/);
  assert.match(prompt, /root cause/);
  assert.match(prompt, /Do not commit, push/);
  assert.match(prompt, /external, flaky/);
});

// A repository holding two commits, with the checkout left on the first one.
// `second` is still reachable, which is exactly the shape of a branch that
// moved on GitHub after this worktree was made.
function twoCommitRepository(prefix: string): {
  root: string;
  git: (...args: string[]) => void;
  head: () => string;
  commit: (content: string, message: string) => string;
} {
  const root = fs.mkdtempSync(path.join(process.cwd(), prefix));
  const git = (...args: string[]) => {
    execFileSync("git", args, { cwd: root, stdio: "pipe" });
  };
  const head = () =>
    execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim();
  const commit = (content: string, message: string) => {
    fs.writeFileSync(path.join(root, "tracked.txt"), content);
    git("add", "tracked.txt");
    // Signing is a property of the machine running the suite, not of what is
    // under test, so the fixture opts out rather than inheriting it.
    git(
      "-c",
      "user.name=Test",
      "-c",
      "user.email=test@example.com",
      "-c",
      "commit.gpgsign=false",
      "commit",
      "--quiet",
      "-m",
      message,
    );
    return head();
  };
  git("init", "--quiet");
  // Pin line endings so the fixture reads back what it wrote on every platform,
  // rather than whatever the machine configures globally.
  git("config", "core.autocrlf", "false");
  return { root, git, head, commit };
}

test("a worktree left behind the pull request head is caught up", () => {
  const repo = twoCommitRepository(".drift-behind-");
  try {
    const first = repo.commit("one\n", "first");
    const second = repo.commit("two\n", "second");
    repo.git("reset", "--hard", first);

    assert.deepEqual(classifyWorktreeDrift(repo.root, second), {
      kind: "behind",
      behind: 1,
    });
    syncWorktreeToPullRequestHead(repo.root, second, "address");

    assert.equal(repo.head(), second);
    assert.equal(
      fs.readFileSync(path.join(repo.root, "tracked.txt"), "utf8"),
      "two\n",
      "the checkout holds what the pull request actually contains",
    );
    assert.deepEqual(classifyWorktreeDrift(repo.root, second), { kind: "current" });
  } finally {
    fs.rmSync(repo.root, { recursive: true, force: true });
  }
});

test("drift the command cannot resolve is refused with the reason and the fix", () => {
  const repo = twoCommitRepository(".drift-refused-");
  try {
    const first = repo.commit("one\n", "first");
    const second = repo.commit("two\n", "second");

    // Ahead: the checkout holds commits the pull request does not.
    assert.deepEqual(classifyWorktreeDrift(repo.root, first), {
      kind: "ahead",
      ahead: 1,
    });
    assert.throws(
      () => syncWorktreeToPullRequestHead(repo.root, first, "address"),
      /1 commit ahead of the pull request head[\s\S]*address would work on code no reviewer has seen/,
    );

    // Diverged: an amend or a rebase on one side.
    repo.git("reset", "--hard", first);
    const rewritten = repo.commit("two, rewritten\n", "second, rewritten");
    assert.deepEqual(classifyWorktreeDrift(repo.root, second), {
      kind: "diverged",
      ahead: 1,
      behind: 1,
    });
    assert.throws(
      () => syncWorktreeToPullRequestHead(repo.root, second, "ci"),
      /have diverged: 1 commit here, 1 commit on the pull request/,
    );

    // Behind, but with work in the tree that a fast-forward would disturb.
    repo.git("reset", "--hard", first);
    fs.writeFileSync(path.join(repo.root, "tracked.txt"), "uncommitted\n");
    assert.throws(
      () => syncWorktreeToPullRequestHead(repo.root, second, "address"),
      /uncommitted changes[\s\S]*Commit or stash them/,
    );
    assert.equal(repo.head(), first, "a refusal leaves the checkout untouched");

    // A head this repository has never seen: the branch was force-pushed.
    repo.git("checkout", "--quiet", "--", "tracked.txt");
    const unknown = "0".repeat(39) + "1";
    assert.deepEqual(classifyWorktreeDrift(repo.root, unknown), {
      kind: "unreachable",
    });
    assert.throws(
      () => syncWorktreeToPullRequestHead(repo.root, unknown, "ci"),
      /not in this repository[\s\S]*force-pushed[\s\S]*git fetch --prune origin/,
    );
    assert.notEqual(rewritten, second);
  } finally {
    fs.rmSync(repo.root, { recursive: true, force: true });
  }
});
