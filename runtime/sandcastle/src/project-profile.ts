import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { stackLabel, type StackProject } from "./stack-detector.js";

// The profile records what a repository is and how to work in it, so the
// specialists a workflow runs are written for that repository rather than chosen
// from a fixed table of four languages.
//
// It lives inside the git common directory, never in the repository's tracked
// tree: sandcastle leaves no footprint in the projects it is pointed at, and an
// untracked file can be rewritten mid-run without dirtying a worktree and
// tripping the workflow's own clean-checkout guards.
export const PROJECT_PROFILE_VERSION = 1;

// One specialist per stage of the workflow. Grouping the fifteen prompt builders
// onto six keys keeps the generated text tractable while still giving every step
// a specialist that knows the code it is about to touch.
export const ROLE_KEYS = [
  "planning",
  "tests",
  "implementation",
  "verification",
  "review",
  "documentation",
] as const;

export type RoleKey = (typeof ROLE_KEYS)[number];

export interface ProfileCommand {
  /** One executable. Never a shell line: these are spawned without a shell. */
  command: string;
  args: string[];
  /** The profiler ran this command in this project and it reported. */
  verified: boolean;
  /** What happened when it ran, or why it could not be run. */
  evidence: string;
}

export interface ProfileSetupCommand extends ProfileCommand {
  /**
   * Relative to the project root. When this path exists the setup command is
   * skipped, so an ordinary run pays nothing: node_modules, vendor, .venv.
   */
  skipWhenPresent?: string;
}

export interface ProfileProject {
  /** Directory owning the project, relative to the repository root ("" = root). */
  root: string;
  /** The marker file that identified it, as the detector reports it. */
  marker: string;
  /** What the project calls itself: "RUST", "TYPESCRIPT". */
  label: string;
  /** One sentence a person would recognise the project by. */
  ecosystem: string;
  /** The specialist for each stage, rendered as its prompt's opening lines. */
  personas: Record<RoleKey, string>;
  test: ProfileCommand;
  setup?: ProfileSetupCommand;
}

export interface ProfileConcern {
  id: string;
  title: string;
  /**
   * When set, this concern's paths and checklist extend a built-in domain review
   * section instead of forming a domain of its own, so an Ecto or Rails
   * repository reaches `database-review` that its own vocabulary never matched.
   */
  augments?: string;
  /** Regex sources, matched against lowercased forward-slashed relative paths. */
  pathPatterns: string[];
  checklist: string[];
}

export interface RepoProfile {
  version: number;
  generatedAt: string;
  fingerprint: string;
  /** One to three sentences describing the repository as a whole. */
  repoSummary: string;
  projects: ProfileProject[];
  concerns: ProfileConcern[];
}

/**
 * What the prompt engineer writes. `version`, `generatedAt` and `fingerprint` are
 * stamped by this module rather than by the agent: an agent inventing a
 * fingerprint would break staleness detection silently, and staleness is
 * something only code is in a position to assert.
 */
export type RepoProfileDraft = Omit<
  RepoProfile,
  "version" | "generatedAt" | "fingerprint"
>;

export type ProfileStatus = "fresh" | "stale" | "absent" | "invalid";

export interface LoadedProfile {
  profile: RepoProfile | null;
  status: ProfileStatus;
  /** A sentence explaining the status, for the log line. */
  reason: string;
}

// ---------------------------------------------------------------------------
// Fingerprinting
// ---------------------------------------------------------------------------

// package.json changes constantly for reasons that do not change how a project is
// built, reviewed or tested. Hashing only the parts that do keeps a version bump
// or a reformat from marking a whole monorepo stale, while a new test script or a
// new framework still does.
const PACKAGE_JSON_SIGNIFICANT = [
  "scripts",
  "dependencies",
  "devDependencies",
  "peerDependencies",
  "workspaces",
  "packageManager",
];

function manifestDigestInput(absoluteMarkerPath: string): string {
  let raw: string;
  try {
    raw = fs.readFileSync(absoluteMarkerPath, "utf8");
  } catch {
    // A permissions blip must not flap the fingerprint and trigger a rebuild of
    // every specialist, so an unreadable manifest hashes to a constant.
    return "<unreadable>";
  }
  if (path.basename(absoluteMarkerPath) !== "package.json") {
    return raw;
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const significant: Record<string, unknown> = {};
    // Built in the fixed order of PACKAGE_JSON_SIGNIFICANT, so the digest does
    // not depend on the manifest's own key order. A replacer array cannot be
    // used here: it filters keys at every level, which would erase the contents
    // of `scripts` and hide the very change this is watching for.
    for (const key of PACKAGE_JSON_SIGNIFICANT) {
      if (key in parsed) significant[key] = parsed[key];
    }
    return JSON.stringify(significant);
  } catch {
    return raw;
  }
}

