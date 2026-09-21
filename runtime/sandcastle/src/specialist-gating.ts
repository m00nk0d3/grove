import fs from "fs";
import path from "path";
import { runCommand, type CommandRunner } from "./workflow-utils.js";

// Specialists that review a finished implementation rather than produce it.
// Each one costs an agent run, so a stage is only worth entering when the diff
// actually contains the kind of change it reviews.
export type ConditionalSpecialist =
  | "security-audit"
  | "database-review"
  | "api-contract-review"
  | "documentation";

// Matched against lowercased, forward-slashed repository paths. These are
// deliberately vocabulary-based rather than tied to one framework, so the same
// gate works for a .NET solution, a Rails app, or a Node service.
const DATABASE_PATTERNS = [
  /(^|\/)migrations?(\/|$)/,
  /\.sql$/,
  /(^|\/)alembic(\/|$)/,
  /schema\.(prisma|rb|graphql|sql)$/,
  /dbcontext/,
  /(^|\/)entities?(\/|$)/,
  /(^|\/)seeds?(\/|$)/,
  /seedlookups/,
];

const SECURITY_PATTERNS = [
  /auth/, // authentication, authorization, [Authorize]
  /login/,
  /(^|\/|[-_.])jwt([-_.]|\/|$)/,
  /token/,
  /password/,
  /credential/,
  /secret/,
  /crypt/,
  /cors/,
  /csrf/,
  /permission/,
  /polic(y|ies)/,
  /(^|\/|[-_.])roles?([-_.]|\/|$)/,
  /session/,
  /middleware/,
  /appsettings/,
  /(^|\/)\.env/,
  /web\.config$/,
];

const API_CONTRACT_PATTERNS = [
  /controller/,
  /(^|\/)routes?(\/|$)/,
  /(^|\/)handlers?(\/|$)/,
  /(^|\/)endpoints?(\/|$)/,
  /(^|\/|[-_.])dtos?([-_.]|\/|$)/,
  /(^|\/)api(\/|$)/,
  /openapi/,
  /swagger/,
  /apiclient/,
  /(^|\/)resolvers?(\/|$)/,
];

const DOCUMENTATION_PATTERNS = [
  /\.mdx?$/,
  /(^|\/)docs?(\/|$)/,
  /(^|\/)adr(\/|$)/,
  /(^|\/)changelog(\.|$)/,
  /(^|\/)readme(\.|$)/,
  /(^|\/)licen[cs]e(\.|$)/,
];

function normalize(file: string): string {
  return file.replace(/\\/g, "/").toLowerCase();
}

function matchesAny(file: string, patterns: RegExp[]): boolean {
  return patterns.some((pattern) => pattern.test(file));
}

export function isDocumentationFile(file: string): boolean {
  return matchesAny(normalize(file), DOCUMENTATION_PATTERNS);
}

// collectChangedFiles reports the worktree's uncommitted changes. The workflow
// does not commit until delivery, so tracked edits come from the diff against
// HEAD and new files have to be read from the untracked list.
export function collectChangedFiles(
  targetDir: string,
  runner: CommandRunner = runCommand,
): string[] {
  const files = new Set<string>();
  const absorb = (raw: string): void => {
    for (const line of raw.split("\n")) {
      const file = line.trim();
      if (file) {
        files.add(file);
      }
    }
  };
  for (const args of [
    ["diff", "--name-only", "HEAD"],
    ["ls-files", "--others", "--exclude-standard"],
  ]) {
    try {
      absorb(runner("git", args, { cwd: targetDir }));
    } catch {
      // A missing HEAD or an unreadable index simply yields no evidence; the
      // caller then runs no conditional specialists rather than failing.
    }
  }
  return [...files];
}

// hasDocumentationSurface reports whether the repository keeps documentation
// that an implementation could be expected to update.
export function hasDocumentationSurface(targetDir: string): boolean {
  for (const candidate of ["docs", "doc", "README.md", "readme.md"]) {
    if (fs.existsSync(path.join(targetDir, candidate))) {
      return true;
    }
  }
  return false;
}

export interface GatingInput {
  changedFiles: string[];
  hasDocumentationSurface: boolean;
}

export function selectConditionalSpecialists(
  input: GatingInput,
): ConditionalSpecialist[] {
  const files = input.changedFiles.map(normalize);
  if (files.length === 0) {
    return [];
  }
  const selected: ConditionalSpecialist[] = [];

  if (files.some((file) => matchesAny(file, SECURITY_PATTERNS))) {
    selected.push("security-audit");
  }
  if (files.some((file) => matchesAny(file, DATABASE_PATTERNS))) {
    selected.push("database-review");
  }
  if (files.some((file) => matchesAny(file, API_CONTRACT_PATTERNS))) {
    selected.push("api-contract-review");
  }
  // Documentation follows the change rather than the subject matter: it is
  // worth running whenever code changed and the repository has somewhere to
  // record it, and pointless for a diff that is already documentation-only.
  const changedNonDocumentation = files.some(
    (file) => !matchesAny(file, DOCUMENTATION_PATTERNS),
  );
  if (changedNonDocumentation && input.hasDocumentationSurface) {
    selected.push("documentation");
  }

  return selected;
}
