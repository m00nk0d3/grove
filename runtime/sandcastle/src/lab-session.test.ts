import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { contentHash } from "./lab-drafts.js";
import { buildStagePrompt } from "./lab-prompts.js";
import { scanQuestions, scanRequests, writeReceipt } from "./lab-protocol.js";
import {
  checkStageOutput,
  claudePathRule,
  entryTitle,
  labAgentArgs,
  labPaths,
  nextStep,
  parseAgentState,
  readRepoMap,
  resumeStep,
  SessionReporter,
  sessionReport,
  stageAgentName,
  stageValues,
  watchStage,
  type LabEntry,
  type LabPaths,
  type SessionDeps,
  type SessionReport,
  type SessionState,
} from "./lab-session.js";
import type { LabStage } from "./lab-prompts.js";
import type { AgentLaunchConfig } from "./workflow-utils.js";

const entry: LabEntry = {
  id: "20260928-081530-abcdef",
  kind: "bug",
  text: "\n  Sync stalls when the gh token expires\n\nThe dashboard freezes.",
  status: "draft",
};

test("Lab paths live under the git common directory and reject escaping IDs", () => {
  const paths = labPaths("/repo/.git", entry.id);
  assert.equal(paths.entryDir, path.join("/repo/.git", "grove-lab", entry.id));
  assert.equal(paths.artifactsDir, path.join(paths.entryDir, "artifacts"));
  assert.equal(paths.doneFile, path.join(paths.entryDir, "done"));
  assert.equal(paths.closeFile, path.join(paths.entryDir, "session.close"), "without a run ID the default name is used");
  assert.equal(labPaths("/repo/.git", entry.id, "run_42").closeFile, path.join(paths.entryDir, "run_42.close"),
    "each run has its own close file, so ending one session never touches another");
  assert.throws(() => labPaths("/repo/.git", entry.id, "../x"), /Invalid Lab entry or run ID/);
  assert.throws(() => labPaths("/repo/.git", "../outside"), /Invalid Lab entry or run ID/);
});

test("the entry title is its first non-blank line", () => {
  assert.equal(entryTitle(entry), "Sync stalls when the gh token expires");
  assert.equal(entryTitle({ ...entry, text: "  \n" }), "Untitled entry");
});

test("agent state is read from herdr agent get", () => {
  assert.equal(parseAgentState(JSON.stringify({ result: { agent: { agent_status: "working" } } })), "working");
  assert.equal(parseAgentState(JSON.stringify({ result: { agent: { agent_status: "idle" } } })), "idle");
  assert.equal(parseAgentState(JSON.stringify({ result: { agent: { agent_status: "sleeping" } } })), "unknown");
  assert.equal(parseAgentState("not json"), "unknown");
});

test("a grill runs scout, interview, spec, and tickets, then waits to publish", () => {
  assert.equal(nextStep("grill", "scout"), "interview");
  assert.equal(nextStep("grill", "interview"), "spec");
  assert.equal(nextStep("grill", "spec"), "tickets");
  assert.equal(nextStep("grill", "tickets"), "publish");
  assert.equal(nextStep("shape", "shape"), "publish");
  assert.notEqual(stageAgentName("scout", entry.id), stageAgentName("spec", entry.id), "each stage has its own agent");
  assert.ok(stageAgentName("interview", entry.id).length <= 32);
});

test("a report marks steps before the current one done and the current one blocked", () => {
  const report = sessionReport("grill", "question", "interview", "2026-09-28T08:00:00.000Z", [4]);
  assert.equal(report.status, "blocked");
  assert.equal(report.current_step, "Interview");
  assert.deepEqual(
    report.steps.map((step) => [step.title, step.status]),
    [["Scout", "succeeded"], ["Interview", "blocked"], ["Spec", "queued"], ["Tickets", "queued"], ["Publish", "queued"]],
  );
  assert.equal(report.summary, "Question 4 is waiting for you in Grove");
});

test("a shaped report is reviewed in its own step, and a spec in the Spec step", () => {
  const shaped = sessionReport("shape", "review", "shape", "t");
  assert.equal(shaped.current_step, "Review");
  assert.deepEqual(shaped.steps.map((step) => step.status), ["succeeded", "blocked", "queued"]);
  assert.match(shaped.summary, /bug report is ready to review/);
  const spec = sessionReport("grill", "review", "spec", "t");
  assert.equal(spec.current_step, "Spec");
  assert.match(spec.summary, /spec is ready to review/);
  const publish = sessionReport("grill", "review", "publish", "t");
  assert.equal(publish.current_step, "Publish", "Grove publishes at the Publish step");
});

