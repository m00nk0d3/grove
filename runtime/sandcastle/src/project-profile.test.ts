import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {
  fingerprintRepoShape,
  loadProfile,
  readProfileDraft,
  writeProfile,
  PROJECT_PROFILE_VERSION,
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
import {
  detectStack,
  detectStackProjects,
  detectSurfaces,
  type StackProject,
} from "./stack-detector.js";

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
        frameworks: [],
        personas: personas("Rust"),
        test: command("cargo", ["test"]),
      },
    ],
    surfaces: [],
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
          frameworks: [],
          personas: personas("Rust"),
          test: command("cargo", ["test"]),
        },
        {
          root: "frontend",
          marker: "frontend/package.json",
          label: "TYPESCRIPT",
          ecosystem: "Vite app",
          frameworks: [],
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
          frameworks: [],
          personas: personas("Python"),
          test: command("pytest", []),
        },
        {
          root: "frontend",
          marker: "frontend/package.json",
          label: "TYPESCRIPT",
          ecosystem: "React app",
          frameworks: [],
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
    assert.equal(readProfileDraft(draftPath, detected, []).projects[0].label, "RUST");

    // A fence with its language tag is a formatting slip, not a failed profile.
    write("```json\n" + JSON.stringify(draft()) + "\n```");
    assert.equal(readProfileDraft(draftPath, detected, []).projects.length, 1);

    // Commands are spawned without a shell, so an operator cannot appear.
    const shelled = draft();
    shelled.projects[0].test = command("cargo test && cargo clippy", []);
    write(shelled);
    assert.throws(() => readProfileDraft(draftPath, detected, []), /not a single executable/);

    const badPattern = draft({
      concerns: [
        { id: "actors", title: "actors", pathPatterns: ["(unclosed"], checklist: ["x"] },
      ],
    });
    write(badPattern);
    assert.throws(() => readProfileDraft(draftPath, detected, []), /\(unclosed/);

    const missingRole = draft();
    delete (missingRole.projects[0].personas as Record<string, string>).review;
    write(missingRole);
    assert.throws(() => readProfileDraft(draftPath, detected, []), /'review' persona/);

    // A profile describing a different repository than the one being worked in.
    write(draft());
    assert.throws(
      () =>
        readProfileDraft(
          draftPath,
          [
            ...detected,
            { stack: "TYPESCRIPT", root: "frontend", marker: "frontend/package.json" },
          ],
          [],
        ),
      /missing an entry for 'frontend'/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("an optional field the prompt engineer left null is read as absent", () => {
  const root = scratch(".profile-null-");
  const draftPath = path.join(root, "draft.json");
  const detected: StackProject[] = [
    { stack: "TYPESCRIPT", root: "frontend", marker: "frontend/package.json", label: "TYPESCRIPT" },
  ];
  // Loose on purpose: this is the raw JSON the agent writes, before validation,
  // and the point is what a key holding null does to it.
  const base = (): {
    repoSummary: string;
    projects: Record<string, unknown>[];
    surfaces: Record<string, unknown>[];
    concerns: Record<string, unknown>[];
  } => ({
    repoSummary: "A frontend and its migrations.",
    projects: [
      {
        root: "frontend",
        marker: "frontend/package.json",
        label: "TYPESCRIPT",
        ecosystem: "A React app",
        frameworks: [],
        personas: personas("React"),
        test: command("npm", ["test"]),
        setup: null,
      },
    ],
    surfaces: [{ root: "migrations", validatedBy: null }],
    concerns: [
      {
        id: "worktree-lifecycle",
        title: "worktree lifecycle",
        augments: null,
        pathPatterns: ["^internal/"],
        checklist: ["one point"],
      },
    ],
  });
  const write = (value: unknown) => fs.writeFileSync(draftPath, JSON.stringify(value));
  try {
    // A JSON template with every key filled in and a null where the value is
    // unknown is the shape an agent reaches for. It means "no value", which is
    // what leaving the key out means, so it must not be read as a wrong value.
    write(base());
    const read = readProfileDraft(draftPath, detected, [{ root: "migrations", label: "SQL" }]);
    assert.equal(read.projects[0].setup, undefined);
    assert.equal(read.surfaces[0].validatedBy, undefined);
    assert.equal(read.concerns[0].augments, undefined);

    // A value that is present but wrong is still refused: only the blank is
    // forgiven, so nothing reaches a reviewer as a silently missing instruction.
    const wrongAugments = base();
    wrongAugments.concerns[0].augments = 7;
    write(wrongAugments);
    assert.throws(
      () => readProfileDraft(draftPath, detected, [{ root: "migrations", label: "SQL" }]),
      /invalid augments value/,
    );

    const wrongValidatedBy = base();
    wrongValidatedBy.surfaces[0].validatedBy = 7;
    write(wrongValidatedBy);
    assert.throws(
      () => readProfileDraft(draftPath, detected, [{ root: "migrations", label: "SQL" }]),
      /invalid validatedBy/,
    );

    const wrongSkipWhenPresent = base();
    wrongSkipWhenPresent.projects[0].setup = { ...command("npm", ["ci"]), skipWhenPresent: 7 };
    write(wrongSkipWhenPresent);
    assert.throws(
      () => readProfileDraft(draftPath, detected, [{ root: "migrations", label: "SQL" }]),
      /invalid skipWhenPresent path/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a surface the detector found and the profile left out is refused", () => {
  const root = scratch(".profile-surfaces-");
  const draftPath = path.join(root, "draft.json");
  const detected: StackProject[] = [
    { stack: "UNKNOWN", root: "backend", marker: "backend/Cargo.toml", label: "RUST" },
  ];
  const surfaces = [{ root: "migrations", label: "SQL" }];
  const write = (value: unknown) => fs.writeFileSync(draftPath, JSON.stringify(value));
  try {
    // The omission this guard exists for: the prompt listed the surface as
    // context and its output shape had no 'surfaces' key, so the agent returned
    // none and every migration change validated every project in the repository.
    write(draft());
    assert.throws(
      () => readProfileDraft(draftPath, detected, surfaces),
      /missing an entry for the SQL surface at 'migrations'/,
    );

    const described = draft({ surfaces: [{ root: "migrations", validatedBy: "backend" }] });
    write(described);
    const read = readProfileDraft(draftPath, detected, surfaces);
    assert.equal(read.surfaces[0].validatedBy, "backend");
    // Only what verification reads survives; the rest of the older shape is
    // read past rather than refused, so a stale profile is not a broken one.
    assert.equal((read.surfaces[0] as unknown as Record<string, unknown>).personas, undefined);

    // A surface the detector never found is as wrong as a project it never
    // found, and just as likely to be a guess than a reading.
    write(draft({ surfaces: [{ root: "styles", validatedBy: "backend" }] }));
    assert.throws(
      () => readProfileDraft(draftPath, detected, []),
      /'styles', which is not a surface in this repository/,
    );

    // And a surface validated by a project that does not exist falls back to
    // validating everything, which is the cost the field removes.
    write(draft({ surfaces: [{ root: "migrations", validatedBy: "frontend" }] }));
    assert.throws(
      () => readProfileDraft(draftPath, detected, surfaces),
      /validated by 'frontend'/,
    );

    // Nothing to describe means nothing to check.
    write(draft());
    assert.equal(readProfileDraft(draftPath, detected, []).surfaces.length, 0);
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

test("a type definition bump does not summon the prompt engineer", () => {
  const root = scratch(".profile-types-");
  try {
    fs.mkdirSync(path.join(root, "frontend"));
    const manifest = path.join(root, "frontend", "package.json");
    const write = (value: unknown) =>
      fs.writeFileSync(manifest, JSON.stringify(value));
    write({
      scripts: { test: "vitest" },
      devDependencies: { "@types/node": "^26.6.2", typescript: "^5.9.0" },
    });
    const shape = () => fingerprintRepoShape(root, detectStackProjects(root));
    const original = shape();

    // @types/node moves a patch every few days and no code the project builds,
    // runs or reviews comes out of it.
    write({
      scripts: { test: "vitest" },
      devDependencies: { "@types/node": "^26.6.3", typescript: "^5.9.0" },
    });
    assert.equal(shape(), original, "a @types patch bump is not a change of shape");

    // Reordering dependencies is a reformat, like reordering the keys above.
    write({
      scripts: { test: "vitest" },
      devDependencies: { typescript: "^5.9.0", "@types/node": "^26.6.2" },
    });
    assert.equal(shape(), original, "dependency order is not a change of shape");

    // A real dependency change still is: it can bring a framework with it.
    write({
      scripts: { test: "vitest" },
      devDependencies: { typescript: "^5.9.0", vitest: "^3.0.0" },
    });
    assert.notEqual(shape(), original, "a new dependency is a change of shape");

    // As is a new test script, which is the change the filter exists to keep.
    const withVite = shape();
    write({ scripts: { test: "jest" }, devDependencies: { typescript: "^5.9.0" } });
    assert.notEqual(shape(), withVite);
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

test("an older profile is used now, not refused for what it could not have known", () => {
  const root = scratch(".profile-oldversion-");
  const profilePath = path.join(root, "agent-flow", "project-profile.json");
  const surfaces = [{ root: "migrations", label: "SQL" }];
  try {
    fs.mkdirSync(path.join(root, "backend"));
    fs.writeFileSync(path.join(root, "backend", "Cargo.toml"), "[package]\n");
    fs.mkdirSync(path.join(root, "migrations"));
    for (const name of ["001.sql", "002.sql", "003.sql"]) {
      fs.writeFileSync(path.join(root, "migrations", name), "-- x\n");
    }
    const projects = detectStackProjects(root);
    assert.equal(detectSurfaces(root, projects).length, 1, "the surface is real");

    // Written by the schema before surfaces had to be described, so it has none.
    // Holding it to the current requirement would report it invalid and discard
    // a set of personas, when the caller is about to regenerate it anyway.
    fs.mkdirSync(path.dirname(profilePath), { recursive: true });
    fs.writeFileSync(
      profilePath,
      JSON.stringify({
        ...draft(),
        surfaces: [],
        version: PROJECT_PROFILE_VERSION - 1,
        fingerprint: fingerprintRepoShape(root, projects, surfaces),
      }),
    );
    const older = loadProfile(profilePath, root, projects, surfaces);
    assert.equal(older.status, "stale");
    assert.match(older.reason, /profile version/);
    assert.ok(older.profile, "a persona set the old schema did write is still usable");
    assert.equal(older.profile!.projects[0].personas.implementation.length > 0, true);

    // The same omission in a current-version profile is a real defect, and the
    // caller has already been told the profile is fine to use.
    fs.writeFileSync(
      profilePath,
      JSON.stringify({
        ...draft(),
        surfaces: [],
        version: PROJECT_PROFILE_VERSION,
        fingerprint: fingerprintRepoShape(root, projects, surfaces),
      }),
    );
    const current = loadProfile(profilePath, root, projects, surfaces);
    assert.equal(current.status, "invalid");
    assert.match(current.reason, /missing an entry for the SQL surface/);
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
