#!/usr/bin/env node

import { createWorktree } from "@ai-hero/sandcastle";
import { createHash } from "node:crypto";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import {
  SPECIALISTS,
  buildImplementationPersona,
  builtinReviewSections,
  getImplementationPrompt,
  personaFor,
} from "./specialists.js";
import {
  detectStack,
  detectStackProjects,
  detectSurfaces,
  readDirectDependencies,
  stackLabel,
  TechStack,
} from "./stack-detector.js";
import {
  fingerprintRepoShape,
  loadProfile,
  readProfileDraft,
  writeProfile,
  type RepoProfile,
  type RoleKey,
} from "./project-profile.js";
import { runSpecialistInPane } from "./herdr-specialist.js";
import {
  collectChangedFiles,
  hasDocumentationSurface,
  planReviewPasses,
  resolveMaxReviewPasses,
  selectConditionalSpecialists,
  type ConditionalSpecialist,
} from "./specialist-gating.js";
import {
  runTrackedWorkflow,
  updateTrackedWorkflow,
} from "./runtime-state.js";
import {
  classifyIssue,
  explicitClassification,
  parseIssueMetadata,
  scopeFilesFromIssue,
} from "./issue-classifier.js";
import {
  confirmAnotherReviewBatch,
  confirmPostApprovedReview,
  confirmStartPrReview,
  formatReviewVerdict,
  postReviewComment,
  readJsonArtifactWithRetry,
  combineAuditVerdicts,
  readAuditVerdict,
  readReviewVerdict,
  REVIEW_BATCH_SIZE,
} from "./review-loop.js";
import { type AuditVerdict, type ReviewVerdict } from "./review-loop.js";
import {
  assertModeOverrideCompatible,
  describeVerificationTasks,
  listVerificationCommands,
  detectRepo,
  FULL_WORKFLOW_STEPS,
  planVerification,
  unresolvedProjects,
  getProjectProfilePath,
  getWorkflowStatePath,
  loadWorkflowState,
  LEAN_WORKFLOW_STEPS,
  parseCliArgs,
  requireCleanWorktree,
  resolveAgentBackend,
  runCommand,
  saveWorkflowState,
  slugifyIssueTitle,
  synchronizeDefaultBranch,
  verifyWorktree,
  warnIfWindowsPathLimitLikely,
  type CommandRunner,
  type WorkflowState,
  type WorkflowStep,
} from "./workflow-utils.js";

// The path to ignore may be a single artifact — a planner's handoff — or a
// directory holding one file per reviewer. Matching it exactly was enough for
// the first and silently wrong for the second: every verdict written inside the
// directory read as a file the stage should not have touched, and the fan-out
// accused each reviewer of editing the implementation.
function isIgnoredPath(file: string, ignoredRelativePath: string): boolean {
  return (
    file === ignoredRelativePath ||
    file.startsWith(`${ignoredRelativePath}/`)
  );
}

export function captureWorktreeState(
  targetDir: string,
  ignoredRelativePath: string,
): string {
  const trackedDiff = runCommand(
    "git",
    [
      "diff",
      "--binary",
      "--src-prefix=a/",
      "--dst-prefix=b/",
      "HEAD",
      "--",
      ".",
      `:(exclude)${ignoredRelativePath}`,
    ],
    { cwd: targetDir },
  );
  const untracked = runCommand(
    "git",
    ["ls-files", "--others", "--exclude-standard", "-z"],
    { cwd: targetDir },
  )
    .split("\0")
    .filter((file) => file && !isIgnoredPath(file, ignoredRelativePath))
    .map((file) => {
      const absolutePath = path.join(targetDir, file);
      const content = fs.lstatSync(absolutePath).isSymbolicLink()
        ? fs.readlinkSync(absolutePath)
        : fs.readFileSync(absolutePath);
      return [file, createHash("sha256").update(content).digest("hex")];
    })
    .sort(([left], [right]) => left.localeCompare(right));
  return JSON.stringify({ trackedDiff, untracked });
}

// An accusation with no evidence is unactionable: "the reviewer modified the
// implementation" leaves whoever reads it with no idea what moved, and the
// worktree is preserved but unexplained. Name the paths.
export function describeWorktreeChange(before: string, after: string): string {
  let a: { trackedDiff: string; untracked: [string, string][] };
  let b: typeof a;
  try {
    a = JSON.parse(before);
    b = JSON.parse(after);
  } catch {
    return "the worktree changed, but the captured state could not be compared";
  }

  const notes: string[] = [];
  const names = (state: typeof a) => new Map(state.untracked);
  const beforeFiles = names(a);
  const afterFiles = names(b);

  const added = [...afterFiles.keys()].filter((file) => !beforeFiles.has(file));
  const removed = [...beforeFiles.keys()].filter((file) => !afterFiles.has(file));
  const rewritten = [...afterFiles.keys()].filter(
    (file) => beforeFiles.has(file) && beforeFiles.get(file) !== afterFiles.get(file),
  );

  const list = (label: string, files: string[]) => {
    if (files.length === 0) return;
    const shown = files.slice(0, 10).join(", ");
    notes.push(
      `${label}: ${shown}${files.length > 10 ? ` and ${files.length - 10} more` : ""}`,
    );
  };
  list("new untracked files", added);
  list("removed untracked files", removed);
  list("rewritten untracked files", rewritten);

  if (a.trackedDiff !== b.trackedDiff) {
    // The diff itself is large and binary-safe; its changed paths are enough.
    const changed = [
      ...new Set(
        [...b.trackedDiff.matchAll(/^\+\+\+ b\/(.+)$/gm)].map((match) => match[1]),
      ),
    ];
    list("tracked files", changed.length > 0 ? changed : ["(see git diff)"]);
  }

  return notes.length > 0 ? notes.join("; ") : "no difference could be identified";
}

// GitHub closes an issue when a merged pull request's description says so in as
// many words. A bare "#1172" links the two and closes nothing, which is what the
// reports carried: every issue had to be closed by hand after its own change
// shipped.
//
// The keyword is written here rather than asked of the reporter, because a
// linkage that matters should not depend on an agent remembering a phrase.
const CLOSING_REFERENCE = /\b(clos(e|es|ed)|fix(e[sd])?|resolv(e|es|ed))\s+#(\d+)\b/gi;

export function ensureIssueClosingReference(
  body: string,
  issueNum: string,
): string {
  for (const match of body.matchAll(CLOSING_REFERENCE)) {
    if (match[5] === issueNum) {
      return body; // the reporter already said it; do not say it twice
    }
  }
  const trimmed = body.replace(/\s+$/, "");
  return `${trimmed}\n\nCloses #${issueNum}\n`;
}

