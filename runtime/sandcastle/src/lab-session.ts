#!/usr/bin/env node

// A Lab session: an agent working through a Lab entry in a Herdr pane, with
// the user answering it in Grove. The agent asks through question cards;
// Grove writes the answers; this process delivers them to the agent, reports
// the session's state to Grove, and closes the pane when Grove says the
// session is over. See docs/LAB_DESIGN.md.

import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { ensureClaudeWorkspaceTrust } from "./claude-trust.js";
import {
  deliverablePrompt,
  startAgentWithReadinessRecovery,
} from "./herdr-specialist.js";
import {
  buildAnswersPrompt,
  buildInterviewSoFar,
  buildProtocolSection,
  buildRepairPrompt,
  buildRequestPrompt,
  cardFileName,
  isPending,
  needsDelivery,
  nextQuestionNumber,
  scanQuestions,
  scanRequests,
  writeReceipt,
  type QuestionRecord,
  type Receipt,
  type RequestRecord,
} from "./lab-protocol.js";
import {
  runTrackedWorkflow,
  updateTrackedWorkflow,
  type RuntimeStep,
} from "./runtime-state.js";
import {
  AGENT_CONTINUITY_PROMPT,
  getAgentLaunchConfig,
  parsePaneId,
  resolveAgentBackend,
  runCommand,
  type AgentLaunchConfig,
  type CommandRunner,
} from "./workflow-utils.js";

export type LabSessionKind = "shape" | "grill";

export const LAB_SESSION_KINDS: readonly LabSessionKind[] = ["shape", "grill"];

export function isLabSessionKind(value: string): value is LabSessionKind {
  return (LAB_SESSION_KINDS as readonly string[]).includes(value);
}

/** Where a Lab entry's files live, under <git-common-dir>/grove-lab/<id>. */
export interface LabPaths {
  labDir: string;
  entryDir: string;
  artifactsDir: string;
  /** The agent writes its progress marker here, one word. */
  stageFile: string;
  /**
   * Grove writes this file to end the session. It is named for the run, so
   * ending one session never ends, or is undone by, another of the same entry.
   */
  closeFile: string;
  /** The agent's question cards, Grove's answers, and delivery receipts. */
  questionsDir: string;
  /** Grove's other messages to the agent, and their receipts. */
  requestsDir: string;
  /** The live session's phase and recent output, for Grove. */
  sessionFile: string;
}

const ENTRY_ID = /^[A-Za-z0-9_-]+$/;

export function labPaths(commonDir: string, entryId: string, runId = "session"): LabPaths {
  if (!ENTRY_ID.test(entryId) || !ENTRY_ID.test(runId)) {
    throw new Error(`Invalid Lab entry or run ID: ${entryId}, ${runId}`);
  }
  const labDir = path.join(commonDir, "grove-lab");
  const entryDir = path.join(labDir, entryId);
  return {
    labDir,
    entryDir,
    artifactsDir: path.join(entryDir, "artifacts"),
    stageFile: path.join(entryDir, "stage"),
    closeFile: path.join(entryDir, `${runId}.close`),
    questionsDir: path.join(entryDir, "questions"),
    requestsDir: path.join(entryDir, "requests"),
    sessionFile: path.join(entryDir, "session.json"),
  };
}

export interface LabEntry {
  id: string;
  kind: "idea" | "bug";
  text: string;
  status: string;
}

export function entryTitle(entry: LabEntry): string {
  return (
    entry.text
      .split("\n")
      .map((line) => line.trim())
      .find((line) => line !== "") ?? "Untitled entry"
  );
}

/** Reads one entry from the Lab index Grove maintains. */
export function readLabEntry(paths: LabPaths, entryId: string): LabEntry {
  const indexPath = path.join(paths.labDir, "entries.json");
  const index = JSON.parse(fs.readFileSync(indexPath, "utf8")) as {
    entries?: LabEntry[];
  };
  const entry = index.entries?.find((candidate) => candidate.id === entryId);
  if (!entry) throw new Error(`Lab entry ${entryId} not found in ${indexPath}`);
  return entry;
}

