import test from "node:test";
import assert from "node:assert/strict";
import { filesOutsideAssignedScope } from "./specialist-gating.js";
import type { StackProject } from "./stack-detector.js";

const groveProjects: StackProject[] = [
  { stack: "GO", root: "", marker: "go.mod" },
  {
    stack: "TYPESCRIPT",
    root: "runtime/sandcastle",
    marker: "runtime/sandcastle/package.json",
  },
  { stack: "TYPESCRIPT", root: "website", marker: "website/package.json" },
];

test("files in another stack's project fail a run's scope", () => {
  assert.deepEqual(
    filesOutsideAssignedScope(
      ["website/src/features/issues/components/IssueList.tsx"],
      "GO",
      groveProjects,
    ),
    ["website/src/features/issues/components/IssueList.tsx"],
  );
  assert.deepEqual(
    filesOutsideAssignedScope(
      ["runtime/sandcastle/src/orchestrator.ts"],
      "GO",
      groveProjects,
    ),
    ["runtime/sandcastle/src/orchestrator.ts"],
  );
});

test("files in the assigned stack pass, including shared roots", () => {
  assert.deepEqual(
    filesOutsideAssignedScope(
      [
        "internal/tui/modal/issue_close.go",
        "cmd/grove/app.go",
        "README.md",
        ".agent/issue-280/PLAN.md",
      ],
      "GO",
      groveProjects,
    ),
    [],
  );
  assert.deepEqual(
    filesOutsideAssignedScope(["website/src/App.tsx"], "TYPESCRIPT", groveProjects),
    [],
  );
});

test("files owned by no project pass; unknown stacks match themselves", () => {
  const single: StackProject[] = [
    { stack: "TYPESCRIPT", root: "website", marker: "website/package.json" },
  ];
  assert.deepEqual(
    filesOutsideAssignedScope(["docs/guide.md"], "TYPESCRIPT", single),
    [],
  );
  const unknown: StackProject[] = [
    { stack: "UNKNOWN", root: "infra", marker: "infra/main.tf" },
  ];
  assert.deepEqual(
    filesOutsideAssignedScope(["infra/main.tf"], "UNKNOWN", unknown),
    [],
  );
  assert.deepEqual(
    filesOutsideAssignedScope(["infra/main.tf"], "GO", unknown),
    ["infra/main.tf"],
  );
});
