import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";
import {
  buildShapePrompt,
  entryTitle,
  labPaths,
  parseAgentState,
  sessionReport,
  watchSession,
  type LabEntry,
  type SessionReport,
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
  const report = sessionReport("grill", "question", "spec", "w1:p3", "2026-09-28T08:00:00.000Z");
  assert.equal(report.status, "blocked");
  assert.equal(report.current_step, "Spec");
  assert.deepEqual(
    report.steps.map((step) => [step.title, step.status]),
    [["Interview", "succeeded"], ["Spec", "blocked"], ["Tickets", "queued"], ["Publish", "queued"]],
  );
  assert.match(report.summary, /Waiting for your answer in pane w1:p3/);
});

test("a finished draft waits for review in Grove, not for an answer", () => {
  const report = sessionReport("shape", "review", "drafted", "w1:p3", "t");
  assert.equal(report.status, "blocked");
  assert.equal(report.current_step, "Publish");
  assert.deepEqual(report.steps.map((step) => step.status), ["succeeded", "blocked"]);
  assert.match(report.summary, /ready to review in Grove/);
});

/** Scripts one conversation: each poll returns the next agent state and stage. */
function scriptedSession(script: { state: string; stage?: string; close?: boolean }[]) {
  let tick = 0;
  let clock = 0;
  const reports: SessionReport[] = [];
  const deps = {
    runner: (command: string, args: string[]) => {
      assert.equal(command, "herdr");
      assert.deepEqual(args.slice(0, 2), ["agent", "get"]);
      const step = script[Math.min(tick, script.length - 1)];
      if (step.state === "gone") throw new Error("no such agent");
      return JSON.stringify({ result: { agent: { agent_status: step.state } } });
    },
    sleep: async (ms: number) => {
      tick++;
      clock += ms;
    },
    exists: () => script[Math.min(tick, script.length - 1)].close === true,
    readStage: () => script[Math.min(tick, script.length - 1)].stage ?? "",
    report: (report: SessionReport) => reports.push(report),
    now: () => clock,
  };
  return { deps, reports };
}

test("a session reports the conversation until Grove closes it", async () => {
  const { deps, reports } = scriptedSession([
    { state: "idle" }, // prompt delivered, agent not started yet
    { state: "working", stage: "shape" },
    { state: "working", stage: "shape" },
    { state: "idle", stage: "shape" }, // turn ended with a question
    { state: "working", stage: "shape" }, // the user answered
    { state: "blocked", stage: "shape" }, // a permission prompt
    { state: "working", stage: "shape" },
    { state: "idle", stage: "drafted" }, // issue.md written
    { state: "idle", stage: "drafted", close: true },
  ]);
  await watchSession("shape", "lab-shape-x", "w1:p3", "/close", deps);

  assert.deepEqual(
    reports.map((report) => `${report.status}:${report.current_step}:${report.summary.split(" ")[0]}`),
    [
      "running:Shape:Starting",
      "running:Shape:Shape",
      "blocked:Shape:Waiting",
      "running:Shape:Shape",
      "blocked:Shape:The",
      "running:Shape:Shape",
      "blocked:Publish:The",
    ],
  );
  assert.match(reports[4].summary, /permission decision/);
  assert.match(reports[6].summary, /ready to review/);
});

test("an idle agent that never started is not reported as waiting on the user", async () => {
  const { deps, reports } = scriptedSession([
    { state: "idle" },
    { state: "idle" },
    { state: "idle", close: true },
  ]);
  await watchSession("shape", "lab-shape-x", "w1:p3", "/close", deps);
  assert.deepEqual(reports.map((report) => report.status), ["running"]);
});

test("a session fails once its agent has been gone for the grace period", async () => {
  const { deps } = scriptedSession([{ state: "working", stage: "shape" }, { state: "gone" }]);
  await assert.rejects(
    watchSession("shape", "lab-shape-x", "w1:p3", "/close", deps),
    /no longer running/,
  );
});

test("the grill prompt points at the bundled skills and overrides where they publish", async () => {
  const { buildGrillPrompt, bundledSkillsDir } = await import("./lab-session.js");
  const fs = await import("node:fs");
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
