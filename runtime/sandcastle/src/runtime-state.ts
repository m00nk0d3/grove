import { randomUUID } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

export type WorkflowKind = "imp" | "review" | "resolve" | "ci" | "clean";
export type WorkflowStatus = "queued" | "running" | "blocked" | "succeeded" | "failed";

export interface RuntimeStep {
  id: string;
  title: string;
  status: string;
  summary?: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
}

export interface RuntimeAgent {
  id: string;
  kind: string;
  name: string;
  status: string;
  summary: string;
  pane_id: string | null;
}

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
  agents: RuntimeAgent[];
  steps: RuntimeStep[];
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

export function removeWorkflow(
  cwd: string,
  id: string,
  stop: boolean,
): { removed: string; stopped: boolean } {
  if (!/^[A-Za-z0-9_-]+$/.test(id)) {
    throw new Error(`Invalid workflow run ID: ${id}`);
  }
  const destination = path.join(workflowStateDir(cwd), `${id}.json`);
  if (!fs.existsSync(destination)) {
    throw new Error(`Unknown workflow run: ${id}`);
  }

  const workflow = JSON.parse(
    fs.readFileSync(destination, "utf8"),
  ) as RuntimeWorkflow;
  const active = ["queued", "running", "blocked"].includes(workflow.status);
  if (active && !stop) {
    throw new Error(`Workflow ${id} is active; confirm stop before removal`);
  }

  let stopped = false;
  if (stop && workflow.pid !== null) {
    if (!processOwnsWorkflow(workflow.pid, id)) {
      throw new Error(
        `Refusing to stop PID ${workflow.pid}: it is not owned by workflow ${id}`,
      );
    }
    for (const agent of workflow.agents ?? []) {
      if (!agent.pane_id) continue;
      const closed = spawnSync("herdr", ["pane", "close", agent.pane_id], {
        encoding: "utf8",
      });
      if (closed.status !== 0) {
        throw new Error(
          (closed.stderr || closed.stdout).trim() ||
            `Unable to close Herdr pane ${agent.pane_id}`,
        );
      }
    }
    try {
      process.kill(workflow.pid, "SIGTERM");
      stopped = true;
    } catch (error) {
      const code =
        error !== null && typeof error === "object" && "code" in error
          ? String(error.code)
          : "";
      if (code !== "ESRCH") throw error;
    }
  }
  fs.rmSync(destination);
  return { removed: id, stopped };
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
  workflow.steps[0].started_at = workflow.updated_at;
  workflow.updated_at = new Date().toISOString();
  activeWorkflow = { cwd, workflow };
  writeWorkflow(cwd, workflow);

  try {
    await run(args);
    workflow.status = "succeeded";
    workflow.current_step = "Complete";
    workflow.progress = { completed: 1, total: 1, percent: 100 };
    const activeStep =
      workflow.steps.find((step) => step.status === "running") ??
      (workflow.steps.length === 1 ? workflow.steps[0] : undefined);
    if (activeStep) {
      activeStep.status = "succeeded";
      activeStep.completed_at = new Date().toISOString();
      if (activeStep.started_at) {
        activeStep.duration_ms =
          Date.parse(activeStep.completed_at) - Date.parse(activeStep.started_at);
      }
    }
  } catch (error) {
    workflow.status = "failed";
    workflow.current_step = "Failed";
    const activeStep =
      workflow.steps.find((step) => step.status === "running") ??
      (workflow.steps.length === 1 ? workflow.steps[0] : undefined);
    if (activeStep) {
      activeStep.status = "failed";
      activeStep.completed_at = new Date().toISOString();
      if (activeStep.started_at) {
        activeStep.duration_ms =
          Date.parse(activeStep.completed_at) - Date.parse(activeStep.started_at);
      }
    }
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

function processOwnsWorkflow(pid: number, id: string): boolean {
  const marker = `GROVE_WORKFLOW_RUN_ID=${id}`;
  try {
    const environment = fs.readFileSync(`/proc/${pid}/environ`, "utf8");
    if (environment.split("\0").includes(marker)) return true;
  } catch {
    // Fall through to the portable process-list check.
  }
  const processInfo = spawnSync("ps", ["eww", "-p", String(pid), "-o", "command="], {
    encoding: "utf8",
  });
  return processInfo.status === 0 && processInfo.stdout.includes(marker);
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