test("every phase that waits on the user blocks the run, and none points at the pane", () => {
  for (const phase of ["question", "fallback", "permission", "review"] as const) {
    const report = sessionReport("grill", phase, "interview", "t", [1]);
    assert.equal(report.status, "blocked", phase);
    assert.doesNotMatch(report.summary, /pane/, phase);
  }
  for (const phase of ["starting", "working"] as const) {
    assert.equal(sessionReport("grill", phase, "interview", "t").status, "running", phase);
  }
});

/** A Lab directory with one entry, for tests that read and write its files. */
function labFixture(): LabPaths {
  const commonDir = fs.mkdtempSync(path.join(os.tmpdir(), "lab-session-"));
  const paths = labPaths(commonDir, entry.id, "run1");
  fs.mkdirSync(paths.artifactsDir, { recursive: true });
  fs.writeFileSync(path.join(paths.labDir, "entries.json"), JSON.stringify({ version: 1, entries: [{ ...entry, reviews: {} }] }));
  return paths;
}

function approve(paths: LabPaths, artifact: string) {
  const body = fs.readFileSync(path.join(paths.artifactsDir, artifact), "utf8");
  const index = JSON.parse(fs.readFileSync(path.join(paths.labDir, "entries.json"), "utf8"));
  index.entries[0].reviews[artifact] = { state: "approved", hash: contentHash(body) };
  fs.writeFileSync(path.join(paths.labDir, "entries.json"), JSON.stringify(index));
}

const SCOUT = "# Scout notes\n\n## Relevant files\n- a.go\n\n## How it works today\n- x\n\n## Constraints\n- y\n\n## Open questions\n1. z\n";
const SPEC =
  "# Offline mode\n\n## Problem\np\n\n## Goals\ng\n\n## Non-goals\nn\n\n## User stories\n1. u\n\n## Decisions\n- Q1\n\n## Design\nd\n\n## Testing\nt\n\n## Edge cases\ne\n\n## Open questions\nnone\n";
const COVERED = JSON.stringify({
  scope: "covered", triggers: "covered", data: "covered", interface: "covered", errors: "covered",
  concurrency: "n/a: one user", compatibility: "covered", testing: "covered", rollout: "covered",
});

test("a session resumes at the first stage whose work is not finished", () => {
  const paths = labFixture();
  assert.equal(resumeStep("grill", paths, entry.id), "scout");
  fs.writeFileSync(paths.scoutFile, SCOUT);
  assert.equal(resumeStep("grill", paths, entry.id), "interview");
  fs.writeFileSync(paths.doneFile, "interview\n");
  assert.equal(resumeStep("grill", paths, entry.id), "spec", "an interview finished just before a stop is not redone");
  fs.rmSync(paths.doneFile);
  fs.writeFileSync(path.join(paths.artifactsDir, "spec.md"), SPEC);
  assert.equal(resumeStep("grill", paths, entry.id), "spec");
  approve(paths, "spec.md");
  assert.equal(resumeStep("grill", paths, entry.id), "tickets");
  fs.writeFileSync(path.join(paths.artifactsDir, "spec.md"), SPEC + "\nrevised\n");
  assert.equal(resumeStep("grill", paths, entry.id), "spec", "an approval of earlier content no longer counts");
  assert.equal(resumeStep("shape", paths, entry.id), "shape");
});

test("each stage's output is checked before the session moves on", () => {
  const paths = labFixture();
  assert.match(checkStageOutput("scout", paths, false, 0)[0], /was not written/);
  fs.writeFileSync(paths.scoutFile, SCOUT);
  assert.deepEqual(checkStageOutput("scout", paths, false, 0), []);

  fs.writeFileSync(paths.coverageFile, COVERED.replace('"rollout":"covered"', '"rollout":"open"'));
  assert.match(checkStageOutput("interview", paths, false, 3).join(), /still lists rollout as open/);
  assert.deepEqual(checkStageOutput("interview", paths, true, 3), [], "the user may end the interview with topics open");
  assert.deepEqual(checkStageOutput("interview", paths, false, 25), [], "so may the 25-question check");
  fs.writeFileSync(paths.coverageFile, COVERED);
  assert.deepEqual(checkStageOutput("interview", paths, false, 3), []);

  fs.writeFileSync(path.join(paths.artifactsDir, "spec.md"), SPEC.replace("## Testing\nt\n\n", ""));
  assert.match(checkStageOutput("spec", paths, false, 0).join(), /"## Testing" section is missing/);
});

