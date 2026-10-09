import assert from "node:assert/strict";
import test from "node:test";
import { add, list } from "./commands.js";

test("added bookmarks are listed newest first", () => {
  let bookmarks = add([], "https://a.example", [], new Date("2026-01-01"));
  bookmarks = add(bookmarks, "https://b.example", ["Docs "], new Date("2026-01-02"));
  assert.deepEqual(list(bookmarks).map((b) => b.title), ["b.example", "a.example"]);
  assert.deepEqual(list(bookmarks, "docs").map((b) => b.title), ["b.example"]);
});
