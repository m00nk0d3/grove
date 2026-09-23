import test from "node:test";
import assert from "node:assert/strict";
import {
  assertAgentSettled,
  assertModeOverrideCompatible,
  FULL_WORKFLOW_STEPS,
  getAgentLaunchConfig,
  getClaudeAgentArgs,
  getPiAgentArgs,
  ensureJavaScriptDependencies,
  getVerificationCommand,
  planVerification,
  summarizeCommandFailure,
  LEAN_WORKFLOW_STEPS,
  loadWorkflowState,
  parseCliArgs,
  PI_COMPACTION_GUARD_PATH,
  resolveAgentBackend,
  slugifyIssueTitle,
  synchronizeDefaultBranch,
  warnIfWindowsPathLimitLikely,
} from "./workflow-utils.js";
import compactionGuard, {
  buildCompactionRecoveryMessage,
} from "./pi-compaction-guard.js";
import {
  buildAssignmentPointerPrompt,
  buildCompletionRetryPrompt,
  deliverablePrompt,
  MAX_INLINE_PROMPT_CHARS,
  findMissingCompletionArtifacts,
  promptAgent,
  startAgentWithReadinessRecovery,
  waitForPiAgentSettled,
} from "./herdr-specialist.js";
import { readAuditVerdict } from "./review-loop.js";
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
import {
  detectStack,
  detectStackProjects,
  findDotNetProject,
  type StackProject,
} from "./stack-detector.js";
import {
  collectChangedFiles,
  selectConditionalSpecialists,
} from "./specialist-gating.js";
import {
  ensureClaudeWorkspaceTrust,
  trustKeyFor,
} from "./claude-trust.js";
import {
  headRepositoryOf,
  validateFixablePullRequest,
} from "./ci-fix.js";
import {
  countFeedback,
  countSuggestions,
  extractSuggestions,
  formatFeedback,
  parseAddressArgs,
  parseFeedbackResponse,
  selectActionableFeedback,
  validateWithRepair,
} from "./address-review.js";
import {
  buildImplementationPersona,
  getImplementationPrompt,
  SPECIALISTS,
} from "./specialists.js";

import fs from "node:fs";
import path from "node:path";
import { execFileSync } from "node:child_process";
import {
  captureWorktreeState,
  createLeanReport,
  implementationSessionId,
  readLeanReportEvidence,
  readPullRequestTitle,
  retitleDeliveryCommit,
  runValidationWithRepair,
} from "./orchestrator.js";

const TS_PERSONA = buildImplementationPersona([
  { stack: "TYPESCRIPT", root: "", marker: "package.json" },
]);
const GO_PERSONA = buildImplementationPersona([
  { stack: "GO", root: "", marker: "go.mod" },
]);

test("parseCliArgs accepts an issue number", () => {
  assert.deepEqual(parseCliArgs(["42"]), {
    requestedRepo: null,
    issueNum: "42",
    modeOverride: null,
    refreshProfile: false,
  });
});

test("parseCliArgs accepts an explicit repository", () => {
  assert.deepEqual(parseCliArgs(["owner/repo", "42"]), {
    requestedRepo: "owner/repo",
    issueNum: "42",
    modeOverride: null,
    refreshProfile: false,
  });
});

