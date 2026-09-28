#!/usr/bin/env node

// A Lab session: an agent working through a Lab entry with the user in its
// Herdr pane. The conversation is the user's and the agent's; this process
// only starts the agent, reports its state to Grove, and closes the pane when
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
  runTrackedWorkflow,
  updateTrackedWorkflow,
  type RuntimeStep,
} from "./runtime-state.js";
import {
  getAgentLaunchConfig,
  parsePaneId,
  resolveAgentBackend,
  runCommand,
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
  /** Grove writes this file to end the session. */
  closeFile: string;
}

const ENTRY_ID = /^[A-Za-z0-9_-]+$/;

export function labPaths(commonDir: string, entryId: string): LabPaths {
  if (!ENTRY_ID.test(entryId)) {
    throw new Error(`Invalid Lab entry ID: ${entryId}`);
  }
  const labDir = path.join(commonDir, "grove-lab");
  const entryDir = path.join(labDir, entryId);
  return {
    labDir,
    entryDir,
    artifactsDir: path.join(entryDir, "artifacts"),
    stageFile: path.join(entryDir, "stage"),
    closeFile: path.join(entryDir, "session.close"),
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

export function buildShapePrompt(entry: LabEntry, repo: string, paths: LabPaths): string {
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
    "- When the report needs something the captured text does not say — steps to reproduce, expected or actual behaviour, environment — ask the user, one question at a time, and wait for the answer. Never invent details.",
    "- The user answers you here, in this pane.",
    "",
    "Output: write exactly one file, at this absolute path:",
    `  ${issuePath}`,
    "If that file already exists, it is the draft from an earlier session: read it and continue from it rather than starting over.",
    "with this structure:",
    "  # <a concise title>",
    "  ## Summary",
    "  ## Steps to reproduce",
    "  ## Expected behaviour",
    "  ## Actual behaviour",
    "  ## Environment",
    "  ## Notes",
    'Write "Unknown" under a heading only when the user has confirmed it is unknown.',
    "",
    `Progress: overwrite ${paths.stageFile} with a single word —`,
    "  shape    when you begin",
    "  drafted  once issue.md is written and complete",
    "",
    "After writing drafted, tell the user the draft is ready to review in Grove's Lab (inspector → Artifacts), then wait.",
    "If the user asks for changes, revise issue.md and write drafted again.",
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
    "Work through three skills, in order, as one continuous conversation with the user in this pane. When you reach each one, read its files in full and follow them, except where the Grove rules below say otherwise.",
    "",
    "1. Interview (grill-with-docs) — follow both:",
    `   ${skill("grilling", "SKILL.md")}`,
    `   ${skill("domain-modeling", "SKILL.md")}  (with CONTEXT-FORMAT.md and ADR-FORMAT.md beside it)`,
    `2. Spec — ${skill("to-spec", "SKILL.md")}`,
    `3. Tickets — ${skill("to-tickets", "SKILL.md")}`,
    "",
    "Move from one step to the next only when the user agrees the current one is done.",
    "",
    "GROVE RULES — these override the skills.",
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
    "After writing drafted, tell the user the drafts are ready to review in Grove's Lab (inspector → Artifacts), then wait.",
    "If the user asks for changes, revise the files and write drafted again.",
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

/** What a session reports to Grove. */
export type SessionPhase = "starting" | "working" | "question" | "permission" | "review";

export interface SessionReport {
  status: "running" | "blocked";
  current_step: string;
  steps: RuntimeStep[];
  summary: string;
}

/**
 * Derives the run's steps and status from the agent's state and the stage it
 * reported. A turn that ends without a draft is a question for the user; one
 * that ends with the draft written is waiting for review in Grove.
 */
export function sessionReport(
  kind: LabSessionKind,
  phase: SessionPhase,
  stage: string,
  paneId: string,
  startedAt: string,
): SessionReport {
  const steps = SESSION_STEPS[kind];
  let current = steps.findIndex((step) => step.stage === stage);
  if (current < 0) current = 0;
  const blocked = phase === "question" || phase === "permission" || phase === "review";
  const summary = {
    starting: "Starting the agent",
    working: `${steps[current].title} in progress`,
    question: `Waiting for your answer in pane ${paneId}`,
    permission: `The agent needs a permission decision in pane ${paneId}`,
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

export interface SessionDeps {
  runner: CommandRunner;
  sleep: (ms: number) => Promise<void>;
  exists: (file: string) => boolean;
  readStage: () => string;
  report: (report: SessionReport, paneId: string) => void;
  now: () => number;
}

/** How long an agent may be missing before the session gives up on it. */
const AGENT_GONE_GRACE_MS = 30_000;
/** How long after the prompt an idle agent is taken to have not started yet. */
const START_GRACE_MS = 60_000;
const POLL_MS = 1_000;

/**
 * Watches the agent until Grove closes the session, reporting each change of
 * phase. Returns when the close file appears.
 */
export async function watchSession(
  kind: LabSessionKind,
  agentName: string,
  paneId: string,
  closeFile: string,
  deps: SessionDeps,
): Promise<void> {
  const startedAt = new Date(deps.now()).toISOString();
  const promptedAt = deps.now();
  let phase: SessionPhase = "starting";
  let stage = "";
  let seenWorking = false;
  let goneSince: number | null = null;

  deps.report(sessionReport(kind, phase, stage, paneId, startedAt), paneId);
  while (!deps.exists(closeFile)) {
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

    const nextStage = deps.readStage();
    let next: SessionPhase = phase;
    if (state === "working") {
      seenWorking = true;
      next = "working";
    } else if (!seenWorking && deps.now() - promptedAt < START_GRACE_MS) {
      next = "starting";
    } else if (state === "blocked") {
      next = "permission";
    } else {
      next = nextStage === "drafted" ? "review" : "question";
    }
    if (next !== phase || nextStage !== stage) {
      phase = next;
      stage = nextStage;
      deps.report(sessionReport(kind, phase, stage, paneId, startedAt), paneId);
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
    return fs.readFileSync(file, "utf8").trim().toLowerCase();
  } catch {
    return "";
  }
}

async function main(kind: LabSessionKind, entryId: string): Promise<void> {
  const repo = process.cwd();
  const paths = labPaths(gitCommonDir(repo), entryId);
  const entry = readLabEntry(paths, entryId);
  fs.mkdirSync(paths.artifactsDir, { recursive: true });
  fs.rmSync(paths.closeFile, { force: true });
  fs.rmSync(paths.stageFile, { force: true });

  const title = `${kind === "shape" ? "Shape" : "Grill"}: ${entryTitle(entry)}`;
  updateTrackedWorkflow({ title, current_step: "Starting" });

  const launch = getAgentLaunchConfig();
  if (launch.backend === "claude") ensureClaudeWorkspaceTrust(repo);
  const promptText =
    kind === "shape"
      ? buildShapePrompt(entry, repo, paths)
      : buildGrillPrompt(entry, repo, paths, bundledSkillsDir());
  const assignmentPath = path.join(paths.entryDir, "assignment.md");
  fs.writeFileSync(assignmentPath, promptText, "utf8");

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
      ...launch.args,
    ]);
    runCommand("herdr", ["agent", "prompt", agentName, deliverablePrompt(promptText, assignmentPath)]);
    runCommand("herdr", ["agent", "focus", agentName]);

    await watchSession(kind, agentName, paneId, paths.closeFile, {
      runner: runCommand,
      sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
      exists: (file) => fs.existsSync(file),
      readStage: () => readStageFile(paths.stageFile),
      now: () => Date.now(),
      report: (report, pane) =>
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
    });
  } finally {
    try {
      runCommand("herdr", ["pane", "close", paneId]);
    } catch {
      // The user may have closed the pane already.
    }
    fs.rmSync(paths.closeFile, { force: true });
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
