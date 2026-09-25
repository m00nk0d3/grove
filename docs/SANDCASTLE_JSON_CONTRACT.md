# Grove ↔ Sandcastle CLI JSON Contract

The interface between Grove (the Go TUI) and the Sandcastle runtime
(`runtime/sandcastle`, the `grove-sandcastle` command). Grove starts, lists,
and removes workflow runs only through these commands and the JSON they print.
Both sides live in this repository; a change to either side of the boundary
updates this document in the same change.

- Writing side: `runtime/sandcastle/src/sandcastle.ts` (commands) and
  `runtime/sandcastle/src/runtime-state.ts` (the run record)
- Reading side: `internal/sandcastle/client.go`
- Background: [ADR-0001](./ADR-0001-grove-mission-control-herdr-sandcastle.md)

## Rules

1. **Grove never runs coding agents directly.** It asks the runtime to start a
   workflow; the runtime starts the workflow command in a Herdr pane, and the
   workflow launches the agents.
2. **Grove chooses the agent.** It sends `--agent` with the configured
   `sandcastle.default_agent` (`opencode` by default) or the agent a request
   names. The runtime accepts `opencode`, `pi`, and `claude`, and rejects any
   other value.
3. **Grove sends `--source grove`** on every start, so a run can be attributed
   to Grove. A run started from a shell has the source `command`.
4. **A missing `default_agent` means `opencode`.**
5. **A missing binary is not fatal.** Grove marks the Sandcastle integration
   *unavailable*, keeps running, and shows the state in the header.
6. **Malformed JSON is not fatal.** Grove marks the integration *degraded*,
   shows the error, and does not render raw command output as status.
7. **A failing command is not fatal.** Grove shows stderr, or the error when
   stderr is empty, to the user.
8. **Forward compatibility.** Either side may add fields; the reader ignores
   fields it does not know. Unknown enum values are legal, and Grove shows
   them as they are. A field in the shapes below may not be removed, renamed,
   or change type without a `version` bump.

## Commands

Every command prints one JSON document on stdout and exits 0, or exits
non-zero with the error on stderr. The runtime always prints JSON; the
`--json` flag Grove passes is accepted and has no further effect.

### `grove-sandcastle status`

Grove's poll. Grove runs it with the repository as the working directory; the
runtime reads the runs of the repository containing that directory.

```json
{
  "version": "0.1.0",
  "updated_at": "2026-09-25T10:00:00.000Z",
  "active_workflows": 1,
  "workflows": [ { "…": "workflow run, see below" } ]
}
```

| Field | Type | Notes |
| --- | --- | --- |
| `version` | string | The runtime's version (`grove-sandcastle --version` prints the same) |
| `updated_at` | string | When the status was produced |
| `active_workflows` | integer | Runs whose status is `queued` or `running` |
| `workflows` | array | The newest 100 runs, newest first by `updated_at` |

### `grove-sandcastle workflow start`

```bash
grove-sandcastle workflow start --json \
  --kind <imp|review|address|ci|resolve|clean> \
  --repo <repository path> \
  --worktree <worktree path> \
  --agent <opencode|pi|claude> \
  --source grove \
  [--issue <number>] [--pr <number>]
```

| Flag | Required | Notes |
| --- | --- | --- |
| `--kind` | no | Defaults to `imp` |
| `--issue` | for `imp` | A positive integer |
| `--pr` | for `review`, `address`, `ci`, `resolve` | A positive integer |
| `--repo` | no | The repository the run belongs to and where its tab opens; defaults to the working directory |
| `--agent` | no | Defaults to `opencode`; see rule 2 |
| `--source` | no | Defaults to `grove` |
| `--worktree` | no | Sent by Grove and currently unused; the run starts in `--repo`, and `imp` records the worktree it creates once it has one |

The runtime then:

1. writes the run record with status `queued`;
2. runs `herdr worktree open` for the repository and reads the `workspace_id`;
3. creates a focused tab labelled with the run's title, with the environment
   variables `GROVE_WORKFLOW_RUN_ID=<id>`, `GROVE_WORKFLOW_SOURCE=<source>`,
   and `AGENT_FLOW_AGENT_BACKEND=<agent>`;
4. runs the workflow command in that tab's pane, for example `imp 42`.

It prints `{"workflow": <workflow run>}`. Starting requires Herdr; a failure at
any of those steps exits non-zero with Herdr's error.

The workflow command itself updates the record as it goes, identifying its run
by `GROVE_WORKFLOW_RUN_ID`. A workflow command started from a shell without
that variable creates its own record, with a new id and the source `command`.

### `grove-sandcastle workflow remove <run-id>`

```bash
grove-sandcastle workflow remove <run-id> --json --repo <repository path> [--stop]
```

Deletes the run's record. A run that is `queued`, `running`, or `blocked` is
active, and is removed only with `--stop`. With `--stop`, and when the run has a
process, the runtime:

- refuses unless that process carries `GROVE_WORKFLOW_RUN_ID=<run-id>` in its
  environment, so it never stops an unrelated process that reused the PID;
- closes the Herdr pane of each of the run's agents;
- sends the process `SIGTERM`.

It prints `{"removed": "<run-id>", "stopped": <true when a process was signalled>}`.

### `grove-sandcastle workflow list` and `workflow get <run-id>`

