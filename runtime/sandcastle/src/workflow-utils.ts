import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import type { TechStack } from "./stack-detector.js";
import type {
  WorkflowMode,
  WorkflowModeSource,
} from "./issue-classifier.js";

export interface CliOptions {
  issueNum: string;
  requestedRepo: string | null;
  modeOverride: WorkflowMode | null;
}

export const FULL_WORKFLOW_STEPS = [
  "issue-analysis",
  "repository-scout",
  "architecture",
  "tests",
  "implementation",
  "verification",
  "adversarial-review",
  "cleanup",
  "delivery",
  "report",
  "push",
  "pr",
  "review",
  "publish-review",
] as const;

export const LEAN_WORKFLOW_STEPS = [
  "lean-planning",
  "lean-implementation",
  "lean-review",
  "verification",
  "cleanup",
  "delivery",
  "report",
  "push",
  "pr",
  "review",
  "publish-review",
] as const;

const LEGACY_WORKFLOW_STEPS = [
  "planner",
  "tdd",
  "implementation",
  "review",
  "cleanup",
  "git",
  "push",
  "pr",
] as const;

export type WorkflowStep =
  | (typeof FULL_WORKFLOW_STEPS)[number]
  | (typeof LEAN_WORKFLOW_STEPS)[number];

export interface WorkflowState {
  version: 5;
  repo: string;
  issueNum: string;
  issueTitle: string;
  branchName: string;
  baseCommit: string;
  completedSteps: WorkflowStep[];
  prUrl?: string;
  reviewCyclesCompleted: number;
  approved: boolean;
  finalVerdict?: string;
  mode: WorkflowMode;
  modeReason: string;
  modeSource: WorkflowModeSource;
}

interface HerdrPaneResponse {
  result?: {
    pane?: {
      pane_id?: string;
    };
  };
}

export type CommandRunner = (
  command: string,
  args: string[],
  options?: { cwd?: string; env?: NodeJS.ProcessEnv },
) => string;

export type AgentBackend = "opencode" | "pi";

export interface AgentLaunchConfig {
  backend: AgentBackend;
  kind: AgentBackend;
  label: string;
  args: string[];
  needsLmStudioEnv: boolean;
}

const DEFAULT_PI_PROVIDER = "lm-studio";
const DEFAULT_PI_MODEL = "qwen/qwen3.5-9b";
const DEFAULT_OPENCODE_MODEL = "lmstudio/qwen/qwen3.5-9b";
export const PI_COMPACTION_GUARD_PATH = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "pi-compaction-guard.js",
);
const PI_CONTINUITY_PROMPT =
  "Agent-flow continuity contract: context compaction is lossy. After any " +
  "compaction, follow the injected recovery message, treat the exact original " +
  "assignment and filesystem/Git state as authoritative, re-read durable " +
  "handoffs, and verify remaining acceptance criteria before continuing.";
const REPO_PATTERN = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;
const ISSUE_PATTERN = /^[1-9][0-9]*$/;

export function getPiAgentArgs(
  env: NodeJS.ProcessEnv = process.env,
): string[] {
  return [
    "--",
    "--provider",
    env.AGENT_FLOW_PI_PROVIDER ?? DEFAULT_PI_PROVIDER,
    "--model",
    env.AGENT_FLOW_PI_MODEL ?? DEFAULT_PI_MODEL,
    "--no-extensions",
    "--extension",
    PI_COMPACTION_GUARD_PATH,
    "--append-system-prompt",
    PI_CONTINUITY_PROMPT,
    "--tools",
    "read,bash,edit,write",
  ];
}

export function getAgentLaunchConfig(
  env: NodeJS.ProcessEnv = process.env,
): AgentLaunchConfig {
  const backend = env.AGENT_FLOW_AGENT_BACKEND ?? "opencode";
  if (backend === "pi") {
    const provider = env.AGENT_FLOW_PI_PROVIDER ?? DEFAULT_PI_PROVIDER;
    return {
      backend,
      kind: "pi",
      label:
        provider === "lm-studio"
          ? "Pi + LM Studio"
          : provider === "bonsai2"
            ? "Pi + Bonsai"
            : `Pi + ${provider}`,
      args: getPiAgentArgs(env),
      needsLmStudioEnv: provider === "lm-studio",
    };
  }
  if (backend !== "opencode") {
    throw new Error(
      `Unsupported AGENT_FLOW_AGENT_BACKEND '${backend}'; expected 'opencode' or 'pi'.`,
    );
  }
  const model = env.AGENT_FLOW_OPENCODE_MODEL ?? DEFAULT_OPENCODE_MODEL;
  return {
    backend,
    kind: "opencode",
    label: model.startsWith("lmstudio/")
      ? "OpenCode + LM Studio"
      : `OpenCode (${model})`,
    args: [
      "--",
      "--pure",
      "--model",
      model,
      "--agent",
      env.AGENT_FLOW_OPENCODE_AGENT ?? "build",
      "--auto",
    ],
    needsLmStudioEnv: false,
  };
}

