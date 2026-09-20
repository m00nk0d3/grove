import fs from "node:fs";
import { createInterface } from "node:readline/promises";
import { runCommand, type CommandRunner } from "./workflow-utils.js";

export const REVIEW_BATCH_SIZE = 5;

export interface ReviewVerdict {
  verdict: "approved" | "blockers";
  summary: string;
  reviewedAreas: string[];
  blockers: string[];
  fixes: string[];
  validation: string[];
  residualRisks: string[];
}

function normalizeStringList(value: unknown): string[] | null {
  if (typeof value === "string") return [value];
  if (
    Array.isArray(value) &&
    value.every((item) => typeof item === "string")
  ) {
    return value;
  }
  return null;
}

export function readReviewVerdict(verdictPath: string): ReviewVerdict {
  if (!fs.existsSync(verdictPath)) {
    throw new Error(`PR reviewer did not write its verdict: ${verdictPath}`);
  }

  let value: unknown;
  try {
    value = JSON.parse(fs.readFileSync(verdictPath, "utf8"));
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to parse PR review verdict: ${detail}`);
  }

  if (!value || typeof value !== "object") {
    throw new Error("PR review verdict must be a JSON object.");
  }

  const verdict = value as Partial<ReviewVerdict>;
  const reviewedAreas = normalizeStringList(verdict.reviewedAreas);
  const blockers = normalizeStringList(verdict.blockers);
  const fixes = normalizeStringList(verdict.fixes);
  const validation = normalizeStringList(verdict.validation);
  const residualRisks = normalizeStringList(verdict.residualRisks);
  if (
    !["approved", "blockers"].includes(verdict.verdict ?? "") ||
    typeof verdict.summary !== "string" ||
    !reviewedAreas ||
    !blockers ||
    !fixes ||
    !validation ||
    !residualRisks
  ) {
    throw new Error("PR review verdict has an invalid schema.");
  }

  return {
    verdict: verdict.verdict as ReviewVerdict["verdict"],
    summary: verdict.summary,
    reviewedAreas,
    blockers,
    fixes,
    validation,
    residualRisks,
  };
}

export function formatReviewVerdict(
  verdict: ReviewVerdict,
  cycle: number,
): string {
  const approved = verdict.verdict === "approved";
  const lines = [
    `# ${approved ? "✅" : "🚧"} Review Cycle ${cycle}: ${approved ? "APPROVED" : "BLOCKERS FOUND"}`,
    "",
    "## 📝 Summary",
    "",
    verdict.summary,
    "",
  ];
  for (const [label, items] of [
    ["🔍 Reviewed Areas", verdict.reviewedAreas],
    ["🚨 Blockers", verdict.blockers],
    ["🛠️ Required Fixes", verdict.fixes],
    ["🧪 Validation", verdict.validation],
    ["⚠️ Residual Risks", verdict.residualRisks],
  ] as const) {
    lines.push(`## ${label}`, "");
    lines.push(
      ...(items.length > 0
        ? items.map((item) => `- ${item}`)
        : ["- _None_"]),
      "",
    );
  }
  return lines.join("\n").trimEnd();
}

export function isAffirmative(answer: string): boolean {
  return ["y", "yes"].includes(answer.trim().toLowerCase());
}

export async function confirmAnotherReviewBatch(): Promise<boolean> {
  const prompt = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    const answer = await prompt.question(
      "Five review cycles still found blockers. Run five more? [y/N] ",
    );
    return isAffirmative(answer);
  } finally {
    prompt.close();
  }
}

export async function confirmStartPrReview(prUrl: string): Promise<boolean> {
  const prompt = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    const answer = await prompt.question(
      `Pull request created: ${prUrl}\nStart the automated PR review now? [y/N] `,
    );
    return isAffirmative(answer);
  } finally {
    prompt.close();
  }
}

export async function confirmPostApprovedReview(
  prUrl: string,
): Promise<boolean> {
  const prompt = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    const answer = await prompt.question(
      `\n📤 Post this verdict to GitHub as a PR review comment?\n${prUrl}\n[y/N] `,
    );
    return isAffirmative(answer);
  } finally {
    prompt.close();
  }
}

export function postReviewComment(
  repo: string,
  prUrl: string,
  reportPath: string,
  runner: CommandRunner = runCommand,
): void {
  runner("gh", [
    "pr",
    "review",
    prUrl,
    "--repo",
    repo,
    "--comment",
    "--body-file",
    reportPath,
  ]);
}
