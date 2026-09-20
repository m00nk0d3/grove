import fs from "node:fs";
import { createInterface } from "node:readline/promises";

export type ReviewRecommendation = "approve" | "request_changes" | "comment";
export type ReviewPostAction = ReviewRecommendation | "none";

export interface PullRequestVerdict {
  recommendation: ReviewRecommendation;
  summary: string;
  linkedIssues: string[];
  acceptanceCriteria: string[];
  reviewedAreas: string[];
  findings: string[];
  strengths: string[];
  validation: string[];
  residualRisks: string[];
}

function normalizeReviewList(value: unknown): string[] | null {
  if (!Array.isArray(value)) return null;

  const normalized: string[] = [];
  for (const item of value) {
    if (typeof item === "string") {
      normalized.push(item);
      continue;
    }
    if (!item || typeof item !== "object" || Array.isArray(item)) return null;

    const parts: string[] = [];
    for (const [key, fieldValue] of Object.entries(item)) {
      if (fieldValue === null || fieldValue === "") continue;
      if (!["string", "number", "boolean"].includes(typeof fieldValue)) {
        return null;
      }
      const label = key
        .replace(/_/g, " ")
        .replace(/([a-z])([A-Z])/g, "$1 $2")
        .replace(/^./, (character) => character.toUpperCase());
      parts.push(`${label}: ${String(fieldValue)}`);
    }
    if (parts.length === 0) return null;
    normalized.push(parts.join("; "));
  }
  return normalized;
}

export function readPullRequestVerdict(
  verdictPath: string,
): PullRequestVerdict {
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

  const verdict = value as Partial<PullRequestVerdict>;
  const linkedIssues = normalizeReviewList(verdict.linkedIssues);
  const acceptanceCriteria = normalizeReviewList(verdict.acceptanceCriteria);
  const reviewedAreas = normalizeReviewList(verdict.reviewedAreas);
  const findings = normalizeReviewList(verdict.findings);
  const strengths = normalizeReviewList(verdict.strengths);
  const validation = normalizeReviewList(verdict.validation);
  const residualRisks = normalizeReviewList(verdict.residualRisks);
  if (
    !["approve", "request_changes", "comment"].includes(
      verdict.recommendation ?? "",
    ) ||
    typeof verdict.summary !== "string" ||
    !linkedIssues ||
    !acceptanceCriteria ||
    !reviewedAreas ||
    !findings ||
    !strengths ||
    !validation ||
    !residualRisks
  ) {
    throw new Error("PR review verdict has an invalid schema.");
  }

  return {
    recommendation: verdict.recommendation as ReviewRecommendation,
    summary: verdict.summary,
    linkedIssues,
    acceptanceCriteria,
    reviewedAreas,
    findings,
    strengths,
    validation,
    residualRisks,
  };
}

function markdownList(items: string[]): string {
  return items.length > 0
    ? items.map((item) => `- ${item}`).join("\n")
    : "- _None_";
}

function recommendationLabel(recommendation: ReviewRecommendation): string {
  switch (recommendation) {
    case "approve":
      return "✅ APPROVE";
    case "request_changes":
      return "⛔ REQUEST CHANGES";
    case "comment":
      return "💬 COMMENT";
  }
}

export function formatPullRequestVerdict(
  verdict: PullRequestVerdict,
  prNumber: string,
): string {
  return [
    `# 🔎 Pull Request #${prNumber} Review`,
    "",
    `> **Recommendation:** ${recommendationLabel(verdict.recommendation)}`,
    "",
    "---",
    "",
    "## 📝 Summary",
    "",
    verdict.summary,
    "",
    "## 🔗 Linked Issues",
    "",
    markdownList(verdict.linkedIssues),
    "",
    "## 🎯 Acceptance Criteria",
    "",
    markdownList(verdict.acceptanceCriteria),
    "",
    "## 🔍 Reviewed Areas",
    "",
    markdownList(verdict.reviewedAreas),
    "",
    "## 🚨 Findings",
    "",
    markdownList(verdict.findings),
    "",
    "## ✨ Strengths",
    "",
    markdownList(verdict.strengths),
    "",
    "## 🧪 Validation",
    "",
    markdownList(verdict.validation),
    "",
    "## ⚠️ Residual Risks",
    "",
    markdownList(verdict.residualRisks),
    "",
  ].join("\n");
}

export function parseReviewPostAction(
  answer: string,
  canSubmitDecision = true,
): ReviewPostAction {
  switch (answer.trim().toLowerCase()) {
    case "a":
    case "approve":
      return canSubmitDecision ? "approve" : "none";
    case "r":
    case "request":
    case "request_changes":
    case "request changes":
      return canSubmitDecision ? "request_changes" : "none";
    case "c":
    case "comment":
      return "comment";
    default:
      return "none";
  }
}

export async function chooseReviewPostAction(
  recommendation: ReviewRecommendation,
  canSubmitDecision = true,
): Promise<ReviewPostAction> {
  const prompt = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    const options = canSubmitDecision
      ? "[a]pprove / [r]equest changes / [c]omment / [n]one"
      : "[c]omment / [n]one";
    const limitation = canSubmitDecision
      ? ""
      : "; GitHub does not allow approve/request-changes on your own PR";
    const answer = await prompt.question(
      `Post to GitHub? ${options} ` +
        `(reviewer recommends ${recommendation.replace("_", " ")}${limitation}): `,
    );
    return parseReviewPostAction(answer, canSubmitDecision);
  } finally {
    prompt.close();
  }
}

export function buildPullRequestReviewPrompt(
  repo: string,
  prNumber: string,
  metadataJson: string,
  verdictPath: string,
): string {
  return `
You are an independent senior pull request reviewer for ${repo}#${prNumber}.

Pull request metadata:
${metadataJson}

Objective:
- Review the pull request in depth against its linked issue requirements and produce a precise GitHub review recommendation.

Required work:
1. Use gh to read the complete PR, comments, reviews, checks, and every linked issue including comments.
2. Derive explicit and implicit acceptance criteria from the linked issue. If no issue is linked, infer intended behavior conservatively from the PR description and diff.
3. Inspect the complete diff and relevant surrounding code in this worktree.
4. Evaluate correctness, regressions, edge cases, security, concurrency, compatibility, error handling, test quality, maintainability, and documentation.
5. Run appropriate read-only validation commands when useful. Do not change files.
6. Choose exactly one recommendation:
   - "approve": no blocking defects remain.
   - "request_changes": one or more concrete blocking defects exist.
   - "comment": useful non-blocking feedback exists but approval is not warranted.
7. Write exactly one JSON object to '${verdictPath}' with this schema:
   {"recommendation":"approve|request_changes|comment","summary":"detailed conclusion","linkedIssues":["issue and purpose"],"acceptanceCriteria":["criterion and whether satisfied"],"reviewedAreas":["area and evidence"],"findings":["severity, file:line, defect, impact, and recommended change"],"strengths":["specific positive observation"],"validation":["command/check and result"],"residualRisks":["remaining risk or uncertainty"]}
   Every item in every array must be one JSON string. Do not use objects as
   array items.

Boundaries:
- Do not modify files, commit, push, post to GitHub, change PR state, or remove the worktree.
- Report only evidence-based findings; do not invent blockers to fill sections.
- Do not write Markdown fences or any text outside the JSON verdict file.

Completion criteria:
- The verdict is detailed enough to post directly as a GitHub review after formatting.
`;
}
