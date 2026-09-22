#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
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

export interface PullRequestMetadata {
  number: number;
  title: string;
  url: string;
  headRefName: string;
  headRefOid: string;
  isCrossRepository: boolean;
  state: string;
  headRepository: { name?: string; nameWithOwner?: string } | null;
  headRepositoryOwner?: { login?: string } | null;
}

// `gh pr view` returns headRepository.nameWithOwner as an empty string, unlike
// `gh pr list`, so the head repository has to be rebuilt from its owner and
// name before it can be compared with the current checkout.
export function headRepositoryOf(metadata: PullRequestMetadata): string {
  const combined = metadata.headRepository?.nameWithOwner?.trim();
  if (combined) return combined;
  const owner = metadata.headRepositoryOwner?.login?.trim();
  const name = metadata.headRepository?.name?.trim();
  return owner && name ? `${owner}/${name}` : "";
}

export interface PullRequestCheck {
  bucket: string;
  link: string;
  name: string;
  state: string;
  workflow: string;
}

const NUMBER_PATTERN = /^[1-9][0-9]*$/;

export function parseCiArgs(args: string[]): string {
  if (args.length === 1 && NUMBER_PATTERN.test(args[0])) return args[0];
  throw new Error("Usage: ci <pr_number>");
}

export function validateFixablePullRequest(
  metadata: PullRequestMetadata,
  repo: string,
  commandName = "ci",
): void {
  if (metadata.state !== "OPEN") {
    throw new Error(`Pull request #${metadata.number} is ${metadata.state.toLowerCase()}.`);
  }
  // isCrossRepository is the authoritative answer. The name comparison only
  // rejects a head repository we could actually identify, so a missing one
  // does not masquerade as a fork.
  const headRepository = headRepositoryOf(metadata);
  if (
    metadata.isCrossRepository ||
    (headRepository !== "" &&
      headRepository.toLowerCase() !== repo.toLowerCase())
  ) {
    throw new Error(
      `Pull request #${metadata.number} comes from a fork. ` +
        `The ${commandName} command only pushes branches owned by this repository.`,
    );
  }
}

export function parseFailedChecks(output: string): PullRequestCheck[] {
  let value: unknown;
  try {
    value = JSON.parse(output);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`GitHub returned invalid CI check JSON: ${detail}`);
  }
  if (!Array.isArray(value)) {
    throw new Error("GitHub returned invalid CI check data.");
  }
  const checks = value.map((item) => {
    if (
      !item ||
      typeof item !== "object" ||
      typeof item.bucket !== "string" ||
      typeof item.link !== "string" ||
      typeof item.name !== "string" ||
      typeof item.state !== "string" ||
      typeof item.workflow !== "string"
    ) {
      throw new Error("GitHub returned an invalid CI check entry.");
    }
    return item as PullRequestCheck;
  });
  return checks.filter((check) => check.bucket === "fail");
}

export function extractActionsRunIds(checks: PullRequestCheck[]): string[] {
  const ids = checks.flatMap((check) => {
    const match = check.link.match(/\/actions\/runs\/([1-9][0-9]*)/);
    return match ? [match[1]] : [];
  });
  return [...new Set(ids)];
}

export function buildCiFixPrompt(
  metadata: PullRequestMetadata,
  failedChecks: PullRequestCheck[],
  diagnosticsPath: string,
): string {
  return `
You are fixing CI failures for pull request #${metadata.number}: ${metadata.title}
PR: ${metadata.url}
Head branch: ${metadata.headRefName}

Failed checks:
${failedChecks.map((check) => `- ${check.workflow ? `${check.workflow} / ` : ""}${check.name}: ${check.state} (${check.link})`).join("\n")}

The orchestrator collected failed GitHub Actions logs at:
${diagnosticsPath}

Objective:
- Diagnose every failed check and make the smallest correct changes needed for this pull request to pass CI.

Required work:
1. Read the collected logs, pull request, linked issues, repository instructions, and relevant code.
2. Reproduce each actionable failure locally when feasible and identify its root cause.
3. Fix failures caused by the pull request without weakening tests, validation, or security controls.
4. Run the narrowest relevant checks while iterating, then run the repository's appropriate validation.
5. Leave the complete fix in the worktree without committing or pushing.

Boundaries:
- Do not commit, push, rewrite history, edit workflow results, or modify unrelated files.
- Do not hide failures with skips, broad ignores, reduced assertions, or success-shaped fallbacks.
- Preserve valid existing work if this command is resuming after an interruption.
- If a failure is external, flaky, or not safely fixable in code, do not invent a change.

Completion criteria:
- Every actionable failed check has a root-cause fix and relevant local validation passes.
`;
}

