# Lab Design

## Status

Accepted. This document replaces the Lab design of 2026-09-28, which itself
replaced epic #228 and its tickets (#229–#235). Storage, the entry model,
review by content hash, publishing, escalation, lifecycle, and concurrency
carry over from that design; the session protocol, the agent instructions,
and the user interface are new.

## Date

2026-10-09

## Purpose

The Lab is where work lives before it becomes a GitHub issue. A captured idea
or bug is taken through a build chain — **scout → interview → spec →
tickets** for an idea, **shape** for a bug — by an agent in a Herdr pane, and
published as an epic with sub-issues, or as a single issue.

The user conducts the whole session inside Grove. The agent asks its questions
as structured cards, Grove shows them and records the answers, and the
session runtime delivers those answers to the agent. The Herdr pane remains
available for watching the agent, but nothing requires the user to type in it.

The agent instructions are Grove's own and are written to work with small,
locally hosted models as well as large hosted ones: one stage per prompt,
explicit procedures, exact output formats, and worked examples. Every stage
is given a map of the repository and, after scouting, notes on the code the
entry touches, so the agent asks questions the code cannot already answer.

## Scope

- Capture ideas and bugs per repository.
- Grill an idea: scout the code, interview the user through question cards,
  draft a spec, tickets, and repository documents (`CONTEXT.md`, decision
  records).
- Shape a bug into a structured report through the same question cards.
- Review each draft in Grove, request changes in Grove, and approve it.
- Publish an epic and its sub-issues with native blocked-by links, placed in
  `Backlog`, or a single bug issue.
- Escalate a shaped bug into a full grill.
- Archive, restore, and delete entries; navigate from a published entry to its
  issues; prevent two Grove instances from running or publishing the same
  entry.
- Evaluate the agent instructions against a chosen model.

## Storage

Lab state lives in the git common directory, so it is shared by every worktree
of the repository, belongs to no branch, and is never tracked or committed:

```
<git-common-dir>/grove-lab/
  entries.json               index of all entries
  repo-map/<commit>.md       repository map for a commit, built by Grove
  <entry-id>/
    scout.md                 the Scout stage's notes on the code involved
    coverage.json            the Interview stage's topic coverage
    questions/
      NNN.json               a question, written by the agent
      NNN.answer.json        its answer, written by Grove
      NNN.sent               delivery receipt, written by the runtime
    requests/
      NNN.json               a message from Grove to the agent
      NNN.sent               delivery receipt, written by the runtime
    artifacts/               drafts written by the agent
      CONTEXT.md             repository documents, at their repository-relative
      docs/adr/NNNN-<slug>.md  paths; approving one copies it there
      spec.md                the epic
      tickets.json           the epic's tickets
      issue.md               a shaped bug report
    stage                    the active stage, written by the runtime
    done                     the stage the agent has finished, written by the agent
    session.json             the live session's phase and recent output, written by the runtime
    <run-id>.close           written by Grove to end that run's session
    session.lock             held while a run starts or publishing is in progress
```

`entries.json` is written atomically (temporary file and rename). Answer and
request files are created exclusively: a second writer finds the file present
and fails, so a question is answered once. Every file under an entry belongs
to the entry, not to a run, and survives a run ending; deleting the entry
removes it. Deleting a clone deletes its Lab; anything worth keeping has been
published by then.

Ownership is strict: the agent writes only `scout.md`, `coverage.json`,
`questions/NNN.json`, `artifacts/`, and `done`; Grove writes `entries.json`,
answers, requests, and close files; the runtime writes `stage`,
`session.json`, and the `.sent` receipts. No file has two writers.

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
| `reviews`  | Review decisions by artifact path, each with the content hash it was made on |
| `created`, `updated` | Timestamps                                                |

Lifecycle:

```
draft ──grill──▶ grilling ──▶ specced ──▶ ticketed ──publish──▶ published
  │                 ▲
  └──shape (bug)──▶ shaping ──publish──▶ published
                    └──escalate──▶ grilling
```

`grilling` covers the Scout and Interview stages. Whether a session is live is
read from Sandcastle and Herdr at runtime, never stored, so it cannot go stale
across restarts.

## Sessions and stages

Every session is a Sandcastle workflow run, shown and tracked like any other:

