#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import { fileURLToPath } from "node:url";
import {
  createWorkflow,
  listWorkflows,
  removeWorkflow,
  saveWorkflow,
  type RuntimeWorkflow,
  type WorkflowKind,
} from "./runtime-state.js";

const VERSION = "0.1.0";

function output(value: unknown): void {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

function flag(args: string[], name: string): string | undefined {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : undefined;
}

export function findHerdrID(
  value: unknown,
  key: "pane_id" | "workspace_id",
): string | undefined {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findHerdrID(item, key);
      if (found) return found;
    }
    return undefined;
  }
  if (value === null || typeof value !== "object") return undefined;
  const record = value as Record<string, unknown>;
  if (typeof record[key] === "string") return record[key];
  for (const child of Object.values(record)) {
    const found = findHerdrID(child, key);
    if (found) return found;
  }
  return undefined;
}

function status(cwd: string): void {
  const workflows = listWorkflows(cwd);
  output({
    version: VERSION,
    updated_at: new Date().toISOString(),
    active_workflows: workflows.filter((workflow) =>
      ["queued", "running"].includes(workflow.status),
    ).length,
    workflows,
  });
}

function workflowList(cwd: string): void {
  output({ workflows: listWorkflows(cwd) });
}

function workflowGet(cwd: string, id: string | undefined): void {
  if (!id) throw new Error("Usage: grove-sandcastle workflow get <run-id> --json");
  const workflow = listWorkflows(cwd).find((candidate) => candidate.id === id);
  if (!workflow) throw new Error(`Unknown workflow run: ${id}`);
  output(workflow);
}

function workflowRemove(cwd: string, id: string | undefined, args: string[]): void {
  if (!id) throw new Error("Usage: grove-sandcastle workflow remove <run-id> [--stop] --json");
  const repo = flag(args, "--repo") ?? cwd;
  output(removeWorkflow(repo, id, args.includes("--stop")));
}

export function resolveWorkflowCommand(
  requestedKind: string,
  issue: string | undefined,
  pullRequest: string | undefined,
): { kind: WorkflowKind; targetArgs: string[] } {
  const kinds: WorkflowKind[] = ["imp", "review", "resolve", "ci", "clean"];
  if (!kinds.includes(requestedKind as WorkflowKind)) {
    throw new Error(`unsupported workflow kind "${requestedKind}"`);
  }
  const kind = requestedKind as WorkflowKind;
  if (kind === "imp" && (!issue || !/^[1-9][0-9]*$/.test(issue))) {
    throw new Error("imp workflow requires --issue <number>");
  }
  if (
    ["review", "resolve", "ci"].includes(kind) &&
    (!pullRequest || !/^[1-9][0-9]*$/.test(pullRequest))
  ) {
    throw new Error(`${kind} workflow requires --pr <number>`);
  }
  return {
    kind,
    targetArgs:
      kind === "imp" ? [issue!] : pullRequest === undefined ? [] : [pullRequest],
  };
}

export function herdrWorktreeOpenArgs(repo: string): string[] {
  return ["worktree", "open", "--cwd", repo, "--path", repo, "--focus"];
}

function workflowStart(cwd: string, args: string[]): void {
  const requestedKind = flag(args, "--kind") ?? "imp";
  const issue = flag(args, "--issue");
  const pullRequest = flag(args, "--pr");
  const repo = flag(args, "--repo") ?? cwd;
  const source = flag(args, "--source") ?? "grove";
  const agent = flag(args, "--agent") ?? "opencode";
  const { kind, targetArgs } = resolveWorkflowCommand(
    requestedKind,
    issue,
    pullRequest,
  );
  if (!["opencode", "pi"].includes(agent)) {
    throw new Error(`unsupported agent "${agent}"; expected opencode or pi`);
  }

  const runID = `run_${randomUUID()}`;
  const workflow = createWorkflow(kind, targetArgs, repo, runID);
  workflow.source = source;
  workflow.default_agent = agent;
  saveWorkflow(repo, workflow);

  const opened = spawnSync("herdr", herdrWorktreeOpenArgs(repo), {
    encoding: "utf8",
  });
  if (opened.status !== 0) {
    throw new Error(
      (opened.stderr || opened.stdout).trim() ||
        "unable to open the Herdr worktree window",
    );
  }
  const workspaceID = findHerdrID(JSON.parse(opened.stdout), "workspace_id");
  if (!workspaceID) throw new Error("Herdr returned no workspace ID");

  const tab = spawnSync(
    "herdr",
    [
      "tab",
      "create",
      "--workspace",
      workspaceID,
      "--cwd",
      repo,
      "--label",
      workflow.title,
      "--env",
      `GROVE_WORKFLOW_RUN_ID=${runID}`,
      "--env",
      `GROVE_WORKFLOW_SOURCE=${source}`,
      "--env",
      `AGENT_FLOW_AGENT_BACKEND=${agent}`,
      "--focus",
    ],
    { encoding: "utf8" },
  );
  if (tab.status !== 0) {
    throw new Error(
      (tab.stderr || tab.stdout).trim() || "unable to create Herdr workflow tab",
    );
  }
  const paneID = findHerdrID(JSON.parse(tab.stdout), "pane_id");
  if (!paneID) throw new Error("Herdr returned no pane ID");

  const started = spawnSync("herdr", ["pane", "run", paneID, kind, ...targetArgs], {
    encoding: "utf8",
  });
  if (started.status !== 0) {
    throw new Error(
      (started.stderr || started.stdout).trim() || "unable to start workflow",
    );
  }

  output({ workflow });
}

function main(args = process.argv.slice(2)): void {
  const cwd = process.cwd();
  if (args[0] === "--version" || args[0] === "-V") {
    process.stdout.write(`grove-sandcastle ${VERSION}\n`);
    return;
  }
  if (args[0] === "status") return status(cwd);
  if (args[0] === "workflow" && args[1] === "list") return workflowList(cwd);
  if (args[0] === "workflow" && args[1] === "get") {
    return workflowGet(cwd, args[2]);
  }
  if (args[0] === "workflow" && args[1] === "remove") {
    return workflowRemove(cwd, args[2], args.slice(3));
  }
  if (args[0] === "workflow" && args[1] === "start") {
    return workflowStart(cwd, args.slice(2));
  }
  throw new Error(
    "Usage: grove-sandcastle status|workflow list|workflow get <id>|workflow remove <id> [--stop]|workflow start --kind <imp|review|resolve|ci|clean> [--issue <number>|--pr <number>]",
  );
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
