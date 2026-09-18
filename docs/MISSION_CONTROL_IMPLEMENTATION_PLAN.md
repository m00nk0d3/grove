# Grove Mission Control Implementation Plan

This plan turns [ADR-0001: Grove Mission Control with Herdr and Sandcastle](./ADR-0001-grove-mission-control-herdr-sandcastle.md) into an implementation roadmap. The ADR is the source of truth for ownership boundaries.

## Goal

Grove becomes the visual control center for local development work. It shows the overall state of worktrees, GitHub issues, pull requests, Sandcastle workflow runs, Pi Agent activity, and Herdr panes, then lets the user drill into a selected worktree, agent, workflow, issue, or pull request.

The target ownership model is:

| Layer | Responsibility |
| --- | --- |
| Grove | Dashboard, navigation, visual status, state correlation, and Sandcastle workflow-start requests |
| Sandcastle | Workflow execution and lifecycle; agent launch, supervision, completion, and multi-agent orchestration |
| Pi Agent | Primary/default coding agent for Grove-started Sandcastle workflows |
| Herdr | Terminal workspaces, tabs, panes, focus, persistence, pane jumps, and pane-local process visibility |
| GitHub | System of record for issues, pull requests, labels, reviews, and checks |

Grove should not launch coding agents directly in the target architecture. Grove starts Sandcastle workflows, Sandcastle launches Pi Agent and any supporting agents, Herdr owns the terminal runtime, and Grove renders the resulting state. Direct agent launchers from Grove are legacy and explicitly out of scope.

Displaying or discovering an agent does not imply lifecycle ownership. Herdr may observe an agent in a pane, and Grove may correlate and display that observation, but Sandcastle owns the agent process and workflow lifecycle. GitHub integration is read-only in Phase 1: Grove displays and correlates metadata but does not mutate GitHub records.

## Non-goals for Phase 1

- Raw Herdr socket subscriptions.
- Real-time push updates from Herdr.
- Real-time push updates from Sandcastle.
- Stop, pause, or retry Sandcastle workflow controls.
- GitHub mutation actions such as labels, assignments, comments, or review submission.
- Full Herdr plugin packaging.
- Remote Herdr machine support.
- New direct Grove agent launchers.
- Grove owning agent process lifecycle.
- Treating Pi Agent as a Grove-spawned process.
- Sandcastle workflow graph visualization as the primary top-level model.
- A separate web dashboard for Sandcastle status.

## Architecture Overview

```text
Grove TUI
  ├─ Mission Control domain model
  │   ├─ Worktrees
  │   ├─ GitHub issues / PRs
  │   ├─ Sandcastle workflow runs
  │   ├─ Sandcastle/Pi agents
  │   └─ Herdr panes / workspaces / jump targets
  │
  ├─ Adapters
  │   ├─ Git adapter          existing
  │   ├─ GitHub adapter       existing gh-based sync
  │   ├─ Sandcastle adapter   new CLI JSON integration
  │   └─ Herdr adapter        new CLI JSON integration
  │
  └─ Renderers
      ├─ Global dashboard
      ├─ Worktree detail dashboard
      ├─ Agent detail dashboard
      ├─ Workflow detail dashboard
      └─ PR / issue detail dashboard
```

The most important seam is:

```text
BuildMissionControlState(
  worktrees,
  issues,
  pullRequests,
  sessions,
  herdrSnapshot,
  sandcastleSnapshot,
) -> MissionControlState
```

Most tests should target this seam because it proves the product behavior without requiring live Herdr, Sandcastle, terminals, or agents.

## Phase 0: Finalize Product Boundary

### Purpose

Make the intended ownership split impossible to misunderstand before implementation starts.

### Tasks

- Keep the ADR as the architecture source of truth.
- Ensure docs say Grove starts and monitors Sandcastle workflows instead of directly launching coding agents.
- Mark direct Claude/Copilot/Aider launchers as legacy/current behavior, not the target flow.
- Document Pi Agent as Sandcastle's default coding agent for Grove-started workflows.
- Document that Herdr owns terminal panes and jump targets.
- Document that Sandcastle status must be visual Bubble Tea/Lip Gloss UI, not raw output.

### Acceptance Criteria

- Contributors can read the ADR and understand that Grove does not own agent process lifecycle.
- Contributors can read the ADR and understand that Pi Agent is the default Sandcastle coding agent.
- Phase 1 scope is clear: dashboard, visual status, workflow start, Herdr open/jump, Herdr-managed worktree create/open.

## Phase 1: Add Mission-Control Domain Model

### Purpose

Give Grove one internal language for Herdr, Sandcastle, GitHub, worktrees, agents, panes, and workflow runs.

### Suggested Modules

- `internal/domain/mission_control.go`
- `internal/domain/mission_control_test.go`

### Core Types

