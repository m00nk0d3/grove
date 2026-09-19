# Issue #170: Phase 2 Plan — Correlation Layer

## Summary

Phase 2 implements the **mission-control state builder** that correlates disparate data sources (GitHub, worktrees, Sandcastle workflows, Herdr panes) into a coherent `MissionControlState`. This is the "glue" layer that turns separate snapshots into one dashboard-worthy state.

This plan specifies:
- Files to modify/create
- Correlation rules and their precedence order
- Edge cases and degraded-state handling
- Test requirements
- NOT actual implementation code

## Target Location

Primary implementation in `internal/mission/control.go` (extends existing `BuildState`).

No new packages needed for Phase 2. All correlation logic lives in the mission control domain package.

---

## 1. Files to Modify/Create

### Create: `.agent/issue-170/PLAN.md`
This plan file documenting what follows.

### Create: `internal/mission/control_test.go` (expand existing)
Add comprehensive tests for all correlation rules and degraded-state scenarios. See test requirements section.

### Modify: `internal/domain/missioncontrolstate.go`
Expand `MissionControlState` with Phase 2-relevant fields that the builder will populate. Keep it simple—only add what's needed to represent correlated state, not raw integration data.

### Modify: `internal/mission/control.go`
Implement correlation logic in the `BuildState` function and supporting helper functions.

---

## 2. Correlation Rules (Precedence Order)

Exact matches win over fuzzy fallbacks. Each rule has a clear precedence order.

### Worktree ↔ Pull Request Matching

```
1. Exact branch match: worktree.Branch == PR.Branch → Linked
2. Branch containment: worktree.Branch contains PR.Branch as substring (e.g., "issue-42-fix" contains "issue-42") → Linked
3. Existing metadata link (from previous state or saved refs) → Linked
4. Fallback by path/name similarity if branches are similar → Tentative match (mark with warning)
```

**Edge Cases:**
- Worktree branch is a WIP extension of PR branch ("feat-x" vs "feat") → Only link if user explicitly confirmed via metadata; otherwise don't auto-link.
- Multiple worktrees have identical branch names in different repos → Don't auto-link; require explicit repo path match.

---

### Worktree ↔ Issue Matching

```
1. Existing metadata link → Linked
2. Branch pattern match: "issue-XXX", "XXX-title", "bug-XXX" prefix → Tentative link (add warning unless confirmed)
3. Linked PR points to issue + branch fallback → Linked if PR already matched
4. Sandcastle workflow metadata links worktree to issue → Linked via workflow correlation
```

**Edge Cases:**
- Branch named "issue-42" but no GitHub issue #42 exists → Don't link; mark as unlinked/manual.
- Multiple issues with similar prefixes → Only link if exact number match or confirmed metadata.

---

### Sandcastle Workflow ↔ Worktree Matching

```
1. Exact worktree_path match (normalized paths, trailing slash ignored) → Linked
2. Repo + branch combination match → Tentative link if repo is same and branch matches
3. GitHub issue/PR ref in workflow metadata → Cross-link via known issue/PR
4. Workflow labels/tags matching worktree naming convention → Tentative (add warning)
```

**Edge Cases:**
- Workflow points to `/repo/foo` but worktree is `/home/user/worktrees/foo` → Normalize both paths before compare.
- Worktree created after workflow started → Link if path matches; workflow state may be stale until refresh.

---

### Sandcastle Agent ↔ Workflow Matching

```
1. Agent belongs to workflow (per Sandcastle run ID) → Linked
2. Default agent for workflow is Pi → Map `default_agent: pi` to `AgentRef.Kind = "pi"`
3. Agent summary or title references worktree/issue → Optional metadata enrichment
```

**Edge Cases:**
- Multiple agents in one workflow → Represent all in `AgentRef[]` slice under the workflow/workitem.
- Pi Agent not explicitly listed but workflow started from Grove → Assume default agent is Pi unless overridden.

---

### Herdr Pane ↔ Sandcastle/Pi Agent Matching

```
1. Pane ID reported by Sandcastle matches pane observed by Herdr → Linked
2. CWD of Herdr pane matches worktree path (normalize paths) → Tentative link
3. Agent name/ID exposed by both systems matches → Linked
4. Recent command/title or terminal content → Optional metadata enrichment; not a primary match rule
```

**Edge Cases:**
- Pane ID stale (Herdr pane removed, agent still running) → Mark workitem degraded; keep previous state but show warning.
- Agent name differs between Sandcastle and Herdr → Link if other criteria match; add metadata note about mismatched names.
- Herdr shows multiple panes for same worktree → Pick the one with matching CWD or most recent agent ID.

---

