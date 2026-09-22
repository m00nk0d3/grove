import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  detectStackProjects,
  findDotNetProject,
  type StackProject,
  type TechStack,
} from "./stack-detector.js";
import type {
  WorkflowMode,
  WorkflowModeSource,
} from "./issue-classifier.js";

export interface CliOptions {
  issueNum: string;
  requestedRepo: string | null;
  modeOverride: WorkflowMode | null;
}

// Steps are agent stages first and bookkeeping second. Git and filesystem work
// that costs nothing is folded into the stage it belongs to rather than shown
// as a workflow stage of its own.
export const FULL_WORKFLOW_STEPS = [
  "planning",
  "tests",
  "implementation",
  "verification",
  "domain-review",
  "documentation",
  "delivery",
  "report",
  "publish",
  "review",
] as const;

export const LEAN_WORKFLOW_STEPS = [
  "lean-planning",
  "lean-implementation",
  "lean-review",
  "verification",
  "domain-review",
  "documentation",
  "delivery",
  "report",
  "publish",
  "review",
] as const;

export type WorkflowStep =
  | (typeof FULL_WORKFLOW_STEPS)[number]
  | (typeof LEAN_WORKFLOW_STEPS)[number];

export interface WorkflowState {
  version: 7;
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

export type AgentBackend = "opencode" | "pi" | "claude";

export const AGENT_BACKENDS: readonly AgentBackend[] = [
  "opencode",
  "pi",
  "claude",
];

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
const DEFAULT_CLAUDE_PERMISSION_MODE = "acceptEdits";
// PowerShell is a separate tool from Bash on Windows, and a specialist working in
// a .NET or Windows repository reaches for it unprompted. Leaving it out does not
// stop the agent using it — it makes every call wait for a person, which surfaces
// as agent_blocked and ends the workflow.
const DEFAULT_CLAUDE_TOOLS = [
  "Read",
  "Write",
  "Edit",
  "Bash",
  "PowerShell",
  "Glob",
  "Grep",
];
export const PI_COMPACTION_GUARD_PATH = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "pi-compaction-guard.js",
);
const AGENT_CONTINUITY_PROMPT =
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
    AGENT_CONTINUITY_PROMPT,
    "--tools",
    "read,bash,edit,write",
  ];
}

export function getClaudeAgentArgs(
  env: NodeJS.ProcessEnv = process.env,
): string[] {
  const tools = (env.AGENT_FLOW_CLAUDE_TOOLS ?? "")
    .split(",")
    .map((tool) => tool.trim())
    .filter((tool) => tool.length > 0);
  const args = ["--"];
  if (env.AGENT_FLOW_CLAUDE_MODEL) {
    args.push("--model", env.AGENT_FLOW_CLAUDE_MODEL);
  }
  args.push(
    "--permission-mode",
    env.AGENT_FLOW_CLAUDE_PERMISSION_MODE ?? DEFAULT_CLAUDE_PERMISSION_MODE,
    "--append-system-prompt",
    AGENT_CONTINUITY_PROMPT,
  );
  // --allowedTools is variadic, so it must stay last in the argument list.
  args.push(
    "--allowedTools",
    ...(tools.length > 0 ? tools : DEFAULT_CLAUDE_TOOLS),
  );
  return args;
}

export function isAgentBackend(value: string): value is AgentBackend {
  return (AGENT_BACKENDS as readonly string[]).includes(value);
}

function unsupportedBackendMessage(value: string): string {
  const expected = AGENT_BACKENDS.map((backend) => `'${backend}'`).join(", ");
  return `Unsupported AGENT_FLOW_AGENT_BACKEND '${value}'; expected one of ${expected}.`;
}

// resolveAgentBackend reports the configured backend without building a full
// launch config, for callers that only need to label telemetry.
export function resolveAgentBackend(
  env: NodeJS.ProcessEnv = process.env,
): AgentBackend {
  const backend = env.AGENT_FLOW_AGENT_BACKEND ?? "opencode";
  if (!isAgentBackend(backend)) {
    throw new Error(unsupportedBackendMessage(backend));
  }
  return backend;
}

