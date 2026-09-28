import assert from "node:assert/strict";
import test from "node:test";
import {
  findHerdrID,
  herdrWorktreeOpenArgs,
  resolveWorkflowCommand,
} from "./sandcastle.js";

test("workflow commands map issues, PRs, and maintenance targets", () => {
  assert.deepEqual(resolveWorkflowCommand("imp", "42", undefined), {
    kind: "imp",
    command: "imp",
    targetArgs: ["42"],
  });
  for (const kind of ["review", "resolve", "ci"]) {
    assert.deepEqual(resolveWorkflowCommand(kind, undefined, "17"), {
      kind,
      command: kind,
      targetArgs: ["17"],
    });
  }
  assert.deepEqual(resolveWorkflowCommand("clean", undefined, undefined), {
    kind: "clean",
    command: "clean",
    targetArgs: [],
  });
});

test("Lab sessions run grove-lab with the session kind and entry", () => {
  for (const kind of ["shape", "grill"]) {
    assert.deepEqual(
      resolveWorkflowCommand(kind, undefined, undefined, "20260928-081530-abcdef"),
      { kind, command: "grove-lab", targetArgs: [kind, "20260928-081530-abcdef"] },
    );
  }
});

test("workflow commands reject missing or invalid targets", () => {
  assert.throws(() => resolveWorkflowCommand("imp", undefined, undefined));
  assert.throws(() => resolveWorkflowCommand("review", undefined, undefined));
  assert.throws(() => resolveWorkflowCommand("unknown", "42", undefined));
  assert.throws(() => resolveWorkflowCommand("shape", undefined, undefined), /--entry/);
  assert.throws(
    () => resolveWorkflowCommand("shape", undefined, undefined, "../../etc"),
    /--entry/,
    "an entry ID cannot reach outside the Lab",
  );
  assert.throws(
    () => resolveWorkflowCommand("grilling", undefined, undefined),
    "the old grilling kind, which had no command, is gone",
  );
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

test("workflow worktree opens use the target repository as the Herdr source", () => {
  assert.deepEqual(herdrWorktreeOpenArgs("/repos/spectre"), [
    "worktree",
    "open",
    "--cwd",
    "/repos/spectre",
    "--path",
    "/repos/spectre",
    "--focus",
  ]);
});
