# Repository map

Commit fixture · 11 tracked files. Built by Grove from the tracked files; read the files themselves for detail.

## Conventions

- Languages: TypeScript (6 files)
- Tests: TypeScript and JavaScript tests (`*.test.*`, `*.spec.*`), 2 files
- Build and test commands:
  - `package.json` scripts: build, test

## Domain documents

### CONTEXT.md

# Bookmarks

**Bookmark**: a URL the user saved, with a title, the tags given to it, and
the time it was added.

**Tag**: a lower-case label on a bookmark. A bookmark has any number of tags;
listing by tag shows the bookmarks that carry it.

**Store**: the JSON file that holds every bookmark, `~/.bookmarks.json`. It is
read whole and written whole on every command.

**Number**: the position of a bookmark in the last `list` output, counting
from 1. `remove` takes a number.

### Decision records

- `docs/adr/0001-store-bookmarks-in-one-json-file.md` — Store bookmarks in one JSON file: All bookmarks are stored in one JSON file, read whole and written whole by every command.

## Layout

```
docs/
  adr/
    0001-store-bookmarks-in-one-json-file.md
src/
  cli.ts
  commands.test.ts
  commands.ts
  store.ts
  tags.test.ts
  tags.ts
CONTEXT.md
README.md
package.json
tsconfig.json
```