test("every stage prompt renders with every placeholder filled", () => {
  const paths = labFixture();
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "lab-repo-"));
  for (const stage of ["scout", "interview", "spec", "tickets", "shape"] as LabStage[]) {
    const prompt = buildStagePrompt(stage, repo, stageValues(stage, entry, repo, paths, "# Repository map", []));
    assert.doesNotMatch(prompt, /\{\{\w+\}\}/, stage);
    assert.ok(prompt.includes(paths.doneFile), `${stage} names its done file`);
    // The tickets stage works from the approved spec, not the capture.
    if (stage !== "tickets") assert.match(prompt, /The dashboard freezes\./, `${stage} carries the entry`);
  }
  const interview = buildStagePrompt("interview", repo, stageValues("interview", entry, repo, paths, "# Repository map", []));
  assert.ok(interview.includes(path.join(paths.questionsDir, "001.json")), "the first card's path is given");
});

test("a repository can replace a stage prompt", () => {
  const paths = labFixture();
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "lab-repo-"));
  fs.mkdirSync(path.join(repo, ".grove", "lab", "prompts"), { recursive: true });
  fs.writeFileSync(path.join(repo, ".grove", "lab", "prompts", "scout.md"), "Scout {{entry_title}} into {{scout_file}}.");
  const prompt = buildStagePrompt("scout", repo, stageValues("scout", entry, repo, paths, "", []));
  assert.equal(prompt, `Scout Sync stalls when the gh token expires into ${paths.scoutFile}.`);
});

test("the repository map is read for the checkout's commit", () => {
  const paths = labFixture();
  assert.match(readRepoMap(paths, "abc"), /no repository map/);
  fs.mkdirSync(path.join(paths.labDir, "repo-map"));
  fs.writeFileSync(path.join(paths.labDir, "repo-map", "abc.md"), "# Repository map\n");
  assert.equal(readRepoMap(paths, "abc"), "# Repository map");
});

type Step = { state: string; close?: boolean; act?: () => void };

/** The entry directory of the stage under test; script steps write into it. */
let current: LabPaths;

/**
 * Runs one stage against a scripted agent and a real entry directory. Each
 * poll reads the next step's agent state; a step's act runs as the poll
 * begins, standing in for the agent or Grove writing files.
 */
async function stage(name: LabStage, script: Step[], kind: "grill" | "shape" = "grill") {
  current = labFixture();
  const paths = current;
  let tick = 0;
  let clock = 0;
  const prompts: string[] = [];
  const keys: string[] = [];
  const reports: SessionReport[] = [];
  const sessions: SessionState[] = [];
  const step = () => script[Math.min(tick, script.length - 1)];
  const begin = () => {
    if (tick < script.length) script[tick].act?.();
  };
  const deps: SessionDeps = {
    runner: (command, args) => {
      assert.equal(command, "herdr");
      switch (`${args[0]} ${args[1]}`) {
        case "agent get":
          if (step().state === "gone") throw new Error("no such agent");
          return JSON.stringify({ result: { agent: { agent_status: step().state } } });
        case "agent prompt":
          prompts.push(args[3]);
          return "";
        case "agent read":
          return "Which database should I use?\n";
        case "agent send-keys":
          keys.push(args[3]);
          return "";
      }
      throw new Error(`unexpected herdr ${args.join(" ")}`);
    },
    sleep: async (ms) => {
      tick++;
      clock += ms;
      begin();
    },
    now: () => clock,
    closed: () => step().close === true,
    readDone: () => (fs.existsSync(paths.doneFile) ? fs.readFileSync(paths.doneFile, "utf8").trim() : ""),
    clearDone: () => fs.rmSync(paths.doneFile, { force: true }),
    checkStage: (finishRequested, answered) => checkStageOutput(name, paths, finishRequested, answered),
    approved: () => {
      const index = JSON.parse(fs.readFileSync(path.join(paths.labDir, "entries.json"), "utf8"));
      return Object.values(index.entries[0].reviews as Record<string, { state: string }>).some((r) => r.state === "approved");
    },
    report: (report) => reports.push(report),
    scanQuestions: () => scanQuestions(paths.questionsDir),
    scanRequests: () => scanRequests(paths.requestsDir),
    writeQuestionReceipt: (n, receipt) => writeReceipt(paths.questionsDir, n, receipt),
    writeRequestReceipt: (n, receipt) => writeReceipt(paths.requestsDir, n, receipt),
    writeSession: (state) => sessions.push(state),
  };
  begin();
  const reporter = new SessionReporter(kind, "Claude Code", deps);
  const result = { paths, prompts, keys, reports, sessions, outcome: undefined as unknown, error: undefined as unknown };
  try {
    result.outcome = await watchStage({ kind, stage: name, agentName: "lab-x", paneId: "w1:p3", agent: "Claude Code", paths }, deps, reporter);
  } catch (error) {
    result.error = error;
  }
  return result;
}

