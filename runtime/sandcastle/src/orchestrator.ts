#!/usr/bin/env node

import { createWorktree } from "@ai-hero/sandcastle";
import { createHash } from "node:crypto";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { SPECIALISTS, getImplementationPrompt } from "./specialists.js";
import { detectStack, TechStack } from "./stack-detector.js";
import { runSpecialistInPane } from "./herdr-specialist.js";
import {
  runTrackedWorkflow,
  updateTrackedWorkflow,
} from "./runtime-state.js";
import {
  classifyIssue,
  explicitClassification,
  parseIssueMetadata,
} from "./issue-classifier.js";
import {
  confirmAnotherReviewBatch,
  confirmPostApprovedReview,
  confirmStartPrReview,
  formatReviewVerdict,
  postReviewComment,
  readReviewVerdict,
  REVIEW_BATCH_SIZE,
} from "./review-loop.js";
import {
  assertModeOverrideCompatible,
  detectRepo,
  FULL_WORKFLOW_STEPS,
  getVerificationCommand,
  getWorkflowStatePath,
  loadWorkflowState,
  LEAN_WORKFLOW_STEPS,
  parseCliArgs,
  requireCleanWorktree,
  runCommand,
  saveWorkflowState,
  slugifyIssueTitle,
  synchronizeDefaultBranch,
  verifyWorktree,
  type WorkflowState,
  type WorkflowStep,
} from "./workflow-utils.js";