```go
type MissionControlState struct {
    WorkItems    []WorkItem
    Worktrees    []WorktreeRef
    Issues       []GitHubIssueRef
    PullRequests []GitHubPullRequestRef
    WorkflowRuns []WorkflowRunRef
    Agents       []AgentRef
    Panes        []PaneRef
    Integrations IntegrationStatus
    Warnings     []IntegrationWarning
    UpdatedAt    time.Time
}
```

```go
type WorkItem struct {
    ID             string
    Kind           WorkItemKind
    Title          string
    Repo           string
    Branch         string
    Status         WorkItemStatus
    Worktree       *WorktreeRef
    Issue          *GitHubIssueRef
    PullRequest    *GitHubPullRequestRef
    WorkflowRuns   []WorkflowRunRef
    Agents         []AgentRef
    Panes          []PaneRef
    Degraded       bool
    DegradedReason string
}
```

```go
type WorkflowRunRef struct {
    ID           string
    Source       string
    Title        string
    Status       WorkflowStatus
    Repo         string
    WorktreePath string
    Branch       string
    IssueNumber  *int
    PRNumber     *int
    DefaultAgent string
    Agents       []AgentRef
    CurrentStep  string
    Steps        []WorkflowStepRef
    Progress     WorkflowProgress
    StartedAt    time.Time
    UpdatedAt    time.Time
}
```

```go
type AgentRef struct {
    ID             string
    Name           string
    Kind           string
    LifecycleOwner AgentLifecycleOwner
    ObservedBy     []AgentObserver
    Status         AgentStatus
    WorktreePath   string
    WorkflowRunID  string
    PaneID         string
    Summary        string
    UpdatedAt      time.Time
}
```

```go
type PaneRef struct {
    Runtime     string
    WorkspaceID string
    TabID       string
    PaneID      string
    CWD         string
    Title       string
    AgentID     string
    Status      string
    Jumpable    bool
}
```

```go
type IntegrationStatus struct {
    Herdr      ExternalIntegration
    Sandcastle ExternalIntegration
    GitHub     ExternalIntegration
}

type ExternalIntegration struct {
    Available bool
    Enabled   bool
    Mode      string
    Version   string
    Error     string
    LastSync  time.Time
}
```

### Status Enums

Add normalized enums for:

- `WorkItemKind`: `worktree`, `issue`, `pull_request`, `branch`, `manual`
- `WorkItemStatus`: `idle`, `queued`, `running`, `blocked`, `failed`, `succeeded`, `unknown`
- `WorkflowStatus`: `queued`, `running`, `blocked`, `failed`, `succeeded`, `cancelled`, `unknown`
- `AgentStatus`: `idle`, `working`, `blocked`, `done`, `failed`, `unknown`
- `AgentLifecycleOwner`: `sandcastle`, `grove_legacy`, `external`
- `AgentObserver`: `sandcastle`, `herdr`, `grove`

`grove_legacy` identifies only agents started by the pre-Mission-Control launchers. Herdr can observe agent presence but is not an `AgentLifecycleOwner`.

### Acceptance Criteria

- Domain types compile independently of Herdr and Sandcastle adapters.
- Domain types do not expose raw Herdr or Sandcastle JSON as their primary shape.
- Pi Agent can be represented as an agent owned by Sandcastle.
- Herdr panes can be represented as jumpable runtime refs.

## Phase 2: Build Mission-Control State Builder

### Purpose

Create a single correlation layer that turns existing Grove data plus integration snapshots into one dashboard state.

### Suggested Module

- `internal/mission/control.go`
- `internal/mission/control_test.go`

### Public API

```go
type BuildInput struct {
    RepoPath            string
    Worktrees           []domain.Worktree
    Issues              []domain.Issue
    PullRequests        []domain.PullRequest
    Sessions            []domain.Session
    HerdrSnapshot       *herdr.Snapshot
    SandcastleSnapshot  *sandcastle.Snapshot
    PreviousState       *domain.MissionControlState
    Now                 time.Time
}

func BuildState(input BuildInput) domain.MissionControlState
```

### Correlation Rules

#### Worktree to pull request

Match by:

1. Exact worktree branch equals PR head branch.
2. Worktree branch contains PR head branch.
3. Existing Grove link metadata, if available.
4. Best-effort path or branch naming fallback.

#### Worktree to issue

Match by:

1. Existing Grove issue/worktree link metadata.
2. Branch naming patterns like `issue-123`, `123-title`, `feat-123`.
3. PR linked to issue.
4. Sandcastle workflow metadata.

#### Sandcastle workflow to worktree

Match by:

1. Exact `worktree_path`.
2. Repo plus branch.
3. GitHub issue or PR ref.
4. Workflow metadata labels.

#### Sandcastle agent to workflow

Match by Sandcastle workflow run ID.

#### Herdr pane to Sandcastle/Pi agent

Match by:

1. Pane ID reported by Sandcastle.
2. CWD matching a Grove worktree path.
3. Agent ID or name if both systems expose it.
4. Recent command/title if available.

#### Grove session to Herdr pane

Match by:

1. Stored Herdr pane ID.
2. Worktree path.
3. Legacy PID only for local runtime sessions.

### Degraded-State Rules