- `grill`: steps **Scout → Interview → Spec → Tickets → Publish**.
- `shape`: steps **Shape → Review → Publish**.

A session is the `grove-lab <shape|grill> <entry-id>` command, which
`grove-sandcastle workflow start --kind <shape|grill> --entry <id>` runs in the
run's Herdr tab. It opens a pane beside it and runs the stages in that pane.

### One agent per stage

Each stage is carried out by a fresh agent with its own prompt. When an agent
finishes its stage, the runtime ends it and starts the next stage's agent in
the same pane. Files are the only memory between stages: each prompt is
self-contained and carries everything its stage needs — the repository map,
the scout notes, the decisions so far, and the previous stage's draft. This
keeps each agent's context small and focused, which matters most for small
models, and makes a session resumable at any point.

Every stage uses `[sandcastle].default_agent` and its configured model.

The agent signals the end of its stage by writing the stage's name to `done`.
The runtime writes the active stage's name to `stage` when it starts that
stage's agent; Grove reads it for display.

| Stage     | Ends when                                     | Next                     |
|-----------|-----------------------------------------------|--------------------------|
| Scout     | `done` is `scout`                             | Interview, immediately   |
| Interview | `done` is `interview`, or Grove asks to finish | Spec, immediately        |
| Spec      | `done` is `spec`                              | Tickets, once `spec.md` is approved |
| Tickets   | `done` is `tickets`                           | Publish, once `tickets.json` is approved |
| Shape     | `done` is `shape`                             | Publish, once `issue.md` is approved |

The runtime reads approvals from `entries.json`: a draft counts as approved
when its recorded hash matches its current content. While a draft waits for
review, its stage's agent stays open, so change requests reach the agent that
wrote it.

Reopening an approved spec — the *Reopen spec* action — ends the Tickets agent
and returns the session to the Spec stage; the existing `tickets.json` is kept
and given to the next Tickets agent as a prior draft.

### Run state

A run is `blocked` whenever it waits on the user: an unanswered question, a
draft to review, a permission request, or a turn that ended without a
question (below). It appears as *Waiting on you* in the Lab and in the
Dashboard's *Attention* list. The run is the source of truth for status,
current step, and pane.

The runtime records in `session.json`, per stage, the agent and model used,
the number of repair prompts sent, and the number of fallback turns, so
instruction-following can be compared across models. `session.json` also
holds the session's phase — `working`, `question`, `fallback`,
`permission`, or `review` — and, for `fallback` and `permission`, the
agent's recent output, so Grove shows the session without reading the pane
itself. For a draft presented for review with problems the agent did not
repair, it lists those problems.

## Question protocol

The agent never asks the user anything in prose. It writes a question card
and ends its turn.

### Question card

`questions/NNN.json`, numbered from `001`:

```json
{
  "id": 3,
  "kind": "choice",
  "question": "Who can trigger a publish?",
  "context": "lab_publish.go holds session.lock only while publishing; any Grove instance can start one.",
  "options": ["Only the entry owner", "Anyone with write access"],
  "recommended": 0,
  "why": "Prevents two users publishing the same entry."
}
```

| Field         | Rules                                                              |
|---------------|--------------------------------------------------------------------|
| `id`          | Equals the file's number                                           |
| `kind`        | `choice` (pick one), `multi` (pick any), or `text`                 |
| `question`    | One question, one sentence                                         |
| `context`     | What the agent found that makes the question necessary; cites a file, or states why the code cannot answer it |
| `options`     | 2–4 strings for `choice` and `multi`; absent for `text`            |
| `recommended` | An option index for `choice`, a list of indices for `multi`, a suggested answer string for `text` |
| `why`         | One sentence on why the recommendation is right                    |

A question with two options covers yes-or-no; there is no separate kind. Every
card also accepts a typed note, so an answer outside the options needs no
kind of its own.

### Answer

`questions/NNN.answer.json`, written by Grove:

```json
{
  "id": 3,
  "choices": [0],
  "text": "Admins can still force-unlock.",
  "answered_at": "2026-10-09T10:14:00Z",
  "revisions": []
}
```

When the runtime delivers an answer it writes the receipt `NNN.sent`:

```json
{ "revision": 0, "via": "grove", "sent_at": "2026-10-09T10:14:02Z" }
```

