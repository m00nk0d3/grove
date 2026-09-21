import fs from "node:fs";
import { createInterface } from "node:readline/promises";
import { runCommand, type CommandRunner } from "./workflow-utils.js";

export const REVIEW_BATCH_SIZE = 5;
const MAX_JSON_READ_RETRIES = 3;

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

// The conditional review specialists report a verdict of their own shape:
// what they reviewed and what blocks, plus fields particular to their domain.
// They do not carry the pull request reviewer's `fixes` and `validation`, so
// validating them against that schema rejected sound audits over fields they
// were never asked for.
export interface AuditVerdict {
  verdict: "approved" | "blockers";
  summary: string;
  reviewedAreas: string[];
  blockers: string[];
}

export function readAuditVerdict(
  verdictPath: string,
  label = "audit",
): AuditVerdict {
  if (!fs.existsSync(verdictPath)) {
    throw new Error(`The ${label} specialist wrote no verdict: ${verdictPath}`);
  }

  let jsonContent: string;
  try {
    jsonContent = fs.readFileSync(verdictPath, "utf8").trim();
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to read the ${label} verdict: ${detail}`);
  }

  // Agents sometimes wrap the object in a Markdown fence despite being asked
  // not to; that is a formatting slip, not a failed audit.
  if (jsonContent.startsWith("```")) {
    // Drop the opening fence together with any language tag on that line,
    // then the closing fence. Removing only the backticks leaves "json"
    // in front of the object.
    jsonContent = jsonContent.replace(/^```[^\n]*\n?/, "");
    const closing = jsonContent.lastIndexOf("```");
    if (closing >= 0) jsonContent = jsonContent.slice(0, closing);
  }
  jsonContent = jsonContent.replace(/<!--[\s\S]*?-->/g, "").trim();

  let value: unknown;
  try {
    value = JSON.parse(jsonContent);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to parse the ${label} verdict: ${detail}`);
  }

  const parsed = value as Partial<AuditVerdict>;
  const reviewedAreas = normalizeStringList(parsed.reviewedAreas);
  const blockers = normalizeStringList(parsed.blockers);
  if (
    !["approved", "blockers"].includes(parsed.verdict ?? "") ||
    typeof parsed.summary !== "string" ||
    !parsed.summary.trim() ||
    !reviewedAreas ||
    !blockers
  ) {
    throw new Error(
      `The ${label} verdict has an invalid schema; it needs verdict, summary, reviewedAreas and blockers.`,
    );
  }

  // "approved" alongside listed blockers is contradictory, and believing the
  // optimistic half would let real findings through.
  const verdict =
    parsed.verdict === "approved" && blockers.length > 0
      ? "blockers"
      : (parsed.verdict as AuditVerdict["verdict"]);
  return { verdict, summary: parsed.summary, reviewedAreas, blockers };
}

export function readReviewVerdict(verdictPath: string): ReviewVerdict {
  if (!fs.existsSync(verdictPath)) {
    throw new Error(`PR reviewer did not write its verdict: ${verdictPath}`);
  }

  let content: string;
  try {
    content = fs.readFileSync(verdictPath, "utf8");
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to read PR review verdict: ${detail}`);
  }

  // Extract JSON content, stripping markdown fences and comments
  let jsonContent = content.trim();
  
  // Remove markdown code blocks (```json or ```)
  if (/^(```\s*json?\s*)/.test(jsonContent)) {
    const startIdx = jsonContent.indexOf("```") + 3;
    jsonContent = jsonContent.slice(startIdx);
  }
  if (/\n\s*\n\s*```/.test(jsonContent)) {
    const endIdx = jsonContent.lastIndexOf("```");
    jsonContent = jsonContent.slice(0, endIdx).trim();
  }
  
  // Remove HTML comments that might wrap the JSON
  jsonContent = jsonContent.replace(/<!--[\s\S]*?-->/g, "");

  let value: unknown;
  try {
    value = JSON.parse(jsonContent);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    
    // Check for incomplete JSON (unclosed braces/brackets at end of file)
    const trimmed = jsonContent.trimEnd();
    if ((trimmed.endsWith("}") && !jsonContent.endsWith("{")) || 
        (trimmed.endsWith("]") && !jsonContent.startsWith("{") && !jsonContent.startsWith("["))) {
      throw new Error(`PR review verdict appears incomplete or truncated. The agent may not have finished writing the JSON file. Verdict content:\n${content.slice(0, 500)}`);
    }
    
    throw new Error(`Unable to parse PR review verdict: ${detail}`);
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

export async function readJsonArtifactWithRetry(
  path: string,
  maxRetries: number = MAX_JSON_READ_RETRIES,
): Promise<ReviewVerdict> {
  if (!fs.existsSync(path)) {
    throw new Error(`JSON artifact not found: ${path}`);
  }

  let lastError: Error | null = null;
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      const content = fs.readFileSync(path, "utf8");
      
      // Strip markdown fences and comments
      let jsonContent = content.trim();
      if (/^(```\s*json?\s*)/.test(jsonContent)) {
        const startIdx = jsonContent.indexOf("```") + 3;
        jsonContent = jsonContent.slice(startIdx);
      }
      if (/\n\s*\n\s*```/.test(jsonContent)) {
        const endIdx = jsonContent.lastIndexOf("```");
        jsonContent = jsonContent.slice(0, endIdx).trim();
      }
      jsonContent = jsonContent.replace(/<!--[\s\S]*?-->/g, "");

      let value: unknown;
      try {
        value = JSON.parse(jsonContent);
      } catch (error) {
        const detail = error instanceof Error ? error.message : String(error);
        
        // Check for incomplete JSON (unclosed braces/brackets at end of file)
        const trimmed = jsonContent.trimEnd();
        if ((trimmed.endsWith("}") && !jsonContent.startsWith("{") && !jsonContent.startsWith("[")) || 
            (trimmed.endsWith("]") && !jsonContent.startsWith("{") && !jsonContent.startsWith("["))) {
          throw new Error(`JSON appears incomplete or truncated. The agent may not have finished writing the JSON file.`);
        }
        
        throw new Error(`Invalid JSON: ${detail}`);
      }

      // Normalize string fields to arrays (agents sometimes return strings instead of arrays)
      const raw = value as Record<string, unknown>;
      const normalized: ReviewVerdict = {
        verdict: raw.verdict as ReviewVerdict["verdict"],
        summary: typeof raw.summary === "string" ? raw.summary : String(raw.summary ?? ""),
        reviewedAreas: normalizeStringList(raw.reviewedAreas) ?? [],
        blockers: normalizeStringList(raw.blockers) ?? [],
        fixes: normalizeStringList(raw.fixes) ?? [],
        validation: normalizeStringList(raw.validation) ?? [],
        residualRisks: normalizeStringList(raw.residualRisks) ?? [],
      };

      return normalized;
    } catch (error) {
      lastError = error instanceof Error ? error : new Error(String(error));
      
      if (attempt < maxRetries) {
        // Don't retry on missing file or permission errors
        const parseErr = lastError as Error;
        if (parseErr.message.includes("not found") || 
            parseErr.message.includes("ENOENT") ||
            parseErr.message.includes("EACCES")) {
          throw lastError;
        }
        
        console.log(
          `\x1b[33m[JSON Validation]\x1b[0m Attempt ${attempt}/${maxRetries} failed to read valid JSON from ${path}: ${parseErr.message}`,
        );
        
        console.log(`\x1b[36m[JSON Validation]\x1b[0m Waiting before retry...`);
        await new Promise((resolve) => setTimeout(resolve, 250));
      } else {
        console.log(
          `\x1b[31m[JSON Validation]\x1b[0m Failed to read valid JSON from ${path} after ${maxRetries} attempts: ${lastError.message}`,
        );
      }
    }
  }

  throw lastError ?? new Error(`Failed to read valid JSON from ${path}.`);
}