const card = (n: number, overrides: Record<string, unknown> = {}) => () =>
  writeFile(current.questionsDir, `${String(n).padStart(3, "0")}.json`, {
    id: n,
    kind: "choice",
    question: `Question ${n}?`,
    context: "lab.go starts sessions per entry.",
    options: ["Yes", "No"],
    recommended: 0,
    why: "Simpler.",
    ...overrides,
  });
const rawCard = (n: number, raw: string) => () =>
  fs.writeFileSync(path.join(current.questionsDir, `${String(n).padStart(3, "0")}.json`), raw);
const answer = (n: number, body: Record<string, unknown>) => () =>
  writeFile(current.questionsDir, `${String(n).padStart(3, "0")}.answer.json`, { id: n, ...body });
const request = (n: number, body: Record<string, unknown>) => () =>
  writeFile(current.requestsDir, `${String(n).padStart(3, "0")}.json`, { id: n, ...body });
const write = (file: () => string, body: string) => () => fs.writeFileSync(file(), body);
const finish = (name: string) => write(() => current.doneFile, `${name}\n`);
const both = (...acts: (() => void)[]) => () => acts.forEach((act) => act());

function writeFile(dir: string, name: string, value: unknown) {
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, name), JSON.stringify(value));
}

const phases = (sessions: SessionState[]) => sessions.map((state) => state.phase);

test("an answer given in Grove is delivered once every card has one", async () => {
  const run = await stage("interview", [
    { state: "working" },
    { state: "idle", act: card(1) },
    { state: "idle" },
    { state: "idle", act: answer(1, { choices: [1], text: "Keep it simple" }) },
    { state: "working" },
    { state: "idle", close: true },
  ]);
  assert.equal(run.error, undefined);
  assert.equal(run.outcome, "closed");
  assert.deepEqual(phases(run.sessions), ["starting", "working", "question", "working"]);
  assert.deepEqual(run.sessions[2].pending, [1]);
  assert.equal(run.prompts.length, 1);
  assert.match(run.prompts[0], /^Answer to question 1: "No" \(option 2\)\.\nUser note: "Keep it simple"/);
  assert.ok(run.prompts[0].includes(path.join(current.questionsDir, "002.json")), "the next card's path is named");
  assert.ok(run.prompts[0].includes(`write interview to ${current.doneFile}`), "and how to finish the interview");
  assert.equal(scanQuestions(current.questionsDir)[0].receipt?.via, "grove");
});

test("several cards from one turn are answered in one prompt", async () => {
  const run = await stage("interview", [
    { state: "working" },
    { state: "idle", act: both(card(1), card(2, { kind: "text", options: undefined, recommended: "Postgres" })) },
    { state: "idle", act: answer(1, { choices: [0] }) },
    { state: "idle" },
    { state: "idle", act: answer(2, { text: "SQLite" }) },
    { state: "working" },
    { state: "idle", close: true },
  ]);
  assert.equal(run.prompts.length, 1, "nothing is sent while a card is still waiting");
  assert.match(run.prompts[0], /Answer to question 1: "Yes" \(option 1\)\.\n\nAnswer to question 2: "SQLite"\./);
  assert.ok(run.prompts[0].includes("003.json"), "numbering continues from the highest card");
});

