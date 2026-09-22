import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {
  fingerprintRepoShape,
  loadProfile,
  readProfileDraft,
  writeProfile,
  ROLE_KEYS,
  type RepoProfileDraft,
  type RoleKey,
} from "./project-profile.js";
import { personaFor } from "./specialists.js";
import { selectProfileConcerns } from "./specialist-gating.js";
import {
  planVerification,
  resolveVerificationTask,
  unresolvedProjects,
  getVerificationCommand,
  ensureProjectSetup,
} from "./workflow-utils.js";
import { detectStack, detectStackProjects, type StackProject } from "./stack-detector.js";

function personas(text: string): Record<RoleKey, string> {
  return Object.fromEntries(
    ROLE_KEYS.map((role) => [role, `${text} — the ${role} specialist for this project.`]),
  ) as Record<RoleKey, string>;
}

function command(name: string, args: string[], verified = true) {
  return { command: name, args, verified, evidence: "ran in the project" };
}

function draft(overrides: Partial<RepoProfileDraft> = {}): RepoProfileDraft {
  return {
    repoSummary: "A backend and a frontend.",
    projects: [
      {
        root: "backend",
        marker: "backend/Cargo.toml",
        label: "RUST",
        ecosystem: "Rust 2021 workspace",
        personas: personas("Rust"),
        test: command("cargo", ["test"]),
      },
    ],
    concerns: [],
    ...overrides,
  };
}

function scratch(prefix: string): string {
  return fs.mkdtempSync(path.join(process.cwd(), prefix));
}

