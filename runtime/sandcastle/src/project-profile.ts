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
export const PROJECT_PROFILE_VERSION = 3;

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

// The command that prepares a fresh checkout carries no 'verified' and no
// 'evidence', and this is not tidiness. Both are read of a test command alone:
// `resolveVerificationTask` lets a test command that is known to work displace a
// built-in one, and logs the evidence of one that did not run. A setup command
// is asked of nobody — the profiler is told to run the test command once, not the
// setup command — so asking it to certify the setup command asked for a claim
// about a run that never happened, and Grove's first profile came back with both
// fields absent from every setup command, which the validator then refused.
export interface ProfileSetupCommand {
  /** One executable. Never a shell line: these are spawned without a shell. */
  command: string;
  args: string[];
  /**
   * Relative to the project root. When this path exists the setup command is
   * skipped, so an ordinary run pays nothing: node_modules, vendor, .venv.
   */
  skipWhenPresent?: string;
}

// A framework is what a project is actually built with, as opposed to what
// language it is written in. A React app and a plain Node service share a
// language and need different specialists; naming the framework is what lets the
// personas and the review concerns be about the code that is really there.
export interface ProfileFramework {
  /** "React", "Alembic", "Tailwind CSS". */
  name: string;
  /** What it does in this repository, in a clause. */
  role: string;
  /** The dependency or file that proves it is here. */
  evidence: string;
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
  /** The frameworks this project is built with. May be empty. */
  frameworks: ProfileFramework[];
  /** The specialist for each stage, rendered as its prompt's opening lines. */
  personas: Record<RoleKey, string>;
  test: ProfileCommand;
  setup?: ProfileSetupCommand;
}

