# Store bookmarks in one JSON file

## Status

Accepted.

## Context

The tool runs one command at a time on one machine. A database would add an
install step for a few hundred records.

## Decision

All bookmarks are stored in one JSON file, read whole and written whole by
every command.

## Consequences

Two commands running at once can overwrite each other's changes. This is
accepted while the tool is single-user.
