import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { agentPaths, labWorkRoot, LabWorkSync } from "./lab-work.js";

function fixture() {
  const lab = agentPaths(fs.mkdtempSync(path.join(os.tmpdir(), "lab-entry-")));
  const work = agentPaths(path.join(fs.mkdtempSync(path.join(os.tmpdir(), "lab-work-")), "stage"));
  const sync = new LabWorkSync(lab, work);
  const write = (file: string, body: string) => {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, body);
  };
  const read = (file: string) => (fs.existsSync(file) ? fs.readFileSync(file, "utf8") : null);
  return { lab, work, sync, write, read };
}

test("the working folder starts with what the entry holds", () => {
  const { lab, work, sync, write, read } = fixture();
  write(lab.scoutFile, "# Scout notes\n");
  write(path.join(lab.artifactsDir, "docs", "adr", "0001-x.md"), "# X\n");
  write(path.join(lab.questionsDir, "001.json"), "{}");
  write(path.join(lab.questionsDir, "001.answer.json"), "{}");
  sync.seed();
  assert.equal(read(work.scoutFile), "# Scout notes\n");
  assert.equal(read(path.join(work.artifactsDir, "docs", "adr", "0001-x.md")), "# X\n");
  assert.equal(read(path.join(work.questionsDir, "001.json")), "{}");
  assert.equal(read(path.join(work.questionsDir, "001.answer.json")), null, "answers stay with Grove");
});

test("what the agent writes reaches the entry", () => {
  const { lab, work, sync, write, read } = fixture();
  sync.seed();
  write(path.join(work.questionsDir, "001.json"), '{"id":1}');
  write(path.join(work.artifactsDir, "spec.md"), "# Spec\n");
  write(work.doneFile, "spec\n");
  write(path.join(work.questionsDir, "notes.txt"), "scratch");
  sync.sync();
  assert.equal(read(path.join(lab.questionsDir, "001.json")), '{"id":1}');
  assert.equal(read(path.join(lab.artifactsDir, "spec.md")), "# Spec\n");
  assert.equal(read(lab.doneFile), "spec\n");
  assert.equal(read(path.join(lab.questionsDir, "notes.txt")), null, "only cards, drafts, and markers are kept in step");
  assert.deepEqual(sync.sync(), [], "nothing moves once both sides agree");
});

test("an edit made in Grove reaches the agent, and the agent's own edit wins a tie", () => {
  const { lab, work, sync, write, read } = fixture();
  write(path.join(lab.artifactsDir, "spec.md"), "# Spec\n");
  sync.seed();
  write(path.join(lab.artifactsDir, "spec.md"), "# Spec, edited by the user\n");
  sync.sync();
  assert.equal(read(path.join(work.artifactsDir, "spec.md")), "# Spec, edited by the user\n");

  write(path.join(lab.artifactsDir, "spec.md"), "# Lab side\n");
  write(path.join(work.artifactsDir, "spec.md"), "# Agent side\n");
  sync.sync();
  assert.equal(read(path.join(lab.artifactsDir, "spec.md")), "# Agent side\n");
  assert.equal(read(path.join(work.artifactsDir, "spec.md")), "# Agent side\n");
});

test("clearing the done marker clears it on both sides", () => {
  const { lab, work, sync, write, read } = fixture();
  sync.seed();
  write(work.doneFile, "scout\n");
  sync.sync();
  sync.clearDone();
  assert.equal(read(lab.doneFile), null);
  assert.equal(read(work.doneFile), null);
  write(work.doneFile, "scout\n");
  sync.sync();
  assert.equal(read(lab.doneFile), "scout\n", "the agent can finish again");
});

test("ending a stage keeps the agent's last writes and removes the folder", () => {
  const { lab, work, sync, write, read } = fixture();
  sync.seed();
  write(lab.coverageFile, "{}");
  write(work.scoutFile, "# Last words\n");
  sync.dispose();
  assert.equal(read(lab.scoutFile), "# Last words\n");
  assert.ok(!fs.existsSync(work.entryDir));
});

test("working folders live under Grove's state directory, one per stage of a run", () => {
  const root = labWorkRoot("e1", "run_1", "spec", { GROVE_STATE_DIR: "/state" });
  assert.equal(root, path.join("/state", "lab-work", "e1-run_1-spec"));
  assert.ok(!root.split(path.sep).includes(".git"), "never inside a repository's .git");
});
