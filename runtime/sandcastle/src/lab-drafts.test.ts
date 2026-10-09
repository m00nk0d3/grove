import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  checkCoverage,
  checkSections,
  contentHash,
  isApproved,
  nextAdrNumber,
  validateIssue,
  validateTickets,
} from "./lab-drafts.js";

const body = "## What\nw\n\n## Why\ny\n\n## Where\n- a.go\n\n## Acceptance criteria\n- [ ] a\n\n## Out of scope\n- b\n";
const tickets = (list: unknown[]) => JSON.stringify({ tickets: list });

test("sections must all be present, in order, outside code blocks", () => {
  assert.deepEqual(checkSections("# T\n## A\n## B extra words\n", ["## A", "## B"], "x"), []);
  assert.deepEqual(checkSections("## B\n## A\n", ["## A", "## B"], "x"), ['x: "## B" is out of order']);
  assert.deepEqual(checkSections("```\n## A\n```\n", ["## A"], "x"), ['x: the "## A" section is missing']);
});

test("a bug report needs a title and every section", () => {
  const full = "# Title\n\n## Summary\n## Steps to reproduce\n## Expected behaviour\n## Actual behaviour\n## Environment\n## Component\n## Notes\n";
  assert.deepEqual(validateIssue(full), []);
  assert.match(validateIssue(full.replace("# Title", "Title")).join(), /first line must be "# "/);
  assert.match(validateIssue(full.replace("## Component\n", "")).join(), /"## Component" section is missing/);
});

test("tickets are checked as Grove checks them before publishing", () => {
  const ok = tickets([
    { key: "01", title: "Prefactor", body, blocked_by: [] },
    { key: "02", title: "Slice", body, blocked_by: ["01"] },
  ]);
  assert.deepEqual(validateTickets(ok), []);
  assert.match(validateTickets("{").join(), /not valid JSON/);
  assert.match(validateTickets(tickets([])).join(), /at least one ticket/);
  const problems = validateTickets(
    tickets([
      { key: "01", title: "", body, blocked_by: ["01"] },
      { key: "01", title: "Dup", body: "## What\nx", blocked_by: ["09"] },
      { key: "03", title: "A", body, blocked_by: ["04"] },
      { key: "04", title: "B", body, blocked_by: ["03"] },
    ]),
  ).join("\n");
  assert.match(problems, /ticket 01 has no "title"/);
  assert.match(problems, /ticket key 01 is used twice/);
  assert.match(problems, /ticket 01 blocks itself/);
  assert.match(problems, /blocked by 09, which is not a ticket/);
  assert.match(problems, /"## Acceptance criteria" section is missing/);
  assert.match(problems, /tickets 03, 04 block each other in a cycle/);
});

test("coverage lists the topics still open and malformed values", () => {
  const all = { scope: "covered", triggers: "covered", data: "covered", interface: "covered", errors: "covered",
    concurrency: "n/a: one writer", compatibility: "covered", testing: "open", rollout: "maybe" };
  const check = checkCoverage(JSON.stringify(all));
  assert.deepEqual(check.open, ["testing", "rollout"]);
  assert.deepEqual(check.problems, ['coverage.json: "rollout" must be "covered", "open", or "n/a: <reason>"']);
  assert.match(checkCoverage("").problems.join(), /missing or is not valid JSON/);
  assert.match(checkCoverage(JSON.stringify({ ...all, rollout: "n/a:" })).problems.join(), /"rollout"/, "n/a needs a reason");
});

test("an approval counts only for the content it was given on", () => {
  const lab = fs.mkdtempSync(path.join(os.tmpdir(), "lab-drafts-"));
  const text = "# Spec\r\nbody\r\n";
  fs.writeFileSync(path.join(lab, "entries.json"), JSON.stringify({
    entries: [{ id: "e1", reviews: { "spec.md": { state: "approved", hash: contentHash("# Spec\nbody\n") } } }],
  }));
  assert.ok(isApproved(lab, "e1", "spec.md", text), "line endings do not matter, as in Grove");
  assert.ok(!isApproved(lab, "e1", "spec.md", text + "more"));
  assert.ok(!isApproved(lab, "e1", "tickets.json", "{}"));
  assert.ok(!isApproved(lab, "e2", "spec.md", text));
  assert.ok(!isApproved(lab, "e1", "spec.md", null));
});

test("the next decision record number follows the repository and the drafts", () => {
  const repo = fs.mkdtempSync(path.join(os.tmpdir(), "lab-adr-"));
  const drafts = fs.mkdtempSync(path.join(os.tmpdir(), "lab-adr-drafts-"));
  assert.equal(nextAdrNumber(repo, drafts), "0001");
  fs.mkdirSync(path.join(repo, "docs", "adr"), { recursive: true });
  fs.writeFileSync(path.join(repo, "docs", "adr", "0003-x.md"), "");
  fs.mkdirSync(path.join(drafts, "docs", "adr"), { recursive: true });
  fs.writeFileSync(path.join(drafts, "docs", "adr", "0004-y.md"), "");
  assert.equal(nextAdrNumber(repo, drafts), "0005");
});
