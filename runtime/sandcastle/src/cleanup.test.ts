import assert from "node:assert/strict";
import test from "node:test";
import {
  buildCleanupPlan,
  executeCleanup,
  parseOpenPullRequestBranches,
  parseWorktrees,
  type MergedPullRequest,
} from "./cleanup.js";

const merged: MergedPullRequest[] = [
  {
    number: 42,
    headRefName: "agent/fix-42",
    url: "https://github.com/owner/repo/pull/42",
  },
];

test("parseWorktrees reads branches, detached state, and locks", () => {
  assert.deepEqual(
    parseWorktrees(
      [
        "worktree /repo",
        "HEAD abc",
        "branch refs/heads/main",
        "",
        "worktree /repo/.sandcastle/reviews/pr-42-abc",
        "HEAD def",
        "detached",
        "locked reason",
      ].join("\n"),
    ),
    [
      { path: "/repo", branch: "main", locked: false },
      {
        path: "/repo/.sandcastle/reviews/pr-42-abc",
        branch: null,
        locked: true,
      },
    ],
  );
});

test("buildCleanupPlan includes merged worktrees and branches only", () => {
  const plan = buildCleanupPlan(
    [
      { path: "/repo", branch: "main", locked: false },
      { path: "/worktrees/merged", branch: "agent/fix-42", locked: false },
      { path: "/worktrees/open", branch: "agent/open-43", locked: false },
      {
        path: "/repo/.sandcastle/reviews/pr-42-abc",
        branch: null,
        locked: false,
      },
    ],
    ["main", "agent/fix-42", "agent/open-43"],
    ["agent/fix-42", "agent/open-43"],
    merged,
    ["/repo"],
    "main",
    () => true,
  );
  assert.deepEqual(
    plan.worktrees.map((item) => item.path),
    ["/worktrees/merged", "/repo/.sandcastle/reviews/pr-42-abc"],
  );
  assert.deepEqual(
    plan.branches.map((item) => item.branch),
    ["agent/fix-42"],
  );
  assert.deepEqual(
    plan.remoteBranches.map((item) => item.branch),
    ["agent/fix-42"],
  );
});

test("buildCleanupPlan skips dirty worktrees and preserves their branches", () => {
  const plan = buildCleanupPlan(
    [{ path: "/worktrees/dirty", branch: "agent/fix-42", locked: false }],
    ["agent/fix-42"],
    ["agent/fix-42"],
    merged,
    [],
    "main",
    () => false,
  );
  assert.equal(plan.worktrees.length, 0);
  assert.equal(plan.branches.length, 0);
  assert.equal(plan.remoteBranches.length, 0);
  assert.match(plan.skipped[0], /Dirty worktree/);
});

test("executeCleanup removes worktrees before local branches", () => {
  const calls: Array<{ command: string; args: string[] }> = [];
  executeCleanup(
    "/repo",
    {
      worktrees: [
        {
          path: "/worktrees/merged",
          branch: "agent/fix-42",
          locked: false,
          pullRequest: merged[0],
        },
      ],
      branches: [{ branch: "agent/fix-42", pullRequest: merged[0] }],
      remoteBranches: [
        { branch: "agent/fix-42", pullRequest: merged[0] },
      ],
      skipped: [],
    },
    (command, args) => {
      calls.push({ command, args });
      return "";
    },
  );
  assert.deepEqual(calls, [
    {
      command: "git",
      args: ["worktree", "remove", "/worktrees/merged"],
    },
    {
      command: "git",
      args: ["branch", "-D", "--", "agent/fix-42"],
    },
    {
      command: "git",
      args: ["push", "origin", "--delete", "agent/fix-42"],
    },
    { command: "git", args: ["worktree", "prune"] },
  ]);
});

test("a branch whose earlier releases merged is spared while a release is open", () => {
  // Release tooling reuses one branch for every release. Four merged release
  // pull requests put that name in the merged map while an open one is live
  // on it; deleting the branch would make GitHub close that open request.
  const releaseBranch = "release-please--branches--main--components--App";
  const plan = buildCleanupPlan(
    [
      { path: "/repo/.sandcastle/worktrees/release", branch: releaseBranch, locked: false },
      { path: "/repo/.sandcastle/worktrees/done", branch: "feat/done", locked: false },
    ],
    [releaseBranch, "feat/done"],
    [releaseBranch, "feat/done"],
    [
      { number: 773, headRefName: releaseBranch, url: "u773" },
      { number: 828, headRefName: releaseBranch, url: "u828" },
      { number: 900, headRefName: "feat/done", url: "u900" },
    ],
    [],
    "main",
    () => true,
    [releaseBranch],
  );

  assert.deepEqual(
    plan.remoteBranches.map((branch) => branch.branch),
    ["feat/done"],
    "the open release branch must never be pushed for deletion",
  );
  assert.deepEqual(
    plan.branches.map((branch) => branch.branch),
    ["feat/done"],
  );
  assert.deepEqual(
    plan.worktrees.map((worktree) => worktree.branch),
    ["feat/done"],
  );
  assert.ok(
    plan.skipped.some((reason) => reason.includes(releaseBranch)),
    "and the reason is reported rather than silently skipped",
  );
});

test("open pull request head refs are parsed, and bad output protects nothing extra", () => {
  assert.deepEqual(
    parseOpenPullRequestBranches('[{"headRefName":"a"},{"headRefName":"b"}]'),
    ["a", "b"],
  );
  assert.deepEqual(parseOpenPullRequestBranches("[]"), []);
  assert.deepEqual(parseOpenPullRequestBranches("not json"), []);
  assert.deepEqual(parseOpenPullRequestBranches('[{"other":1}]'), []);
});