function readPullRequest(repo: string, prNumber: string): PullRequestMetadata {
  const output = runCommand("gh", [
    "pr",
    "view",
    prNumber,
    "--repo",
    repo,
    "--json",
    "number,title,url,headRefName,headRefOid,isCrossRepository,state,headRepository,headRepositoryOwner",
  ]);
  const value = JSON.parse(output) as Partial<PullRequestMetadata>;
  if (
    typeof value.number !== "number" ||
    typeof value.title !== "string" ||
    typeof value.url !== "string" ||
    typeof value.headRefName !== "string" ||
    typeof value.headRefOid !== "string" ||
    typeof value.isCrossRepository !== "boolean" ||
    typeof value.state !== "string"
    // The head repository is deliberately not required here. GitHub omits it
    // once the fork it lived in is deleted, and isCrossRepository remains the
    // authoritative answer, so its absence belongs to the fork check rather
    // than being reported as malformed metadata.
  ) {
    throw new Error(`GitHub returned invalid metadata for ${repo}#${prNumber}.`);
  }
  return value as PullRequestMetadata;
}

function readFailedChecks(repo: string, prNumber: string): PullRequestCheck[] {
  const result = spawnSync(
    "gh",
    [
      "pr",
      "checks",
      prNumber,
      "--repo",
      repo,
      "--json",
      "bucket,link,name,state,workflow",
    ],
    { encoding: "utf8", maxBuffer: 10 * 1024 * 1024 },
  );
  if (result.error) {
    throw new Error(`Unable to run gh: ${result.error.message}`);
  }
  if (![0, 1, 8].includes(result.status ?? -1)) {
    const detail = (result.stderr || result.stdout).trim();
    throw new Error(
      `gh pr checks exited with status ${result.status}${detail ? `: ${detail}` : ""}`,
    );
  }
  return parseFailedChecks(result.stdout);
}

export function safePathComponent(value: string): string {
  return value.replace(/[^A-Za-z0-9._-]+/g, "-").replace(/^-+|-+$/g, "");
}

export type WorktreeDrift =
  | { kind: "current" }
  | { kind: "behind"; behind: number }
  | { kind: "ahead"; ahead: number }
  | { kind: "diverged"; ahead: number; behind: number }
  | { kind: "unreachable" };

// How the checkout stands relative to what the pull request actually contains.
// The callers fetch before reaching here, so a head that is still unknown was
// rewritten rather than merely missed.
export function classifyWorktreeDrift(
  worktreePath: string,
  headRefOid: string,
): WorktreeDrift {
  const head = runCommand("git", ["rev-parse", "HEAD"], { cwd: worktreePath });
  if (head === headRefOid) {
    return { kind: "current" };
  }
  const known =
    spawnSync("git", ["cat-file", "-e", `${headRefOid}^{commit}`], {
      cwd: worktreePath,
    }).status === 0;
  if (!known) {
    return { kind: "unreachable" };
  }
  const [ahead, behind] = runCommand(
    "git",
    ["rev-list", "--left-right", "--count", `HEAD...${headRefOid}`],
    { cwd: worktreePath },
  )
    .split(/\s+/)
    .map((value) => Number(value));
  if (ahead === 0) {
    return { kind: "behind", behind };
  }
  if (behind === 0) {
    return { kind: "ahead", ahead };
  }
  return { kind: "diverged", ahead, behind };
}

function commitCount(count: number): string {
  return `${count} commit${count === 1 ? "" : "s"}`;
}

