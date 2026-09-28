# Lab Design

## Status

Accepted. This document replaces the Lab design in epic #228 and its tickets
(#229–#235). The Lab is being rebuilt against it; the earlier implementation is
superseded.

## Date

2026-09-28

## Purpose

The Lab is where work lives before it becomes a GitHub issue. A captured idea
or bug is taken through the build chain — **grill-with-docs → to-spec →
to-tickets** — by an agent in a Herdr pane, reviewed artifact by artifact in
Grove, and published as an epic with sub-issues, or, for a bug, as a single
issue.

## Scope

- Capture ideas and bugs per repository.
- Grill an entry: interview, shared context (`CONTEXT.md`), decision records
  (ADRs), spec, and tracer-bullet tickets.
- Review and approve each artifact.
- Publish an epic and its sub-issues with native blocked-by links, placed in
  `Backlog`.
- Shape a bug into a structured report and publish it as one issue.
- Escalate a shaped bug into a full grill.
- Archive, restore, and delete entries; navigate from a published entry to its
  issues; prevent two Grove instances from running or publishing the same entry.

## Storage

Lab state lives in the git common directory, so it is shared by every worktree
of the repository, belongs to no branch, and is never tracked or committed:

```
<git-common-dir>/grove-lab/
  entries.json               index of all entries
  <entry-id>/
    artifacts/               drafts written by the agent
      CONTEXT.md
      adr/NNNN-<slug>.md
      spec.md
      tickets.json
      issue.md               shaped bug report
    session.lock             held while a run starts or publishing is in progress
```

`entries.json` is written atomically (temporary file and rename). Deleting a
clone deletes its Lab; anything worth keeping has been published by then.

## Entry model

| Field      | Meaning                                                             |
|------------|---------------------------------------------------------------------|
| `id`       | Stable identifier, generated once at capture                        |
| `kind`     | `idea` or `bug`                                                     |
| `text`     | The captured text; its first line is the title                      |
| `status`   | Lifecycle state, below                                              |
| `mode`     | `grill` or `shape`, set when a run first starts                     |
| `archived` | Hidden from active views; independent of `status`                   |
| `issues`   | Published issue numbers: `epic` and `tickets`, or the single `issue` |
| `runs`     | Sandcastle run IDs, oldest first                                    |
| `created`, `updated` | Timestamps                                                |

Lifecycle:

```
draft ──grill──▶ grilling ──▶ specced ──▶ ticketed ──publish──▶ published
  │                 ▲
  └──shape (bug)──▶ shaping ──publish──▶ published
                    └──escalate──▶ grilling
```

Whether a session is live is read from Sandcastle and Herdr at runtime, never
stored, so it cannot go stale across restarts.

## Runs

Every session is a Sandcastle workflow run, shown and tracked like any other:

- `grill`: steps **Interview → Spec → Tickets → Publish**.
- `shape`: steps **Shape → Publish**.

A run whose agent is waiting for the user is `blocked`, and appears as
*Waiting on you* in the Lab and in the Dashboard's *Attention* list. The entry
links to its runs; the run is the source of truth for status, current step, and
pane.

The agent is `[sandcastle].default_agent`. It runs in the base checkout, may
read the repository freely, and writes only to the entry's `artifacts/`
directory. It reports its stage (`interview`, `spec`, `tickets`, `done`) to a
state file that the runtime turns into step telemetry. It never publishes.

Conversations with the agent happen only in its Herdr pane. Grove does not read
from or type into the pane; it links to it.

### Skills

Pinned copies of grill-with-docs (grilling and domain-modeling), to-spec, and
to-tickets, from mattpocock/skills (MIT), ship with the Sandcastle runtime. A
Grove prompt layer on top sets the output paths, the `tickets.json` schema, and
the stage markers, and replaces each skill's publishing step with "stop at
drafted". Shaping uses its own prompt with a fixed template: Summary, Steps to
reproduce, Expected, Actual, Environment, Notes. The shaping agent asks for
missing information rather than inventing it.

## User interface

The Lab is a navigation rail tab (`l`).

**List.** Styled like the Dashboard's workflow list, with tabs
`[ ACTIVE / ATTENTION ]`, `[ DRAFTS ]`, `[ PUBLISHED ]`, and `[ ARCHIVED ]`,
switched with `[` and `]`. Rows take two lines: a state marker, title, and status
badge; then kind, stage, pane, and age. Keys `1`, `2`, and `3` filter by all,
ideas, and bugs within the current tab.

**Keys.** `c` captures a new entry. Enter performs the entry's next step: grill
(or shape, for a bug) a draft, open the pane of a live run, review pending
artifacts, or open a published entry's epic in the Issues tab. Every other
operation is in the Actions panel, which lists only what the entry's state
allows. The Lab defines no other letter keys, so global keys behave the same in
every view.

**Inspector.** `v` opens the inspector for the selected entry: header cards for
stage, elapsed time, artifacts, and issues, and tabs *Overview*, *Steps*,
*Artifacts*, and *Capture*.

## Review and approval

Each artifact is `draft`, `approved`, or `discarded`. In the *Artifacts* tab:

- **Approve** marks it approved. An approved `CONTEXT.md` or ADR is written to
  its normal path in the selected checkout, default the base checkout, as an
  uncommitted change; an existing `CONTEXT.md` is merged, not overwritten.
- **Edit** opens it in `$EDITOR`; it remains a draft.
- **Discard** excludes it.
- **Request changes** opens the agent's pane; the agent revises the draft in
  place.

Publishing is available once the spec and tickets are approved. `CONTEXT.md`
and ADRs never block it. Grove never commits.

## Publishing

Publishing is performed by the run's Publish step in code, through `gh` and the
user's `gh` authentication, never by the agent:

1. A preview lists the epic, each ticket, the blocked-by edges, labels, and the
   board. Nothing is written until the user confirms.
2. The epic is created from the spec, with a *Glossary and decisions* section
   summarising the approved `CONTEXT.md` terms and ADRs, and labelled `epic`.
3. Tickets are created in dependency order, labelled `ready-for-agent`, and
   attached to the epic as native sub-issues.
4. Blocked-by relationships are set through GitHub's native issue dependencies.
5. Every issue is added to the project named by `[lab] project` and set to
   `Backlog`. When the key is empty and the repository is linked to exactly one
   project, that project is used; otherwise the user is asked once and the
   answer is saved.
6. Each issue number is recorded on the entry as soon as it is created, so a
   failed publish resumes instead of creating duplicates.

A shaped bug publishes one issue labelled `bug`, through the same path.

## Escalation

*Escalate to grill* ends a shaping run and starts a grill run seeded with the
shaped report. It is also available after the bug is published; the bug issue
is kept and becomes a sub-issue of the new epic.

## Lifecycle end

- **Archive** is available in any state and moves the entry to the Archived
  tab, hiding it elsewhere, including the Dashboard. If a run is live, Grove
  asks whether to stop it. **Restore** returns the entry unchanged.
- **Delete** is available for drafts and archived entries without a live run.
  It removes the entry's directory after confirmation. Published issues are not
  affected.
- **Navigation.** For a published entry, Enter opens the Issues tab with the
  epic selected. *Open on GitHub* opens it in the browser.

## Concurrency

`session.lock` records the owning process ID, host, and time, and is held only
while a run is being started or an entry is being published. A second Grove
instance refuses those operations with the owner's host and process ID. Viewing
an entry and opening its pane are never blocked. A lock whose process is no
longer running on the same host is taken over automatically; a lock from
another host can be cleared from the Actions panel.

## Delivery

The Lab is built in working slices, each usable on its own:

1. Storage and capture.
2. The Lab tab and inspector.
3. Shaping and publishing.
4. Grilling and approval.
5. Escalation and lifecycle.
