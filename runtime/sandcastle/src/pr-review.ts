#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runSpecialistInPane } from "./herdr-specialist.js";
import { runTrackedWorkflow } from "./runtime-state.js";
import {
  buildPullRequestReviewPrompt,
  chooseReviewPostAction,
  formatPullRequestVerdict,
  readPullRequestVerdict,
  type ReviewPostAction,
} from "./pull-request-review.js";
import {
  detectRepo,
  getWorkflowStatePath,
  requireCleanWorktree,
  runCommand,
} from "./workflow-utils.js";

interface ReviewCliOptions {
  requestedRepo: string | null;
  prNumber: string;
}

interface PullRequestMetadata {
  number: number;
  title: string;
  url: string;
  body: string;
  headRefName: string;
  headRefOid: string;
  baseRefName: string;
  isDraft: boolean;
  author: { login: string };
  closingIssuesReferences: Array<{
    number: number;
    title: string;
    url: string;
  }>;
}

const REPO_PATTERN = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;
const NUMBER_PATTERN = /^[1-9][0-9]*$/;

export function parseReviewCliArgs(args: string[]): ReviewCliOptions {
  if (args.length === 1 && NUMBER_PATTERN.test(args[0])) {
    return { requestedRepo: null, prNumber: args[0] };
  }
  if (
    args.length === 2 &&
    REPO_PATTERN.test(args[0]) &&
    NUMBER_PATTERN.test(args[1])
  ) {
    return { requestedRepo: args[0], prNumber: args[1] };
  }
  throw new Error(
    "Usage: review <pr_number> OR review <owner/repo> <pr_number>",
  );
}

export function canSubmitReviewDecision(
  authorLogin: string,
  viewerLogin: string,
): boolean {
  return authorLogin.toLowerCase() !== viewerLogin.toLowerCase();
}

function readPullRequestMetadata(
  repo: string,
  prNumber: string,
): PullRequestMetadata {
  const output = runCommand("gh", [
    "pr",
    "view",
    prNumber,
    "--repo",
    repo,
    "--json",
    "number,title,url,body,headRefName,headRefOid,baseRefName,isDraft,author,closingIssuesReferences",
  ]);
  const value = JSON.parse(output) as Partial<PullRequestMetadata>;
  if (
    typeof value.number !== "number" ||
    typeof value.title !== "string" ||
    typeof value.url !== "string" ||
    typeof value.headRefOid !== "string" ||
    typeof value.headRefName !== "string" ||
    typeof value.baseRefName !== "string" ||
    !value.author ||
    typeof value.author.login !== "string" ||
    !Array.isArray(value.closingIssuesReferences)
  ) {
    throw new Error(`GitHub returned invalid metadata for ${repo}#${prNumber}.`);
  }
  return value as PullRequestMetadata;
}

function prepareReviewWorktree(
  repoRoot: string,
  prNumber: string,
  headOid: string,
): string {
  const reviewRoot = path.join(repoRoot, ".sandcastle", "reviews");
  const worktreePath = path.join(
    reviewRoot,
    `pr-${prNumber}-${headOid.slice(0, 8)}`,
  );
  if (fs.existsSync(worktreePath)) {
    const currentHead = runCommand("git", ["rev-parse", "HEAD"], {
      cwd: worktreePath,
    });
    if (currentHead !== headOid) {
      throw new Error(
        `Existing review worktree is at ${currentHead}, expected ${headOid}: ${worktreePath}`,
      );
    }
    requireCleanWorktree(worktreePath);
    return worktreePath;
  }

  fs.mkdirSync(reviewRoot, { recursive: true });
  runCommand("git", ["fetch", "origin", `pull/${prNumber}/head`], {
    cwd: repoRoot,
  });
  runCommand("git", ["worktree", "add", "--detach", worktreePath, headOid], {
    cwd: repoRoot,
  });
  return worktreePath;
}

function postReview(
  repo: string,
  prNumber: string,
  action: ReviewPostAction,
  reportPath: string,
): void {
  if (action === "none") return;
  const flag = {
    approve: "--approve",
    request_changes: "--request-changes",
    comment: "--comment",
  }[action];
  runCommand("gh", [
    "pr",
    "review",
    prNumber,
    "--repo",
    repo,
    flag,
    "--body-file",
    reportPath,
  ]);
}