- If a stored Herdr pane ID cannot be found, mark the related work item as degraded but keep it visible.
- If Sandcastle is unavailable, preserve previous workflow cache if available and mark it stale.
- If Sandcastle JSON is malformed, preserve previous valid state and add an integration warning.
- If Herdr is unavailable outside Herdr, show standalone mode rather than an error.
- If Herdr is unavailable inside Herdr, mark Herdr degraded and disable pane jumps.

### Acceptance Criteria

- Builder can produce a complete state with no Herdr snapshot.
- Builder can produce a complete state with no Sandcastle snapshot.
- Builder correlates worktree, GitHub, workflow, agent, and pane data.
- Builder marks stale/degraded state explicitly.

## Phase 3: Define Sandcastle CLI JSON Contract

### Purpose

Give Grove a stable integration contract that does not require scraping logs.

### Required Commands

```bash
sandcastle status --json
sandcastle workflow list --json
sandcastle workflow get <run-id> --json
sandcastle workflow start --json \
  --repo <repo-path> \
  --worktree <worktree-path> \
  --agent pi \
  --source grove
```

### Optional Later Commands

```bash
sandcastle workflow start --issue <number> --agent pi --json
sandcastle workflow start --pr <number> --agent pi --json
sandcastle agent list --json
```

### Minimum Workflow List Shape

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
      "default_agent": "pi",
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

### Minimum Workflow Start Response

```json
{
  "workflow": {
    "id": "run_123",
    "status": "queued",
    "default_agent": "pi",
    "worktree_path": "/path/to/worktree"
  }
}
```

### Rules

- Grove sends `--agent pi` unless a future workflow template explicitly selects another agent.
- Grove never runs `pi` directly.
- If a Grove-started workflow omits `default_agent`, Grove treats it as `pi`.
- Malformed JSON becomes a visible integration error.
- Missing Sandcastle is non-fatal.

### Acceptance Criteria

- The contract is documented in Grove docs.
- Grove tests use this contract through fakes before depending on a live Sandcastle binary.

## Phase 4: Add Sandcastle Adapter

### Purpose

Let Grove read Sandcastle status and start Pi-backed workflows through a fakeable CLI adapter.

### Suggested Package

- `internal/integration/sandcastle`

### Interfaces

```go
type Client interface {
    Available(ctx context.Context) IntegrationInfo
    Snapshot(ctx context.Context, repoPath string) (Snapshot, error)
    StartWorkflow(ctx context.Context, req StartWorkflowRequest) (WorkflowRun, error)
}
```

```go
type CommandRunner interface {
    Run(ctx context.Context, name string, args ...string) (stdout []byte, stderr []byte, err error)
}
```

### Core Types

```go
type Snapshot struct {
    Workflows  []WorkflowRun
    Agents     []Agent
    CapturedAt time.Time
}
```

```go
type StartWorkflowRequest struct {
    RepoPath     string
    WorktreePath string
    Branch       string
    IssueNumber  *int
    PRNumber     *int
    AgentKind    string
    Source       string
}
```

### Behavior

- Resolve binary from config, defaulting to `sandcastle`.
- Use command timeouts.
- Preserve stderr in errors.
- Normalize statuses into Grove domain statuses.
- Default `AgentKind` to `pi`.
- Send `--source grove`.
- Never panic on missing fields or malformed JSON.
- Treat missing Sandcastle as unavailable, not fatal.

### Config

Prefer simple top-level sections:

```toml
[sandcastle]
enabled = true
binary = "sandcastle"
poll_interval_seconds = 5
default_agent = "pi"
```

### Acceptance Criteria

- `Snapshot` parses workflow list JSON.
- `StartWorkflow` sends `--agent pi` by default.
- Errors preserve command stderr.
- Missing binary produces unavailable integration state.
- Adapter tests use a fake command runner.

## Phase 5: Add Herdr Adapter

### Purpose

Let Grove open worktrees, create worktrees, list panes/agents, and jump to panes through Herdr when running in Herdr mode.

### Suggested Package

- `internal/integration/herdr`

### Interfaces

```go
type Client interface {
    Available(ctx context.Context) IntegrationInfo
    Snapshot(ctx context.Context) (Snapshot, error)
    OpenWorktree(ctx context.Context, req OpenWorktreeRequest) (PaneRef, error)
    CreateWorktree(ctx context.Context, req CreateWorktreeRequest) (WorktreeRef, error)
    FocusPane(ctx context.Context, paneID string) error
}
```

### Detection

- `HERDR_ENV=1` means Grove is running inside Herdr.
- Capture `HERDR_WORKSPACE_ID`, `HERDR_TAB_ID`, and `HERDR_PANE_ID` when present.
- Check binary availability and server status before enabling Herdr mode.

### Phase-1 Commands

```bash
herdr workspace list --json
herdr pane list --json
herdr agent list --json
herdr worktree list --json
herdr worktree open --path <path> --no-focus --json
herdr worktree create --branch <branch> --base <base> --path <path> --no-focus --json
```

