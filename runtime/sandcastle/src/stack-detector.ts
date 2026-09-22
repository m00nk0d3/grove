import fs from "fs";
import path from "path";

export type KnownTechStack = "GO" | "PYTHON" | "TYPESCRIPT" | "CSHARP";

// UNKNOWN is a project this runtime recognises as a project without knowing how
// to build or review it. Keeping the union closed is deliberate: it is what makes
// the persona table and the verification switch exhaustive, so the compiler names
// every site that has to decide what an unprofiled repository does.
export type TechStack = KnownTechStack | "UNKNOWN";

export function isKnownStack(stack: TechStack): stack is KnownTechStack {
  return stack !== "UNKNOWN";
}

// A repository can hold more than one stack — a .NET solution under backend/
// beside a TypeScript app under frontend/ is ordinary. Each one is recorded
// with the directory that owns it so validation runs where the project lives
// rather than at the repository root.
export interface StackProject {
  stack: TechStack;
  /** Directory owning the project, relative to the repository root ("" = root). */
  root: string;
  /** The file that identifies the project, relative to the repository root. */
  marker: string;
  /**
   * What the project calls itself, for display and for profile lookup: "RUST",
   * "JAVA". Absent for the built-in stacks, where it equals `stack`.
   */
  label?: string;
}

// stackLabel is what a human and a generated profile see. It separates "how this
// runtime treats the project" (stack) from "what the project is" (label), so a
// repository holding both Rust and Ruby is not two indistinguishable UNKNOWNs.
export function stackLabel(project: StackProject): string {
  return project.label ?? project.stack;
}

// Projects are commonly one level below the root (backend/, frontend/, src/),
// and occasionally two (apps/web/). Going deeper mostly finds vendored copies.
const PROJECT_SEARCH_DEPTH = 2;

const IGNORED_DIRECTORIES = new Set([
  ".git",
  "node_modules",
  "bin",
  "obj",
  "dist",
  "build",
  "out",
  "target",
  ".venv",
  "venv",
  "vendor",
  "testdata",
  "fixtures",
]);

function isSolutionFile(name: string): boolean {
  return name.endsWith(".sln") || name.endsWith(".slnx");
}

function isProjectFile(name: string): boolean {
  return name.endsWith(".csproj");
}

// Markers for projects this runtime knows how to build and review itself. These
// are consulted first, so a Python project that also keeps a Makefile is still a
// Python project.
//
// Deliberately excluded: Makefile, Dockerfile and *.nix. They appear beside real
// projects as often as they identify one, and a false project root stops the walk
// descending into the directory that holds the actual project.
const UNKNOWN_MARKERS: Array<[string, string]> = [
  ["Cargo.toml", "RUST"],
  ["pom.xml", "JAVA"],
  ["build.gradle", "JAVA"],
  ["build.gradle.kts", "JAVA"],
  ["build.sbt", "SCALA"],
  ["Gemfile", "RUBY"],
  ["composer.json", "PHP"],
  ["mix.exs", "ELIXIR"],
  ["deno.json", "DENO"],
  ["deno.jsonc", "DENO"],
  ["Package.swift", "SWIFT"],
  ["pubspec.yaml", "DART"],
  ["stack.yaml", "HASKELL"],
  ["CMakeLists.txt", "CMAKE"],
];

// Ordered by specificity: a directory is attributed to the first stack whose
// marker it contains, so a .NET project that also carries a package.json for
// tooling is still a .NET project.
function markerFor(
  files: string[],
): { stack: TechStack; marker: string; label?: string } | undefined {
  if (files.includes("go.mod")) {
    return { stack: "GO", marker: "go.mod" };
  }
  for (const candidate of ["pyproject.toml", "requirements.txt"]) {
    if (files.includes(candidate)) {
      return { stack: "PYTHON", marker: candidate };
    }
  }
  const solution = files.find(isSolutionFile);
  if (solution) {
    return { stack: "CSHARP", marker: solution };
  }
  const project = files.find(isProjectFile);
  if (project) {
    return { stack: "CSHARP", marker: project };
  }
  if (files.includes("package.json")) {
    return { stack: "TYPESCRIPT", marker: "package.json" };
  }
  // A project in a language this runtime has no built-in support for is still a
  // project. Reporting it is what lets the profile supply its persona and its
  // commands instead of the repository being mistaken for a TypeScript one.
  for (const [marker, label] of UNKNOWN_MARKERS) {
    if (files.includes(marker)) {
      return { stack: "UNKNOWN", marker, label };
    }
  }
  return undefined;
}

function toPosix(value: string): string {
  return value.split(path.sep).join("/");
}