// A pull request branch moves while work is in flight: a suggestion committed
// from the web interface, a push from another checkout, an assessment asked for
// mid-review. A worktree that is merely behind is the ordinary case and only
// needs catching up. Every other shape of drift means the checkout holds
// something the pull request does not, which is a decision for the caller.
export function syncWorktreeToPullRequestHead(
  worktreePath: string,
  headRefOid: string,
  commandName = "ci",
): void {
  const drift = classifyWorktreeDrift(worktreePath, headRefOid);
  if (drift.kind === "current") {
    return;
  }
  const head = headRefOid.slice(0, 7);
  if (drift.kind === "unreachable") {
    throw new Error(
      `The pull request head ${head} is not in this repository, so the worktree cannot be moved to it:\n` +
        `  ${worktreePath}\n` +
        "The branch was most likely force-pushed after this checkout last fetched.\n" +
        `Run \`git fetch --prune origin\` and start ${commandName} again.`,
    );
  }
  if (drift.kind === "ahead") {
    throw new Error(
      `This worktree is ${commitCount(drift.ahead)} ahead of the pull request head ${head}:\n` +
        `  ${worktreePath}\n` +
        `Those commits are not in the pull request, so ${commandName} would work on code no reviewer has seen.\n` +
        "Push them, or reset the worktree to the pull request head, then run it again.",
    );
  }
  if (drift.kind === "diverged") {
    throw new Error(
      `This worktree and the pull request head ${head} have diverged: ` +
        `${commitCount(drift.ahead)} here, ${commitCount(drift.behind)} on the pull request.\n` +
        `  ${worktreePath}\n` +
        "A rebase or an amend on one side leaves no safe automatic answer.\n" +
        "Reconcile the branch by hand, then run it again.",
    );
  }
  const dirty = runCommand("git", ["status", "--porcelain"], {
    cwd: worktreePath,
  });
  if (dirty) {
    throw new Error(
      `The pull request head ${head} is ${commitCount(drift.behind)} ahead of this worktree, ` +
        "which has uncommitted changes:\n" +
        `  ${worktreePath}\n` +
        "Commit or stash them so the worktree can be fast-forwarded, then run it again.",
    );
  }
  runCommand("git", ["merge", "--ff-only", headRefOid], { cwd: worktreePath });
  console.log(
    `\x1b[32m[Worktree]\x1b[0m Fast-forwarded ${commitCount(drift.behind)} to the pull request head ${head}.`,
  );
}