Pane focus should use the supported Herdr focus command for the installed version.

### Rules

- Do not rely on another client's focused pane.
- Prefer explicit pane/workspace IDs.
- Use `--current` only when targeting Grove's own Herdr pane.
- Use `--no-focus` for background opens unless the user explicitly asks to jump.
- Missing Herdr outside Herdr is standalone mode.
- Herdr failure inside Herdr is degraded mode.
- Worktree fallback requires explicit user confirmation.

### Config

```toml
[herdr]
enabled = true
binary = "herdr"
poll_interval_seconds = 5
prefer_worktree_api = true
```

### Acceptance Criteria

- Herdr mode is detected from environment plus command availability.
- Herdr snapshot can parse panes, agents, workspaces, and worktrees.
- Grove can request open worktree and focus pane through the adapter.
- Adapter tests use a fake command runner.

## Phase 6: Add Runtime References to Sessions

### Purpose

Stop assuming every live session can be represented by a long-lived process ID.

### Current Problem

Existing active sessions are PID-first. That works for local terminal launches but not for Herdr panes, where a launcher process may exit while the pane remains alive.

### Domain Changes

Extend session data conceptually:

```go
type Session struct {
    ID            int64
    WorktreePath  string
    Runtime       SessionRuntime
    RuntimeID     string
    WorkspaceID   *string
    TabID         *string
    PaneID        *string
    ShellPID      *int
    AgentName     *string
    WorkflowRunID *string
    Status        SessionStatus
    StartedAt     time.Time
    UpdatedAt     time.Time
}
```

### Migration

Add a new migration after the current active sessions migration:

```sql
ALTER TABLE active_sessions ADD COLUMN runtime TEXT DEFAULT 'local';
ALTER TABLE active_sessions ADD COLUMN runtime_id TEXT;
ALTER TABLE active_sessions ADD COLUMN herdr_workspace_id TEXT;
ALTER TABLE active_sessions ADD COLUMN herdr_tab_id TEXT;
ALTER TABLE active_sessions ADD COLUMN herdr_pane_id TEXT;
ALTER TABLE active_sessions ADD COLUMN workflow_run_id TEXT;
ALTER TABLE active_sessions ADD COLUMN degraded_reason TEXT;
```

### Health Checks

- `runtime=local`: use existing PID checks.
- `runtime=herdr`: query Herdr snapshot.
- `workflow_run_id` present: query Sandcastle snapshot.
- Do not mark a Herdr-backed session dead just because `ShellPID` is empty.

### Acceptance Criteria

- Old session rows still load.
- New Herdr-backed rows can persist pane IDs.
- Health checks do not delete live Herdr-backed sessions due to missing PID.

## Phase 7: Add Integration Polling to Bubble Tea

### Purpose

Keep Grove's mission-control state fresh without blocking the TUI.

### New Messages

```go
type integrationsTickMsg struct{}

type sandcastleSyncedMsg struct {
    Snapshot sandcastle.Snapshot
    Err      error
}

type herdrSyncedMsg struct {
    Snapshot herdr.Snapshot
    Err      error
}

type missionControlUpdatedMsg struct {
    State domain.MissionControlState
}
```

### Flow

1. App starts.
2. Load config.
3. Load Git/GitHub/session state.
4. Detect Herdr.
5. Detect Sandcastle.
6. Start a 5-second integration tick.
7. On each tick:
   - refresh Herdr snapshot when enabled/available
   - refresh Sandcastle snapshot when enabled/available
   - rebuild mission-control state
   - render updated dashboard

### Error Handling

- External commands must run asynchronously through Bubble Tea commands.
- Use timeouts.
- Keep previous valid snapshot when refresh fails.
- Mark integration degraded.
- Avoid repeated noisy errors every poll interval.
- Show last successful sync timestamp.

### Acceptance Criteria

- UI remains responsive while integrations refresh.
- Integration errors appear as stable degraded state.
- Manual refresh can force Herdr/Sandcastle/GitHub refresh together.

## Phase 8: Add Global Mission-Control Dashboard

### Purpose

Create the first visible version of Grove as a control center.

### View Strategy

Add a new view before replacing existing views:

```go
viewDashboard
viewWorktrees
viewIssues
viewPRs
viewWorkflowRuns
viewAgents
```

The dashboard can become the default once stable. Existing Worktrees, Issues, and PRs views should remain available during migration.

### Dashboard Layout

```text
┌ Grove Mission Control ───────────────────────────────────────┐
│ Repo: m00nk0d3/grove      Herdr: connected   Sandcastle: ok  │
├ Work Items ───────────────┬ Agents ─────────┬ Workflows ─────┤
│ issue-42  running   42%   │ pi-main working │ run_123 42%    │
│ pr-17     blocked         │ pi-review idle  │ run_124 block  │
│ feat-x    idle            │                 │                │
├ Details ─────────────────────────────────────────────────────┤
│ Selected: issue-42                                           │
│ Worktree: ../worktrees/issue-42                              │
│ PR: #17                                                      │
│ Agent: pi-main working                                       │
│ Pane: w1:p3 jumpable                                         │
│ Current step: Editing renderer state model                   │
└ j/k select • Enter details • o open pane • W workflow • q quit┘
```

