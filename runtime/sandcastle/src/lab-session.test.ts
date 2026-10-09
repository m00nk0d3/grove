import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { scanQuestions, scanRequests, writeReceipt } from "./lab-protocol.js";
import type { AgentLaunchConfig } from "./workflow-utils.js";
import {
  buildShapePrompt,
  claudePathRule,
  entryTitle,
  labAgentArgs,
  labPaths,
  parseAgentState,
  sessionReport,
  watchSession,
  type LabEntry,
  type LabPaths,
  type SessionDeps,
  type SessionReport,
  type SessionState,
} from "./lab-session.js";

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
  assert.equal(paths.stageFile, path.join(paths.entryDir, "stage"));
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

test("the shape prompt names its output, markers, and limits", () => {
  const paths = labPaths("/repo/.git", entry.id);
  const prompt = buildShapePrompt(entry, "/repo", paths);
  assert.match(prompt, /The dashboard freezes\./, "the captured text is included");
  assert.ok(prompt.includes(path.join(paths.artifactsDir, "issue.md")), "the output path is absolute");
  assert.ok(prompt.includes(paths.stageFile));
  for (const heading of ["## Summary", "## Steps to reproduce", "## Expected behaviour", "## Actual behaviour", "## Environment", "## Notes"]) {
    assert.ok(prompt.includes(heading), heading);
  }
  assert.match(prompt, /Never invent details/);
  assert.match(prompt, /Do not publish/);
});

test("agent state is read from herdr agent get", () => {
  assert.equal(parseAgentState(JSON.stringify({ result: { agent: { agent_status: "working" } } })), "working");
  assert.equal(parseAgentState(JSON.stringify({ result: { agent: { agent_status: "idle" } } })), "idle");
  assert.equal(parseAgentState(JSON.stringify({ result: { agent: { agent_status: "sleeping" } } })), "unknown");
  assert.equal(parseAgentState("not json"), "unknown");
});

test("a report marks steps before the current stage done and the current one blocked", () => {
  const report = sessionReport("grill", "question", "spec", "2026-09-28T08:00:00.000Z", [4]);
  assert.equal(report.status, "blocked");
  assert.equal(report.current_step, "Spec");
  assert.deepEqual(
    report.steps.map((step) => [step.title, step.status]),
    [["Interview", "succeeded"], ["Spec", "blocked"], ["Tickets", "queued"], ["Publish", "queued"]],
  );
  assert.equal(report.summary, "Question 4 is waiting for you in Grove");
});

test("a finished draft waits for review in Grove, not for an answer", () => {
  const report = sessionReport("shape", "review", "drafted", "t");
  assert.equal(report.status, "blocked");
  assert.equal(report.current_step, "Publish");
  assert.deepEqual(report.steps.map((step) => step.status), ["succeeded", "blocked"]);
  assert.match(report.summary, /ready to review in Grove/);
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

type Step = { state: string; stage?: string; close?: boolean; act?: () => void };

/**
 * Runs a session against a scripted agent and a real entry directory. Each
 * poll reads the next step's agent state; a step's act runs as the poll
 * begins, standing in for the agent or Grove writing files.
 */
async function runSessionAt(kind: "shape" | "grill", script: Step[]) {
  const paths = current;
  fs.mkdirSync(paths.artifactsDir, { recursive: true });
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
    exists: () => step().close === true,
    readStage: () => step().stage ?? "",
    report: (report) => reports.push(report),
    now: () => clock,
    scanQuestions: () => scanQuestions(paths.questionsDir),
    scanRequests: () => scanRequests(paths.requestsDir),
    writeQuestionReceipt: (n, receipt) => writeReceipt(paths.questionsDir, n, receipt),
    writeRequestReceipt: (n, receipt) => writeReceipt(paths.requestsDir, n, receipt),
    writeSession: (state) => sessions.push(state),
  };
  begin();
  const result = { paths, prompts, keys, reports, sessions, error: undefined as unknown };
  try {
    await watchSession({ kind, agentName: "lab-x", paneId: "w1:p3", agent: "Claude Code", paths }, deps);
  } catch (error) {
    result.error = error;
  }
  return result;
}

/** The entry directory of the session under test; script steps write into it. */
let current: LabPaths;
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

function writeFile(dir: string, name: string, value: unknown) {
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, name), JSON.stringify(value));
}

async function session(kind: "shape" | "grill", script: Step[]) {
  const commonDir = fs.mkdtempSync(path.join(os.tmpdir(), "lab-paths-"));
  current = labPaths(commonDir, entry.id, "run1");
  const result = await runSessionAt(kind, script);
  return result;
}

const phases = (sessions: SessionState[]) => sessions.map((state) => state.phase);