// fingerprintRepoShape answers "has this repository changed in a way that makes
// its specialists wrong?" — cheap enough to compute on every run, which is what
// lets detection itself be the staleness check.
//
// Lockfiles are deliberately excluded: they are large, they churn on every
// transitive bump, and a lockfile change never changes how a project is tested.
export function fingerprintRepoShape(
  targetDir: string,
  projects: StackProject[],
): string {
  const shape = [...projects]
    .sort((a, b) => a.root.localeCompare(b.root))
    .map((project) => ({
      root: project.root,
      marker: project.marker,
      stack: project.stack,
      label: stackLabel(project),
      manifest: manifestDigestInput(path.join(targetDir, project.marker)),
    }));
  return crypto
    .createHash("sha256")
    .update(JSON.stringify({ version: PROJECT_PROFILE_VERSION, shape }))
    .digest("hex");
}

// ---------------------------------------------------------------------------
// Reading and validating
// ---------------------------------------------------------------------------

// Agents wrap objects in Markdown fences despite being asked not to, and add
// explanatory comments around them. Both are formatting slips rather than failed
// work, and the PR reviewer's verdict reader already forgives them.
function unwrapJson(content: string): string {
  let text = content.trim();
  if (text.startsWith("```")) {
    text = text.replace(/^```[^\n]*\n?/, "");
    const closing = text.lastIndexOf("```");
    if (closing >= 0) text = text.slice(0, closing);
  }
  return text.replace(/<!--[\s\S]*?-->/g, "").trim();
}

// Commands are handed to spawnSync with no shell, so a shell line does not fail
// as a shell line — the whole string is looked up as a program name and the error
// is incomprehensible. Rejecting the metacharacters here turns that into a
// profiling failure with a sentence that says what to do.
const COMMAND_PATTERN = /^[A-Za-z0-9._\/\\-]+$/;
const MIN_PERSONA_LENGTH = 40;
const MAX_PERSONA_LENGTH = 4000;
const MAX_PATTERN_LENGTH = 200;

function fail(message: string): never {
  throw new Error(message);
}

function readCommand(
  value: unknown,
  where: string,
  { setup = false }: { setup?: boolean } = {},
): ProfileCommand {
  if (!value || typeof value !== "object") {
    fail(`${where} is missing.`);
  }
  const candidate = value as Partial<ProfileSetupCommand>;
  if (typeof candidate.command !== "string" || !candidate.command.trim()) {
    fail(`${where} has no command.`);
  }
  if (!COMMAND_PATTERN.test(candidate.command)) {
    fail(
      `${where} is not a single executable: ${JSON.stringify(candidate.command)}. ` +
        "Commands are run without a shell, so operators such as && and | cannot appear in them.",
    );
  }
  if (
    !Array.isArray(candidate.args) ||
    candidate.args.some((arg) => typeof arg !== "string")
  ) {
    fail(`${where} needs args to be an array of strings.`);
  }
  if (typeof candidate.verified !== "boolean") {
    fail(`${where} needs verified to say whether the command was run.`);
  }
  if (typeof candidate.evidence !== "string" || !candidate.evidence.trim()) {
    fail(`${where} needs evidence describing what happened when it was run.`);
  }
  const command: ProfileSetupCommand = {
    command: candidate.command,
    args: candidate.args as string[],
    verified: candidate.verified,
    evidence: candidate.evidence,
  };
  if (setup && candidate.skipWhenPresent !== undefined) {
    if (
      typeof candidate.skipWhenPresent !== "string" ||
      path.isAbsolute(candidate.skipWhenPresent) ||
      candidate.skipWhenPresent.includes("..")
    ) {
      fail(`${where} has an invalid skipWhenPresent path.`);
    }
    command.skipWhenPresent = candidate.skipWhenPresent;
  }
  return command;
}

function readProject(value: unknown, index: number): ProfileProject {
  if (!value || typeof value !== "object") {
    fail(`Profile project ${index} is not an object.`);
  }
  const candidate = value as Partial<ProfileProject>;
  if (typeof candidate.root !== "string") {
    fail(`Profile project ${index} has no root.`);
  }
  const root = candidate.root;
  if (path.isAbsolute(root) || root.includes("..") || root.startsWith("./")) {
    fail(`Profile project '${root}' must be a plain relative path.`);
  }
  if (typeof candidate.marker !== "string" || !candidate.marker.trim()) {
    fail(`Profile project '${root}' has no marker.`);
  }
  if (typeof candidate.label !== "string" || !/^[A-Z][A-Z0-9_]*$/.test(candidate.label)) {
    fail(
      `Profile project '${root}' needs an uppercase label such as RUST or TYPESCRIPT.`,
    );
  }
  if (typeof candidate.ecosystem !== "string" || !candidate.ecosystem.trim()) {
    fail(`Profile project '${root}' has no ecosystem description.`);
  }
  const personas = candidate.personas;
  if (!personas || typeof personas !== "object") {
    fail(`Profile project '${root}' has no personas.`);
  }
  const resolved = {} as Record<RoleKey, string>;
  for (const role of ROLE_KEYS) {
    const persona = (personas as Record<string, unknown>)[role];
    if (typeof persona !== "string" || persona.trim().length < MIN_PERSONA_LENGTH) {
      fail(
        `Profile project '${root}' has no usable '${role}' persona; each one needs at least ${MIN_PERSONA_LENGTH} characters.`,
      );
    }
    if (persona.length > MAX_PERSONA_LENGTH) {
      fail(`Profile project '${root}' has an oversized '${role}' persona.`);
    }
    resolved[role] = persona.trim();
  }
  const project: ProfileProject = {
    root,
    marker: candidate.marker,
    label: candidate.label,
    ecosystem: candidate.ecosystem,
    personas: resolved,
    test: readCommand(candidate.test, `Profile project '${root}' test command`),
  };
  if (candidate.setup !== undefined) {
    project.setup = readCommand(
      candidate.setup,
      `Profile project '${root}' setup command`,
      { setup: true },
    ) as ProfileSetupCommand;
  }
  return project;
}

function readConcern(value: unknown, index: number): ProfileConcern {
  if (!value || typeof value !== "object") {
    fail(`Profile concern ${index} is not an object.`);
  }
  const candidate = value as Partial<ProfileConcern>;
  if (typeof candidate.id !== "string" || !/^[a-z0-9][a-z0-9-]*$/.test(candidate.id)) {
    fail(`Profile concern ${index} needs a lowercase slug id.`);
  }
  if (typeof candidate.title !== "string" || !candidate.title.trim()) {
    fail(`Profile concern '${candidate.id}' has no title.`);
  }
  if (
    !Array.isArray(candidate.pathPatterns) ||
    candidate.pathPatterns.length === 0
  ) {
    fail(`Profile concern '${candidate.id}' lists no path patterns.`);
  }
  for (const pattern of candidate.pathPatterns) {
    if (typeof pattern !== "string" || pattern.length > MAX_PATTERN_LENGTH) {
      fail(`Profile concern '${candidate.id}' has an unusable path pattern.`);
    }
    try {
      new RegExp(pattern);
    } catch {
      fail(
        `Profile concern '${candidate.id}' has a path pattern that is not a valid regular expression: ${pattern}`,
      );
    }
  }
  if (
    !Array.isArray(candidate.checklist) ||
    candidate.checklist.length === 0 ||
    candidate.checklist.some((item) => typeof item !== "string" || !item.trim())
  ) {
    fail(`Profile concern '${candidate.id}' has an empty checklist.`);
  }
  const concern: ProfileConcern = {
    id: candidate.id,
    title: candidate.title,
    pathPatterns: candidate.pathPatterns as string[],
    checklist: candidate.checklist as string[],
  };
  if (candidate.augments !== undefined) {
    if (typeof candidate.augments !== "string" || !candidate.augments.trim()) {
      fail(`Profile concern '${candidate.id}' has an invalid augments value.`);
    }
    concern.augments = candidate.augments;
  }
  return concern;
}

// readProfileDraft validates what the prompt engineer wrote, in the manner of the
// audit verdict reader: forgive formatting, refuse anything that would be acted on
// wrongly, and say in one sentence what is missing.
export function readProfileDraft(
  draftPath: string,
  detected: StackProject[],
): RepoProfileDraft {
  if (!fs.existsSync(draftPath)) {
    throw new Error(`The prompt engineer wrote no profile: ${draftPath}`);
  }
  let content: string;
  try {
    content = fs.readFileSync(draftPath, "utf8");
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to read the project profile: ${detail}`);
  }
  let value: unknown;
  try {
    value = JSON.parse(unwrapJson(content));
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to parse the project profile: ${detail}`);
  }
  const candidate = value as Partial<RepoProfileDraft>;
  if (typeof candidate.repoSummary !== "string" || !candidate.repoSummary.trim()) {
    throw new Error("The project profile has no repoSummary.");
  }
  if (!Array.isArray(candidate.projects) || candidate.projects.length === 0) {
    throw new Error("The project profile lists no projects.");
  }
  const projects = candidate.projects.map(readProject);

  const seen = new Set<string>();
  for (const project of projects) {
    if (seen.has(project.root)) {
      throw new Error(`The project profile has two entries for '${project.root}'.`);
    }
    seen.add(project.root);
  }
  // A profile that describes a project the detector never found, or omits one it
  // did, is describing a different repository than the one being worked in.
  for (const project of detected) {
    if (!seen.has(project.root)) {
      throw new Error(
        `The project profile is missing an entry for '${project.root || "the repository root"}' (${project.marker}).`,
      );
    }
  }
  const detectedRoots = new Set(detected.map((project) => project.root));
  for (const project of projects) {
    if (!detectedRoots.has(project.root)) {
      throw new Error(
        `The project profile describes '${project.root}', which is not a project in this repository.`,
      );
    }
  }

  const concerns = Array.isArray(candidate.concerns)
    ? candidate.concerns.map(readConcern)
    : [];
  const concernIds = new Set<string>();
  for (const concern of concerns) {
    if (concernIds.has(concern.id)) {
      throw new Error(`The project profile has two concerns called '${concern.id}'.`);
    }
    concernIds.add(concern.id);
  }

  return { repoSummary: candidate.repoSummary, projects, concerns };
}

