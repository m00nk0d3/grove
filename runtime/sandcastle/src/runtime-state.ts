import { randomUUID } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

export type WorkflowKind = "imp" | "review" | "resolve" | "ci" | "clean";
export type WorkflowStatus = "queued" | "running" | "succeeded" | "failed";

export interface RuntimeWorkflow {
  id: string;
  title: string;
  status: WorkflowStatus;
  repo: string;
  worktree_path: string;
  branch: string;
  default_agent: string;
  current_step: string;
  progress: { completed: number; total: number; percent: number };
  github: { issue: number | null; pull_request: number | null };
  agents: [];
  steps: Array<{ id: string; title: string; status: string }>;
  started_at: string;
  updated_at: string;
  kind: WorkflowKind;
  pid: number | null;
  source: string;
  error?: string;
}

let activeWorkflow: { cwd: string; workflow: RuntimeWorkflow } | null = null;

function gitOutput(cwd: string, args: string[]): string {
  const result = spawnSync("git", args, { cwd, encoding: "utf8" });
  return result.status === 0 ? result.stdout.trim() : "";
}

export function workflowStateDir(cwd: string): string {
  const commonDir = gitOutput(cwd, ["rev-parse", "--git-common-dir"]);
  if (commonDir) {
    return path.join(path.resolve(cwd, commonDir), "grove-workflows");
  }
  return path.join(os.homedir(), ".grove", "workflows");
}

function numericTarget(args: string[]): number | null {
  const value = args.find((arg) => /^[1-9][0-9]*$/.test(arg));
  return value ? Number(value) : null;
}

function titleFor(kind: WorkflowKind, target: number | null): string {
  const suffix = target === null ? "" : ` #${target}`;
  switch (kind) {
    case "imp":
      return `Implement issue${suffix}`;
    case "review":
      return `Review pull request${suffix}`;
    case "resolve":
      return `Resolve conflicts${suffix}`;
    case "ci":
      return `Repair CI${suffix}`;
    case "clean":
      return "Clean merged worktrees";
  }
}

function writeWorkflow(cwd: string, workflow: RuntimeWorkflow): void {
  const dir = workflowStateDir(cwd);
  fs.mkdirSync(dir, { recursive: true });
  const destination = path.join(dir, `${workflow.id}.json`);
  const temporary = `${destination}.${process.pid}.tmp`;
  fs.writeFileSync(temporary, `${JSON.stringify(workflow, null, 2)}\n`, {
    mode: 0o600,
  });
  fs.renameSync(temporary, destination);
}

export function createWorkflow(
  kind: WorkflowKind,
  args: string[],
  cwd = process.cwd(),
  id = process.env.GROVE_WORKFLOW_RUN_ID ?? `run_${randomUUID()}`,
): RuntimeWorkflow {
  const target = numericTarget(args);
  const now = new Date().toISOString();
  const isIssue = kind === "imp";
  return {
    id,
    title: titleFor(kind, target),
    status: "queued",
    repo: gitOutput(cwd, ["rev-parse", "--show-toplevel"]) || path.resolve(cwd),
    worktree_path: cwd,
    branch: gitOutput(cwd, ["branch", "--show-current"]),
    default_agent:
      process.env.AGENT_FLOW_AGENT_BACKEND === "pi" ? "pi" : "opencode",
    current_step: "Queued",
    progress: { completed: 0, total: 1, percent: 0 },
    github: {
      issue: isIssue ? target : null,
      pull_request: !isIssue && kind !== "clean" ? target : null,
    },
    agents: [],
    steps: [{ id: kind, title: titleFor(kind, target), status: "queued" }],
    started_at: now,
    updated_at: now,
    kind,
    pid: null,
    source: process.env.GROVE_WORKFLOW_SOURCE ?? "command",
  };
}

export function saveWorkflow(cwd: string, workflow: RuntimeWorkflow): void {
  writeWorkflow(cwd, workflow);
}

export function updateTrackedWorkflow(
  update: Partial<RuntimeWorkflow>,
): void {
  if (activeWorkflow === null) return;
  Object.assign(activeWorkflow.workflow, update, {
    updated_at: new Date().toISOString(),
  });
  writeWorkflow(activeWorkflow.cwd, activeWorkflow.workflow);
}

export async function runTrackedWorkflow(
  kind: WorkflowKind,
  args: string[],
  run: (args: string[]) => Promise<void>,
): Promise<void> {
  const cwd = process.cwd();
  const workflow = createWorkflow(kind, args, cwd);
  workflow.status = "running";
  workflow.pid = process.pid;
  workflow.current_step = `Running ${kind}`;
  workflow.steps[0].status = "running";
  workflow.updated_at = new Date().toISOString();
  activeWorkflow = { cwd, workflow };
  writeWorkflow(cwd, workflow);

  try {
    await run(args);
    workflow.status = "succeeded";
    workflow.current_step = "Complete";
    workflow.progress = { completed: 1, total: 1, percent: 100 };
    workflow.steps[0].status = "succeeded";
  } catch (error) {
    workflow.status = "failed";
    workflow.current_step = "Failed";
    workflow.steps[0].status = "failed";
    workflow.error = error instanceof Error ? error.message : String(error);
    throw error;
  } finally {
    workflow.pid = null;
    workflow.updated_at = new Date().toISOString();
    writeWorkflow(cwd, workflow);
    activeWorkflow = null;
  }
}

function processAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

export function listWorkflows(cwd = process.cwd()): RuntimeWorkflow[] {
  const dir = workflowStateDir(cwd);
  if (!fs.existsSync(dir)) return [];

  const workflows: RuntimeWorkflow[] = [];
  for (const name of fs.readdirSync(dir)) {
    if (!name.endsWith(".json")) continue;
    try {
      const workflow = JSON.parse(
        fs.readFileSync(path.join(dir, name), "utf8"),
      ) as RuntimeWorkflow;
      if (
        workflow.status === "running" &&
        workflow.pid !== null &&
        !processAlive(workflow.pid)
      ) {
        workflow.status = "failed";
        workflow.current_step = "Process exited unexpectedly";
        workflow.steps[0].status = "failed";
        workflow.error = "workflow process is no longer running";
        workflow.pid = null;
        workflow.updated_at = new Date().toISOString();
        writeWorkflow(cwd, workflow);
      }
      workflows.push(workflow);
    } catch {
      // Ignore unrelated files. Runtime writes are atomic.
    }
  }

  return workflows
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
    .slice(0, 100);
}
