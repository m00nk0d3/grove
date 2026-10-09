#!/usr/bin/env node

// A Lab session: an entry taken through its stages — scout, interview, spec,
// and tickets for an idea; shape for a bug — by one fresh agent per stage, in
// a Herdr pane, with the user answering in Grove. The agent asks through
// question cards; Grove writes the answers and approvals; this process starts
// each stage's agent, delivers what Grove writes, checks what the agent
// writes, reports the session's state to Grove, and closes the pane when
// Grove says the session is over. See docs/LAB_DESIGN.md.

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
  checkCoverage,
  isApproved,
  nextAdrNumber,
  readIfExists,
  validateIssue,
  validateScout,
  validateSpec,
  validateTickets,
} from "./lab-drafts.js";
import { buildStagePrompt, NONE, type LabStage, type PromptValues } from "./lab-prompts.js";
import { agentPaths, labWorkRoot, LabWorkSync, type AgentPaths } from "./lab-work.js";
import {
  buildAnswersPrompt,
  buildDraftRepairPrompt,
  buildInterviewSoFar,
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
  /** The runtime writes the active stage here, for Grove. */
  stageFile: string;
  /** The agent writes its stage's name here when the stage is finished. */
  doneFile: string;
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
  scoutFile: string;
  coverageFile: string;
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
    doneFile: path.join(entryDir, "done"),
    closeFile: path.join(entryDir, `${runId}.close`),
    questionsDir: path.join(entryDir, "questions"),
    requestsDir: path.join(entryDir, "requests"),
    sessionFile: path.join(entryDir, "session.json"),
    scoutFile: path.join(entryDir, "scout.md"),
    coverageFile: path.join(entryDir, "coverage.json"),
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

/** A step of a session's run: a stage with an agent, or waiting to publish. */
export type SessionStep = LabStage | "publish";

/** The stages of each kind of session, in order. */
export const SESSION_STAGES: Record<LabSessionKind, LabStage[]> = {
  grill: ["scout", "interview", "spec", "tickets"],
  shape: ["shape"],
};

/** The draft the user approves before a stage's session moves on. */
export const REVIEWED_DRAFT: Partial<Record<LabStage, string>> = {
  spec: "spec.md",
  tickets: "tickets.json",
  shape: "issue.md",
};

/** The stages that ask the user through question cards. */
const ASKING_STAGES: readonly LabStage[] = ["interview", "shape"];

/** The steps a session's run reports. A shaped report is reviewed in its own step. */
export const SESSION_STEPS: Record<LabSessionKind, { id: string; title: string }[]> = {
  grill: [
    { id: "scout", title: "Scout" },
    { id: "interview", title: "Interview" },
    { id: "spec", title: "Spec" },
    { id: "tickets", title: "Tickets" },
    { id: "publish", title: "Publish" },
  ],
  shape: [
    { id: "shape", title: "Shape" },
    { id: "review", title: "Review" },
    { id: "publish", title: "Publish" },
  ],
};

export function nextStep(kind: LabSessionKind, step: SessionStep): SessionStep {
  const stages = SESSION_STAGES[kind];
  const at = stages.indexOf(step as LabStage);
  return at >= 0 && at + 1 < stages.length ? stages[at + 1] : "publish";
}

/**
 * Where a session picks up, from what the entry's files show: the first
 * stage whose work is not finished. A stage the agent finished just before
 * the session stopped is recorded in the done file.
 */
export function resumeStep(kind: LabSessionKind, paths: LabPaths, entryId: string): SessionStep {
  const approved = (name: string) =>
    isApproved(paths.labDir, entryId, name, readIfExists(path.join(paths.artifactsDir, name)));
  const done = readMarker(paths.doneFile);
  if (kind === "shape") return approved("issue.md") ? "publish" : "shape";
  if (approved("tickets.json")) return "publish";
  if (approved("spec.md")) return "tickets";
  if (readIfExists(path.join(paths.artifactsDir, "spec.md")) !== null || done === "interview") return "spec";
  const scout = readIfExists(paths.scoutFile);
  if (scout === null || (validateScout(scout).length > 0 && done !== "scout")) return "scout";
  return "interview";
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
 * answer; `fallback` is a turn that ended without one; `review` is a draft
 * waiting for approval, or a session waiting to be published.
 */
export type SessionPhase = "starting" | "working" | "question" | "fallback" | "permission" | "review";

const BLOCKED_PHASES: readonly SessionPhase[] = ["question", "fallback", "permission", "review"];

export interface SessionReport {
  status: "running" | "blocked";
  current_step: string;
  steps: RuntimeStep[];
  summary: string;
}

const REVIEW_SUMMARY: Partial<Record<SessionStep, string>> = {
  spec: "The spec is ready to review in Grove's Lab",
  tickets: "The tickets are ready to review in Grove's Lab",
  shape: "The bug report is ready to review in Grove's Lab",
  publish: "Ready to publish from Grove's Lab",
};

/** The id of the run step a stage and phase are reported as. */
function stepId(kind: LabSessionKind, step: SessionStep, phase: SessionPhase): string {
  return kind === "shape" && step === "shape" && phase === "review" ? "review" : step;
}

/**
 * Derives the run's steps and status from the session's step and phase.
 * Every phase that waits on the user blocks the run.
 */
export function sessionReport(
  kind: LabSessionKind,
  phase: SessionPhase,
  step: SessionStep,
  startedAt: string,
  pending: number[] = [],
): SessionReport {
  const steps = SESSION_STEPS[kind];
  let current = steps.findIndex((s) => s.id === stepId(kind, step, phase));
  if (current < 0) current = 0;
  const blocked = BLOCKED_PHASES.includes(phase);
  const summary = {
    starting: "Starting the agent",
    working: `${steps[current].title} in progress`,
    question: `Question ${pending[0] ?? ""} is waiting for you in Grove`.replace("  ", " "),
    fallback: "The agent replied without a question card; reply in Grove",
    permission: "The agent needs a permission decision; answer it in Grove",
    review: REVIEW_SUMMARY[step] ?? "The draft is ready to review in Grove's Lab",
  }[phase];
  return {
    status: blocked ? "blocked" : "running",
    current_step: steps[current].title,
    summary,
    steps: steps.map((s, index) => ({
      id: s.id,
      title: s.title,
      status:
        index < current ? "succeeded" : index === current ? (blocked ? "blocked" : "running") : "queued",
      ...(index === current ? { summary, started_at: startedAt } : {}),
    })),
  };
}

/** Repairs and fallbacks, counted for comparing models. */
export interface SessionCounts {
  repairs: number;
  fallbacks: number;
}

/** The session state Grove reads from session.json. */
export interface SessionState {
  phase: SessionPhase;
  /** The active step: a stage, or publish. */
  stage: SessionStep;
  pane_id: string;
  agent: string;
  /** Numbers of the valid cards waiting for an answer. */
  pending: number[];
  /** The agent's recent output, for a fallback or permission card. */
  output?: string;
  /** Problems the draft under review still has after the agent's repairs. */
  problems?: string[];
  counts: SessionCounts;
  stage_counts: Partial<Record<LabStage, SessionCounts>>;
  updated_at: string;
}

export interface StageContext {
  kind: LabSessionKind;
  stage: LabStage;
  agentName: string;
  paneId: string;
  /** The backend and model, recorded for comparing models. */
  agent: string;
  /** The Lab entry, which the runtime and Grove read. */
  paths: LabPaths;
  /** Where the agent writes: the stage's working folder, outside .git. */
  work: AgentPaths;
}

export interface SessionDeps {
  runner: CommandRunner;
  sleep: (ms: number) => Promise<void>;
  now: () => number;
  closed: () => boolean;
  readDone: () => string;
  clearDone: () => void;
  /** Brings the agent's working folder and the Lab entry in step. */
  sync: () => void;
  /** Problems with what the stage wrote, phrased for the agent. */
  checkStage: (finishRequested: boolean, answered: number) => string[];
  /** Whether the user approved the stage's draft as it is now. */
  approved: () => boolean;
  /**
   * Whether what the stage builds on still stands. The tickets stage builds
   * on the approved spec; the user reopening the spec ends the stage.
   */
  prerequisitesHold: () => boolean;
  report: (report: SessionReport, pane: string | null) => void;
  scanQuestions: () => QuestionRecord[];
  scanRequests: () => RequestRecord[];
  writeQuestionReceipt: (number: number, receipt: Receipt) => void;
  writeRequestReceipt: (number: number, receipt: Receipt) => void;
  writeSession: (state: SessionState) => void;
}

/** How long an agent may be missing before the session gives up on it. */
const AGENT_GONE_GRACE_MS = 30_000;
/** How long after its brief an idle agent is taken to have not started yet. */
const START_GRACE_MS = 60_000;
/** How long after a later prompt an idle agent is taken to not have picked it up. */
const PROMPT_GRACE_MS = 20_000;
/** Repair prompts sent for one card or one draft before moving on without it. */
export const MAX_REPAIRS = 2;
/** Lines of agent output shown on a fallback or permission card. */
const OUTPUT_LINES = 30;
const POLL_MS = 1_000;

/** The keys that answer a permission prompt: its default choice, or cancel. */
export function permissionKeys(allow: boolean): string[] {
  return allow ? ["enter"] : ["esc"];
}

/** What the agent does instead of asking another question, by stage. */
export function finishHint(stage: LabStage, paths: AgentPaths): string {
  switch (stage) {
    case "interview":
      return `if the interview is complete, update ${paths.coverageFile}, then write interview to ${paths.doneFile} and stop.`;
    case "shape":
      return `if the report needs nothing more from the user, write ${path.join(paths.artifactsDir, "issue.md")}, then write shape to ${paths.doneFile} and stop.`;
    default:
      return `when your work is complete, write ${stage} to ${paths.doneFile} and stop.`;
  }
}

/**
 * Tracks a session's counts and publishes its state, to the run and to
 * session.json, whenever it changes.
 */
export class SessionReporter {
  readonly counts: SessionCounts = { repairs: 0, fallbacks: 0 };
  readonly stageCounts: Partial<Record<LabStage, SessionCounts>> = {};
  private last = "";
  private readonly startedAt: string;

  constructor(
    private readonly kind: LabSessionKind,
    private readonly agent: string,
    private readonly deps: Pick<SessionDeps, "report" | "writeSession" | "now">,
  ) {
    this.startedAt = new Date(deps.now()).toISOString();
  }

  count(stage: LabStage, what: keyof SessionCounts): void {
    this.counts[what]++;
    const forStage = (this.stageCounts[stage] ??= { repairs: 0, fallbacks: 0 });
    forStage[what]++;
  }

  publish(
    phase: SessionPhase,
    step: SessionStep,
    paneId: string | null,
    extra: { pending?: number[]; output?: string; problems?: string[] } = {},
  ): void {
    const pending = extra.pending ?? [];
    const key = JSON.stringify([phase, step, paneId, pending, extra.output ?? "", extra.problems ?? [], this.counts]);
    if (key === this.last) return;
    this.last = key;
    this.deps.report(sessionReport(this.kind, phase, step, this.startedAt, pending), paneId);
    this.deps.writeSession({
      phase,
      stage: step,
      pane_id: paneId ?? "",
      agent: this.agent,
      pending,
      ...(extra.output !== undefined ? { output: extra.output } : {}),
      ...(extra.problems && extra.problems.length > 0 ? { problems: extra.problems } : {}),
      counts: { ...this.counts },
      stage_counts: JSON.parse(JSON.stringify(this.stageCounts)) as SessionState["stage_counts"],
      updated_at: new Date(this.deps.now()).toISOString(),
    });
  }
}

/** How a stage ends: done, closed by Grove, or sent back by the user. */
export type StageOutcome = "finished" | "closed" | "reopened";

/**
 * Runs one stage until it is finished or Grove closes the session. Each poll
 * reads the agent's state and the protocol files; whenever the agent is
 * idle, it delivers what Grove has written — requests first, then repairs of
 * invalid cards, then answers once every waiting card has one. When the
 * agent marks the stage done, its output is checked, repaired at most twice,
 * and, for a reviewed draft, held until the user approves it.
 */
export async function watchStage(
  ctx: StageContext,
  deps: SessionDeps,
  reporter: SessionReporter,
): Promise<StageOutcome> {
  const { stage, agentName, paneId, paths, work } = ctx;
  const promptedAt = deps.now();
  const finish = finishHint(stage, work);
  const reviewed = REVIEWED_DRAFT[stage] !== undefined;
  const asks = ASKING_STAGES.includes(stage);
  const cardRepairs = new Map<number, { count: number; raw: string }>();
  let draftRepairs = 0;
  let finishRequested = false;

  let seenWorking = false;
  let goneSince: number | null = null;
  // The brief has just been sent; the agent's first turn answers it.
  let awaitingTurn = true;
  let awaitingSince = promptedAt;
  let turnActive = false;
  let turnPrompted = false;
  let waitingAtTurnStart: number[] = [];
  let fallbackOutput: string | null = null;

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
  const publish = (phase: SessionPhase, extra: { pending?: number[]; output?: string; problems?: string[] } = {}) =>
    reporter.publish(phase, stage, paneId, extra);

  publish("starting");
  while (!deps.closed()) {
    // What the agent wrote reaches the Lab, and the user's edits the agent.
    deps.sync();
    if (!deps.prerequisitesHold()) return "reopened";
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

    // Only a stage that asks deals with question cards. A card left open by
    // an earlier stage, such as shaping before an escalation, waits for the
    // next stage that asks; an answer given meanwhile is replayed in its brief.
    const scan = () => (asks ? deps.scanQuestions() : []);
    let records = scan();
    const done = () => deps.readDone() === stage;

    if (state === "working") {
      if (!turnActive) {
        turnActive = true;
        turnPrompted = awaitingTurn;
        waitingAtTurnStart = records.filter(isPending).map((record) => record.number);
      }
      awaitingTurn = false;
      seenWorking = true;
      fallbackOutput = null;
      publish("working");
      await deps.sleep(POLL_MS);
      continue;
    }
    if (!seenWorking && deps.now() - promptedAt < START_GRACE_MS && !records.some(isPending) && !done()) {
      publish("starting");
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
        const movedOn = done() || records.some((record) => record.number > highest && record.question !== undefined);
        if (movedOn) {
          for (const record of records) {
            if (waitingAtTurnStart.includes(record.number) && isPending(record)) {
              deps.writeQuestionReceipt(record.number, { revision: 0, via: "pane", sent_at: stamp() });
            }
          }
          records = scan();
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
      publish("permission", { output: readOutput() });
      await deps.sleep(POLL_MS);
      continue;
    }

    // The agent is idle. A prompt it has not picked up yet is given time,
    // unless the agent has already answered it with a card or by finishing.
    const pending = records.filter(isPending);
    if (awaitingTurn) {
      const answered = pending.length > 0 || done();
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

    const nextCard = path.join(work.questionsDir, cardFileName(nextQuestionNumber(records)));
    const request = requests.find((r) => r.request && r.request.kind !== "permission" && !r.receipt);
    if (request?.request) {
      const target = {
        stage,
        doneFile: work.doneFile,
        draftsDir: work.artifactsDir,
        coverageFile: work.coverageFile,
        asks,
      };
      if (request.request.kind === "change" || request.request.kind === "finish_interview") {
        // The agent finishes again once it has done what was asked.
        deps.clearDone();
        draftRepairs = 0;
      }
      if (request.request.kind === "finish_interview") {
        finishRequested = true;
        // Cards still open are not going to be answered.
        for (const record of records.filter(isPending)) {
          deps.writeQuestionReceipt(record.number, { revision: 0, via: "skipped", sent_at: stamp() });
        }
      }
      prompt(buildRequestPrompt(request.request, nextCard, finish, target));
      deps.writeRequestReceipt(request.number, { revision: 0, via: "grove", sent_at: stamp() });
      await deps.sleep(POLL_MS);
      continue;
    }

    const broken = records.filter((record) => record.error !== undefined && !record.answer && !record.receipt).at(-1);
    if (broken) {
      const tried = cardRepairs.get(broken.number);
      if (!tried || (tried.raw !== broken.raw && tried.count < MAX_REPAIRS)) {
        cardRepairs.set(broken.number, { count: (tried?.count ?? 0) + 1, raw: broken.raw });
        reporter.count(stage, "repairs");
        prompt(buildRepairPrompt(broken, path.join(work.questionsDir, path.basename(broken.file))));
        await deps.sleep(POLL_MS);
        continue;
      }
    }

    if (pending.length > 0) {
      publish("question", { pending: pending.map((record) => record.number) });
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

    if (done()) {
      const answeredCount = records.filter((record) => record.answer || record.receipt?.via === "pane").length;
      const problems = deps.checkStage(finishRequested, answeredCount);
      if (problems.length > 0 && draftRepairs < MAX_REPAIRS) {
        draftRepairs++;
        reporter.count(stage, "repairs");
        deps.clearDone();
        prompt(buildDraftRepairPrompt(problems, stage, work.doneFile));
        await deps.sleep(POLL_MS);
        continue;
      }
      if (!reviewed || deps.approved()) return "finished";
      publish("review", { problems });
      await deps.sleep(POLL_MS);
      continue;
    }

    if (fallbackOutput === null) {
      reporter.count(stage, "fallbacks");
      fallbackOutput = readOutput();
    }
    publish("fallback", { output: fallbackOutput });
    await deps.sleep(POLL_MS);
  }
  return "closed";
}

/** Checks what a stage wrote, phrased for the agent. */
export function checkStageOutput(
  stage: LabStage,
  paths: LabPaths,
  finishRequested: boolean,
  answered: number,
): string[] {
  const draft = (name: string) => readIfExists(path.join(paths.artifactsDir, name));
  const missing = (file: string) => [`${file} was not written`];
  switch (stage) {
    case "scout": {
      const text = readIfExists(paths.scoutFile);
      return text === null ? missing(paths.scoutFile) : validateScout(text);
    }
    case "interview": {
      const coverage = checkCoverage(readIfExists(paths.coverageFile) ?? "");
      const problems = [...coverage.problems];
      // Open topics are allowed once the user ended the interview, or after
      // the 25-question check let the agent write the spec.
      const open = coverage.open.filter((topic) => !coverage.problems.some((p) => p.includes(`"${topic}"`)));
      if (open.length > 0 && !finishRequested && answered < 25) {
        problems.push(
          `coverage.json still lists ${open.join(", ")} as open. Ask about each with a question card, or set it to "n/a: <reason>", before finishing`,
        );
      }
      return problems;
    }
    case "spec": {
      const text = draft("spec.md");
      return text === null ? missing(path.join(paths.artifactsDir, "spec.md")) : validateSpec(text);
    }
    case "tickets": {
      const text = draft("tickets.json");
      return text === null ? missing(path.join(paths.artifactsDir, "tickets.json")) : validateTickets(text);
    }
    case "shape": {
      const text = draft("issue.md");
      return text === null ? missing(path.join(paths.artifactsDir, "issue.md")) : validateIssue(text);
    }
  }
}

/** Every value a stage prompt can use. */
export function stageValues(
  stage: LabStage,
  entry: LabEntry,
  repo: string,
  paths: AgentPaths,
  repoMap: string,
  records: QuestionRecord[],
): Omit<PromptValues, "protocol"> {
  const orNone = (text: string | null) => (text === null || text.trim() === "" ? NONE : text.trim());
  const draft = (name: string) => readIfExists(path.join(paths.artifactsDir, name));
  const next = nextQuestionNumber(records);
  const previous: Record<LabStage, string | null> = {
    scout: readIfExists(paths.scoutFile),
    interview: null,
    spec: draft("spec.md"),
    tickets: draft("tickets.json"),
    shape: draft("issue.md"),
  };
  return {
    repo,
    entry_kind: entry.kind,
    entry_title: entryTitle(entry),
    entry_text: entry.text.trim(),
    stage,
    done_file: paths.doneFile,
    entry_dir: paths.entryDir,
    artifacts_dir: paths.artifactsDir,
    repo_map: repoMap,
    scout_file: paths.scoutFile,
    scout_notes: orNone(readIfExists(paths.scoutFile)),
    // A grilled bug that was shaped first starts from its report.
    shaped_report: stage === "shape" ? NONE : orNone(draft("issue.md")),
    questions_dir: paths.questionsDir,
    next_question_number: String(next),
    next_question_path: path.join(paths.questionsDir, cardFileName(next)),
    interview_so_far: orNone(buildInterviewSoFar(records)),
    coverage_file: paths.coverageFile,
    coverage: orNone(readIfExists(paths.coverageFile)),
    spec_file: path.join(paths.artifactsDir, "spec.md"),
    spec: orNone(draft("spec.md")),
    tickets_file: path.join(paths.artifactsDir, "tickets.json"),
    issue_file: path.join(paths.artifactsDir, "issue.md"),
    previous_draft: orNone(previous[stage]),
    next_adr_number: nextAdrNumber(repo, paths.artifactsDir),
  };
}

/** The repository map Grove built for the checkout's commit. */
export function readRepoMap(paths: LabPaths, commit: string | null): string {
  if (commit) {
    const map = readIfExists(path.join(paths.labDir, "repo-map", `${commit}.md`));
    if (map !== null) return map.trim();
  }
  return "(no repository map is available; explore the repository with your file tools)";
}

function gitCommonDir(repo: string): string {
  const result = spawnSync("git", ["rev-parse", "--git-common-dir"], {
    cwd: repo,
    encoding: "utf8",
  });
  if (result.status !== 0) throw new Error(`Not a git repository: ${repo}`);
  return path.resolve(repo, result.stdout.trim());
}

function headCommit(repo: string): string | null {
  const result = spawnSync("git", ["rev-parse", "HEAD"], { cwd: repo, encoding: "utf8" });
  return result.status === 0 ? result.stdout.trim() : null;
}

function readMarker(file: string): string {
  try {
    if (!fs.statSync(file).isFile()) return "";
    return fs.readFileSync(file, "utf8").trim().toLowerCase();
  } catch {
    return "";
  }
}

/**
 * Removes a session marker file, tolerating a stale directory left at its
 * path (an agent that mistakes a marker for a directory). `recursive` lets
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
  paths: AgentPaths,
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

/** The name of a stage's agent in Herdr; one per stage, so stages never collide. */
export function stageAgentName(stage: LabStage, entryId: string): string {
  return `lab-${stage}-${entryId}`.slice(0, 32);
}

async function main(kind: LabSessionKind, entryId: string): Promise<void> {
  const repo = process.cwd();
  const runId = process.env.GROVE_WORKFLOW_RUN_ID ?? `lab-${process.pid}`;
  const paths = labPaths(gitCommonDir(repo), entryId, runId);
  const entry = readLabEntry(paths, entryId);
  fs.mkdirSync(paths.artifactsDir, { recursive: true });
  clearMarkerFile(paths.closeFile);

  const title = `${kind === "shape" ? "Shape" : "Grill"}: ${entryTitle(entry)}`;
  updateTrackedWorkflow({ title, current_step: "Starting" });

  const launch = getAgentLaunchConfig();
  if (launch.backend === "claude") ensureClaudeWorkspaceTrust(repo);
  const repoMap = readRepoMap(paths, headCommit(repo));
  const runtimeId = process.env.GROVE_WORKFLOW_RUN_ID;

  const deps: SessionDeps = {
    runner: runCommand,
    sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
    now: () => Date.now(),
    closed: () => fs.existsSync(paths.closeFile),
    readDone: () => readMarker(paths.doneFile),
    clearDone: () => clearMarkerFile(paths.doneFile),
    sync: () => {},
    checkStage: () => [],
    approved: () => false,
    prerequisitesHold: () => true,
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
        agents: pane
          ? [
              {
                id: `${runtimeId ?? entryId}:${kind}`,
                kind: resolveAgentBackend(),
                name: `lab-${kind}-${entryId}`,
                status: report.status === "blocked" ? "blocked" : "working",
                summary: report.summary,
                pane_id: pane,
              },
            ]
          : [],
      }),
  };
  const reporter = new SessionReporter(kind, launch.label, deps);

  try {
    let step = resumeStep(kind, paths, entryId);
    while (step !== "publish") {
      const outcome = await runStage(step, kind, entry, repo, repoMap, paths, launch, deps, reporter);
      if (outcome === "closed") return;
      // A reopened spec is revised before the tickets are drafted again.
      step = outcome === "reopened" ? "spec" : nextStep(kind, step);
    }
    // Every draft is approved: Grove publishes, then closes the session.
    fs.writeFileSync(paths.stageFile, "publish\n", "utf8");
    reporter.publish("review", "publish", null);
    while (!deps.closed()) await deps.sleep(POLL_MS);
  } finally {
    clearMarkerFile(paths.closeFile);
    clearMarkerFile(paths.sessionFile);
  }
  const steps = SESSION_STEPS[kind].map((step) => ({ id: step.id, title: step.title, status: "succeeded" }));
  updateTrackedWorkflow({ current_step: "Closed", steps, agents: [] });
}

/** Runs one stage with its own agent, in a pane of its own. */
async function runStage(
  stage: LabStage,
  kind: LabSessionKind,
  entry: LabEntry,
  repo: string,
  repoMap: string,
  paths: LabPaths,
  launch: AgentLaunchConfig,
  deps: SessionDeps,
  reporter: SessionReporter,
): Promise<StageOutcome> {
  fs.writeFileSync(paths.stageFile, `${stage}\n`, "utf8");
  clearMarkerFile(paths.doneFile);

  // The agent works in a folder of its own outside .git, which Claude Code
  // will not write into; the runtime keeps it and the entry in step.
  const runId = path.basename(paths.closeFile, ".close");
  const workRoot = labWorkRoot(entry.id, runId, stage);
  fs.rmSync(workRoot, { recursive: true, force: true });
  const sync = new LabWorkSync(paths, agentPaths(workRoot));
  sync.seed();
  const work = sync.work;

  // A resumed session's questions are replayed in the brief, answers the
  // previous agent never received included, so those count as delivered.
  const records = scanQuestions(paths.questionsDir);
  const promptText = buildStagePrompt(stage, repo, stageValues(stage, entry, repo, work, repoMap, records));
  const assignmentPath = path.join(paths.entryDir, `assignment-${stage}.md`);
  fs.writeFileSync(assignmentPath, promptText, "utf8");
  for (const record of records.filter(needsDelivery)) {
    writeReceipt(paths.questionsDir, record.number, {
      revision: record.answer?.revisions?.length ?? 0,
      via: "grove",
      sent_at: new Date().toISOString(),
    });
  }

  const paneId = parsePaneId(runCommand("herdr", ["pane", "split", "--current", "--direction", "right", "--cwd", repo]));
  const agentName = stageAgentName(stage, entry.id);
  try {
    startAgentWithReadinessRecovery(agentName, [
      "agent",
      "start",
      agentName,
      "--kind",
      launch.kind,
      "--pane",
      paneId,
      ...labAgentArgs(launch, work),
    ]);
    // The user answers in Grove, so the pane is not focused.
    runCommand("herdr", ["agent", "prompt", agentName, deliverablePrompt(promptText, assignmentPath)]);
    const review = REVIEWED_DRAFT[stage];
    return await watchStage({ kind, stage, agentName, paneId, agent: launch.label, paths, work }, {
      ...deps,
      sync: () => sync.sync(),
      clearDone: () => sync.clearDone(),
      checkStage: (finishRequested, answered) => checkStageOutput(stage, paths, finishRequested, answered),
      approved: () =>
        review !== undefined &&
        isApproved(paths.labDir, entry.id, review, readIfExists(path.join(paths.artifactsDir, review))),
      prerequisitesHold: () =>
        stage !== "tickets" ||
        isApproved(paths.labDir, entry.id, "spec.md", readIfExists(path.join(paths.artifactsDir, "spec.md"))),
    }, reporter);
  } finally {
    try {
      runCommand("herdr", ["pane", "close", paneId]);
    } catch {
      // The user may have closed the pane already.
    }
    sync.dispose();
  }
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