`workflow list` prints `{"workflows": [...]}`, the same list as `status`.
`workflow get` prints a single workflow run, or fails for an unknown id. Grove
does not currently call either.

## Workflow run

The record the runtime writes for each run, and the entry shape of `status`,
`workflow list`, and `workflow get`.

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | `run_<uuid>`; stable for the run's lifetime and the record's file name |
| `kind` | string | `imp`, `review`, `address`, `ci`, `resolve`, or `clean` |
| `title` | string | For example `Implement issue #42`, `Review pull request #17`, `Clean merged worktrees` |
| `status` | string | See [Status vocabulary](#status-vocabulary) |
| `repo` | string | The repository's top-level directory |
| `worktree_path` | string | Where the run works: the repository at first, and for `imp` the worktree it creates |
| `branch` | string | The branch checked out there |
| `default_agent` | string | The agent backend; see rules 2 and 4 |
| `current_step` | string | The current step's title, `Complete`, or `Failed` |
| `progress` | object | `{completed, total, percent}`, integers, `percent` from 0 to 100 |
| `github` | object | `{issue, pull_request}`, each an integer or `null` |
| `agents` | array | See below; empty when none |
| `steps` | array | See below |
| `started_at` | string | RFC 3339 |
| `updated_at` | string | RFC 3339; the last change, and the order runs are listed in |
| `pid` | integer or null | The workflow process while it runs; `null` once it has exited |
| `source` | string | `grove` or `command`; see rule 3 |
| `error` | string | Present on a failed run: why it failed |

`github.issue` is set for `imp`, `github.pull_request` for the kinds that take
`--pr`, and both are `null` for `clean`.

`agents[]`:

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | The agent's id within the run |
| `kind` | string | The agent backend running it |
| `name` | string | Display name, for example `af-pull-request-reviewer-1160-49` |
| `status` | string | See [Status vocabulary](#status-vocabulary) |
| `summary` | string | What the agent is doing |
| `pane_id` | string or null | Its Herdr pane, or `null` when it has none |

`steps[]`:

| Field | Type | Notes |
| --- | --- | --- |
| `id` | string | The step's id |
| `title` | string | Human-readable title |
| `status` | string | See [Status vocabulary](#status-vocabulary) |
| `summary` | string | Optional: what the step did |
| `started_at`, `completed_at` | string | Optional, RFC 3339 |
| `duration_ms` | integer | Optional: how long the step took |

A run starts with one step named after its kind. `imp` replaces it with its
workflow's stages as it runs.

## Status vocabulary

The values the runtime writes. Grove shows any other value as it is.

| Entity | Values |
| --- | --- |
| Workflow run | `queued`, `running`, `blocked`, `succeeded`, `failed` |
| Agent | `working`, `idle`, `blocked`, `failed`, `done` |
| Step | `queued`, `running`, `succeeded`, `failed` |

A run is **active** while it is `queued`, `running`, or `blocked`: it can be
removed only with `--stop`. `status` counts only `queued` and `running` runs in
`active_workflows`.

## Storage and lifecycle

- Each run is one file, `<common-git-dir>/grove-workflows/<id>.json`, written
  atomically (to a temporary file, then renamed) with mode `0600`.
  `<common-git-dir>` is `git rev-parse --git-common-dir`, which every worktree
  of the repository shares; outside a repository the runtime falls back to
  `~/.grove/workflows`.
- When the runtime reads the runs, a `running` run whose `pid` is no longer
  alive is rewritten as `failed`, with `current_step` `Process exited
  unexpectedly` and an `error`.
- Records are never deleted automatically. `status` lists the newest 100.

## Reports

Reports are not part of the command interface: Grove reads them directly from
`<common-git-dir>/agent-flow/`, where the workflows write them beside their
checkpoints.

| Kind | Files |
| --- | --- |
| `imp` | `issue-<N>-implementation-report.md`, `issue-<N>-review-verdict.md` |
| `review` | `pr-<N>-<head sha, 8 characters>-review.md`, one per reviewed head |
| `ci` | `ci-pr-<N>/failures.md` |

The same directory holds `issue-<N>.json` (an `imp` run's checkpoint, which a
retry resumes from), `issue-<N>-pr-title.txt`, `issue-<N>-lean-evidence.json`,
`pr-<N>-<sha>-verdict.json`, and `project-profile.json`. Grove does not read
them. Renaming a report is a change to this contract.

## Conventions

- **Times** are RFC 3339 in UTC (`2026-09-25T10:00:00.000Z`).
- **Paths** are absolute and native to the platform, so they use backslashes
  on Windows.
- **Nulls.** `github.issue`, `github.pull_request`, `pid`, and
  `agents[].pane_id` are `null` when absent, not omitted. Grove also tolerates
  `null` for `agents` and `steps`.

## Versioning

The runtime's `version` is the version of this interface, independent of
Grove's release number, and is currently `0.1.0`. Adding fields or values
does not change it. Removing or renaming a field, changing a type, or changing
an encoding rule increments it, together with a changelog entry, and Grove
keeps reading the previous version until both sides have moved.

## Tests

`internal/sandcastle/client_test.go` checks Grove's side with a fake command
runner, using payloads in these shapes: status and start responses, the
arguments of start and remove, a missing binary, malformed JSON, a failing
command, and empty lists. The runtime's tests (`npm test` in
`runtime/sandcastle`) cover its side. Change the payloads in both when a shape
here changes.
