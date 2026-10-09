// Grove's Lab stage prompts: loading them, with a repository's overrides, and
// filling their placeholders. The templates live in prompts/lab/, whose
// README is the contract for every placeholder.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export type LabStage = "scout" | "interview" | "spec" | "tickets" | "shape";

export const PLACEHOLDERS = [
  "repo",
  "entry_kind",
  "entry_title",
  "entry_text",
  "stage",
  "done_file",
  "entry_dir",
  "artifacts_dir",
  "repo_map",
  "scout_file",
  "scout_notes",
  "shaped_report",
  "questions_dir",
  "next_question_number",
  "next_question_path",
  "interview_so_far",
  "coverage_file",
  "coverage",
  "spec_file",
  "spec",
  "tickets_file",
  "issue_file",
  "previous_draft",
  "next_adr_number",
  "protocol",
] as const;

export type PromptValues = Record<(typeof PLACEHOLDERS)[number], string>;

/** The value of a placeholder whose content does not exist. */
export const NONE = "(none)";

/** The prompts shipped with the runtime, beside dist/. */
export function builtinPromptsDir(): string {
  return path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "prompts", "lab");
}

/**
 * Reads a prompt file: the repository's own copy under .grove/lab/prompts
 * when it has one, otherwise the built-in prompt.
 */
export function loadPromptTemplate(name: string, repo: string, builtinDir = builtinPromptsDir()): string {
  const override = path.join(repo, ".grove", "lab", "prompts", name);
  for (const file of [override, path.join(builtinDir, name)]) {
    try {
      return fs.readFileSync(file, "utf8").replace(/\r\n/g, "\n");
    } catch {
      // Try the next location.
    }
  }
  throw new Error(`The Lab prompt ${name} was not found in ${builtinDir}`);
}

/**
 * Fills {{name}} placeholders. Values are inserted as they are, so a value
 * that itself contains {{…}}, such as a repository file quoted in the map, is
 * never expanded. Unknown placeholders are left in place and reported.
 */
export function renderPrompt(template: string, values: Partial<PromptValues>): { text: string; unknown: string[] } {
  const unknown = new Set<string>();
  const text = template.replace(/\{\{(\w+)\}\}/g, (whole, name: string) => {
    const value = (values as Record<string, string | undefined>)[name];
    if (value === undefined) {
      unknown.add(name);
      return whole;
    }
    return value;
  });
  return { text, unknown: [...unknown] };
}

/** A stage prompt, with protocol.md rendered into it for the stages that ask. */
export function buildStagePrompt(stage: LabStage, repo: string, values: Omit<PromptValues, "protocol">, builtinDir?: string): string {
  const protocol = renderPrompt(loadPromptTemplate("protocol.md", repo, builtinDir), values).text;
  const { text, unknown } = renderPrompt(loadPromptTemplate(`${stage}.md`, repo, builtinDir), { ...values, protocol });
  if (unknown.length > 0) {
    throw new Error(`The Lab prompt ${stage}.md uses unknown placeholders: ${unknown.join(", ")}`);
  }
  return text;
}