/** The steps a session's run reports, and the stage marker each one starts at. */
export const SESSION_STEPS: Record<LabSessionKind, { title: string; stage: string }[]> = {
  shape: [
    { title: "Shape", stage: "shape" },
    { title: "Publish", stage: "drafted" },
  ],
  grill: [
    { title: "Interview", stage: "interview" },
    { title: "Spec", stage: "spec" },
    { title: "Tickets", stage: "tickets" },
    { title: "Publish", stage: "drafted" },
  ],
};

/**
 * How the agent asks the user anything, and, for a resumed session, the
 * questions already asked. Every brief carries it.
 */
export function buildConversationSection(paths: LabPaths, records: QuestionRecord[]): string {
  const next = path.join(paths.questionsDir, cardFileName(nextQuestionNumber(records)));
  const lines = [buildProtocolSection(paths.questionsDir, next)];
  const soFar = buildInterviewSoFar(records);
  if (soFar !== "") {
    lines.push(
      "",
      "This session resumes an earlier one. These questions were already asked; do not ask them again:",
      "<<<",
      soFar,
      ">>>",
    );
  }
  return lines.join("\n");
}

/** What the agent does instead of asking another question, by session kind. */
export function finishHint(kind: LabSessionKind, paths: LabPaths): string {
  return kind === "shape"
    ? `if the report needs nothing more from the user, write ${path.join(paths.artifactsDir, "issue.md")}, then write drafted to ${paths.stageFile} and stop.`
    : "if you need nothing more from the user right now, continue with the next step of your instructions.";
}

export function buildShapePrompt(
  entry: LabEntry,
  repo: string,
  paths: LabPaths,
  records: QuestionRecord[] = [],
): string {
  const issuePath = path.join(paths.artifactsDir, "issue.md");
  return [
    "You are shaping a bug report for the Grove Lab.",
    "",
    `Repository: ${repo}`,
    `Entry: ${entryTitle(entry)}`,
    "",
    "The bug as the user captured it:",
    "<<<",
    entry.text,
    ">>>",
    "",
    "Turn it into a clear, actionable GitHub bug report.",
    "",
    "How to work:",
    "- You may read the repository to name the component involved. Do not diagnose a root cause beyond what the evidence shows, and do not change any file in the repository.",
    "- When the report needs something the captured text does not say — steps to reproduce, expected or actual behaviour, environment — ask the user with a question card, one question per turn. Never invent details.",
    "",
    buildConversationSection(paths, records),
    "",
    "Output: write exactly one file, at this absolute path:",
    `  ${issuePath}`,
    "with this structure:",
    "  # <a concise title>",
    "  ## Summary",
    "  ## Steps to reproduce",
    "  ## Expected behaviour",
    "  ## Actual behaviour",
    "  ## Environment",
    "  ## Notes",
    'Write "Unknown" under a heading only when the user has confirmed it is unknown.',
    "If that file already exists, it is the draft from an earlier session: read it and continue from it rather than starting over.",
    "",
    `Progress: overwrite ${paths.stageFile} with a single word —`,
    "  shape    when you begin",
    "  drafted  once issue.md is written and complete",
    "",
    "After writing drafted, stop. Grove shows the draft to the user.",
    "If the user asks for changes, you receive them as a message: revise issue.md and write drafted again.",
    "",
    "Do not publish. Do not create issues or run any command that changes anything on GitHub; Grove publishes once the user approves the draft.",
  ].join("\n");
}

/**
 * The bundled skills, copied from mattpocock/skills. They ship beside dist/ in
 * the installed package; see skills/mattpocock/README.md.
 */
export function bundledSkillsDir(): string {
  return path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "skills", "mattpocock");
}