### Grove Session ↔ Herdr Pane Matching

```
1. Stored pane ID in session matches current Herdr → Linked
2. Worktree path matches → Tentative link
3. Legacy PID-only (local runtime) → Keep as-is but mark as potentially stale outside Herdr
```

**Edge Cases:**
- Session has `ShellPID` but no Herdr pane → Still valid for local runtime sessions; don't deprecate purely based on missing pane ID.
- Herdr env vars present but snapshot fails → Standalone mode, show last known state.

---

## 3. Degraded-State Rules

The builder must always produce a complete `MissionControlState`, even when integrations are partially unavailable. Never panic or return errors—mark things degraded instead.

### Sandcastle Unavailable

- If no Sandcastle snapshot: set `Integrations.Sandcastle.Mode = "missing"`
- Preserve previous valid workflow state if available via persistence
- Add `Warning` with message about Sandcastle unavailability
- Don't delete existing workflows; mark them stale if refresh fails repeatedly

### Herdr Unavailable (Outside Herdr)

- Set `Integrations.Herdr.Mode = "standalone"`
- Grove continues local worktree/Git behavior
- Sandcastle status can still render if binary available
- No pane jumps or Herdr worktree opens; mark those actions as unavailable

### Herdr Degraded (Inside Herdr)

- Set `Integrations.Herdr.Mode = "degraded"`
- Disable pane jump functionality
- Show error message to user
- Previous state still renders; don't wipe on first failure

### Malformed Sandcastle JSON

- Try parsing with `go json.Decoder`; if unmarshaling fails, return an error from adapter layer
- Catch error, log it, and preserve previous valid snapshot if available via persistence
- Add `Warning` with integration error details (truncate long errors)

### Missing Integration Reference

- If a worktree exists but has no matching workflow/issue/PR: keep it in state as standalone workitem
- Don't delete orphaned worktrees from state; they're still valid Git workspaces

---

## 4. Domain Type Adjustments

### `MissionControlState` (add fields)

Add these new fields to capture correlation results:

```go
type MissionControlState struct {
    Status       string // "unknown" | "degraded"
    Details      map[string]interface{} // For runtime details if needed
    RepoPath     string
    WorkItems    []WorkItem
    Worktrees    []Worktree
    Issues       []Issue
    PullRequests []PullRequest
    Sessions     []Session
    WorkflowRuns []WorkflowRunRef
    Agents       []AgentRef
    Panes        []PaneRef
    Integrations IntegrationStatus
    Warnings     []Warning
    UpdatedAt    time.Time

    // Phase 2: Correlation metadata (keep separate from primary fields)
    Correlations struct {
        WorktreeToPR     map[string]string // worktree path -> linked PR number or ""
        WorktreeToIssue  map[string]string // worktree path -> linked issue number or ""
        WorkflowToWorktree map[string]string // workflow ID -> linked worktree path
        AgentToWorkflow   map[string]string // agent ID -> linked workflow ID
        PaneToAgent       map[string]string // pane ID -> linked agent ID (empty if linked to workitem directly)
    }
}
```

Keep the correlation maps internal to `MissionControlState`; they're implementation details for rendering, not public-facing fields. The `WorkItem` struct already has pointers to related items; use those for primary links.

### `WorkItem` (already exists, confirm shape)

```go
type WorkItem struct {
    ID        string
    CreatedAt int64
    UpdatedAt int64
}
```

Phase 2 doesn't need to expand this yet. Future phases will add `Kind`, `Title`, `Status`, etc. Keep it simple now—just an identifier and timestamps.

---

## 5. Builder Function Signature (Keep Existing, Enhance Behavior)

Current signature from `control.go`:

```go
type BuildInput struct {
    RepoPath           string
    Worktrees          []domain.Worktree
    Issues             []domain.Issue
    PullRequests       []domain.PullRequest
    Sessions           []domain.Session
    HerdrSnapshot      *herdr.Snapshot
    SandcastleSnapshot *sandcastle.Snapshot
    PreviousState      *domain.MissionControlState // Optional: for stateless runs, use nil
    Now                time.Time
}

func BuildState(input BuildInput) domain.MissionControlState
```

Phase 2 **does not change this signature**. The correlation logic lives inside `BuildState`.

Add comments to describe the correlation order. No new parameters needed.

---

## 6. Edge Cases Summary

