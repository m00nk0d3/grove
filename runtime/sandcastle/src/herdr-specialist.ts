import {
  assertAgentSettled,
  getAgentLaunchConfig,
  parsePaneId,
  runCommand,
  type AgentBackend,
  type CommandRunner,
} from "./workflow-utils.js";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { ensureClaudeWorkspaceTrust } from "./claude-trust.js";

const DEFAULT_LM_STUDIO_URL = "http://127.0.0.1:1234/v1";
const AGENT_TIMEOUT_MS = 1_800_000;
const AGENT_READY_TIMEOUT_MS = 120_000;
const WAIT_INTERVAL_MS = 100;
const waitBuffer = new Int32Array(new SharedArrayBuffer(4));

interface ContinuityEvent {
  type: string;
  error?: string;
}

function readContinuityEvents(statusPath: string): ContinuityEvent[] {
  if (!fs.existsSync(statusPath)) return [];
  return fs
    .readFileSync(statusPath, "utf8")
    .trim()
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as ContinuityEvent);
}

export function waitForPiAgentSettled(
  statusPath: string,
  timeoutMs = AGENT_TIMEOUT_MS,
  afterEventCount = 0,
): number {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() <= deadline) {
    const events = readContinuityEvents(statusPath).slice(afterEventCount);
    if (events.some((event) => event.type === "agent-settled")) {
      const finalCompactionEvent = [...events]
        .reverse()
        .find((event) =>
          [
            "compaction-started",
            "compaction-completed",
            "compaction-failed",
          ].includes(event.type),
        );
      if (finalCompactionEvent?.type === "compaction-failed") {
        throw new Error(
          `Pi context compaction failed: ${finalCompactionEvent.error ?? "unknown error"}`,
        );
      }
      return events.filter(
        (event) => event.type === "compaction-completed",
      ).length;
    }
    Atomics.wait(waitBuffer, 0, 0, WAIT_INTERVAL_MS);
  }
  throw new Error(
    `Pi did not reach its authoritative settled state within ${timeoutMs}ms.`,
  );
}

function waitForPiCompaction(
  statusPath: string,
  afterEventCount: number,
  timeoutMs = AGENT_TIMEOUT_MS,
): void {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() <= deadline) {
    const compactionEvent = readContinuityEvents(statusPath)
      .slice(afterEventCount)
      .find((event) =>
        ["compaction-completed", "compaction-failed"].includes(event.type),
      );
    if (compactionEvent?.type === "compaction-completed") return;
    if (compactionEvent?.type === "compaction-failed") {
      throw new Error(
        `Pi context compaction failed: ${compactionEvent.error ?? "unknown error"}`,
      );
    }
    Atomics.wait(waitBuffer, 0, 0, WAIT_INTERVAL_MS);
  }
  throw new Error(`Pi did not complete context compaction within ${timeoutMs}ms.`);
}

export function findMissingCompletionArtifacts(paths: string[]): string[] {
  return paths.filter(
    (artifactPath) =>
      !fs.existsSync(artifactPath) ||
      !fs.statSync(artifactPath).isFile() ||
      !fs.readFileSync(artifactPath, "utf8").trim(),
  );
}

function getCompletionProblems(
  artifacts: string[],
  validator?: () => void,
): string[] {
  const missingArtifacts = findMissingCompletionArtifacts(artifacts);
  if (missingArtifacts.length > 0) {
    return missingArtifacts.map(
      (artifactPath) => `Missing or empty artifact: ${artifactPath}`,
    );
  }
  if (validator) {
    try {
      validator();
    } catch (error) {
      return [
        error instanceof Error
          ? `Invalid completion artifact: ${error.message}`
          : `Invalid completion artifact: ${String(error)}`,
      ];
    }
  }
  return [];
}

export function buildCompletionRetryPrompt(
  problems: string[],
  backend: AgentBackend = "opencode",
): string {
  const artifactGuidance =
    backend === "claude"
      ? `Write each artifact with the Write tool, using its absolute path.`
      : `If you're using OpenCode (API-based agent), write each artifact using bash commands like:
  echo '{"key":"value"}' > "/abs/path/to/artifact.json"`;
  return `The previous turn stopped without satisfying these completion requirements:
${problems.map((problem) => `- ${problem}`).join("\n")}

Important reminder: All required artifacts must be written to filesystem paths specified in the original assignment.
${artifactGuidance}

The context has been compacted and the exact original assignment re-injected.
Do not restart broad exploration. Reconcile the current worktree and Git state,
finish the remaining acceptance criteria and validation, then write every
required artifact exactly as specified by the original assignment.`;
}