export function buildGrillPrompt(
  entry: LabEntry,
  repo: string,
  paths: LabPaths,
  skillsDir: string,
  shapedReport?: string,
  records: QuestionRecord[] = [],
): string {
  const skill = (...parts: string[]) => path.join(skillsDir, ...parts);
  const artifact = (...parts: string[]) => path.join(paths.artifactsDir, ...parts);
  return [
    "You are running a Grove Lab grilling session with the user.",
    "",
    `Repository: ${repo}`,
    `Entry: ${entryTitle(entry)}`,
    "",
    "The idea as the user captured it:",
    "<<<",
    entry.text,
    ">>>",
    "",
    ...(shapedReport
      ? [
          "This entry began as a bug and was shaped into the report below. It turned out bigger than one issue; start the interview from what the report already establishes rather than from a blank page.",
          "<<<",
          shapedReport.trim(),
          ">>>",
          "",
        ]
      : []),
    "Work through three skills, in order, as one continuous session with the user. When you reach each one, read its files in full and follow them, except where the Grove rules below say otherwise.",
    "",
    "1. Interview (grill-with-docs) — follow both:",
    `   ${skill("grilling", "SKILL.md")}`,
    `   ${skill("domain-modeling", "SKILL.md")}  (with CONTEXT-FORMAT.md and ADR-FORMAT.md beside it)`,
    `2. Spec — ${skill("to-spec", "SKILL.md")}`,
    `3. Tickets — ${skill("to-tickets", "SKILL.md")}`,
    "",
    "Move from one step to the next only when the user agrees the current one is done: ask with a \"choice\" question card.",
    "",
    "GROVE RULES — these override the skills.",
    "",
    buildConversationSection(paths, records),
    "",
    "The skills sometimes tell you to ask several questions at once, to present a list for the user to react to, or to check something with the user. Do every one of these through question cards, one question per turn.",
    "",
    "Where files go:",
    "- Never create or change a file in the repository. Read it as much as you need.",
    `- Write every document the skills would write into the repository to the same repository-relative path under ${paths.artifactsDir}.`,
    `  For example: CONTEXT.md goes to ${artifact("CONTEXT.md")}, and a decision record to ${artifact("docs", "adr", "0003-<slug>.md")}.`,
    "  Grove copies each one into the repository when the user approves it.",
    "- When the repository already has the CONTEXT.md you would write (at the root, or where CONTEXT-MAP.md points), copy it to its artifacts path first and extend it there, so the draft keeps every existing term.",
    `- Number a new ADR after the highest number in both the repository's docs/adr/ and ${artifact("docs", "adr")}.`,
    "",
    "Nothing is published:",
    "- There is no issue tracker to configure; ignore any instruction about /setup-matt-pocock-skills.",
    "- Do not create issues, labels, or comments, and run no command that changes anything on GitHub. Grove publishes once the user approves the drafts.",
    `- to-spec: instead of publishing, write the spec to ${artifact("spec.md")}. Its first line is "# <title of the epic>", followed by the template's sections.`,
    `- to-tickets: once the user approves the breakdown, write it to ${artifact("tickets.json")} instead of publishing, as:`,
    '    {"tickets": [{"key": "01", "title": "…", "body": "…", "blocked_by": []}]}',
    '  key: "01", "02", … in dependency order, blockers first.',
    '  body: Markdown with the issue template\'s "## What to build" and "## Acceptance criteria" sections only; Grove adds the parent and blocking links itself.',
    "  blocked_by: the keys of the tickets that block this one.",
    "",
    `Progress: overwrite ${paths.stageFile} with a single word as you go —`,
    "  interview  when the interview starts",
    "  spec       when you start the spec",
    "  tickets    when you start the tickets",
    "  drafted    once spec.md and tickets.json are written",
    "",
    "After writing drafted, stop. Grove shows the drafts to the user.",
    "If the user asks for changes, you receive them as a message: revise the files and write drafted again.",
    "",
    `If ${paths.artifactsDir} already holds drafts, this session resumes an earlier one: read them and continue from where the work stopped rather than starting over.`,
  ].join("\n");
}

