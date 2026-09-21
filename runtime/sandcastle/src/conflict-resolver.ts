#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { createInterface } from "node:readline/promises";
import { fileURLToPath } from "node:url";
import { parseWorktrees } from "./cleanup.js";
import { runSpecialistInPane } from "./herdr-specialist.js";
import { runTrackedWorkflow } from "./runtime-state.js";
import {
  detectRepo,
  requireCleanWorktree,
  runCommand,
  verifyWorktree,
} from "./workflow-utils.js";

interface PullRequestMetadata {
  number: number;
  title: string;
  url: string;
  headRefName: string;
  headRefOid: string;
  baseRefName: string;
  isCrossRepository: boolean;
  state: string;
  headRepository: { nameWithOwner: string };
}

interface MergeResult {
  status: number;
  output: string;
}

const NUMBER_PATTERN = /^[1-9][0-9]*$/;

export function parseResolveArgs(args: string[]): string {
  if (args.length === 1 && NUMBER_PATTERN.test(args[0])) return args[0];
  throw new Error("Usage: resolve <pr_number>");
}

export function validateResolvablePullRequest(
  metadata: PullRequestMetadata,
  repo: string,
): void {
  if (metadata.state !== "OPEN") {
    throw new Error(`Pull request #${metadata.number} is ${metadata.state.toLowerCase()}.`);
  }
  if (
    metadata.isCrossRepository ||
    metadata.headRepository.nameWithOwner.toLowerCase() !== repo.toLowerCase()
  ) {
    throw new Error(
      `Pull request #${metadata.number} comes from a fork. ` +
        "The resolve command only pushes branches owned by this repository.",
    );
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
    "number,title,url,headRefName,headRefOid,baseRefName,isCrossRepository,state,headRepository",
  ]);
  const value = JSON.parse(output) as Partial<PullRequestMetadata>;
  if (
    typeof value.number !== "number" ||
    typeof value.title !== "string" ||
    typeof value.url !== "string" ||
    typeof value.headRefName !== "string" ||
    typeof value.headRefOid !== "string" ||
    typeof value.baseRefName !== "string" ||
    typeof value.isCrossRepository !== "boolean" ||
    typeof value.state !== "string" ||
    !value.headRepository ||
    typeof value.headRepository.nameWithOwner !== "string"
  ) {
    throw new Error(`GitHub returned invalid metadata for ${repo}#${prNumber}.`);
  }
  return value as PullRequestMetadata;
}

function safePathComponent(value: string): string {
  return value.replace(/[^A-Za-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "");
}

function prepareWorktree(
  repoRoot: string,
  metadata: PullRequestMetadata,
): string {
  const worktrees = parseWorktrees(
    runCommand("git", ["worktree", "list", "--porcelain"], { cwd: repoRoot }),
  );
  const existing = worktrees.find(
    (worktree) => worktree.branch === metadata.headRefName,
  );
  if (existing) {
    requireCleanWorktree(existing.path);
    const head = runCommand("git", ["rev-parse", "HEAD"], { cwd: existing.path });
    if (head !== metadata.headRefOid) {
      throw new Error(
        `Existing worktree is not at the PR head ${metadata.headRefOid}: ${existing.path}`,
      );
    }
    return existing.path;
  }

  const worktreePath = path.join(
    repoRoot,
    ".sandcastle",
    "conflicts",
    `pr-${metadata.number}-${safePathComponent(metadata.headRefName)}`,
  );
  if (fs.existsSync(worktreePath)) {
    throw new Error(`Unregistered conflict worktree path already exists: ${worktreePath}`);
  }

  const localBranchExists =
    spawnSync("git", ["show-ref", "--verify", "--quiet", `refs/heads/${metadata.headRefName}`], {
      cwd: repoRoot,
    }).status === 0;
  if (localBranchExists) {
    const localHead = runCommand("git", ["rev-parse", metadata.headRefName], {
      cwd: repoRoot,
    });
    if (localHead !== metadata.headRefOid) {
      throw new Error(
        `Local branch ${metadata.headRefName} is not at the PR head. ` +
          "Push, reset, or remove it before resolving conflicts.",
      );
    }
    runCommand("git", ["worktree", "add", worktreePath, metadata.headRefName], {
      cwd: repoRoot,
    });
  } else {
    runCommand(
      "git",
      [
        "worktree",
        "add",
        "--track",
        "-b",
        metadata.headRefName,
        worktreePath,
        `origin/${metadata.headRefName}`,
      ],
      { cwd: repoRoot },
    );
  }
  return worktreePath;
}

function mergeWithoutCommit(
  targetDir: string,
  baseRefName: string,
): MergeResult {
  const result = spawnSync(
    "git",
    ["merge", "--no-commit", "--no-ff", `origin/${baseRefName}`],
    {
      cwd: targetDir,
      encoding: "utf8",
      maxBuffer: 10 * 1024 * 1024,
    },
  );
  if (result.error) {
    throw new Error(`Unable to run git merge: ${result.error.message}`);
  }
  return {
    status: result.status ?? 1,
    output: `${result.stdout}${result.stderr}`.trim(),
  };
}

export function buildConflictPrompt(
  metadata: PullRequestMetadata,
  conflictedFiles: string[],
): string {
  return `
You are resolving merge conflicts for pull request #${metadata.number}: ${metadata.title}
PR: ${metadata.url}
Head branch: ${metadata.headRefName}
Base branch: ${metadata.baseRefName}

Conflicted files:
${conflictedFiles.map((file) => `- ${file}`).join("\n")}

Objective:
- Resolve every merge conflict while preserving the intended behavior from both the pull request and the latest base branch.

Required work:
1. Read the pull request, linked issues, both sides of every conflict, and relevant surrounding code.
2. Resolve all conflict markers with the smallest coherent integration.
3. Preserve upstream changes unless the PR intentionally supersedes them.
4. Run focused tests while iterating.
5. Leave all resolved files in the worktree without committing or pushing.

Boundaries:
- Do not abort the merge, commit, push, rewrite history, or modify unrelated files.
- Do not blindly choose "ours" or "theirs"; reconcile behavior using repository evidence.
- Do not weaken tests or suppress failures.

Completion criteria:
- No unmerged paths or conflict markers remain, and focused tests pass.
`;
}

async function confirmDelivery(): Promise<boolean> {
  const prompt = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    const answer = await prompt.question(
      "Commit and push this conflict resolution? [y/N] ",
    );
    return ["y", "yes"].includes(answer.trim().toLowerCase());
  } finally {
    prompt.close();
  }
}

