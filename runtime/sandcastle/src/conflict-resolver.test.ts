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

test("resolve trusts isCrossRepository rather than the head repository name", () => {
  // `gh pr view` returns nameWithOwner as an empty string, and omits the head
  // repository entirely once a fork is deleted. Neither means the pull request
  // is a fork, and resolve used to reject its own repository's branches on the
  // strength of the empty string alone.
  assert.doesNotThrow(() =>
    validateResolvablePullRequest(
      { ...pullRequest, headRepository: { nameWithOwner: "" } },
      "owner/repo",
    ),
  );
  assert.doesNotThrow(() =>
    validateResolvablePullRequest(
      {
        ...pullRequest,
        headRepository: { name: "repo", nameWithOwner: "" },
        headRepositoryOwner: { login: "owner" },
      },
      "owner/repo",
    ),
  );
  assert.doesNotThrow(() =>
    validateResolvablePullRequest(
      { ...pullRequest, headRepository: null },
      "owner/repo",
    ),
  );
  // A head repository that is identifiable and belongs elsewhere is still a
  // fork, and the message names resolve rather than ci.
  assert.throws(
    () =>
      validateResolvablePullRequest(
        {
          ...pullRequest,
          headRepository: { name: "repo", nameWithOwner: "" },
          headRepositoryOwner: { login: "someone-else" },
        },
        "owner/repo",
      ),
    /comes from a fork\. The resolve command only pushes branches owned by this repository/,
  );
});

test("conflict prompt requires evidence-based resolution without delivery", () => {
  const prompt = buildConflictPrompt(pullRequest, ["src/api.ts", "src/api.test.ts"]);
  assert.match(prompt, /src\/api\.ts/);
  assert.match(prompt, /preserving the intended behavior from both/);
  assert.match(prompt, /Do not abort the merge, commit, push/);
});