export type AgentState = "working" | "idle" | "blocked" | "done" | "unknown" | "gone";

/** Finds an agent's status in `herdr agent get` output. */
export function parseAgentState(output: string): AgentState {
  let value: unknown;
  try {
    value = JSON.parse(output);
  } catch {
    return "unknown";
  }
  const find = (node: unknown): string | null => {
    if (!node || typeof node !== "object") return null;
    for (const [key, child] of Object.entries(node)) {
      if (key === "agent_status" && typeof child === "string") return child;
      const nested = find(child);
      if (nested) return nested;
    }
    return null;
  };
  const status = find(value);
  switch (status) {
    case "working":
    case "idle":
    case "blocked":
    case "done":
      return status;
    default:
      return "unknown";
  }
}

/**
 * What a session reports to Grove. `question` is a card waiting for an
 * answer; `fallback` is a turn that ended without one.
 */
export type SessionPhase = "starting" | "working" | "question" | "fallback" | "permission" | "review";

const BLOCKED_PHASES: readonly SessionPhase[] = ["question", "fallback", "permission", "review"];

export interface SessionReport {
  status: "running" | "blocked";
  current_step: string;
  steps: RuntimeStep[];
  summary: string;
}

/**
 * Derives the run's steps and status from the session's phase and the stage
 * the agent reported. Every phase that waits on the user blocks the run.
 */
export function sessionReport(
  kind: LabSessionKind,
  phase: SessionPhase,
  stage: string,
  startedAt: string,
  pending: number[] = [],
): SessionReport {
  const steps = SESSION_STEPS[kind];
  let current = steps.findIndex((step) => step.stage === stage);
  if (current < 0) current = 0;
  const blocked = BLOCKED_PHASES.includes(phase);
  const summary = {
    starting: "Starting the agent",
    working: `${steps[current].title} in progress`,
    question: `Question ${pending[0] ?? ""} is waiting for you in Grove`.replace("  ", " "),
    fallback: "The agent replied without a question card; reply in Grove",
    permission: "The agent needs a permission decision; answer it in Grove",
    review: "The draft is ready to review in Grove's Lab",
  }[phase];
  return {
    status: blocked ? "blocked" : "running",
    current_step: steps[current].title,
    summary,
    steps: steps.map((step, index) => ({
      id: step.stage,
      title: step.title,
      status:
        index < current ? "succeeded" : index === current ? (blocked ? "blocked" : "running") : "queued",
      ...(index === current ? { summary, started_at: startedAt } : {}),
    })),
  };
}

/** The session state Grove reads from session.json. */
export interface SessionState {
  phase: SessionPhase;
  stage: string;
  pane_id: string;
  agent: string;
  /** Numbers of the valid cards waiting for an answer. */
  pending: number[];
  /** The agent's recent output, for a fallback or permission card. */
  output?: string;
  counts: { repairs: number; fallbacks: number };
  updated_at: string;
}

export interface SessionContext {
  kind: LabSessionKind;
  agentName: string;
  paneId: string;
  /** The backend and model, recorded for comparing models. */
  agent: string;
  paths: LabPaths;
}

export interface SessionDeps {
  runner: CommandRunner;
  sleep: (ms: number) => Promise<void>;
  exists: (file: string) => boolean;
  readStage: () => string;
  report: (report: SessionReport, pane: string | null) => void;
  now: () => number;
  scanQuestions: () => QuestionRecord[];
  scanRequests: () => RequestRecord[];
  writeQuestionReceipt: (number: number, receipt: Receipt) => void;
  writeRequestReceipt: (number: number, receipt: Receipt) => void;
  writeSession: (state: SessionState) => void;
}