// ---------------------------------------------------------------------------
// Loading and writing
// ---------------------------------------------------------------------------

// A corrupt profile is reported rather than thrown, unlike a corrupt workflow
// checkpoint. A checkpoint drives resume, so ignoring one is unsafe; the profile
// has a complete fallback in the built-in stacks, and throwing would let a single
// bad hand-edit stop every command on the repository.
export function loadProfile(
  profilePath: string,
  targetDir: string,
  projects: StackProject[],
): LoadedProfile {
  if (!fs.existsSync(profilePath)) {
    return { profile: null, status: "absent", reason: "no profile has been written yet" };
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(fs.readFileSync(profilePath, "utf8"));
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    return { profile: null, status: "invalid", reason: `it could not be parsed (${detail})` };
  }
  const candidate = parsed as Partial<RepoProfile>;
  if (typeof candidate.version !== "number") {
    return { profile: null, status: "invalid", reason: "it records no version" };
  }
  if (candidate.version > PROJECT_PROFILE_VERSION) {
    // Written by a newer sandcastle. Overwriting it would let two installed
    // versions take turns rewriting the same file.
    return {
      profile: null,
      status: "invalid",
      reason: `it was written by a newer version of sandcastle (profile version ${candidate.version})`,
    };
  }
  let draft: RepoProfileDraft;
  try {
    draft = readProfileDraft(profilePath, projects);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    return { profile: null, status: "invalid", reason: detail };
  }
  const profile: RepoProfile = {
    version: candidate.version,
    generatedAt: typeof candidate.generatedAt === "string" ? candidate.generatedAt : "",
    fingerprint: typeof candidate.fingerprint === "string" ? candidate.fingerprint : "",
    ...draft,
  };
  if (candidate.version < PROJECT_PROFILE_VERSION) {
    // Usable now, regenerated next: the run in progress is not degraded to the
    // built-ins just because the schema moved.
    return {
      profile,
      status: "stale",
      reason: `it was written for profile version ${candidate.version}`,
    };
  }
  const current = fingerprintRepoShape(targetDir, projects);
  if (profile.fingerprint !== current) {
    return { profile, status: "stale", reason: "the repository's projects have changed" };
  }
  return { profile, status: "fresh", reason: "it matches the repository" };
}