export async function main(args: string[] = process.argv.slice(2)): Promise<void> {
  if (process.env.HERDR_ENV !== "1") {
    throw new Error("review must be run from a Herdr-managed pane.");
  }
  const { requestedRepo, prNumber } = parseReviewCliArgs(args);
  const repoRoot = runCommand("git", ["rev-parse", "--show-toplevel"]);
  const detectedRepo = detectRepo(repoRoot);
  if (!detectedRepo) {
    throw new Error("Unable to identify the current checkout's GitHub repository.");
  }
  if (requestedRepo && requestedRepo.toLowerCase() !== detectedRepo.toLowerCase()) {
    throw new Error(
      `Requested repository ${requestedRepo} does not match the current checkout ${detectedRepo}.`,
    );
  }

  const repo = detectedRepo;
  const metadata = readPullRequestMetadata(repo, prNumber);
  const worktreePath = prepareReviewWorktree(
    repoRoot,
    prNumber,
    metadata.headRefOid,
  );
  const dataDir = path.dirname(getWorkflowStatePath(repoRoot, `review-${prNumber}`));
  const verdictPath = path.join(
    dataDir,
    `pr-${prNumber}-${metadata.headRefOid.slice(0, 8)}-verdict.json`,
  );
  const reportPath = path.join(
    dataDir,
    `pr-${prNumber}-${metadata.headRefOid.slice(0, 8)}-review.md`,
  );
  let specialistPaneId: string | null = null;

  console.log(`\x1b[32m[Pull Request]\x1b[0m ${repo}#${prNumber}: ${metadata.title}`);
  console.log(`\x1b[36m[Worktree]\x1b[0m ${worktreePath}`);
  fs.rmSync(verdictPath, { force: true });
  try {
    runSpecialistInPane({
      role: "pull-request-reviewer",
      promptText: buildPullRequestReviewPrompt(
        repo,
        prNumber,
        JSON.stringify(metadata, null, 2),
        verdictPath,
      ),
      targetDir: worktreePath,
      issueOrPrNumber: prNumber,
      completionArtifacts: [verdictPath],
      completionValidator: () => {
        readPullRequestVerdict(verdictPath);
      },
      onPaneChanged: (paneId) => {
        specialistPaneId = paneId;
      },
    });
  } finally {
    if (specialistPaneId) {
      try {
        runCommand("herdr", ["pane", "close", specialistPaneId]);
      } catch (error) {
        console.error(`Failed to close Herdr pane ${specialistPaneId}:`, error);
      }
    }
  }

  const verdict = readPullRequestVerdict(verdictPath);
  const report = formatPullRequestVerdict(verdict, prNumber);
  fs.writeFileSync(reportPath, report, { mode: 0o600 });
  requireCleanWorktree(worktreePath);
  console.log(`\n\x1b[36m[Detailed Verdict]\x1b[0m\n${report}`);

  const viewerLogin = runCommand("gh", ["api", "user", "--jq", ".login"]);
  const canSubmitDecision = canSubmitReviewDecision(
    metadata.author.login,
    viewerLogin,
  );
  if (!canSubmitDecision) {
    console.log(
      "\x1b[33m[Self Review]\x1b[0m GitHub permits posting this verdict only as a comment.",
    );
  }
  const action = await chooseReviewPostAction(
    verdict.recommendation,
    canSubmitDecision,
  );
  postReview(repo, prNumber, action, reportPath);
  console.log(
    action === "none"
      ? "\x1b[33m[Not Posted]\x1b[0m Review preserved locally."
      : `\x1b[32m[Posted]\x1b[0m GitHub review action: ${action.replace("_", " ")}`,
  );
  console.log(`\x1b[32m[Worktree]\x1b[0m Preserved at ${worktreePath}`);
  console.log(`\x1b[32m[Report]\x1b[0m ${reportPath}`);
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  runTrackedWorkflow("review", process.argv.slice(2), main).catch((error: unknown) => {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`\x1b[31m[Review Error]\x1b[0m ${message}`);
    process.exit(1);
  });
}
