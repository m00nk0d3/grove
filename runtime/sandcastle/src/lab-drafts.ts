// What each Lab stage writes, and how the runtime checks it before moving on.
// A problem is phrased for the agent: the runtime sends the list back in a
// repair prompt. See docs/LAB_DESIGN.md, "Drafts".

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

export const SCOUT_SECTIONS = [
  "# Scout notes",
  "## Relevant files",
  "## How it works today",
  "## Constraints",
  "## Open questions",
];

export const SPEC_SECTIONS = [
  "## Problem",
  "## Goals",
  "## Non-goals",
  "## User stories",
  "## Decisions",
  "## Design",
  "## Testing",
  "## Edge cases",
  "## Open questions",
];

export const ISSUE_SECTIONS = [
  "## Summary",
  "## Steps to reproduce",
  "## Expected behaviour",
  "## Actual behaviour",
  "## Environment",
  "## Component",
  "## Notes",
];

export const TICKET_SECTIONS = ["## What", "## Why", "## Where", "## Acceptance criteria", "## Out of scope"];

export const COVERAGE_TOPICS = [
  "scope",
  "triggers",
  "data",
  "interface",
  "errors",
  "concurrency",
  "compatibility",
  "testing",
  "rollout",
] as const;

const normalize = (text: string) => text.replace(/\r\n/g, "\n");

