// The Lab question protocol: the files through which a session's agent asks
// the user questions and Grove answers them. The agent writes question cards,
// Grove writes answers and requests, and the session runtime delivers them and
// writes a receipt for each delivery. No file has two writers. See
// docs/LAB_DESIGN.md, "Question protocol".

import fs from "node:fs";
import path from "node:path";

export type QuestionKind = "choice" | "multi" | "text";

const QUESTION_KINDS: readonly QuestionKind[] = ["choice", "multi", "text"];

/** A question card, written by the agent to questions/NNN.json. */
export interface LabQuestion {
  id: number;
  kind: QuestionKind;
  question: string;
  context: string;
  options?: string[];
  /** An option index for choice, indices for multi, a suggestion for text. */
  recommended: number | number[] | string;
  why: string;
}

/** One earlier answer to a question, kept when the user revises it. */
export interface LabAnswerRevision {
  choices?: number[];
  text?: string;
  answered_at?: string;
}

/** An answer, written by Grove to questions/NNN.answer.json. */
export interface LabAnswer {
  id: number;
  choices?: number[];
  text?: string;
  answered_at?: string;
  revisions?: LabAnswerRevision[];
}

/** A delivery receipt, written by the runtime to NNN.sent. */
export interface Receipt {
  /** For an answer, how many revisions it had when delivered. */
  revision: number;
  /** "skipped" closes a card left open when the user ended the interview. */
  via: "grove" | "pane" | "skipped";
  sent_at: string;
}

export type RequestKind = "reply" | "change" | "finish_interview" | "permission";

const REQUEST_KINDS: readonly RequestKind[] = ["reply", "change", "finish_interview", "permission"];

/** A message from Grove to the agent, written to requests/NNN.json. */
export interface LabRequest {
  id: number;
  kind: RequestKind;
  text?: string;
  /** For a permission request: whether the user allowed it. */
  allow?: boolean;
  created_at?: string;
}

/** A question file and everything written about it. */
export interface QuestionRecord {
  number: number;
  file: string;
  /** The parsed card, when it is valid. */
  question?: LabQuestion;
  /** Why the card is invalid, when it is not. */
  error?: string;
  /** The card's raw content, so a repaired card can be told from the original. */
  raw: string;
  answer?: LabAnswer;
  receipt?: Receipt;
}

export interface RequestRecord {
  number: number;
  request?: LabRequest;
  error?: string;
  receipt?: Receipt;
}

const CARD_FILE = /^(\d{3,})\.json$/;

export function cardFileName(number: number): string {
  return `${String(number).padStart(3, "0")}.json`;
}

export function answerFileName(number: number): string {
  return `${String(number).padStart(3, "0")}.answer.json`;
}

export function receiptFileName(number: number): string {
  return `${String(number).padStart(3, "0")}.sent`;
}

const isObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const nonEmptyString = (value: unknown): value is string =>
  typeof value === "string" && value.trim() !== "";

/**
 * Validates a question card. The error names the field and the rule, because
 * it is sent to the agent verbatim in a repair prompt.
 */
export function validateQuestion(
  raw: string,
  expectedId: number,
): { ok: true; question: LabQuestion } | { ok: false; error: string } {
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    return { ok: false, error: `the file is not valid JSON (${error instanceof Error ? error.message : String(error)})` };
  }
  if (!isObject(value)) return { ok: false, error: "the file must hold one JSON object" };
  if (value.id !== expectedId) {
    return { ok: false, error: `"id" must be ${expectedId}, the number in the file name` };
  }
  if (typeof value.kind !== "string" || !(QUESTION_KINDS as readonly string[]).includes(value.kind)) {
    return { ok: false, error: `"kind" must be "choice", "multi", or "text"` };
  }
  const kind = value.kind as QuestionKind;
  for (const field of ["question", "context", "why"] as const) {
    if (!nonEmptyString(value[field])) return { ok: false, error: `"${field}" must be a non-empty string` };
  }
  const question: LabQuestion = {
    id: expectedId,
    kind,
    question: (value.question as string).trim(),
    context: (value.context as string).trim(),
    why: (value.why as string).trim(),
    recommended: "",
  };
  if (kind === "text") {
    if (value.options !== undefined) return { ok: false, error: `a "text" question has no "options"` };
    if (!nonEmptyString(value.recommended)) {
      return { ok: false, error: `"recommended" must be a suggested answer string for a "text" question` };
    }
    question.recommended = value.recommended.trim();
    return { ok: true, question };
  }
  const options = value.options;
  if (!Array.isArray(options) || options.length < 2 || options.length > 4) {
    return { ok: false, error: `"options" must be a list of 2 to 4 strings for a "${kind}" question` };
  }
  if (!options.every(nonEmptyString)) return { ok: false, error: `every entry in "options" must be a non-empty string` };
  question.options = options.map((option) => option.trim());
  const inRange = (index: unknown): index is number =>
    Number.isInteger(index) && (index as number) >= 0 && (index as number) < options.length;
  if (kind === "choice") {
    if (!inRange(value.recommended)) {
      return {
        ok: false,
        error: `"recommended" is ${JSON.stringify(value.recommended)} but must be an option index from 0 to ${options.length - 1}`,
      };
    }
    question.recommended = value.recommended;
    return { ok: true, question };
  }
  const recommended = value.recommended;
  if (!Array.isArray(recommended) || recommended.length === 0 || !recommended.every(inRange)) {
    return {
      ok: false,
      error: `"recommended" must be a list of option indices from 0 to ${options.length - 1} for a "multi" question`,
    };
  }
  if (new Set(recommended).size !== recommended.length) {
    return { ok: false, error: `"recommended" lists the same option twice` };
  }
  question.recommended = recommended as number[];
  return { ok: true, question };
}

