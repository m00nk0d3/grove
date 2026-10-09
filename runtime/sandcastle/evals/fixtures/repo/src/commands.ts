import { type Bookmark } from "./store.js";
import { hasTag, normalizeTags } from "./tags.js";

export function add(bookmarks: Bookmark[], url: string, tags: string[], now: Date): Bookmark[] {
  const title = new URL(url).hostname;
  return [...bookmarks, { url, title, tags: normalizeTags(tags), added: now.toISOString() }];
}

/** The bookmarks as list shows them: newest first, optionally one tag. */
export function list(bookmarks: Bookmark[], tag?: string): Bookmark[] {
  const shown = tag ? bookmarks.filter((b) => hasTag(b.tags, tag)) : bookmarks;
  return [...shown].sort((a, b) => b.added.localeCompare(a.added));
}

/** Removes the bookmark shown at number in the last list, counting from 1. */
export function remove(bookmarks: Bookmark[], number: number): Bookmark[] {
  return bookmarks.filter((_, index) => index !== number - 1);
}
