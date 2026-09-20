# Grove ↔ Sandcastle CLI JSON Contract

**Phase 3 deliverable (issue #172).** Normative integration contract for the
Sandcastle CLI JSON that Grove implements against.

- Source: [Mission Control Implementation Plan, Phase 3](./MISSION_CONTROL_IMPLEMENTATION_PLAN.md)
- Ownership boundaries: [ADR-0001](./ADR-0001-grove-mission-control-herdr-sandcastle.md)
- Consuming side: Grove mission control (Phase 4 adapter, `internal/sandcastle`)

Status: documented. Grove must implement the read side against these exact
shapes, through fakes, before depending on a live Sandcastle binary.

## Scope

In scope for this contract:

```bash
grove-sandcastle status --json
grove-sandcastle workflow list --json
grove-sandcastle workflow get <run-id> --json
grove-sandcastle workflow start --json \
  --kind imp \
  --repo <repo-path> \
  --worktree <worktree-path> \
  --agent opencode \
  --source grove
```

Additional supported workflow targets:

```bash
grove-sandcastle workflow start --kind imp --issue <number> --agent opencode --json
grove-sandcastle workflow start --kind review --pr <number> --agent opencode --json
grove-sandcastle workflow start --kind resolve --pr <number> --agent opencode --json
grove-sandcastle workflow start --kind ci --pr <number> --agent opencode --json
grove-sandcastle workflow start --kind clean --agent opencode --json
```

Not in scope: stop, pause, retry, or cancel workflow controls, raw log
scraping, and real-time push updates. Grove integrates through CLI JSON only.

## Normative Rules

These rules are normative for both sides.

1. **Grove never runs coding agents directly.** Grove starts a Sandcastle workflow;
   Sandcastle launches OpenCode (and any supporting agents). Grove's direct
   agent launcher paths are legacy and must not be used for this flow.
2. **Grove sends `--agent opencode`** on `workflow start` unless a future workflow
   template explicitly selects another agent.
3. **Grove sends `--source grove`** on every `workflow start` Grove issues,
   so Sandcastle can attribute the run to Grove.
4. **`default_agent` fallback:** if a Grove-started run omits
   `default_agent`, Grove treats it as `opencode`.
5. **Missing Sandcastle binary: non-fatal.** Grove marks the Sandcastle
   integration unavailable, keeps the dashboard in a degraded state with the
   last known work items preserved, and shows a clear integration warning.
   Grove startup must not fail.
6. **Malformed JSON: non-fatal, visible error.** Grove must not crash, must
   not render raw command output as status, and must surface a clear
   integration error in the dashboard. The last good state is preserved.
7. **Command failure (non-zero exit or binary error): non-fatal.** Grove
   preserves stderr or structured error details for user-facing diagnostics
   and shows the integration error in the dashboard.
8. **Forward compatibility.** The minimum shapes below are extendable.
   Sandcastle may add fields and Grove must ignore unknown fields. Unknown
   enum values are legal and Grove must render them literally, never crash.
   Grove may not add new required fields to these minimum shapes without a
   version bump.

## Commands

### `grove-sandcastle status --json`

Lightweight aggregate check-in for Grove's poll loop. Returns the same
workflow run shape as `workflow list` so Grove has one parser for both.

```json
{
  "version": "0.4.0",
  "updated_at": "2026-09-17T21:19:00Z",
  "active_workflows": 1,
  "workflows": [
    {
      "id": "run_123",
      "title": "Implement issue #42",
      "status": "running",
      "repo": "/home/user/dev/project",
      "worktree_path": "/home/user/dev/project-worktrees/issue-42",
      "branch": "issue-42",
      "default_agent": "opencode",
      "current_step": "Editing files",
      "progress": {
        "completed": 3,
        "total": 7,
        "percent": 42
      },
      "github": {
        "issue": 42,
        "pull_request": null
      },
      "agents": [
        {
          "id": "agent_pi_1",
          "kind": "pi",
          "name": "pi-main",
          "status": "working",
          "summary": "Refactoring renderer state model",
          "pane_id": "w1:p3"
        }
      ],
      "steps": [
        {
          "id": "step_1",
          "title": "Inspect repo",
          "status": "succeeded"
        },
        {
          "id": "step_2",
          "title": "Implement mission-control model",
          "status": "running"
        }
      ],
      "started_at": "2026-09-17T21:00:00Z",
      "updated_at": "2026-09-17T21:19:00Z"
    }
  ]
}
```

Top-level fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `version` | string | yes | Sandcastle contract/binary version |
| `updated_at` | string | yes | RFC 3339 UTC, authoritative for staleness |
| `active_workflows` | integer | yes | Count of `workflows` entries with an active status |
| `workflows` | array | yes | Workflow runs, same shape as `workflow list` entries; empty array when none |

### `grove-sandcastle workflow list --json`

List of workflow runs currently tracked by Sandcastle. Sandcastle does not
need to filter by status; Grove decides what to render (e.g. active runs on
the global dashboard).

```json
{
  "workflows": [
    {
      "id": "run_123",
      "title": "Implement issue #42",
      "status": "running",
      "repo": "/home/user/dev/project",
      "worktree_path": "/home/user/dev/project-worktrees/issue-42",
      "branch": "issue-42",
      "default_agent": "opencode",
      "current_step": "Editing files",
      "progress": {
        "completed": 3,
        "total": 7,
        "percent": 42
      },
      "github": {
        "issue": 42,
        "pull_request": null
      },
      "agents": [
        {
          "id": "agent_pi_1",
          "kind": "pi",
          "name": "pi-main",
          "status": "working",
          "summary": "Refactoring renderer state model",
          "pane_id": "w1:p3"
        }
      ],
      "steps": [
        {
          "id": "step_1",
          "title": "Inspect repo",
          "status": "succeeded"
        },
        {
          "id": "step_2",
          "title": "Implement mission-control model",
          "status": "running"
        }
      ],
      "started_at": "2026-09-17T21:00:00Z",
      "updated_at": "2026-09-17T21:19:00Z"
    }
  ]
}
```

#### Workflow run (canonical shape)

This is the single canonical workflow run shape. It is the entry shape for
`workflow list`, the full object for `workflow get`, an entry of `status`,
and (a subset of) the `workflow` object in the start response.

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `id` | string | yes | Unique run id (e.g. `run_123`); stable for the run's lifetime |
| `title` | string | yes | Human-readable summary (e.g. `Implement issue #42`) |
| `status` | string | yes | See Status Vocabulary; Grove must tolerate unknown values |
| `repo` | string | yes | Absolute repo path |
| `worktree_path` | string | yes | Absolute worktree path where the run executes |
| `branch` | string | yes | Branch the run is working on |
| `default_agent` | string | no | Coding agent kind; Grove defaults to `opencode` when omitted |
| `current_step` | string | no | Human-readable current step title; empty or omitted when idle |
| `progress` | object | yes | `{completed, total, percent}` integers; `percent` in 0..100 |
| `github` | object | yes | `{issue, pull_request}`; see below |
| `agents` | array | yes | Agent entries; empty array when none |
| `steps` | array | yes | Step entries; empty array when none |
| `started_at` | string | yes | RFC 3339 UTC |
| `updated_at` | string | yes | RFC 3339 UTC, last state change |

`github`:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `issue` | integer or null | yes | Issue number, or null when the run is not issue-backed |
| `pull_request` | integer or null | yes | PR number, or null when the run has no PR |

`agents[]`:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `id` | string | yes | Agent run id (e.g. `agent_pi_1`) |
| `kind` | string | yes | Agent kind; `pi` for Pi Agent |
| `name` | string | yes | Display name (e.g. `pi-main`) |
| `status` | string | yes | Agent status, see Status Vocabulary |
| `summary` | string | yes | Concise current-activity summary |
| `pane_id` | string or null | yes | Herdr pane identifier (e.g. `w1:p3`) or null when not pane-backed |

`steps[]`:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `id` | string | yes | Step id (e.g. `step_1`) |
| `title` | string | yes | Human-readable step title |
| `status` | string | yes | Step status, see Status Vocabulary |

### `grove-sandcastle workflow get <run-id> --json`

Single workflow run object, exactly the same shape as one entry from
`workflow list`, without the `workflows` wrapper array. Use for drill-down
detail after a user selects a workflow run.

```json
{
  "id": "run_123",
  "title": "Implement issue #42",
  "status": "running",
  "repo": "/home/user/dev/project",
  "worktree_path": "/home/user/dev/project-worktrees/issue-42",
  "branch": "issue-42",
  "default_agent": "opencode",
  "current_step": "Editing files",
  "progress": {
    "completed": 3,
    "total": 7,
    "percent": 42
  },
  "github": {
    "issue": 42,
    "pull_request": null
  },
  "agents": [
    {
      "id": "agent_pi_1",
      "kind": "pi",
      "name": "pi-main",
      "status": "working",
      "summary": "Refactoring renderer state model",
      "pane_id": "w1:p3"
    }
  ],
  "steps": [
    {
      "id": "step_1",
      "title": "Inspect repo",
      "status": "succeeded"
    },
    {
      "id": "step_2",
      "title": "Implement mission-control model",
      "status": "running"
    }
  ],
  "started_at": "2026-09-17T21:00:00Z",
  "updated_at": "2026-09-17T21:19:00Z"
}
```

### `grove-sandcastle workflow start --json`

Grove → Sandcastle request to start a workflow. Grove must always send:

```bash
grove-sandcastle workflow start --json \
  --kind imp \
  --repo <repo-path> \
  --worktree <worktree-path> \
  --agent opencode \
  --source grove
```

Request flags:

| Flag | Required | Notes |
| --- | --- | --- |
| `--repo` | yes | Absolute repo path |
| `--worktree` | yes | Absolute path to the target worktree |
| `--agent` | yes | Always `pi` from Grove, unless a future template selects another agent (rule 2) |
| `--source` | yes | `grove` for Grove-initiated starts (rule 3) |
| `--issue` | no | Deferred; issue number for future issue-backed starts |
| `--pr` | no | Deferred; PR number for future PR-backed starts |

Minimum start response:

```json
{
  "workflow": {
    "id": "run_123",
    "status": "queued",
    "default_agent": "opencode",
    "worktree_path": "/path/to/worktree"
  }
}
```

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `workflow.id` | string | yes | The run id returned by the start; poll `status`/`list` with it |
| `workflow.status` | string | yes | `queued` when accepted |
| `workflow.default_agent` | string | no | As per run shape; Grove defaults to `opencode` when omitted |
| `workflow.worktree_path` | string | no | Target worktree path |

The response may include additional fields from the full run shape (rule 8).
Grove must not require more than the fields marked required.

## Status Vocabulary

Minimum values. Both sides may extend the set; Grove must render unknown
values literally and never crash.

| Entity | Minimum values |
| --- | --- |
| Workflow run status | `queued`, `running`, `blocked`, `failed`, `succeeded` |
| Agent status | `working`, `idle`, `blocked`, `failed`, `done` |
| Step status | `running`, `succeeded`, `failed` |

## Conventions

- **Time.** All timestamps are RFC 3339 in UTC with a `Z` suffix
  (e.g. `2026-09-17T21:19:00Z`). `updated_at` is the authoritative field
  for staleness and freshness checks.
- **Paths.** Absolute paths (POSIX on Linux).
- **Null semantics.** `github.pull_request` and `github.issue` are `null`
  when absent, not omitted. `agents[].pane_id` is `null` when the agent is
  not pane-backed.
- **Empty collections.** `agents` and `steps` are empty arrays when there
  are none. Grove must also tolerate `null` for these fields.
- **Exit codes.** `0` on success with JSON on stdout; non-zero on command
  failure. A missing binary is detected by Grove before it invokes the
  command (rule 5) and is a distinct condition from command failure.
- **Output format.** JSON is the only required output format for Grove.
  Grove does not scrape text modes.

## Versioning

- The `version` field (status response) identifies the contract version
  the responding binary implements.
- Additive changes (new fields, new status values) are non-breaking and must
  not break Grove.
- Breaking changes (renaming or removing minimum-shape fields, changing field
  types, or changing JSON encoding rules) require a `version` bump, a
  changelog entry, and a documented migration window. Grove must tolerate the
  prior major version during the transition.

## Testability (Phase 4)

Adapter tests must be derived from the payloads in this document, using
fake command runners rather than a live Sandcastle binary. The minimum
coverage set (from [ADR-0001, Testing Decisions](./ADR-0001-grove-mission-control-herdr-sandcastle.md)):

- successful `workflow list` parsing,
- successful `workflow get` parsing,
- successful `status` parsing,
- successful start response parsing,
- missing binary,
- malformed JSON,
- command failure (non-zero exit),
- empty state (empty `workflows` array, empty `agents`/`steps`).

If a documented shape changes, the corresponding test payloads and this
document change together.

## References

- [Mission Control Implementation Plan](./MISSION_CONTROL_IMPLEMENTATION_PLAN.md)
  — Phase 3 (this contract's home in the roadmap) and Phase 4 (adapter).
- [ADR-0001](./ADR-0001-grove-mission-control-herdr-sandcastle.md) — ownership
  boundaries and integration consequences Grove must honor.
