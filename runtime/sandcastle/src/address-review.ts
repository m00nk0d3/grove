#!/usr/bin/env node

import fs from "node:fs";
import { fileURLToPath } from "node:url";
import {
  prepareWorktree,
  validateFixablePullRequest,
  type PullRequestMetadata,
} from "./ci-fix.js";
import { runSpecialistInPane } from "./herdr-specialist.js";
import { runTrackedWorkflow, updateTrackedWorkflow } from "./runtime-state.js";
import { personaFor, SPECIALISTS } from "./specialists.js";
import { detectStackProjects } from "./stack-detector.js";
import { loadProfile, type RepoProfile } from "./project-profile.js";
import {
  detectRepo,
  getProjectProfilePath,
  planVerification,
  pullRequestChangedFiles,
  requireCleanWorktree,
  resolveAgentBackend,
  runCommand,
  describeVerificationTasks,
  verifyWorktree,
} from "./workflow-utils.js";

const NUMBER_PATTERN = /^[1-9][0-9]*$/;

// A pull request under active review can accumulate a lot of conversation.
// Enough of it reaches the agent to act on; the rest it can read from GitHub
// itself, which the prompt tells it how to do.
const MAX_THREADS = 40;
const MAX_REVIEWS = 10;
const MAX_COMMENTS = 20;

// A review body is often the whole of the feedback — a reviewer who leaves no
// line comments puts everything here, and several thousand characters of
// structured findings is normal. Truncating that to a paragraph throws the
// feedback away, so review bodies get a much larger budget than the shorter
// remarks left on a line or under the pull request.
const MAX_REVIEW_BODY_CHARS = 8000;
const MAX_COMMENT_CHARS = 3000;

export interface ReviewThreadComment {
  author: string;
  body: string;
  url: string;
}

export interface ReviewThread {
  path: string;
  line: number | null;
  isResolved: boolean;
  isOutdated: boolean;
  comments: ReviewThreadComment[];
}

export interface PullRequestReview {
  author: string;
  body: string;
  /** CHANGES_REQUESTED, APPROVED or COMMENTED. */
  state: string;
  url: string;
}

export interface PullRequestFeedback {
  threads: ReviewThread[];
  reviews: PullRequestReview[];
  comments: Array<{ author: string; body: string; url: string }>;
}

// A reviewer can attach a suggested change: GitHub renders a ```suggestion
// block as a literal replacement for the lines the thread sits on. It is the
// most precise feedback there is, so it is pulled out and shown as the
// proposed replacement rather than left buried in prose.
const SUGGESTION_PATTERN = /```suggestion\r?\n([\s\S]*?)```/g;

export function extractSuggestions(body: string): string[] {
  return [...body.matchAll(SUGGESTION_PATTERN)].map((match) =>
    match[1].replace(/\s+$/, ""),
  );
}

export function threadHasSuggestion(thread: ReviewThread): boolean {
  return thread.comments.some(
    (comment) => extractSuggestions(comment.body).length > 0,
  );
}

// An approval is not a reason to ignore what the reviewer wrote; plenty of
// reviewers approve and leave notes in the same breath.
const RELEVANT_REVIEW_STATES = new Set([
  "CHANGES_REQUESTED",
  "APPROVED",
  "COMMENTED",
]);

export interface AddressOptions {
  prNumber: string;
  /** Pick up a worktree an earlier run left dirty after a failed gate. */
  resume: boolean;
}

export function parseAddressArgs(args: string[]): AddressOptions {
  const resume = args.includes("--continue");
  const rest = args.filter((arg) => arg !== "--continue");
  if (rest.length === 1 && NUMBER_PATTERN.test(rest[0])) {
    return { prNumber: rest[0], resume };
  }
  throw new Error("Usage: address <pr_number> [--continue]");
}

function truncate(body: string, limit: number): string {
  const trimmed = body.trim();
  return trimmed.length > limit
    ? `${trimmed.slice(0, limit)}\n… (truncated; read the full text on GitHub)`
    : trimmed;
}

