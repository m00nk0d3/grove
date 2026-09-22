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
      return; // this directory owns a project; nested markers belong to it
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