### Required Visual Elements

- Integration badges for Herdr, Sandcastle, and GitHub.
- Work item rows.
- Workflow cards.
- Pi Agent rows.
- Status badges.
- Progress bars.
- Degraded-state warnings.
- Last sync timestamp.

### Acceptance Criteria

- Dashboard shows worktrees, agents, workflows, and integrations.
- Sandcastle status is rendered visually.
- Pi Agent appears as the default Sandcastle coding actor.
- Existing views remain reachable.

## Phase 9: Add Drill-Down Dashboards

### Purpose

Let users inspect what is happening inside a selected worktree, agent, or workflow and then return to the global dashboard.

### Model Additions

```go
type detailViewKind int

const (
    detailNone detailViewKind = iota
    detailWorktree
    detailAgent
    detailWorkflow
    detailIssue
    detailPullRequest
)
```

```go
missionState       domain.MissionControlState
selectedWorkItemIdx int
selectedAgentIdx    int
selectedWorkflowIdx int
detailKind          detailViewKind
detailID            string
```

### Navigation

- `Enter`: open selected item detail.
- `Esc` or `Backspace`: return to dashboard.
- `o`: open or jump Herdr pane.
- `s`: start Sandcastle workflow.
- `/`: fuzzy search includes work items, workflows, agents, and panes.
- `r`: manual refresh all integrations.

### Worktree Detail

Show:

- Path.
- Branch.
- Dirty status.
- Linked issue.
- Linked PR.
- Active Sandcastle workflow.
- Pi Agent status.
- Herdr pane IDs.
- Current step.
- Timeline.
- Recent summary.
- Degraded warnings.

### Agent Detail

Show:

- Agent kind, usually `pi`.
- Owner, usually `sandcastle`.
- Status.
- Workflow run.
- Worktree.
- Pane.
- Current task.
- Recent summary.
- Last update.

Phase 1 actions:

- Jump to pane.
- View workflow.
- View worktree.

No direct prompt, kill, pause, retry, or stop controls in phase 1.

### Workflow Detail

Show:

- Run ID.
- Status.
- Default agent: Pi.
- Worktree.
- Issue/PR.
- Progress bar.
- Current step.
- Step list.
- Agents.
- Pane targets.
- Errors.
- Last update.

### Acceptance Criteria

- User can drill into a worktree and return.
- User can drill into an agent and return.
- User can drill into a workflow and return.
- Detail views never show raw Sandcastle JSON as primary UI.

## Phase 10: Start Sandcastle Workflows from Grove

### Purpose

Replace the target agent-launch action with a Sandcastle workflow-start action.

### UX

```text
Start Sandcastle Workflow

Target:
  Worktree: /path/to/worktree
  Issue: #42
  PR: none

Workflow:
  > Implement / continue work
    Review PR
    Explain status
    Custom prompt

Agent:
  Pi Agent default

Enter start • Esc cancel
```

### Start Request

```go
StartWorkflowRequest{
    RepoPath:      repoPath,
    WorktreePath: selected.Worktree.Path,
    Branch:       selected.Worktree.Branch,
    IssueNumber:  selected.Issue.Number,
    PRNumber:     selected.PullRequest.Number,
    AgentKind:    "pi",
    Source:       "grove",
}
```

### Behavior

- Start by selected worktree.
- Start by selected issue.
- Start by selected pull request.
- Default agent to Pi.
- Show optimistic queued state after start.
- Refresh Sandcastle snapshot after start.
- Attach returned workflow run to selected work item.
- If returned state includes Herdr pane ID, mark pane as jumpable.

### Acceptance Criteria

- Starting a workflow from Grove calls Sandcastle, not Pi directly.
- Start requests default to `pi`.
- Started workflow appears visually in Grove.
- Errors are visible and do not crash the UI.

## Phase 11: Herdr Pane Jump/Open and Worktree Runtime

### Purpose

Make Grove compatible with Herdr's terminal runtime while preserving standalone mode.

### Open Existing Worktree in Herdr Mode

```bash
herdr worktree open --path <path> --no-focus --json
```

Store:

- workspace ID.
- tab ID.
- pane ID.
- worktree path.
- runtime: `herdr`.

If the user asks to jump, focus the pane through Herdr.

### Create Worktree in Herdr Mode

```bash
herdr worktree create \
  --branch <branch> \
  --base <base> \
  --path <path> \
  --no-focus \
  --json
```

### Fallback Flow

If Herdr worktree create/open fails:

1. Show the Herdr error.
2. Offer explicit fallback.
3. If accepted, create/open through existing direct Git behavior.
4. Try to register/open the resulting worktree in Herdr.
5. If registration fails, mark the worktree as degraded or unattached.

### Standalone Mode