`revision` is the number of revisions the delivered answer had, so a revised
answer is delivered again. `via` is `grove` for an answer given in Grove,
`pane` for a question the agent moved past without one (below), and
`skipped` for a question still open when the user ended the interview.

### Delivery

Grove only writes answer and request files. The session runtime, which already
polls the agent every second, delivers them: when an answer or request has no
current receipt and the agent is idle, it submits it with `herdr agent prompt`
and writes the receipt. The user can answer the moment a card appears; delivery
waits for the agent to be ready. Only the runtime prompts the agent, so nothing
is ever sent twice, and an answer given while Grove is closing or after the
agent crashed is still delivered.

The prompt states the answer and the exact next step, so the agent never has to
count questions or recall a path:

```
Answer to question 3: "Only the entry owner" (option 1).
User note: "Admins can still force-unlock."
Next: write question 4 to <absolute path>/questions/004.json and stop,
or, if the interview is complete, write "interview" to <absolute path>/done.
```

### Several questions in one turn

The agent is instructed to ask one question per turn. If it writes several,
Grove shows them in order. When every pending card is answered, the runtime
sends all the answers in one prompt, and the next question number continues
from the highest written.

### Repair

Grove validates each card when it appears. An invalid card — unparseable JSON,
a missing field, a recommendation out of range — is not shown. The runtime
sends a repair prompt naming the file and the exact error, and asks the agent
to rewrite the file and stop:

```
questions/004.json is invalid: "recommended" is 3 but there are 2 options.
Rewrite <absolute path>/questions/004.json and stop.
```

After two failed repairs of the same card, the turn is treated as a turn
without a question.

### A turn without a question

When the agent's turn ends without a new valid card and without writing
`done`, Grove shows a fallback card: the agent's last thirty lines of output,
which the runtime reads with `herdr agent read` and records in
`session.json`, and a reply box. The reply is delivered as a
request, followed by the protocol reminder for the next question number. No
automatic nudge is sent first: the output is usually a genuine question asked
in prose.

### Revising an answer

During the Interview stage, any answered question can be answered again from
the decisions log. Grove rewrites the answer file, moving the previous answer
into `revisions`, and the runtime delivers:

```
Revision to question 2: was "git common directory", now "per worktree".
If any later question or answer depended on it, ask about it again.
Next: write question 8 to <absolute path>/questions/008.json and stop.
```

From the Spec stage on, the log is read-only. A changed decision is then a
change request on the spec.

### Answers given in the pane

Grove is the only supported place to answer, and the interface never points
the user to the pane for that. Typing in the pane cannot be prevented, so the
following keeps a session from getting stuck: when the agent starts a turn
that the runtime did not prompt, and that turn ends with a newer question or
with `done`, every card that was waiting before the turn is closed by a
receipt whose `via` is `pane`. The decisions log shows it as answered in the
pane.

### Requests

Every other message from Grove to the agent is a request, `requests/NNN.json`:

| `kind`             | Sent when                                     | Delivered as |
|--------------------|-----------------------------------------------|--------------|
| `reply`            | The user answers a fallback card              | The text, then the protocol reminder |
| `change`           | The user requests changes to a draft          | The text and the draft's path, with instructions to revise in place and write `done` again |
| `finish_interview` | The user chooses *Write the spec now*         | An instruction to mark every uncovered topic `open` in `coverage.json` and write `done` |
| `permission`       | The user answers a permission card            | Keystrokes, below |

Requests are delivered exactly like answers.

## Permissions

The agent's job is narrow and known in advance: read the repository, run
read-only commands, and write under its entry's directory. The runtime starts
each backend with a permission configuration that allows exactly that and
denies everything else without asking:

- reading any file in the base checkout;
- listing and searching files;
- read-only `git` commands (`log`, `show`, `diff`, `grep`, `blame`, `ls-files`);
- writing files under `<git-common-dir>/grove-lab/<entry-id>/`.

Each backend — Claude, OpenCode, Pi — is configured through its own permission
mechanism. A backend that cannot be configured this way, or an agent that asks
anyway, produces a Herdr `blocked` state; Grove then shows a permission card
with the agent's last output and **Allow** and **Deny**, and the runtime
answers the prompt with `herdr agent send-keys`. That path drives a terminal
interface and is a fallback, not the design.

## Agent instructions