/** How long an agent may be missing before the session gives up on it. */
const AGENT_GONE_GRACE_MS = 30_000;
/** How long after the brief an idle agent is taken to have not started yet. */
const START_GRACE_MS = 60_000;
/** How long after a later prompt an idle agent is taken to not have picked it up. */
const PROMPT_GRACE_MS = 20_000;
/** Repair prompts sent for one card before its turn counts as a fallback. */
export const MAX_REPAIRS = 2;
/** Lines of agent output shown on a fallback or permission card. */
const OUTPUT_LINES = 30;
const POLL_MS = 1_000;

/** The keys that answer a permission prompt: its default choice, or cancel. */
export function permissionKeys(allow: boolean): string[] {
  return allow ? ["enter"] : ["esc"];
}

/**
 * Runs the session until Grove closes it. Each poll reads the agent's state
 * and the protocol files; whenever the agent is idle, it delivers what Grove
 * has written — requests first, then repairs of invalid cards, then answers
 * once every waiting card has one — and otherwise reports what the session
 * is waiting for.
 */
export async function watchSession(ctx: SessionContext, deps: SessionDeps): Promise<void> {
  const { kind, agentName, paneId, paths } = ctx;
  const startedAt = new Date(deps.now()).toISOString();
  const promptedAt = deps.now();
  const finish = finishHint(kind, paths);
  const counts = { repairs: 0, fallbacks: 0 };
  const repairs = new Map<number, { count: number; raw: string }>();

  let seenWorking = false;
  let goneSince: number | null = null;
  // The brief has just been sent; the agent's first turn answers it.
  let awaitingTurn = true;
  let awaitingSince = promptedAt;
  let turnActive = false;
  let turnPrompted = false;
  let waitingAtTurnStart: number[] = [];
  let fallbackOutput: string | null = null;
  let lastPublished = "";

  const stamp = () => new Date(deps.now()).toISOString();
  const readOutput = (): string => {
    try {
      return deps.runner("herdr", ["agent", "read", agentName, "--lines", String(OUTPUT_LINES)]).trimEnd();
    } catch {
      return "";
    }
  };
  const prompt = (text: string) => {
    deps.runner("herdr", ["agent", "prompt", agentName, text]);
    awaitingTurn = true;
    awaitingSince = deps.now();
  };
  const publish = (phase: SessionPhase, stage: string, pending: number[] = [], output?: string) => {
    const key = JSON.stringify([phase, stage, pending, output ?? "", counts]);
    if (key === lastPublished) return;
    lastPublished = key;
    deps.report(sessionReport(kind, phase, stage, startedAt, pending), paneId);
    deps.writeSession({
      phase,
      stage,
      pane_id: paneId,
      agent: ctx.agent,
      pending,
      ...(output !== undefined ? { output } : {}),
      counts: { ...counts },
      updated_at: stamp(),
    });
  };

  publish("starting", "");
  while (!deps.exists(paths.closeFile)) {
    let state: AgentState;
    try {
      state = parseAgentState(deps.runner("herdr", ["agent", "get", agentName]));
    } catch {
      state = "gone";
    }
    if (state === "gone" || state === "unknown") {
      goneSince ??= deps.now();
      if (deps.now() - goneSince > AGENT_GONE_GRACE_MS) {
        throw new Error(`The agent in pane ${paneId} is no longer running`);
      }
      await deps.sleep(POLL_MS);
      continue;
    }
    goneSince = null;

    const stage = deps.readStage();
    let records = deps.scanQuestions();

    if (state === "working") {
      if (!turnActive) {
        turnActive = true;
        turnPrompted = awaitingTurn;
        waitingAtTurnStart = records.filter(isPending).map((record) => record.number);
      }
      awaitingTurn = false;
      seenWorking = true;
      fallbackOutput = null;
      publish("working", stage);
      await deps.sleep(POLL_MS);
      continue;
    }
    if (!seenWorking && deps.now() - promptedAt < START_GRACE_MS && !records.some(isPending)) {
      publish("starting", stage);
      await deps.sleep(POLL_MS);
      continue;
    }

    if (turnActive) {
      turnActive = false;
      // A turn no one prompted from here was started by typing in the pane.
      // When it moved the session on, the cards it left behind were answered
      // there, and are closed so the session cannot wait on them forever.
      if (!turnPrompted && waitingAtTurnStart.length > 0) {
        const highest = Math.max(...waitingAtTurnStart);
        const movedOn =
          stage === "drafted" || records.some((record) => record.number > highest && record.question !== undefined);
        if (movedOn) {
          for (const record of records) {
            if (waitingAtTurnStart.includes(record.number) && isPending(record)) {
              deps.writeQuestionReceipt(record.number, { revision: 0, via: "pane", sent_at: stamp() });
            }
          }
          records = deps.scanQuestions();
        }
      }
    }

    const requests = deps.scanRequests();
    if (state === "blocked") {
      const permission = requests.find((r) => r.request?.kind === "permission" && !r.receipt);
      if (permission?.request) {
        for (const key of permissionKeys(permission.request.allow === true)) {
          deps.runner("herdr", ["agent", "send-keys", agentName, key]);
        }
        deps.writeRequestReceipt(permission.number, { revision: 0, via: "grove", sent_at: stamp() });
        awaitingTurn = true;
        awaitingSince = deps.now();
        await deps.sleep(POLL_MS);
        continue;
      }
      publish("permission", stage, [], readOutput());
      await deps.sleep(POLL_MS);
      continue;
    }

    // The agent is idle. A prompt it has not picked up yet is given time,
    // unless the agent has already answered it with a card or a draft.
    const pending = records.filter(isPending);
    if (awaitingTurn) {
      const answered = pending.length > 0 || stage === "drafted";
      const grace = seenWorking ? PROMPT_GRACE_MS : START_GRACE_MS;
      if (!answered && deps.now() - awaitingSince < grace) {
        await deps.sleep(POLL_MS);
        continue;
      }
      awaitingTurn = false;
    }

    // A permission answer that arrives after the prompt went away is moot.
    for (const stale of requests.filter((r) => r.request?.kind === "permission" && !r.receipt)) {
      deps.writeRequestReceipt(stale.number, { revision: 0, via: "grove", sent_at: stamp() });
    }

    const nextCard = path.join(paths.questionsDir, cardFileName(nextQuestionNumber(records)));
    const request = requests.find((r) => r.request && r.request.kind !== "permission" && !r.receipt);
    if (request?.request) {
      prompt(buildRequestPrompt(request.request, nextCard, finish, paths.artifactsDir, paths.stageFile));
      deps.writeRequestReceipt(request.number, { revision: 0, via: "grove", sent_at: stamp() });
      await deps.sleep(POLL_MS);
      continue;
    }

    const broken = records.filter((record) => record.error !== undefined && !record.answer && !record.receipt).at(-1);
    if (broken) {
      const tried = repairs.get(broken.number);
      if (!tried || (tried.raw !== broken.raw && tried.count < MAX_REPAIRS)) {
        repairs.set(broken.number, { count: (tried?.count ?? 0) + 1, raw: broken.raw });
        counts.repairs++;
        prompt(buildRepairPrompt(broken));
        await deps.sleep(POLL_MS);
        continue;
      }
    }

    if (pending.length > 0) {
      publish("question", stage, pending.map((record) => record.number));
      await deps.sleep(POLL_MS);
      continue;
    }

    const answered = records.filter(needsDelivery);
    if (answered.length > 0) {
      prompt(buildAnswersPrompt(answered, nextCard, finish));
      for (const record of answered) {
        deps.writeQuestionReceipt(record.number, {
          revision: record.answer?.revisions?.length ?? 0,
          via: "grove",
          sent_at: stamp(),
        });
      }
      await deps.sleep(POLL_MS);
      continue;
    }

    if (stage === "drafted") {
      publish("review", stage);
    } else {
      if (fallbackOutput === null) {
        counts.fallbacks++;
        fallbackOutput = readOutput();
      }
      publish("fallback", stage, [], fallbackOutput);
    }
    await deps.sleep(POLL_MS);
  }
}