// selectActionableFeedback drops what the author cannot act on: resolved
// threads, their own comments, and empty bodies. Outdated threads are kept but
// marked, because the code they were left on has since changed and the agent
// has to check the current state before acting.
export function selectActionableFeedback(
  feedback: PullRequestFeedback,
  viewerLogin: string,
): PullRequestFeedback {
  const notViewer = (author: string) =>
    author.toLowerCase() !== viewerLogin.toLowerCase();
  return {
    threads: feedback.threads
      .filter(
        (thread) =>
          !thread.isResolved &&
          thread.comments.some(
            (comment) => notViewer(comment.author) && comment.body.trim(),
          ),
      )
      .slice(0, MAX_THREADS),
    reviews: feedback.reviews
      .filter(
        (review) =>
          notViewer(review.author) &&
          review.body.trim() &&
          RELEVANT_REVIEW_STATES.has(review.state),
      )
      .slice(0, MAX_REVIEWS),
    comments: feedback.comments
      .filter((comment) => notViewer(comment.author) && comment.body.trim())
      .slice(0, MAX_COMMENTS),
  };
}

export function countFeedback(feedback: PullRequestFeedback): number {
  return (
    feedback.threads.length +
    feedback.reviews.length +
    feedback.comments.length
  );
}

export function countSuggestions(feedback: PullRequestFeedback): number {
  return feedback.threads.filter(threadHasSuggestion).length;
}