function readJson(file: string): unknown {
  try {
    return JSON.parse(fs.readFileSync(file, "utf8"));
  } catch {
    return undefined;
  }
}

function readReceipt(file: string): Receipt | undefined {
  const value = readJson(file);
  if (!isObject(value) || typeof value.revision !== "number") return undefined;
  return {
    revision: value.revision,
    via: value.via === "pane" || value.via === "skipped" ? value.via : "grove",
    sent_at: typeof value.sent_at === "string" ? value.sent_at : "",
  };
}

function readAnswer(file: string, id: number): LabAnswer | undefined {
  const value = readJson(file);
  if (!isObject(value)) return undefined;
  const answer: LabAnswer = { id };
  if (Array.isArray(value.choices)) answer.choices = value.choices.filter((c): c is number => Number.isInteger(c));
  if (typeof value.text === "string") answer.text = value.text;
  if (typeof value.answered_at === "string") answer.answered_at = value.answered_at;
  if (Array.isArray(value.revisions)) answer.revisions = value.revisions.filter(isObject) as LabAnswerRevision[];
  return answer;
}

function numberedFiles(dir: string): number[] {
  let names: string[];
  try {
    names = fs.readdirSync(dir);
  } catch {
    return [];
  }
  return names
    .map((name) => CARD_FILE.exec(name))
    .filter((match): match is RegExpExecArray => match !== null)
    .map((match) => Number(match[1]))
    .sort((a, b) => a - b);
}

/** Reads every question card in dir, with its answer and receipt. */
export function scanQuestions(dir: string): QuestionRecord[] {
  return numberedFiles(dir).map((number) => {
    const file = path.join(dir, cardFileName(number));
    let raw = "";
    try {
      raw = fs.readFileSync(file, "utf8");
    } catch {
      // Removed between listing and reading; validated as empty.
    }
    const result = validateQuestion(raw, number);
    return {
      number,
      file,
      raw,
      ...(result.ok ? { question: result.question } : { error: result.error }),
      answer: readAnswer(path.join(dir, answerFileName(number)), number),
      receipt: readReceipt(path.join(dir, receiptFileName(number))),
    };
  });
}

/** Reads every request in dir, with its receipt. */
export function scanRequests(dir: string): RequestRecord[] {
  return numberedFiles(dir).map((number) => {
    const value = readJson(path.join(dir, cardFileName(number)));
    const receipt = readReceipt(path.join(dir, receiptFileName(number)));
    if (!isObject(value) || typeof value.kind !== "string" || !(REQUEST_KINDS as readonly string[]).includes(value.kind)) {
      return { number, error: "unreadable request", receipt };
    }
    const request: LabRequest = { id: number, kind: value.kind as RequestKind };
    if (typeof value.text === "string") request.text = value.text;
    if (typeof value.allow === "boolean") request.allow = value.allow;
    if (typeof value.created_at === "string") request.created_at = value.created_at;
    return { number, request, receipt };
  });
}

export function writeReceipt(dir: string, number: number, receipt: Receipt): void {
  fs.mkdirSync(dir, { recursive: true });
  const file = path.join(dir, receiptFileName(number));
  const tmp = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, `${JSON.stringify(receipt)}\n`, "utf8");
  fs.renameSync(tmp, file);
}

/** A valid card no one has answered, in Grove or in the pane. */
export function isPending(record: QuestionRecord): boolean {
  return record.question !== undefined && record.answer === undefined && record.receipt === undefined;
}

/** An answer whose latest revision has not been delivered. */
export function needsDelivery(record: QuestionRecord): boolean {
  if (!record.answer || !record.question) return false;
  if (!record.receipt) return true;
  if (record.receipt.via !== "grove") return false;
  return record.receipt.revision < (record.answer.revisions?.length ?? 0);
}

export function nextQuestionNumber(records: QuestionRecord[]): number {
  return records.reduce((highest, record) => Math.max(highest, record.number), 0) + 1;
}

/** The answer as the agent reads it, such as `"Keep it" (option 1)`. */
export function describeChoices(question: LabQuestion, choices: number[] | undefined): string {
  const options = question.options ?? [];
  const picked = (choices ?? []).filter((index) => index >= 0 && index < options.length);
  return picked.map((index) => `"${options[index]}" (option ${index + 1})`).join(", ");
}