export function getVerificationCommand(
  stack: TechStack,
): { command: string; args: string[] } {
  switch (stack) {
    case "GO":
      return { command: "go", args: ["test", "./..."] };
    case "PYTHON":
      return { command: "python", args: ["-m", "pytest"] };
    case "TYPESCRIPT":
      return { command: "npm", args: ["test"] };
  }
}

export function parseCliArgs(args: string[]): CliOptions {
  const leanCount = args.filter((arg) => arg === "--lean").length;
  const fullCount = args.filter((arg) => arg === "--full").length;
  if (leanCount > 0 && fullCount > 0) {
    throw new Error("Conflicting workflow flags: choose either --lean or --full.\n" + usage());
  }
  if (leanCount > 1 || fullCount > 1) throw new Error(usage());

  const positional = args.filter((arg) => arg !== "--lean" && arg !== "--full");
  if (positional.some((arg) => arg.startsWith("--"))) throw new Error(usage());
  const modeOverride: WorkflowMode | null =
    leanCount === 1 ? "lean" : fullCount === 1 ? "full" : null;

  if (positional.length === 1 && ISSUE_PATTERN.test(positional[0])) {
    return { requestedRepo: null, issueNum: positional[0], modeOverride };
  }
  if (
    positional.length === 2 &&
    REPO_PATTERN.test(positional[0]) &&
    ISSUE_PATTERN.test(positional[1])
  ) {
    return {
      requestedRepo: positional[0],
      issueNum: positional[1],
      modeOverride,
    };
  }
  throw new Error(usage());
}

export function assertModeOverrideCompatible(
  state: WorkflowState,
  modeOverride: WorkflowMode | null,
): void {
  if (modeOverride && state.mode !== modeOverride) {
    throw new Error(
      `Cannot resume ${state.repo}#${state.issueNum} with --${modeOverride}: ` +
      `the checkpoint uses ${state.mode} mode (${state.modeReason}).`,
    );
  }
}

function usage(): string {
  return "Usage: imp <issue_number> [--lean|--full] OR imp <owner/repo> <issue_number> [--lean|--full] (agent-flow is an alias)";
}

export function slugifyIssueTitle(title: string): string {
  const slug = title
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60)
    .replace(/-+$/g, "");

  return slug || "issue";
}

export const runCommand: CommandRunner = (
  command,
  args,
  options = {},
): string => {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env,
    encoding: "utf8",
    maxBuffer: 10 * 1024 * 1024,
  });

  if (result.error) {
    throw new Error(`Unable to run ${command}: ${result.error.message}`);
  }
  if (result.status !== 0) {
    const detail = (result.stderr || result.stdout).trim();
    throw new Error(
      `${command} exited with status ${result.status}${detail ? `: ${detail}` : ""}`,
    );
  }
  return result.stdout.trim();
};

export function synchronizeDefaultBranch(
  repoRoot: string,
  runner: CommandRunner = runCommand,
): string {
  const status = runner(
    "git",
    ["status", "--porcelain", "--untracked-files=normal"],
    { cwd: repoRoot },
  )
    .split("\n")
    .filter((line) => line && line !== "?? .sandcastle/")
    .join("\n");
  if (status) {
    throw new Error(
      `The checkout must be clean before synchronizing the default branch:\n${status}`,
    );
  }

  const defaultBranch = runner(
    "gh",
    [
      "repo",
      "view",
      "--json",
      "defaultBranchRef",
      "--jq",
      ".defaultBranchRef.name",
    ],
    { cwd: repoRoot },
  );
  if (!defaultBranch) {
    throw new Error("Unable to determine the repository's default branch.");
  }

  const currentBranch = runner("git", ["branch", "--show-current"], {
    cwd: repoRoot,
  });
  if (currentBranch !== defaultBranch) {
    throw new Error(
      `imp must start from the default branch '${defaultBranch}', but '${currentBranch || "detached HEAD"}' is checked out.`,
    );
  }

  runner("git", ["pull", "--ff-only", "origin", defaultBranch], {
    cwd: repoRoot,
  });
  return defaultBranch;
}

