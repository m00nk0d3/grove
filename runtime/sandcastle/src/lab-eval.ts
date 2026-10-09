#!/usr/bin/env node

// The Lab evaluation harness: runs real Lab sessions on the fixture entries
// in evals/fixtures against a chosen agent and model, playing the user from a
// script, and scores how well the agent followed the instructions. It needs a
// live model and a Herdr session, and does not run in CI. See
// docs/LAB_DESIGN.md, "Evaluation".
//
//   npm run lab-eval -- --agent claude --model sonnet [--only bug,vague-idea] [--minutes 30]

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { contentHash, readIfExists } from "./lab-drafts.js";
import { formatScorecard, scoreRun, scriptedAnswer, type EvalScore, type ScriptedAnswer } from "./lab-eval-score.js";
import type { LabStage } from "./lab-prompts.js";
import { answerFileName, isPending, scanQuestions } from "./lab-protocol.js";
import { labPaths, REVIEWED_DRAFT, type LabSessionKind, type SessionState } from "./lab-session.js";
import { AGENT_BACKENDS, type AgentBackend } from "./workflow-utils.js";

const RUNTIME_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const EVALS_DIR = path.join(RUNTIME_DIR, "evals");
const POLL_MS = 1_000;

interface Fixture {
  name: string;
  kind: "idea" | "bug";
  sessions: LabSessionKind[];
  text: string;
  overrides: Record<string, ScriptedAnswer>;
  revisions: { after: number; question: number; choices?: number[]; text?: string }[];
}

interface Options {
  agent: AgentBackend;
  model: string;
  only: string[];
  minutes: number;
}

export function parseArgs(argv: string[]): Options {
  const value = (flag: string) => {
    const at = argv.indexOf(flag);
    return at >= 0 ? argv[at + 1] : undefined;
  };
  const agent = value("--agent");
  if (!agent || !(AGENT_BACKENDS as readonly string[]).includes(agent)) {
    throw new Error(`--agent must be one of ${AGENT_BACKENDS.join(", ")}`);
  }
  const model = value("--model");
  if (!model) throw new Error("--model is required");
  const minutes = Number(value("--minutes") ?? "30");
  if (!(minutes > 0)) throw new Error("--minutes must be a positive number");
  return { agent: agent as AgentBackend, model, only: (value("--only") ?? "").split(",").filter(Boolean), minutes };
}

/** The environment that selects the agent and model for a session. */
export function agentEnv(agent: AgentBackend, model: string): NodeJS.ProcessEnv {
  const variable = { claude: "AGENT_FLOW_CLAUDE_MODEL", opencode: "AGENT_FLOW_OPENCODE_MODEL", pi: "AGENT_FLOW_PI_MODEL" }[agent];
  return { AGENT_FLOW_AGENT_BACKEND: agent, [variable]: model };
}

function git(cwd: string, ...args: string[]) {
  const result = spawnSync("git", args, {
    cwd,
    encoding: "utf8",
    env: { ...process.env, GIT_AUTHOR_NAME: "eval", GIT_AUTHOR_EMAIL: "eval@example.com", GIT_COMMITTER_NAME: "eval", GIT_COMMITTER_EMAIL: "eval@example.com" },
  });
  if (result.status !== 0) throw new Error(`git ${args.join(" ")}: ${result.stderr}`);
  return result.stdout.trim();
}

function listFiles(root: string, base = root): string[] {
  return fs.readdirSync(root, { withFileTypes: true }).flatMap((d) => {
    const full = path.join(root, d.name);
    if (d.name === ".git") return [];
    return d.isDirectory() ? listFiles(full, base) : [path.relative(base, full).split(path.sep).join("/")];
  });
}