test("an invalid card is repaired at most twice, then the turn falls back to a reply", async () => {
  const run = await stage("interview", [
    { state: "working" },
    { state: "idle", act: card(1, { recommended: 3 }) },
    { state: "working" },
    { state: "idle", act: rawCard(1, "{ not json") },
    { state: "working" },
    { state: "idle", act: card(1, { recommended: 9 }) },
    { state: "idle" },
    { state: "idle", close: true },
  ]);
  assert.equal(run.prompts.length, 2);
  assert.match(run.prompts[0], /^001\.json is invalid: "recommended" is 3 but must be an option index from 0 to 1\./);
  assert.match(run.prompts[1], /001\.json is invalid: the file is not valid JSON/);
  const last = run.sessions.at(-1)!;
  assert.equal(last.phase, "fallback");
  assert.deepEqual(last.counts, { repairs: 2, fallbacks: 1 });
  assert.deepEqual(last.stage_counts.interview, { repairs: 2, fallbacks: 1 }, "counts are kept per stage");
  assert.match(last.output ?? "", /Which database/);
});

test("a turn without a card shows the agent's output, and a reply is delivered", async () => {
  const run = await stage("shape", [
    { state: "working" },
    { state: "idle" },
    { state: "idle", act: request(1, { kind: "reply", text: "It happens on Windows only" }) },
    { state: "working" },
    { state: "idle", close: true },
  ], "shape");
  assert.deepEqual(phases(run.sessions).slice(0, 3), ["starting", "working", "fallback"]);
  assert.equal(run.sessions[2].output, "Which database should I use?");
  assert.match(run.prompts[0], /^The user replies: "It happens on Windows only"\n\nNext: write question 1 to /);
  assert.match(run.prompts[0], /write .*issue\.md, then write shape to /, "a shaping agent is told how to finish");
});

test("a revised answer is delivered again as a revision", async () => {
  const run = await stage("interview", [
    { state: "working" },
    { state: "idle", act: card(1) },
    { state: "idle", act: answer(1, { choices: [0] }) },
    { state: "working" },
    { state: "idle", act: card(2) },
    { state: "idle", act: both(answer(1, { choices: [1], revisions: [{ choices: [0] }] }), answer(2, { choices: [0] })) },
    { state: "working" },
    { state: "idle", close: true },
  ]);
  assert.equal(run.prompts.length, 2);
  assert.match(run.prompts[1], /^Revision to question 1: was "Yes" \(option 1\), now "No" \(option 2\)\./);
  assert.match(run.prompts[1], /Answer to question 2: "Yes"/);
  assert.equal(scanQuestions(current.questionsDir)[0].receipt?.revision, 1);
});

test("cards left behind by a turn typed in the pane are closed as answered there", async () => {
  const run = await stage("interview", [
    { state: "working" },
    { state: "idle", act: card(1) },
    { state: "idle" },
    { state: "working" }, // the user typed in the pane
    { state: "idle", act: card(2) },
    { state: "idle", close: true },
  ]);
  assert.equal(run.prompts.length, 0);
  const [first, second] = scanQuestions(current.questionsDir);
  assert.equal(first.receipt?.via, "pane");
  assert.equal(second.receipt, undefined);
});

test("a permission prompt is shown with the agent's output and answered by keys", async () => {
  const run = await stage("scout", [
    { state: "working" },
    { state: "blocked" },
    { state: "blocked", act: request(1, { kind: "permission", allow: true }) },
    { state: "working" },
    { state: "blocked", act: request(2, { kind: "permission", allow: false }) },
    { state: "idle", close: true },
  ]);
  assert.equal(run.sessions.find((state) => state.phase === "permission")?.output, "Which database should I use?");
  assert.deepEqual(run.keys, ["enter", "esc"]);
});

test("a stage without review finishes once its output is valid", async () => {
  const run = await stage("scout", [
    { state: "working" },
    { state: "idle", act: finish("scout") },
    { state: "working" },
    { state: "idle", act: both(write(() => current.scoutFile, SCOUT), finish("scout")) },
    { state: "idle", close: true },
  ]);
  assert.equal(run.outcome, "finished");
  assert.equal(run.prompts.length, 1, "the missing notes were asked for once");
  assert.match(run.prompts[0], /^What you wrote has problems:\n- .*scout\.md was not written/);
  assert.match(run.prompts[0], /then write scout to /);
});