// detectStackProjects reports every stack the repository contains, outermost
// first. A directory that owns a project is not descended into, so a solution's
// individual projects and a workspace's packages do not each become entries.
export function detectStackProjects(
  targetDir: string,
  depth: number = PROJECT_SEARCH_DEPTH,
): StackProject[] {
  const found: StackProject[] = [];

  const walk = (dir: string, remaining: number): void => {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return; // unreadable directory: treat as no evidence
    }
    const files = entries.filter((entry) => entry.isFile()).map((e) => e.name);
    const marker = markerFor(files);
    if (marker) {
      const root = toPosix(path.relative(targetDir, dir));
      found.push({
        stack: marker.stack,
        root,
        marker: root ? `${root}/${marker.marker}` : marker.marker,
        ...(marker.label ? { label: marker.label } : {}),
      });
      // A nested project owns its subtree: a solution's individual projects and
      // a workspace's packages must not each become entries of their own.
      //
      // The repository root is the exception. A manifest there is very often
      // tooling — formatters, hooks, workspace declarations — sitting above the
      // real projects, and letting it own everything made a backend added later
      // invisible: no specialist, no test command, and no staleness when it
      // appeared.
      if (root !== "") return;
    }
    if (remaining <= 0) {
      return;
    }
    for (const entry of entries) {
      if (
        !entry.isDirectory() ||
        entry.name.startsWith(".") ||
        IGNORED_DIRECTORIES.has(entry.name)
      ) {
        continue;
      }
      walk(path.join(dir, entry.name), remaining - 1);
    }
  };

  walk(targetDir, depth);
  return found;
}

// detectStack reports the repository's primary stack. Multi-stack repositories
// should prefer detectStackProjects; this remains for callers that need a single
// label. A repository with no marker at all keeps the historical TypeScript
// answer, because that value only feeds agent role names and one log line; a
// repository whose projects are all unrecognised reports UNKNOWN, which is what
// routes it to its profile instead of to npm.
export function detectStack(targetDir: string): TechStack {
  const projects = detectStackProjects(targetDir);
  if (projects.length === 0) {
    return "TYPESCRIPT";
  }
  const priority: TechStack[] = [
    "GO",
    "PYTHON",
    "CSHARP",
    "TYPESCRIPT",
    "UNKNOWN",
  ];
  for (const stack of priority) {
    if (projects.some((project) => project.stack === stack)) {
      return stack;
    }
  }
  return "TYPESCRIPT";
}

// findDotNetProject returns the solution or project to build, relative to
// targetDir, or undefined when the directory holds no .NET project.
export function findDotNetProject(targetDir: string): string | undefined {
  return detectStackProjects(targetDir).find(
    (project) => project.stack === "CSHARP",
  )?.marker;
}

// ---------------------------------------------------------------------------
// Dependency evidence
// ---------------------------------------------------------------------------

// The names a project depends on directly. These are evidence, not conclusions:
// a curated table mapping "react" to a React specialist would rebuild the closed
// set this runtime exists to escape. Detection supplies the facts and the prompt
// engineer decides what they mean for this repository.
//
// Every reader is best-effort. A manifest this cannot parse yields nothing rather
// than failing a run, because the evidence is a bonus and never a prerequisite.
export function readDirectDependencies(absoluteMarkerPath: string): string[] {
  let raw: string;
  try {
    raw = fs.readFileSync(absoluteMarkerPath, "utf8");
  } catch {
    return [];
  }
  // A UTF-8 byte order mark is legal in these files and makes JSON.parse throw,
  // which would silently cost every framework fact the manifest holds.
  raw = raw.replace(/^﻿/, "");
  const name = path.basename(absoluteMarkerPath);
  try {
    if (name === "package.json") return packageJsonDependencies(raw);
    if (name === "requirements.txt") return requirementsDependencies(raw);
    if (name === "pyproject.toml") return pyprojectDependencies(raw);
    if (name.endsWith(".csproj")) return csprojDependencies(raw);
    if (name === "Cargo.toml") return tomlSectionKeys(raw, "dependencies");
    if (name === "composer.json") return packageJsonDependencies(raw);
    if (name === "Gemfile") return gemfileDependencies(raw);
  } catch {
    return [];
  }
  return [];
}

function unique(names: string[]): string[] {
  return [...new Set(names.map((entry) => entry.trim()).filter(Boolean))];
}

function packageJsonDependencies(raw: string): string[] {
  const parsed = JSON.parse(raw) as Record<string, unknown>;
  const names: string[] = [];
  for (const key of ["dependencies", "devDependencies", "peerDependencies", "require", "require-dev"]) {
    const section = parsed[key];
    if (section && typeof section === "object") {
      names.push(...Object.keys(section as Record<string, unknown>));
    }
  }
  return unique(names);
}