export function implementationSessionId(repo: string, issueNum: string): string {
  const bytes = createHash("sha256")
    .update(`agent-flow:${repo}:${issueNum}:implementation`)
    .digest()
    .subarray(0, 16);
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = bytes.toString("hex");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

// The reporter has read the whole diff, so it is the one stage that can say
// what the change did rather than what the issue asked for. Its title is only
// used when it is a single usable line; anything else falls back to the issue
// title, which is at least accurate.
const MAX_PR_TITLE_LENGTH = 100;

export function readPullRequestTitle(
  titlePath: string,
  fallback: string,
): string {
  let raw: string;
  try {
    raw = fs.readFileSync(titlePath, "utf8");
  } catch {
    return fallback;
  }
  const title = raw
    .split("\n")
    .map((line) => line.trim())
    .find((line) => line.length > 0)
    // A model asked for "one line, no quotes" occasionally supplies quotes,
    // a Markdown heading, or a bullet anyway.
    ?.replace(/^#+\s*/, "")
    .replace(/^[-*]\s*/, "")
    .replace(/^["'`]|["'`]$/g, "")
    .trim();

  if (!title || title.length > MAX_PR_TITLE_LENGTH) {
    return fallback;
  }
  // A title that just restates the task tells a changelog reader nothing.
  if (/^(resolve|implement|fix)\s+(issue\s*)?#?\d+$/i.test(title)) {
    return fallback;
  }
  return title;
}

// The delivery commit is made before the reporter exists, so it carries a
// placeholder subject. Release tooling builds its changelog from commit
// subjects, which is how a release ends up listing "resolve #1086" instead of
// what shipped. Rewriting the subject is only safe while the commit has never
// left this machine, so anything that cannot prove that leaves it alone.
const PLACEHOLDER_SUBJECT = /^(fix|feat|chore)(\([^)]*\))?: resolve #\d+$/i;

export function retitleDeliveryCommit(
  targetDir: string,
  branchName: string,
  title: string,
  runner: CommandRunner = runCommand,
): "retitled" | "skipped" {
  try {
    const onRemote = runner(
      "git",
      ["ls-remote", "--heads", "origin", branchName],
      { cwd: targetDir },
    ).trim();
    if (onRemote !== "") {
      return "skipped"; // published already; rewriting would diverge
    }
  } catch {
    return "skipped"; // cannot prove it is unpushed, so do not touch it
  }

  let subject: string;
  let body: string;
  try {
    subject = runner("git", ["log", "-1", "--format=%s"], {
      cwd: targetDir,
    }).trim();
    body = runner("git", ["log", "-1", "--format=%b"], { cwd: targetDir }).trim();
  } catch {
    return "skipped";
  }

  // Only the placeholder is replaced. A subject someone wrote deliberately,
  // or one already retitled by an earlier run, is left as it is.
  if (!PLACEHOLDER_SUBJECT.test(subject) || subject === title) {
    return "skipped";
  }

  const args = ["commit", "--amend", "-m", title];
  if (body) {
    args.push("-m", body); // keep the trailers the delivery commit carried
  }
  runner("git", args, { cwd: targetDir });
  return "retitled";
}

export function runValidationWithRepair(
  validate: () => void,
  repair: (failure: string) => void,
): void {
  try {
    validate();
    return;
  } catch (error) {
    const failure = error instanceof Error ? error.message : String(error);
    console.log(
      "\x1b[33m[Validation Repair]\x1b[0m Returning the failure to the implementation specialist.",
    );
    repair(failure);
  }

  try {
    validate();
  } catch (error) {
    const failure = error instanceof Error ? error.message : String(error);
    throw new Error(
      `Validation still fails after one implementation repair attempt: ${failure}`,
    );
  }
}

export function createLeanReport(
  repo: string,
  issueNum: string,
  issueTitle: string,
  baseCommit: string,
  stack: TechStack,
  targetDir: string,
  evidence: LeanReportEvidence,
  profile: RepoProfile | null = null,
): string {
  const changedFiles = runCommand(
    "git",
    ["diff", "--name-status", baseCommit, "HEAD"],
    { cwd: targetDir },
  );
  const verification = listVerificationCommands(
    planVerification(detectStackProjects(targetDir), [], profile),
  );
  const head = runCommand("git", ["rev-parse", "--short", "HEAD"], {
    cwd: targetDir,
  });
  const files = changedFiles
    ? changedFiles.split("\n").map((line) => `- \`${line.replace(/\t/g, "  ")}\``)
    : ["- No changed files detected."];
  const bullets = (items: string[]) =>
    items.map((item) => `- ${item.replace(/\s+/g, " ").trim()}`).join("\n");
  const tableCell = (value: string) =>
    value.replace(/\s+/g, " ").trim().replace(/\|/g, "\\|");
  const criteria = evidence.acceptanceCriteria
    .map(
      ({ criterion, evidence: criterionEvidence, status }) =>
        `| ${tableCell(criterion)} | ${tableCell(criterionEvidence)} | ${status} |`,
    )
    .join("\n");
  return `# 🚀 Implementation Report
> **Issue:** ${repo}#${issueNum} — ${issueTitle}

## 🧭 Overview
- ${evidence.summary.replace(/\s+/g, " ").trim()}
- Orchestrator commit: \`${head}\`.

## ✅ What Changed
${bullets(evidence.changes)}

## 🏗️ Architecture and Data Flow
${bullets(evidence.architecture)}

## 📂 Files Changed
${files.join("\n")}

## 🧪 Validation
${bullets(evidence.validation)}
- Orchestrator delivery gate passed:
${verification}
- Orchestrator delivery gate passed: \`git diff --check\`

## 🎯 Acceptance Criteria
| Criterion | Evidence | Status |
| --- | --- | --- |
${criteria}

## 🔄 Compatibility
${bullets(evidence.compatibility)}

## ⚠️ Known Limitations and Risks
${bullets(evidence.risks)}

## 👀 Reviewer Checklist
${bullets(evidence.reviewedAreas)}
`;
}

type AcceptanceCriterionEvidence = {
  criterion: string;
  evidence: string;
  status: "✅" | "⚠️" | "❌";
};

export type LeanReportEvidence = {
  status: "complete";
  verdict: "approved" | "blockers";
  summary: string;
  changes: string[];
  architecture: string[];
  validation: string[];
  acceptanceCriteria: AcceptanceCriterionEvidence[];
  compatibility: string[];
  risks: string[];
  reviewedAreas: string[];
};

function isNonEmptyStrings(value: unknown): value is string[] {
  return (
    Array.isArray(value) &&
    value.length > 0 &&
    value.every((item) => typeof item === "string" && item.trim().length > 0)
  );
}

export function readLeanReportEvidence(evidencePath: string): LeanReportEvidence {
  let value: unknown;
  try {
    value = JSON.parse(fs.readFileSync(evidencePath, "utf8"));
  } catch {
    throw new Error("Lean verifier/reviewer did not produce valid completion JSON.");
  }
  if (!value || typeof value !== "object") {
    throw new Error("Lean verifier/reviewer completion signal is incomplete.");
  }
  const rawEvidence = value as Record<string, unknown>;
  const evidence = value as Partial<LeanReportEvidence>;
  const validCriteria =
    Array.isArray(evidence.acceptanceCriteria) &&
    evidence.acceptanceCriteria.length > 0 &&
    evidence.acceptanceCriteria.every(
      (criterion) =>
        criterion &&
        typeof criterion === "object" &&
        typeof criterion.criterion === "string" &&
        criterion.criterion.trim().length > 0 &&
        typeof criterion.evidence === "string" &&
        criterion.evidence.trim().length > 0 &&
        ["✅", "⚠️", "❌"].includes(criterion.status),
    );
  const validation =
    typeof rawEvidence.validation === "string" && rawEvidence.validation.trim()
      ? [rawEvidence.validation]
      : rawEvidence.validation;
  const inferredVerdict = validCriteria
    ? evidence.acceptanceCriteria?.every((criterion) => criterion.status === "✅")
      ? "approved"
      : "blockers"
    : undefined;
  const verdict = evidence.verdict ?? inferredVerdict;
  const verdictMatchesCriteria =
    verdict === "approved"
      ? inferredVerdict === "approved"
      : verdict === "blockers"
        ? inferredVerdict === "blockers"
        : false;
  if (
    evidence.status !== "complete" ||
    !["approved", "blockers"].includes(verdict ?? "") ||
    !verdictMatchesCriteria ||
    typeof evidence.summary !== "string" ||
    !evidence.summary.trim() ||
    !isNonEmptyStrings(evidence.changes) ||
    !isNonEmptyStrings(evidence.architecture) ||
    !isNonEmptyStrings(validation) ||
    !validCriteria ||
    !isNonEmptyStrings(evidence.compatibility) ||
    !isNonEmptyStrings(evidence.risks) ||
    !isNonEmptyStrings(evidence.reviewedAreas)
  ) {
    throw new Error("Lean verifier/reviewer completion signal is incomplete.");
  }
  return {
    ...(evidence as LeanReportEvidence),
    verdict: verdict as "approved" | "blockers",
    validation,
  };
}

export async function main(args: string[] = process.argv.slice(2)): Promise<void> {
  const { requestedRepo, issueNum, modeOverride, refreshProfile } =
    parseCliArgs(args);
  if (process.env.HERDR_ENV !== "1") {
    throw new Error("agent-flow must be run from a Herdr-managed pane.");
  }

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
  console.log(`\x1b[36m[Git]\x1b[0m Synchronizing the default branch...`);
  const defaultBranch = synchronizeDefaultBranch(repoRoot);
  console.log(`\x1b[36m[Git]\x1b[0m ${defaultBranch} is up to date.`);
  const statePath = getWorkflowStatePath(repoRoot, issueNum);
  let state = loadWorkflowState(statePath);
  if (state && (state.repo !== repo || state.issueNum !== issueNum)) {
    throw new Error(`Workflow checkpoint does not match ${repo}#${issueNum}: ${statePath}`);
  }
  let modeReport: string;
  if (state) assertModeOverrideCompatible(state, modeOverride);
  // Read on every run, not only when starting one: a resumed workflow still has
  // to know which part of the repository the issue is about, in order to pick
  // the specialists its remaining stages run in.
  const issueMetadata = parseIssueMetadata(
    runCommand("gh", [
      "issue",
      "view",
      issueNum,
      "--repo",
      repo,
      "--json",
      "title,body,labels",
    ]),
  );
  if (!state) {
    const classification = modeOverride
      ? explicitClassification(modeOverride)
      : classifyIssue(issueMetadata);
    const issueSlug = slugifyIssueTitle(issueMetadata.title);
    state = {
      version: 7,
      repo,
      issueNum,
      issueTitle: issueMetadata.title,
      branchName: `agent/${issueSlug}-${issueNum}`,
      baseCommit: runCommand("git", ["rev-parse", "HEAD"], { cwd: repoRoot }),
      completedSteps: [],
      reviewCyclesCompleted: 0,
      approved: false,
      mode: classification.mode,
      modeReason: classification.reason,
      modeSource: classification.source,
    };
    modeReport = `${classification.mode.toUpperCase()} — ${classification.reason}`;
  } else if (modeOverride) {
    modeReport =
      `${state.mode.toUpperCase()} — explicit --${modeOverride} override matches ` +
      `the persisted selection (${state.modeReason})`;
  } else {
    modeReport = `${state.mode.toUpperCase()} — persisted selection: ${state.modeReason}`;
  }
  saveWorkflowState(statePath, state);
  const issueTitle = state.issueTitle;
  const branchName = state.branchName;
  // Posix separators, deliberately, and not path.join. Every artifact path below
  // is built by string concatenation and then compared against paths git prints,
  // which are always forward-slashed. path.join produced ".agent\issue-N" on
  // Windows and the concatenation made ".agent\issue-N/LEAN_PLAN.md", which
  // matched nothing git reported: the lean planner's own handoff was counted as a
  // file it should not have touched, and the stage failed every time. Windows
  // accepts forward slashes in filesystem calls, so path.join(targetDir, agentDir)
  // still works.
  const agentDir = `.agent/issue-${issueNum}`;

  console.log(`\x1b[32m[Target Repository]\x1b[0m ${repo} | Issue #${issueNum}`);
  console.log(`\x1b[34m[Workflow Mode]\x1b[0m ${modeReport}`);
  console.log(`\x1b[36m[Sandcastle]\x1b[0m Creating isolated worktree...`);
  warnIfWindowsPathLimitLikely(repoRoot);
  const worktree = await createWorktree({
    branchStrategy: { type: "branch", branch: branchName },
    cwd: repoRoot,
  });
  const targetDir = worktree.worktreePath;
  const workflowSteps =
    state.mode === "lean" ? LEAN_WORKFLOW_STEPS : FULL_WORKFLOW_STEPS;
  const runtimeSteps = workflowSteps.map((step) => ({
    id: step,
    title: step.replaceAll("-", " "),
    status: state.completedSteps.includes(step) ? "succeeded" : "queued",
  }));
  updateTrackedWorkflow({
    title: `Implement #${issueNum}: ${issueTitle}`,
    repo: repoRoot,
    worktree_path: targetDir,
    branch: branchName,
    current_step: state.completedSteps.at(-1) ?? "Preparing workflow",
    progress: {
      completed: state.completedSteps.length,
      total: workflowSteps.length,
      percent: Math.round(
        (state.completedSteps.length / workflowSteps.length) * 100,
      ),
    },
    steps: runtimeSteps,
  });
  let specialistPaneId: string | null = null;
  let workflowSucceeded = false;
  let workflowPaused = false;
  let prUrl = state.prUrl ?? "";

  try {
    const checkedOutBranch = runCommand(
      "git",
      ["branch", "--show-current"],
      { cwd: targetDir },
    );
    if (worktree.branch !== branchName || checkedOutBranch !== branchName) {
      throw new Error(
        `Expected worktree branch ${branchName}, but Sandcastle created ${worktree.branch} with ${checkedOutBranch || "detached HEAD"} checked out.`,
      );
    }

    const stack: TechStack = detectStack(targetDir);
    // Detection runs on every workflow: it is a filesystem walk, it costs
    // nothing next to an agent turn, and it is what decides whether the
    // repository's specialists are still the right ones.
    const projects = detectStackProjects(targetDir);
    const surfaces = detectSurfaces(targetDir, projects);
    const profilePath = getProjectProfilePath(repoRoot);
    const profileDraftPath = `${agentDir}/project-profile.json`;
    const requirementsPath = `${agentDir}/REQUIREMENTS.md`;
    const contextPath = `${agentDir}/CONTEXT.md`;
    const planPath = `${agentDir}/PLAN.md`;
    const leanPlanPath = `${agentDir}/LEAN_PLAN.md`;
    const leanCompletionPath = path.join(
      path.dirname(statePath),
      `issue-${issueNum}-lean-evidence.json`,
    );
    const reportPath = path.join(
      path.dirname(statePath),
      `issue-${issueNum}-implementation-report.md`,
    );
    const prTitlePath = path.join(
      path.dirname(statePath),
      `issue-${issueNum}-pr-title.txt`,
    );
    const reviewReportPath = path.join(
      path.dirname(statePath),
      `issue-${issueNum}-review-verdict.md`,
    );
    const verdictPath = path.join(targetDir, ".agent-review-verdict.json");
    const implementerSessionId = implementationSessionId(repo, issueNum);
    console.log(`\x1b[36m[Sandcastle]\x1b[0m ${targetDir} on ${branchName}`);
    console.log(`\x1b[36m[Stack Detector]\x1b[0m Assigned specialist: ${stack}`);
    if (state.completedSteps.length > 0) {
      console.log(
        `\x1b[33m[Resume]\x1b[0m Continuing after ${state.completedSteps.at(-1)}.`,
      );
    }

    // Mission control reads the workflow's own status and its agents', so a
    // stage waiting for a person has to be reported as blocked on both for the
    // dashboard to surface it instead of showing a long-running stage.
    const reportAgent = (
      role: string,
      status: "working" | "blocked",
      summary: string,
    ): void => {
      updateTrackedWorkflow({
        status: status === "blocked" ? "blocked" : "running",
        agents: specialistPaneId
          ? [
              {
                id: `${process.env.GROVE_WORKFLOW_RUN_ID ?? "workflow"}:${role}`,
                kind: resolveAgentBackend(),
                name: role,
                status,
                summary,
                pane_id: specialistPaneId,
              },
            ]
          : [],
      });
    };

    const runSpecialist = (
      role: string,
      promptText: string,
      completionArtifacts: string[] = [],
      completionValidator?: () => void,
      sessionId?: string,
    ): void => {
      runSpecialistInPane({
        role,
        promptText,
        targetDir,
        issueOrPrNumber: issueNum,
        sessionId,
        completionArtifacts,
        completionValidator,
        onPaneChanged: (paneId) => {
          specialistPaneId = paneId;
          reportAgent(role, "working", `Executing ${role} stage`);
        },
        // A stage waiting on a person should read as waiting, not as slow.
        onAgentStatus: (status, summary) => reportAgent(role, status, summary),
      });
    };

    const runStep = (
      step: WorkflowStep,
      action: () => void,
    ): void => {
      if (state.completedSteps.includes(step)) {
        console.log(`\x1b[33m[Resume]\x1b[0m Skipping completed step: ${step}`);
        return;
      }
      const trackedStep = runtimeSteps.find((candidate) => candidate.id === step);
      const startedAt = new Date().toISOString();
      if (trackedStep) {
        Object.assign(trackedStep, { status: "running", started_at: startedAt });
      }
      updateTrackedWorkflow({ current_step: step, steps: runtimeSteps });
      try {
        action();
        state.completedSteps.push(step);
        saveWorkflowState(statePath, state);
        if (trackedStep) {
          const completedAt = new Date().toISOString();
          Object.assign(trackedStep, {
            status: "succeeded",
            completed_at: completedAt,
            duration_ms: Date.parse(completedAt) - Date.parse(startedAt),
          });
        }
        updateTrackedWorkflow({
          current_step: step,
          steps: runtimeSteps,
          progress: {
            completed: state.completedSteps.length,
            total: workflowSteps.length,
            percent: Math.round(
              (state.completedSteps.length / workflowSteps.length) * 100,
            ),
          },
        });
      } catch (error) {
        if (trackedStep) {
          const completedAt = new Date().toISOString();
          Object.assign(trackedStep, {
            status: "failed",
            completed_at: completedAt,
            duration_ms: Date.parse(completedAt) - Date.parse(startedAt),
          });
        }
        updateTrackedWorkflow({ current_step: step, steps: runtimeSteps });
        throw error;
      }
    };

    const verifyWithRepair = (handoffPath: string): void => {
      // Name every command the gate runs, so a repair in a multi-stack
      // repository knows which project it has to make pass.
      const verification = describeVerificationTasks(
        planVerification(detectStackProjects(targetDir), [], profile),
      );
      runValidationWithRepair(
        () => verifyWorktree(targetDir, undefined, profile),
        (failure) =>
          runSpecialist(
            `${stack.toLowerCase()}-validation-repair`,
            SPECIALISTS.IMPLEMENTER_VALIDATION_FIXES(
              persona,
              issueNum,
              repo,
              handoffPath,
              verification,
              failure,
            ),
            [],
            undefined,
            implementerSessionId,
          ),
      );
    };

    // Review specialists whose subject matter appears in only some diffs.
    // Selecting them from the changed files keeps an ordinary issue at the cost
    // it has today, while a migration or an authorization change still gets the
    // dedicated pass it warrants. The selection is computed once, after the
    // implementation has settled.
    let conditionalSelection: ConditionalSpecialist[] | null = null;
    const selectedSpecialists = (): ConditionalSpecialist[] => {
      if (conditionalSelection === null) {
        conditionalSelection = selectConditionalSpecialists({
          changedFiles: collectChangedFiles(targetDir),
          hasDocumentationSurface: hasDocumentationSurface(targetDir),
        });
        console.log(
          conditionalSelection.length > 0
            ? `\x1b[36m[Specialists]\x1b[0m Diff selects: ${conditionalSelection.join(", ")}`
            : "\x1b[36m[Specialists]\x1b[0m No conditional specialists apply to this diff.",
        );
      }
      return conditionalSelection;
    };

    // One review covering whichever concerns the diff raises. Three separate
    // stages meant three agent runs, three pane boots and three re-reads of
    // the same diff for what is one pass over one change.
    const runDomainReview = (handoffPath: string): void => {
      runStep("domain-review", () => {
        const builtinDomains = selectedSpecialists().filter(
          (specialist) => specialist !== "documentation",
        );
        // One specialist per concern, ranked by how much of the diff each one
        // actually matches. A generated concern either extends a built-in
        // checklist or stands as a review of its own, so a repository whose
        // vocabulary the built-in patterns never matched still gets the review
        // it needs.
        const plan = planReviewPasses({
          changedFiles: collectChangedFiles(targetDir),
          builtins: builtinDomains,
          builtinSections: builtinReviewSections(),
          concerns: profile?.concerns ?? [],
          maxPasses: resolveMaxReviewPasses(),
        });
        if (plan.passes.length === 0) {
          console.log(
            "\x1b[33m[Specialists]\x1b[0m Skipping domain-review: the diff raises none of its concerns.",
          );
          return;
        }
        console.log(
          `\x1b[36m[Specialists]\x1b[0m Domain review, one specialist each: ${plan.passes
            .map((pass) => `${pass.title} (${pass.matchCount})`)
            .join(", ")}`,
        );
        if (plan.skipped.length > 0) {
          console.log(
            `\x1b[33m[Specialists]\x1b[0m Not reviewed, the pass cap was reached: ${plan.skipped
              .map((pass) => `${pass.title} (${pass.matchCount})`)
              .join(", ")}`,
          );
        }
        // Keep the verdicts inside the worktree: Claude needs approval to write
        // outside its working directory, and .agent/ is removed at delivery.
        const reviewDirRelative = `${agentDir}/domain-review`;
        const verdictPath = path.join(
          targetDir,
          reviewDirRelative,
          "combined.json",
        );
        fs.mkdirSync(path.join(targetDir, reviewDirRelative), {
          recursive: true,
        });

        let due = plan.passes;
        for (let cycle = 1; cycle <= REVIEW_BATCH_SIZE; cycle += 1) {
          const results: {
            id: string;
            title: string;
            verdict: AuditVerdict;
          }[] = [];
          for (const pass of due) {
            const passPath = path.join(
              targetDir,
              reviewDirRelative,
              `${pass.slug}.json`,
            );
            fs.rmSync(passPath, { force: true });
            // The whole review directory is excluded, so one reviewer's verdict
            // is never mistaken for another reviewer editing the implementation.
            const stateBefore = captureWorktreeState(
              targetDir,
              reviewDirRelative,
            );
            runSpecialist(
              `${pass.slug}-${cycle}`,
              SPECIALISTS.DOMAIN_REVIEWER(
                diffPersonaOf("review"),
                issueNum,
                repo,
                passPath,
                pass,
              ),
              [passPath],
              () => {
                readAuditVerdict(passPath, pass.title);
              },
            );
            const stateAfter = captureWorktreeState(targetDir, reviewDirRelative);
            if (stateAfter !== stateBefore) {
              throw new Error(
                `The ${pass.title} reviewer modified the implementation instead of reporting blockers — ` +
                  `${describeWorktreeChange(stateBefore, stateAfter)}.`,
              );
            }
            results.push({
              id: pass.id,
              title: pass.title,
              verdict: readAuditVerdict(passPath, pass.title),
            });
          }

          const combined = combineAuditVerdicts(results, plan.skipped);
          if (combined.verdict === "approved") {
            return;
          }
          // One fix agent per cycle rather than one per reviewer. The
          // implementation specialist holds a single session, and several
          // sequential fix turns would multiply the cost and let two fixes
          // conflict over the same file.
          fs.writeFileSync(
            verdictPath,
            `${JSON.stringify(combined, null, 2)}\n`,
          );
          const blocking = results.filter(
            (result) => result.verdict.verdict === "blockers",
          );
          console.log(
            `\x1b[33m[domain-review]\x1b[0m Blockers from ${blocking
              .map((result) => result.title)
              .join(", ")} in cycle ${cycle}; returning control to the implementation specialist.`,
          );
          // Only the reviewers that blocked are asked again. Re-running a
          // specialist that already approved costs an agent run to be told the
          // same thing twice, and is what would make the cap expensive.
          const blocked = new Set(blocking.map((result) => result.id));
          due = plan.passes.filter((pass) => blocked.has(pass.id));
          runSpecialist(
            `${stack.toLowerCase()}-domain-review-fix-${cycle}`,
            SPECIALISTS.IMPLEMENTER_REVIEW_FIXES(
              persona,
              issueNum,
              repo,
              handoffPath,
              verdictPath,
            ),
            [],
            undefined,
            implementerSessionId,
          );
          verifyWithRepair(handoffPath);
        }
        throw new Error(
          `The domain reviewer still reports blockers after ${REVIEW_BATCH_SIZE} implementation cycles.`,
        );
      });
    };

    const runDocumentationStage = (handoffPath: string): void => {
      runStep("documentation", () => {
        if (!selectedSpecialists().includes("documentation")) {
          console.log(
            "\x1b[33m[Specialists]\x1b[0m Skipping documentation: the diff changes nothing this repository documents.",
          );
          return;
        }
        runSpecialist(
          "documentation-specialist",
          SPECIALISTS.DOCUMENTATION_SPECIALIST(
            diffPersonaOf("documentation"),
            issueNum,
            repo,
            issueTitle,
          ),
        );
        verifyWithRepair(handoffPath);
      });
    };

    const removeAgentArtifacts = (): void => {
      fs.rmSync(path.join(targetDir, agentDir), { recursive: true, force: true });
      const parentAgentDir = path.join(targetDir, ".agent");
      if (
        fs.existsSync(parentAgentDir) &&
        fs.readdirSync(parentAgentDir).length === 0
      ) {
        fs.rmdirSync(parentAgentDir);
      }
    };

    const commitChanges = (subject: string): void => {
      const status = runCommand("git", ["status", "--porcelain"], { cwd: targetDir });
      if (!status) {
        throw new Error("The workflow produced no changes to commit.");
      }
      verifyWorktree(targetDir, undefined, profile);
      runCommand("git", ["add", "-A"], { cwd: targetDir });
      runCommand("git", ["diff", "--cached", "--check"], { cwd: targetDir });
      runCommand(
        "git",
        [
          "commit",
          "-m",
          subject,
          "-m",
          "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>",
        ],
        { cwd: targetDir },
      );
      requireCleanWorktree(targetDir);
    };

    // The specialists this repository gets are written for it, once per shape of
    // the repository rather than once per workflow. This runs before the first
    // stage and outside runStep: it produces no workflow step, appears in no
    // checkpoint, and after it succeeds the profile is fresh, so the next run
    // does not repeat it.
    const loaded = loadProfile(profilePath, targetDir, projects, surfaces);
    let profile = loaded.profile;
    {
      if (projects.length === 0) {
        // Nothing to profile yet: a repository with no manifest has no project
        // for the prompt engineer to describe, and the validator refuses a
        // profile that lists none. Generating here would fail, retry, and fail
        // again. The fingerprint moves the moment a manifest appears, and the
        // profile is written then.
        console.log(
          "\x1b[33m[Prompt Engineer]\x1b[0m No project detected yet, so there is nothing to write specialists for. This runs once the repository has a manifest.",
        );
      } else if (refreshProfile || loaded.status !== "fresh") {
        const why = refreshProfile
          ? "a refresh was requested"
          : loaded.reason;
        console.log(
          `\x1b[36m[Prompt Engineer]\x1b[0m Writing this repository's specialists: ${why}.`,
        );
        fs.mkdirSync(path.join(targetDir, agentDir), { recursive: true });
        const absoluteDraftPath = path.join(targetDir, profileDraftPath);
        runSpecialist(
          "prompt-engineer",
          SPECIALISTS.PROMPT_ENGINEER(
            repo,
            profileDraftPath,
            projects.map((project) => ({
              root: project.root,
              marker: project.marker,
              label: stackLabel(project),
              // Facts, read from the manifest. The prompt engineer decides what
              // they mean; it does not decide what they are.
              dependencies: readDirectDependencies(
                path.join(targetDir, project.marker),
              ),
            })),
            surfaces,
          ),
          [absoluteDraftPath],
          () => {
            readProfileDraft(absoluteDraftPath, projects);
          },
        );
        profile = writeProfile(
          profilePath,
          readProfileDraft(absoluteDraftPath, projects),
          fingerprintRepoShape(targetDir, projects, surfaces),
        );
        fs.rmSync(absoluteDraftPath, { force: true });
        // The profile decides what this workflow will execute, so what it chose
        // is stated plainly at the moment it is adopted rather than left to be
        // discovered in a log.
        for (const project of profile.projects) {
          const where = project.root || "the repository root";
          console.log(
            `\x1b[36m[Prompt Engineer]\x1b[0m ${where} (${project.label}): ${project.test.command} ${project.test.args.join(" ")}` +
              (project.test.verified ? "" : " [unverified]"),
          );
          if (project.setup) {
            console.log(
              `\x1b[36m[Prompt Engineer]\x1b[0m ${where} setup: ${project.setup.command} ${project.setup.args.join(" ")}`,
            );
          }
          if (project.frameworks.length > 0) {
            console.log(
              `\x1b[36m[Prompt Engineer]\x1b[0m ${where} frameworks: ${project.frameworks
                .map((framework) => framework.name)
                .join(", ")}`,
            );
          } else {
            // Frameworks are the structured field later stages can act on, and
            // a profile that puts them in prose instead is silently poorer. A
            // project that declares dependencies but names none is worth saying
            // out loud rather than discovering by reading the file.
            const declared = readDirectDependencies(
              path.join(targetDir, project.marker),
            );
            if (declared.length > 0) {
              console.log(
                `\x1b[33m[Prompt Engineer]\x1b[0m ${where} names no frameworks, though it declares ${declared.length} dependencies. Rerun with --refresh-profile to have them identified.`,
              );
            }
          }
        }
      }
    }

    // A project nothing can validate is refused here, before any agent runs: the
    // repair loop cannot fix a missing test command, and would spend three
    // attempts discovering that.
    const unresolved = unresolvedProjects(projects, profile, targetDir);
    if (unresolved.length > 0) {
      const names = unresolved
        .map((project) => `${project.root || "the repository root"} (${project.marker})`)
        .join(", ");
      throw new Error(
        `No test command is known for ${names}.\n` +
          `Run 'imp ${issueNum} --refresh-profile' to have the prompt engineer establish one, ` +
          `or add it to ${profilePath}.`,
      );
    }

    // What a step will touch decides which specialist it gets. Before any diff
    // exists that comes from the issue; afterwards, from the diff itself, so a
    // change confined to one project is reviewed by that project's specialist.
    const issueScope = scopeFilesFromIssue(issueMetadata, projects);
    const personaOf = (role: RoleKey, scopeFiles?: string[]): string =>
      personaFor(role, projects, profile, scopeFiles ?? issueScope);
    const diffPersonaOf = (role: RoleKey): string =>
      personaFor(role, projects, profile, collectChangedFiles(targetDir));
    const persona = personaOf("implementation");

    if (state.mode === "full") {
      runStep("planning", () =>
        runSpecialist(
          "planner",
          SPECIALISTS.PLANNER(
            personaOf("planning"),
            issueNum,
            repo,
            requirementsPath,
            contextPath,
            planPath,
          ),
          [
            path.join(targetDir, requirementsPath),
            path.join(targetDir, contextPath),
            path.join(targetDir, planPath),
          ],
        ),
      );
      runStep("tests", () =>
        runSpecialist(
          "test-engineer",
          SPECIALISTS.TEST_ENGINEER(
            personaOf("tests"),
            issueNum,
            requirementsPath,
            contextPath,
            planPath,
          ),
        ),
      );
      runStep("implementation", () =>
        runSpecialist(
          `${stack.toLowerCase()}-implementer`,
          getImplementationPrompt(
            persona,
            issueNum,
            requirementsPath,
            contextPath,
            planPath,
          ),
          [],
          undefined,
          implementerSessionId,
        ),
      );
      runStep("verification", () => {
        runSpecialist(
          "verifier",
          SPECIALISTS.VERIFIER(
            diffPersonaOf("verification"),
            issueNum,
            requirementsPath,
            planPath,
          ),
        );
        verifyWithRepair(planPath);
      });
      runDomainReview(planPath);
      runDocumentationStage(planPath);
    } else {
      runStep("lean-planning", () => {
        const stateBefore = captureWorktreeState(targetDir, leanPlanPath);
        runSpecialist(
          "lean-planner",
          SPECIALISTS.LEAN_PLANNER(
            personaOf("planning"),
            issueNum,
            repo,
            leanPlanPath,
          ),
          [path.join(targetDir, leanPlanPath)],
        );
        const stateAfter = captureWorktreeState(targetDir, leanPlanPath);
        if (stateAfter !== stateBefore) {
          throw new Error(
            "Lean planner modified files outside its handoff artifact — " +
              `${describeWorktreeChange(stateBefore, stateAfter)}.`,
          );
        }
        const absoluteLeanPlanPath = path.join(targetDir, leanPlanPath);
        if (
          !fs.existsSync(absoluteLeanPlanPath) ||
          !fs.readFileSync(absoluteLeanPlanPath, "utf8").trim()
        ) {
          throw new Error("Lean planner did not produce a compact handoff.");
        }
      });
      runStep("lean-implementation", () =>
        runSpecialist(
          `${stack.toLowerCase()}-lean-implementer`,
          SPECIALISTS.LEAN_IMPLEMENTER(persona, issueNum, repo, leanPlanPath),
          [],
          undefined,
          implementerSessionId,
        ),
      );
      runStep("lean-review", async () => {
        for (let cycle = 1; cycle <= REVIEW_BATCH_SIZE; cycle += 1) {
          fs.rmSync(leanCompletionPath, { force: true });
          const reviewState = captureWorktreeState(targetDir, "__no_ignored_file__");
          runSpecialist(
            `lean-verifier-reviewer-${cycle}`,
            SPECIALISTS.LEAN_REVIEWER(
              diffPersonaOf("review"),
              issueNum,
              repo,
              leanPlanPath,
              leanCompletionPath,
            ),
            [leanCompletionPath],
            async () => {
              // Validate the JSON completion was written correctly
              try {
                const evidence = await readJsonArtifactWithRetry(leanCompletionPath);
                readLeanReportEvidence(leanCompletionPath);
              } catch (error) {
                throw new Error(
                  `Lean review verdict invalid after cycle ${cycle}. The agent must produce valid, complete JSON.`
                );
              }
            },
          );
          if (
            captureWorktreeState(targetDir, "__no_ignored_file__") !==
            reviewState
          ) {
            throw new Error(
              "Lean verifier/reviewer modified the implementation instead of reporting blockers.",
            );
          }
          const evidence = readLeanReportEvidence(leanCompletionPath);
          if (evidence.verdict === "approved") return;

          console.log(
            `\x1b[33m[Lean Review]\x1b[0m Blockers found in cycle ${cycle}; returning control to the implementation specialist.`,
          );
          runSpecialist(
            `${stack.toLowerCase()}-lean-implementer-fix-${cycle}`,
            SPECIALISTS.IMPLEMENTER_REVIEW_FIXES(
              persona,
              issueNum,
              repo,
              leanPlanPath,
              leanCompletionPath,
            ),
            [],
            undefined,
            implementerSessionId,
          );
        }
        throw new Error(
          `Lean review still has blockers after ${REVIEW_BATCH_SIZE} implementation cycles.`,
        );
      });
      runStep("verification", () => verifyWithRepair(leanPlanPath));
      runDomainReview(leanPlanPath);
      runDocumentationStage(leanPlanPath);
    }
    runStep("delivery", () => {
      // Planning artifacts are scaffolding for the agents, not deliverables.
      removeAgentArtifacts();
      const status = runCommand("git", ["status", "--porcelain"], { cwd: targetDir });
      const head = runCommand("git", ["rev-parse", "HEAD"], { cwd: targetDir });
      if (!status && head !== state.baseCommit) {
        console.log(
          "\x1b[33m[Resume]\x1b[0m Changes are already committed; skipping duplicate Git work.",
        );
      } else {
        commitChanges(`fix: resolve #${issueNum}`);
      }
      requireCleanWorktree(targetDir);
    });
    runStep("report", () => {
      fs.rmSync(reportPath, { force: true });
      fs.rmSync(prTitlePath, { force: true });
      if (state.mode === "full") {
        runSpecialist(
          "implementation-reporter",
          SPECIALISTS.REPORTER(
            repo,
            issueNum,
            issueTitle,
            state.baseCommit,
            reportPath,
            prTitlePath,
          ),
          [reportPath, prTitlePath],
        );
      } else {
        fs.writeFileSync(
          reportPath,
          createLeanReport(
            repo,
            issueNum,
            issueTitle,
            state.baseCommit,
            stack,
            targetDir,
            readLeanReportEvidence(leanCompletionPath),
          ),
          { mode: 0o600 },
        );
      }
      if (!fs.existsSync(reportPath) || !fs.readFileSync(reportPath, "utf8").trim()) {
        throw new Error("Implementation reporter did not produce a report.");
      }
      requireCleanWorktree(targetDir);
      console.log(`\n\x1b[36m[Implementation Report]\x1b[0m\n${fs.readFileSync(reportPath, "utf8")}`);
    });
    runStep("publish", () => {
      // The reporter names what the change did; the issue title only says what
      // was asked for, which reads as boilerplate in a changelog.
      const pullRequestTitle = readPullRequestTitle(prTitlePath, issueTitle);
      console.log(`[35m[GitHub][0m Title: ${pullRequestTitle}`);
      if (
        retitleDeliveryCommit(targetDir, branchName, pullRequestTitle) ===
        "retitled"
      ) {
        console.log(
          "[36m[Git][0m Retitled the delivery commit so the changelog reads usefully.",
        );
      }
      console.log(`\x1b[33m[Git]\x1b[0m Pushing branch ${branchName}...`);
      runCommand("git", ["push", "-u", "origin", branchName], { cwd: targetDir });
      console.log(`\x1b[35m[GitHub]\x1b[0m Opening pull request...`);
      prUrl = runCommand(
        "gh",
        [
          "pr",
          "list",
          "--repo",
          repo,
          "--head",
          branchName,
          "--state",
          "all",
          "--limit",
          "1",
          "--json",
          "url",
          "--jq",
          ".[0].url // empty",
        ],
        { cwd: targetDir },
      );
      // Written into the file both branches below publish from, so the pull
      // request carries it whether it is being opened or updated.
      fs.writeFileSync(
        reportPath,
        ensureIssueClosingReference(
          fs.readFileSync(reportPath, "utf8"),
          issueNum,
        ),
      );
      if (!prUrl) {
        prUrl = runCommand(
          "gh",
          [
            "pr",
            "create",
            "--repo",
            repo,
            "--head",
            branchName,
            "--title",
            pullRequestTitle,
            "--body-file",
            reportPath,
          ],
          { cwd: targetDir },
        );
      } else {
        runCommand(
          "gh",
          ["pr", "edit", prUrl, "--title", pullRequestTitle, "--body-file", reportPath],
          { cwd: targetDir },
        );
      }
      state.prUrl = prUrl;
    });

    if (!state.completedSteps.includes("review")) {
      if (!(await confirmStartPrReview(prUrl))) {
        workflowPaused = true;
        console.log(
          `\x1b[33m[Paused]\x1b[0m PR review deferred. Resume with agent-flow ${issueNum}.`,
        );
        return;
      }
      while (!state.approved) {
        if (
          state.reviewCyclesCompleted > 0 &&
          state.reviewCyclesCompleted % REVIEW_BATCH_SIZE === 0 &&
          !(await confirmAnotherReviewBatch())
        ) {
          throw new Error(
            `PR review stopped after ${state.reviewCyclesCompleted} cycles with blockers remaining.`,
          );
        }

        const cycle = state.reviewCyclesCompleted + 1;
        fs.rmSync(verdictPath, { force: true });
        const reviewState = captureWorktreeState(
          targetDir,
          path.relative(targetDir, verdictPath),
        );
        runSpecialist(
          `pr-review-${cycle}`,
          SPECIALISTS.PR_REVIEWER(
            diffPersonaOf("review"),
            repo,
            issueNum,
            prUrl,
            verdictPath,
            cycle,
          ),
          [verdictPath],
          async () => {
            await readJsonArtifactWithRetry(verdictPath);
          },
        );
        const verdict = await readJsonArtifactWithRetry(verdictPath);
        if (
          captureWorktreeState(
            targetDir,
            path.relative(targetDir, verdictPath),
          ) !== reviewState
        ) {
          throw new Error(
            "PR reviewer modified the implementation instead of reporting blockers.",
          );
        }
        state.reviewCyclesCompleted = cycle;
        state.finalVerdict = formatReviewVerdict(verdict, cycle);

        if (verdict.verdict === "approved") {
          fs.rmSync(verdictPath, { force: true });
          requireCleanWorktree(targetDir);
          state.approved = true;
          saveWorkflowState(statePath, state);
          console.log(`\n\x1b[32m[PR Review Approved]\x1b[0m\n${state.finalVerdict}`);
          break;
        }

        console.log(
          `\n\x1b[33m[PR Review Blockers]\x1b[0m Returning control to the original implementation specialist.\n${state.finalVerdict}`,
        );
        runSpecialist(
          `${stack.toLowerCase()}-implementer-fix-${cycle}`,
          SPECIALISTS.IMPLEMENTER_REVIEW_FIXES(
            persona,
            issueNum,
            repo,
            state.mode === "lean" ? leanPlanPath : planPath,
            verdictPath,
          ),
          [],
          undefined,
          implementerSessionId,
        );
        fs.rmSync(verdictPath, { force: true });
        commitChanges(`fix: address PR review cycle ${cycle} (#${issueNum})`);
        runCommand("git", ["push", "origin", branchName], { cwd: targetDir });
        saveWorkflowState(statePath, state);
        console.log(
          `\n\x1b[33m[PR Review Fixes Applied]\x1b[0m A fresh reviewer will verify cycle ${cycle + 1}.`,
        );
      }
      // Publishing the verdict completes the review; it is the same stage,
      // not a separate one.
      if (!state.approved || !state.finalVerdict) {
        throw new Error("Cannot publish a PR review without an approved verdict.");
      }
      fs.writeFileSync(reviewReportPath, `${state.finalVerdict}\n`, {
        mode: 0o600,
      });
      if (await confirmPostApprovedReview(prUrl)) {
        postReviewComment(repo, prUrl, reviewReportPath);
        console.log("\x1b[32m[Posted]\x1b[0m ✅ Review comment posted to GitHub.");
      } else {
        console.log(
          `\x1b[33m[Not Posted]\x1b[0m Review preserved at ${reviewReportPath}`,
        );
      }
      state.completedSteps.push("review");
      saveWorkflowState(statePath, state);
    }
    workflowSucceeded = true;
  } finally {
    if (specialistPaneId) {
      try {
        runCommand("herdr", ["pane", "close", specialistPaneId]);
      } catch (error) {
        console.error(`Failed to close Herdr pane ${specialistPaneId}:`, error);
      }
    }
    if (!workflowSucceeded && !workflowPaused) {
      console.error(
        `Workflow interrupted. Worktree preserved for automatic resume: ${targetDir}`,
      );
    }
  }

  console.log(`\x1b[32m[COMPLETE]\x1b[0m PR approved: ${prUrl}`);
  console.log(`\x1b[32m[Worktree]\x1b[0m Preserved at ${targetDir}`);
}

const isEntrypoint =
  process.argv[1] !== undefined &&
  fs.realpathSync(process.argv[1]) === fs.realpathSync(fileURLToPath(import.meta.url));

if (isEntrypoint) {
  runTrackedWorkflow("imp", process.argv.slice(2), main).catch((err: unknown) => {
    const message = err instanceof Error ? err.message : String(err);
    console.error(`\x1b[31m[Pipeline Error]\x1b[0m ${message}`);
    process.exit(1);
  });
}