function gitCommonDir(repo: string): string {
  const result = spawnSync("git", ["rev-parse", "--git-common-dir"], {
    cwd: repo,
    encoding: "utf8",
  });
  if (result.status !== 0) throw new Error(`Not a git repository: ${repo}`);
  return path.resolve(repo, result.stdout.trim());
}

function readStageFile(file: string): string {
  try {
    if (!fs.statSync(file).isFile()) return "";
    return fs.readFileSync(file, "utf8").trim().toLowerCase();
  } catch {
    return "";
  }
}

/**
 * Removes a session marker file, tolerating a stale directory left at its
 * path (an agent that mistakes `stage` for a directory). `recursive` lets
 * the same call clear files and directories.
 */
function clearMarkerFile(file: string): void {
  fs.rmSync(file, { force: true, recursive: true });
}

function writeJsonAtomic(file: string, value: unknown): void {
  const tmp = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, `${JSON.stringify(value, null, 2)}\n`, "utf8");
  fs.renameSync(tmp, file);
}

/**
 * A path as Claude Code's permission rules spell an absolute path: forward
 * slashes, a leading `//`, and a Windows drive as its lower-case letter, so
 * `C:\Users\me` becomes `//c/Users/me`.
 */
export function claudePathRule(file: string): string {
  let posix = file.replace(/\\/g, "/").replace(/\/+$/, "");
  const drive = /^([A-Za-z]):(\/.*)?$/.exec(posix);
  if (drive) posix = `/${drive[1].toLowerCase()}${drive[2] ?? ""}`;
  return `/${posix.startsWith("/") ? posix : `/${posix}`}`;
}

