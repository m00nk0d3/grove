import assert from "node:assert/strict";
import test from "node:test";
import {
  buildConflictPrompt,
  parseResolveArgs,
  validateResolvablePullRequest,
} from "./conflict-resolver.js";

const pullRequest = {
  number: 42,
  title: "Resolve behavior",
  url: "https://github.com/owner/repo/pull/42",
  headRefName: "feature/resolve",
  headRefOid: "abc123",
  baseRefName: "main",
  isCrossRepository: false,
  state: "OPEN",
  headRepository: { nameWithOwner: "owner/repo" },
};

test("parseResolveArgs accepts exactly one PR number", () => {
  assert.equal(parseResolveArgs(["42"]), "42");
  assert.throws(() => parseResolveArgs([]), /Usage: resolve/);
  assert.throws(() => parseResolveArgs(["owner/repo", "42"]), /Usage: resolve/);
  assert.throws(() => parseResolveArgs(["../42"]), /Usage: resolve/);
});

test("validateResolvablePullRequest rejects merged and fork PRs", () => {
  assert.doesNotThrow(() =>
    validateResolvablePullRequest(pullRequest, "owner/repo"),
  );
  assert.throws(
    () =>
      validateResolvablePullRequest(
        { ...pullRequest, state: "MERGED" },
        "owner/repo",
      ),
    /is merged/,
  );
  assert.throws(
    () =>
      validateResolvablePullRequest(
        { ...pullRequest, isCrossRepository: true },
        "owner/repo",
      ),
    /comes from a fork/,
  );
});

test("conflict prompt requires evidence-based resolution without delivery", () => {
  const prompt = buildConflictPrompt(pullRequest, ["src/api.ts", "src/api.test.ts"]);
  assert.match(prompt, /src\/api\.ts/);
  assert.match(prompt, /preserving the intended behavior from both/);
  assert.match(prompt, /Do not abort the merge, commit, push/);
});