export function getWorkflowStatePath(
  repoRoot: string,
  issueNum: string,
): string {
  const gitCommonDir = runCommand(
    "git",
    ["rev-parse", "--git-common-dir"],
    { cwd: repoRoot },
  );
  return path.join(
    path.resolve(repoRoot, gitCommonDir),
    "agent-flow",
    `issue-${issueNum}.json`,
  );
}

export function loadWorkflowState(statePath: string): WorkflowState | null {
  if (!fs.existsSync(statePath)) return null;

  let value: unknown;
  try {
    value = JSON.parse(fs.readFileSync(statePath, "utf8"));
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to read workflow checkpoint ${statePath}: ${detail}`);
  }

  if (!value || typeof value !== "object") {
    throw new Error(`Invalid workflow checkpoint: ${statePath}`);
  }

  const candidate = value as Partial<Omit<WorkflowState, "version">> & {
    version?: number;
  };
  const completedSteps = candidate.completedSteps;
  if (
    ![1, 2, 3, 4, 5].includes(candidate.version ?? 0) ||
    typeof candidate.repo !== "string" ||
    typeof candidate.issueNum !== "string" ||
    typeof candidate.issueTitle !== "string" ||
    typeof candidate.branchName !== "string" ||
    typeof candidate.baseCommit !== "string" ||
    !Array.isArray(completedSteps) ||
    (candidate.prUrl !== undefined && typeof candidate.prUrl !== "string")
  ) {
    throw new Error(`Invalid workflow checkpoint: ${statePath}`);
  }

  if (candidate.version === 5) {
    if (
      !["lean", "full"].includes(candidate.mode ?? "") ||
      typeof candidate.modeReason !== "string" ||
      !["automatic", "explicit", "migration"].includes(candidate.modeSource ?? "")
    ) {
      throw new Error(`Invalid workflow checkpoint: ${statePath}`);
    }
    const expectedSteps =
      candidate.mode === "lean" ? LEAN_WORKFLOW_STEPS : FULL_WORKFLOW_STEPS;
    const isModePrefix = completedSteps.every(
      (step, index) => step === expectedSteps[index],
    );
    if (!isModePrefix) {
      throw new Error(`Invalid workflow checkpoint: ${statePath}`);
    }
    return candidate as WorkflowState;
  }

  if (candidate.version === 4) {
    if (
      !["lean", "full"].includes(candidate.mode ?? "") ||
      typeof candidate.modeReason !== "string" ||
      !["automatic", "explicit", "migration"].includes(candidate.modeSource ?? "")
    ) {
      throw new Error(`Invalid workflow checkpoint: ${statePath}`);
    }
    const previousSteps =
      candidate.mode === "lean"
        ? [
            "implementation",
            "verification",
            "delivery",
            "report",
            "push",
            "pr",
            "review",
            "publish-review",
          ]
        : FULL_WORKFLOW_STEPS;
    const isModePrefix = completedSteps.every(
      (step, index) => step === previousSteps[index],
    );
    if (!isModePrefix) {
      throw new Error(`Invalid workflow checkpoint: ${statePath}`);
    }
    return {
      ...(candidate as Omit<WorkflowState, "version" | "completedSteps">),
      version: 5,
      completedSteps:
        candidate.mode === "lean"
          ? []
          : (completedSteps as WorkflowStep[]),
      modeReason:
        candidate.mode === "lean"
          ? `${candidate.modeReason}; migrated version 4 lean checkpoint and conservatively rerunning all three lean stages`
          : candidate.modeReason,
      modeSource:
        candidate.mode === "lean"
          ? "migration"
          : (candidate.modeSource as WorkflowModeSource),
    };
  }

  const isCurrentPrefix = completedSteps.every(
    (step, index) => step === FULL_WORKFLOW_STEPS[index],
  );
  if (candidate.version === 2 || candidate.version === 3) {
    if (!isCurrentPrefix) {
      throw new Error(`Invalid workflow checkpoint: ${statePath}`);
    }
    return {
      ...(candidate as Omit<
        WorkflowState,
        "version" | "mode" | "modeReason" | "modeSource"
      >),
      version: 5,
      reviewCyclesCompleted: candidate.reviewCyclesCompleted ?? 0,
      approved: candidate.approved ?? false,
      mode: "full",
      modeReason: `migrated version ${candidate.version} checkpoint; preserving the legacy full workflow`,
      modeSource: "migration",
    };
  }

  const isLegacyPrefix = completedSteps.every(
    (step, index) => step === LEGACY_WORKFLOW_STEPS[index],
  );
  if (!isLegacyPrefix) {
    throw new Error(`Invalid workflow checkpoint: ${statePath}`);
  }

  const migratedSteps: WorkflowStep[] = [];
  const legacySteps = completedSteps as unknown as string[];
  if (legacySteps.includes("review")) {
    migratedSteps.push(...FULL_WORKFLOW_STEPS.slice(0, 7));
  }
  if (legacySteps.includes("cleanup")) migratedSteps.push("cleanup");
  if (legacySteps.includes("git")) migratedSteps.push("delivery");
  if (legacySteps.includes("push")) migratedSteps.push("push");
  if (legacySteps.includes("pr")) migratedSteps.push("pr");

  return {
    ...(candidate as Omit<
      WorkflowState,
      "version" | "completedSteps" | "reviewCyclesCompleted" | "approved"
    >),
    version: 5,
    completedSteps: migratedSteps,
    reviewCyclesCompleted: 0,
    approved: false,
    mode: "full",
    modeReason: "migrated version 1 checkpoint; preserving the legacy full workflow",
    modeSource: "migration",
  };
}

export function saveWorkflowState(
  statePath: string,
  state: WorkflowState,
): void {
  fs.mkdirSync(path.dirname(statePath), { recursive: true });
  const temporaryPath = `${statePath}.${process.pid}.tmp`;
  fs.writeFileSync(temporaryPath, `${JSON.stringify(state, null, 2)}\n`, {
    mode: 0o600,
  });
  fs.renameSync(temporaryPath, statePath);
}

export function detectRepo(repoRoot: string): string | null {
  try {
    const repo = runCommand(
      "gh",
      ["repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner"],
      { cwd: repoRoot },
    );
    if (repo) return repo;
  } catch {
    // Fall through to parsing the origin URL.
  }

  try {
    const url = runCommand(
      "git",
      ["config", "--get", "remote.origin.url"],
      { cwd: repoRoot },
    );
    const match = url.match(/github\.com[:/]([^/]+\/[^/]+?)(?:\.git)?$/);
    return match?.[1] ?? null;
  } catch {
    return null;
  }
}

export function parsePaneId(output: string): string {
  let response: HerdrPaneResponse;
  try {
    response = JSON.parse(output) as HerdrPaneResponse;
  } catch {
    throw new Error(`Herdr returned invalid JSON: ${output}`);
  }
  const paneId = response.result?.pane?.pane_id;
  if (!paneId) {
    throw new Error(`Herdr did not return a pane ID: ${output}`);
  }
  return paneId;
}

export function assertAgentSettled(output: string, role: string): void {
  let response: unknown;
  try {
    response = JSON.parse(output);
  } catch {
    return;
  }

  const findState = (value: unknown): string | null => {
    if (!value || typeof value !== "object") return null;
    for (const [key, child] of Object.entries(value)) {
      if (
        (key === "state" || key === "status") &&
        typeof child === "string" &&
        ["blocked", "unknown"].includes(child)
      ) {
        return child;
      }
      const nestedState = findState(child);
      if (nestedState) return nestedState;
    }
    return null;
  };

  const state = findState(response);
  if (state) {
    throw new Error(`${role} agent stopped in ${state} state.`);
  }
}

export function requireCleanWorktree(targetDir: string): void {
  const status = runCommand("git", ["status", "--porcelain"], { cwd: targetDir });
  if (status) {
    throw new Error(
      `The agent workflow left uncommitted changes:\n${status}`,
    );
  }
}

export function verifyWorktree(stack: TechStack, targetDir: string): void {
  const verification = getVerificationCommand(stack);
  console.log(
    `\x1b[36m[Validation]\x1b[0m ${verification.command} ${verification.args.join(" ")}`,
  );
  runCommand(verification.command, verification.args, { cwd: targetDir });
  // Strip trailing whitespace from all tracked files before diff --check
  const changedFiles = runCommand(
    "git",
    ["diff", "--name-only", "HEAD"],
    { cwd: targetDir },
  );
  if (changedFiles) {
    for (const file of changedFiles.split("\n").filter(Boolean)) {
      try {
        const absPath = path.join(targetDir, file);
        if (fs.existsSync(absPath) && fs.statSync(absPath).isFile()) {
          const content = fs.readFileSync(absPath, "utf8");
          const cleaned = content.replace(/[ \t]+$/gm, "");
          if (cleaned !== content) {
            fs.writeFileSync(absPath, cleaned);
          }
        }
      } catch {
        // Skip files that can't be read (e.g. binary files)
      }
    }
  }
  runCommand("git", ["diff", "--check"], { cwd: targetDir });
}