test("parseCliArgs accepts lean and full overrides in either supported form", () => {
  assert.deepEqual(parseCliArgs(["42", "--lean"]), {
    requestedRepo: null,
    issueNum: "42",
    modeOverride: "lean",
    refreshProfile: false,
  });
  assert.deepEqual(parseCliArgs(["--full", "owner/repo", "42"]), {
    requestedRepo: "owner/repo",
    issueNum: "42",
    modeOverride: "full",
    refreshProfile: false,
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
    "fix-oauth-callback-don-t-lose-query",
  );
  assert.equal(slugifyIssueTitle("  Crème brûlée  "), "creme-brulee");
  assert.equal(slugifyIssueTitle("🚀"), "issue");
  // A single word longer than the budget still has to be cut.
  assert.equal(slugifyIssueTitle("a".repeat(80)), "a".repeat(40));
});

test("slugifyIssueTitle truncates on word boundaries to bound worktree paths", () => {
  const title =
    "feat(agent portal): KPI evolution selector at target group granularity";

  const slug = slugifyIssueTitle(title);
  assert.equal(slug, "feat-agent-portal-kpi-evolution-selector");
  assert.ok(slug.length <= 40, `slug too long: ${slug.length}`);
  assert.ok(!slug.endsWith("-"), "slug must not end with a separator");
  // The previous hard character cut ended this title mid-word, at "-gra".
  assert.ok(!/-gra$/.test(slug), `slug was cut mid-word: ${slug}`);

  assert.equal(slugifyIssueTitle(title, 20), "feat-agent-portal");
});

test("warnIfWindowsPathLimitLikely never breaks the workflow", () => {
  // `git config --get` exits non-zero when the key is unset.
  assert.doesNotThrow(() =>
    warnIfWindowsPathLimitLikely("/repo", () => {
      throw new Error("exit status 1");
    }),
  );
  assert.doesNotThrow(() => warnIfWindowsPathLimitLikely("/repo", () => "true"));
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
      "--auto",
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

test("getClaudeAgentArgs uses Claude Code defaults and supports overrides", () => {
  const defaultArgs = getClaudeAgentArgs({});
  assert.equal(defaultArgs[0], "--");
  assert.deepEqual(defaultArgs.slice(1, 3), ["--permission-mode", "acceptEdits"]);
  assert.equal(defaultArgs[3], "--append-system-prompt");
  assert.match(defaultArgs[4], /continuity contract/);
  assert.deepEqual(defaultArgs.slice(5), [
    "--allowedTools",
    "Read",
    "Write",
    "Edit",
    "Bash",
    // Windows exposes PowerShell as its own tool. Without it every PowerShell
    // call waits for a person and the run ends in agent_blocked.
    "PowerShell",
    "Glob",
    "Grep",
  ]);

  const overrideArgs = getClaudeAgentArgs({
    AGENT_FLOW_CLAUDE_MODEL: "claude-opus-5",
    AGENT_FLOW_CLAUDE_PERMISSION_MODE: "plan",
    AGENT_FLOW_CLAUDE_TOOLS: "Read, Bash ,Write",
  });
  assert.deepEqual(overrideArgs.slice(0, 5), [
    "--",
    "--model",
    "claude-opus-5",
    "--permission-mode",
    "plan",
  ]);
  assert.deepEqual(overrideArgs.slice(-4), [
    "--allowedTools",
    "Read",
    "Bash",
    "Write",
  ]);
});

test("getAgentLaunchConfig supports the Claude Code backend", () => {
  assert.deepEqual(
    getAgentLaunchConfig({ AGENT_FLOW_AGENT_BACKEND: "claude" }),
    {
      backend: "claude",
      kind: "claude",
      label: "Claude Code",
      args: getClaudeAgentArgs({}),
      needsLmStudioEnv: false,
    },
  );
  assert.equal(
    getAgentLaunchConfig({
      AGENT_FLOW_AGENT_BACKEND: "claude",
      AGENT_FLOW_CLAUDE_MODEL: "claude-opus-5",
    }).label,
    "Claude Code (claude-opus-5)",
  );
});

test("resolveAgentBackend validates the configured backend", () => {
  assert.equal(resolveAgentBackend({}), "opencode");
  assert.equal(
    resolveAgentBackend({ AGENT_FLOW_AGENT_BACKEND: "claude" }),
    "claude",
  );
  assert.equal(resolveAgentBackend({ AGENT_FLOW_AGENT_BACKEND: "pi" }), "pi");
  assert.throws(
    () => resolveAgentBackend({ AGENT_FLOW_AGENT_BACKEND: "unknown" }),
    /Unsupported AGENT_FLOW_AGENT_BACKEND/,
  );
});

test("agent startup recovers from a transient not-ready failure", () => {
  const startArgs = ["agent", "start", "af-role-1-2", "--kind", "claude"];

  const calls: string[][] = [];
  startAgentWithReadinessRecovery("af-role-1-2", startArgs, (_command, args) => {
    calls.push(args);
    return "";
  });
  assert.deepEqual(calls, [startArgs], "a clean start must not wait");

  const recoveredCalls: string[][] = [];
  startAgentWithReadinessRecovery("af-role-1-2", startArgs, (_command, args) => {
    recoveredCalls.push(args);
    if (recoveredCalls.length === 1) {
      throw new Error("agent af-role-1-2 is blocked during startup");
    }
    return "";
  });
  assert.deepEqual(recoveredCalls[1], [
    "agent",
    "wait",
    "af-role-1-2",
    "--until",
    "idle",
    "--timeout",
    "120000",
  ]);

  assert.throws(
    () =>
      startAgentWithReadinessRecovery("af-role-1-2", startArgs, (_c, args) => {
        if (args[1] === "start") {
          throw new Error("agent af-role-1-2 is blocked during startup");
        }
        throw new Error("timed out waiting for idle");
      }),
    /blocked during startup/,
    "a failed wait must surface the original startup error",
  );
});

test("completion retry guidance matches the active backend", () => {
  assert.match(
    buildCompletionRetryPrompt(["Missing artifact"], "claude"),
    /Write tool/,
  );
  assert.match(
    buildCompletionRetryPrompt(["Missing artifact"], "opencode"),
    /echo '\{"key":"value"\}'/,
  );
  assert.match(
    buildCompletionRetryPrompt(["Missing artifact"]),
    /echo '\{"key":"value"\}'/,
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
  // The conditional specialists sit between verification and cleanup in both
  // modes; each one decides from the diff whether it has anything to review.
  assert.deepEqual(LEAN_WORKFLOW_STEPS, [
    "lean-planning",
    "lean-implementation",
    "lean-review",
    "verification",
    "domain-review",
    "documentation",
    "delivery",
    "report",
    "publish",
    "review",
  ]);
  assert.deepEqual(FULL_WORKFLOW_STEPS, [
    "planning",
    "tests",
    "implementation",
    "verification",
    "domain-review",
    "documentation",
    "delivery",
    "report",
    "publish",
    "review",
  ]);

  // Every step is an agent stage or a gate that owns its own bookkeeping;
  // Git plumbing is folded into the stage it belongs to rather than tracked
  // as a stage of its own.
  for (const retired of [
    "issue-analysis",
    "repository-scout",
    "architecture",
    "cleanup",
    "push",
    "pr",
    "publish-review",
    "security-audit",
    "database-review",
    "api-contract-review",
    "adversarial-review",
  ]) {
    assert.ok(
      !(FULL_WORKFLOW_STEPS as readonly string[]).includes(retired),
      `${retired} should no longer be a tracked step`,
    );
  }
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
    SPECIALISTS.LEAN_PLANNER(TS_PERSONA, "42", "owner/repo", ".agent/issue-42/LEAN_PLAN.md"),
    SPECIALISTS.LEAN_IMPLEMENTER(
      TS_PERSONA,
      "42",
      "owner/repo",
      ".agent/issue-42/LEAN_PLAN.md",
    ),
    SPECIALISTS.LEAN_REVIEWER(
      TS_PERSONA,
      "42",
      "owner/repo",
      ".agent/issue-42/LEAN_PLAN.md",
      ".agent-lean-verification.json",
    ),
    SPECIALISTS.PLANNER(
      TS_PERSONA,
      "42",
      "owner/repo",
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
      ".agent/PLAN.md",
    ),
    SPECIALISTS.TEST_ENGINEER(
      TS_PERSONA,
      "42",
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
      ".agent/PLAN.md",
    ),
    getImplementationPrompt(
      TS_PERSONA,
      "42",
      ".agent/REQUIREMENTS.md",
      ".agent/CONTEXT.md",
      ".agent/PLAN.md",
    ),
    SPECIALISTS.VERIFIER(
      TS_PERSONA,
      "42",
      ".agent/REQUIREMENTS.md",
      ".agent/PLAN.md",
    ),
    SPECIALISTS.DOMAIN_REVIEWER(TS_PERSONA, "42", "owner/repo", "/tmp/verdict.json", {
      title: "security",
      sections: "## Concern: security",
      matchCount: 2,
    }),
    SPECIALISTS.IMPLEMENTER_VALIDATION_FIXES(
      TS_PERSONA,
      "42",
      "owner/repo",
      ".agent/PLAN.md",
      "npm test",
      "git exited with status 2: file.ts:3: trailing whitespace.",
    ),
    SPECIALISTS.PR_REVIEWER(
      TS_PERSONA,
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
    TS_PERSONA,
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
    TS_PERSONA,
    "42",
    "owner/repo",
    ".agent/issue-42/LEAN_PLAN.md",
  );
  const prompt = SPECIALISTS.LEAN_IMPLEMENTER(
    TS_PERSONA,
    "42",
    "owner/repo",
    ".agent/issue-42/LEAN_PLAN.md",
  );
  const reviewer = SPECIALISTS.LEAN_REVIEWER(
    TS_PERSONA,
    "42",
    "owner/repo",
    ".agent/issue-42/LEAN_PLAN.md",
    ".agent-lean-verification.json",
  );
  assert.match(planner, /scope, acceptance criteria, affected areas/);
  assert.match(planner, /only new or modified artifact/);
  assert.match(planner, /Do not modify product code/);
  assert.match(prompt, /Lean Implementation Specialist/);
  assert.match(prompt, /Read the complete GitHub issue using: gh issue view/);
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
    GO_PERSONA,
    "42",
    "owner/repo",
    ".agent/LEAN_PLAN.md",
    "/tmp/review.json",
  );
  const reviewer = SPECIALISTS.PR_REVIEWER(
    TS_PERSONA,
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
    TS_PERSONA,
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
    version: 7 as const,
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
  } catch (error) {
    throw new Error(
      `PR review verdict invalid: ${error instanceof Error ? error.message : String(error)}`
    );
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
  } catch (error) {
    throw new Error(
      `PR review verdict invalid: ${error instanceof Error ? error.message : String(error)}`
    );
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

test("legacy checkpoints migrate to the current step list", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-state-"));
  const statePath = path.join(root, "issue-42.json");
  const write = (value: unknown) =>
    fs.writeFileSync(statePath, JSON.stringify(value));
  const base = {
    repo: "owner/repo",
    issueNum: "42",
    issueTitle: "Test issue",
    branchName: "agent/test-42",
    baseCommit: "abc123",
  };

  try {
    // Planning used to be three stages. Those names no longer exist, so the
    // work is replanned rather than silently treated as done.
    write({
      ...base,
      version: 2,
      completedSteps: ["issue-analysis", "repository-scout"],
    });
    let state = loadWorkflowState(statePath);
    assert.equal(state?.version, 7);
    assert.equal(state?.mode, "full");
    assert.equal(state?.modeSource, "migration");
    assert.deepEqual(state?.completedSteps, []);
    assert.equal(state?.reviewCyclesCompleted, 0);
    assert.equal(state?.approved, false);

    // Steps that still line up from the start are kept, so a checkpoint only
    // reruns the stages it can no longer account for.
    write({
      ...base,
      version: 5,
      mode: "lean",
      modeReason: "single focused change",
      modeSource: "automatic",
      completedSteps: ["lean-planning", "lean-implementation", "cleanup"],
      reviewCyclesCompleted: 0,
      approved: false,
    });
    state = loadWorkflowState(statePath);
    assert.equal(state?.version, 7);
    assert.equal(state?.mode, "lean");
    assert.deepEqual(state?.completedSteps, [
      "lean-planning",
      "lean-implementation",
    ]);
    assert.match(state?.modeReason ?? "", /rerunning stages after/);

    // A checkpoint already on the current list is returned untouched.
    const current = {
      ...base,
      version: 7,
      mode: "full" as const,
      modeReason: "scope spans multiple components",
      modeSource: "automatic" as const,
      completedSteps: ["planning", "tests"],
      reviewCyclesCompleted: 0,
      approved: false,
    };
    write(current);
    state = loadWorkflowState(statePath);
    assert.deepEqual(state?.completedSteps, ["planning", "tests"]);
    assert.equal(state?.modeReason, "scope spans multiple components");
    assert.equal(state?.modeSource, "automatic");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a checkpoint whose steps are not a prefix of its mode is rejected", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-state-"));
  const statePath = path.join(root, "issue-42.json");
  try {
    fs.writeFileSync(
      statePath,
      JSON.stringify({
        repo: "owner/repo",
        issueNum: "42",
        issueTitle: "Test issue",
        branchName: "agent/test-42",
        baseCommit: "abc123",
        version: 7,
        mode: "full",
        modeReason: "scope spans multiple components",
        modeSource: "automatic",
        completedSteps: ["tests", "planning"],
        reviewCyclesCompleted: 0,
        approved: false,
      }),
    );
    assert.throws(() => loadWorkflowState(statePath), /Invalid workflow checkpoint/);
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

test("detectStack recognizes a .NET solution nested below the repository root", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-test-"));
  try {
    // A .NET repository commonly keeps its solution in backend/ or src/, with
    // a JavaScript frontend beside it; without this the repository looks like
    // plain TypeScript and gets `npm test` as its verification command.
    fs.mkdirSync(path.join(root, "backend"));
    fs.writeFileSync(path.join(root, "backend", "App.sln"), "");
    assert.equal(detectStack(root), "CSHARP");
    assert.equal(findDotNetProject(root), "backend/App.sln");

    // `dotnet test` resolves from the working directory, so the solution has
    // to be named or the run fails with MSB1003.
    assert.deepEqual(getVerificationCommand("CSHARP", root), {
      command: "dotnet",
      args: ["test", "backend/App.sln"],
    });

    // A solution wins over a bare project so every test project is covered.
    fs.writeFileSync(path.join(root, "backend", "App.csproj"), "");
    assert.equal(findDotNetProject(root), "backend/App.sln");

    // With no directory to inspect, fall back to the bare command.
    assert.deepEqual(getVerificationCommand("CSHARP"), {
      command: "dotnet",
      args: ["test"],
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("detectStack ignores build output when looking for a .NET project", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-test-"));
  try {
    fs.mkdirSync(path.join(root, "node_modules"));
    fs.writeFileSync(path.join(root, "node_modules", "Vendored.csproj"), "");
    assert.equal(detectStack(root), "TYPESCRIPT");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a multi-stack repository is identified as every stack it contains", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-test-"));
  try {
    fs.mkdirSync(path.join(root, "backend"));
    fs.mkdirSync(path.join(root, "frontend"));
    fs.writeFileSync(path.join(root, "backend", "App.sln"), "");
    fs.writeFileSync(path.join(root, "frontend", "package.json"), "{}");

    assert.deepEqual(detectStackProjects(root), [
      { stack: "CSHARP", root: "backend", marker: "backend/App.sln" },
      {
        stack: "TYPESCRIPT",
        root: "frontend",
        marker: "frontend/package.json",
      },
    ]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("validation runs each project where it lives, chosen by the diff", () => {
  const projects: StackProject[] = [
    { stack: "CSHARP", root: "backend", marker: "backend/App.sln" },
    { stack: "TYPESCRIPT", root: "frontend", marker: "frontend/package.json" },
  ];

  // A backend-only change must not be "validated" by the frontend's tests.
  assert.deepEqual(
    planVerification(projects, ["backend/src/Api/AgentController.cs"]).map(
      (task) => task.label,
    ),
    ["dotnet test App.sln (in backend)"],
  );

  assert.deepEqual(
    planVerification(projects, ["frontend/src/KpiTray.tsx"]).map(
      (task) => task.label,
    ),
    ["npm test (in frontend)"],
  );

  // A change spanning both trees has to satisfy both.
  assert.deepEqual(
    planVerification(projects, [
      "backend/src/Api/AgentController.cs",
      "frontend/src/KpiTray.tsx",
    ]).map((task) => task.label),
    ["dotnet test App.sln (in backend)", "npm test (in frontend)"],
  );

  // Unattributable changes are validated everywhere rather than guessed at.
  assert.equal(planVerification(projects, ["README.md"]).length, 2);
  assert.equal(planVerification(projects, []).length, 2);

  // No recognised project keeps the historical repository-root default.
  assert.deepEqual(planVerification([], ["src/index.ts"]), [
    {
      stack: "TYPESCRIPT",
      command: "npm",
      args: ["test"],
      root: "",
      label: "npm test",
      source: "builtin",
    },
  ]);
});

test("the implementation persona names every stack it may have to edit", () => {
  const single = buildImplementationPersona([
    { stack: "CSHARP", root: "", marker: "App.sln" },
  ]);
  assert.match(single, /Senior \.NET Engineer/);
  assert.doesNotMatch(single, /more than one stack/);

  const mixed = buildImplementationPersona([
    { stack: "CSHARP", root: "backend", marker: "backend/App.sln" },
    { stack: "TYPESCRIPT", root: "frontend", marker: "frontend/package.json" },
  ]);
  assert.match(mixed, /more than one stack/);
  assert.match(mixed, /backend: CSHARP/);
  assert.match(mixed, /frontend: TYPESCRIPT/);
  assert.match(mixed, /Senior \.NET Engineer/);
  assert.match(mixed, /Senior TypeScript Engineer/);
});

test("a blocked agent is handed to a person instead of failing the stage", () => {
  const blocked = () => {
    throw new Error(
      'herdr exited with status 1: {"error":{"code":"agent_blocked","message":"agent af-reviewer is blocked and requires interactive input"}}',
    );
  };

  // The person answers, the agent goes idle, and the prompt is delivered.
  const calls: string[][] = [];
  let answered = false;
  const output = promptAgent("af-reviewer", "review this", {
    extraArgs: ["--wait"],
    runner: (_command, args) => {
      calls.push(args);
      if (args[1] === "wait") {
        answered = true;
        return "";
      }
      if (args[1] === "prompt" && !answered) {
        blocked();
      }
      return "delivered";
    },
  });
  assert.equal(output, "delivered");
  assert.deepEqual(
    calls.map((args) => args.slice(0, 2).join(" ")),
    ["agent prompt", "agent focus", "agent wait", "agent prompt"],
    "the pane is focused, waited on, then the prompt is retried",
  );
  assert.ok(calls[2].includes("idle"));

  // Nobody answers: the original block is surfaced, not the wait failure.
  assert.throws(
    () =>
      promptAgent("af-reviewer", "review this", {
        runner: (_command, args) => {
          if (args[1] === "wait") throw new Error("timed out waiting for idle");
          if (args[1] === "focus") return "";
          return blocked() as unknown as string;
        },
      }),
    /agent_blocked/,
  );

  // Opting out restores fail-fast, and unrelated failures are never swallowed.
  assert.throws(
    () => promptAgent("af-reviewer", "x", { timeoutMs: 0, runner: blocked }),
    /agent_blocked/,
  );
  assert.throws(
    () =>
      promptAgent("af-reviewer", "x", {
        runner: () => {
          throw new Error("herdr: no such agent");
        },
      }),
    /no such agent/,
  );
});

test("Claude workspace trust is recorded before an agent launches", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-trust-"));
  const configPath = path.join(root, ".claude.json");
  const worktree = path.join(root, "worktrees", "issue-42");
  fs.mkdirSync(worktree, { recursive: true });
  try {
    // Anything else in the file, including credentials, must survive untouched.
    fs.writeFileSync(
      configPath,
      JSON.stringify({
        oauthAccount: { accountUuid: "keep-me" },
        projects: {
          "C:/already/trusted": { hasTrustDialogAccepted: true, other: 1 },
        },
      }),
    );

    assert.equal(ensureClaudeWorkspaceTrust(worktree, configPath), "recorded");

    const written = JSON.parse(fs.readFileSync(configPath, "utf8"));
    assert.deepEqual(written.oauthAccount, { accountUuid: "keep-me" });
    assert.deepEqual(written.projects["C:/already/trusted"], {
      hasTrustDialogAccepted: true,
      other: 1,
    });
    assert.equal(
      written.projects[trustKeyFor(worktree)].hasTrustDialogAccepted,
      true,
    );
    // Claude keys projects with forward slashes on every platform.
    assert.ok(!trustKeyFor(worktree).includes("\\"));

    // Recording twice must not rewrite the file or lose sibling fields.
    assert.equal(
      ensureClaudeWorkspaceTrust(worktree, configPath),
      "already-trusted",
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("Claude workspace trust never rewrites a config it cannot understand", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-trust-"));
  const configPath = path.join(root, ".claude.json");
  try {
    // A missing config is left for Claude to create.
    assert.equal(ensureClaudeWorkspaceTrust(root, configPath), "skipped");

    // Unparseable content is preserved byte for byte rather than replaced.
    const corrupt = '{"projects": {oops';
    fs.writeFileSync(configPath, corrupt);
    assert.equal(ensureClaudeWorkspaceTrust(root, configPath), "skipped");
    assert.equal(fs.readFileSync(configPath, "utf8"), corrupt);

    fs.writeFileSync(configPath, JSON.stringify(["not", "an", "object"]));
    assert.equal(ensureClaudeWorkspaceTrust(root, configPath), "skipped");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("conditional specialists are selected from the changed files", () => {
  const withDocs = (changedFiles: string[]) =>
    selectConditionalSpecialists({
      changedFiles,
      hasDocumentationSurface: true,
    });

  assert.deepEqual(
    withDocs(["frontend/src/components/KpiTray.tsx"]),
    ["documentation"],
    "an ordinary UI change must not pay for audits it does not need",
  );

  assert.deepEqual(
    withDocs([
      "backend/src/Infrastructure/Migrations/20260101_AddColumn.cs",
    ]).sort(),
    ["database-review", "documentation"],
  );

  assert.ok(
    withDocs(["backend/src/API/Controllers/AuthController.cs"]).includes(
      "security-audit",
    ),
  );
  assert.ok(
    withDocs(["backend/src/API/Controllers/AuthController.cs"]).includes(
      "api-contract-review",
    ),
  );

  assert.deepEqual(
    withDocs(["docs/adr/0001-thing.md", "README.md"]),
    [],
    "a documentation-only diff must not trigger the documentation stage",
  );

  assert.deepEqual(withDocs([]), [], "an empty diff selects nothing");

  assert.deepEqual(
    selectConditionalSpecialists({
      changedFiles: ["src/index.ts"],
      hasDocumentationSurface: false,
    }),
    [],
    "a repository with nowhere to document anything skips the stage",
  );
});

test("changed files are collected from tracked edits and untracked additions", () => {
  const calls: string[][] = [];
  const files = collectChangedFiles("/repo", (_command, args) => {
    calls.push(args);
    return args[0] === "diff" ? "src/a.ts\nsrc/b.ts" : "src/c.ts";
  });

  assert.deepEqual(files, ["src/a.ts", "src/b.ts", "src/c.ts"]);
  assert.deepEqual(calls, [
    ["diff", "--name-only", "HEAD"],
    ["ls-files", "--others", "--exclude-standard"],
  ]);

  // A repository without a HEAD commit must not fail the workflow.
  assert.deepEqual(
    collectChangedFiles("/repo", () => {
      throw new Error("fatal: bad revision 'HEAD'");
    }),
    [],
  );
});

test("address parses its arguments and rejects anything else", () => {
  assert.equal(parseAddressArgs(["1160"]).prNumber, "1160");
  for (const args of [[], ["0"], ["abc"], ["1160", "extra"], ["owner/repo"]]) {
    assert.throws(() => parseAddressArgs(args), /Usage: address/);
  }
});

test("review feedback is parsed and narrowed to what the author can act on", () => {
  const raw = JSON.stringify({
    data: {
      repository: {
        pullRequest: {
          reviewThreads: {
            nodes: [
              {
                path: "src/Api.cs",
                line: 42,
                isResolved: false,
                isOutdated: false,
                comments: { nodes: [{ author: { login: "alice" }, body: "Guard this null", url: "u1" }] },
              },
              {
                path: "src/Old.cs",
                line: 7,
                isResolved: false,
                isOutdated: true,
                comments: { nodes: [{ author: { login: "bob" }, body: "Rename it", url: "u2" }] },
              },
              {
                path: "src/Done.cs",
                line: 1,
                isResolved: true,
                isOutdated: false,
                comments: { nodes: [{ author: { login: "alice" }, body: "Settled", url: "u3" }] },
              },
              {
                path: "src/Mine.cs",
                line: 3,
                isResolved: false,
                isOutdated: false,
                comments: { nodes: [{ author: { login: "me" }, body: "note to self", url: "u4" }] },
              },
            ],
          },
          reviews: {
            nodes: [
              { author: { login: "alice" }, body: "Needs work", state: "CHANGES_REQUESTED", url: "r1" },
              { author: { login: "bob" }, body: "LGTM", state: "APPROVED", url: "r2" },
            ],
          },
          comments: {
            nodes: [
              { author: { login: "carol" }, body: "Why this approach?", url: "c1" },
              { author: { login: "me" }, body: "because", url: "c2" },
            ],
          },
        },
      },
    },
  });

  const selected = selectActionableFeedback(parseFeedbackResponse(raw), "me");

  assert.deepEqual(
    selected.threads.map((thread) => thread.path),
    ["src/Api.cs", "src/Old.cs"],
    "resolved threads and the author's own threads are dropped; outdated ones are kept",
  );
  assert.deepEqual(
    selected.reviews.map((review) => [review.author, review.state]),
    [
      ["alice", "CHANGES_REQUESTED"],
      ["bob", "APPROVED"],
    ],
    "an approving reviewer who still wrote notes is not discarded",
  );
  assert.deepEqual(
    selected.comments.map((comment) => comment.author),
    ["carol"],
    "the author's own comments are not feedback to act on",
  );
  assert.equal(countFeedback(selected), 5);

  const formatted = formatFeedback(selected);
  assert.match(formatted, /src\/Api\.cs:42/);
  assert.match(formatted, /OUTDATED/, "outdated threads must be labelled for the agent");
  assert.match(formatted, /Changes requested/);
  assert.match(formatted, /Other review notes/);
  assert.match(formatted, /Why this approach\?/);
  assert.doesNotMatch(formatted, /note to self/);
});

test("an empty pull request yields no feedback to act on", () => {
  const empty = parseFeedbackResponse(JSON.stringify({ data: { repository: { pullRequest: {} } } }));
  assert.equal(countFeedback(selectActionableFeedback(empty, "me")), 0);
});

test("a same-repo pull request is not mistaken for a fork", () => {
  // `gh pr view` returns headRepository.nameWithOwner as an empty string,
  // which the old comparison read as "not this repository".
  const sameRepo = {
    number: 1160,
    title: "Some PR",
    url: "u",
    headRefName: "agent/feat-x-1082",
    headRefOid: "abc",
    baseRefName: "main",
    isCrossRepository: false,
    state: "OPEN",
    headRepository: { name: "OPSupervisor", nameWithOwner: "" },
    headRepositoryOwner: { login: "TP-Software-Development" },
  };

  assert.equal(headRepositoryOf(sameRepo), "TP-Software-Development/OPSupervisor");
  assert.doesNotThrow(() =>
    validateFixablePullRequest(sameRepo, "TP-Software-Development/OPSupervisor", "address"),
  );

  // A real fork must still be refused, by either signal.
  assert.throws(
    () =>
      validateFixablePullRequest(
        { ...sameRepo, isCrossRepository: true },
        "TP-Software-Development/OPSupervisor",
      ),
    /comes from a fork/,
  );
  assert.throws(
    () =>
      validateFixablePullRequest(
        { ...sameRepo, headRepositoryOwner: { login: "someone-else" } },
        "TP-Software-Development/OPSupervisor",
      ),
    /comes from a fork/,
  );

  // An unidentifiable head repository falls back to isCrossRepository alone.
  assert.doesNotThrow(() =>
    validateFixablePullRequest(
      { ...sameRepo, headRepository: null, headRepositoryOwner: null },
      "TP-Software-Development/OPSupervisor",
    ),
  );

  assert.throws(
    () =>
      validateFixablePullRequest(
        { ...sameRepo, state: "CLOSED" },
        "TP-Software-Development/OPSupervisor",
      ),
    /is closed/,
  );
});

test("suggested changes are surfaced as the reviewer's literal replacement", () => {
  const body =
    "This reads better inverted:\n\n```suggestion\nif (value == null) {\n  return fallback;\n}\n```\n\nup to you";

  assert.deepEqual(extractSuggestions(body), [
    "if (value == null) {\n  return fallback;\n}",
  ]);
  assert.deepEqual(extractSuggestions("no suggestion here"), []);

  const feedback = selectActionableFeedback(
    {
      threads: [
        {
          path: "src/Api.cs",
          line: 42,
          isResolved: false,
          isOutdated: false,
          comments: [{ author: "alice", body, url: "u1" }],
        },
      ],
      reviews: [],
      comments: [],
    },
    "me",
  );

  assert.equal(countSuggestions(feedback), 1);
  const formatted = formatFeedback(feedback);
  assert.match(formatted, /SUGGESTED CHANGE/);
  assert.match(
    formatted,
    /if \(value == null\) \{\n {2}return fallback;\n\}/,
    "the proposed code must appear verbatim, not truncated or paraphrased",
  );
});

test("a suggestion longer than the truncation limit is never cut", () => {
  const long = Array.from({ length: 200 }, (_, i) => `line ${i};`).join("\n");
  const feedback: Parameters<typeof formatFeedback>[0] = {
    threads: [
      {
        path: "src/Big.cs",
        line: 1,
        isResolved: false,
        isOutdated: false,
        comments: [
          { author: "alice", body: "```suggestion\n" + long + "\n```", url: "u" },
        ],
      },
    ],
    reviews: [],
    comments: [],
  };
  const formatted = formatFeedback(feedback);
  assert.match(formatted, /line 199;/);
  assert.doesNotMatch(formatted, /truncated/);
});

test("a long review body survives; shorter remarks are still bounded", () => {
  // Reviewers who leave no line comments put the whole review in the body,
  // and several thousand characters of findings is ordinary.
  const longReview = "F".repeat(6000);
  const longComment = "C".repeat(6000);

  const formatted = formatFeedback({
    threads: [],
    reviews: [{ author: "alice", body: longReview, state: "APPROVED", url: "r" }],
    comments: [{ author: "bob", body: longComment, url: "c" }],
  });

  assert.ok(
    formatted.includes("F".repeat(6000)),
    "a 6000-character review body must reach the agent intact",
  );
  assert.ok(
    !formatted.includes("C".repeat(3001)),
    "a pull request comment is still bounded",
  );
  assert.match(formatted, /truncated; read the full text on GitHub/);
});

test("a stalled prompt waits for the turn instead of re-sending it", () => {
  // Herdr delivers the prompt, then fails if it does not observe the agent
  // start within five seconds. Re-sending would duplicate the instruction.
  const calls: string[][] = [];
  const output = promptAgent("af-responder", "address this feedback", {
    extraArgs: ["--wait", "--timeout", "1800000"],
    settleTimeoutMs: 1800000,
    runner: (_command, args) => {
      calls.push(args);
      if (args[1] === "prompt") {
        throw new Error(
          'herdr exited with status 1: {"error":{"code":"agent_prompt_stalled","message":"agent prompt produced no observed working or blocked state within 5000 ms; current status is idle"}}',
        );
      }
      return "settled";
    },
  });

  assert.equal(output, "settled");
  assert.deepEqual(
    calls.map((args) => args.slice(0, 2).join(" ")),
    ["agent prompt", "agent wait"],
    "the prompt must not be sent twice",
  );
  assert.deepEqual(calls[1], ["agent", "wait", "af-responder", "--timeout", "1800000"]);
  assert.ok(
    !calls[1].includes("--until"),
    "a stalled turn may end idle, done or blocked",
  );
});

test("address accepts --continue to pick up a failed run's worktree", () => {
  assert.deepEqual(parseAddressArgs(["1163"]), {
    prNumber: "1163",
    resume: false,
  });
  assert.deepEqual(parseAddressArgs(["1163", "--continue"]), {
    prNumber: "1163",
    resume: true,
  });
  assert.deepEqual(parseAddressArgs(["--continue", "1163"]), {
    prNumber: "1163",
    resume: true,
  });
  for (const args of [[], ["--continue"], ["abc", "--continue"]]) {
    assert.throws(() => parseAddressArgs(args), /Usage: address/);
  }
});

test("address keeps handing a failing gate back until it passes", () => {
  const roles: string[] = [];
  let attempts = 0;
  // Fails twice, then passes — a first fix often reveals the next failure.
  const verify = () => {
    attempts += 1;
    if (attempts < 3) throw new Error(`dotnet test exited with status 1: ${attempts} failed`);
  };

  validateWithRepair(
    process.cwd(),
    "owner/repo",
    "1163",
    "persona",
    (role) => roles.push(role),
    verify,
  );

  assert.equal(attempts, 3, "the gate is re-run after every repair");
  assert.deepEqual(roles, [
    "review-responder-validation-fix-1",
    "review-responder-validation-fix-2",
  ]);
});

test("address gives up with the real failure and points at the worktree", () => {
  const roles: string[] = [];
  assert.throws(
    () =>
      validateWithRepair(
        process.cwd(),
        "owner/repo",
        "1163",
        "persona",
        (role) => roles.push(role),
        () => {
          throw new Error("dotnet test exited with status 1: Backend.Tests crashed");
        },
      ),
    (error: Error) =>
      /still fails after 2 repair attempts/.test(error.message) &&
      /Backend\.Tests crashed/.test(error.message) &&
      /--continue/.test(error.message),
  );
  assert.equal(roles.length, 2, "bounded: it does not retry forever");
});

test("address does not call the agent when the gate passes first time", () => {
  const roles: string[] = [];
  validateWithRepair(process.cwd(), "owner/repo", "1163", "persona", (role) => roles.push(role), () => {});
  assert.deepEqual(roles, []);
});

test("an audit verdict is judged on its own schema, not the PR reviewer's", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-verdict-"));
  const verdictPath = path.join(root, "security-audit-verdict.json");
  try {
    // Exactly what SPECIALISTS.SECURITY_AUDITOR asks for: no `fixes`, no
    // `validation`. The PR reviewer schema rejected this as invalid.
    fs.writeFileSync(
      verdictPath,
      JSON.stringify({
        verdict: "approved",
        summary: "No authorization or input-handling defects in this diff.",
        reviewedAreas: ["LobLabelsController [Authorize] coverage"],
        blockers: [],
        preExisting: [],
        residualRisks: [],
      }),
    );
    assert.throws(() => readReviewVerdict(verdictPath), /invalid schema/);

    const audit = readAuditVerdict(verdictPath, "security-audit");
    assert.equal(audit.verdict, "approved");
    assert.deepEqual(audit.blockers, []);

    // A fenced object is a formatting slip, not a failed audit.
    fs.writeFileSync(
      verdictPath,
      '```json\n{"verdict":"blockers","summary":"s","reviewedAreas":["a"],"blockers":["high | x.cs:1 | y"]}\n```',
    );
    assert.equal(readAuditVerdict(verdictPath).verdict, "blockers");

    // "approved" while listing blockers is contradictory; believe the blockers.
    fs.writeFileSync(
      verdictPath,
      JSON.stringify({
        verdict: "approved",
        summary: "s",
        reviewedAreas: ["a"],
        blockers: ["high | x.cs:1 | real finding"],
      }),
    );
    assert.equal(readAuditVerdict(verdictPath).verdict, "blockers");

    for (const bad of [
      { summary: "s", reviewedAreas: ["a"], blockers: [] },
      { verdict: "approved", reviewedAreas: ["a"], blockers: [] },
      { verdict: "approved", summary: "  ", reviewedAreas: ["a"], blockers: [] },
      { verdict: "maybe", summary: "s", reviewedAreas: ["a"], blockers: [] },
    ]) {
      fs.writeFileSync(verdictPath, JSON.stringify(bad));
      assert.throws(() => readAuditVerdict(verdictPath, "database-review"), /invalid schema/);
    }

    fs.rmSync(verdictPath);
    assert.throws(() => readAuditVerdict(verdictPath, "security-audit"), /wrote no verdict/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("the pull request title says what changed, or falls back to the issue", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".agent-flow-title-"));
  const titlePath = path.join(root, "issue-1086-pr-title.txt");
  const fallback = "feat(kpi): hide target groups with no data";
  try {
    const write = (value: string) => fs.writeFileSync(titlePath, value);

    write("feat(kpi): skip target groups whose rollup returns no rows\n");
    assert.equal(
      readPullRequestTitle(titlePath, fallback),
      "feat(kpi): skip target groups whose rollup returns no rows",
    );

    // Models decorate despite being told not to.
    write('  "fix(api): return 409 on duplicate lob_shortname"  \n');
    assert.equal(
      readPullRequestTitle(titlePath, fallback),
      "fix(api): return 409 on duplicate lob_shortname",
    );
    write("# feat(ui): collapse empty KPI rows\n\nsome stray prose\n");
    assert.equal(
      readPullRequestTitle(titlePath, fallback),
      "feat(ui): collapse empty KPI rows",
    );
    write("- perf(kpi): memoize target group resolution");
    assert.equal(
      readPullRequestTitle(titlePath, fallback),
      "perf(kpi): memoize target group resolution",
    );

    // A restatement of the task is exactly what we are trying to avoid.
    for (const useless of ["resolve #1086", "Implement issue 1086", "fix #1086"]) {
      write(useless);
      assert.equal(readPullRequestTitle(titlePath, fallback), fallback);
    }

    write("");
    assert.equal(readPullRequestTitle(titlePath, fallback), fallback);
    write(`feat(kpi): ${"x".repeat(200)}`);
    assert.equal(readPullRequestTitle(titlePath, fallback), fallback);

    fs.rmSync(titlePath);
    assert.equal(readPullRequestTitle(titlePath, fallback), fallback);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("the delivery commit is retitled only while it is still unpublished", () => {
  const title = "feat(kpi): skip target groups whose rollup returns no rows";
  const fake = (opts: { onRemote?: string; subject: string; body?: string }) => {
    const calls: string[][] = [];
    const runner = (_command: string, args: string[]) => {
      calls.push(args);
      if (args[0] === "ls-remote") return opts.onRemote ?? "";
      if (args[0] === "log" && args.includes("--format=%s")) return opts.subject;
      if (args[0] === "log" && args.includes("--format=%b")) return opts.body ?? "";
      return "";
    };
    return { calls, runner };
  };

  // Unpushed, placeholder subject: retitle and keep the trailers.
  const fresh = fake({
    subject: "fix: resolve #1086",
    body: "Co-authored-by: Copilot <x@y>",
  });
  assert.equal(
    retitleDeliveryCommit("/wt", "agent/x-1086", title, fresh.runner),
    "retitled",
  );
  const amend = fresh.calls.find((args) => args[0] === "commit");
  assert.deepEqual(amend, [
    "commit",
    "--amend",
    "-m",
    title,
    "-m",
    "Co-authored-by: Copilot <x@y>",
  ]);

  // Already on the remote: rewriting would diverge from what others have.
  const pushed = fake({
    onRemote: "abc123\trefs/heads/agent/x-1086",
    subject: "fix: resolve #1086",
  });
  assert.equal(
    retitleDeliveryCommit("/wt", "agent/x-1086", title, pushed.runner),
    "skipped",
  );
  assert.ok(!pushed.calls.some((args) => args[0] === "commit"));

  // A subject someone chose deliberately is never overwritten.
  const deliberate = fake({ subject: "feat(kpi): a subject a human wrote" });
  assert.equal(
    retitleDeliveryCommit("/wt", "agent/x-1086", title, deliberate.runner),
    "skipped",
  );

  // ls-remote failing means we cannot prove it is unpushed.
  assert.equal(
    retitleDeliveryCommit("/wt", "agent/x-1086", title, (_c, args) => {
      if (args[0] === "ls-remote") throw new Error("network down");
      return "fix: resolve #1086";
    }),
    "skipped",
  );

  // Re-running after a retitle is a no-op rather than a second amend.
  const already = fake({ subject: title });
  assert.equal(
    retitleDeliveryCommit("/wt", "agent/x-1086", title, already.runner),
    "skipped",
  );
});

test("a JavaScript project checked out without its dependencies gets them installed", () => {
  const root = fs.mkdtempSync(path.join(process.cwd(), ".js-deps-"));
  try {
    const calls: { args: string[]; cwd?: string }[] = [];
    const runner = (_command: string, args: string[], options?: { cwd?: string }) => {
      calls.push({ args, cwd: options?.cwd });
      return "";
    };

    // A git worktree carries no ignored files, so this is what every fresh
    // checkout of a JavaScript project looks like.
    fs.writeFileSync(path.join(root, "package.json"), "{}");
    fs.writeFileSync(path.join(root, "package-lock.json"), "{}");
    assert.equal(ensureJavaScriptDependencies(root, runner), "installed");
    assert.deepEqual(calls, [{ args: ["ci"], cwd: root }], "a lockfile means ci");

    // Without a lockfile there is nothing for ci to honour.
    calls.length = 0;
    fs.rmSync(path.join(root, "package-lock.json"));
    assert.equal(ensureJavaScriptDependencies(root, runner), "installed");
    assert.deepEqual(calls, [{ args: ["install"], cwd: root }]);

    // Already installed: nothing is run, so verification does not pay for an
    // install on every repair cycle.
    calls.length = 0;
    fs.mkdirSync(path.join(root, "node_modules"));
    assert.equal(ensureJavaScriptDependencies(root, runner), "present");
    assert.deepEqual(calls, []);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a failing verification command reports the failure, not the whole transcript", () => {
  // Short output is worth reading in full.
  const short = summarizeCommandFailure("npm test", 1, "  1 test failed  ");
  assert.equal(short, "npm test failed with exit code 1:\n1 test failed");

  assert.match(
    summarizeCommandFailure("npm test", null, "   "),
    /produced no output/,
  );

  // A long transcript: a restore log and hundreds of warnings, one real
  // failure in the middle, and the summary at the end.
  const noise = Array.from(
    { length: 400 },
    (_, index) => `warning CS0618: 'Transform' is obsolete [project ${index}]`,
  );
  const transcript = [
    ...noise.slice(0, 200),
    "  X KpiRollupsTests.SuppressesEmptyGroups [FAIL]",
    "  Assert.Equal() Failure: Values differ",
    "  Expected: 3",
    "  Actual:   4",
    ...noise.slice(200),
    "Failed!  - Failed:     1, Passed:  2438, Skipped:     0, Total:  2439",
  ].join("\n");

  const summary = summarizeCommandFailure(
    "dotnet test OPS.sln (in backend)",
    1,
    transcript,
  );
  assert.ok(
    summary.length < transcript.length / 4,
    "the transcript is cut down rather than passed along",
  );
  assert.match(summary, /dotnet test OPS\.sln \(in backend\) failed with exit code 1/);
  assert.match(summary, /SuppressesEmptyGroups \[FAIL\]/, "the failing test survives");
  assert.match(summary, /Assert\.Equal\(\) Failure/, "so does its assertion");
  assert.match(
    summary,
    /Expected: 3[\s\S]*Actual:   4/,
    "an assertion's values follow the line naming it, so they come along",
  );
  assert.match(summary, /Failed:     1, Passed:  2438/, "and the closing summary");
  assert.ok(
    !summary.includes("[project 100]"),
    "the wall of obsolete-API warnings names no failure and is dropped",
  );

  // Nothing in the transcript names a failure, so the end of it is all the
  // signal there is and it is shown rather than dropped.
  const silent = Array.from(
    { length: 500 },
    (_, index) => `step ${index} completed`,
  ).join("\n");
  const fallback = summarizeCommandFailure("npm test", 1, silent);
  assert.match(fallback, /showing the end of its output/);
  assert.match(fallback, /step 499 completed/);
});

// A prompt reaches herdr as one command-line argument, and Windows refuses a
// command line past 32767 characters with ENAMETOOLONG. A review of a wide pull
// request carries its metadata, which measured about 47 KB on the request that
// first hit this, so the spawn failed before the agent existed.
test("an assignment that fits a command line is sent as it is", () => {
  const assignment = "x".repeat(MAX_INLINE_PROMPT_CHARS);
  assert.equal(
    deliverablePrompt(assignment, "/tmp/agent-flow/assignment.md"),
    assignment,
  );
});

test("an assignment too large for a command line is sent as a path to read", () => {
  const assignmentPath = "/tmp/agent-flow-continuity-abc/assignment.md";
  const assignment = "x".repeat(MAX_INLINE_PROMPT_CHARS + 1);

  const delivered = deliverablePrompt(assignment, assignmentPath);

  assert.notEqual(delivered, assignment);
  assert.match(delivered, /assignment\.md/);
  assert.ok(
    delivered.includes(assignmentPath),
    "the agent has to be told exactly which file to read",
  );
});

test("the delivered prompt fits a Windows command line whatever the assignment size", () => {
  // The platform limit, not this module's threshold: the threshold exists to
  // stay under this, so the test is worth nothing if it only restates it.
  const windowsCommandLineLimit = 32767;
  for (const size of [0, 1, MAX_INLINE_PROMPT_CHARS, 50_000, 5_000_000]) {
    const delivered = deliverablePrompt(
      "x".repeat(size),
      "/tmp/agent-flow-continuity-abc/assignment.md",
    );
    assert.ok(
      delivered.length < windowsCommandLineLimit / 2,
      `a ${size} character assignment produced a ${delivered.length} character prompt`,
    );
  }
});

test("the pointer prompt tells the agent to read the file first and not ask for a resend", () => {
  const pointer = buildAssignmentPointerPrompt("/tmp/x/assignment.md");
  assert.match(pointer, /before doing anything else/i);
  assert.match(pointer, /do not ask/i);
  assert.match(pointer, /complete assignment/i);
});