// `herdr agent start` reports a failure when the agent is still blocked at the
// end of its startup window, which an agent's own splash or notice screen can
// trigger before it settles at an interactive prompt. The agent is usually
// live by then, so give it a bounded chance to reach an idle state and only
// surface the original startup failure if it never gets there.
export function startAgentWithReadinessRecovery(
  agentName: string,
  agentArgs: string[],
  runner: CommandRunner = runCommand,
): void {
  try {
    runner("herdr", agentArgs);
    return;
  } catch (startError) {
    console.log(
      `\x1b[33m[Continuity]\x1b[0m ${agentName} was not ready when startup returned; waiting for it to settle.`,
    );
    try {
      runner("herdr", [
        "agent",
        "wait",
        agentName,
        "--until",
        "idle",
        "--timeout",
        String(AGENT_READY_TIMEOUT_MS),
      ]);
    } catch {
      throw startError;
    }
  }
}

const BLOCKED_PATTERN = /agent_blocked|requires interactive input/i;
const STALLED_PATTERN = /agent_prompt_stalled/i;
const DEFAULT_HUMAN_INPUT_TIMEOUT_MS = 900_000;

// How long to leave a blocked agent waiting for a person before giving up.
// Setting AGENT_FLOW_HUMAN_INPUT_TIMEOUT_MS to 0 restores the previous
// behaviour, where anything needing a person fails the workflow immediately.
function humanInputTimeoutMs(env: NodeJS.ProcessEnv = process.env): number {
  const configured = Number(env.AGENT_FLOW_HUMAN_INPUT_TIMEOUT_MS);
  return Number.isFinite(configured) && configured >= 0
    ? configured
    : DEFAULT_HUMAN_INPUT_TIMEOUT_MS;
}

// An agent sometimes stops for something only a person can give: a permission
// decision, a workspace it has not been trusted with, a judgement call. Herdr
// reports that as `agent_blocked`, which used to end the run outright. Hand the
// pane to the person instead and continue once they have answered.
//
// The retry re-sends the prompt, which is right when the agent was blocked
// before the prompt reached it — the common case, since Herdr refuses to type
// into a blocked agent. An agent that blocks midway through work it already
// accepted will see the instruction twice; that is wasteful but recoverable,
// where failing the stage is not.
export function promptAgent(
  agentName: string,
  promptText: string,
  options: {
    extraArgs?: string[];
    timeoutMs?: number;
    runner?: CommandRunner;
    /** How long the agent's own turn may take, for a stalled submission. */
    settleTimeoutMs?: number;
    /** Reports the wait to Grove so the dashboard can show it. */
    onBlocked?: (detail: string) => void;
    onResumed?: () => void;
  } = {},
): string {
  const runner = options.runner ?? runCommand;
  const args = [
    "agent",
    "prompt",
    agentName,
    promptText,
    ...(options.extraArgs ?? []),
  ];
  try {
    return runner("herdr", args);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);

    // Herdr accepts and delivers the prompt, then requires the agent to be
    // seen working or blocked within a fixed five seconds. A large prompt, or
    // an agent that simply takes a moment to begin, misses that window and is
    // reported as stalled even though the instruction landed. Re-sending would
    // hand the agent the same work twice, so wait out the turn it has already
    // started instead.
    if (STALLED_PATTERN.test(detail)) {
      console.log(
        `\x1b[33m[Continuity]\x1b[0m ${agentName} was not seen starting within Herdr's observation window; ` +
          "the prompt was delivered, so waiting for the turn to finish.",
      );
      return runner("herdr", [
        "agent",
        "wait",
        agentName,
        "--timeout",
        String(options.settleTimeoutMs ?? DEFAULT_HUMAN_INPUT_TIMEOUT_MS),
      ]);
    }

    const waitMs = options.timeoutMs ?? humanInputTimeoutMs();
    if (!BLOCKED_PATTERN.test(detail) || waitMs <= 0) {
      throw error;
    }

    console.log(
      `\x1b[33m[Input Needed]\x1b[0m ${agentName} is waiting for a person. ` +
        "Answer it in its Herdr pane; the workflow resumes on its own once the " +
        `agent is idle again (waiting up to ${Math.round(waitMs / 1000)}s).`,
    );
    options.onBlocked?.(detail);
    try {
      runner("herdr", ["agent", "focus", agentName]);
    } catch {
      // Focusing is a convenience; a pane that cannot be focused is still answerable.
    }

    try {
      runner("herdr", [
        "agent",
        "wait",
        agentName,
        "--until",
        "idle",
        "--timeout",
        String(waitMs),
      ]);
    } catch {
      throw error; // nobody answered: report the original block
    }
    console.log(
      `\x1b[32m[Input Received]\x1b[0m ${agentName} is responsive again; continuing.`,
    );
    options.onResumed?.();
    return runner("herdr", args);
  }
}