export async function main(args: string[] = process.argv.slice(2)): Promise<void> {
  if (process.env.HERDR_ENV !== "1") {
    throw new Error("resolve must be run from a Herdr-managed pane.");
  }
  const prNumber = parseResolveArgs(args);
  const repoRoot = runCommand("git", ["rev-parse", "--show-toplevel"]);
  const repo = detectRepo(repoRoot);
  if (!repo) throw new Error("Unable to identify this checkout's GitHub repository.");

  const metadata = readPullRequest(repo, prNumber);
  validateResolvablePullRequest(metadata, repo);
  runCommand("git", ["fetch", "--prune", "origin"], { cwd: repoRoot });
  const targetDir = prepareWorktree(repoRoot, metadata);
  const originalHead = runCommand("git", ["rev-parse", "HEAD"], { cwd: targetDir });

  console.log(`\n# 🔀 Conflict Resolution for ${repo}#${prNumber}\n`);
  console.log(`- **PR:** ${metadata.title}`);
  console.log(`- **Merge:** origin/${metadata.baseRefName} → ${metadata.headRefName}`);
  console.log(`- **Worktree:** ${targetDir}\n`);

  const merge = mergeWithoutCommit(targetDir, metadata.baseRefName);
  const conflictedFiles = runCommand(
    "git",
    ["diff", "--name-only", "--diff-filter=U"],
    { cwd: targetDir },
  ).split("\n").filter(Boolean);
  if (merge.status !== 0 && conflictedFiles.length === 0) {
    throw new Error(`Merge failed without resolvable conflicts:\n${merge.output}`);
  }
  if (
    merge.status === 0 &&
    runCommand("git", ["status", "--porcelain"], { cwd: targetDir }) === ""
  ) {
    console.log("✅ The PR branch already contains the latest base branch.");
    return;
  }

  if (conflictedFiles.length > 0) {
    console.log("## 🚧 Conflicts");
    for (const file of conflictedFiles) console.log(`- ${file}`);
    console.log();
    runSpecialistInPane({
      role: "conflict-resolver",
      promptText: buildConflictPrompt(metadata, conflictedFiles),
      targetDir,
      issueOrPrNumber: prNumber,
    });
  } else {
    console.log("✅ Base branch merged cleanly; no manual conflicts were required.\n");
  }

  const remainingConflicts = runCommand(
    "git",
    ["diff", "--name-only", "--diff-filter=U"],
    { cwd: targetDir },
  );
  if (remainingConflicts) {
    throw new Error(`Unresolved merge conflicts remain:\n${remainingConflicts}`);
  }
  const currentHead = runCommand("git", ["rev-parse", "HEAD"], { cwd: targetDir });
  if (currentHead !== originalHead) {
    throw new Error("The conflict resolver committed changes unexpectedly.");
  }
  verifyWorktree(targetDir);
  runCommand("git", ["diff", "--cached", "--check"], { cwd: targetDir });

  console.log("\n## ✅ Resolution Ready\n");
  console.log(`- Conflicts resolved: ${conflictedFiles.length}`);
  console.log("- Full deterministic validation passed");
  console.log(`- Worktree preserved at: ${targetDir}\n`);

  if (!(await confirmDelivery())) {
    console.log("🛑 Not committed or pushed. The resolved merge remains in the worktree.");
    return;
  }
  runCommand("git", ["add", "-A"], { cwd: targetDir });
  runCommand(
    "git",
    [
      "commit",
      "-m",
      `chore: resolve conflicts for PR #${prNumber}`,
      "-m",
      "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>",
    ],
    { cwd: targetDir },
  );
  runCommand("git", ["push", "origin", metadata.headRefName], { cwd: targetDir });
  requireCleanWorktree(targetDir);
  console.log(`✅ Conflict resolution committed and pushed to ${metadata.url}`);
  console.log(`🌳 Worktree preserved at ${targetDir}`);
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  runTrackedWorkflow("resolve", process.argv.slice(2), main).catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
