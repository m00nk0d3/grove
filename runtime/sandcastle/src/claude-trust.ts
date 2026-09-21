import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// Claude Code asks for confirmation the first time it runs in a directory and
// records the answer per exact path — a trusted repository does not extend to
// its worktrees. A workflow creates a new worktree for every issue and every
// pull request review, so an unattended agent meets that dialog on each run and
// blocks waiting for a keypress, which surfaces as `agent_blocked` on the first
// prompt. Recording the answer before launch is the narrowest fix: it grants
// exactly the directory the agent is about to work in, and leaves Claude's
// permission model otherwise untouched.
export type TrustOutcome = "already-trusted" | "recorded" | "skipped";

export function claudeConfigPath(
  env: NodeJS.ProcessEnv = process.env,
): string {
  return env.CLAUDE_CONFIG_DIR
    ? path.join(env.CLAUDE_CONFIG_DIR, ".claude.json")
    : path.join(os.homedir(), ".claude.json");
}

// Claude keys projects by its own working directory, with forward slashes on
// every platform. That directory is fully resolved, so a path reached through
// a symlink or a Windows 8.3 short name ("MARTIN~1.17-") has to be expanded
// here too, or the entry is written under a name Claude never looks up.
export function trustKeyFor(directory: string): string {
  const absolute = path.resolve(directory);
  let resolved = absolute;
  try {
    resolved = fs.realpathSync.native(absolute);
  } catch {
    // Directory may not exist yet; the plain absolute path is the best key.
  }
  return resolved.split(path.sep).join("/");
}

// ensureClaudeWorkspaceTrust is best effort by design. The file it edits also
// holds Claude's own credentials and is written by Claude itself, so anything
// unexpected — a missing file, unreadable JSON, a failed write — leaves it
// untouched and reports "skipped" rather than failing the workflow or
// rewriting a file it does not understand.
export function ensureClaudeWorkspaceTrust(
  directory: string,
  configPath: string = claudeConfigPath(),
): TrustOutcome {
  let raw: string;
  try {
    raw = fs.readFileSync(configPath, "utf8");
  } catch {
    return "skipped"; // no config yet: Claude will create one and ask
  }

  let config: unknown;
  try {
    config = JSON.parse(raw);
  } catch {
    return "skipped"; // never rewrite a file we could not parse
  }
  if (!config || typeof config !== "object" || Array.isArray(config)) {
    return "skipped";
  }

  const root = config as Record<string, unknown>;
  const existingProjects = root.projects;
  const projects =
    existingProjects && typeof existingProjects === "object" && !Array.isArray(existingProjects)
      ? (existingProjects as Record<string, unknown>)
      : {};

  const key = trustKeyFor(directory);
  const existing = projects[key];
  const entry =
    existing && typeof existing === "object" && !Array.isArray(existing)
      ? (existing as Record<string, unknown>)
      : {};
  if (entry.hasTrustDialogAccepted === true) {
    return "already-trusted";
  }

  projects[key] = { ...entry, hasTrustDialogAccepted: true };
  root.projects = projects;

  // Write through a temporary file so a crash cannot truncate the original.
  // Claude may write the same file concurrently; the last writer wins and the
  // entry is only needed at launch, so a lost update costs one dialog rather
  // than any data.
  const temporaryPath = `${configPath}.grove-${process.pid}.tmp`;
  try {
    const mode = fs.statSync(configPath).mode & 0o777;
    fs.writeFileSync(temporaryPath, `${JSON.stringify(root, null, 2)}\n`, {
      mode,
    });
    fs.renameSync(temporaryPath, configPath);
  } catch {
    try {
      fs.rmSync(temporaryPath, { force: true });
    } catch {
      // nothing further to do; the original file is untouched
    }
    return "skipped";
  }
  return "recorded";
}
