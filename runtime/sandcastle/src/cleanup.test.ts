import assert from "node:assert/strict";
import test from "node:test";
import {
  buildCleanupPlan,
  executeCleanup,
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