test("an answer given in Grove is delivered once every card has one", async () => {
  const run = await session("grill", [
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: card(1) },
    { state: "idle", stage: "interview" },
    { state: "idle", stage: "interview", act: answer(1, { choices: [1], text: "Keep it simple" }) },
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", close: true },
  ]);
  assert.equal(run.error, undefined);
  assert.deepEqual(phases(run.sessions), ["starting", "working", "question", "working"]);
  assert.deepEqual(run.sessions[2].pending, [1]);
  assert.equal(run.prompts.length, 1);
  assert.match(run.prompts[0], /^Answer to question 1: "No" \(option 2\)\.\nUser note: "Keep it simple"/);
  assert.ok(run.prompts[0].includes(path.join(current.questionsDir, "002.json")), "the next card's path is named");
  const records = scanQuestions(current.questionsDir);
  assert.deepEqual(records[0].receipt && { revision: records[0].receipt.revision, via: records[0].receipt.via }, {
    revision: 0,
    via: "grove",
  });
});

test("several cards from one turn are answered in one prompt", async () => {
  const run = await session("grill", [
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: () => (card(1)(), card(2, { kind: "text", options: undefined, recommended: "Postgres" })()) },
    { state: "idle", stage: "interview", act: answer(1, { choices: [0] }) },
    { state: "idle", stage: "interview" },
    { state: "idle", stage: "interview", act: answer(2, { text: "SQLite" }) },
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", close: true },
  ]);
  assert.equal(run.prompts.length, 1, "nothing is sent while a card is still waiting");
  assert.match(run.prompts[0], /Answer to question 1: "Yes" \(option 1\)\.\n\nAnswer to question 2: "SQLite"\./);
  assert.ok(run.prompts[0].includes("003.json"), "numbering continues from the highest card");
  assert.deepEqual(run.sessions.find((state) => state.phase === "question")?.pending, [1, 2]);
});

test("an invalid card is repaired at most twice, then the turn falls back to a reply", async () => {
  const run = await session("grill", [
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: card(1, { recommended: 3 }) },
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: rawCard(1, "{ not json") },
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: card(1, { recommended: 9 }) },
    { state: "idle", stage: "interview" },
    { state: "idle", stage: "interview", close: true },
  ]);
  assert.equal(run.prompts.length, 2);
  assert.match(run.prompts[0], /^001\.json is invalid: "recommended" is 3 but must be an option index from 0 to 1\./);
  assert.match(run.prompts[1], /001\.json is invalid: the file is not valid JSON/);
  const last = run.sessions.at(-1)!;
  assert.equal(last.phase, "fallback");
  assert.deepEqual(last.counts, { repairs: 2, fallbacks: 1 });
  assert.match(last.output ?? "", /Which database/);
});

test("a turn without a card shows the agent's output, and a reply is delivered", async () => {
  const run = await session("shape", [
    { state: "working", stage: "shape" },
    { state: "idle", stage: "shape" },
    { state: "idle", stage: "shape", act: request(1, { kind: "reply", text: "It happens on Windows only" }) },
    { state: "working", stage: "shape" },
    { state: "idle", stage: "shape", close: true },
  ]);
  assert.deepEqual(phases(run.sessions).slice(0, 3), ["starting", "working", "fallback"]);
  assert.equal(run.sessions[2].output, "Which database should I use?");
  assert.equal(run.prompts.length, 1);
  assert.match(run.prompts[0], /^The user replies: "It happens on Windows only"\n\nNext: write question 1 to /);
  assert.match(run.prompts[0], /write .*issue\.md, then write drafted/, "a shaping agent is told how to finish");
  assert.equal(scanRequests(current.requestsDir)[0].receipt?.via, "grove");
});

test("a revised answer is delivered again as a revision", async () => {
  const run = await session("grill", [
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: card(1) },
    { state: "idle", stage: "interview", act: answer(1, { choices: [0] }) },
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: card(2) },
    {
      state: "idle",
      stage: "interview",
      act: () => (answer(1, { choices: [1], revisions: [{ choices: [0] }] })(), answer(2, { choices: [0] })()),
    },
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", close: true },
  ]);
  assert.equal(run.prompts.length, 2);
  assert.match(run.prompts[1], /^Revision to question 1: was "Yes" \(option 1\), now "No" \(option 2\)\./);
  assert.match(run.prompts[1], /ask about it again/);
  assert.match(run.prompts[1], /Answer to question 2: "Yes"/);
  assert.equal(scanQuestions(current.questionsDir)[0].receipt?.revision, 1);
});