/** Read-only commands a Lab agent may run without asking. */
const LAB_READ_ONLY_COMMANDS = ["git log", "git show", "git diff", "git grep", "git blame", "git ls-files", "git status"];

/**
 * The agent's command-line arguments for a Lab session. The agent reads the
 * repository, runs read-only commands, and writes under its entry's directory;
 * on Claude Code, which can enforce that, everything else is refused without
 * asking, so the session never stops on a permission prompt. OpenCode runs
 * with --auto and Pi without a permission system, so neither prompts.
 */
export function labAgentArgs(
  launch: AgentLaunchConfig,
  paths: LabPaths,
  env: NodeJS.ProcessEnv = process.env,
): string[] {
  if (launch.backend !== "claude") return launch.args;
  const args = ["--"];
  if (env.AGENT_FLOW_CLAUDE_MODEL) args.push("--model", env.AGENT_FLOW_CLAUDE_MODEL);
  args.push(
    "--permission-mode",
    "dontAsk",
    "--add-dir",
    paths.entryDir,
    "--append-system-prompt",
    AGENT_CONTINUITY_PROMPT,
    // --allowedTools is variadic, so it must stay last in the argument list.
    // An Edit rule covers every tool that writes files.
    "--allowedTools",
    "Read",
    "Glob",
    "Grep",
    ...LAB_READ_ONLY_COMMANDS.map((command) => `Bash(${command}:*)`),
    `Edit(${claudePathRule(paths.entryDir)}/**)`,
  );
  return args;
}