### Prompts

Grove's agent instructions are its own. They live in
`runtime/sandcastle/prompts/lab/`:

| File            | Stage                                              |
|-----------------|----------------------------------------------------|
| `scout.md`      | Scout                                              |
| `interview.md`  | Interview                                          |
| `spec.md`       | Spec, including repository documents               |
| `tickets.md`    | Tickets                                            |
| `shape.md`      | Shape                                              |
| `protocol.md`   | The question card format and rules, included by `interview.md` and `shape.md` |

Every prompt has the same sections in the same order, so a model meets one
shape throughout:

1. **ROLE** — one line.
2. **GOAL** — one sentence describing a finished stage.
3. **INPUTS** — the injected material, each part between clear delimiters.
4. **PROCEDURE** — numbered steps, in the order the agent performs them.
5. **RULES** — imperative and checkable; no "should", "try to", or "consider".
6. **EXAMPLES** — two or three good outputs and one or two bad ones, each bad
   one followed by why it is bad.
7. **SELF-CHECK** — a checklist the agent runs before writing any file.
8. **WHEN DONE** — the exact file to write and the instruction to stop.

Each prompt is around three to four thousand tokens. Placeholders
(`{{entry_dir}}`, `{{next_question_path}}`, `{{repo_map}}`, …) are filled by the
runtime; the agent is never asked to derive a path.

A repository can replace any prompt with `.grove/lab/prompts/<file>` in its
base checkout, to tune instructions for a project or a model without rebuilding
Grove.

### Sources

The prompts are written for Grove and draw on:

- the interview discipline of the `grill-me` skill — one question per turn,
  depth-first through the decision tree, dependencies settled before the
  decisions that hang off them, the code explored before a question is asked,
  and a recommended answer with every question;
- Matt Pocock's skills (MIT): from `grilling`, that finding facts is the
  agent's job and making decisions is the user's; from `domain-modeling`, the
  `CONTEXT.md` and decision-record formats and recording terms as soon as they
  are settled; from `to-spec`, user stories and testing seams; from
  `to-tickets`, vertical slices, prefactoring first, and expand–contract for
  wide refactors.

The pinned copies in `runtime/sandcastle/skills/mattpocock/` are removed. Their
MIT license notice moves to `runtime/sandcastle/prompts/lab/NOTICE`.

The worked examples in `interview.md` are drawn from real grilling sessions,
including the one that produced this design.

## Repository context

### Repository map

When a run starts, Grove builds a map of the base checkout at its current
commit, without a model, and stores it as `repo-map/<commit>.md`. An existing
map for the same commit is reused. Every stage of the run receives the same
map, so all stages see one consistent view of the code.

The map contains:

- the file tree, honoring `.gitignore`, with vendored and generated directories
  collapsed to one line;
- one line on the purpose of each directory or package, from its package
  documentation comment, its README's first paragraph, or its module header;
- every `CONTEXT.md` in full, and each decision record by title and decision;
- the languages used, where tests live, and the build and test commands found
  in the `Makefile`, `package.json`, or equivalent.

The map is held to a token budget, `[lab] repo_map_tokens`, by default 8,000.
When the repository does not fit, the tree is trimmed from the deepest levels
up, and trimmed directories are listed with their file counts.

A typical prompt in a 40,000-token context window is budgeted as follows:

| Part                                         | Tokens   |
|----------------------------------------------|----------|
| Stage instructions and examples              | ~4,000   |
| Repository map                               | ~8,000   |
| Scout notes                                  | ~4,000   |
| Decisions so far, or the previous draft      | ~6,000   |
| Working room: file reads, turns, output      | ~18,000  |

### Scout

The Scout stage reads the code the entry concerns before anyone is asked
anything. Given the entry text and the map, the agent finds and reads the
relevant code and writes `scout.md`, of at most about 4,000 tokens:

- **Relevant files** — each path with one line on why it matters.
- **How it works today** — short points, each citing `file:line`.
- **Constraints** — existing patterns, decision records, and tests that limit
  the design.
- **Open questions** — what the code cannot answer; the interview's agenda.

`scout.md` is not reviewed. It is shown in the entry page, so the user can see
what the questions are based on, and is given to every later stage.

## Interview