- Continue current direct Git worktree behavior.
- Continue current local shell opening behavior.
- Sandcastle status can still render if available.

### Acceptance Criteria

- Inside Herdr, worktree open/create prefers Herdr.
- Outside Herdr, Grove behaves as it does today.
- Fallback is explicit, not automatic.
- Degraded state is visible.

## Phase 12: Add Persistence for Integration References

### Purpose

Persist durable references without blindly caching the entire live dashboard.

### Integration Refs Table

```sql
CREATE TABLE IF NOT EXISTS integration_refs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    local_key TEXT NOT NULL,
    external_system TEXT NOT NULL,
    external_id TEXT NOT NULL,
    metadata_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(kind, local_key, external_system)
);
```

Examples:

- worktree path to Herdr pane ID.
- worktree path to Sandcastle workflow run ID.
- issue number to Sandcastle workflow run ID.
- PR number to Sandcastle workflow run ID.

### Optional Workflow Cache

```sql
CREATE TABLE IF NOT EXISTS workflow_runs_cache (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    repo_path TEXT,
    worktree_path TEXT,
    branch TEXT,
    issue_number INTEGER,
    pr_number INTEGER,
    default_agent TEXT,
    title TEXT,
    current_step TEXT,
    progress_completed INTEGER,
    progress_total INTEGER,
    progress_percent INTEGER,
    summary TEXT,
    raw_json TEXT,
    started_at DATETIME,
    updated_at DATETIME,
    cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Rules

- Persist refs and last-known workflow summaries.
- Do not persist credentials.
- Do not treat cached workflow state as fresh.
- Show stale markers when live Sandcastle refresh fails.

### Acceptance Criteria

- Grove starts with useful last-known workflow state.
- Runtime refs survive restart.
- Stale state is clearly marked.
- No secrets are stored.

## Phase 13: Extend Fuzzy Finder

### Purpose

Make mission-control objects quickly reachable.

### New Search Kinds

- `KindWorkflowRun`
- `KindAgent`
- `KindPane`
- `KindWorkItem`

### Actions

- Workflow result opens workflow detail.
- Agent result opens agent detail.
- Pane result jumps through Herdr.
- Work item result opens the relevant detail dashboard.

### Acceptance Criteria

- Fuzzy search includes workflows, Pi agents, panes, and work items.
- Selecting a result navigates to the correct dashboard or action.

## Phase 14: Add Visual Components

### Purpose

Keep the new dashboard visually consistent and reusable.

### Suggested Components

- `status_badge`
- `progress_bar`
- `timeline`
- `integration_badge`
- `agent_row`
- `workflow_card`
- `work_item_row`

### Status Mapping

| Status | Visual intent |
| --- | --- |
| running / working | cyan or blue |
| idle | muted gray |
| blocked | yellow |
| failed | red |
| succeeded / done | green |
| unknown | dim |

### Workflow Card Fields

- Title.
- Status badge.
- Progress bar.
- Current step.
- Default agent: Pi.
- Worktree.
- Issue/PR.
- Pane ID if jumpable.

### Timeline Example

```text
✓ Inspect repo
✓ Plan changes
● Implement mission-control model
○ Add Herdr adapter
○ Add Sandcastle adapter
```

### Header Integration Badges

Connected:

```text
Herdr: connected   Sandcastle: connected   GitHub: synced 2m ago
```

Degraded:

```text
Herdr: standalone   Sandcastle: unavailable   GitHub: stale
```

### Acceptance Criteria

- Components use the existing theme system.
- Status rendering is consistent across dashboard and detail views.
- Sandcastle workflow state is visual and scannable.

## Phase 15: Update Configuration

### Purpose

Let users enable/disable integrations and configure binaries without disrupting existing config.

### Domain Config

```go
type Config struct {
    GitHub      GitHubConfig
    Appearance  AppearanceConfig
    AIAgents    AIAgentsConfig
    Worktrees   WorktreesConfig
    Herdr       HerdrConfig
    Sandcastle  SandcastleConfig
}
```

### Herdr Config

```go
type HerdrConfig struct {
    Enabled             bool   `toml:"enabled"`
    Binary              string `toml:"binary"`
    PollIntervalSeconds int    `toml:"poll_interval_seconds"`
    PreferWorktreeAPI   bool   `toml:"prefer_worktree_api"`
}
```

### Sandcastle Config

```go
type SandcastleConfig struct {
    Enabled             bool   `toml:"enabled"`
    Binary              string `toml:"binary"`
    PollIntervalSeconds int    `toml:"poll_interval_seconds"`
    DefaultAgent        string `toml:"default_agent"`
}
```

### Defaults

```toml
[herdr]
enabled = true
binary = "herdr"
poll_interval_seconds = 5
prefer_worktree_api = true