async function main(kind: LabSessionKind, entryId: string): Promise<void> {
  const repo = process.cwd();
  const runId = process.env.GROVE_WORKFLOW_RUN_ID ?? `lab-${process.pid}`;
  const paths = labPaths(gitCommonDir(repo), entryId, runId);
  const entry = readLabEntry(paths, entryId);
  fs.mkdirSync(paths.artifactsDir, { recursive: true });
  clearMarkerFile(paths.closeFile);
  clearMarkerFile(paths.stageFile);

  const title = `${kind === "shape" ? "Shape" : "Grill"}: ${entryTitle(entry)}`;
  updateTrackedWorkflow({ title, current_step: "Starting" });

  const launch = getAgentLaunchConfig();
  if (launch.backend === "claude") ensureClaudeWorkspaceTrust(repo);
  // A resumed session's questions are replayed in the brief, answers the
  // previous agent never received included, so those count as delivered.
  const records = scanQuestions(paths.questionsDir);
  const promptText =
    kind === "shape"
      ? buildShapePrompt(entry, repo, paths, records)
      : buildGrillPrompt(entry, repo, paths, bundledSkillsDir(), readShapedReport(paths), records);
  const assignmentPath = path.join(paths.entryDir, "assignment.md");
  fs.writeFileSync(assignmentPath, promptText, "utf8");
  for (const record of records.filter(needsDelivery)) {
    writeReceipt(paths.questionsDir, record.number, {
      revision: record.answer?.revisions?.length ?? 0,
      via: "grove",
      sent_at: new Date().toISOString(),
    });
  }

  const paneArgs = ["pane", "split", "--current", "--direction", "right", "--cwd", repo];
  const paneId = parsePaneId(runCommand("herdr", paneArgs));
  const agentName = `lab-${kind}-${entryId}`.slice(0, 32);
  try {
    startAgentWithReadinessRecovery(agentName, [
      "agent",
      "start",
      agentName,
      "--kind",
      launch.kind,
      "--pane",
      paneId,
      ...labAgentArgs(launch, paths),
    ]);
    // The user answers in Grove, so the pane is not focused.
    runCommand("herdr", ["agent", "prompt", agentName, deliverablePrompt(promptText, assignmentPath)]);

    await watchSession(
      { kind, agentName, paneId, agent: launch.label, paths },
      {
        runner: runCommand,
        sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
        exists: (file) => fs.existsSync(file),
        readStage: () => readStageFile(paths.stageFile),
        now: () => Date.now(),
        scanQuestions: () => scanQuestions(paths.questionsDir),
        scanRequests: () => scanRequests(paths.requestsDir),
        writeQuestionReceipt: (number, receipt) => writeReceipt(paths.questionsDir, number, receipt),
        writeRequestReceipt: (number, receipt) => writeReceipt(paths.requestsDir, number, receipt),
        writeSession: (state) => writeJsonAtomic(paths.sessionFile, state),
        report: (report, pane: string | null) =>
          updateTrackedWorkflow({
            status: report.status,
            current_step: report.current_step,
            steps: report.steps,
            agents: [
              {
                id: `${process.env.GROVE_WORKFLOW_RUN_ID ?? agentName}:${kind}`,
                kind: resolveAgentBackend(),
                name: agentName,
                status: report.status === "blocked" ? "blocked" : "working",
                summary: report.summary,
                pane_id: pane,
              },
            ],
          }),
      },
    );
  } finally {
    try {
      runCommand("herdr", ["pane", "close", paneId]);
    } catch {
      // The user may have closed the pane already.
    }
    clearMarkerFile(paths.closeFile);
    clearMarkerFile(paths.sessionFile);
  }
  const steps = SESSION_STEPS[kind].map((step) => ({ id: step.stage, title: step.title, status: "succeeded" }));
  updateTrackedWorkflow({ current_step: "Closed", steps, agents: [] });
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  const [kind, entryId] = process.argv.slice(2);
  if (!kind || !isLabSessionKind(kind) || !entryId) {
    console.error("Usage: grove-lab <shape|grill> <entry-id>");
    process.exit(2);
  }
  runTrackedWorkflow(kind, [entryId], () => main(kind, entryId)).catch((error: unknown) => {
    console.error(`\x1b[31m[Lab]\x1b[0m ${error instanceof Error ? error.message : String(error)}`);
    process.exit(1);
  });
}

/** The shaped report of a bug escalated to a grill, if it has one. */
function readShapedReport(paths: LabPaths): string | undefined {
  try {
    const report = fs.readFileSync(path.join(paths.artifactsDir, "issue.md"), "utf8");
    return report.trim() === "" ? undefined : report;
  } catch {
    return undefined;
  }
}