test("cards left behind by a turn typed in the pane are closed as answered there", async () => {
  const run = await session("grill", [
    { state: "working", stage: "interview" },
    { state: "idle", stage: "interview", act: card(1) },
    { state: "idle", stage: "interview" },
    { state: "working", stage: "interview" }, // the user typed in the pane
    { state: "idle", stage: "interview", act: card(2) },
    { state: "idle", stage: "interview", close: true },
  ]);
  assert.equal(run.prompts.length, 0);
  const [first, second] = scanQuestions(current.questionsDir);
  assert.equal(first.receipt?.via, "pane");
  assert.equal(second.receipt, undefined);
  assert.deepEqual(run.sessions.at(-1)?.pending, [2]);
});

test("a permission prompt is shown with the agent's output and answered by keys", async () => {
  const run = await session("grill", [
    { state: "working", stage: "interview" },
    { state: "blocked", stage: "interview" },
    { state: "blocked", stage: "interview", act: request(1, { kind: "permission", allow: true }) },
    { state: "working", stage: "interview" },
    { state: "blocked", stage: "interview", act: request(2, { kind: "permission", allow: false }) },
    { state: "idle", stage: "interview", close: true },
  ]);
  assert.equal(run.sessions.find((state) => state.phase === "permission")?.output, "Which database should I use?");
  assert.deepEqual(run.keys, ["enter", "esc"]);
  assert.ok(scanRequests(current.requestsDir).every((record) => record.receipt));
});

test("a finished draft waits for review, and a change request reaches the agent", async () => {
  const run = await session("shape", [
    { state: "working", stage: "shape" },
    { state: "idle", stage: "drafted" },
    { state: "idle", stage: "drafted", act: request(1, { kind: "change", text: "Mention the proxy" }) },
    { state: "working", stage: "shape" },
    { state: "idle", stage: "drafted" },
    { state: "idle", stage: "drafted", close: true },
  ]);
  assert.deepEqual(phases(run.sessions), ["starting", "working", "review", "working", "review"]);
  assert.match(run.prompts[0], /Mention the proxy/);
  assert.ok(run.prompts[0].includes(`write drafted to ${current.stageFile}`));
});

test("an idle agent that never started is not reported as waiting on the user", async () => {
  const run = await session("shape", [{ state: "idle" }, { state: "idle" }, { state: "idle", close: true }]);
  assert.deepEqual(phases(run.sessions), ["starting"]);
});

test("a session fails once its agent has been gone for the grace period", async () => {
  const run = await session("shape", [{ state: "working", stage: "shape" }, { state: "gone" }]);
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

test("the grill prompt points at the bundled skills and overrides where they publish", async () => {
  const { buildGrillPrompt, bundledSkillsDir } = await import("./lab-session.js");
  const idea: LabEntry = { ...entry, kind: "idea", text: "Offline mode\n\nCache the last sync." };
  const paths = labPaths("/repo/.git", idea.id);
  const skills = bundledSkillsDir();
  const prompt = buildGrillPrompt(idea, "/repo", paths, skills);

  for (const file of [
    ["grilling", "SKILL.md"],
    ["domain-modeling", "SKILL.md"],
    ["to-spec", "SKILL.md"],
    ["to-tickets", "SKILL.md"],
  ]) {
    const full = path.join(skills, ...file);
    assert.ok(prompt.includes(full), `the prompt names ${file.join("/")}`);
    assert.ok(fs.existsSync(full), `${file.join("/")} ships with the runtime`);
  }
  assert.ok(fs.existsSync(path.join(skills, "domain-modeling", "CONTEXT-FORMAT.md")));
  assert.ok(fs.existsSync(path.join(skills, "LICENSE")), "the license ships with the skills");

  assert.match(prompt, /Cache the last sync\./);
  assert.ok(prompt.includes(path.join(paths.artifactsDir, "spec.md")));
  assert.ok(prompt.includes(path.join(paths.artifactsDir, "tickets.json")));
  assert.ok(prompt.includes(path.join(paths.artifactsDir, "docs", "adr", "0003-<slug>.md")), "repository documents mirror their paths");
  assert.match(prompt, /Never create or change a file in the repository/);
  assert.match(prompt, /ignore any instruction about \/setup-matt-pocock-skills/);
  for (const stage of ["interview", "spec", "tickets", "drafted"]) {
    assert.match(prompt, new RegExp(`^  ${stage} `, "m"), stage);
  }
});

test("an escalated bug's grill starts from its shaped report", async () => {
  const { buildGrillPrompt } = await import("./lab-session.js");
  const paths = labPaths("/repo/.git", entry.id);
  const report = "# Sync stalls on token expiry\n\n## Summary\nThe dashboard freezes.";
  const prompt = buildGrillPrompt(entry, "/repo", paths, "/skills", report);
  assert.match(prompt, /began as a bug and was shaped into the report below/);
  assert.ok(prompt.includes(report));
  assert.ok(prompt.indexOf(report) < prompt.indexOf("Work through three skills"), "the report comes before the skills");
  assert.doesNotMatch(buildGrillPrompt(entry, "/repo", paths, "/skills"), /began as a bug/);
});
