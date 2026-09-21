import test from "node:test";
import assert from "node:assert/strict";
import {
  assertAgentSettled,
  assertModeOverrideCompatible,
  FULL_WORKFLOW_STEPS,
  getAgentLaunchConfig,
  getPiAgentArgs,
  getVerificationCommand,
  LEAN_WORKFLOW_STEPS,
  loadWorkflowState,
  parseCliArgs,
  PI_COMPACTION_GUARD_PATH,
  slugifyIssueTitle,
  synchronizeDefaultBranch,
} from "./workflow-utils.js";
import compactionGuard, {
  buildCompactionRecoveryMessage,
} from "./pi-compaction-guard.js";
import {
  buildCompletionRetryPrompt,
  findMissingCompletionArtifacts,
  waitForPiAgentSettled,
} from "./herdr-specialist.js";
import {
  formatReviewVerdict,
  isAffirmative,
  postReviewComment,
  readReviewVerdict,
} from "./review-loop.js";
import {
  classifyIssue,
  explicitClassification,
  parseIssueMetadata,
} from "./issue-classifier.js";
import { detectStack } from "./stack-detector.js";
import { getImplementationPrompt, SPECIALISTS } from "./specialists.js";
import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import {
  captureWorktreeState,
  createLeanReport,
  implementationSessionId,
  readLeanReportEvidence,
  runValidationWithRepair,
} from "./orchestrator.js";

test("parseCliArgs accepts an issue number", () => {
  assert.deepEqual(parseCliArgs(["42"]), {
    requestedRepo: null,
    issueNum: "42",
    modeOverride: null,
  });
});

test("parseCliArgs accepts an explicit repository", () => {
  assert.deepEqual(parseCliArgs(["owner/repo", "42"]), {
    requestedRepo: "owner/repo",
    issueNum: "42",
    modeOverride: null,
  });
});

test("parseCliArgs accepts lean and full overrides in either supported form", () => {
  assert.deepEqual(parseCliArgs(["42", "--lean"]), {
    requestedRepo: null,
    issueNum: "42",
    modeOverride: "lean",
  });
  assert.deepEqual(parseCliArgs(["--full", "owner/repo", "42"]), {
    requestedRepo: "owner/repo",
    issueNum: "42",
    modeOverride: "full",
  });
});

test("parseCliArgs rejects unsafe and malformed values", () => {
  for (const args of [
    [],
    ["0"],
    ["owner/repo", "1; rm"],
    ["owner/repo/extra", "1"],
    ["owner/repo", "1", "extra"],
    ["42", "--lean", "--full"],
    ["42", "--unknown"],
  ]) {
    assert.throws(() => parseCliArgs(args), /Usage:|Conflicting workflow flags/);
  }
});

test("slugifyIssueTitle creates safe, bounded branch components", () => {
  assert.equal(
    slugifyIssueTitle("Fix OAuth callback: don't lose query params!"),
    "fix-oauth-callback-don-t-lose-query-params",
  );
  assert.equal(slugifyIssueTitle("  Crème brûlée  "), "creme-brulee");
  assert.equal(slugifyIssueTitle("🚀"), "issue");
  assert.equal(slugifyIssueTitle("a".repeat(80)), "a".repeat(60));
});

test("getPiAgentArgs uses LM Studio defaults and supports overrides", () => {
  const defaultArgs = getPiAgentArgs({});
  assert.deepEqual(defaultArgs.slice(0, 7), [
    "--",
    "--provider",
    "lm-studio",
    "--model",
    "qwen/qwen3.5-9b",
    "--no-extensions",
    "--extension",
  ]);
  assert.equal(
    defaultArgs[defaultArgs.indexOf("--extension") + 1],
    PI_COMPACTION_GUARD_PATH,
  );
  assert.match(
    defaultArgs[defaultArgs.indexOf("--append-system-prompt") + 1],
    /context compaction is lossy/,
  );
  assert.deepEqual(defaultArgs.slice(-2), [
    "--tools",
    "read,bash,edit,write",
  ]);
  const overrideArgs = getPiAgentArgs({
    AGENT_FLOW_PI_PROVIDER: "local-provider",
    AGENT_FLOW_PI_MODEL: "local-model",
  });
  assert.equal(overrideArgs[overrideArgs.indexOf("--provider") + 1], "local-provider");
  assert.equal(overrideArgs[overrideArgs.indexOf("--model") + 1], "local-model");
  assert.match(
    overrideArgs[overrideArgs.indexOf("--append-system-prompt") + 1],
    /context compaction is lossy/,
  );
  assert.deepEqual(overrideArgs.slice(-2), [
    "--tools",
    "read,bash,edit,write",
  ]);
});