/** A fresh checkout of the fixture repository with the entry captured. */
function prepare(fixture: Fixture): { checkout: string; entryId: string } {
  const checkout = fs.mkdtempSync(path.join(os.tmpdir(), `lab-eval-${fixture.name}-`));
  fs.cpSync(path.join(EVALS_DIR, "fixtures", "repo"), checkout, { recursive: true });
  git(checkout, "init", "-q");
  git(checkout, "add", "-A");
  git(checkout, "commit", "-q", "-m", "fixture");
  const labDir = path.join(checkout, ".git", "grove-lab");
  const entryId = `eval-${fixture.name}`;
  fs.mkdirSync(path.join(labDir, "repo-map"), { recursive: true });
  fs.copyFileSync(path.join(EVALS_DIR, "fixtures", "repo-map.md"), path.join(labDir, "repo-map", `${git(checkout, "rev-parse", "HEAD")}.md`));
  const now = new Date().toISOString();
  const entry = { id: entryId, kind: fixture.kind, text: fixture.text, status: "draft", reviews: {}, created: now, updated: now };
  fs.writeFileSync(path.join(labDir, "entries.json"), JSON.stringify({ version: 1, entries: [entry] }, null, 2));
  return { checkout, entryId };
}

/** Records the scripted user's approval of a draft, as Grove would. */
function approve(labDir: string, entryId: string, artifact: string, body: string) {
  const file = path.join(labDir, "entries.json");
  const index = JSON.parse(fs.readFileSync(file, "utf8"));
  const entry = index.entries.find((e: { id: string }) => e.id === entryId);
  entry.reviews = { ...entry.reviews, [artifact]: { state: "approved", hash: contentHash(body) } };
  fs.writeFileSync(file, JSON.stringify(index, null, 2));
}

