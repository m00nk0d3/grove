import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import {
  planReviewPasses,
  resolveMaxReviewPasses,
  DEFAULT_MAX_REVIEW_PASSES,
} from "./specialist-gating.js";
import { combineAuditVerdicts, type AuditVerdict } from "./review-loop.js";
import {
  detectSurfaces,
  detectStackProjects,
  readDirectDependencies,
} from "./stack-detector.js";
import { planVerification } from "./workflow-utils.js";
import type { ProfileConcern, RepoProfile } from "./project-profile.js";
import { builtinReviewSections } from "./specialists.js";

function scratch(prefix: string): string {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

function concern(
  id: string,
  patterns: string[],
  augments?: string,
): ProfileConcern {
  return {
    id,
    title: id.replace(/-/g, " "),
    ...(augments ? { augments } : {}),
    pathPatterns: patterns,
    checklist: [`check ${id}`],
  };
}

function verdict(blockers: string[]): AuditVerdict {
  return {
    verdict: blockers.length > 0 ? "blockers" : "approved",
    summary: blockers.length > 0 ? "found something" : "nothing found",
    reviewedAreas: ["looked at the diff"],
    blockers,
  };
}

test("dependencies are read from each kind of manifest, and never thrown", () => {
  const root = scratch("deps-");
  try {
    const write = (name: string, body: string) => {
      fs.writeFileSync(path.join(root, name), body);
      return path.join(root, name);
    };

    assert.deepEqual(
      readDirectDependencies(
        write(
          "package.json",
          JSON.stringify({
            dependencies: { react: "18" },
            devDependencies: { tailwindcss: "3" },
          }),
        ),
      ),
      ["react", "tailwindcss"],
    );

    assert.deepEqual(
      readDirectDependencies(
        write(
          "pyproject.toml",
          '[project]\ndependencies = ["fastapi>=0.1", "alembic"]\n[tool.poetry.dependencies]\npython = "^3.11"\nsqlalchemy = "*"\n',
        ),
      ),
      // python is the interpreter, not a dependency worth naming.
      ["fastapi", "alembic", "sqlalchemy"],
    );

    assert.deepEqual(
      readDirectDependencies(
        write("requirements.txt", "# comment\nDjango==4.2\npsycopg[binary]>=3\n-r other.txt\n"),
      ),
      ["Django", "psycopg"],
    );

    assert.deepEqual(
      readDirectDependencies(
        write(
          "App.csproj",
          '<Project><ItemGroup><PackageReference Include="Microsoft.EntityFrameworkCore" Version="8.0.0" /></ItemGroup></Project>',
        ),
      ),
      ["Microsoft.EntityFrameworkCore"],
    );

    // A manifest this cannot parse costs nothing: the evidence is a bonus.
    assert.deepEqual(readDirectDependencies(write("broken.json", "{{{")), []);
    assert.deepEqual(
      readDirectDependencies(path.join(root, "does-not-exist.json")),
      [],
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a directory of one kind of work is a surface; a stray file is not", () => {
  const root = scratch("surfaces-");
  try {
    fs.mkdirSync(path.join(root, "backend"));
    fs.writeFileSync(path.join(root, "backend", "pyproject.toml"), "[project]");
    // Inside the project, so the project already covers it.
    fs.mkdirSync(path.join(root, "backend", "migrations"));
    for (const name of ["a.sql", "b.sql", "c.sql"]) {
      fs.writeFileSync(path.join(root, "backend", "migrations", name), "");
    }
    fs.mkdirSync(path.join(root, "database"));
    for (const name of ["001.sql", "002.sql", "003.sql"]) {
      fs.writeFileSync(path.join(root, "database", name), "");
    }
    // Two files is not a body of work.
    fs.mkdirSync(path.join(root, "tiny"));
    for (const name of ["x.sql", "y.sql"]) {
      fs.writeFileSync(path.join(root, "tiny", name), "");
    }
    // One stray migration among code is not a migrations directory.
    fs.mkdirSync(path.join(root, "scripts"));
    fs.writeFileSync(path.join(root, "scripts", "seed.sql"), "");
    fs.writeFileSync(path.join(root, "scripts", "run.ts"), "");
    fs.writeFileSync(path.join(root, "scripts", "build.ts"), "");

    const surfaces = detectSurfaces(root, detectStackProjects(root));
    assert.deepEqual(
      surfaces.map((surface) => `${surface.root}:${surface.label}`),
      ["database:SQL"],
    );
    assert.match(surfaces[0].evidence, /3 of 3 files/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a change confined to a surface validates the project that covers it", () => {
  const python = {
    stack: "PYTHON" as const,
    root: "backend",
    marker: "backend/pyproject.toml",
  };
  const typescript = {
    stack: "TYPESCRIPT" as const,
    root: "frontend",
    marker: "frontend/package.json",
  };
  const projects = [python, typescript];
  const withSurface = {
    version: 2,
    generatedAt: "",
    fingerprint: "",
    repoSummary: "",
    projects: [],
    concerns: [],
    surfaces: [
      {
        root: "database",
        label: "SQL",
        purpose: "migrations",
        personas: {} as never,
        validatedBy: "backend",
      },
    ],
  } as unknown as RepoProfile;

  const labels = (files: string[], profile: RepoProfile | null) =>
    planVerification(projects, files, profile).map((task) => task.label);

  // Today, with nothing declared: a lone migration runs every suite.
  assert.deepEqual(labels(["database/001.sql"], null), [
    "python -m pytest (in backend)",
    "npm test (in frontend)",
  ]);

  // Declared: only the project whose tests cover it.
  assert.deepEqual(labels(["database/001.sql"], withSurface), [
    "python -m pytest (in backend)",
  ]);

  // A surface change alongside a frontend change still runs the frontend.
  assert.deepEqual(
    labels(["database/001.sql", "frontend/src/App.tsx"], withSurface),
    ["python -m pytest (in backend)", "npm test (in frontend)"],
  );

  // A surface that names no owner keeps the conservative answer: too slow is a
  // better failure than unvalidated.
  const unowned = {
    ...withSurface,
    surfaces: [{ ...withSurface.surfaces[0], validatedBy: undefined }],
  } as RepoProfile;
  assert.deepEqual(labels(["database/001.sql"], unowned), [
    "python -m pytest (in backend)",
    "npm test (in frontend)",
  ]);
});

test("each concern gets its own reviewer, ranked by how much of the diff it matches", () => {
  const changedFiles = [
    "database/001.sql",
    "database/002.sql",
    "database/003.sql",
    "src/auth/login.ts",
    "styles/button.css",
  ];
  const plan = planReviewPasses({
    changedFiles,
    builtins: ["security-audit", "database-review"],
    builtinSections: builtinReviewSections(),
    concerns: [concern("tailwind-design", ["styles/"])],
    maxPasses: 5,
  });

  // Ranked by matched files, so the reviewer with the most to look at goes first.
  assert.deepEqual(
    plan.passes.map((pass) => pass.id),
    ["database-review", "security-audit", "tailwind-design"],
  );
  assert.equal(plan.skipped.length, 0);

  // Each reviewer carries only its own checklist. That is the whole point:
  // one prompt with six checklists reviews each of them less well.
  const database = plan.passes.find((pass) => pass.id === "database-review");
  assert.match(database!.sections, /Destructive operations/);
  assert.doesNotMatch(database!.sections, /Authentication and authorization/);

  const design = plan.passes.find((pass) => pass.id === "tailwind-design");
  assert.match(design!.sections, /check tailwind-design/);
  assert.doesNotMatch(design!.sections, /Destructive operations/);
});

test("a concern that extends a built-in is reviewed with it, not beside it", () => {
  const plan = planReviewPasses({
    changedFiles: ["priv/repo/migrations/001.exs"],
    builtins: [],
    builtinSections: builtinReviewSections(),
    concerns: [concern("ecto", ["priv/repo/"], "database-review")],
    maxPasses: 5,
  });
  assert.equal(plan.passes.length, 1, "one reviewer, not two");
  assert.match(plan.passes[0].sections, /Destructive operations/);
  assert.match(plan.passes[0].sections, /check ecto/);
});

test("the cap bounds authored concerns but never drops a built-in", () => {
  const changedFiles = [
    "styles/a.css",
    "styles/b.css",
    "styles/c.css",
    "styles/d.css",
    "docs/a.md",
    "docs/b.md",
    "docs/c.md",
    "i18n/a.json",
    "i18n/b.json",
    // One security-relevant file: matched once, and severe.
    "src/auth.ts",
  ];
  const plan = planReviewPasses({
    changedFiles,
    builtins: ["security-audit"],
    builtinSections: builtinReviewSections(),
    concerns: [
      concern("design", ["styles/"]),
      concern("copy", ["docs/"]),
      concern("i18n", ["i18n/"]),
    ],
    maxPasses: 2,
  });

  // A one-file security audit outranks nothing on match count, and would be the
  // first thing dropped. Losing it because stylesheets moved is the wrong way
  // to be wrong, so built-ins are counted against the cap but never cut.
  assert.ok(
    plan.passes.some((pass) => pass.id === "security-audit"),
    "the security audit survives a cap it does not win on count",
  );
  assert.equal(plan.passes.length, 2);
  assert.deepEqual(
    plan.skipped.map((pass) => pass.id),
    ["copy", "i18n"],
  );
  assert.equal(plan.skipped[0].matchCount, 3);
});

test("reviewer names stay unique inside the agent-name limit", () => {
  const plan = planReviewPasses({
    changedFiles: ["a/x.ts", "b/x.ts"],
    builtins: [],
    builtinSections: {},
    concerns: [
      concern("database-migrations-additive", ["a/"]),
      concern("database-migrations-rollback", ["b/"]),
    ],
    maxPasses: 5,
  });

  const slugs = plan.passes.map((pass) => pass.slug);
  assert.equal(new Set(slugs).size, slugs.length, "two reviewers, two names");

  // herdr truncates the agent name to 32 characters, and two reviewers
  // truncating alike would have the second resume the first's session.
  for (const slug of slugs) {
    const agentName = `af-${slug}-5-123456-987654`;
    assert.ok(
      agentName.length <= 32,
      `${agentName} is ${agentName.length} characters`,
    );
  }
});

test("several reviewers produce one verdict the implementer can act on", () => {
  const combined = combineAuditVerdicts(
    [
      { id: "database-review", title: "database", verdict: verdict(["drops a column"]) },
      { id: "security-audit", title: "security", verdict: verdict([]) },
      { id: "design", title: "design", verdict: verdict(["contrast too low"]) },
    ],
    [{ id: "i18n", title: "i18n", matchCount: 2 }],
  );

  assert.equal(combined.verdict, "blockers");
  // Each finding keeps the concern that raised it, so a fix knows which
  // checklist it is answering.
  assert.deepEqual(combined.blockers, [
    "database-review | drops a column",
    "design | contrast too low",
  ]);
  assert.match(combined.summary, /database: found something/);
  // A concern the cap skipped is recorded in the artifact, not only the console.
  assert.ok(
    combined.reviewedAreas.some((area) => /i18n \| not reviewed/.test(area)),
  );

  const clean = combineAuditVerdicts([
    { id: "security-audit", title: "security", verdict: verdict([]) },
  ]);
  assert.equal(clean.verdict, "approved");
});

test("the review cap is configurable and refuses nonsense", () => {
  assert.equal(resolveMaxReviewPasses({}), DEFAULT_MAX_REVIEW_PASSES);
  assert.equal(resolveMaxReviewPasses({ AGENT_FLOW_MAX_REVIEW_PASSES: "3" }), 3);
  assert.equal(
    resolveMaxReviewPasses({ AGENT_FLOW_MAX_REVIEW_PASSES: "0" }),
    DEFAULT_MAX_REVIEW_PASSES,
  );
  assert.equal(
    resolveMaxReviewPasses({ AGENT_FLOW_MAX_REVIEW_PASSES: "many" }),
    DEFAULT_MAX_REVIEW_PASSES,
  );
});

// --------------------------------------------------------------------------
// Regressions found by an adversarial sweep. Each one was a real defect.
// --------------------------------------------------------------------------

test("a project owns its whole tree, so no surface forms inside it", () => {
  const root = scratch("adv-owned-");
  try {
    // A project at the repository root owns everything below it.
    fs.writeFileSync(path.join(root, "package.json"), "{}");
    fs.mkdirSync(path.join(root, "migrations"));
    for (const n of ["1.sql", "2.sql", "3.sql"]) {
      fs.writeFileSync(path.join(root, "migrations", n), "");
    }
    assert.deepEqual(detectSurfaces(root, detectStackProjects(root)), []);

    // And so does a nested one.
    const nested = scratch("adv-owned-nested-");
    try {
      fs.mkdirSync(path.join(nested, "backend"));
      fs.writeFileSync(path.join(nested, "backend", "pyproject.toml"), "[project]");
      fs.mkdirSync(path.join(nested, "backend", "migrations"));
      for (const n of ["1.sql", "2.sql", "3.sql"]) {
        fs.writeFileSync(path.join(nested, "backend", "migrations", n), "");
      }
      assert.deepEqual(detectSurfaces(nested, detectStackProjects(nested)), []);
    } finally {
      fs.rmSync(nested, { recursive: true, force: true });
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a byte order mark does not hide a manifest's dependencies", () => {
  const root = scratch("adv-bom-");
  try {
    const manifest = path.join(root, "package.json");
    fs.writeFileSync(manifest, "﻿" + JSON.stringify({ dependencies: { react: "18" } }));
    assert.deepEqual(readDirectDependencies(manifest), ["react"]);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("a pattern that can take exponential time to match is refused", async () => {
  const { readProfileDraft } = await import("./project-profile.js");
  const root = scratch("adv-redos-");
  const draftPath = path.join(root, "draft.json");
  const personas = Object.fromEntries(
    ["planning", "tests", "implementation", "verification", "review", "documentation"].map(
      (role) => [role, "You are a Senior Engineer for this repository and follow its conventions."],
    ),
  );
  const detected = [
    { stack: "PYTHON" as const, root: "backend", marker: "backend/pyproject.toml" },
  ];
  const base = {
    repoSummary: "x",
    projects: [
      {
        root: "backend",
        marker: "backend/pyproject.toml",
        label: "PYTHON",
        ecosystem: "svc",
        frameworks: [],
        personas,
        test: { command: "node", args: [], verified: true, evidence: "ran" },
      },
    ],
    surfaces: [],
    concerns: [] as unknown[],
  };
  const write = (concerns: unknown[]) =>
    fs.writeFileSync(draftPath, JSON.stringify({ ...base, concerns }));
  try {
    // /^(a+)+$/ against a 41-character path never returns, and a JavaScript
    // regex cannot be interrupted: the run would stall for ever with no error.
    write([{ id: "evil", title: "evil", pathPatterns: ["^(a+)+$"], checklist: ["x"] }]);
    assert.throws(
      () => readProfileDraft(draftPath, detected),
      /exponential time/,
    );

    // An ordinary path vocabulary is still accepted.
    write([
      { id: "fine", title: "fine", pathPatterns: ["(^|/)migrations?(/|$)"], checklist: ["x"] },
    ]);
    assert.equal(readProfileDraft(draftPath, detected).concerns.length, 1);

    // Everything rendered into a prompt is bounded.
    write([
      {
        id: "huge",
        title: "huge",
        pathPatterns: ["(^|/)x/"],
        checklist: Array.from({ length: 2000 }, (_, i) => `item ${i}`),
      },
    ]);
    assert.throws(() => readProfileDraft(draftPath, detected), /checklist items/);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("verification refuses when nothing can validate the change", async () => {
  const { verifyWorktree } = await import("./workflow-utils.js");
  const root = scratch("adv-nothing-");
  try {
    // A project this runtime has no command for, and no profile to supply one.
    fs.mkdirSync(path.join(root, "engine"));
    fs.writeFileSync(path.join(root, "engine", "Cargo.toml"), "[package]");
    // Silently running nothing would let the delivery gate pass on an
    // unvalidated change, which is the one answer verification must never give.
    assert.throws(
      () => verifyWorktree(root, ["engine/src/main.rs"], null),
      /Nothing can validate this change/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