[sandcastle]
enabled = true
binary = "sandcastle"
poll_interval_seconds = 5
default_agent = "pi"
```

### Compatibility

- Keep `[ai_agents]` for existing users while direct launchers remain.
- Treat `[ai_agents]` as legacy once Sandcastle workflow start is stable.
- Settings should eventually display integration availability.

### Acceptance Criteria

- Missing config fields default correctly.
- Existing config files still load.
- Saved config includes Herdr and Sandcastle defaults.
- Default Sandcastle agent is `pi`.

## Phase 16: Error and Degraded-State UX

### Purpose

Keep the dashboard honest when integrations fail.

### Sandcastle Unavailable

```text
Sandcastle: unavailable
Workflow status cannot be refreshed.
Existing worktree and GitHub data are still available.
```

### Herdr Outside Herdr

```text
Herdr: standalone
Using local terminal behavior.
```

### Herdr Degraded Inside Herdr

```text
Herdr: degraded
Could not query Herdr server: <error>
Pane jumps disabled.
```

### Worktree Fallback

```text
Herdr could not create this worktree.

Options:
  1. Retry Herdr
  2. Create with Git directly and register with Herdr afterward
  3. Cancel
```

If registration fails:

```text
Worktree created, but not attached to Herdr.
Jump target unavailable.
```

### Acceptance Criteria

- Integration failures are visible.
- Failures do not look like success.
- Fallback is explicit.
- Repeated polling errors do not spam the user.

## Phase 17: Testing Plan

### Mission-Control Builder Tests

Cover:

- Worktree plus PR branch produces one work item.
- Worktree plus issue branch produces one work item.
- Sandcastle workflow attaches to worktree by path.
- Sandcastle workflow attaches to issue by metadata.
- Pi Agent from Sandcastle appears under workflow.
- Herdr pane attaches to Pi Agent by pane ID.
- Herdr pane attaches to worktree by CWD.
- Missing Sandcastle produces unavailable integration status.
- Missing Herdr outside Herdr produces standalone status.
- Stale Herdr pane ID marks work item degraded.
- Malformed Sandcastle status produces warning but preserves previous state.

### Sandcastle Adapter Tests

Cover:

- Parses workflow list.
- Parses workflow start response.
- Sends `--agent pi` by default.
- Preserves a custom agent only if explicitly passed.
- Handles missing binary.
- Handles command failure.
- Handles malformed JSON.
- Handles empty workflow list.
- Maps statuses correctly.

### Herdr Adapter Tests

Cover:

- Detects Herdr environment.
- Parses pane list.
- Parses agent list.
- Parses worktree list.
- Opens worktree.
- Creates worktree.
- Focuses pane.
- Handles missing binary.
- Handles command failure.
- Does not assume focused pane.
- Preserves stderr in errors.

### Bubble Tea Update Tests

Cover:

- Mission dashboard is default or reachable.
- Pressing Enter opens detail dashboard.
- Escape returns to global dashboard.
- Start workflow action dispatches Sandcastle request.
- Start workflow action defaults to Pi Agent.
- Herdr jump action dispatches focus/open command.
- Integration refresh updates mission-control state.
- Integration errors show visible degraded state.

### Renderer Tests

Cover:

- Global dashboard shows worktrees, workflows, agents, and integrations.
- Workflow card shows Pi Agent.
- Progress bar renders percent.
- Blocked status is visually distinct.
- Failed status is visually distinct.
- Worktree detail shows linked issue, PR, workflow, and pane.
- Agent detail shows lifecycle owner `Sandcastle`, observer `Herdr` when applicable, and kind `Pi`.
- Raw Sandcastle JSON is not displayed.

### Persistence Tests

Cover:

- Session can store Herdr workspace/tab/pane IDs.
- Legacy local PID sessions still load.
- Runtime refs survive restart.
- Workflow refs persist.
- Stale refs can be marked degraded.
- No credentials are persisted.

## Phase 18: Suggested Implementation Order

### Milestone 1: ADR, Config, and Domain

- Finalize ADR wording.
- Add Herdr config.
- Add Sandcastle config.
- Add mission-control domain types.
- Add status enums.
- Add config tests.

Deliverable: Grove compiles with new domain/config types and no major UI change.

### Milestone 2: Sandcastle Adapter

- Add command runner.
- Add Sandcastle client.
- Parse workflow list.
- Start workflow with default `pi`.
- Add adapter tests.

Deliverable: Grove can call or fake Sandcastle and parse workflow state.

### Milestone 3: Herdr Adapter

- Add Herdr client.
- Parse panes, agents, and worktrees.
- Implement pane open/focus.
- Add adapter tests.

Deliverable: Grove can call or fake Herdr and normalize pane/worktree/agent state.

### Milestone 4: Mission-Control State Builder

- Add builder input/output.
- Correlate worktrees, issues, PRs, workflows, agents, panes, and sessions.
- Add comprehensive tests.

Deliverable: one function produces the future dashboard state.

### Milestone 5: Persistence Upgrades

- Migrate active sessions.
- Add integration refs.
- Add workflow cache if needed.
- Preserve old sessions.

Deliverable: Grove can persist Herdr/Sandcastle references without breaking existing sessions.

### Milestone 6: Integration Polling

- Add Herdr/Sandcastle refresh commands.
- Add Bubble Tea messages.
- Add 5-second tick.
- Rebuild mission-control state after refresh.
- Show integration availability and degraded status.

Deliverable: Grove has live-ish mission-control state in memory.

### Milestone 7: Global Mission-Control Dashboard

- Add dashboard view.
- Add integration badges.
- Add workflow cards.
- Add Pi Agent rows.
- Add renderer tests.

Deliverable: first visible version of the new Grove control center.

### Milestone 8: Drill-Down Dashboards

- Add worktree detail.
- Add agent detail.
- Add workflow detail.
- Add navigation.
- Add fuzzy search integration.

Deliverable: user can drill into worktrees, agents, and workflows, then return to overview.

### Milestone 9: Start Sandcastle Workflows

- Add start workflow modal/action.
- Default to Pi.
- Start from worktree.
- Start from issue.
- Start from PR.
- Refresh status.

Deliverable: Grove starts Pi-backed Sandcastle workflows.

### Milestone 10: Herdr Jump/Open/Worktree Mode

- Open/jump pane.
- Herdr-mode worktree open/create.
- Explicit fallback flow.
- Degraded-state rendering.

Deliverable: Grove can jump into Herdr terminals and use Herdr-managed runtime worktrees.

### Milestone 11: Docs and Legacy Launcher Cleanup

- Update README.
- Update RUNBOOK.
- Update help modal.
- Mark direct launchers legacy or hide them behind config.

Deliverable: Grove's documented flow is Sandcastle-first.

## Proposed PR Breakdown

### PR 1: ADR, Config, Domain Foundation

- Finalize ADR.
- Add Herdr/Sandcastle config structs.
- Add mission-control domain types.
- Add status enums.
- Add config tests.

### PR 2: Sandcastle Adapter

- Add command runner.
- Add Sandcastle client.
- Parse workflow list.
- Start workflow with default `pi`.
- Test command success/failure/malformed JSON.

### PR 3: Herdr Adapter

- Add Herdr client.
- Parse panes, agents, workspaces, and worktrees.
- Implement pane open/focus.
- Test Herdr command behavior.

### PR 4: Mission-Control State Builder

- Add builder.
- Correlate worktrees, issues, PRs, workflows, agents, panes, and sessions.
- Add comprehensive tests.

### PR 5: Persistence Upgrades

- Migrate active sessions.
- Add integration refs.
- Add workflow cache if needed.
- Preserve old sessions.

### PR 6: Mission-Control Dashboard UI

- Add dashboard view.
- Add integration badges.
- Add workflow cards.
- Add Pi Agent rows.
- Add renderer tests.

### PR 7: Drill-Down Dashboards

- Add worktree detail.
- Add agent detail.
- Add workflow detail.
- Add navigation and fuzzy search.

### PR 8: Start Sandcastle Workflows

- Add start workflow modal/action.
- Default to Pi.
- Start from worktree, issue, and PR.
- Refresh status after start.

### PR 9: Herdr Jump/Open/Worktree Mode

- Add open/jump pane behavior.
- Add Herdr-mode worktree open/create.
- Add explicit fallback flow.
- Add degraded-state rendering.

### PR 10: Documentation and Legacy Launcher Cleanup

- Update README.
- Update RUNBOOK.
- Update help modal.
- Mark direct launchers legacy or hide behind config.

## Key Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Grove, Herdr, and Sandcastle disagree about ownership | Keep ADR ownership strict: Grove correlates and requests workflow starts, Herdr owns terminal runtime, Sandcastle owns workflow and agent lifecycle |
| Dashboard becomes a raw JSON/log viewer | Normalize into mission-control state and render native components |
| First implementation gets too large | Ship dashboard/status/start/jump first; delay stop/pause/retry and GitHub mutations |
| Herdr IDs go stale | Store Herdr IDs plus Git identity and reconcile by path/branch |
| Sandcastle contract changes | Use adapter layer and tests around Grove's normalized model |
| Existing Grove users lose functionality | Preserve standalone mode and legacy launchers until Sandcastle flow is stable |
| Polling causes noisy UI errors | Keep previous good snapshot and render stable degraded integration state |
| Pi Agent ownership gets blurred | Grove never launches `pi`; Grove asks Sandcastle to start Pi-backed workflows |

## Definition of Done for Phase 1

- Grove has a mission-control dashboard.
- Grove has drill-down dashboards for worktrees, agents, and workflows.
- Grove can display Sandcastle workflow status visually.
- Grove can start Sandcastle workflows that default to Pi Agent.
- Grove can show Pi Agent status as part of Sandcastle workflow state.
- Grove can open or jump to Herdr panes when Herdr is available.
- Grove can use Herdr-managed worktree open/create in Herdr mode.
- Grove preserves standalone behavior outside Herdr.
- Grove does not launch coding agents directly in the new target flow.
- Integration failures are visible, not silent.
- The mission-control state builder is covered by tests.
- Herdr and Sandcastle adapters are covered with fake command runners.
