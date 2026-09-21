import fs from "fs";
import path from "path";

export type TechStack = "GO" | "PYTHON" | "TYPESCRIPT" | "CSHARP";

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

// Ordered by specificity: a directory is attributed to the first stack whose
// marker it contains, so a .NET project that also carries a package.json for
// tooling is still a .NET project.
function markerFor(files: string[]): { stack: TechStack; marker: string } | undefined {
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
// should prefer detectStackProjects; this remains for callers that need a
// single label, and keeps TypeScript as the historical fallback.
export function detectStack(targetDir: string): TechStack {
  const projects = detectStackProjects(targetDir);
  if (projects.length === 0) {
    return "TYPESCRIPT";
  }
  const priority: TechStack[] = ["GO", "PYTHON", "CSHARP", "TYPESCRIPT"];
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
