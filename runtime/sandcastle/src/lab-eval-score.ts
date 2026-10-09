// Scoring a Lab evaluation run from the files the session left behind. See
// docs/LAB_DESIGN.md, "Evaluation", and src/lab-eval.ts, which drives the
// runs.

import path from "node:path";
import { checkCoverage, COVERAGE_TOPICS } from "./lab-drafts.js";
import type { LabQuestion, QuestionRecord } from "./lab-protocol.js";
import type { LabStage } from "./lab-prompts.js";
import type { SessionCounts } from "./lab-session.js";

/** How the scripted user answers a card: an override, or the recommendation. */
export interface ScriptedAnswer {
  choices?: number[];
  text?: string;
}

export function scriptedAnswer(question: LabQuestion, override?: ScriptedAnswer): ScriptedAnswer {
  if (override) return override;
  if (question.kind === "text") return { text: String(question.recommended) };
  const recommended = question.recommended;
  return { choices: Array.isArray(recommended) ? recommended : [recommended as number] };
}

export interface EvalScore {
  fixture: string;
  outcome: "completed" | "timed-out" | "failed";
  minutes: number;
  questions: number;
  cards: { written: number; valid: number; repairs: number; fallbacks: number };
  /** Cards whose context cites a repository file or says why the code cannot answer. */
  grounding: { grounded: number; citing: number; total: number };
  /** Questions the scout notes already answer. */
  redundant: number[];
  coverage: { settled: number; total: number } | null;
  /** Whether each stage's output passed validation without a repair. */
  firstTry: Partial<Record<LabStage, boolean>>;
  stageCounts: Partial<Record<LabStage, SessionCounts>>;
  stageMinutes: Partial<Record<string, number>>;
}

const STOP_WORDS = new Set(
  "the a an and or of to in on for with is are be it this that what which who when where how should does do we our your their from by as at into than then there these those would could will can".split(" "),
);

/** The words of a sentence that carry meaning. */
export function contentWords(text: string): Set<string> {
  return new Set(
    text
      .toLowerCase()
      .split(/[^a-z0-9_.-]+/)
      .filter((word) => word.length > 3 && !STOP_WORDS.has(word)),
  );
}

const EXPLAINS_GAP = /cannot|can't|can not|does not (say|show|decide|answer)|doesn't (say|show|decide|answer)|no code|not in the code|user's (choice|decision)|a decision/i;

/** Whether a card's context names one of the repository's files. */
export function citesFile(context: string, files: string[]): boolean {
  const lower = context.toLowerCase();
  return files.some((file) => {
    const f = file.toLowerCase();
    return lower.includes(f) || lower.includes(path.posix.basename(f));
  });
}

/** The lines under the scout notes' "How it works today" heading. */
export function scoutFacts(scout: string): string[] {
  const lines = scout.replace(/\r\n/g, "\n").split("\n");
  const start = lines.findIndex((line) => /^## How it works today/i.test(line.trim()));
  if (start < 0) return [];
  const facts: string[] = [];
  for (const line of lines.slice(start + 1)) {
    if (/^#{1,6} /.test(line.trim())) break;
    if (line.trim() !== "") facts.push(line.trim());
  }
  return facts;
}

/**
 * Questions whose words mostly appear in a single fact of the scout notes:
 * the agent asked what it had already found out.
 */
export function redundantQuestions(records: QuestionRecord[], scout: string, threshold = 0.6): number[] {
  const facts = scoutFacts(scout).map(contentWords);
  return records
    .filter((record) => record.question)
    .filter((record) => {
      const words = contentWords(record.question!.question);
      if (words.size < 3) return false;
      return facts.some((fact) => [...words].filter((w) => fact.has(w)).length / words.size >= threshold);
    })
    .map((record) => record.number);
}

export interface RunFiles {
  fixture: string;
  outcome: EvalScore["outcome"];
  minutes: number;
  repoFiles: string[];
  records: QuestionRecord[];
  scout: string | null;
  coverage: string | null;
  counts: SessionCounts;
  stageCounts: Partial<Record<LabStage, SessionCounts>>;
  stageMinutes: Partial<Record<string, number>>;
  /** The stages that produced output, in order. */
  stages: LabStage[];
}

export function scoreRun(run: RunFiles): EvalScore {
  const valid = run.records.filter((record) => record.question);
  const citing = valid.filter((record) => citesFile(record.question!.context, run.repoFiles));
  const grounded = valid.filter(
    (record) => citesFile(record.question!.context, run.repoFiles) || EXPLAINS_GAP.test(record.question!.context),
  );
  let coverage: EvalScore["coverage"] = null;
  if (run.coverage !== null) {
    const check = checkCoverage(run.coverage);
    coverage = { settled: COVERAGE_TOPICS.length - check.open.length, total: COVERAGE_TOPICS.length };
  }
  const firstTry: EvalScore["firstTry"] = {};
  for (const stage of run.stages) {
    // A stage's repairs count card repairs too; a stage that asks is judged
    // on its output only when it asked nothing that needed repair.
    firstTry[stage] = (run.stageCounts[stage]?.repairs ?? 0) === 0;
  }
  return {
    fixture: run.fixture,
    outcome: run.outcome,
    minutes: Math.round(run.minutes * 10) / 10,
    questions: valid.length,
    cards: { written: run.records.length, valid: valid.length, repairs: run.counts.repairs, fallbacks: run.counts.fallbacks },
    grounding: { grounded: grounded.length, citing: citing.length, total: valid.length },
    redundant: run.scout ? redundantQuestions(run.records, run.scout) : [],
    coverage,
    firstTry,
    stageCounts: run.stageCounts,
    stageMinutes: run.stageMinutes,
  };
}

const pct = (n: number, d: number) => (d === 0 ? "—" : `${Math.round((100 * n) / d)}%`);

/** A plain-text scorecard, one block per fixture. */
export function formatScorecard(label: string, scores: EvalScore[]): string {
  const lines = [`Lab evaluation — ${label}`, ""];
  for (const s of scores) {
    lines.push(`${s.fixture}: ${s.outcome} in ${s.minutes} min`);
    lines.push(`  questions ${s.questions}   cards ${s.cards.valid}/${s.cards.written} valid   repairs ${s.cards.repairs}   fallbacks ${s.cards.fallbacks}`);
    lines.push(
      `  grounded ${pct(s.grounding.grounded, s.grounding.total)} (cites a file ${pct(s.grounding.citing, s.grounding.total)})   redundant ${s.redundant.length === 0 ? "none" : s.redundant.map((n) => `Q${n}`).join(", ")}`,
    );
    if (s.coverage) lines.push(`  coverage ${s.coverage.settled}/${s.coverage.total}`);
    const tries = Object.entries(s.firstTry).map(([stage, ok]) => `${stage} ${ok ? "✓" : "✗"}`);
    if (tries.length > 0) lines.push(`  first try: ${tries.join("  ")}`);
    const times = Object.entries(s.stageMinutes).map(([stage, minutes]) => `${stage} ${minutes}m`);
    if (times.length > 0) lines.push(`  stage minutes: ${times.join("  ")}`);
    lines.push("");
  }
  return lines.join("\n");
}