test("compaction recovery restores the exact assignment and recovery steps", () => {
  const message = buildCompactionRecoveryMessage(
    "Implement owner/repo#42 and write RESULT.json.",
  );
  assert.match(message, /filesystem\/Git state as authoritative/);
  assert.match(message, /remaining acceptance criteria/);
  assert.match(
    message,
    /<exact-original-assignment>\nImplement owner\/repo#42 and write RESULT\.json\.\n<\/exact-original-assignment>/,
  );
});

test("compaction guard injects recovery context and records the event", async () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".compaction-guard-"));
  const assignmentPath = path.join(root, "assignment.md");
  const statusPath = path.join(root, "status.jsonl");
  fs.writeFileSync(assignmentPath, "Complete the verifier stage.", "utf8");
  const previousAssignment = process.env.AGENT_FLOW_ASSIGNMENT_FILE;
  const previousStatus = process.env.AGENT_FLOW_COMPACTION_STATUS_FILE;
  process.env.AGENT_FLOW_ASSIGNMENT_FILE = assignmentPath;
  process.env.AGENT_FLOW_COMPACTION_STATUS_FILE = statusPath;
  const handlers = new Map<
    string,
    (event: {
      reason?: string;
      compactionEntry?: { tokensBefore?: number };
    }) => void | Promise<void>
  >();
  const messages: string[] = [];

  try {
    compactionGuard({
      on: (event, handler) => handlers.set(event, handler),
      sendMessage: (message) => messages.push(message.content),
    });
    assert.match(fs.readFileSync(statusPath, "utf8"), /"type":"guard-ready"/);
    await handlers.get("session_compact")?.({
      reason: "threshold",
      compactionEntry: { tokensBefore: 33000 },
    });
    await handlers.get("agent_settled")?.({});
    assert.match(messages[0], /Complete the verifier stage/);
    assert.match(
      fs.readFileSync(statusPath, "utf8"),
      /"type":"compaction-completed".*"tokensBefore":33000/,
    );
    assert.equal(waitForPiAgentSettled(statusPath, 0), 1);
  } finally {
    if (previousAssignment === undefined) {
      delete process.env.AGENT_FLOW_ASSIGNMENT_FILE;
    } else {
      process.env.AGENT_FLOW_ASSIGNMENT_FILE = previousAssignment;
    }
    if (previousStatus === undefined) {
      delete process.env.AGENT_FLOW_COMPACTION_STATUS_FILE;
    } else {
      process.env.AGENT_FLOW_COMPACTION_STATUS_FILE = previousStatus;
    }
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("Pi settlement rejects compaction failures and missing settlement", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".pi-settlement-"));
  const statusPath = path.join(root, "status.jsonl");
  try {
    fs.writeFileSync(
      statusPath,
      '{"type":"compaction-failed","error":"summary request canceled"}\n{"type":"agent-settled"}\n',
      "utf8",
    );
    assert.throws(
      () => waitForPiAgentSettled(statusPath, 0),
      /summary request canceled/,
    );
    fs.writeFileSync(statusPath, '{"type":"guard-ready"}\n', "utf8");
    assert.throws(
      () => waitForPiAgentSettled(statusPath, 0),
      /authoritative settled state/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("completion artifact recovery identifies missing output and focuses retry", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".artifact-recovery-"));
  const completePath = path.join(root, "complete.json");
  const missingPath = path.join(root, "missing.json");
  fs.writeFileSync(completePath, '{"status":"complete"}', "utf8");
  try {
    assert.deepEqual(
      findMissingCompletionArtifacts([completePath, missingPath]),
      [missingPath],
    );
    const prompt = buildCompletionRetryPrompt([missingPath]);
    assert.match(
      prompt,
      new RegExp(missingPath.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
    );
    assert.match(prompt, /Do not restart broad exploration/);
    assert.match(prompt, /exact original assignment re-injected/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("getAgentLaunchConfig defaults to OpenCode with a Pi override", () => {
  assert.deepEqual(getAgentLaunchConfig({}), {
    backend: "opencode",
    kind: "opencode",
    label: "OpenCode + LM Studio",
    args: [
      "--",
      "--pure",
      "--model",
      "lmstudio/qwen/qwen3.5-9b",
      "--agent",
      "build",
      "--tools",
      "read,bash,edit,write",
    ],
    needsLmStudioEnv: false,
  });
  assert.equal(
    getAgentLaunchConfig({
      AGENT_FLOW_OPENCODE_MODEL: "opencode/mimo-v2.5-free",
    }).label,
    "OpenCode (opencode/mimo-v2.5-free)",
  );
  assert.deepEqual(getAgentLaunchConfig({ AGENT_FLOW_AGENT_BACKEND: "pi" }), {
    backend: "pi",
    kind: "pi",
    label: "Pi + LM Studio",
    args: getPiAgentArgs({ AGENT_FLOW_AGENT_BACKEND: "pi" }),
    needsLmStudioEnv: true,
  });
  assert.deepEqual(
    getAgentLaunchConfig({
      AGENT_FLOW_AGENT_BACKEND: "pi",
      AGENT_FLOW_PI_PROVIDER: "bonsai2",
      AGENT_FLOW_PI_MODEL: "bonsai-2-27b",
    }),
    {
      backend: "pi",
      kind: "pi",
      label: "Pi + Bonsai",
      args: getPiAgentArgs({
        AGENT_FLOW_AGENT_BACKEND: "pi",
        AGENT_FLOW_PI_PROVIDER: "bonsai2",
        AGENT_FLOW_PI_MODEL: "bonsai-2-27b",
      }),
      needsLmStudioEnv: false,
    },
  );
  assert.equal(
    getAgentLaunchConfig({
      AGENT_FLOW_AGENT_BACKEND: "pi",
      AGENT_FLOW_PI_PROVIDER: "lm-studio",
    }).needsLmStudioEnv,
    true,
  );
  assert.throws(
    () => getAgentLaunchConfig({ AGENT_FLOW_AGENT_BACKEND: "unknown" }),
    /Unsupported AGENT_FLOW_AGENT_BACKEND/,
  );
});

test("synchronizeDefaultBranch fast-forwards with only Sandcastle state untracked", () => {
  const calls: Array<{ command: string; args: string[] }> = [];
  const responses = new Map([
    [
      "git status --porcelain --untracked-files=normal",
      "?? .sandcastle/",
    ],
    [
      "gh repo view --json defaultBranchRef --jq .defaultBranchRef.name",
      "main",
    ],
    ["git branch --show-current", "main"],
    ["git pull --ff-only origin main", "Already up to date."],
  ]);
  const runner = (command: string, args: string[]): string => {
    calls.push({ command, args });
    return responses.get([command, ...args].join(" ")) ?? "";
  };

  assert.equal(synchronizeDefaultBranch("/repo", runner), "main");
  assert.deepEqual(calls.at(-1), {
    command: "git",
    args: ["pull", "--ff-only", "origin", "main"],
  });
});

test("synchronizeDefaultBranch rejects dirty or non-default checkouts", () => {
  assert.throws(
    () =>
      synchronizeDefaultBranch("/repo", (command, args) =>
        [command, ...args].join(" ") ===
        "git status --porcelain --untracked-files=normal"
          ? " M src/index.ts"
          : "",
      ),
    /checkout must be clean/,
  );

  assert.throws(
    () =>
      synchronizeDefaultBranch("/repo", (command, args) => {
        const invocation = [command, ...args].join(" ");
        if (
          invocation === "git status --porcelain --untracked-files=normal"
        ) {
          return "";
        }
        if (invocation.includes("defaultBranchRef")) return "main";
        if (invocation === "git branch --show-current") return "feature";
        return "";
      }),
    /must start from the default branch 'main'.*'feature'/,
  );
});

test("getVerificationCommand returns deterministic stack checks", () => {
  assert.deepEqual(getVerificationCommand("GO"), {
    command: "go",
    args: ["test", "./..."],
  });
  assert.deepEqual(getVerificationCommand("PYTHON"), {
    command: "python",
    args: ["-m", "pytest"],
  });
  assert.deepEqual(getVerificationCommand("TYPESCRIPT"), {
    command: "npm",
    args: ["test"],
  });
});

test("lean workflow tracks three agent stages before the delivery gate", () => {
  assert.deepEqual(LEAN_WORKFLOW_STEPS, [
    "lean-planning",
    "lean-implementation",
    "lean-review",
    "verification",
    "cleanup",
    "delivery",
    "report",
    "push",
    "pr",
    "review",
    "publish-review",
  ]);
  assert.deepEqual(FULL_WORKFLOW_STEPS.slice(0, 8), [
    "issue-analysis",
    "repository-scout",
    "architecture",
    "tests",
    "implementation",
    "verification",
    "adversarial-review",
    "cleanup",
  ]);
});

test("worktree state compares content across resumed and staged changes", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".lean-planning-state-"));
  const git = (...args: string[]) =>
    execFileSync("git", args, { cwd: root, stdio: "pipe" });
  const planPath = ".agent/issue-42/LEAN_PLAN.md";
  try {
    git("init", "--quiet");
    fs.writeFileSync(path.join(root, "tracked.txt"), "original\n");
    git("add", "tracked.txt");
    git(
      "-c",
      "user.name=Test",
      "-c",
      "user.email=test@example.com",
      "commit",
      "--quiet",
      "-m",
      "initial",
    );
    fs.writeFileSync(path.join(root, "tracked.txt"), "existing tracked work\n");
    fs.writeFileSync(path.join(root, "resumed.txt"), "existing work\n");
    const before = captureWorktreeState(root, planPath);

    fs.mkdirSync(path.join(root, ".agent", "issue-42"), { recursive: true });
    fs.writeFileSync(path.join(root, planPath), "implementation plan\n");
    assert.equal(captureWorktreeState(root, planPath), before);

    git("add", "tracked.txt");
    assert.equal(
      captureWorktreeState(root, planPath),
      before,
      "staging metadata alone does not count as an implementation edit",
    );

    fs.writeFileSync(path.join(root, "resumed.txt"), "planner changed it\n");
    assert.notEqual(captureWorktreeState(root, planPath), before);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("every specialist enforces the shared file-size standard", () => {
  const prompts = [
    SPECIALISTS.LEAN_PLANNER("42", "owner/repo", ".agent/issue-42/LEAN_PLAN.md"),
    SPECIALISTS.LEAN_IMPLEMENTER(
      "TYPESCRIPT",
      "42",
      "owner/repo",
      ".agent/issue-42/LEAN_PLAN.md",
    ),
    SPECIALISTS.LEAN_REVIEWER(
      "42",
      "owner/repo",
      ".agent/issue-42/LEAN_PLAN.md",
      ".agent-lean-verification.json",
    ),
    SPECIALISTS.ISSUE_ANALYST("42", "owner/repo", ".agent/REQUIREMENTS.md"),
    SPECIALISTS.REPOSITORY_SCOUT(
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
    ),
    SPECIALISTS.ARCHITECT(
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
      ".agent/PLAN.md",
    ),
    SPECIALISTS.TEST_ENGINEER(
      "42",
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
      ".agent/PLAN.md",
    ),
    getImplementationPrompt(
      "TYPESCRIPT",
      "42",
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
      ".agent/PLAN.md",
    ),
    SPECIALISTS.VERIFIER(
      "42",
      ".agent/REQUIREMENTS.md",
      ".agent/PLAN.md",
    ),
    SPECIALISTS.REVIEWER("42", ".agent/REQUIREMENTS.md"),
    SPECIALISTS.IMPLEMENTER_VALIDATION_FIXES(
      "TYPESCRIPT",
      "42",
      "owner/repo",
      ".agent/PLAN.md",
      "npm test",
      "git exited with status 2: file.ts:3: trailing whitespace.",
    ),
    SPECIALISTS.PR_REVIEWER(
      "owner/repo",
      "42",
      "https://github.com/owner/repo/pull/7",
      "/tmp/verdict.json",
      1,
    ),
  ];

  for (const prompt of prompts) {
    assert.match(prompt, /file length is a design signal, not a hard limit/);
    assert.match(prompt, /Split by responsibility, never solely to satisfy a line count/);
  }
});

test("test specialist does not invent tests for non-behavioral changes", () => {
  const prompt = SPECIALISTS.TEST_ENGINEER(
    "42",
    ".agent/REQUIREMENTS.md",
    ".agent/CONTEXT.md",
    ".agent/PLAN.md",
  );

  assert.match(prompt, /documentation, metadata, or configuration-only/i);
  assert.match(prompt, /do not invent a permanent test/i);
  assert.match(prompt, /no test changes are appropriate/i);
});

test("lean specialists have distinct planning, implementation, and review contracts", () => {
  const planner = SPECIALISTS.LEAN_PLANNER(
    "42",
    "owner/repo",
    ".agent/issue-42/LEAN_PLAN.md",
  );
  const prompt = SPECIALISTS.LEAN_IMPLEMENTER(
    "TYPESCRIPT",
    "42",
    "owner/repo",
    ".agent/issue-42/LEAN_PLAN.md",
  );
  const reviewer = SPECIALISTS.LEAN_REVIEWER(
    "42",
    "owner/repo",
    ".agent/issue-42/LEAN_PLAN.md",
    ".agent-lean-verification.json",
  );
  assert.match(planner, /scope, acceptance criteria, affected areas/);
  assert.match(planner, /only new or modified artifact/);
  assert.match(planner, /Do not modify product code/);
  assert.match(prompt, /Lean Implementation Specialist/);
  assert.match(prompt, /Fetch and read the complete GitHub issue/);
  assert.match(prompt, /LEAN_PLAN\.md/);
  assert.match(prompt, /do not invent permanent tests/i);
  assert.match(prompt, /Do not commit, push, create or edit pull requests/);
  assert.match(reviewer, /independent Lean Verifier and Reviewer/);
  assert.match(reviewer, /Inspect the complete git diff/);
  assert.match(reviewer, /Do not modify source files/);
  assert.match(reviewer, /implementation specialist can fix them/);
  assert.match(reviewer, /"status":"complete"/);
  assert.match(reviewer, /"verdict":"approved\|blockers"/);
  assert.match(reviewer, /"changes":\["concrete behavior or API change reviewed"\]/);
  assert.match(reviewer, /Do not use generic claims/);
  assert.match(reviewer, /Do not commit, push, create or edit pull requests/);
});

test("implementation sessions are stable per repository issue", () => {
  const first = implementationSessionId("owner/repo", "42");
  assert.equal(first, implementationSessionId("owner/repo", "42"));
  assert.notEqual(first, implementationSessionId("owner/repo", "43"));
  assert.match(
    first,
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
  );
});

test("review blockers are handed back to the implementation specialist", () => {
  const fixer = SPECIALISTS.IMPLEMENTER_REVIEW_FIXES(
    "GO",
    "42",
    "owner/repo",
    ".agent/LEAN_PLAN.md",
    "/tmp/review.json",
  );
  const reviewer = SPECIALISTS.PR_REVIEWER(
    "owner/repo",
    "42",
    "https://github.com/owner/repo/pull/7",
    "/tmp/verdict.json",
    1,
  );
  assert.match(fixer, /returning as the original Implementation Specialist/);
  assert.match(fixer, /Fix every confirmed blocker/);
  assert.match(fixer, /Do not review or approve your own work/);
  assert.match(reviewer, /do not modify source files/i);
  assert.match(reviewer, /original implementation specialist/);
  assert.doesNotMatch(reviewer, /fix all confirmed blockers in the worktree/i);
});

test("verifier is required to run the whitespace delivery gate", () => {
  const verifier = SPECIALISTS.VERIFIER(
    "42",
    ".agent/REQUIREMENTS.md",
    ".agent/PLAN.md",
  );

  assert.match(verifier, /git diff --check/);
  assert.match(verifier, /fix whitespace errors/i);
});

test("validation failures receive one bounded implementation repair attempt", () => {
  let validationAttempts = 0;
  const failures: string[] = [];

  runValidationWithRepair(
    () => {
      validationAttempts += 1;
      if (validationAttempts === 1) {
        throw new Error("file.go:166: trailing whitespace.");
      }
    },
    (failure) => failures.push(failure),
  );

  assert.equal(validationAttempts, 2);
  assert.deepEqual(failures, ["file.go:166: trailing whitespace."]);
});

test("validation reports the final failure after its repair attempt", () => {
  assert.throws(
    () =>
      runValidationWithRepair(
        () => {
          throw new Error("go test failed");
        },
        () => {},
      ),
    /Validation still fails after one implementation repair attempt: go test failed/,
  );
});

test("lean reports use concrete reviewer evidence instead of generic placeholders", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".lean-report-"));
  const git = (...args: string[]) =>
    execFileSync("git", args, { cwd: root, stdio: "pipe" });
  const evidencePath = path.join(root, "evidence.json");
  try {
    git("init", "--quiet");
    fs.writeFileSync(path.join(root, "runner.ts"), "export const available = false;\n");
    git("add", "runner.ts");
    git(
      "-c",
      "user.name=Test",
      "-c",
      "user.email=test@example.com",
      "commit",
      "--quiet",
      "-m",
      "initial",
    );
    const baseCommit = git("rev-parse", "HEAD").toString().trim();
    fs.writeFileSync(path.join(root, "runner.ts"), "export const available = true;\n");
    git("add", "runner.ts");
    git(
      "-c",
      "user.name=Test",
      "-c",
      "user.email=test@example.com",
      "commit",
      "--quiet",
      "-m",
      "add availability check",
    );
    fs.writeFileSync(
      evidencePath,
      JSON.stringify({
        status: "complete",
        summary: "Added Sandcastle command discovery and execution.",
        changes: ["The runner now checks command availability before spawning it."],
        architecture: ["runner.ts owns command discovery before process execution."],
        validation: ["npm test passed with 12 tests."],
        acceptanceCriteria: [
          {
            criterion: "Report whether Sandcastle is installed.",
            evidence: "Availability test covers installed and missing commands.",
            status: "✅",
          },
        ],
        compatibility: ["Existing command arguments remain unchanged."],
        risks: ["No known residual risks in the command-runner path."],
        reviewedAreas: ["Confirm missing commands return unavailable without spawning."],
      }),
    );
    const report = createLeanReport(
      "owner/repo",
      "173",
      "Add command runner",
      baseCommit,
      "TYPESCRIPT",
      root,
      readLeanReportEvidence(evidencePath),
    );
    assert.match(report, /checks command availability before spawning it/);
    assert.match(report, /runner\.ts owns command discovery/);
    assert.match(report, /Report whether Sandcastle is installed/);
    assert.doesNotMatch(report, /Applied a focused implementation/);
    assert.doesNotMatch(report, /Preserved the repository's existing architecture/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("lean evidence accepts a single validation command from local models", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".lean-evidence-"));
  const evidencePath = path.join(root, "evidence.json");
  try {
    fs.writeFileSync(
      evidencePath,
      JSON.stringify({
        status: "complete",
        summary: "The adapter behavior is covered.",
        changes: ["Added worktree adapter calls."],
        architecture: ["Client delegates through CommandRunner."],
        validation: "go test ./internal/herdr: passed",
        acceptanceCriteria: [
          {
            criterion: "Commands preserve stderr.",
            evidence: "Focused failure tests pass.",
            status: "✅",
          },
        ],
        compatibility: ["No direct Git fallback was added."],
        risks: ["No known residual risks in the reviewed scope."],
        reviewedAreas: ["Command arguments and error propagation."],
      }),
    );
    const evidence = readLeanReportEvidence(evidencePath);
    assert.equal(evidence.verdict, "approved");
    assert.deepEqual(evidence.validation, [
      "go test ./internal/herdr: passed",
    ]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("issue classification is deterministic, conservative, and scope-aware", () => {
  assert.deepEqual(
    classifyIssue({
      title: "Fix login redirect",
      body: "The callback fails intermittently.",
      labels: [],
    }),
    {
      mode: "full",
      reason: "scope is ambiguous, so the conservative full workflow was selected",
      source: "automatic",
    },
  );
  assert.equal(
    classifyIssue({
      title: "Correct typo in README",
      body: "Fix the spelling in README.md.",
      labels: ["docs"],
    }).mode,
    "lean",
  );
  assert.equal(
    classifyIssue({
      title: "Update docs for API migration",
      body: "Document the database migration and breaking change.",
      labels: ["docs"],
    }).mode,
    "full",
  );
  assert.equal(
    classifyIssue({
      title: "Adjust timeout",
      body: "Update src/client.ts to use the documented default.",
      labels: [],
    }).mode,
    "lean",
  );
  assert.equal(
    classifyIssue({
      title: "Update request flow",
      body: "Touch src/a.ts, src/b.ts, and test/c.test.ts.",
      labels: [],
    }).mode,
    "full",
  );
});

test("issue metadata parsing and explicit classifications are transparent", () => {
  assert.deepEqual(
    parseIssueMetadata(
      JSON.stringify({
        title: "A title",
        body: null,
        labels: [{ name: "docs" }],
      }),
    ),
    { title: "A title", body: "", labels: ["docs"] },
  );
  assert.deepEqual(explicitClassification("lean"), {
    mode: "lean",
    reason: "explicit --lean override",
    source: "explicit",
  });
});

test("resume rejects an override that conflicts with persisted mode", () => {
  const state = {
    version: 5 as const,
    repo: "owner/repo",
    issueNum: "42",
    issueTitle: "Test",
    branchName: "agent/test-42",
    baseCommit: "abc123",
    completedSteps: [],
    reviewCyclesCompleted: 0,
    approved: false,
    mode: "lean" as const,
    modeReason: "scope is localized to README.md",
    modeSource: "automatic" as const,
  };
  assert.doesNotThrow(() => assertModeOverrideCompatible(state, "lean"));
  assert.throws(
    () => assertModeOverrideCompatible(state, "full"),
    /checkpoint uses lean mode/,
  );
});

test("assertAgentSettled rejects blocked and unknown agents", () => {
  assert.doesNotThrow(() =>
    assertAgentSettled('{"result":{"state":"done"}}', "reviewer"),
  );
  assert.throws(
    () => assertAgentSettled('{"result":{"state":"blocked"}}', "reviewer"),
    /reviewer agent stopped in blocked state/,
  );
  assert.throws(
    () => assertAgentSettled('{"result":{"status":"unknown"}}', "planner"),
    /planner agent stopped in unknown state/,
  );
});

test("review verdicts are validated and formatted in detail", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-verdict-"));
  const verdictPath = path.join(root, "verdict.json");
  try {
    fs.writeFileSync(
      verdictPath,
      JSON.stringify({
        verdict: "blockers",
        summary: "One correctness blocker requires implementation work.",
        reviewedAreas: ["Nil handling in the request path."],
        blockers: ["Nil input panicked."],
        fixes: ["Guard nil input before dereferencing it."],
        validation: ["go test ./..."],
        residualRisks: [],
      }),
    );
    const verdict = readReviewVerdict(verdictPath);
    assert.equal(verdict.verdict, "blockers");
    const report = formatReviewVerdict(verdict, 2);
    assert.match(report, /🚧 Review Cycle 2: BLOCKERS FOUND/);
    assert.match(report, /## 🛠️ Required Fixes/);
    assert.match(report, /- Guard nil input before dereferencing it/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("review verdicts normalize a single validation string", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-verdict-"));
  const verdictPath = path.join(root, "verdict.json");
  try {
    fs.writeFileSync(
      verdictPath,
      JSON.stringify({
        verdict: "approved",
        summary: "The implementation satisfies the acceptance criteria.",
        reviewedAreas: ["Dashboard rendering."],
        blockers: [],
        fixes: ["None required."],
        validation: "go test ./...: passed.",
        residualRisks: [],
      }),
    );
    const verdict = readReviewVerdict(verdictPath);
    assert.deepEqual(verdict.validation, ["go test ./...: passed."]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("review continuation accepts only explicit affirmative answers", () => {
  assert.equal(isAffirmative("yes"), true);
  assert.equal(isAffirmative(" Y "), true);
  assert.equal(isAffirmative(""), false);
  assert.equal(isAffirmative("no"), false);
});

test("approved verdicts are posted as detailed PR review comments", () => {
  let invocation:
    | { command: string; args: string[] }
    | undefined;
  postReviewComment(
    "owner/repo",
    "https://github.com/owner/repo/pull/42",
    "/tmp/review.md",
    (command, args) => {
      invocation = { command, args };
      return "";
    },
  );
  assert.deepEqual(invocation, {
    command: "gh",
    args: [
      "pr",
      "review",
      "https://github.com/owner/repo/pull/42",
      "--repo",
      "owner/repo",
      "--comment",
      "--body-file",
      "/tmp/review.md",
    ],
  });
});

test("version 2 checkpoints migrate to resumable review state", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-state-"));
  const statePath = path.join(root, "issue-42.json");
  try {
    fs.writeFileSync(
      statePath,
      JSON.stringify({
        version: 2,
        repo: "owner/repo",
        issueNum: "42",
        issueTitle: "Test issue",
        branchName: "agent/test-42",
        baseCommit: "abc123",
        completedSteps: ["issue-analysis", "repository-scout"],
      }),
    );
    const state = loadWorkflowState(statePath);
    assert.equal(state?.version, 5);
    assert.equal(state?.reviewCyclesCompleted, 0);
    assert.equal(state?.approved, false);
    assert.equal(state?.mode, "full");
    assert.equal(state?.modeSource, "migration");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("version 3 checkpoints preserve the full workflow on migration", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-v3-state-"));
  const statePath = path.join(root, "issue-43.json");
  try {
    fs.writeFileSync(
      statePath,
      JSON.stringify({
        version: 3,
        repo: "owner/repo",
        issueNum: "43",
        issueTitle: "Existing workflow",
        branchName: "agent/existing-43",
        baseCommit: "def456",
        completedSteps: ["issue-analysis", "repository-scout", "architecture"],
        reviewCyclesCompleted: 0,
        approved: false,
      }),
    );
    const state = loadWorkflowState(statePath);
    assert.equal(state?.version, 5);
    assert.equal(state?.mode, "full");
    assert.equal(state?.modeSource, "migration");
    assert.match(state?.modeReason ?? "", /version 3/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("version 4 lean checkpoints conservatively rerun all three lean stages", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-v4-state-"));
  const statePath = path.join(root, "issue-44.json");
  try {
    fs.writeFileSync(
      statePath,
      JSON.stringify({
        version: 4,
        repo: "owner/repo",
        issueNum: "44",
        issueTitle: "Small docs fix",
        branchName: "agent/small-docs-fix-44",
        baseCommit: "123abc",
        completedSteps: ["implementation", "verification"],
        reviewCyclesCompleted: 0,
        approved: false,
        mode: "lean",
        modeReason: "label 'docs' indicates a small documentation or maintenance change",
        modeSource: "automatic",
      }),
    );
    const state = loadWorkflowState(statePath);
    assert.equal(state?.version, 5);
    assert.equal(state?.mode, "lean");
    assert.equal(state?.modeSource, "migration");
    assert.deepEqual(state?.completedSteps, []);
    assert.match(state?.modeReason ?? "", /conservatively rerunning all three lean stages/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("version 4 full checkpoints preserve completed full-mode steps", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-v4-full-state-"));
  const statePath = path.join(root, "issue-45.json");
  try {
    fs.writeFileSync(
      statePath,
      JSON.stringify({
        version: 4,
        repo: "owner/repo",
        issueNum: "45",
        issueTitle: "Large behavior change",
        branchName: "agent/large-behavior-change-45",
        baseCommit: "456def",
        completedSteps: ["issue-analysis", "repository-scout", "architecture"],
        reviewCyclesCompleted: 0,
        approved: false,
        mode: "full",
        modeReason: "scope spans multiple components",
        modeSource: "automatic",
      }),
    );
    const state = loadWorkflowState(statePath);
    assert.equal(state?.version, 5);
    assert.equal(state?.mode, "full");
    assert.deepEqual(state?.completedSteps, [
      "issue-analysis",
      "repository-scout",
      "architecture",
    ]);
    assert.equal(state?.modeReason, "scope spans multiple components");
    assert.equal(state?.modeSource, "automatic");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("detectStack recognizes Go, Python, and defaults to TypeScript", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-test-"));
  try {
    assert.equal(detectStack(root), "TYPESCRIPT");
    fs.writeFileSync(path.join(root, "requirements.txt"), "");
    assert.equal(detectStack(root), "PYTHON");
    fs.writeFileSync(path.join(root, "go.mod"), "");
    assert.equal(detectStack(root), "GO");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
