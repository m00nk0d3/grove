import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  createWorkflow,
  listWorkflows,
  removeWorkflow,
  saveWorkflow,
} from "./runtime-state.js";

test("runtime state is discoverable through the repository git directory", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "grove-runtime-"));
  try {
    const initialized = spawnSync("git", ["init", "-q", repo]);
    assert.equal(initialized.status, 0);

    const workflow = createWorkflow("imp", ["42"], repo, "run_test");
    workflow.status = "running";
    workflow.pid = process.pid;
    saveWorkflow(repo, workflow);

    const workflows = listWorkflows(repo);
    assert.equal(workflows.length, 1);
    assert.equal(workflows[0].id, "run_test");
    assert.equal(workflows[0].status, "running");
    assert.equal(workflows[0].github.issue, 42);
  } finally {
    fs.rmSync(repo, { recursive: true, force: true });
  }
});

test("completed workflow history can be removed without stopping a process", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "grove-runtime-"));
  try {
    const initialized = spawnSync("git", ["init", "-q", repo]);
    assert.equal(initialized.status, 0);

    const workflow = createWorkflow("review", ["42"], repo, "run_completed");
    workflow.status = "succeeded";
    saveWorkflow(repo, workflow);

    assert.deepEqual(removeWorkflow(repo, workflow.id, false), {
      removed: "run_completed",
      stopped: false,
    });
    assert.equal(listWorkflows(repo).length, 0);
  } finally {
    fs.rmSync(repo, { recursive: true, force: true });
  }
});

test("active workflow removal requires explicit stop confirmation", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "grove-runtime-"));
  try {
    const initialized = spawnSync("git", ["init", "-q", repo]);
    assert.equal(initialized.status, 0);

    const workflow = createWorkflow("imp", ["42"], repo, "run_active");
    workflow.status = "running";
    workflow.pid = null;
    saveWorkflow(repo, workflow);

    assert.throws(
      () => removeWorkflow(repo, workflow.id, false),
      /confirm stop/,
    );
    removeWorkflow(repo, workflow.id, true);
    assert.equal(listWorkflows(repo).length, 0);
  } finally {
    fs.rmSync(repo, { recursive: true, force: true });
  }
});

test("workflow stop refuses a PID without matching ownership metadata", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "grove-runtime-"));
  try {
    const initialized = spawnSync("git", ["init", "-q", repo]);
    assert.equal(initialized.status, 0);

    const workflow = createWorkflow("imp", ["42"], repo, "run_stale");
    workflow.status = "running";
    workflow.pid = process.pid;
    saveWorkflow(repo, workflow);

    assert.throws(
      () => removeWorkflow(repo, workflow.id, true),
      /not owned by workflow/,
    );
    assert.equal(listWorkflows(repo).length, 1);
  } finally {
    fs.rmSync(repo, { recursive: true, force: true });
  }
});