// The worktree is keyed by the pull request branch, so a second workflow on
// the same pull request reuses the checkout the first one made.
export function prepareWorktree(
  repoRoot: string,
  metadata: PullRequestMetadata,
  subdirectory = "ci",
  allowDirty = false,
): string {
  const worktrees = parseWorktrees(
    runCommand("git", ["worktree", "list", "--porcelain"], { cwd: repoRoot }),
  );
  const existing = worktrees.find(
    (worktree) => worktree.branch === metadata.headRefName,
  );
  if (existing) {
    syncWorktreeToPullRequestHead(
      existing.path,
      metadata.headRefOid,
      subdirectory,
    );
    const ciRoot = path.join(repoRoot, ".sandcastle", subdirectory);
    const relativeToCiRoot = path.relative(ciRoot, existing.path);
    const isCiWorktree =
      relativeToCiRoot !== "" &&
      relativeToCiRoot !== ".." &&
      !relativeToCiRoot.startsWith(`..${path.sep}`) &&
      !path.isAbsolute(relativeToCiRoot);
    if (
      runCommand("git", ["status", "--porcelain"], { cwd: existing.path }) !== "" &&
      !isCiWorktree &&
      !allowDirty
    ) {
      // Git allows one worktree per branch, so this checkout is the only place
      // the pull request branch can be worked on, and the uncommitted changes
      // may equally be someone's work in progress or an earlier run of this
      // command that failed its delivery gate. Refusing by default is right;
      // --continue is how the caller says which it is.
      throw new Error(
        `The worktree for this pull request has uncommitted changes, so ${subdirectory} will not touch it:\n` +
          `  ${existing.path}\n` +
          "Git allows only one worktree per branch, so there is nowhere else to check it out.\n" +
          `If that work is your own, commit or stash it first. If it is an earlier ${subdirectory} run ` +
          `that failed, rerun with --continue to pick it up.`,
      );
    }
    return existing.path;
  }

  const worktreePath = path.join(
    repoRoot,
    ".sandcastle",
    subdirectory,
    `pr-${metadata.number}-${safePathComponent(metadata.headRefName)}`,
  );
  if (fs.existsSync(worktreePath)) {
    throw new Error(`Unregistered worktree path already exists: ${worktreePath}`);
  }

  const localBranchExists =
    spawnSync(
      "git",
      ["show-ref", "--verify", "--quiet", `refs/heads/${metadata.headRefName}`],
      { cwd: repoRoot },
    ).status === 0;
  if (localBranchExists) {
    const localHead = runCommand("git", ["rev-parse", metadata.headRefName], {
      cwd: repoRoot,
    });
    if (localHead !== metadata.headRefOid) {
      throw new Error(
        `Local branch ${metadata.headRefName} is not at the PR head. ` +
          "Update or remove it before fixing CI.",
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

function collectDiagnostics(
  repo: string,
  prNumber: string,
  failedChecks: PullRequestCheck[],
  repoRoot: string,
): string {
  const diagnosticsDir = path.join(
    path.resolve(
      repoRoot,
      runCommand("git", ["rev-parse", "--git-common-dir"], { cwd: repoRoot }),
    ),
    "agent-flow",
    `ci-pr-${prNumber}`,
  );
  fs.mkdirSync(diagnosticsDir, { recursive: true });
  const sections = [
    `# Failed CI checks for ${repo}#${prNumber}`,
    "",
    ...failedChecks.map(
      (check) =>
        `- ${check.workflow ? `${check.workflow} / ` : ""}${check.name}: ${check.state} (${check.link})`,
    ),
  ];

  for (const runId of extractActionsRunIds(failedChecks)) {
    const result = spawnSync(
      "gh",
      ["run", "view", runId, "--repo", repo, "--log-failed"],
      { encoding: "utf8", maxBuffer: 20 * 1024 * 1024 },
    );
    sections.push("", `## GitHub Actions run ${runId}`, "");
    if (result.error) {
      sections.push(`Unable to collect logs: ${result.error.message}`);
    } else if (result.status !== 0) {
      sections.push(
        `Unable to collect logs (exit ${result.status}):`,
        (result.stderr || result.stdout).trim(),
      );
    } else {
      sections.push("```text", result.stdout.trim(), "```");
    }
  }

  const diagnosticsPath = path.join(diagnosticsDir, "failures.md");
  fs.writeFileSync(diagnosticsPath, `${sections.join("\n")}\n`);
  return diagnosticsPath;
}

export async function main(args: string[] = process.argv.slice(2)): Promise<void> {
  if (process.env.HERDR_ENV !== "1") {
    throw new Error("ci must be run from a Herdr-managed pane.");
  }
  const prNumber = parseCiArgs(args);
  const repoRoot = runCommand("git", ["rev-parse", "--show-toplevel"]);
  const repo = detectRepo(repoRoot);
  if (!repo) throw new Error("Unable to identify this checkout's GitHub repository.");

  const metadata = readPullRequest(repo, prNumber);
  validateFixablePullRequest(metadata, repo);
  const failedChecks = readFailedChecks(repo, prNumber);
  if (failedChecks.length === 0) {
    console.log(`✅ No failed CI checks found for ${metadata.url}`);
    return;
  }

  runCommand("git", ["fetch", "--prune", "origin"], { cwd: repoRoot });
  const targetDir = prepareWorktree(repoRoot, metadata);
  const originalHead = runCommand("git", ["rev-parse", "HEAD"], { cwd: targetDir });
  const diagnosticsPath = collectDiagnostics(
    repo,
    prNumber,
    failedChecks,
    repoRoot,
  );

  console.log(`\n# 🔧 CI Repair for ${repo}#${prNumber}\n`);
  console.log(`- **PR:** ${metadata.title}`);
  console.log(`- **Failed checks:** ${failedChecks.length}`);
  console.log(`- **Worktree:** ${targetDir}\n`);
  for (const check of failedChecks) {
    console.log(`- ❌ ${check.workflow ? `${check.workflow} / ` : ""}${check.name}`);
  }
  console.log();

  runSpecialistInPane({
    role: "ci-fixer",
    promptText: buildCiFixPrompt(metadata, failedChecks, diagnosticsPath),
    targetDir,
    issueOrPrNumber: prNumber,
  });

  const currentHead = runCommand("git", ["rev-parse", "HEAD"], { cwd: targetDir });
  if (currentHead !== originalHead) {
    throw new Error("The CI fixer committed changes unexpectedly.");
  }
  if (runCommand("git", ["status", "--porcelain"], { cwd: targetDir }) === "") {
    throw new Error(
      "The CI fixer made no changes. The failures may be external, flaky, or require manual investigation.",
    );
  }

  verifyWorktree(targetDir);
  runCommand("git", ["diff", "--check"], { cwd: targetDir });
  runCommand("git", ["add", "-A"], { cwd: targetDir });
  runCommand(
    "git",
    [
      "commit",
      "-m",
      `fix: repair CI failures for PR #${prNumber}`,
      "-m",
      "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>",
    ],
    { cwd: targetDir },
  );
  runCommand("git", ["push", "origin", metadata.headRefName], { cwd: targetDir });
  requireCleanWorktree(targetDir);

  console.log(`\n✅ CI fixes committed and pushed to ${metadata.url}`);
  console.log(`🌳 Worktree preserved at ${targetDir}`);
  console.log("🔄 GitHub will run CI again for the new commit.");
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  runTrackedWorkflow("ci", process.argv.slice(2), main).catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  });
}
