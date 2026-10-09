import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export interface Bookmark {
  url: string;
  title: string;
  tags: string[];
  added: string;
}

export const STORE_PATH = path.join(os.homedir(), ".bookmarks.json");

/** Reads every bookmark. A missing store holds none. */
export function load(file = STORE_PATH): Bookmark[] {
  try {
    return JSON.parse(fs.readFileSync(file, "utf8")) as Bookmark[];
  } catch {
    return [];
  }
}

/** Writes every bookmark, replacing the store. */
export function save(bookmarks: Bookmark[], file = STORE_PATH): void {
  fs.writeFileSync(file, JSON.stringify(bookmarks, null, 2));
}
