export type WorkflowMode = "lean" | "full";
export type WorkflowModeSource = "automatic" | "explicit" | "migration";

export interface IssueMetadata {
  title: string;
  body: string;
  labels: string[];
}

export interface WorkflowClassification {
  mode: WorkflowMode;
  reason: string;
  source: WorkflowModeSource;
}

interface GhIssue {
  title?: unknown;
  body?: unknown;
  labels?: unknown;
}

const FULL_LABELS = new Set([
  "architecture",
  "breaking",
  "breaking-change",
  "breaking change",
  "epic",
  "large",
  "migration",
  "multi-component",
  "refactor",
  "security",
  "type: breaking",
  "type: migration",
  "type: refactor",
  "type: security",
  "vulnerability",
]);

const LEAN_LABELS = new Set([
  "chore",
  "documentation",
  "docs",
  "spelling",
  "typo",
  "type: chore",
  "type: docs",
]);

const FULL_TEXT_SIGNALS: Array<[RegExp, string]> = [
  [/\bbreaking[- ]change\b/i, "describes a breaking change"],
  [/\b(?:security|vulnerability)\b/i, "has security implications"],
  [/\b(?:migrate|migration)\b/i, "requires a migration"],
  [/\b(?:architecture|architectural|re-architect(?:ure|ing)?)\b/i, "requires architecture work"],
  [/\brefactor(?:ing)?\b/i, "explicitly requests refactoring"],
  [/\bacross (?:multiple|several) (?:components|packages|services|modules)\b/i, "spans multiple components"],
];

const FILE_PATH_PATTERN =
  /(?:^|[\s('"`])((?:[\w.-]+\/)+[\w.-]+\.[A-Za-z0-9]+|(?:README|CHANGELOG|CONTRIBUTING|LICENSE)(?:\.[A-Za-z0-9]+)?)(?=$|[\s)'":,`])/gim;

function normalizedLabels(labels: string[]): string[] {
  return labels.map((label) => label.trim().toLowerCase()).filter(Boolean);
}

function countChecklistItems(body: string): number {
  return body.match(/^\s*[-*]\s+\[[ xX]\]\s+/gm)?.length ?? 0;
}

function referencedFiles(text: string): string[] {
  return [...text.matchAll(FILE_PATH_PATTERN)].map((match) => match[1]);
}

// scopeFilesFromIssue answers "which part of this repository is the issue about?"
// so the stages that run before any diff exists — planning, tests, implementation
// — get the specialist for the code they are about to touch rather than a
// composite describing the whole repository.
//
// Evidence comes from paths written in the issue and from labels that name an
// area. An issue that offers neither yields nothing, and the caller then treats
// every project as in scope: a vague issue should get the broad specialist, not a
// confident guess at the wrong language.
export function scopeFilesFromIssue(
  metadata: IssueMetadata,
  projects: { root: string }[],
): string[] {
  const roots = projects
    .map((project) => project.root)
    .filter((root) => root.length > 0);
  const scope = new Set<string>();

  for (const file of referencedFiles(`${metadata.title}\n${metadata.body}`)) {
    scope.add(file.replace(/\\/g, "/"));
  }

  // A label such as 'area: backend' or plain 'frontend' names a project root as
  // reliably as a path does, and issues carry them far more often.
  const text = normalizedLabels(metadata.labels).join(" ");
  for (const root of roots) {
    const name = root.split("/").pop() ?? root;
    if (new RegExp(`(^|[^a-z0-9])${escapeRegExp(name.toLowerCase())}([^a-z0-9]|$)`).test(text)) {
      scope.add(`${root}/`);
    }
  }

  return [...scope];
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

export function classifyIssue(metadata: IssueMetadata): WorkflowClassification {
  const labels = normalizedLabels(metadata.labels);
  const fullLabel = labels.find((label) => FULL_LABELS.has(label));
  if (fullLabel) {
    return {
      mode: "full",
      reason: `label '${fullLabel}' indicates high-impact or broad work`,
      source: "automatic",
    };
  }

  const text = `${metadata.title}\n${metadata.body}`;
  for (const [pattern, reason] of FULL_TEXT_SIGNALS) {
    if (pattern.test(text)) {
      return { mode: "full", reason, source: "automatic" };
    }
  }

  const files = new Set(referencedFiles(text));
  if (files.size >= 3) {
    return {
      mode: "full",
      reason: `references ${files.size} distinct files, indicating broad scope`,
      source: "automatic",
    };
  }

  const checklistItems = countChecklistItems(metadata.body);
  if (checklistItems >= 4) {
    return {
      mode: "full",
      reason: `contains ${checklistItems} acceptance-criteria items`,
      source: "automatic",
    };
  }

  if (metadata.body.length > 3000) {
    return {
      mode: "full",
      reason: "has a long, detailed description with potentially substantial scope",
      source: "automatic",
    };
  }

  const leanLabel = labels.find((label) => LEAN_LABELS.has(label));
  if (leanLabel) {
    return {
      mode: "lean",
      reason: `label '${leanLabel}' indicates a small documentation or maintenance change`,
      source: "automatic",
    };
  }

  if (/\b(?:chore|docs?|documentation|readme|typo|spelling)\b/i.test(metadata.title)) {
    return {
      mode: "lean",
      reason: "title indicates a documentation or typo-only change",
      source: "automatic",
    };
  }

  if (files.size === 1 && metadata.body.length <= 1200 && checklistItems <= 2) {
    return {
      mode: "lean",
      reason: `scope is localized to ${[...files][0]}`,
      source: "automatic",
    };
  }

  return {
    mode: "full",
    reason: "scope is ambiguous, so the conservative full workflow was selected",
    source: "automatic",
  };
}

export function explicitClassification(mode: WorkflowMode): WorkflowClassification {
  return {
    mode,
    reason: `explicit --${mode} override`,
    source: "explicit",
  };
}

export function parseIssueMetadata(json: string): IssueMetadata {
  let value: GhIssue;
  try {
    value = JSON.parse(json) as GhIssue;
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`Unable to parse issue metadata from GitHub: ${detail}`);
  }
  if (
    typeof value.title !== "string" ||
    (value.body !== null && typeof value.body !== "string") ||
    !Array.isArray(value.labels)
  ) {
    throw new Error("GitHub returned invalid issue metadata.");
  }
  const labels = value.labels.map((label) => {
    if (
      !label ||
      typeof label !== "object" ||
      typeof (label as { name?: unknown }).name !== "string"
    ) {
      throw new Error("GitHub returned an invalid issue label.");
    }
    return (label as { name: string }).name;
  });
  return { title: value.title, body: value.body ?? "", labels };
}