export function writeProfile(
  profilePath: string,
  draft: RepoProfileDraft,
  fingerprint: string,
): RepoProfile {
  const profile: RepoProfile = {
    version: PROJECT_PROFILE_VERSION,
    generatedAt: new Date().toISOString(),
    fingerprint,
    ...draft,
  };
  fs.mkdirSync(path.dirname(profilePath), { recursive: true });
  const temporaryPath = `${profilePath}.${process.pid}.tmp`;
  fs.writeFileSync(temporaryPath, `${JSON.stringify(profile, null, 2)}\n`);
  fs.renameSync(temporaryPath, profilePath);
  return profile;
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

export function profileProjectFor(
  profile: RepoProfile | null,
  root: string,
): ProfileProject | undefined {
  return profile?.projects.find((project) => project.root === root);
}

// projectsInScope narrows a repository's projects to the ones a step is about to
// touch, by the same rule verification already uses to decide what to test: the
// deepest project root owning a file wins, and nothing attributable means every
// project rather than a confident guess at the wrong one.
export function projectsInScope(
  projects: StackProject[],
  scopeFiles: string[],
): StackProject[] {
  if (projects.length === 0) return [];
  const ownerOf = (file: string): StackProject | undefined => {
    const normalized = file.replace(/\\/g, "/");
    let owner: StackProject | undefined;
    for (const project of projects) {
      const matches =
        project.root === "" || normalized.startsWith(`${project.root}/`);
      if (matches && (!owner || project.root.length > owner.root.length)) {
        owner = project;
      }
    }
    return owner;
  };
  const affected = new Set<StackProject>();
  for (const file of scopeFiles) {
    const owner = ownerOf(file);
    if (owner) affected.add(owner);
  }
  return affected.size > 0
    ? projects.filter((project) => affected.has(project))
    : projects;
}