// A surface is a body of work with no manifest of its own: SQL migrations, a
// stylesheet tree, infrastructure definitions. It has no test command of its own,
// so all it has to say is whose tests cover it.
//
// That is the whole of it. An earlier shape also asked for a label, a purpose and
// six personas, and nothing ever read them: `personaFor` only ever consulted a
// project's personas, so the prompt engineer paid for six specialists per surface
// and a hard validation failure if one came back too short, in exchange for text
// no code path could reach. What is left here is what verification actually
// consumes.
export interface ProfileSurface {
  /** Directory owning the surface, relative to the repository root. */
  root: string;
  /**
   * The root of the project whose tests cover changes here. Without it a change
   * confined to this surface has to fall back to validating every project.
   */
  validatedBy?: string;
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
  surfaces: ProfileSurface[];
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

// The maps whose entries are package names, as opposed to `scripts` and
// `workspaces`, whose entries are keyed by anything the manifest chooses.
const PACKAGE_JSON_DEPENDENCY_MAPS = [
  "dependencies",
  "devDependencies",
  "peerDependencies",
];

// Type definitions describe types to the compiler; no code the project builds,
// runs or reviews comes out of them. `@types/node` moves a patch every few days,
// and without this one bump marks the whole monorepo stale and summons the prompt
// engineer to rewrite every specialist for a change that cannot make one of them
// wrong. Dependencies are also sorted below, so reordering them is a reformat
// rather than a change of substance.
function significantDependencies(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  const entries = Object.entries(value as Record<string, unknown>)
    .filter(([name]) => !name.startsWith("@types/"))
    .sort(([a], [b]) => a.localeCompare(b));
  return Object.fromEntries(entries);
}

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
      if (!(key in parsed)) continue;
      significant[key] = PACKAGE_JSON_DEPENDENCY_MAPS.includes(key)
        ? significantDependencies(parsed[key])
        : parsed[key];
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
  surfaces: { root: string; label: string }[] = [],
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
  // A surface appearing, vanishing or changing kind makes the specialists wrong
  // in the same way a new project does, so it belongs in the same signal.
  const surfaceShape = [...surfaces]
    .sort((a, b) => a.root.localeCompare(b.root))
    .map((surface) => ({ root: surface.root, label: surface.label }));
  return crypto
    .createHash("sha256")
    .update(
      JSON.stringify({
        version: PROJECT_PROFILE_VERSION,
        shape,
        surfaces: surfaceShape,
      }),
    )
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

// An optional field the agent was told to omit comes back as `null` about as
// often as it is left out, because a JSON template with every key filled in and a
// null where the value is unknown is the shape models reach for first. `null` and
// absent mean the same thing, so both do.
//
// The cost of getting this wrong is not one failed run. A draft the validator
// refuses is never written to the profile, so the profile stays stale, so the
// next workflow invokes the prompt engineer again and draws the same rejection —
// a full agent run burned per workflow, for ever, on a field that was blank.
// Refusing a genuinely wrong value is still right; only the blank is forgiven.
function isAbsent(value: unknown): value is null | undefined {
  return value === undefined || value === null;
}

// Commands are handed to spawnSync with no shell, so a shell line does not fail
// as a shell line — the whole string is looked up as a program name and the error
// is incomprehensible. Rejecting the metacharacters here turns that into a
// profiling failure with a sentence that says what to do.
const COMMAND_PATTERN = /^[A-Za-z0-9._\/\\-]+$/;
const MIN_PERSONA_LENGTH = 40;
const MAX_PERSONA_LENGTH = 6000;
const MAX_PATTERN_LENGTH = 200;
const MAX_PATH_PATTERNS = 12;
const MAX_CHECKLIST_ITEMS = 12;
const MAX_CHECKLIST_ITEM_LENGTH = 400;
const MAX_CONCERNS = 24;
const MAX_FRAMEWORKS = 24;
const MAX_PROJECTS = 24;
const MAX_SURFACES = 24;

// A quantifier applied to a group that itself contains one — (a+)+, (x*)* — is
// the shape that makes a regular expression take exponential time. These
// patterns are authored by an agent and then run against every changed file on
// every workflow, and a JavaScript regex cannot be interrupted once it starts:
// a single bad pattern stalls the run for ever with no error to report. One
// 41-character path against /^(a+)+$/ never returns.
const NESTED_QUANTIFIER = /\([^()]*[*+}][^()]*\)\s*[*+{]/;

function assertUsablePattern(pattern: string, where: string): void {
  if (pattern.length > MAX_PATTERN_LENGTH) {
    fail(`${where} has a path pattern longer than ${MAX_PATTERN_LENGTH} characters.`);
  }
  if (NESTED_QUANTIFIER.test(pattern)) {
    fail(
      `${where} has a path pattern that can take exponential time to match: ${pattern}. ` +
        "A repeated group that already contains a repetition cannot be run safely; " +
        "match the path segments directly instead.",
    );
  }
  try {
    new RegExp(pattern);
  } catch {
    fail(
      `${where} has a path pattern that is not a valid regular expression: ${pattern}`,
    );
  }
}

function fail(message: string): never {
  throw new Error(message);
}

function readCommand(value: unknown, where: string): ProfileCommand {
  const { command, args } = readExecutable(value, where);
  const candidate = value as Partial<ProfileCommand>;
  // A command nobody claimed to have run is not a missing answer, it is an
  // answer: it reads as unverified, which is the one reading that cannot act
  // wrongly. `resolveVerificationTask` gives an unverified test command to the
  // built-in that works today, and falls back to the profile's own only where
  // nothing is known — so forgiving the blank cannot run a command as though it
  // were trustworthy. Refusing it could: a draft is only written once it
  // validates, so a refusal leaves the profile stale and the next workflow
  // summons the prompt engineer to draw the same rejection.
  const verified = isAbsent(candidate.verified) ? false : candidate.verified;
  if (typeof verified !== "boolean") {
    fail(`${where} needs verified to say whether the command was run.`);
  }
  if (typeof candidate.evidence !== "string" || !candidate.evidence.trim()) {
    fail(`${where} needs evidence describing what happened when it was run.`);
  }
  return { command, args, verified, evidence: candidate.evidence };
}

// The setup command is the same executable and the same no-shell rule, without
// the two fields that describe a run. Keys the prompt engineer wrote for the
// older shape are read past: `command` and `args` are copied and the rest is
// dropped, so a profile written against the last schema still validates and is
// still usable.
function readSetupCommand(value: unknown, where: string): ProfileSetupCommand {
  const { command, args } = readExecutable(value, where);
  const { skipWhenPresent } = value as Partial<ProfileSetupCommand>;
  if (isAbsent(skipWhenPresent)) {
    return { command, args };
  }
  if (
    typeof skipWhenPresent !== "string" ||
    path.isAbsolute(skipWhenPresent) ||
    skipWhenPresent.includes("..")
  ) {
    fail(`${where} has an invalid skipWhenPresent path.`);
  }
  return { command, args, skipWhenPresent };
}

// One executable and its arguments, checked for both. Shared by the test and
// setup commands, which differ only in what they say about having been run.
function readExecutable(
  value: unknown,
  where: string,
): { command: string; args: string[] } {
  if (!value || typeof value !== "object") {
    fail(`${where} is missing.`);
  }
  const candidate = value as Partial<ProfileCommand>;
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
  return { command: candidate.command, args: candidate.args as string[] };
}

// Every reader below rebuilds its result field by field, so a key the prompt
// engineer writes but nothing here copies is dropped without complaint. Adding a
// field to the schema means adding it in three places: the interface, the
// validation, and the constructed object.
function readPersonas(value: unknown, where: string): Record<RoleKey, string> {
  if (!value || typeof value !== "object") {
    fail(`${where} has no personas.`);
  }
  const resolved = {} as Record<RoleKey, string>;
  for (const role of ROLE_KEYS) {
    const persona = (value as Record<string, unknown>)[role];
    if (typeof persona !== "string" || persona.trim().length < MIN_PERSONA_LENGTH) {
      fail(
        `${where} has no usable '${role}' persona; each one needs at least ${MIN_PERSONA_LENGTH} characters.`,
      );
    }
    if (persona.length > MAX_PERSONA_LENGTH) {
      fail(`${where} has an oversized '${role}' persona.`);
    }
    resolved[role] = persona.trim();
  }
  return resolved;
}

function readRelativeRoot(value: unknown, where: string): string {
  if (typeof value !== "string") {
    fail(`${where} has no root.`);
  }
  if (path.isAbsolute(value) || value.includes("..") || value.startsWith("./")) {
    fail(`${where} root '${value}' must be a plain relative path.`);
  }
  return value;
}

function readFrameworks(value: unknown, where: string): ProfileFramework[] {
  if (value === undefined) return [];
  if (!Array.isArray(value)) {
    fail(`${where} frameworks must be a list.`);
  }
  if (value.length > MAX_FRAMEWORKS) {
    fail(`${where} lists ${value.length} frameworks; at most ${MAX_FRAMEWORKS} are allowed.`);
  }
  return value.map((entry, index) => {
    if (!entry || typeof entry !== "object") {
      fail(`${where} framework ${index} is not an object.`);
    }
    const candidate = entry as Partial<ProfileFramework>;
    for (const field of ["name", "role", "evidence"] as const) {
      if (typeof candidate[field] !== "string" || !candidate[field]!.trim()) {
        fail(`${where} framework ${index} has no ${field}.`);
      }
    }
    return {
      name: candidate.name!.trim(),
      role: candidate.role!.trim(),
      evidence: candidate.evidence!.trim(),
    };
  });
}

function readSurface(value: unknown, index: number): ProfileSurface {
  if (!value || typeof value !== "object") {
    fail(`Profile surface ${index} is not an object.`);
  }
  // A surface written against the older shape brings a label, a purpose and six
  // personas with it. They are read past rather than refused: the profile that
  // carries them is still correct about the only thing that matters, which is
  // who validates this surface, and a run is not degraded to the conservative
  // answer over text nothing consumes.
  const candidate = value as Partial<ProfileSurface>;
  const root = readRelativeRoot(candidate.root, `Profile surface ${index}`);
  if (!root) {
    fail(`Profile surface ${index} has an empty root.`);
  }
  const surface: ProfileSurface = { root };
  if (!isAbsent(candidate.validatedBy)) {
    if (typeof candidate.validatedBy !== "string") {
      fail(`Profile surface '${root}' has an invalid validatedBy.`);
    }
    surface.validatedBy = candidate.validatedBy;
  }
  return surface;
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
  const project: ProfileProject = {
    root,
    marker: candidate.marker,
    label: candidate.label,
    ecosystem: candidate.ecosystem,
    frameworks: readFrameworks(
      candidate.frameworks,
      `Profile project '${root}'`,
    ),
    personas: readPersonas(candidate.personas, `Profile project '${root}'`),
    test: readCommand(candidate.test, `Profile project '${root}' test command`),
  };
  if (!isAbsent(candidate.setup)) {
    project.setup = readSetupCommand(
      candidate.setup,
      `Profile project '${root}' setup command`,
    );
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
  if (candidate.pathPatterns.length > MAX_PATH_PATTERNS) {
    fail(
      `Profile concern '${candidate.id}' lists ${candidate.pathPatterns.length} path patterns; at most ${MAX_PATH_PATTERNS} are allowed.`,
    );
  }
  for (const pattern of candidate.pathPatterns) {
    if (typeof pattern !== "string") {
      fail(`Profile concern '${candidate.id}' has an unusable path pattern.`);
    }
    assertUsablePattern(pattern, `Profile concern '${candidate.id}'`);
  }
  if (
    !Array.isArray(candidate.checklist) ||
    candidate.checklist.length === 0 ||
    candidate.checklist.some((item) => typeof item !== "string" || !item.trim())
  ) {
    fail(`Profile concern '${candidate.id}' has an empty checklist.`);
  }
  // Every checklist item is rendered into a reviewer's prompt, so an unbounded
  // list is an unbounded prompt.
  if (candidate.checklist.length > MAX_CHECKLIST_ITEMS) {
    fail(
      `Profile concern '${candidate.id}' has ${candidate.checklist.length} checklist items; at most ${MAX_CHECKLIST_ITEMS} are allowed.`,
    );
  }
  if (
    candidate.checklist.some(
      (item) => (item as string).length > MAX_CHECKLIST_ITEM_LENGTH,
    )
  ) {
    fail(
      `Profile concern '${candidate.id}' has a checklist item longer than ${MAX_CHECKLIST_ITEM_LENGTH} characters.`,
    );
  }
  const concern: ProfileConcern = {
    id: candidate.id,
    title: candidate.title,
    pathPatterns: candidate.pathPatterns as string[],
    checklist: candidate.checklist as string[],
  };
  if (!isAbsent(candidate.augments)) {
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
//
// `detectedSurfaces` is required rather than defaulted, because the whole point of
// passing it is the check that every one of them was described. A caller that
// quietly omitted the argument would be running without the guard while looking
// like it ran with it.
export function readProfileDraft(
  draftPath: string,
  detected: StackProject[],
  detectedSurfaces: { root: string; label: string }[],
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
  if (candidate.projects.length > MAX_PROJECTS) {
    throw new Error(
      `The project profile lists ${candidate.projects.length} projects; at most ${MAX_PROJECTS} are allowed.`,
    );
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

  if (Array.isArray(candidate.surfaces) && candidate.surfaces.length > MAX_SURFACES) {
    throw new Error(
      `The project profile lists ${candidate.surfaces.length} surfaces; at most ${MAX_SURFACES} are allowed.`,
    );
  }
  const surfaces = Array.isArray(candidate.surfaces)
    ? candidate.surfaces.map(readSurface)
    : [];
  const surfaceRoots = new Set<string>();
  for (const surface of surfaces) {
    if (surfaceRoots.has(surface.root)) {
      throw new Error(`The project profile has two surfaces for '${surface.root}'.`);
    }
    surfaceRoots.add(surface.root);
    // A directory cannot be both, or a change there would be attributed twice
    // and validated by whichever rule happened to run first.
    if (seen.has(surface.root)) {
      throw new Error(
        `Profile surface '${surface.root}' is also a project in this repository.`,
      );
    }
    // A surface validated by a project that does not exist would silently fall
    // back to validating everything, which is the behaviour surfaces exist to fix.
    if (!isAbsent(surface.validatedBy) && !seen.has(surface.validatedBy)) {
      throw new Error(
        `Profile surface '${surface.root}' says it is validated by '${surface.validatedBy}', which is not a project in this repository.`,
      );
    }
  }
  // A surface the detector found and the profile left out is given the project
  // that owns it, and the answer is derived rather than refused. Refusing it
  // looked strict and was the opposite: a draft is only written once it
  // validates, so a refusal left the profile unwritten, the next workflow
  // summoned the prompt engineer, and it drew the same rejection — no workflow
  // on that repository could start at all. The first profile written for Grove
  // came back with `"surfaces": null` and one real surface,
  // `internal/data/migrations`, waiting behind it.
  //
  // The derived owner is also the right one far more often than it is a guess: a
  // surface inside a project is covered by that project's tests, since the
  // detector will not report a directory a project owns. The prompt engineer is
  // asked to name the owner because the exception is real — a top-level
  // `migrations/` directory whose coverage comes from a service's suite — and its
  // answer overrides this. A surface no project owns is left with no owner, which
  // validates every project: conservative, never wrong, and named in the log.
  const detectedSurfaceRoots = new Set(detectedSurfaces.map((surface) => surface.root));
  for (const surface of surfaces) {
    if (!detectedSurfaceRoots.has(surface.root)) {
      throw new Error(
        `The project profile describes a surface at '${surface.root}', which is not a surface in this repository.`,
      );
    }
  }
  for (const detectedSurface of detectedSurfaces) {
    if (surfaceRoots.has(detectedSurface.root)) continue;
    const owner = deepestProjectFor(detected, detectedSurface.root);
    const surface: ProfileSurface = { root: detectedSurface.root };
    if (owner) surface.validatedBy = owner.root;
    surfaces.push(surface);
    surfaceRoots.add(surface.root);
  }

  if (Array.isArray(candidate.concerns) && candidate.concerns.length > MAX_CONCERNS) {
    throw new Error(
      `The project profile lists ${candidate.concerns.length} concerns; at most ${MAX_CONCERNS} are allowed.`,
    );
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

  return { repoSummary: candidate.repoSummary, projects, surfaces, concerns };
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
  surfaces: { root: string; label: string }[] = [],
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
    // A profile written against an older schema is not held to a requirement
    // that postdates it. It is regenerated on this very run — the caller's
    // freshness test sends an older version to the prompt engineer — so
    // refusing it over a field its schema never had would throw away a usable
    // set of personas and degrade the run in progress to the built-ins, which
    // is exactly what the version check below exists to prevent.
    draft = readProfileDraft(
      profilePath,
      projects,
      candidate.version < PROJECT_PROFILE_VERSION ? [] : surfaces,
    );
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
  const current = fingerprintRepoShape(targetDir, projects, surfaces);
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

// The project a repository-relative path belongs to: the deepest root that
// contains it, so a subdirectory is never attributed to the project above it. A
// path outside every project belongs to none, which is a different answer from
// the repository root and the only case where nothing owns a path at all.
function deepestProjectFor(
  projects: StackProject[],
  relativePath: string,
): StackProject | undefined {
  const normalized = relativePath.replace(/\\/g, "/");
  let owner: StackProject | undefined;
  for (const project of projects) {
    const matches = project.root === "" || normalized.startsWith(`${project.root}/`);
    if (matches && (!owner || project.root.length > owner.root.length)) {
      owner = project;
    }
  }
  return owner;
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
  const affected = new Set<StackProject>();
  for (const file of scopeFiles) {
    const owner = deepestProjectFor(projects, file);
    if (owner) affected.add(owner);
  }
  return affected.size > 0
    ? projects.filter((project) => affected.has(project))
    : projects;
}