// The feedback goes into the prompt rather than a file. A file outside the
// worktree needs separate approval from some agents, and a file inside it
// would show up in the diff the agent is supposed to be keeping clean.
export function formatFeedback(feedback: PullRequestFeedback): string {
  const sections: string[] = [];

  const renderReviews = (heading: string, states: string[]): void => {
    const matching = feedback.reviews.filter((review) =>
      states.includes(review.state),
    );
    if (matching.length === 0) return;
    sections.push(
      `## ${heading}\n\n${matching
        .map(
          (review) =>
            `### @${review.author}\n${truncate(review.body, MAX_REVIEW_BODY_CHARS)}\n\n<${review.url}>`,
        )
        .join("\n\n")}`,
    );
  };

  renderReviews("Changes requested", ["CHANGES_REQUESTED"]);
  renderReviews(
    "Other review notes (the reviewer approved or commented, and still wrote this)",
    ["APPROVED", "COMMENTED"],
  );

  if (feedback.threads.length > 0) {
    sections.push(
      `## Unresolved review threads\n\n${feedback.threads
        .map((thread, index) => {
          const where = thread.line
            ? `${thread.path}:${thread.line}`
            : thread.path;
          const outdated = thread.isOutdated
            ? " — OUTDATED, the code here has changed since"
            : "";
          const conversation = thread.comments
            .map((comment) => {
              const suggestions = extractSuggestions(comment.body);
              if (suggestions.length === 0) {
                return `- @${comment.author}: ${truncate(comment.body, MAX_COMMENT_CHARS)}`;
              }
              // Show the proposed replacement verbatim; truncating it would
              // turn an exact change into a guess.
              const blocks = suggestions
                .map((code) => `\`\`\`suggestion\n${code}\n\`\`\``)
                .join("\n");
              return `- @${comment.author} proposed a SUGGESTED CHANGE for these lines:\n${blocks}`;
            })
            .join("\n");
          return `### ${index + 1}. ${where}${outdated}\n${conversation}`;
        })
        .join("\n\n")}`,
    );
  }

  if (feedback.comments.length > 0) {
    sections.push(
      `## Pull request comments\n\nThese are conversation rather than line comments; some ask for a change and some do not.\n\n${feedback.comments
        .map((comment) => `- @${comment.author}: ${truncate(comment.body, MAX_COMMENT_CHARS)}`)
        .join("\n")}`,
    );
  }

  return sections.join("\n\n");
}

// The gate runs the repository's own tests and waits for them, so a failure is
// a real result rather than a timeout. A failure caused by the agent's own
// change is the agent's to fix, and one attempt is often not enough — a first
// fix can surface the next failure behind it. Keep handing the output back
// until the gate passes or the attempts run out, so the command either
// delivers something that builds or says plainly that it could not.
const MAX_VALIDATION_ATTEMPTS = 3;

export async function validateWithRepair(
  targetDir: string,
  repo: string,
  prNumber: string,
  persona: string,
  runSpecialist: (role: string, promptText: string) => void,
  verify: (dir: string) => void | Promise<void> = verifyWorktree,
  profile: RepoProfile | null = null,
): Promise<void> {
  const commands = describeVerificationTasks(
    planVerification(detectStackProjects(targetDir), [], profile),
  );

  for (let attempt = 1; attempt <= MAX_VALIDATION_ATTEMPTS; attempt += 1) {
    try {
      await verify(targetDir);
      if (attempt > 1) {
        console.log(
          `\x1b[32m[Validation]\x1b[0m Passed after ${attempt - 1} repair attempt(s).`,
        );
      }
      return;
    } catch (error) {
      const failure = error instanceof Error ? error.message : String(error);
      if (attempt === MAX_VALIDATION_ATTEMPTS) {
        throw new Error(
          `The delivery gate still fails after ${MAX_VALIDATION_ATTEMPTS - 1} repair attempts. ` +
            `The changes are left in ${targetDir} for inspection; rerun with --continue once the cause is understood.\n${failure}`,
        );
      }
      console.log(
        `\x1b[33m[Validation]\x1b[0m Gate failed (attempt ${attempt}); returning the output to the agent.`,
      );
      runSpecialist(
        `review-responder-validation-fix-${attempt}`,
        SPECIALISTS.REVIEW_VALIDATION_FIXES(
          persona,
          repo,
          prNumber,
          commands,
          failure,
          attempt,
        ),
      );
    }
  }
}

// On a resume the worktree already holds the previous run's edits. Saying so
// is the difference between the agent finishing that work and starting over on
// top of it.
export function withResumeNote(feedback: string): string {
  return `${feedback}

## Work already in progress

An earlier run of this command left uncommitted changes in this worktree; its
delivery gate failed before they could be committed. Inspect the output of
'git diff' first and treat that work as a starting point: keep what is correct,
finish what is incomplete, and fix whatever made validation fail. Do not start
again from scratch, and do not revert it wholesale without saying why.`;
}

function readReviewDecision(repo: string, prNumber: string): string {
  try {
    return runCommand("gh", [
      "pr",
      "view",
      prNumber,
      "--repo",
      repo,
      "--json",
      "reviewDecision",
      "--jq",
      ".reviewDecision // \"\"",
    ]).trim();
  } catch {
    return ""; // an unknown decision only costs the approval notice
  }
}

function readPullRequest(repo: string, prNumber: string): PullRequestMetadata {
  const output = runCommand("gh", [
    "pr",
    "view",
    prNumber,
    "--repo",
    repo,
    "--json",
    "number,title,url,headRefName,headRefOid,baseRefName,isCrossRepository,state,headRepository,headRepositoryOwner",
  ]);
  const value = JSON.parse(output) as Partial<PullRequestMetadata>;
  // A field this command uses but never asked for reads as undefined and
  // degrades silently — baseRefName decides which specialist answers the
  // review — so the shape is checked rather than assumed.
  if (
    typeof value.number !== "number" ||
    typeof value.headRefName !== "string" ||
    typeof value.headRefOid !== "string" ||
    typeof value.baseRefName !== "string" ||
    typeof value.isCrossRepository !== "boolean" ||
    typeof value.state !== "string"
  ) {
    throw new Error(`GitHub returned invalid metadata for ${repo}#${prNumber}.`);
  }
  return value as PullRequestMetadata;
}

export function parseFeedbackResponse(raw: string): PullRequestFeedback {
  const response = JSON.parse(raw) as {
    data?: {
      repository?: {
        pullRequest?: {
          reviewThreads?: {
            nodes?: Array<{
              path?: string;
              line?: number | null;
              isResolved?: boolean;
              isOutdated?: boolean;
              comments?: {
                nodes?: Array<{
                  author?: { login?: string };
                  body?: string;
                  url?: string;
                }>;
              };
            }>;
          };
          reviews?: {
            nodes?: Array<{
              author?: { login?: string };
              body?: string;
              state?: string;
              url?: string;
            }>;
          };
          comments?: {
            nodes?: Array<{
              author?: { login?: string };
              body?: string;
              url?: string;
            }>;
          };
        };
      };
    };
  };
  const pullRequest = response.data?.repository?.pullRequest;
  return {
    threads: (pullRequest?.reviewThreads?.nodes ?? []).map((thread) => ({
      path: thread.path ?? "(unknown file)",
      line: thread.line ?? null,
      isResolved: thread.isResolved === true,
      isOutdated: thread.isOutdated === true,
      comments: (thread.comments?.nodes ?? []).map((comment) => ({
        author: comment.author?.login ?? "unknown",
        body: comment.body ?? "",
        url: comment.url ?? "",
      })),
    })),
    reviews: (pullRequest?.reviews?.nodes ?? []).map((review) => ({
      author: review.author?.login ?? "unknown",
      body: review.body ?? "",
      state: review.state ?? "",
      url: review.url ?? "",
    })),
    comments: (pullRequest?.comments?.nodes ?? []).map((comment) => ({
      author: comment.author?.login ?? "unknown",
      body: comment.body ?? "",
      url: comment.url ?? "",
    })),
  };
}

function readFeedback(repo: string, prNumber: string): PullRequestFeedback {
  const [owner, name] = repo.split("/");
  const query = `query($owner: String!, $name: String!, $number: Int!) {
    repository(owner: $owner, name: $name) {
      pullRequest(number: $number) {
        reviewThreads(first: 100) {
          nodes {
            path
            line
            isResolved
            isOutdated
            comments(first: 20) {
              nodes { author { login } body url }
            }
          }
        }
        reviews(last: 20) {
          nodes { author { login } body state url }
        }
        comments(last: 50) {
          nodes { author { login } body url }
        }
      }
    }
  }`;
  return parseFeedbackResponse(
    runCommand("gh", [
      "api",
      "graphql",
      "-F",
      `owner=${owner}`,
      "-F",
      `name=${name}`,
      "-F",
      `number=${prNumber}`,
      "-f",
      `query=${query}`,
    ]),
  );
}

export async function main(args: string[] = process.argv.slice(2)): Promise<void> {
  if (process.env.HERDR_ENV !== "1") {
    throw new Error("address must be run from a Herdr-managed pane.");
  }
  const { prNumber, resume } = parseAddressArgs(args);
  const repoRoot = runCommand("git", ["rev-parse", "--show-toplevel"]);
  const repo = detectRepo(repoRoot);
  if (!repo) throw new Error("Unable to identify this checkout's GitHub repository.");

  const metadata = readPullRequest(repo, prNumber);
  validateFixablePullRequest(metadata, repo, "address");

  const reviewDecision = readReviewDecision(repo, prNumber);
  const viewerLogin = runCommand("gh", ["api", "user", "--jq", ".login"]);
  const feedback = selectActionableFeedback(
    readFeedback(repo, prNumber),
    viewerLogin,
  );
  const total = countFeedback(feedback);
  if (total === 0) {
    console.log(
      `✅ No unaddressed review feedback from others on ${metadata.url}`,
    );
    return;
  }

  runCommand("git", ["fetch", "--prune", "origin"], { cwd: repoRoot });
  const targetDir = prepareWorktree(repoRoot, metadata, "address", resume);
  const originalHead = runCommand("git", ["rev-parse", "HEAD"], {
    cwd: targetDir,
  });

  console.log(`\n# 💬 Review Feedback for ${repo}#${prNumber}\n`);
  console.log(`- **PR:** ${metadata.title}`);
  console.log(`- **Unresolved threads:** ${feedback.threads.length}`);
  console.log(`- **Review notes:** ${feedback.reviews.length}`);
  console.log(`- **Suggested changes:** ${countSuggestions(feedback)}`);
  console.log(`- **Comments:** ${feedback.comments.length}`);
  console.log(`- **Worktree:** ${targetDir}`);
  if (resume) {
    console.log("- ↻ **Continuing** from an earlier run: the worktree already holds its changes.");
  }
  if (reviewDecision === "APPROVED") {
    // Worth saying out loud: a new commit can retract an approval the author
    // already has, depending on the repository's branch protection.
    console.log(
      "- ⚠️  **Already approved.** Pushing may dismiss that approval.",
    );
  }
  console.log();

  let specialistPaneId: string | null = null;
  const reportAgent = (status: "working" | "blocked", summary: string): void => {
    updateTrackedWorkflow({
      status: status === "blocked" ? "blocked" : "running",
      agents: specialistPaneId
        ? [
            {
              id: `${process.env.GROVE_WORKFLOW_RUN_ID ?? "address"}:review-responder`,
              kind: resolveAgentBackend(),
              name: "review-responder",
              status,
              summary,
              pane_id: specialistPaneId,
            },
          ]
        : [],
    });
  };

  // The specialist that answers this review is the one for the code the pull
  // request actually changes.
  const projects = detectStackProjects(targetDir);
  const profile = loadProfile(
    getProjectProfilePath(repoRoot),
    targetDir,
    projects,
  ).profile;
  const persona = personaFor(
    "implementation",
    projects,
    profile,
    pullRequestChangedFiles(targetDir, metadata.baseRefName),
  );
  const runSpecialist = (role: string, promptText: string): void => {
    runSpecialistInPane({
      role,
      promptText,
      targetDir,
      issueOrPrNumber: prNumber,
      onPaneChanged: (paneId) => {
        specialistPaneId = paneId;
        reportAgent("working", `Addressing review feedback on #${prNumber}`);
      },
      onAgentStatus: (status, summary) => reportAgent(status, summary),
    });
  };

  runSpecialist(
    "review-responder",
    SPECIALISTS.REVIEW_RESPONDER(
      persona,
      repo,
      prNumber,
      metadata.title,
      resume ? withResumeNote(formatFeedback(feedback)) : formatFeedback(feedback),
    ),
  );

  const currentHead = runCommand("git", ["rev-parse", "HEAD"], {
    cwd: targetDir,
  });
  if (currentHead !== originalHead) {
    throw new Error("The review responder committed changes unexpectedly.");
  }
  if (runCommand("git", ["status", "--porcelain"], { cwd: targetDir }) === "") {
    // Reviewers do sometimes ask only questions. That is a real outcome, not a
    // failure, but it still needs the author rather than a silent success.
    console.log(
      `\n💬 The review responder made no code changes to ${metadata.url}.\n` +
        "Its final response above explains each item; anything needing a reply is yours to post.",
    );
    return;
  }

  await validateWithRepair(
    targetDir,
    repo,
    prNumber,
    persona,
    runSpecialist,
    (dir) => verifyWorktree(dir, undefined, profile),
    profile,
  );
  runCommand("git", ["diff", "--check"], { cwd: targetDir });
  runCommand("git", ["add", "-A"], { cwd: targetDir });
  runCommand(
    "git",
    [
      "commit",
      "-m",
      `fix: address review feedback on PR #${prNumber}`,
      "-m",
      "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>",
    ],
    { cwd: targetDir },
  );
  runCommand("git", ["push", "origin", metadata.headRefName], {
    cwd: targetDir,
  });
  requireCleanWorktree(targetDir);

  console.log(`\n✅ Review feedback addressed and pushed to ${metadata.url}`);
  console.log(`🌳 Worktree preserved at ${targetDir}`);
  console.log(
    "💬 Replying to reviewers and resolving threads is left to you, on GitHub.",
  );
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  runTrackedWorkflow("address", process.argv.slice(2), main).catch(
    (error: unknown) => {
      console.error(error instanceof Error ? error.message : String(error));
      process.exitCode = 1;
    },
  );
}
