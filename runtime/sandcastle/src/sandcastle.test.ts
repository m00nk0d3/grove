import assert from "node:assert/strict";
import test from "node:test";
import { findHerdrID, resolveWorkflowCommand } from "./sandcastle.js";

test("workflow commands map issues, PRs, and maintenance targets", () => {
  assert.deepEqual(resolveWorkflowCommand("imp", "42", undefined), {
    kind: "imp",
    targetArgs: ["42"],
  });
  for (const kind of ["review", "resolve", "ci"]) {
    assert.deepEqual(resolveWorkflowCommand(kind, undefined, "17"), {
      kind,
      targetArgs: ["17"],
    });
  }
  assert.deepEqual(resolveWorkflowCommand("clean", undefined, undefined), {
    kind: "clean",
    targetArgs: [],
  });
});

test("workflow commands reject missing or invalid targets", () => {
  assert.throws(() => resolveWorkflowCommand("imp", undefined, undefined));
  assert.throws(() => resolveWorkflowCommand("review", undefined, undefined));
  assert.throws(() => resolveWorkflowCommand("unknown", "42", undefined));
});

test("Herdr identifiers are found in worktree and tab responses", () => {
  assert.equal(
    findHerdrID(
      { result: { workspace: { workspace_id: "w6" } } },
      "workspace_id",
    ),
    "w6",
  );
  assert.equal(
    findHerdrID({ result: { root_pane: { pane_id: "w6:p30" } } }, "pane_id"),
    "w6:p30",
  );
  assert.equal(findHerdrID({ result: {} }, "pane_id"), undefined);
});