interface SpecialistOptions {
  role: string;
  promptText: string;
  targetDir: string;
  issueOrPrNumber: string;
  sessionId?: string;
  completionArtifacts?: string[];
  completionValidator?: () => void;
  onPaneChanged?: (paneId: string | null) => void;
  /**
   * Reports the agent's state so Grove's dashboard can show a stage that is
   * waiting for a person rather than one that merely looks slow.
   */
  onAgentStatus?: (status: "working" | "blocked", summary: string) => void;
}

export function runSpecialistInPane(options: SpecialistOptions): void {
  const {
    role,
    promptText,
    targetDir,
    issueOrPrNumber,
    sessionId,
    completionArtifacts = [],
    completionValidator,
    onPaneChanged,
    onAgentStatus,
  } = options;
  const launch = getAgentLaunchConfig();
  if (launch.backend === "claude") {
    // A fresh worktree is a directory Claude has not seen, and its trust
    // dialog blocks the first prompt rather than the launch.
    const trust = ensureClaudeWorkspaceTrust(targetDir);
    if (trust === "recorded") {
      console.log(
        `\x1b[36m[Workspace]\x1b[0m Recorded Claude workspace trust for ${targetDir}.`,
      );
    } else if (trust === "skipped") {
      console.log(
        `\x1b[33m[Workspace]\x1b[0m Could not pre-record Claude workspace trust for ${targetDir}; ` +
          "the agent may block on the workspace trust dialog.",
      );
    }
  }
  let effectivePrompt =
    `Authoritative working directory: ${targetDir}\n` +
    "Perform all repository inspection, Git commands, validation, and edits " +
    "in that worktree. Do not substitute another checkout.\n" +
    "This step may be resuming after an interruption. Inspect and preserve " +
    "valid existing work, then continue from the current state.\n" +
    promptText;
  if (launch.backend === "opencode") {
    effectivePrompt = effectivePrompt + `

IMPORTANT: All required artifacts must be written to absolute filesystem paths.
For OpenCode agents, use bash commands like: echo '{"..."}' > "/absolute/path/to/file.json"
`;
  } else if (launch.backend === "claude") {
    effectivePrompt = effectivePrompt + `

IMPORTANT: All required artifacts must be written to absolute filesystem paths.
Use the Write tool with the absolute path for each artifact.
`;
  }
  const continuityDir = fs.mkdtempSync(
    path.join(os.tmpdir(), "agent-flow-continuity-"),
  );
  const assignmentPath = path.join(continuityDir, "assignment.md");
  const compactionStatusPath = path.join(continuityDir, "compaction.jsonl");
  fs.writeFileSync(assignmentPath, effectivePrompt, "utf8");
  console.log(`\x1b[35m[Specialist: ${role}]\x1b[0m Running via ${launch.label}...`);
  let paneId: string | null = null;

  try {
    const paneArgs = [
      "pane",
      "split",
      "--current",
      "--direction",
      "right",
      "--cwd",
      targetDir,
    ];
    if (launch.needsLmStudioEnv) {
      paneArgs.push(
        "--env",
        `OPENAI_BASE_URL=${process.env.OPENAI_BASE_URL ?? DEFAULT_LM_STUDIO_URL}`,
        "--env",
        `OPENAI_API_KEY=${process.env.OPENAI_API_KEY ?? "lm-studio"}`,
      );
    }
    if (launch.backend === "pi") {
      paneArgs.push(
        "--env",
        `AGENT_FLOW_ASSIGNMENT_FILE=${assignmentPath}`,
        "--env",
        `AGENT_FLOW_COMPACTION_STATUS_FILE=${compactionStatusPath}`,
      );
    }
    paneArgs.push("--no-focus");
    paneId = parsePaneId(runCommand("herdr", paneArgs));
    onPaneChanged?.(paneId);
    const agentName =
      `af-${role.toLowerCase()}-${issueOrPrNumber}-${process.pid}`.slice(0, 32);

    const agentArgs = [
      "agent",
      "start",
      agentName,
      "--kind",
      launch.kind,
      "--pane",
      paneId,
      ...launch.args,
    ];
    if (launch.backend === "pi" && sessionId) {
      agentArgs.push("--session-id", sessionId);
    }
    startAgentWithReadinessRecovery(agentName, agentArgs);
    if (
      launch.backend === "pi" &&
      !readContinuityEvents(compactionStatusPath).some(
        (event) => event.type === "guard-ready",
      )
    ) {
      throw new Error("Pi started without initializing the compaction guard.");
    }
    const continuityCursor = readContinuityEvents(compactionStatusPath).length;
    const promptOutput = promptAgent(agentName, effectivePrompt, {
      extraArgs: ["--wait", "--timeout", String(AGENT_TIMEOUT_MS)],
      settleTimeoutMs: AGENT_TIMEOUT_MS,
      onBlocked: () =>
        onAgentStatus?.("blocked", `Waiting for input in pane ${paneId ?? "unknown"}`),
      onResumed: () => onAgentStatus?.("working", `Executing ${role} stage`),
      });
    assertAgentSettled(promptOutput, role);
    if (launch.backend === "pi") {
      const completed = waitForPiAgentSettled(
        compactionStatusPath,
        AGENT_TIMEOUT_MS,
        continuityCursor,
      );
      if (completed > 0) {
        console.log(
          `\x1b[36m[Continuity]\x1b[0m ${role} recovered from ${completed} context compaction${completed === 1 ? "" : "s"}.`,
        );
      }
    }
    let completionProblems = getCompletionProblems(
      completionArtifacts,
      completionValidator,
    );
    if (completionProblems.length > 0 && launch.backend === "pi") {
      console.log(
        `\x1b[33m[Continuity]\x1b[0m ${role} settled without required output; compacting and retrying completion.`,
      );
      const compactionCursor = readContinuityEvents(compactionStatusPath).length;
      promptAgent(
        agentName,
        "/compact Preserve the exact assignment, completed work, concrete evidence, validation results, required output paths, and remaining steps.",
      );
      waitForPiCompaction(compactionStatusPath, compactionCursor);

      const retryCursor = readContinuityEvents(compactionStatusPath).length;
      const retryOutput = promptAgent(agentName, buildCompletionRetryPrompt(completionProblems, launch.backend), {
        extraArgs: ["--wait", "--timeout", String(AGENT_TIMEOUT_MS)],
      settleTimeoutMs: AGENT_TIMEOUT_MS,
        onBlocked: () =>
          onAgentStatus?.("blocked", `Waiting for input in pane ${paneId ?? "unknown"}`),
        onResumed: () => onAgentStatus?.("working", `Executing ${role} stage`),
        });
      assertAgentSettled(retryOutput, `${role} completion retry`);
      waitForPiAgentSettled(
        compactionStatusPath,
        AGENT_TIMEOUT_MS,
        retryCursor,
      );
      completionProblems = getCompletionProblems(
        completionArtifacts,
        completionValidator,
      );
    } else if (completionProblems.length > 0 && launch.backend !== "pi") {
      console.log(
        `\x1b[33m[Continuity]\x1b[0m ${role} settled without required output; prompting retry.`,
      );
      const retryOutput = promptAgent(agentName, buildCompletionRetryPrompt(completionProblems, launch.backend), {
        extraArgs: ["--wait", "--timeout", String(AGENT_TIMEOUT_MS)],
      settleTimeoutMs: AGENT_TIMEOUT_MS,
        onBlocked: () =>
          onAgentStatus?.("blocked", `Waiting for input in pane ${paneId ?? "unknown"}`),
        onResumed: () => onAgentStatus?.("working", `Executing ${role} stage`),
        });
      assertAgentSettled(retryOutput, `${role} completion retry`);

      // Immediately re-check completion artifacts for non-Pi backends
      const retryCursor = 0;

      completionProblems = getCompletionProblems(
        completionArtifacts,
        completionValidator,
      );
    } else if (completionProblems.length > 0) {
      console.log(
        `\x1b[33m[Continuity]\x1b[0m ${role} settled without required output; throwing error.`,
      );
      // Unreachable for known backends; retained as a safety net
      throw new Error(
        `${role} did not satisfy completion requirements:\n${completionProblems.join("\n")}`,
      );
    }
    if (completionProblems.length > 0) {
      throw new Error(
        `${role} did not satisfy completion requirements:\n${completionProblems.join("\n")}`,
      );
    }
  } finally {
    try {
      if (paneId) {
        runCommand("herdr", ["pane", "close", paneId]);
        onPaneChanged?.(null);
      }
    } finally {
      fs.rmSync(continuityDir, { recursive: true, force: true });
    }
  }
}
