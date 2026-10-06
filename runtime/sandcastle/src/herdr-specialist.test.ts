import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";
import { resolveAgentArtifactPaths } from "./herdr-specialist.js";

const worktree = path.join(
  "/repo/.sandcastle/worktrees",
  "agent-add-close-issues-command-to-actions-menu-270",
);

test("relative artifact paths resolve against the worktree, not the checkout", () => {
  const prompt = `
Write requirements to '.agent/issue-270/REQUIREMENTS.md'.
The plan is at .agent/issue-270/PLAN.md, and the handoff at
\`.agent/issue-270/HANDOFF.md\`.
`;
  const resolved = resolveAgentArtifactPaths(prompt, worktree);
  assert.match(resolved, /\/REQUIREMENTS\.md/);
  for (const name of ["REQUIREMENTS.md", "PLAN.md", "HANDOFF.md"]) {
    assert.ok(
      resolved.includes(`${worktree}/.agent/issue-270/${name}`),
      `${name} must be written inside the worktree:\n${resolved}`,
    );
    assert.ok(
      !resolved.includes(`../.agent/issue-270/${name}`),
      `${name} must not escape the worktree`,
    );
  }
});

test("an already-absolute artifact path is left alone", () => {
  const absolute = `${worktree}/.agent/issue-270/REQUIREMENTS.md`;
  assert.equal(
    resolveAgentArtifactPaths(`Write to ${absolute}.`, worktree),
    `Write to ${absolute}.`,
  );
});

test("paths outside the agent directory are untouched", () => {
  const prompt = "Edit cmd/grove/app.go and runtime/sandcastle/src/orchestrator.ts.";
  assert.equal(resolveAgentArtifactPaths(prompt, worktree), prompt);
});

test("a leading artifact path at the start of the assignment resolves", () => {
  assert.equal(
    resolveAgentArtifactPaths(".agent/issue-7/PLAN.md", worktree),
    `${worktree}/.agent/issue-7/PLAN.md`,
  );
});