export function getAgentLaunchConfig(
  env: NodeJS.ProcessEnv = process.env,
): AgentLaunchConfig {
  const backend = resolveAgentBackend(env);
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
  if (backend === "claude") {
    const model = env.AGENT_FLOW_CLAUDE_MODEL;
    return {
      backend,
      kind: "claude",
      label: model ? `Claude Code (${model})` : "Claude Code",
      args: getClaudeAgentArgs(env),
      needsLmStudioEnv: false,
    };
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

export interface VerificationTask {
  stack: TechStack;
  command: string;
  args: string[];
  /** Directory to run in, relative to the repository root ("" = root). */
  root: string;
  label: string;
}

function taskFor(project: StackProject): VerificationTask {
  const base = getVerificationCommand(project.stack);
  const args =
    project.stack === "CSHARP"
      ? ["test", project.marker.slice(project.marker.lastIndexOf("/") + 1)]
      : base.args;
  const where = project.root ? ` (in ${project.root})` : "";
  return {
    stack: project.stack,
    command: base.command,
    args,
    root: project.root,
    label: `${base.command} ${args.join(" ")}${where}`,
  };
}

// planVerification chooses which projects a change has to be validated against.
// Running every project in a multi-stack repository wastes time and can fail on
// an unrelated pre-existing break, while running only one silently skips the
// stack that actually changed.
export function planVerification(
  projects: StackProject[],
  changedFiles: string[],
): VerificationTask[] {
  if (projects.length === 0) {
    // No recognised project: preserve the historical repository-root default.
    const fallback = getVerificationCommand("TYPESCRIPT");
    return [
      {
        stack: "TYPESCRIPT",
        command: fallback.command,
        args: fallback.args,
        root: "",
        label: `${fallback.command} ${fallback.args.join(" ")}`,
      },
    ];
  }

  const ownerOf = (file: string): StackProject | undefined => {
    const normalized = file.replace(/\\/g, "/");
    let owner: StackProject | undefined;
    for (const project of projects) {
      const matches =
        project.root === "" || normalized.startsWith(`${project.root}/`);
      // The deepest matching root wins, so frontend/ beats a root project.
      if (matches && (!owner || project.root.length > owner.root.length)) {
        owner = project;
      }
    }
    return owner;
  };

  const affected = new Set<StackProject>();
  for (const file of changedFiles) {
    const owner = ownerOf(file);
    if (owner) {
      affected.add(owner);
    }
  }

  // Nothing attributable — a root-level config or an unknown change — means the
  // safe answer is to validate everything rather than guess.
  const selected =
    affected.size > 0
      ? projects.filter((project) => affected.has(project))
      : projects;

  return selected.map(taskFor);
}

export function getVerificationCommand(
  stack: TechStack,
  targetDir?: string,
): { command: string; args: string[] } {
  switch (stack) {
    case "GO":
      return { command: "go", args: ["test", "./..."] };
    case "PYTHON":
      return { command: "python", args: ["-m", "pytest"] };
    case "TYPESCRIPT":
      return { command: "npm", args: ["test"] };
    case "CSHARP": {
      // `dotnet test` resolves a project from the working directory, so a
      // solution kept below the repository root (backend/App.sln) has to be
      // named explicitly or the command fails with MSB1003.
      const project = targetDir ? findDotNetProject(targetDir) : undefined;
      return { command: "dotnet", args: project ? ["test", project] : ["test"] };
    }
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

// A worktree repeats its branch slug in its directory name, so every character
// here is charged against Windows' 260-character path limit for each file in
// the checkout. Keep the slug short, and cut it on a word boundary so the
// branch still reads as something a human chose.
const DEFAULT_SLUG_MAX_LENGTH = 40;

export function slugifyIssueTitle(
  title: string,
  maxLength: number = DEFAULT_SLUG_MAX_LENGTH,
): string {
  const normalized = title
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

  if (normalized.length <= maxLength) {
    return normalized || "issue";
  }

  let slug = "";
  for (const word of normalized.split("-").filter(Boolean)) {
    const candidate = slug ? `${slug}-${word}` : word;
    if (candidate.length > maxLength) {
      break;
    }
    slug = candidate;
  }

  // A first word longer than the budget cannot be kept whole.
  if (!slug) {
    slug = normalized.slice(0, maxLength).replace(/-+$/g, "");
  }

  return slug || "issue";
}

// Windows limits most paths to 260 characters unless long paths are enabled in
// Git. Git reports the failure once it is already part-way through the
// checkout, naming only the files it could not write, so warn before creating
// the worktree where the cause is still obvious.
export function warnIfWindowsPathLimitLikely(
  repoRoot: string,
  runner: CommandRunner = runCommand,
): void {
  if (process.platform !== "win32") {
    return;
  }
  let configured = "";
  try {
    configured = runner("git", ["config", "--get", "core.longpaths"], {
      cwd: repoRoot,
    });
  } catch {
    configured = ""; // unset: git exits non-zero
  }
  if (configured.trim().toLowerCase() === "true") {
    return;
  }
  console.log(
    "\x1b[33m[Git]\x1b[0m core.longpaths is not enabled. Checking out deeply " +
      "nested files can fail on the Windows 260-character path limit. " +
      "Enable it with: git config --global core.longpaths true",
  );
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
    ![1, 2, 3, 4, 5, 6, 7].includes(candidate.version ?? 0) ||
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

  const hasModeFields =
    ["lean", "full"].includes(candidate.mode ?? "") &&
    typeof candidate.modeReason === "string" &&
    ["automatic", "explicit", "migration"].includes(candidate.modeSource ?? "");

  if (candidate.version === 7) {
    if (!hasModeFields) {
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

  // Versions 1-6 predate the current step list: planning was three separate
  // stages, and Git bookkeeping had stages of its own. Rather than guess how a
  // retired stage maps onto the current one, keep the completed steps that
  // still line up from the start and rerun the rest. Every stage is safe to
  // repeat, so rerunning costs time rather than correctness.
  const mode: WorkflowMode = candidate.mode === "lean" ? "lean" : "full";
  const expectedSteps =
    mode === "lean" ? LEAN_WORKFLOW_STEPS : FULL_WORKFLOW_STEPS;
  const retained: WorkflowStep[] = [];
  for (const step of completedSteps as unknown as string[]) {
    if (step !== expectedSteps[retained.length]) break;
    retained.push(step as WorkflowStep);
  }
  const resumePoint = retained.length > 0 ? retained[retained.length - 1] : "the start";

  return {
    ...(candidate as Omit<
      WorkflowState,
      | "version"
      | "completedSteps"
      | "reviewCyclesCompleted"
      | "approved"
      | "mode"
      | "modeReason"
      | "modeSource"
    >),
    version: 7,
    completedSteps: retained,
    reviewCyclesCompleted: candidate.reviewCyclesCompleted ?? 0,
    approved: candidate.approved ?? false,
    mode,
    modeReason: `migrated version ${candidate.version} checkpoint; rerunning stages after ${resumePoint}`,
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

// The workflow validates before committing, so the change set is whatever the
// worktree currently holds: edits to tracked files plus new untracked ones.
function collectTrackedChanges(targetDir: string): string[] {
  const files: string[] = [];
  for (const args of [
    ["diff", "--name-only", "HEAD"],
    ["ls-files", "--others", "--exclude-standard"],
  ]) {
    try {
      files.push(
        ...runCommand("git", args, { cwd: targetDir })
          .split("\n")
          .map((line) => line.trim())
          .filter(Boolean),
      );
    } catch {
      // No HEAD or an unreadable index simply yields no attribution, which
      // makes planVerification validate every project.
    }
  }
  return files;
}

// A git worktree is populated from tracked files alone, so a JavaScript project
// checked out for a workflow has no node_modules and its test command fails
// before it runs a single test. Install them once, honouring a lockfile when the
// project has one.
export function ensureJavaScriptDependencies(
  projectDir: string,
  runner: CommandRunner = runCommand,
): "present" | "installed" {
  if (fs.existsSync(path.join(projectDir, "node_modules"))) {
    return "present";
  }
  const locked = ["package-lock.json", "npm-shrinkwrap.json"].some((lockfile) =>
    fs.existsSync(path.join(projectDir, lockfile)),
  );
  runner("npm", locked ? ["ci"] : ["install"], { cwd: projectDir });
  return "installed";
}

const FAILURE_LINE =
  /\b(error|errors|fail|fails|failed|failing|failure|failures|assert)\b/i;
const MAX_FAILURE_CHARS = 4000;
const FAILURE_TAIL_LINES = 20;
const FAILURE_CONTEXT_LINES = 3;
const LEADING_NAMED_LINES = 30;

// A failing test command reports through its whole transcript: a restore log,
// every compiler warning, and only then the failure. Handing all of it to an
// agent buries the one thing it has to act on, so a transcript past a readable
// size is reduced to the lines that name a failure. When nothing names one, the
// end of the output is the only signal there is.
export function summarizeCommandFailure(
  label: string,
  status: number | null,
  output: string,
): string {
  const header = `${label} failed with exit code ${status ?? "unknown"}`;
  const trimmed = output.trim();
  if (!trimmed) {
    return `${header}. The command produced no output.`;
  }
  if (trimmed.length <= MAX_FAILURE_CHARS) {
    return `${header}:\n${trimmed}`;
  }
  const lines = trimmed.split(/\r?\n/);
  // A line naming a failure rarely carries the detail: an assertion prints its
  // expected and actual values on the lines that follow, so they come along.
  const keep = new Set<number>();
  lines.forEach((line, index) => {
    if (!FAILURE_LINE.test(line)) return;
    for (let offset = 0; offset <= FAILURE_CONTEXT_LINES; offset += 1) {
      if (index + offset < lines.length) keep.add(index + offset);
    }
  });
  if (keep.size === 0) {
    return cap(
      `${header}, showing the end of its output:\n` +
        lines.slice(-FAILURE_TAIL_LINES).join("\n"),
    );
  }
  const selected = [...keep].sort((a, b) => a - b);
  const kept: string[] = [];
  let previous: number | undefined;
  for (const index of selected) {
    if (previous !== undefined && index > previous + 1) {
      kept.push("...");
    }
    kept.push(lines[index]);
    previous = index;
  }
  return cap(
    `${header}, showing the lines that name a failure:\n${kept.join("\n")}`,
  );
}

function cap(summary: string): string {
  return summary.length <= MAX_FAILURE_CHARS
    ? summary
    : `${summary.slice(0, MAX_FAILURE_CHARS)}\n... (output truncated)`;
}

// Verification commands are run here rather than through runCommand so the
// transcript survives long enough to be summarized; runCommand folds it into an
// error message whole.
function runVerificationTask(task: VerificationTask, cwd: string): void {
  const result = spawnSync(task.command, task.args, {
    cwd,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.error) {
    throw new Error(`Unable to run ${task.label}: ${result.error.message}`);
  }
  if (result.status !== 0) {
    throw new Error(
      summarizeCommandFailure(
        task.label,
        result.status,
        `${result.stdout ?? ""}\n${result.stderr ?? ""}`,
      ),
    );
  }
}

export function verifyWorktree(targetDir: string, touched?: string[]): void {
  const tasks = planVerification(
    detectStackProjects(targetDir),
    touched ?? collectTrackedChanges(targetDir),
  );
  for (const task of tasks) {
    const cwd = task.root ? path.join(targetDir, task.root) : targetDir;
    if (
      task.stack === "TYPESCRIPT" &&
      ensureJavaScriptDependencies(cwd) === "installed"
    ) {
      console.log(
        `\x1b[32m[Validation]\x1b[0m Installed JavaScript dependencies in ${task.root || "."}`,
      );
    }
    console.log(`\x1b[36m[Validation]\x1b[0m ${task.label}`);
    runVerificationTask(task, cwd);
  }
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
