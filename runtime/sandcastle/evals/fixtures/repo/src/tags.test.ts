import assert from "node:assert/strict";
import test from "node:test";
import { normalizeTags } from "./tags.js";

test("tags are trimmed, lower-case, and unique", () => {
  assert.deepEqual(normalizeTags([" Go", "go", "", "Rust"]), ["go", "rust"]);
});
