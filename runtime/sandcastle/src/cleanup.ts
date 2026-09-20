#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { createInterface } from "node:readline/promises";
import { fileURLToPath } from "node:url";
import { detectRepo, runCommand, type CommandRunner } from "./workflow-utils.js";
import { runTrackedWorkflow } from "./runtime-state.js";

export interface WorktreeInfo {
  path: string;
  branch: string | null;
  locked: boolean;
}

export interface MergedPullRequest {
  number: number;
  headRefName: string;
  url: string;
}

export interface CleanupWorktree extends WorktreeInfo {
  pullRequest: MergedPullRequest;
}

export interface CleanupBranch {
  branch: string;
  pullRequest: MergedPullRequest;
}

export interface CleanupPlan {
  worktrees: CleanupWorktree[];
  branches: CleanupBranch[];
  remoteBranches: CleanupBranch[];
  skipped: string[];
}

export function parseWorktrees(output: string): WorktreeInfo[] {
  if (!output.trim()) return [];
  return output.trim().split(/\n\n+/).map((block) => {
    const lines = block.split("\n");
    const worktreeLine = lines.find((line) => line.startsWith("worktree "));
    if (!worktreeLine) {
      throw new Error("Git returned an invalid worktree record.");
    }
    const branchLine = lines.find((line) => line.startsWith("branch refs/heads/"));
    return {
      path: worktreeLine.slice("worktree ".length),
      branch: branchLine?.slice("branch refs/heads/".length) ?? null,
      locked: lines.some((line) => line === "locked" || line.startsWith("locked ")),
    };
  });
}

function reviewPrNumber(worktreePath: string): number | null {
  const match = worktreePath.match(
    /(?:^|\/)\.sandcastle\/reviews\/pr-([1-9][0-9]*)-[^/]+$/,
  );
  return match ? Number(match[1]) : null;
}

export function buildCleanupPlan(
  worktrees: WorktreeInfo[],
  localBranches: string[],
  remoteBranches: string[],
  mergedPullRequests: MergedPullRequest[],
  protectedPaths: string[],
  defaultBranch: string,
  isClean: (worktreePath: string) => boolean,
): CleanupPlan {
  const mergedByBranch = new Map(
    mergedPullRequests.map((pullRequest) => [
      pullRequest.headRefName,
      pullRequest,
    ]),
  );
  const mergedByNumber = new Map(
    mergedPullRequests.map((pullRequest) => [pullRequest.number, pullRequest]),
  );
  const protectedPathSet = new Set(protectedPaths.map((item) => path.resolve(item)));
  const skipped: string[] = [];
  const worktreeCandidates: CleanupWorktree[] = [];
  const protectedWorktreeBranches = new Set<string>();

  for (const worktree of worktrees) {
    if (protectedPathSet.has(path.resolve(worktree.path))) {
      if (worktree.branch) protectedWorktreeBranches.add(worktree.branch);
      continue;
    }
    const pullRequest = worktree.branch
      ? mergedByBranch.get(worktree.branch)
      : mergedByNumber.get(reviewPrNumber(worktree.path) ?? -1);
    if (!pullRequest) continue;
    if (worktree.locked) {
      skipped.push(`Locked worktree: ${worktree.path}`);
      if (worktree.branch) protectedWorktreeBranches.add(worktree.branch);
      continue;
    }
    if (!isClean(worktree.path)) {
      skipped.push(`Dirty worktree: ${worktree.path}`);
      if (worktree.branch) protectedWorktreeBranches.add(worktree.branch);
      continue;
    }
    worktreeCandidates.push({ ...worktree, pullRequest });
  }

  const branches = localBranches
    .filter(
      (branch) =>
        branch !== defaultBranch &&
        !protectedWorktreeBranches.has(branch) &&
        mergedByBranch.has(branch),
    )
    .map((branch) => ({
      branch,
      pullRequest: mergedByBranch.get(branch)!,
    }));
  const remoteBranchCandidates = remoteBranches
    .filter(
      (branch) =>
        branch !== defaultBranch &&
        !protectedWorktreeBranches.has(branch) &&
        mergedByBranch.has(branch),
    )
    .map((branch) => ({
      branch,
      pullRequest: mergedByBranch.get(branch)!,
    }));

  return {
    worktrees: worktreeCandidates,
    branches,
    remoteBranches: remoteBranchCandidates,
    skipped,
  };
}

