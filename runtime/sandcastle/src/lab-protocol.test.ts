import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import {
  buildAnswersPrompt,
  buildInterviewSoFar,
  describeAnswer,
  isPending,
  needsDelivery,
  nextQuestionNumber,
  scanQuestions,
  validateQuestion,
  writeReceipt,
  type LabQuestion,
  type QuestionRecord,
} from "./lab-protocol.js";

const fixtures = JSON.parse(
  fs.readFileSync(
    path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "testdata", "lab-question-cards.json"),
    "utf8",
  ),
) as { cases: { name: string; id: number; valid: boolean; card: unknown }[] };

test("question cards are validated as the shared fixtures say", () => {
  for (const fixture of fixtures.cases) {
    const result = validateQuestion(JSON.stringify(fixture.card), fixture.id);
    assert.equal(result.ok, fixture.valid, fixture.name);
  }
});

test("validation errors name the field and the rule, for the repair prompt", () => {
  const result = validateQuestion(
    JSON.stringify({ id: 1, kind: "choice", question: "q", context: "c", options: ["A", "B"], recommended: 3, why: "w" }),
    1,
  );
  assert.deepEqual(result, { ok: false, error: '"recommended" is 3 but must be an option index from 0 to 1' });
  assert.match((validateQuestion("{", 1) as { error: string }).error, /^the file is not valid JSON/);
});

const choice: LabQuestion = {
  id: 1,
  kind: "choice",
  question: "Keep it?",
  context: "c",
  options: ["Yes", "No"],
  recommended: 0,
  why: "w",
};

test("answers are described by option text and number, notes kept apart", () => {
  assert.equal(describeAnswer(choice, { choices: [1], text: "mostly" }), '"No" (option 2)');
  assert.equal(describeAnswer({ ...choice, kind: "multi" }, { choices: [0, 1] }), '"Yes" (option 1), "No" (option 2)');
  assert.equal(describeAnswer({ ...choice, kind: "text", options: undefined }, { text: "SQLite" }), '"SQLite"');
  assert.equal(describeAnswer(choice, { text: "Neither, use a flag" }), '"Neither, use a flag"', "a note alone is the answer");
});

test("a card is pending until it is answered in Grove or closed as answered in the pane", () => {
  const base: QuestionRecord = { number: 1, file: "001.json", raw: "", question: choice };
  assert.ok(isPending(base));
  assert.ok(!isPending({ ...base, answer: { id: 1, choices: [0] } }));
  assert.ok(!isPending({ ...base, receipt: { revision: 0, via: "pane", sent_at: "" } }));
  assert.ok(!isPending({ ...base, question: undefined, error: "bad" }), "an invalid card is never shown");
});

test("an answer is delivered once per revision, and never when answered in the pane", () => {
  const answered: QuestionRecord = { number: 1, file: "001.json", raw: "", question: choice, answer: { id: 1, choices: [0] } };
  assert.ok(needsDelivery(answered));
  assert.ok(!needsDelivery({ ...answered, receipt: { revision: 0, via: "grove", sent_at: "" } }));
  const revised = { ...answered, answer: { id: 1, choices: [1], revisions: [{ choices: [0] }] } };
  assert.ok(needsDelivery({ ...revised, receipt: { revision: 0, via: "grove", sent_at: "" } }));
  assert.ok(!needsDelivery({ ...revised, receipt: { revision: 1, via: "grove", sent_at: "" } }));
  assert.ok(!needsDelivery({ ...answered, receipt: { revision: 0, via: "pane", sent_at: "" } }));
});

test("question numbers continue after the highest card, invalid ones included", () => {
  assert.equal(nextQuestionNumber([]), 1);
  const records = [2, 7].map((number) => ({ number, file: "", raw: "", error: "x" }));
  assert.equal(nextQuestionNumber(records), 8);
});

test("the questions directory is read with answers and receipts", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "lab-protocol-"));
  fs.writeFileSync(path.join(dir, "001.json"), JSON.stringify({ ...choice }));
  fs.writeFileSync(path.join(dir, "001.answer.json"), JSON.stringify({ id: 1, choices: [1], text: "n" }));
  writeReceipt(dir, 1, { revision: 0, via: "grove", sent_at: "t" });
  fs.writeFileSync(path.join(dir, "002.json"), "{ broken");
  fs.writeFileSync(path.join(dir, "notes.txt"), "ignored");
  const [first, second] = scanQuestions(dir);
  assert.equal(first.question?.question, "Keep it?");
  assert.deepEqual(first.answer, { id: 1, choices: [1], text: "n" });
  assert.equal(first.receipt?.via, "grove");
  assert.equal(second.question, undefined);
  assert.match(second.error ?? "", /not valid JSON/);
  assert.deepEqual(scanQuestions(path.join(dir, "missing")), []);
});

test("a resumed session replays every question with its outcome", () => {
  const records: QuestionRecord[] = [
    { number: 1, file: "", raw: "", question: choice, answer: { id: 1, choices: [0], text: "for now" } },
    { number: 2, file: "", raw: "", question: { ...choice, question: "Split it?" }, receipt: { revision: 0, via: "pane", sent_at: "" } },
    { number: 3, file: "", raw: "", question: { ...choice, question: "Ship it?" } },
  ];
  assert.equal(
    buildInterviewSoFar(records),
    [
      'Q1. Keep it? → "Yes" (option 1) (User note: "for now")',
      "Q2. Split it? → answered in the pane; the answer was not recorded.",
      "Q3. Ship it? → still waiting for the user. Do not write another question until it is answered.",
    ].join("\n"),
  );
});

test("an answers prompt ends with the exact next step", () => {
  const records: QuestionRecord[] = [{ number: 1, file: "", raw: "", question: choice, answer: { id: 1, choices: [1] } }];
  const prompt = buildAnswersPrompt(records, path.join("q", "002.json"), "continue.");
  assert.equal(prompt, `Answer to question 1: "No" (option 2).\n\nNext: write question 2 to ${path.join("q", "002.json")} and stop, or, continue.`);
});