| Case | Behavior |
| --- | --- |
| No Herdr snapshot (outside Herdr) | Standalone mode; no pane jumps; show last-known state or empty panes |
| No Sandcastle snapshot | Unavailable integration; preserve previous workflow state if available via persistence |
| Malformed Sandcastle JSON | Log error; use previous valid state; mark integration degraded |
| Worktree without PR/issue match | Keep as standalone workitem; don't auto-link to anything |
| Multiple potential PR links for same branch | Only link if exact match or confirmed metadata; otherwise add warning |
| Stale Herdr pane ID | Mark related workitem degraded; keep previous state visible |
| Agent without workflow | Can still appear as standalone agent ref; show in agents list but not linked to workflow |
| Pane with no agent ID | Link to workitem directly via CWD/worktree path if available |

---

## 7. Test Requirements

Create tests for `internal/mission/control_test.go` covering:

### Correlation Tests

1. **Worktree-PR exact branch match**
   - Given worktree with branch "issue-42" and PR with head branch "issue-42"
   - BuildState links them
2. **Worktree-PR branch containment**
   - Given worktree "issue-42-fix" and PR "issue-42"
   - BuildState links them (contains match)
3. **Worktree-PR no match, different repo path**
   - Given same branch name but different repo paths
   - Don't link; keep both independent
4. **Worktree-issue via PR fallback**
   - Worktree linked to PR that links to issue → transitive linkage works
5. **Orphan worktree (no matches)**
   - Worktree exists but no matching PR/issue/workflow
   - Appears as standalone workitem

### Sandcastle Integration Tests

6. **No Sandcastle snapshot**
   - Given `SandcastleSnapshot = nil`
   - `Integrations.Sandcastle.Mode = "missing"`
7. **Malformed JSON in adapter**
   - Adapter returns error; builder catches and marks degraded
8. **Previous state preserved on refresh failure**
   - First run succeeds, second fails due to malformed JSON
   - State still renders with stale marker

### Herdr Integration Tests

9. **No Herdr snapshot (outside Herdr)**
   - `HerdrSnapshot = nil` → standalone mode
10. **Malformed Herdr JSON**
    - Adapter error handled; previous state preserved if available
11. **Stale Herdr pane ID**
    - Pane in previous state not found in current snapshot
    - Workitem marked degraded

### Agent-Panework Items Tests

12. **Pane to agent via ID match**
    - Given matching pane ID and agent ID
    - Link works correctly
13. **CWD-based fallback linkage**
    - No agent ID but CWD matches worktree path → link to workitem
14. **Multiple panes for same worktree**
    - Pick one with most relevant criteria; mark others appropriately

### Degraded-State Tests

15. **Sandcastle unavailable, workflows still render**
16. **Herdr degraded inside Herdr**
17. **Malformed integration JSON → warning added**
18. **Empty inputs → empty MissionControlState**

---

## 8. Implementation Notes

### Do NOT Call External Commands from BuildState

Per issue requirements: "Correlation-only issue; one agent should focus on builder tests and matching helpers."

The `BuildState` function must be pure given its input. If Herdr/Sandcastle adapters need to call external commands, that happens in the adapter layer, not here. This plan keeps `BuildState` stateless with respect to external process execution.

### Use Provided Types

- `domain.Worktree`, `domain.Issue`, `domain.PullRequest`, `domain.Session` are already defined
- `herdr.Snapshot` and `sandcastle.Snapshot` types will be provided by adapter layers (or mocked in tests)
- No need to create new domain types for Phase 2

### Cloning Logic Already Exists

The existing `cloneWorktrees`, `cloneIssues`, etc. functions handle defensive deep copying. Keep that behavior; it prevents accidental mutation of source data.

---

## 9. Acceptance Criteria (Issue #170)

Phase 2 is complete when:

- [ ] Linked worktree/issue/PR/workflow/agent/pane state produces one coherent work item where appropriate
- [ ] Exact IDs/paths/branches win over fuzzy fallbacks (precedence rules implemented)
- [ ] Matching is deterministic given the same inputs
- [ ] Correlation tests pass for all scenarios in section 7 above
- [ ] Degraded-state handling doesn't crash or panic
- [ ] Previous state is preserved gracefully when integrations are unavailable
- [ ] No external commands called from `BuildState`

---

## 10. Out of Scope (Per Issue #170)

The following are explicitly **not** part of Phase 2 implementation:

- Calling external commands from `BuildState`
- Real-time push updates from Herdr or Sandcastle (Phase 6+)
- GitHub mutation actions (labels, comments, etc.)
- Full Herdr plugin packaging
- Remote Herdr machine support
- Direct Grove agent launchers (legacy behavior)

These will come in later phases.

---

## 11. Related Issues & Dependencies

- **Blocked by**: Builder API issue #169 (ensure adapter layers are stable first)
- **Parent epic**: Issue #150 (Grove Mission Control)

Ensure issue #169 is resolved before implementing this plan, per the implementation plan's explicit dependency note.