export function describeAnswer(question: LabQuestion, answer: { choices?: number[]; text?: string }): string {
  const chosen = describeChoices(question, answer.choices);
  const note = answer.text?.trim() ?? "";
  if (question.kind === "text" || chosen === "") return note === "" ? "(no answer given)" : `"${note}"`;
  return chosen;
}

function userNote(question: LabQuestion, answer: { text?: string }): string | null {
  if (question.kind === "text") return null;
  const note = answer.text?.trim() ?? "";
  return note === "" ? null : `User note: "${note}"`;
}

/** The line every prompt ends with: exactly what the agent does next. */
export function nextStepLine(nextCardPath: string, finish: string): string {
  return `Next: write question ${path.basename(nextCardPath, ".json").replace(/^0+/, "")} to ${nextCardPath} and stop, or, ${finish}`;
}

/**
 * The prompt that delivers answers, revised answers included, followed by the
 * next step.
 */
export function buildAnswersPrompt(records: QuestionRecord[], nextCardPath: string, finish: string): string {
  const lines: string[] = [];
  for (const record of records) {
    const { question, answer } = record;
    if (!question || !answer) continue;
    const revisions = answer.revisions ?? [];
    const delivered = record.receipt?.via === "grove";
    if (delivered && revisions.length > 0) {
      const previous = revisions[revisions.length - 1];
      lines.push(
        `Revision to question ${record.number}: was ${describeAnswer(question, previous)}, now ${describeAnswer(question, answer)}.`,
      );
      const note = userNote(question, answer);
      if (note) lines.push(note);
      lines.push("If any later question or answer depended on it, ask about it again.");
    } else {
      lines.push(`Answer to question ${record.number}: ${describeAnswer(question, answer)}.`);
      const note = userNote(question, answer);
      if (note) lines.push(note);
    }
    lines.push("");
  }
  lines.push(nextStepLine(nextCardPath, finish));
  return lines.join("\n");
}

/** Asks the agent to fix a stage output that failed validation. */
export function buildDraftRepairPrompt(problems: string[], stage: string, doneFile: string): string {
  return [
    "What you wrote has problems:",
    ...problems.map((problem) => `- ${problem}`),
    "",
    `Fix every one of them in place, then write ${stage} to ${doneFile} and stop.`,
  ].join("\n");
}

export function buildRepairPrompt(record: QuestionRecord): string {
  return [
    `${path.basename(record.file)} is invalid: ${record.error}.`,
    `Rewrite ${record.file} so it follows the question card format, and stop.`,
  ].join("\n");
}

/** Where a request is delivered: the stage and the files it finishes with. */
export interface RequestTarget {
  stage: string;
  doneFile: string;
  draftsDir: string;
  coverageFile: string;
  /** Whether the stage asks the user through question cards. */
  asks: boolean;
}

/** Delivers a request other than a permission, which is answered by keys. */
export function buildRequestPrompt(request: LabRequest, nextCardPath: string, finish: string, target: RequestTarget): string {
  const text = request.text?.trim() ?? "";
  switch (request.kind) {
    case "reply":
      return [`The user replies: "${text}"`, "", nextStepLine(nextCardPath, finish)].join("\n");
    case "change":
      return [
        `The user asks for changes to the drafts in ${target.draftsDir}:`,
        "<<<",
        text,
        ">>>",
        target.asks
          ? "Revise the drafts in place. If a change needs a decision from the user, ask it with one question card first."
          : "Revise the drafts in place with the smallest change that does what the user asks. Record anything you cannot decide as an open question in the draft.",
        `When the drafts are revised, write ${target.stage} to ${target.doneFile} and stop.`,
      ].join("\n");
    case "finish_interview":
      return [
        "The user has ended the interview. Ask no more questions.",
        `Set every topic that is not settled to "open" in ${target.coverageFile}, then write ${target.stage} to ${target.doneFile} and stop.`,
      ].join("\n");
    case "permission":
      return "";
  }
}

/** One line per question so far, for an agent resuming a session. */
export function buildInterviewSoFar(records: QuestionRecord[]): string {
  const lines: string[] = [];
  for (const record of records) {
    const { question, answer, receipt } = record;
    if (!question) continue;
    if (answer) {
      const note = userNote(question, answer);
      lines.push(`Q${record.number}. ${question.question} → ${describeAnswer(question, answer)}${note ? ` (${note})` : ""}`);
    } else if (receipt?.via === "pane") {
      lines.push(`Q${record.number}. ${question.question} → answered in the pane; the answer was not recorded.`);
    } else if (receipt?.via === "skipped") {
      lines.push(`Q${record.number}. ${question.question} → not answered: the user ended the interview.`);
    } else {
      lines.push(`Q${record.number}. ${question.question} → still waiting for the user. Do not write another question until it is answered.`);
    }
  }
  return lines.join("\n");
}