/** Runs one session to the point of publishing, playing the user. */
async function runSession(
  fixture: Fixture,
  kind: LabSessionKind,
  checkout: string,
  entryId: string,
  options: Options,
  log: fs.WriteStream,
): Promise<{ outcome: EvalScore["outcome"]; last: SessionState | null; stageMinutes: Record<string, number> }> {
  const runId = `${entryId}-${kind}`;
  const paths = labPaths(path.join(checkout, ".git"), entryId, runId);
  const child = spawn(process.execPath, [path.join(RUNTIME_DIR, "dist", "lab-session.js"), kind, entryId], {
    cwd: checkout,
    env: { ...process.env, ...agentEnv(options.agent, options.model), GROVE_WORKFLOW_RUN_ID: runId },
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout.pipe(log, { end: false });
  child.stderr.pipe(log, { end: false });
  let exited = false;
  let failed = false;
  child.on("exit", (code) => {
    exited = true;
    failed = code !== 0;
  });

  const deadline = Date.now() + options.minutes * 60_000;
  const stageStarted = new Map<string, number>();
  const stageMinutes: Record<string, number> = {};
  let last: SessionState | null = null;
  let answered = 0;
  const revised = new Set<number>();
  let outcome: EvalScore["outcome"] = "timed-out";

  while (!exited) {
    if (Date.now() > deadline) break;
    const session = readIfExists(paths.sessionFile);
    if (session !== null) {
      try {
        last = JSON.parse(session) as SessionState;
      } catch {
        // Caught mid-write; read it on the next poll.
      }
    }
    if (last && !stageStarted.has(last.stage)) {
      for (const [stage, started] of stageStarted) stageMinutes[stage] ??= Math.round((Date.now() - started) / 6_000) / 10;
      stageStarted.set(last.stage, Date.now());
    }

    for (const record of scanQuestions(paths.questionsDir).filter(isPending)) {
      const reply = scriptedAnswer(record.question!, fixture.overrides[String(record.number)]);
      const answer = { id: record.number, ...reply, answered_at: new Date().toISOString() };
      fs.writeFileSync(path.join(paths.questionsDir, answerFileName(record.number)), JSON.stringify(answer));
      answered++;
      log.write(`[eval] Q${record.number}: ${record.question!.question} -> ${JSON.stringify(reply)}\n`);
    }
    for (const revision of fixture.revisions) {
      if (answered < revision.after || revised.has(revision.question) || last?.stage !== "interview") continue;
      const file = path.join(paths.questionsDir, answerFileName(revision.question));
      const previous = readIfExists(file);
      if (previous === null) continue;
      const old = JSON.parse(previous);
      const { choices, text } = revision;
      fs.writeFileSync(file, JSON.stringify({ ...old, choices, text, revisions: [...(old.revisions ?? []), { choices: old.choices, text: old.text }] }));
      revised.add(revision.question);
      log.write(`[eval] revised Q${revision.question}\n`);
    }

    if (last?.phase === "review") {
      if (last.stage === "publish") {
        outcome = "completed";
        fs.writeFileSync(paths.closeFile, "");
        break;
      }
      const artifact = REVIEWED_DRAFT[last.stage as LabStage];
      const body = artifact ? readIfExists(path.join(paths.artifactsDir, artifact)) : null;
      if (artifact && body !== null) approve(paths.labDir, entryId, artifact, body);
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_MS));
  }

  if (outcome === "timed-out" && !exited) fs.writeFileSync(paths.closeFile, "");
  for (let waited = 0; !exited && waited < 15_000; waited += 500) await new Promise((r) => setTimeout(r, 500));
  if (!exited) child.kill();
  for (const [stage, started] of stageStarted) stageMinutes[stage] ??= Math.round((Date.now() - started) / 6_000) / 10;
  if (failed && outcome !== "completed") outcome = "failed";
  return { outcome, last, stageMinutes };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (!process.env.HERDR_ENV) throw new Error("Run the evaluation from a Herdr pane: each session opens its agent beside it.");
  const { fixtures } = JSON.parse(fs.readFileSync(path.join(EVALS_DIR, "fixtures", "entries.json"), "utf8")) as { fixtures: Fixture[] };
  const chosen = fixtures.filter((f) => options.only.length === 0 || options.only.includes(f.name));
  const resultsDir = path.join(EVALS_DIR, "results");
  fs.mkdirSync(resultsDir, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const label = `${options.agent} ${options.model}`;
  const slug = `${stamp}-${options.agent}-${options.model.replace(/[^A-Za-z0-9_.-]+/g, "_")}`;
  const log = fs.createWriteStream(path.join(resultsDir, `${slug}.log`));

  const scores: EvalScore[] = [];
  for (const fixture of chosen) {
    console.log(`[eval] ${fixture.name}…`);
    const started = Date.now();
    const { checkout, entryId } = prepare(fixture);
    let outcome: EvalScore["outcome"] = "completed";
    const stageCounts: SessionState["stage_counts"] = {};
    const counts = { repairs: 0, fallbacks: 0 };
    const stageMinutes: Record<string, number> = {};
    const stages: LabStage[] = [];
    for (const kind of fixture.sessions) {
      const result = await runSession(fixture, kind, checkout, entryId, options, log);
      if (result.last) {
        counts.repairs += result.last.counts.repairs;
        counts.fallbacks += result.last.counts.fallbacks;
        Object.assign(stageCounts, result.last.stage_counts);
      }
      Object.assign(stageMinutes, result.stageMinutes);
      stages.push(...(Object.keys(result.stageMinutes).filter((s) => s !== "publish") as LabStage[]));
      if (result.outcome !== "completed") {
        outcome = result.outcome;
        break;
      }
    }
    const paths = labPaths(path.join(checkout, ".git"), entryId);
    const score = scoreRun({
      fixture: fixture.name,
      outcome,
      minutes: (Date.now() - started) / 60_000,
      repoFiles: listFiles(checkout),
      records: scanQuestions(paths.questionsDir),
      scout: readIfExists(paths.scoutFile),
      coverage: readIfExists(paths.coverageFile),
      counts,
      stageCounts,
      stageMinutes,
      stages,
    });
    scores.push(score);
    console.log(formatScorecard(label, [score]));
    console.log(`[eval] files kept in ${checkout}`);
  }
  log.end();
  fs.writeFileSync(path.join(resultsDir, `${slug}.json`), JSON.stringify({ agent: options.agent, model: options.model, scores }, null, 2));
  console.log(formatScorecard(label, scores));
  console.log(`[eval] results written to ${path.join(resultsDir, `${slug}.json`)}`);
}

const isEntrypoint =
  process.argv[1] !== undefined && fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  main().catch((error: unknown) => {
    console.error(`\x1b[31m[eval]\x1b[0m ${error instanceof Error ? error.message : String(error)}`);
    process.exit(1);
  });
}