test("a project in a language the runtime does not know is still found", () => {
  const root = scratch(".profile-detect-");
  try {
    fs.mkdirSync(path.join(root, "backend"));
    fs.writeFileSync(path.join(root, "backend", "Cargo.toml"), "[package]\n");
    fs.mkdirSync(path.join(root, "frontend"));
    fs.writeFileSync(path.join(root, "frontend", "package.json"), "{}");

    const projects = detectStackProjects(root);
    const rust = projects.find((project) => project.root === "backend");
    assert.deepEqual(rust, {
      stack: "UNKNOWN",
      root: "backend",
      marker: "backend/Cargo.toml",
      label: "RUST",
    });
    // A built-in stack still wins where both markers sit in one directory.
    fs.writeFileSync(path.join(root, "frontend", "Cargo.toml"), "[package]\n");
    assert.equal(
      detectStackProjects(root).find((project) => project.root === "frontend")?.stack,
      "TYPESCRIPT",
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a repository of only unknown projects reports UNKNOWN rather than TypeScript", () => {
  const root = scratch(".profile-unknown-");
  try {
    fs.writeFileSync(path.join(root, "Cargo.toml"), "[package]\n");
    assert.equal(detectStack(root), "UNKNOWN");
    assert.equal(getVerificationCommand("UNKNOWN"), null);
    // An empty directory keeps the historical answer; it feeds only role names.
    const empty = scratch(".profile-empty-");
    try {
      assert.equal(detectStack(empty), "TYPESCRIPT");
    } finally {
      fs.rmSync(empty, { recursive: true, force: true });
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a verified command wins, an unverified one never displaces a built-in", () => {
  const rust: StackProject = {
    stack: "UNKNOWN",
    root: "backend",
    marker: "backend/Cargo.toml",
    label: "RUST",
  };
  const typescript: StackProject = {
    stack: "TYPESCRIPT",
    root: "frontend",
    marker: "frontend/package.json",
  };
  const profile = writeableProfile(
    draft({
      projects: [
        {
          root: "backend",
          marker: "backend/Cargo.toml",
          label: "RUST",
          ecosystem: "Rust",
          personas: personas("Rust"),
          test: command("cargo", ["test"]),
        },
        {
          root: "frontend",
          marker: "frontend/package.json",
          label: "TYPESCRIPT",
          ecosystem: "Vite app",
          personas: personas("TypeScript"),
          // Not run by the profiler, so it must not replace `npm test`.
          test: command("pnpm", ["test"], false),
        },
      ],
    }),
  );

  // The profile is the only thing that can validate a project the runtime does
  // not recognise.
  assert.equal(resolveVerificationTask(rust, null), null);
  assert.deepEqual(unresolvedProjects([rust], null), [rust]);
  assert.equal(resolveVerificationTask(rust, profile)?.command, "cargo");
  assert.equal(resolveVerificationTask(rust, profile)?.source, "profile");

  // An unverified command loses to one that works today.
  const resolved = resolveVerificationTask(typescript, profile);
  assert.equal(resolved?.command, "npm");
  assert.equal(resolved?.source, "builtin");

  // Attribution by changed file is unchanged.
  const tasks = planVerification([rust, typescript], ["backend/src/main.rs"], profile);
  assert.deepEqual(
    tasks.map((task) => task.label),
    ["cargo test (in backend)"],
  );
});

test("a step gets the specialist for the code it is about to touch", () => {
  const python: StackProject = {
    stack: "PYTHON",
    root: "backend",
    marker: "backend/pyproject.toml",
  };
  const typescript: StackProject = {
    stack: "TYPESCRIPT",
    root: "frontend",
    marker: "frontend/package.json",
  };
  const projects = [python, typescript];
  const profile = writeableProfile(
    draft({
      projects: [
        {
          root: "backend",
          marker: "backend/pyproject.toml",
          label: "PYTHON",
          ecosystem: "FastAPI service",
          personas: personas("Python"),
          test: command("pytest", []),
        },
        {
          root: "frontend",
          marker: "frontend/package.json",
          label: "TYPESCRIPT",
          ecosystem: "React app",
          personas: personas("TypeScript"),
          test: command("npm", ["test"]),
        },
      ],
    }),
  );

  // A pull request confined to the Python project is reviewed by the Python
  // reviewer, and the TypeScript app is never mentioned.
  const review = personaFor("review", projects, profile, ["backend/app/api.py"]);
  assert.match(review, /Python — the review specialist/);
  assert.doesNotMatch(review, /TypeScript/);

  // Each role is distinct, so implementation does not get the reviewer.
  assert.match(
    personaFor("implementation", projects, profile, ["backend/app/api.py"]),
    /Python — the implementation specialist/,
  );

  // A change spanning both is given both, named by their directories.
  const both = personaFor("review", projects, profile, [
    "backend/app/api.py",
    "frontend/src/App.tsx",
  ]);
  assert.match(both, /more than one stack/);
  assert.match(both, /- backend: PYTHON/);
  assert.match(both, /- frontend: TYPESCRIPT/);

  // Nothing attributable means every project rather than a guess at one.
  assert.match(personaFor("review", projects, profile, []), /more than one stack/);
});

test("without a profile the built-in personas are unchanged", () => {
  const typescript: StackProject = {
    stack: "TYPESCRIPT",
    root: "",
    marker: "package.json",
  };
  assert.match(
    personaFor("implementation", [typescript], null, ["src/index.ts"]),
    /Senior TypeScript Engineer/,
  );
  // An unrecognised project has no honest built-in expertise to claim.
  const rust: StackProject = {
    stack: "UNKNOWN",
    root: "",
    marker: "Cargo.toml",
    label: "RUST",
  };
  const persona = personaFor("implementation", [rust], null, ["src/main.rs"]);
  assert.match(persona, /Senior Software Engineer/);
  assert.doesNotMatch(persona, /TypeScript/);
});

test("the profile draft is validated before any of it reaches an agent", () => {
  const root = scratch(".profile-draft-");
  const draftPath = path.join(root, "draft.json");
  const detected: StackProject[] = [
    { stack: "UNKNOWN", root: "backend", marker: "backend/Cargo.toml", label: "RUST" },
  ];
  const write = (value: unknown) =>
    fs.writeFileSync(draftPath, typeof value === "string" ? value : JSON.stringify(value));
  try {
    write(draft());
    assert.equal(readProfileDraft(draftPath, detected).projects[0].label, "RUST");

    // A fence with its language tag is a formatting slip, not a failed profile.
    write("```json\n" + JSON.stringify(draft()) + "\n```");
    assert.equal(readProfileDraft(draftPath, detected).projects.length, 1);

    // Commands are spawned without a shell, so an operator cannot appear.
    const shelled = draft();
    shelled.projects[0].test = command("cargo test && cargo clippy", []);
    write(shelled);
    assert.throws(() => readProfileDraft(draftPath, detected), /not a single executable/);

    const badPattern = draft({
      concerns: [
        { id: "actors", title: "actors", pathPatterns: ["(unclosed"], checklist: ["x"] },
      ],
    });
    write(badPattern);
    assert.throws(() => readProfileDraft(draftPath, detected), /\(unclosed/);

    const missingRole = draft();
    delete (missingRole.projects[0].personas as Record<string, string>).review;
    write(missingRole);
    assert.throws(() => readProfileDraft(draftPath, detected), /'review' persona/);

    // A profile describing a different repository than the one being worked in.
    write(draft());
    assert.throws(
      () =>
        readProfileDraft(draftPath, [
          ...detected,
          { stack: "TYPESCRIPT", root: "frontend", marker: "frontend/package.json" },
        ]),
      /missing an entry for 'frontend'/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("the fingerprint moves when the repository's shape does, and not otherwise", () => {
  const root = scratch(".profile-fingerprint-");
  try {
    fs.mkdirSync(path.join(root, "frontend"));
    const manifest = path.join(root, "frontend", "package.json");
    fs.writeFileSync(manifest, JSON.stringify({ version: "1.0.0", scripts: { test: "vitest" } }));

    const shape = () => fingerprintRepoShape(root, detectStackProjects(root));
    const original = shape();
    assert.equal(shape(), original, "stable across calls");

    // A version bump changes the manifest but not how the project is tested.
    fs.writeFileSync(manifest, JSON.stringify({ version: "1.0.1", scripts: { test: "vitest" } }));
    assert.equal(shape(), original, "a version bump is not a change of shape");

    // A new test script is.
    fs.writeFileSync(manifest, JSON.stringify({ version: "1.0.1", scripts: { test: "jest" } }));
    assert.notEqual(shape(), original);

    // So is a project appearing.
    const withScript = shape();
    fs.mkdirSync(path.join(root, "backend"));
    fs.writeFileSync(path.join(root, "backend", "Cargo.toml"), "[package]\n");
    assert.notEqual(shape(), withScript);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a profile is reported stale or invalid rather than thrown away", () => {
  const root = scratch(".profile-load-");
  const profilePath = path.join(root, "agent-flow", "project-profile.json");
  try {
    fs.mkdirSync(path.join(root, "backend"));
    fs.writeFileSync(path.join(root, "backend", "Cargo.toml"), "[package]\n");
    const projects = detectStackProjects(root);

    assert.equal(loadProfile(profilePath, root, projects).status, "absent");

    writeProfile(profilePath, draft(), fingerprintRepoShape(root, projects));
    assert.equal(loadProfile(profilePath, root, projects).status, "fresh");

    // The repository moves; the profile is kept and used, and regenerated next.
    fs.writeFileSync(path.join(root, "backend", "Cargo.toml"), "[package]\nname = 'x'\n");
    const stale = loadProfile(profilePath, root, projects);
    assert.equal(stale.status, "stale");
    assert.ok(stale.profile, "a stale persona still beats the wrong one");

    // A corrupt profile must not stop every command on the repository.
    fs.writeFileSync(profilePath, "{ not json");
    const invalid = loadProfile(profilePath, root, projects);
    assert.equal(invalid.status, "invalid");
    assert.equal(invalid.profile, null);

    // One written by a newer sandcastle is left alone rather than overwritten.
    fs.writeFileSync(profilePath, JSON.stringify({ ...draft(), version: 99 }));
    const newer = loadProfile(profilePath, root, projects);
    assert.equal(newer.status, "invalid");
    assert.match(newer.reason, /newer version/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a generated concern reaches a review the built-in vocabulary would miss", () => {
  const concerns = [
    {
      id: "ecto-migrations",
      title: "Ecto migrations",
      augments: "database-review",
      pathPatterns: ["(^|/)priv/repo/"],
      checklist: ["Ecto migrations run inside a transaction by default."],
    },
    {
      id: "supervision",
      title: "actor supervision",
      pathPatterns: ["(^|/)lib/[^/]+/application\\.ex$"],
      checklist: ["A new process is placed under a supervisor."],
    },
  ];

  const matched = selectProfileConcerns(["priv/repo/migrations/001_add.exs"], concerns);
  assert.deepEqual(
    matched.map((concern) => concern.id),
    ["ecto-migrations"],
  );
  assert.equal(matched[0].augments, "database-review");

  assert.deepEqual(
    selectProfileConcerns(["lib/app/application.ex"], concerns).map((c) => c.id),
    ["supervision"],
  );
  assert.deepEqual(selectProfileConcerns(["README.md"], concerns), []);
});

test("setup runs only when its marker is absent", () => {
  const root = scratch(".profile-setup-");
  try {
    const calls: string[][] = [];
    const runner = (name: string, args: string[]) => {
      calls.push([name, ...args]);
      return "";
    };
    const setup = { command: "cargo", args: ["fetch"], skipWhenPresent: "target" };

    assert.equal(ensureProjectSetup(setup, root, runner), "installed");
    assert.deepEqual(calls, [["cargo", "fetch"]]);

    calls.length = 0;
    fs.mkdirSync(path.join(root, "target"));
    assert.equal(ensureProjectSetup(setup, root, runner), "present");
    assert.deepEqual(calls, [], "an ordinary run pays nothing");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

// writeProfile stamps the code-owned fields; tests that need a RepoProfile
// rather than a draft go through it so they exercise the same shape the
// runtime reads back.
function writeableProfile(value: RepoProfileDraft) {
  const root = scratch(".profile-temp-");
  try {
    const profilePath = path.join(root, "profile.json");
    return writeProfile(profilePath, value, "fingerprint");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}