test("an interview that leaves topics open is sent back, unless the user ended it", async () => {
  const open = COVERED.replace('"rollout":"covered"', '"rollout":"open"');
  const sentBack = await stage("interview", [
    { state: "working" },
    { state: "idle", act: both(write(() => current.coverageFile, open), finish("interview")) },
    { state: "idle", close: true },
  ]);
  assert.match(sentBack.prompts[0], /still lists rollout as open/);

  const ended = await stage("interview", [
    { state: "working" },
    { state: "idle", act: card(1) },
    { state: "idle", act: request(1, { kind: "finish_interview" }) },
    { state: "working" },
    { state: "idle", act: both(write(() => current.coverageFile, open), finish("interview")) },
    { state: "idle", close: true },
  ]);
  assert.match(ended.prompts[0], /The user has ended the interview/);
  assert.equal(ended.outcome, "finished", "an ended interview may leave topics open");
  assert.equal(scanQuestions(current.questionsDir)[0].receipt?.via, "skipped", "its open card is closed");
});

test("a reviewed draft waits for approval, and a change request reaches its agent", async () => {
  const specFile = () => path.join(current.artifactsDir, "spec.md");
  const run = await stage("spec", [
    { state: "working" },
    { state: "idle", act: both(write(specFile, SPEC), finish("spec")) },
    { state: "idle", act: request(1, { kind: "change", text: "Mention the proxy" }) },
    { state: "working" },
    { state: "idle", act: both(write(specFile, SPEC + "\nproxy\n"), finish("spec")) },
    { state: "idle", act: () => approve(current, "spec.md") },
    { state: "idle", close: true },
  ]);
  assert.equal(run.outcome, "finished", "approval moves the session on");
  assert.deepEqual(phases(run.sessions), ["starting", "working", "review", "working", "review"]);
  assert.match(run.prompts[0], /Mention the proxy/);
  assert.match(run.prompts[0], /Record anything you cannot decide as an open question/, "the spec stage asks no questions");
  assert.ok(run.prompts[0].includes(`write spec to ${current.doneFile}`));
});

test("a draft that stays invalid is presented for review with its problems", async () => {
  const specFile = () => path.join(current.artifactsDir, "spec.md");
  const broken = SPEC.replace("## Testing\nt\n\n", "");
  const run = await stage("spec", [
    { state: "working" },
    { state: "idle", act: both(write(specFile, broken), finish("spec")) },
    { state: "working" },
    { state: "idle", act: finish("spec") },
    { state: "working" },
    { state: "idle", act: finish("spec") },
    { state: "idle", close: true },
  ]);
  assert.equal(run.prompts.length, 2);
  const review = run.sessions.at(-1)!;
  assert.equal(review.phase, "review");
  assert.match(review.problems?.join() ?? "", /"## Testing" section is missing/);
});

test("an idle agent that never started is not reported as waiting on the user", async () => {
  const run = await stage("scout", [{ state: "idle" }, { state: "idle" }, { state: "idle", close: true }]);
  assert.deepEqual(phases(run.sessions), ["starting"]);
});

test("a stage fails once its agent has been gone for the grace period", async () => {
  const run = await stage("scout", [{ state: "working" }, { state: "gone" }]);
  assert.match(String(run.error), /no longer running/);
});

test("Claude is confined to reading and to writing its entry, without prompts", () => {
  assert.equal(claudePathRule("C:\\Users\\me\\repo\\.git\\grove-lab\\x"), "//c/Users/me/repo/.git/grove-lab/x");
  assert.equal(claudePathRule("/home/me/repo/.git/grove-lab/x/"), "//home/me/repo/.git/grove-lab/x");
  const paths = labPaths("/repo/.git", entry.id);
  const claude: AgentLaunchConfig = { backend: "claude", kind: "claude", label: "Claude Code", args: ["--", "--x"], needsLmStudioEnv: false };
  const args = labAgentArgs(claude, paths, { AGENT_FLOW_CLAUDE_MODEL: "sonnet" });
  assert.deepEqual(args.slice(0, 7), ["--", "--model", "sonnet", "--permission-mode", "dontAsk", "--add-dir", paths.entryDir]);
  const tools = args.slice(args.indexOf("--allowedTools") + 1);
  assert.ok(tools.includes("Read") && tools.includes("Grep") && tools.includes("Bash(git log:*)"));
  assert.ok(!tools.includes("Write") && !tools.includes("Edit") && !tools.includes("Bash"), "no unscoped write or shell");
  assert.equal(tools.at(-1), `Edit(${claudePathRule(paths.entryDir)}/**)`);
  const opencode: AgentLaunchConfig = { ...claude, backend: "opencode", kind: "opencode", args: ["--", "--auto"] };
  assert.deepEqual(labAgentArgs(opencode, paths), ["--", "--auto"], "other backends keep their arguments");
});
