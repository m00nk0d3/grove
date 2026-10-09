import assert from "node:assert/strict";
import test from "node:test";
import {
  citesFile,
  formatScorecard,
  redundantQuestions,
  scoreRun,
  scoutFacts,
  scriptedAnswer,
  type RunFiles,
} from "./lab-eval-score.js";
import type { LabQuestion, QuestionRecord } from "./lab-protocol.js";

const question = (q: Partial<LabQuestion>): LabQuestion => ({
  id: 1,
  kind: "choice",
  question: "Keep it?",
  context: "c",
  options: ["A", "B"],
  recommended: 1,
  why: "w",
  ...q,
});

const record = (number: number, q: Partial<LabQuestion>): QuestionRecord => ({
  number,
  file: "",
  raw: "",
  question: question({ id: number, ...q }),
});

test("the scripted user takes the recommendation unless told otherwise", () => {
  assert.deepEqual(scriptedAnswer(question({})), { choices: [1] });
  assert.deepEqual(scriptedAnswer(question({ kind: "multi", recommended: [0, 1] })), { choices: [0, 1] });
  assert.deepEqual(scriptedAnswer(question({ kind: "text", options: undefined, recommended: "Linux" })), { text: "Linux" });
  assert.deepEqual(scriptedAnswer(question({}), { choices: [0] }), { choices: [0] });
});

test("a context is grounded when it names a repository file", () => {
  const files = ["src/commands.ts", "CONTEXT.md"];
  assert.ok(citesFile("remove() in src/commands.ts filters by index", files));
  assert.ok(citesFile("commands.ts:22 filters by index", files), "a basename counts");
  assert.ok(!citesFile("The tool stores bookmarks.", files));
});

const SCOUT = `# Scout notes

## Relevant files
- src/commands.ts

## How it works today
- remove() deletes the bookmark at its stored index, while list() shows bookmarks sorted newest first (src/commands.ts:16).
- Every command reads and writes the whole store file (src/store.ts:14).

## Constraints
- x
`;

test("a question the scout notes already answer is flagged", () => {
  assert.equal(scoutFacts(SCOUT).length, 2);
  const records = [
    record(1, { question: "Does remove delete by stored index while list sorts newest first?" }),
    record(2, { question: "Should removing ask for confirmation first?" }),
  ];
  assert.deepEqual(redundantQuestions(records, SCOUT), [1]);
});

test("a run is scored from its files", () => {
  const run: RunFiles = {
    fixture: "bug",
    outcome: "completed",
    minutes: 12.34,
    repoFiles: ["src/commands.ts"],
    records: [
      record(1, { context: "src/commands.ts sorts in list()" }),
      record(2, { context: "The code cannot say which the user prefers." }),
      record(3, { context: "Unclear." }),
      { number: 4, file: "", raw: "{", error: "bad" },
    ],
    scout: SCOUT,
    coverage: JSON.stringify({ scope: "covered", triggers: "covered", data: "covered", interface: "open", errors: "covered",
      concurrency: "n/a: one user", compatibility: "covered", testing: "covered", rollout: "covered" }),
    counts: { repairs: 1, fallbacks: 0 },
    stageCounts: { interview: { repairs: 1, fallbacks: 0 }, spec: { repairs: 0, fallbacks: 0 } },
    stageMinutes: { scout: 2, interview: 6 },
    stages: ["interview", "spec"],
  };
  const score = scoreRun(run);
  assert.equal(score.questions, 3);
  assert.deepEqual(score.cards, { written: 4, valid: 3, repairs: 1, fallbacks: 0 });
  assert.deepEqual(score.grounding, { grounded: 2, citing: 1, total: 3 });
  assert.deepEqual(score.coverage, { settled: 8, total: 9 });
  assert.deepEqual(score.firstTry, { interview: false, spec: true });
  assert.equal(score.minutes, 12.3);
  const card = formatScorecard("claude sonnet", [score]);
  assert.match(card, /bug: completed in 12\.3 min/);
  assert.match(card, /grounded 67% \(cites a file 33%\)/);
  assert.match(card, /first try: interview ✗  spec ✓/);
});
