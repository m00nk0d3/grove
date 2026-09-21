import {
  assertAgentSettled,
  getAgentLaunchConfig,
  parsePaneId,
  runCommand,
} from "./workflow-utils.js";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const DEFAULT_LM_STUDIO_URL = "http://127.0.0.1:1234/v1";
const AGENT_TIMEOUT_MS = 1_800_000;
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

export function buildCompletionRetryPrompt(problems: string[]): string {
  return `The previous turn stopped without satisfying these completion requirements:
${problems.map((problem) => `- ${problem}`).join("\n")}

Important reminder: All required artifacts must be written to filesystem paths specified in the original assignment.
If you're using OpenCode (API-based agent), write each artifact using bash commands like:
  echo '{"key":"value"}' > "/abs/path/to/artifact.json"

The context has been compacted and the exact original assignment re-injected.
Do not restart broad exploration. Reconcile the current worktree and Git state,
finish the remaining acceptance criteria and validation, then write every
required artifact exactly as specified by the original assignment.`;
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
  } = options;
  const launch = getAgentLaunchConfig();
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
    runCommand("herdr", agentArgs);
    if (
      launch.backend === "pi" &&
      !readContinuityEvents(compactionStatusPath).some(
        (event) => event.type === "guard-ready",
      )
    ) {
      throw new Error("Pi started without initializing the compaction guard.");
    }
    const continuityCursor = readContinuityEvents(compactionStatusPath).length;
    const promptOutput = runCommand("herdr", [
      "agent",
      "prompt",
      agentName,
      effectivePrompt,
      "--wait",
      "--timeout",
      String(AGENT_TIMEOUT_MS),
    ]);
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
      runCommand("herdr", [
        "agent",
        "prompt",
        agentName,
        "/compact Preserve the exact assignment, completed work, concrete evidence, validation results, required output paths, and remaining steps.",
      ]);
      waitForPiCompaction(compactionStatusPath, compactionCursor);

      const retryCursor = readContinuityEvents(compactionStatusPath).length;
      const retryOutput = runCommand("herdr", [
        "agent",
        "prompt",
        agentName,
        buildCompletionRetryPrompt(completionProblems),
        "--wait",
        "--timeout",
        String(AGENT_TIMEOUT_MS),
      ]);
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
    } else if (completionProblems.length > 0 && launch.backend === "opencode") {
      console.log(
        `\x1b[33m[Continuity]\x1b[0m ${role} settled without required output; prompting retry.`,
      );
      const retryOutput = runCommand("herdr", [
        "agent",
        "prompt",
        agentName,
        buildCompletionRetryPrompt(completionProblems),
        "--wait",
        "--timeout",
        String(AGENT_TIMEOUT_MS),
      ]);
      assertAgentSettled(retryOutput, `${role} completion retry`);

      // Immediately re-check completion artifacts for OpenCode
      const retryCursor = 0;

      completionProblems = getCompletionProblems(
        completionArtifacts,
        completionValidator,
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