The Interview prompt receives the map, `scout.md`, and the decisions so far.
It asks one question per turn, works depth-first, settles each decision before
the decisions that depend on it, and gives a recommendation with every
question. It never asks what `scout.md` or the code already answers: every
card's `context` cites what the agent found or states why the code cannot
answer it.

### Coverage

The agent keeps `coverage.json`, one entry per topic:

```json
{
  "scope": "covered",
  "triggers": "covered",
  "data": "covered",
  "interface": "covered",
  "errors": "covered",
  "concurrency": "n/a: a single user edits an entry",
  "compatibility": "open",
  "testing": "open",
  "rollout": "open"
}
```

The topics are scope and non-goals, triggers and users, data and storage,
interface and experience, errors and edge cases, concurrency, compatibility
and migration, testing, and rollout. Each is `covered`, `open`, or `n/a:`
followed by a reason.

The agent may finish the interview only when every topic is `covered` or
`n/a`, and every open question in `scout.md` has been answered. Grove shows
coverage as a strip above the card, such as `6/9 topics`.

After twenty-five questions, the agent's next card must be a `choice` asking
whether there is enough to write the spec.

The user can end the interview at any time with *Write the spec now*. The
agent marks every remaining topic `open` and finishes; the Spec stage lists
them under *Open questions*.

## Drafts

### Spec

`spec.md` has these sections, in order:

1. `# <title>`
2. **Problem** — from the user's perspective.
3. **Goals** and **Non-goals**.
4. **User stories** — numbered, *As a …, I want …, so that …*.
5. **Decisions** — one point per answered question, citing it (`Q3`).
6. **Design** — how the change fits the existing code, citing files from
   `scout.md`.
7. **Testing** — the seams the change is tested at, preferring existing seams
   and the highest one available, and prior art among the existing tests.
8. **Edge cases**.
9. **Open questions** — every topic left `open`, and anything unresolved.

The Spec stage also drafts repository documents where the interview settled
them: new or changed glossary terms in `CONTEXT.md`, and a decision record for
each decision that is hard to reverse, surprising without context, and the
result of a real trade-off.

### Tickets

`tickets.json` is `{"tickets": [{"key", "title", "body", "blocked_by"}]}`.
Each ticket is a tracer bullet: a narrow, complete path through every layer,
demoable on its own, and small enough for one agent's fresh context.
Prefactoring comes first. A wide mechanical refactor is sequenced as
expand–contract: add the new form beside the old, migrate callers in batches,
then remove the old form.

Each body has these sections: **What**, **Why**, **Where** (the files to
change, from `scout.md`), **Acceptance criteria** (a checklist), and **Out of
scope**.

### Shaped bug

`issue.md` has these sections: `# <title>`, **Summary**, **Steps to
reproduce**, **Expected behaviour**, **Actual behaviour**, **Environment**,
**Component** (the code involved, citing files), and **Notes**.

### Validation

Grove validates drafts when the agent writes `done`. A spec or bug report
missing a section, a ticket body missing a section, or a ticket set with a
missing key or title, a repeated key, an unknown or self-referencing blocker,
or a cycle is not presented for review. The runtime sends a repair prompt
naming each problem, as for question cards. After two failed repairs the draft
is presented with the problems listed above it, and approval is refused until
they are fixed.

## User interface

The Lab is a navigation rail tab (`l`). It has two views: the list and the
entry page.

### List

One list, grouped by what each entry needs:

| Group           | Entries                                                    | Row action                     |
|-----------------|------------------------------------------------------------|--------------------------------|
| **NEEDS YOU**   | An unanswered question, permission, or fallback card; a draft to review; ready to publish; a failed run or interrupted publication; a session that stopped before finishing | `Answer question 4`, `Allow a command`, `Reply to the agent`, `Review spec`, `Publish 6 issues`, `Retry`, `Resume grill` |
| **WORKING**     | A live run whose agent is working                          | `Scouting…`, `Writing spec…`   |
| **NOT STARTED** | Captured, never run                                        | `Start grill`, `Shape bug`     |
| **DONE**        | Published                                                  | `Open #123`                    |

*NEEDS YOU* is ordered by how long each entry has waited, longest first; the
other groups, newest first. *DONE* shows the five most recent entries and
expands when its header is selected. Rows take two lines: a state marker,
title, and status badge; then the row action, stage, kind, and age.