function readMergedPullRequests(repo: string): MergedPullRequest[] {
  const output = runCommand("gh", [
    "pr",
    "list",
    "--repo",
    repo,
    "--state",
    "merged",
    "--limit",
    "1000",
    "--json",
    "number,headRefName,url",
  ]);
  const value = JSON.parse(output) as unknown;
  if (
    !Array.isArray(value) ||
    !value.every(
      (item) =>
        item &&
        typeof item === "object" &&
        typeof item.number === "number" &&
        typeof item.headRefName === "string" &&
        typeof item.url === "string",
    )
  ) {
    throw new Error("GitHub returned invalid merged pull request metadata.");
  }
  return value as MergedPullRequest[];
}

function printPlan(plan: CleanupPlan): void {
  console.log("\n# 🧹 Cleanup Preview\n");
  if (plan.worktrees.length > 0) {
    console.log("## 🌳 Worktrees");
    for (const item of plan.worktrees) {
      console.log(`- #${item.pullRequest.number} ${item.path}`);
    }
    console.log();
  }
  if (plan.branches.length > 0) {
    console.log("## 🌿 Local Branches");
    for (const item of plan.branches) {
      console.log(`- #${item.pullRequest.number} ${item.branch}`);
    }
    console.log();
  }
  if (plan.remoteBranches.length > 0) {
    console.log("## ☁️ Remote Branches");
    for (const item of plan.remoteBranches) {
      console.log(`- #${item.pullRequest.number} origin/${item.branch}`);
    }
    console.log();
  }
  if (plan.skipped.length > 0) {
    console.log("## ⚠️ Skipped");
    for (const item of plan.skipped) console.log(`- ${item}`);
    console.log();
  }
}

async function confirmCleanup(): Promise<boolean> {
  const prompt = createInterface({
    input: process.stdin,
    output: process.stdout,
  });
  try {
    const answer = await prompt.question(
      "Remove the listed worktrees plus local and origin branches? [y/N] ",
    );
    return ["y", "yes"].includes(answer.trim().toLowerCase());
  } finally {
    prompt.close();
  }
}

export function executeCleanup(
  repoRoot: string,
  plan: CleanupPlan,
  runner: CommandRunner = runCommand,
): void {
  for (const worktree of plan.worktrees) {
    runner("git", ["worktree", "remove", worktree.path], { cwd: repoRoot });
  }
  for (const branch of plan.branches) {
    runner("git", ["branch", "-D", "--", branch.branch], { cwd: repoRoot });
  }
  for (const branch of plan.remoteBranches) {
    runner("git", ["push", "origin", "--delete", branch.branch], {
      cwd: repoRoot,
    });
  }
  runner("git", ["worktree", "prune"], { cwd: repoRoot });
}

export async function main(args: string[] = process.argv.slice(2)): Promise<void> {
  if (args.length > 0) throw new Error("Usage: clean");
  const repoRoot = runCommand("git", ["rev-parse", "--show-toplevel"]);
  const currentWorktree = fs.realpathSync(repoRoot);
  const repo = detectRepo(repoRoot);
  if (!repo) throw new Error("Unable to identify this checkout's GitHub repository.");

  console.log(`\x1b[36m[Cleanup]\x1b[0m Fetching merged PR state for ${repo}...`);
  runCommand("git", ["fetch", "--prune", "origin"], { cwd: repoRoot });
  const defaultBranch = runCommand(
    "gh",
    ["repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name"],
  );
  const worktrees = parseWorktrees(
    runCommand("git", ["worktree", "list", "--porcelain"], { cwd: repoRoot }),
  );
  const localBranches = runCommand(
    "git",
    ["for-each-ref", "--format=%(refname:short)", "refs/heads"],
    { cwd: repoRoot },
  ).split("\n").filter(Boolean);
  const remoteBranches = runCommand(
    "git",
    [
      "for-each-ref",
      "--format=%(refname:strip=3)",
      "refs/remotes/origin",
    ],
    { cwd: repoRoot },
  ).split("\n").filter((branch) => branch && branch !== "HEAD");
  const plan = buildCleanupPlan(
    worktrees,
    localBranches,
    remoteBranches,
    readMergedPullRequests(repo),
    [currentWorktree],
    defaultBranch,
    (worktreePath) =>
      runCommand("git", ["status", "--porcelain"], { cwd: worktreePath }) === "",
  );

  printPlan(plan);
  if (
    plan.worktrees.length === 0 &&
    plan.branches.length === 0 &&
    plan.remoteBranches.length === 0
  ) {
    console.log("✨ Nothing eligible for cleanup.");
    return;
  }
  if (!(await confirmCleanup())) {
    console.log("🛑 Cleanup cancelled; nothing was removed.");
    return;
  }
  executeCleanup(repoRoot, plan);
  console.log(
    `✅ Removed ${plan.worktrees.length} worktree(s), ${plan.branches.length} local branch(es), and ${plan.remoteBranches.length} remote branch(es).`,
  );
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  runTrackedWorkflow("clean", process.argv.slice(2), main).catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