/** The headings of a Markdown document, outside fenced code blocks. */
function headings(text: string): string[] {
  const out: string[] = [];
  let fenced = false;
  for (const line of normalize(text).split("\n")) {
    if (/^\s*```/.test(line)) fenced = !fenced;
    else if (!fenced && /^#{1,6} /.test(line)) out.push(line.trim());
  }
  return out;
}

/**
 * Checks that every required heading is present, in order. Headings are
 * matched case-insensitively and may carry extra text after the required
 * words, such as "## Goals and scope"; other headings may sit between them.
 */
export function checkSections(text: string, required: string[], name: string): string[] {
  const found = headings(text).map((h) => h.toLowerCase());
  const problems: string[] = [];
  let from = 0;
  for (const heading of required) {
    const wanted = heading.toLowerCase();
    const at = found.findIndex((h, i) => i >= from && (h === wanted || h.startsWith(`${wanted} `)));
    if (at < 0) {
      const anywhere = found.some((h) => h === wanted || h.startsWith(`${wanted} `));
      problems.push(anywhere ? `${name}: "${heading}" is out of order` : `${name}: the "${heading}" section is missing`);
      continue;
    }
    from = at + 1;
  }
  return problems;
}

function titled(text: string, name: string): string[] {
  const first = normalize(text).split("\n").find((line) => line.trim() !== "") ?? "";
  return /^# \S/.test(first.trim()) ? [] : [`${name}: the first line must be "# " followed by the title`];
}

export function validateScout(text: string): string[] {
  if (text.trim() === "") return ["scout.md is empty"];
  return checkSections(text, SCOUT_SECTIONS, "scout.md");
}

export function validateSpec(text: string): string[] {
  if (text.trim() === "") return ["spec.md is empty"];
  return [...titled(text, "spec.md"), ...checkSections(text, SPEC_SECTIONS, "spec.md")];
}

export function validateIssue(text: string): string[] {
  if (text.trim() === "") return ["issue.md is empty"];
  return [...titled(text, "issue.md"), ...checkSections(text, ISSUE_SECTIONS, "issue.md")];
}

/**
 * Checks tickets.json the way Grove checks it before publishing
 * (internal/domain/lab_tickets.go), and also each body's sections.
 */
export function validateTickets(raw: string): string[] {
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    return [`tickets.json is not valid JSON (${error instanceof Error ? error.message : String(error)})`];
  }
  const list = (value as { tickets?: unknown })?.tickets;
  if (!Array.isArray(list) || list.length === 0) return ['tickets.json must be {"tickets": [ … ]} with at least one ticket'];
  const problems: string[] = [];
  const keys = new Set<string>();
  const edges = new Map<string, string[]>();
  list.forEach((ticket, index) => {
    const t = (ticket ?? {}) as Record<string, unknown>;
    const key = typeof t.key === "string" ? t.key.trim() : "";
    const label = key === "" ? `ticket ${index + 1}` : `ticket ${key}`;
    if (key === "") problems.push(`${label} has no "key"`);
    else if (keys.has(key)) problems.push(`ticket key ${key} is used twice`);
    keys.add(key);
    if (typeof t.title !== "string" || t.title.trim() === "") problems.push(`${label} has no "title"`);
    if (typeof t.body !== "string" || t.body.trim() === "") problems.push(`${label} has no "body"`);
    else problems.push(...checkSections(t.body, TICKET_SECTIONS, `${label} body`));
    const blockers = Array.isArray(t.blocked_by) ? t.blocked_by.filter((b): b is string => typeof b === "string") : [];
    if (t.blocked_by !== undefined && !Array.isArray(t.blocked_by)) problems.push(`${label}: "blocked_by" must be a list of keys`);
    if (blockers.includes(key)) problems.push(`ticket ${key} blocks itself`);
    edges.set(key, [...(edges.get(key) ?? []), ...blockers]);
  });
  for (const [key, blockers] of edges) {
    for (const blocker of blockers) {
      if (blocker !== key && !keys.has(blocker)) problems.push(`ticket ${key} is blocked by ${blocker}, which is not a ticket`);
    }
  }
  // Kahn's algorithm: whatever cannot be ordered sits on a cycle.
  const remaining = new Map([...edges].map(([key, blockers]) => [key, blockers.filter((b) => b !== key && keys.has(b))]));
  let progressed = true;
  while (progressed) {
    progressed = false;
    for (const [key, blockers] of remaining) {
      if (blockers.every((b) => !remaining.has(b))) {
        remaining.delete(key);
        progressed = true;
      }
    }
  }
  if (remaining.size > 0) problems.push(`tickets ${[...remaining.keys()].sort().join(", ")} block each other in a cycle`);
  return problems;
}

export interface CoverageCheck {
  problems: string[];
  /** Topics still "open". */
  open: string[];
}

export function checkCoverage(raw: string): CoverageCheck {
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return { problems: ["coverage.json is missing or is not valid JSON"], open: [...COVERAGE_TOPICS] };
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return { problems: ["coverage.json must be one JSON object"], open: [...COVERAGE_TOPICS] };
  }
  const record = value as Record<string, unknown>;
  const problems: string[] = [];
  const open: string[] = [];
  for (const topic of COVERAGE_TOPICS) {
    const status = record[topic];
    if (status === "covered") continue;
    if (status === "open") {
      open.push(topic);
    } else if (typeof status === "string" && /^n\/a:\s*\S/.test(status)) {
      continue;
    } else {
      problems.push(`coverage.json: "${topic}" must be "covered", "open", or "n/a: <reason>"`);
      open.push(topic);
    }
  }
  return { problems, open };
}

export function contentHash(text: string): string {
  return crypto.createHash("sha256").update(normalize(text)).digest("hex");
}

/**
 * Whether the user approved an artifact as it is now, read from the review
 * Grove records on the entry: approved, on a hash of this exact content.
 */
export function isApproved(labDir: string, entryId: string, artifact: string, content: string | null): boolean {
  if (content === null) return false;
  let index: { entries?: { id: string; reviews?: Record<string, { state?: string; hash?: string }> }[] };
  try {
    index = JSON.parse(fs.readFileSync(path.join(labDir, "entries.json"), "utf8"));
  } catch {
    return false;
  }
  const review = index.entries?.find((e) => e.id === entryId)?.reviews?.[artifact];
  return review?.state === "approved" && review.hash === contentHash(content);
}

export function readIfExists(file: string): string | null {
  try {
    return normalize(fs.readFileSync(file, "utf8"));
  } catch {
    return null;
  }
}

/**
 * The next decision record number: one past the highest in the repository's
 * docs/adr and the drafts' docs/adr, four digits.
 */
export function nextAdrNumber(repo: string, artifactsDir: string): string {
  let highest = 0;
  for (const dir of [path.join(repo, "docs", "adr"), path.join(artifactsDir, "docs", "adr")]) {
    let names: string[] = [];
    try {
      names = fs.readdirSync(dir);
    } catch {
      continue;
    }
    for (const name of names) {
      const match = /^(\d{4})-/.exec(name);
      if (match) highest = Math.max(highest, Number(match[1]));
    }
  }
  return String(highest + 1).padStart(4, "0");
}