`1`, `2`, and `3` filter by all, ideas, and bugs. `0` toggles between active
and archived entries. `c` captures a new entry as *Idea → Grill* or *Bug →
Report*. Enter opens the entry page.

### Entry page

The entry page replaces the list within the Lab tab; `Esc` returns to it. It
has three parts:

- **Stepper** — the run's steps with the current one marked, such as
  `Scout ✓ ─ Interview 6/9 ─ Spec ─ Tickets ─ Publish`.
- **Panel** — what the current step needs from the user: a question card, a
  draft to review, the publish preview, or the agent's status while it works.
- **Decisions log** — every answered question with its answer, most recent
  first; questions answered in the pane are marked as such.

Enter always performs the panel's primary action. Less common actions —
*Archive*, *Delete*, *Escalate to grill*, *Reopen spec*, *Write the spec now*,
*View agent*, *Open on GitHub*, *End session*, *Clear lock* — are in the
actions menu, opened with `.`, which lists only what the entry's state allows.
*View agent* focuses the Herdr pane.

While the entry page is open it owns the keyboard: global keys are suspended
except `Esc`, `?`, and `q`, which asks for confirmation. Whatever has focus
receives every other key.

### Question card

```
┌ Interview · question 3 ──────────────── 6/9 topics ┐
│ Who can trigger a publish?                          │
│ lab_publish.go holds session.lock only while        │
│ publishing; any Grove instance can start one.       │
│                                                     │
│ ▸ 1. Only the entry owner            recommended    │
│   2. Anyone with write access                       │
│                                                     │
│ Why: prevents two users publishing the same entry.  │
│ Note: _                                             │
└─────────────────────────────────────────────────────┘
```

`↑`/`↓` or `1`–`4` choose an option (`Space` toggles one for `multi`), `Tab`
moves focus from the options to the note to the decisions log, and Enter
submits. In the note, every key is text; `Esc` leaves it. On the decisions
log, Enter on an answered question opens it for revision while the interview
lasts. Fallback and permission cards use the same frame, with the agent's
output in place of options.

## Review and approval

Each artifact is `draft`, `approved`, or `discarded`. A decision is recorded
with a hash of the content it was made on: when the agent revises the file, the
decision no longer applies and the artifact is a draft again, so what is
published is always what the user approved.

Review is a gate between stages. The panel shows one draft at a time:

- **Spec** — the rendered `spec.md`. Enter approves it and starts the Tickets
  stage. `c` opens a box for a change request, delivered to the Spec agent;
  the panel shows the agent revising until the new draft arrives. `e` edits
  the draft in `$VISUAL` or `$EDITOR`; it remains a draft until approved.
- **Tickets** — the tickets as a list, indented by their blocked-by
  relationships. `↑`/`↓` select a ticket and `→` expands its body. `c` on a
  selected ticket opens the change box beginning with its key, such as
  `T3: `; the same box takes feedback on the whole set, including its
  granularity and dependencies. Enter approves the set and shows the publish
  preview. `e` edits `tickets.json`.
- **Bug report** — `issue.md`, reviewed like the spec; Enter approves it and
  shows the publish preview.

Repository documents appear beneath the spec as *Also drafted*, each shown as
a difference from the checkout's existing file. `y` approves one, copying it
to its path in the base checkout as an uncommitted change; `n` discards it.
The agent starts from the checkout's existing file, so a draft extends it;
Grove replaces an existing file only when the draft keeps every one of its
lines, and otherwise refuses the approval, naming a line that would be lost.
Paths outside the checkout or inside `.git` are refused. Repository documents
never block publishing. Grove never commits.

## Publishing

Publishing is the last step of the stepper. It is performed by Grove in code,
through `gh` and the user's `gh` authentication, never by the agent.

1. The panel previews the epic, each ticket, the blocked-by edges, labels, and
   the board. Nothing is written until the user presses Enter.
2. Labels the publication needs and the repository lacks are created; existing
   labels are left unchanged.
3. The epic is created from the spec — title from its first `# ` heading —
   with a *Glossary and decisions* section summarising the approved
   `CONTEXT.md` files in full, folded, and each approved decision record by its
   title and first paragraph, and labelled `epic`.
