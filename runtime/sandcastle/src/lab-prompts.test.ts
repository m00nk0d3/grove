import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { builtinPromptsDir, PLACEHOLDERS, renderPrompt } from "./lab-prompts.js";
import { validateQuestion } from "./lab-protocol.js";

const STAGE_FILES = ["scout.md", "interview.md", "spec.md", "tickets.md", "shape.md"];
const SECTIONS = ["ROLE", "GOAL", "INPUTS", "PROCEDURE", "RULES", "EXAMPLES", "SELF-CHECK", "WHEN DONE"];
const read = (name: string) => fs.readFileSync(path.join(builtinPromptsDir(), name), "utf8").replace(/\r\n/g, "\n");

/** Headings outside fenced code blocks. */
function headings(text: string): string[] {
  let fenced = false;
  return text.split("\n").filter((line) => {
    if (/^\s*```/.test(line)) fenced = !fenced;
    return !fenced && /^## /.test(line);
  });
}

test("every stage prompt has the eight sections, in order", () => {
  for (const file of STAGE_FILES) {
    assert.deepEqual(headings(read(file)).map((h) => h.slice(3).trim()), SECTIONS, file);
  }
  assert.deepEqual(headings(read("protocol.md")), [], "the protocol is a fragment");
});

test("prompts use only the placeholders the README documents", () => {
  const readme = read("README.md");
  for (const name of PLACEHOLDERS) assert.ok(readme.includes(`\`{{${name}}}\``), `README documents ${name}`);
  for (const file of [...STAGE_FILES, "protocol.md"]) {
    const { unknown } = renderPrompt(read(file), Object.fromEntries(PLACEHOLDERS.map((p) => [p, "x"])));
    assert.deepEqual(unknown, [], file);
  }
  for (const file of ["interview.md", "shape.md"]) assert.match(read(file), /\{\{protocol\}\}/, file);
});

test("every example card in the prompts is a valid card", () => {
  for (const file of ["protocol.md", "interview.md", "shape.md"]) {
    const blocks = [...read(file).matchAll(/```json\n([\s\S]*?)```/g)].map((m) => m[1]);
    for (const block of blocks) {
      const value = JSON.parse(block) as { id?: number; kind?: string };
      if (value.kind === undefined) continue; // coverage and other examples
      const result = validateQuestion(block, value.id ?? -1);
      assert.ok(result.ok, `${file}: ${block}\n${result.ok ? "" : result.error}`);
    }
  }
});

test("rendering inserts values as they are, never expanding them again", () => {
  const { text, unknown } = renderPrompt("A {{repo_map}} B {{nope}}", { repo_map: "{{stage}}" });
  assert.equal(text, "A {{stage}} B {{nope}}");
  assert.deepEqual(unknown, ["nope"]);
});