export function captureWorktreeState(
  targetDir: string,
  ignoredRelativePath: string,
): string {
  const status = runCommand(
    "git",
    ["status", "--porcelain=v1", "--untracked-files=all"],
    { cwd: targetDir },
  )
    .split("\n")
    .filter((line) => line && line.slice(3) !== ignoredRelativePath);
  const trackedDiff = runCommand(
    "git",
    ["diff", "--binary", "HEAD", "--", ".", `:(exclude)${ignoredRelativePath}`],
    { cwd: targetDir },
  );
  const untracked = runCommand(
    "git",
    ["ls-files", "--others", "--exclude-standard", "-z"],
    { cwd: targetDir },
  )
    .split("\0")
    .filter((file) => file && file !== ignoredRelativePath)
    .map((file) => {
      const absolutePath = path.join(targetDir, file);
      const content = fs.lstatSync(absolutePath).isSymbolicLink()
        ? fs.readlinkSync(absolutePath)
        : fs.readFileSync(absolutePath);
      return [file, createHash("sha256").update(content).digest("hex")];
    });
  return JSON.stringify({ status, trackedDiff, untracked });
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
): string {
  const changedFiles = runCommand(
    "git",
    ["diff", "--name-status", baseCommit, "HEAD"],
    { cwd: targetDir },
  );
  const verification = getVerificationCommand(stack);
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
- Orchestrator delivery gate passed: \`${verification.command} ${verification.args.join(" ")}\`
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
  const { requestedRepo, issueNum, modeOverride } = parseCliArgs(args);
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
  if (!state) {
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
    const classification = modeOverride
      ? explicitClassification(modeOverride)
      : classifyIssue(issueMetadata);
    const issueSlug = slugifyIssueTitle(issueMetadata.title);
    state = {
      version: 5,
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
  const agentDir = path.join(".agent", `issue-${issueNum}`);

  console.log(`\x1b[32m[Target Repository]\x1b[0m ${repo} | Issue #${issueNum}`);
  console.log(`\x1b[34m[Workflow Mode]\x1b[0m ${modeReport}`);
  console.log(`\x1b[36m[Sandcastle]\x1b[0m Creating isolated worktree...`);
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
          updateTrackedWorkflow({
            agents: paneId
              ? [
                  {
                    id: `${process.env.GROVE_WORKFLOW_RUN_ID ?? "workflow"}:${role}`,
                    kind:
                      process.env.AGENT_FLOW_AGENT_BACKEND === "pi"
                        ? "pi"
                        : "opencode",
                    name: role,
                    status: "working",
                    summary: `Executing ${role} stage`,
                    pane_id: paneId,
                  },
                ]
              : [],
          });
        },
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
      const verification = getVerificationCommand(stack);
      runValidationWithRepair(
        () => verifyWorktree(stack, targetDir),
        (failure) =>
          runSpecialist(
            `${stack.toLowerCase()}-validation-repair`,
            SPECIALISTS.IMPLEMENTER_VALIDATION_FIXES(
              stack,
              issueNum,
              repo,
              handoffPath,
              `${verification.command} ${verification.args.join(" ")}`,
              failure,
            ),
            [],
            undefined,
            implementerSessionId,
          ),
      );
    };

    const commitChanges = (subject: string): void => {
      const status = runCommand("git", ["status", "--porcelain"], { cwd: targetDir });
      if (!status) {
        throw new Error("The workflow produced no changes to commit.");
      }
      verifyWorktree(stack, targetDir);
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

    if (state.mode === "full") {
      runStep("issue-analysis", () =>
        runSpecialist(
          "issue-analyst",
          SPECIALISTS.ISSUE_ANALYST(issueNum, repo, requirementsPath),
          [path.join(targetDir, requirementsPath)],
        ),
      );
      runStep("repository-scout", () =>
        runSpecialist(
          "repository-scout",
          SPECIALISTS.REPOSITORY_SCOUT(requirementsPath, contextPath),
          [path.join(targetDir, contextPath)],
        ),
      );
      runStep("architecture", () =>
        runSpecialist(
          "architect",
          SPECIALISTS.ARCHITECT(requirementsPath, contextPath, planPath),
          [path.join(targetDir, planPath)],
        ),
      );
      runStep("tests", () =>
        runSpecialist(
          "test-engineer",
          SPECIALISTS.TEST_ENGINEER(
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
            stack,
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
          SPECIALISTS.VERIFIER(issueNum, requirementsPath, planPath),
        );
        verifyWithRepair(planPath);
      });
      runStep("adversarial-review", () => {
        runSpecialist(
          "reviewer",
          SPECIALISTS.REVIEWER(issueNum, requirementsPath),
        );
        verifyWithRepair(planPath);
      });
      runStep("cleanup", () => {
        fs.rmSync(path.join(targetDir, agentDir), { recursive: true, force: true });
        const parentAgentDir = path.join(targetDir, ".agent");
        if (fs.existsSync(parentAgentDir) && fs.readdirSync(parentAgentDir).length === 0) {
          fs.rmdirSync(parentAgentDir);
        }
      });
    } else {
      runStep("lean-planning", () => {
        const stateBefore = captureWorktreeState(targetDir, leanPlanPath);
        runSpecialist(
          "lean-planner",
          SPECIALISTS.LEAN_PLANNER(issueNum, repo, leanPlanPath),
          [path.join(targetDir, leanPlanPath)],
        );
        const stateAfter = captureWorktreeState(targetDir, leanPlanPath);
        if (stateAfter !== stateBefore) {
          throw new Error("Lean planner modified files outside its handoff artifact.");
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
          SPECIALISTS.LEAN_IMPLEMENTER(stack, issueNum, repo, leanPlanPath),
          [],
          undefined,
          implementerSessionId,
        ),
      );
      runStep("lean-review", () => {
        for (let cycle = 1; cycle <= REVIEW_BATCH_SIZE; cycle += 1) {
          fs.rmSync(leanCompletionPath, { force: true });
          const reviewState = captureWorktreeState(targetDir, "__no_ignored_file__");
          runSpecialist(
            `lean-verifier-reviewer-${cycle}`,
            SPECIALISTS.LEAN_REVIEWER(
              issueNum,
              repo,
              leanPlanPath,
              leanCompletionPath,
            ),
            [leanCompletionPath],
            () => {
              readLeanReportEvidence(leanCompletionPath);
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
              stack,
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
      runStep("cleanup", () => {
        fs.rmSync(path.join(targetDir, agentDir), { recursive: true, force: true });
        const parentAgentDir = path.join(targetDir, ".agent");
        if (fs.existsSync(parentAgentDir) && fs.readdirSync(parentAgentDir).length === 0) {
          fs.rmdirSync(parentAgentDir);
        }
      });
    }
    runStep("delivery", () => {
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
      if (state.mode === "full") {
        runSpecialist(
          "implementation-reporter",
          SPECIALISTS.REPORTER(
            repo,
            issueNum,
            issueTitle,
            state.baseCommit,
            reportPath,
          ),
          [reportPath],
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
    runStep("push", () => {
      console.log(`\x1b[33m[Git]\x1b[0m Pushing branch ${branchName}...`);
      runCommand("git", ["push", "-u", "origin", branchName], { cwd: targetDir });
    });
    runStep("pr", () => {
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
            issueTitle,
            "--body-file",
            reportPath,
          ],
          { cwd: targetDir },
        );
      } else {
        runCommand(
          "gh",
          ["pr", "edit", prUrl, "--title", issueTitle, "--body-file", reportPath],
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
          SPECIALISTS.PR_REVIEWER(repo, issueNum, prUrl, verdictPath, cycle),
          [verdictPath],
          () => {
            readReviewVerdict(verdictPath);
          },
        );
        const verdict = readReviewVerdict(verdictPath);
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
            stack,
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
      state.completedSteps.push("review");
      saveWorkflowState(statePath, state);
    }
    if (!state.completedSteps.includes("publish-review")) {
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
      state.completedSteps.push("publish-review");
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