4. Tickets are created in dependency order, labelled `ready-for-agent`, and
   attached to the epic as native sub-issues
   (`POST /repos/{owner}/{repo}/issues/{epic}/sub_issues`).
5. Blocked-by relationships are set through GitHub's native issue dependencies
   (`POST /repos/{owner}/{repo}/issues/{n}/dependencies/blocked_by`). Both take
   the issue's database ID, which Grove looks up; not its number.
6. Every issue is added to the project named by `[lab] project` and set to
   `Backlog`. When the key is empty and the repository is linked to exactly one
   project, that project is used; otherwise the user is asked once and the
   answer is saved.
7. The epic's number, each ticket's number, and each sub-issue and blocked-by
   link are recorded on the entry as soon as they are made, so a failed
   publication — GitHub rate-limits links made in quick succession — resumes
   where it stopped instead of creating or linking anything twice.

A shaped bug publishes one issue labelled `bug`, through the same path: the
title is the draft's first `# ` heading and the body is the rest.

Publishing holds the entry's lock and checks that the approved draft is still
exactly what the preview showed; a draft revised after the preview is not
published. After publishing, Grove ends the session.

## Resuming

A session can stop at any point: the agent crashes, Herdr restarts, the
machine sleeps. Its entry then appears under *NEEDS YOU* as *Resume*. A new
run picks up from the files:

- The stage is derived from what exists: without `scout.md`, Scout; with it
  but without a finished interview, Interview; without `spec.md` or with it
  unapproved, Spec; with an approved spec, Tickets.
- The new stage prompt carries an *Interview so far* block: each question and
  its answer, one line each.
- Answers written after the previous agent stopped, and never delivered, are
  included in that block, and receive their receipts with it.
- A question the previous agent wrote and the user has not answered stays
  open; the new agent is told it is waiting and must not write another until
  it is answered.

## Escalation

*Escalate to grill* ends a shaping run and starts a grill run. `issue.md` is
added to the entry input the Scout stage receives. The shaping questions and
answers stay in the decisions log and numbering continues from them; the
Interview prompt is told they are already answered and are not to be asked
again. Escalation is also available after the bug is published; the bug issue
is kept and becomes a sub-issue of the new epic.

## Lifecycle end

- **Archive** is available in any state and hides the entry from the active
  list and the Dashboard; `0` shows archived entries. If a run is live, Grove
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
another host can be cleared from the actions menu.

Answers and requests are created exclusively, so two Grove instances cannot
answer the same question twice; the second is told it was already answered.

## Evaluation

The prompts are measured, not judged by reading. `npm run lab-eval -- --agent
<backend> --model <name>` in `runtime/sandcastle` runs a set of fixture entries
— a clear feature idea, a vague idea, a bug, and an escalation — against a
small pinned fixture repository, `runtime/sandcastle/evals/fixtures/repo`, from
a Herdr pane. A scripted user answers each card with its recommendation, or
with a set override for chosen questions, revises an earlier answer so
revisions are exercised, and approves each draft. The fixture's repository map
is kept beside it as `repo-map.md`; a Go test keeps it identical to the map
Grove builds, so the harness needs no Grove binary.

Each run reports:

- card validity, repair prompts, and fallback turns;
- grounding — the share of questions whose `context` cites a file that exists;
- redundancy — questions whose answer already appears in `scout.md`;
- coverage reached and the number of questions asked;
- whether the spec, tickets, and bug report pass validation on the first
  attempt.

Results are written to `runtime/sandcastle/evals/results/`, so a change to a
prompt can be compared with the run before it. The prompts are complete when
they score well on the model the user runs. The harness needs a live model and
does not run in CI.

## Delivery

The Lab is rebuilt in working slices, each usable on its own:

1. **Protocol and runtime.** Question, answer, and request files; delivery by
   the runtime; repair and fallback; resuming; permission configuration per
   backend.
2. **Prompts and context.** The repository map, one agent per stage, the Scout
   stage, the Grove prompts, coverage, and the evaluation harness; removal of
   the pinned skills.
3. **Entry page and cards.** The stepper, question, fallback, and permission
   cards, and the decisions log.
4. **List.** The grouped list, filters, and the archive toggle.
5. **Review and publishing.** The review gates, change requests, repository
   documents, and the publish panel.
6. **Shaping and escalation.** Bug shaping and escalation on the new stages.
