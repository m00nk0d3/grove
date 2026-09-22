import fs from "fs";
import path from "path";
import { runCommand, type CommandRunner } from "./workflow-utils.js";
import type { ProfileConcern } from "./project-profile.js";

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

// The built-in vocabularies are one repository's idea of what a migration or a
// controller is called. A profile carries the words this repository actually
// uses, so an Ecto or Rails change reaches database-review even though none of
// `alembic`, `schema.prisma` or `dbcontext` ever matched it.
export function selectProfileConcerns(
  changedFiles: string[],
  concerns: ProfileConcern[],
): ProfileConcern[] {
  const files = changedFiles.map(normalize);
  if (files.length === 0) return [];
  return concerns.filter((concern) => {
    const patterns = concern.pathPatterns.map((source) => new RegExp(source));
    return files.some((file) => patterns.some((pattern) => pattern.test(file)));
  });
}

// ---------------------------------------------------------------------------
// Planning the review
// ---------------------------------------------------------------------------

// One reviewer per concern, rather than one reviewer carrying every checklist.
// Three concerns in one prompt is a shallower review of each than three prompts
// with one concern apiece, and the whole point of naming a concern is that
// someone looks at the diff through it specifically.
export interface ReviewPass {
  /** Short, unique, and part of the agent name — see MAX_SLUG_LENGTH. */
  slug: string;
  id: string;
  title: string;
  /** The complete checklist text for this one pass. */
  sections: string;
  builtin: boolean;
  /** Changed files this concern's vocabulary actually matched. */
  matchCount: number;
}

export interface SkippedPass {
  id: string;
  title: string;
  matchCount: number;
}

export interface ReviewPlan {
  passes: ReviewPass[];
  skipped: SkippedPass[];
}

// `herdr agent start` names are truncated to 32 characters, and two reviewers
// truncating to the same name would have the second resume the first's session,
// carrying its context — which defeats the point of separate passes.
const MAX_SLUG_LENGTH = 12;

export const DEFAULT_MAX_REVIEW_PASSES = 5;

export function resolveMaxReviewPasses(
  env: NodeJS.ProcessEnv = process.env,
): number {
  const raw = env.AGENT_FLOW_MAX_REVIEW_PASSES;
  if (raw === undefined || raw.trim() === "") return DEFAULT_MAX_REVIEW_PASSES;
  const parsed = Number(raw);
  if (!Number.isInteger(parsed) || parsed < 1 || parsed > 10) {
    console.log(
      `\x1b[33m[Specialists]\x1b[0m Ignoring AGENT_FLOW_MAX_REVIEW_PASSES='${raw}'; it must be a whole number from 1 to 10.`,
    );
    return DEFAULT_MAX_REVIEW_PASSES;
  }
  return parsed;
}

const BUILTIN_PATTERNS: Record<string, RegExp[]> = {
  "security-audit": SECURITY_PATTERNS,
  "database-review": DATABASE_PATTERNS,
  "api-contract-review": API_CONTRACT_PATTERNS,
};

const BUILTIN_TITLES: Record<string, string> = {
  "security-audit": "security",
  "database-review": "database and migrations",
  "api-contract-review": "interface contract",
};

function countMatches(files: string[], patterns: RegExp[]): number {
  return files.filter((file) => matchesAny(file, patterns)).length;
}

function slugify(id: string, taken: Set<string>): string {
  let slug = id.replace(/[^a-z0-9]/g, "").slice(0, MAX_SLUG_LENGTH) || "review";
  if (!taken.has(slug)) {
    taken.add(slug);
    return slug;
  }
  // Collisions are resolved in place rather than by appending, so the slug
  // cannot grow past the length the agent name budget allows.
  for (let suffix = 2; suffix < 100; suffix += 1) {
    const marker = String(suffix);
    const candidate = slug.slice(0, MAX_SLUG_LENGTH - marker.length) + marker;
    if (!taken.has(candidate)) {
      taken.add(candidate);
      return candidate;
    }
  }
  taken.add(slug);
  return slug;
}

export function planReviewPasses(input: {
  changedFiles: string[];
  builtins: string[];
  builtinSections: Record<string, string>;
  concerns: ProfileConcern[];
  maxPasses: number;
}): ReviewPlan {
  const files = input.changedFiles.map(normalize);
  const candidates: Omit<ReviewPass, "slug">[] = [];

  for (const id of input.builtins) {
    const patterns = BUILTIN_PATTERNS[id] ?? [];
    candidates.push({
      id,
      title: BUILTIN_TITLES[id] ?? id,
      sections: input.builtinSections[id] ?? "",
      builtin: true,
      matchCount: countMatches(files, patterns),
    });
  }

  for (const concern of selectProfileConcerns(input.changedFiles, input.concerns)) {
    const patterns = concern.pathPatterns.map((source) => new RegExp(source));
    const body = `## Concern: ${concern.title}\n\n${concern.checklist
      .map((item) => `- ${item}`)
      .join("\n")}\n`;
    // A concern that extends a built-in is reviewed with that built-in's
    // checklist too, in the same pass, rather than as a second opinion on it.
    const augmented = concern.augments
      ? candidates.find((candidate) => candidate.id === concern.augments)
      : undefined;
    if (augmented) {
      augmented.sections = `${augmented.sections}\n${body}`;
      augmented.matchCount += countMatches(files, patterns);
      continue;
    }
    candidates.push({
      id: concern.id,
      title: concern.title,
      sections: concern.augments
        ? `${input.builtinSections[concern.augments] ?? ""}\n${body}`
        : body,
      builtin: false,
      matchCount: countMatches(files, patterns),
    });
  }

  candidates.sort(
    (a, b) =>
      b.matchCount - a.matchCount ||
      Number(b.builtin) - Number(a.builtin) ||
      a.id.localeCompare(b.id),
  );

  // Built-ins are counted against the cap but never dropped by it. Ranking on
  // matched-file count systematically under-ranks a small, severe diff: one file
  // that drops a column matches once, while a broad rename in an auth-adjacent
  // directory matches a dozen times. Losing a security audit because a token
  // refresh touched eight stylesheets is the wrong way to be wrong.
  // Built-ins reserve their slots before anything competes for the rest, so a
  // high-count authored concern cannot take the slot a built-in would have had.
  const builtins = candidates.filter((candidate) => candidate.builtin);
  const authored = candidates.filter((candidate) => !candidate.builtin);
  const remaining = Math.max(0, input.maxPasses - builtins.length);
  const kept = new Set([...builtins, ...authored.slice(0, remaining)]);

  const passes = candidates.filter((candidate) => kept.has(candidate));
  const skipped: SkippedPass[] = authored
    .slice(remaining)
    .map((candidate) => ({
      id: candidate.id,
      title: candidate.title,
      matchCount: candidate.matchCount,
    }));

  const taken = new Set<string>();
  return {
    passes: passes.map((pass) => ({ ...pass, slug: slugify(pass.id, taken) })),
    skipped,
  };
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
