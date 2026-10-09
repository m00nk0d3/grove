/** Normalizes tags: trimmed, lower-case, without duplicates. */
export function normalizeTags(tags: string[]): string[] {
  return [...new Set(tags.map((tag) => tag.trim().toLowerCase()).filter((tag) => tag !== ""))];
}

export function hasTag(tags: string[], tag: string): boolean {
  return tags.includes(tag.trim().toLowerCase());
}