function requirementsDependencies(raw: string): string[] {
  return unique(
    raw
      .split("\n")
      .map((line) => line.split("#")[0].trim())
      .filter((line) => line && !line.startsWith("-"))
      // Strip whatever version specifier, extra or marker follows the name.
      .map((line) => line.split(/[<>=!~;[\s]/)[0]),
  );
}

// A line scan rather than a TOML parser: the two shapes that name dependencies
// are unambiguous, and a dependency list is not worth a parser dependency.
function pyprojectDependencies(raw: string): string[] {
  const names: string[] = [];
  const arrayMatch = raw.match(/^\s*dependencies\s*=\s*\[([\s\S]*?)\]/m);
  if (arrayMatch) {
    for (const entry of arrayMatch[1].split(",")) {
      const quoted = entry.match(/["']([^"']+)["']/);
      if (quoted) names.push(quoted[1].split(/[<>=!~;[\s]/)[0]);
    }
  }
  names.push(...tomlSectionKeys(raw, "tool.poetry.dependencies"));
  names.push(...tomlSectionKeys(raw, "project.optional-dependencies"));
  return unique(names.filter((entry) => entry.toLowerCase() !== "python"));
}

// The keys of one [section] of a TOML file, up to the next section header.
function tomlSectionKeys(raw: string, section: string): string[] {
  const lines = raw.split("\n");
  const header = `[${section}]`;
  const names: string[] = [];
  let inside = false;
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed.startsWith("[")) {
      inside = trimmed === header;
      continue;
    }
    if (!inside || !trimmed || trimmed.startsWith("#")) continue;
    const key = trimmed.split("=")[0].trim().replace(/^["']|["']$/g, "");
    if (key) names.push(key);
  }
  return unique(names);
}

function csprojDependencies(raw: string): string[] {
  return unique(
    [...raw.matchAll(/PackageReference\s+Include\s*=\s*"([^"]+)"/g)].map(
      (match) => match[1],
    ),
  );
}

function gemfileDependencies(raw: string): string[] {
  return unique(
    [...raw.matchAll(/^\s*gem\s+["']([^"']+)["']/gm)].map((match) => match[1]),
  );
}

// ---------------------------------------------------------------------------
// Surfaces
// ---------------------------------------------------------------------------

// A surface is a directory that owns no manifest but has an unmistakable job:
// SQL migrations, a stylesheet tree, infrastructure definitions. Without this a
// change confined to one of them belongs to no project, and validation falls back
// to running every suite in the repository rather than the one that covers it.
export interface DetectedSurface {
  /** Directory owning the surface, relative to the repository root. */
  root: string;
  /** What the surface is: "SQL", "STYLES", "INFRASTRUCTURE". */
  label: string;
  /** Why it was recognised, for the log and for the prompt engineer. */
  evidence: string;
}

// Recognised by what the files are, never by what the directory is called: a
// folder named "database" holding TypeScript is not a SQL surface, and a folder
// named "bits" holding nothing but migrations is.
const SURFACE_KINDS: Array<{ label: string; extensions: string[] }> = [
  { label: "SQL", extensions: [".sql"] },
  { label: "STYLES", extensions: [".css", ".scss", ".sass", ".less", ".styl"] },
  { label: "INFRASTRUCTURE", extensions: [".tf", ".tfvars", ".bicep"] },
  { label: "SCHEMA", extensions: [".proto", ".graphql", ".gql", ".avsc"] },
];

// Enough files to be a body of work, and enough of them alike to be one kind of
// work. Both bounds exist to keep a stray migration beside application code from
// turning its directory into a surface.
const MIN_SURFACE_FILES = 3;
const MIN_SURFACE_SHARE = 0.6;

function isOwned(root: string, owned: Set<string>): boolean {
  for (const projectRoot of owned) {
    // The repository root is deliberately not an owner here, for the same
    // reason the walk descends past it: a manifest at the root is usually
    // tooling above the real work, and letting it claim everything would mean a
    // repository with a root package.json could never have a surface.
    if (projectRoot === "") continue;
    if (root === projectRoot || root.startsWith(`${projectRoot}/`)) return true;
  }
  return false;
}

export function detectSurfaces(
  targetDir: string,
  projects: StackProject[],
  depth: number = PROJECT_SEARCH_DEPTH,
): DetectedSurface[] {
  const owned = new Set(projects.map((project) => project.root));
  const found: DetectedSurface[] = [];

  const walk = (dir: string, remaining: number): void => {
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      return;
    }
    const root = toPosix(path.relative(targetDir, dir));
    // A project owns its whole tree, not just its own directory: migrations
    // inside a project are already covered by that project's tests and its
    // specialists. A project at the repository root therefore owns everything,
    // and the repository has no surfaces at all.
    if (isOwned(root, owned)) return;

    const files = entries.filter((entry) => entry.isFile()).map((e) => e.name);
    if (root !== "" && files.length >= MIN_SURFACE_FILES) {
      for (const kind of SURFACE_KINDS) {
        const matching = files.filter((file) =>
          kind.extensions.some((extension) =>
            file.toLowerCase().endsWith(extension),
          ),
        );
        if (matching.length / files.length >= MIN_SURFACE_SHARE) {
          found.push({
            root,
            label: kind.label,
            evidence: `${matching.length} of ${files.length} files in ${root} are ${kind.extensions.join(" or ")}`,
          });
          return; // this directory is the surface; do not also report its children
        }
      }
    }
    if (remaining <= 0) return;
    for (const entry of entries) {
      if (
        !entry.isDirectory() ||
        entry.name.startsWith(".") ||
        IGNORED_DIRECTORIES.has(entry.name)
      ) {
        continue;
      }
      walk(path.join(dir, entry.name), remaining - 1);
    }
  };

  // One level deeper than projects: migrations usually sit in database/migrations.
  walk(targetDir, depth + 1);
  return found;
}
