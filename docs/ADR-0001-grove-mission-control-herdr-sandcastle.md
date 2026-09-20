# ADR-0001: Grove Mission Control with Herdr and Sandcastle

## Status

Proposed

## Date

2026-09-17

## Context

Grove currently acts as a terminal UI for managing Git worktrees, syncing GitHub pull requests and issues, launching AI coding agents, and tracking active local sessions. The current product already has the foundations of a control center:

- A Bubble Tea/Lip Gloss terminal UI with worktree, issue, pull request, settings, fuzzy finder, help, and active-session surfaces.
- Git worktree operations owned directly by Grove through local Git integration.
- GitHub pull request and issue synchronization through the GitHub CLI.
- Historical direct launchers for Claude Code, GitHub Copilot CLI, and Aider
  were removed after workflow launching moved to Sandcastle.
- Local persistence for configuration, GitHub metadata, agent history, and active sessions.
- Active sessions tracked primarily through process IDs and worktree paths.

The desired direction is larger than simply making Grove aware of Herdr. Grove should become a local control center for development work: a place to see all worktrees, agents, issues, pull requests, workflows, panes, and implementation progress, then drill into any selected worktree or agent for a more detailed dashboard.

Herdr and Sandcastle should fit into this direction as infrastructure layers:

- Herdr owns terminal reality: workspaces, tabs, panes, focus, persistent terminal sessions, pane-local process visibility, and jump targets. It may report that an agent is present in a pane, but it does not own the agent's workflow or process lifecycle.
- Sandcastle owns agent workflow execution: agent launching, multi-agent orchestration, workflow runs, steps, progress, and workflow lifecycle. Pi Agent is the primary/default coding agent for Sandcastle-driven Grove workflows.
- Grove owns the human-facing control-center experience: the dashboard, navigation, correlation, summaries, and high-level orchestration UX.
- GitHub owns issue and pull-request records. Grove reads and correlates that metadata in phase 1; GitHub mutations are deferred.

The user explicitly prefers Sandcastle status to be rendered visually inside Grove using the existing Go TUI framework. Grove should not display Sandcastle status as raw CLI output or as an external dashboard. Sandcastle should provide structured data, and Grove should convert that into native visual components.

## Decision

Grove will remain a standalone application first, with a Herdr-aware mode when it is running inside Herdr. Grove will become the primary control-center UI and unified state model. Herdr and Sandcastle will be integrated through adapters rather than shaping Grove's domain model directly.

The ownership model is:

| Concern | Owner |
| --- | --- |
| Global dashboard UX | Grove |
| Drill-down dashboards | Grove |
| Worktree, agent, PR, issue, pane, and workflow correlation | Grove |
| Visual Sandcastle workflow status | Grove |
| GitHub PR and issue metadata display | Grove |
| GitHub PR and issue mutation actions | Out of scope for phase 1 |
| Terminal panes, workspace/tab/pane IDs, focus, and jumps | Herdr |
| Persistent terminal runtime | Herdr |
| Pane-local agent presence and terminal state | Herdr |
| Agent process lifecycle (launch, supervision, completion) | Sandcastle |
| Sandcastle workflow execution | Sandcastle, defaulting to OpenCode for coding work |
| Multi-agent orchestration | Sandcastle |
| Workflow lifecycle controls beyond start | Out of scope for phase 1 |

Grove's phase-1 goal is not to expose every possible control. The phase-1 goal is to establish the correct product shape and state model:

1. A global mission-control dashboard.
2. Drill-down dashboards for selected worktrees and agents.
3. Herdr-backed pane open/jump support.
4. Herdr-managed worktree create/open support when running inside Herdr.
5. Sandcastle workflow start support.
6. Visual Sandcastle workflow status display inside Grove.
7. GitHub PR/issue metadata correlation with worktrees and workflow runs.

Grove should not directly mirror Herdr's data model or Sandcastle's data model. Instead, Grove should define its own mission-control domain model and use adapters to project external state into that model.

Grove should not launch coding agents directly in the target architecture. Grove starts and monitors Sandcastle workflow runs; Sandcastle decides how to launch, supervise, and coordinate agents. For Grove-started coding workflows, Sandcastle should use Pi Agent by default unless the workflow explicitly declares another agent kind.

Ownership and observation are distinct: Grove may display an agent, and Herdr may observe its pane or process, without either system owning that agent's lifecycle. Sandcastle remains the lifecycle owner for every agent in the target flow.

## Problem Statement

Developers running many concurrent AI-assisted development streams need one place to understand what is happening. They need to see worktrees, agents, issues, pull requests, terminal panes, and workflow progress together instead of switching between Git commands, GitHub, terminal tabs, Herdr, and Sandcastle.

The Grove experience reduces context switching for Git worktrees, GitHub
metadata, and Sandcastle workflows. Grove needs a clear control-center model
and ownership boundary so users do not have to mentally connect:

- A Git worktree in Grove.
- A Herdr pane or workspace where the terminal is running.
- A Sandcastle workflow run that owns the agent work.
- A GitHub issue or pull request that describes the task.
- One or more coding agents performing the implementation.

The user wants Grove to show the overall state, allow selecting a worktree or agent, open a dedicated dashboard for that selected item, inspect implementation progress, then return to the overall dashboard.

## Solution

Grove will introduce a mission-control experience built on its existing Bubble Tea/Lip Gloss TUI. The global dashboard will show the overall state of worktrees, agents, issues, pull requests, Herdr panes, and Sandcastle workflow runs. Selecting a worktree or agent will open a drill-down dashboard that explains what is happening there.

Sandcastle status will be consumed as structured CLI JSON and rendered visually inside Grove. The UI should show workflow run cards, Pi Agent status rows, progress indicators, step timelines, blocked/error badges, and concise output summaries. Raw Sandcastle logs can remain available as a linked detail or debug view later, but they are not the primary phase-1 experience.

Grove should initiate Sandcastle workflows instead of spawning coding agents itself. Existing direct launchers can remain as legacy/current behavior until replaced, but the future control-center flow is: Grove starts a Sandcastle workflow, Sandcastle launches Pi Agent and any supporting agents, Herdr owns the terminal panes, and Grove renders the resulting state.

Herdr compatibility will be implemented as a runtime adapter. When Grove is running inside Herdr, Grove will use Herdr for terminal and worktree runtime operations. When Grove is not running inside Herdr, Grove will preserve the current standalone behavior.

In Herdr mode:

- Worktree create/open operations should go through Herdr where possible.
- Pane creation and focus should go through Herdr.
- Grove should store Herdr workspace, tab, and pane identifiers alongside Grove's own worktree/session identity.
- Grove should poll Herdr at a moderate cadence for phase-1 status updates.
- If a Herdr operation fails, Grove should show the failure and offer an explicit direct-Git fallback for that action.
- If fallback is accepted, Grove should attempt to register or open the resulting worktree in Herdr afterward; if that fails, Grove should mark the item as degraded or unattached.

In standalone mode:

- Grove should continue to use its existing Git worktree operations.
- Grove should continue to launch shells using the existing local terminal behavior until a runtime abstraction replaces direct calls. Direct agent launching should be treated as legacy/current behavior and moved behind Sandcastle workflow starts in the target architecture.
- Grove should still display Sandcastle state if Sandcastle is available.

## User Stories

1. As a developer, I want Grove to show all active worktrees in one dashboard, so that I can understand what work is in progress.
2. As a developer, I want Grove to show all active agents in one dashboard, so that I can see which agents are working, idle, blocked, or done.
3. As a developer, I want Grove to show GitHub issues alongside worktrees, so that I can connect implementation work to the original task.
4. As a developer, I want Grove to show GitHub pull requests alongside worktrees, so that I can track review-ready work without leaving the terminal.
5. As a developer, I want Grove to show Sandcastle workflow runs visually, so that I can understand agent progress without reading raw logs.
6. As a developer, I want Grove to show Herdr pane targets, so that I can jump directly to the terminal where work is happening.
7. As a developer, I want Grove to remain usable without Herdr, so that I can keep the existing standalone workflow.
8. As a developer running inside Herdr, I want Grove to use Herdr for pane operations, so that terminal sessions stay persistent and jumpable.
9. As a developer running inside Herdr, I want Grove to use Herdr-managed worktree operations, so that Herdr and Grove do not disagree about runtime worktrees.
10. As a developer, I want Grove to clearly warn me when Herdr operations fail, so that I do not accidentally create split-brain state.
11. As a developer, I want Grove to offer explicit fallback when Herdr fails, so that I can keep working while understanding the tradeoff.
12. As a developer, I want fallback-created worktrees to be marked as degraded or unattached if Herdr registration fails, so that the dashboard stays honest.
13. As a developer, I want to press a worktree and see a worktree detail dashboard, so that I can inspect implementation status for that worktree.
14. As a developer, I want the worktree detail dashboard to show branch state, dirty state, linked issue, linked pull request, active workflow, active agents, and Herdr panes, so that I can understand the complete context.
15. As a developer, I want to press an agent and see an agent detail dashboard, so that I can inspect what that agent is doing.
16. As a developer, I want the agent detail dashboard to show agent kind, status, assigned workflow, current task, current worktree, Herdr pane, and recent summary, so that I can decide whether to intervene.
17. As a developer, I want Grove to show blocked agents prominently, so that I can answer questions or approvals quickly.
18. As a developer, I want Grove to show failed workflow steps prominently, so that I can identify broken runs quickly.
19. As a developer, I want Grove to show idle agents separately from working agents, so that I can reuse available capacity.
20. As a developer, I want Grove to show implementation progress as a visual timeline, so that I can quickly understand what has happened and what remains.
21. As a developer, I want Grove to show concise workflow output summaries, so that I can understand progress without reading a full terminal transcript.
22. As a developer, I want Grove to link workflow runs to issues and pull requests, so that I can navigate from planning to implementation to review.
23. As a developer, I want Grove to link workflow runs to worktrees, so that I know where files are being changed.
24. As a developer, I want Grove to link workflow runs to Herdr panes, so that I can jump to the live terminal when needed.
25. As a developer, I want Grove to link workflow runs to agents, so that I can see which agent is responsible for each part of the work.
26. As a developer, I want to start a Sandcastle workflow from Grove for a selected worktree, so that I can begin Pi Agent work from the control center.
27. As a developer, I want to start a Sandcastle workflow from Grove for a selected GitHub issue, so that I can move from issue triage to implementation.
28. As a developer, I want to start a Sandcastle workflow from Grove for a selected pull request, so that I can run review or follow-up automation.
29. As a developer, I want Grove to show only supported phase-1 workflow controls, so that the UI does not imply actions that are not reliable yet.
30. As a developer, I want stop, pause, and retry controls to be delayed until they are reliable, so that phase 1 does not become fragile.
31. As a developer, I want GitHub mutation actions to be delayed until later, so that phase 1 can focus on dashboard and workflow visibility.
32. As a developer, I want Sandcastle workflows started from Grove to use Pi Agent by default, so that Grove has one consistent primary coding-agent path.
33. As a developer, I want Sandcastle to own agent launching and orchestration, so that Grove does not duplicate process management logic.
34. As a developer, I want Grove to display agents launched by Sandcastle, so that the dashboard reflects the real workflow executor.
35. As a developer, I want Grove to display agents visible through Herdr or Sandcastle even if Grove did not launch them, so that the dashboard reflects reality.
36. As a developer, I want externally discovered agents to be clearly marked by ownership source, so that I know whether Sandcastle, Herdr, or Grove knows how to control them.
37. As a developer, I want Grove to reconcile stale Herdr pane IDs using worktree path, branch, and repository identity, so that restarts do not break the dashboard.
38. As a developer, I want Grove to store Herdr IDs plus Git identity, so that it can both jump directly and recover when runtime IDs go stale.
39. As a developer, I want Grove to poll Herdr at a moderate cadence in phase 1, so that status feels current without requiring raw socket subscriptions.
40. As a developer, I want Grove to eventually support real-time socket updates, so that the dashboard can become more responsive later.
41. As a developer, I want Grove to parse Sandcastle CLI JSON instead of scraping text, so that status rendering is reliable.
42. As a developer, I want Grove to show malformed Sandcastle responses as explicit errors, so that broken integration is visible.
43. As a developer, I want Grove to show missing Sandcastle as an unavailable integration, so that standalone Grove still works.
44. As a developer, I want Grove to show missing Herdr as standalone mode, so that I do not get errors outside Herdr.
45. As a developer, I want Grove to respect Herdr's injected workspace, tab, and pane context, so that commands target the correct terminal session.
46. As a developer, I want Grove to avoid relying on another Herdr client's focused pane, so that actions are deterministic.
47. As a developer, I want Grove to keep focus stable when opening background panes, so that the dashboard does not steal my current terminal context.
48. As a developer, I want Grove to surface Herdr version or capability problems clearly, so that I know why a command is unavailable.
49. As a developer, I want Grove to render Sandcastle status using the existing theme system, so that the new dashboard feels native.
50. As a developer, I want status badges to be consistent across worktrees, agents, and workflow runs, so that I can scan the dashboard quickly.
51. As a developer, I want drill-down dashboards to have a clear back path to the global dashboard, so that navigation stays fast.
52. As a developer, I want fuzzy search to include workflow runs and agents, so that I can jump directly to the work I care about.
53. As a developer, I want active-session health checks to understand Herdr-backed sessions, so that Grove does not mark live Herdr panes as dead just because a launcher PID exited.
54. As a developer, I want local terminal sessions and Herdr-backed sessions to be represented consistently, so that the UI does not feel split across modes.
55. As a developer, I want Grove to persist enough integration state to restart cleanly, so that the dashboard is useful across sessions.
56. As a developer, I want Grove to show stale or unreconciled integration state distinctly, so that I do not trust outdated information.
57. As a developer, I want configuration to let me enable or disable Herdr and Sandcastle integrations, so that I can control the workflow.
58. As a developer, I want Grove to show integration availability in settings or status, so that I know whether Herdr and Sandcastle are connected.
59. As a developer, I want Grove to avoid storing credentials in integration metadata, so that local persistence stays safe.
60. As a developer, I want Grove to document the Herdr/Sandcastle ownership model, so that future implementation work does not blur responsibilities.
61. As a maintainer, I want a single high-level testing seam for mission-control state, so that tests cover behavior without coupling to implementation details.
62. As a maintainer, I want fake Herdr and Sandcastle adapters in tests, so that dashboard behavior can be tested without launching external tools.

## Implementation Decisions

- Grove remains standalone-first. Herdr compatibility is an enhanced runtime mode, not a hard dependency.
- Grove becomes the control-center UI and owns the unified mission-control state model.
- Herdr owns terminal runtime state: panes, tabs, workspaces, terminal focus, persistent sessions, pane jumps, and pane-local agent presence detection.
- Sandcastle owns workflow execution, agent launching, and multi-agent orchestration.
- GitHub owns issue and pull-request records; Grove's phase-1 integration is read-only display and correlation.
- Grove consumes Sandcastle state through CLI JSON in phase 1. The contract is documented in [SANDCASTLE_JSON_CONTRACT.md](./SANDCASTLE_JSON_CONTRACT.md) and is the implementation target for the Sandcastle adapter.
- Sandcastle workflow starts from Grove should default to OpenCode as the primary coding agent.
- Sandcastle workflow status from Grove should identify OpenCode as the default coding actor unless a workflow reports a different agent.
- Grove should not directly spawn coding agents in the target architecture; it should ask Sandcastle to start workflow runs.
- Grove renders Sandcastle status visually using the existing Bubble Tea/Lip Gloss UI stack.
- Grove should not display raw Sandcastle command output as the primary status display.
- Grove should introduce a mission-control domain model that is independent of both Herdr and Sandcastle.
- The mission-control model should correlate worktrees, agents, GitHub issues, GitHub pull requests, Herdr pane references, and Sandcastle workflow runs.
- The top-level product experience is a global dashboard with drill-down dashboards.
- The global dashboard should show worktrees, agents, issues, pull requests, workflow runs, and pane/session status.
- Worktree drill-down should show branch state, dirty state, linked pull request, linked issue, active workflow runs, active agents, pane targets, and recent progress.
- Agent drill-down should show agent kind, lifecycle status, assigned work, workflow run, worktree, Herdr pane, and recent summary.
- Pull request and issue drill-down should show GitHub metadata, linked worktree, linked workflow run, linked agents, and status.
- Herdr mode is detected through Herdr runtime environment and capability checks.
- Herdr integration should start through CLI commands that return JSON.
- Raw Herdr socket integration is deferred until live event subscriptions are required.
- Herdr polling should use a moderate cadence in phase 1, approximately every 5 seconds while Grove is running.
- Grove should store Herdr workspace, tab, and pane identifiers alongside Grove's own repository/worktree/branch identity.
- Grove should reconcile stale Herdr identifiers using Git identity such as repository, worktree path, branch, pull request, or issue when possible.
- Grove should not rely only on Herdr IDs because Herdr sessions, machines, and restarts can make old IDs stale.
- Grove should not rely only on Git identity because Herdr IDs are needed for direct pane jumps.
- In Herdr mode, worktree create/open operations should prefer Herdr's worktree commands.
- If Herdr worktree operations fail, Grove should show the error and offer explicit direct-Git fallback.
- Automatic silent fallback is rejected because it can create split-brain state.
- If direct-Git fallback creates or opens a worktree, Grove should attempt to register or open it in Herdr afterward.
- If post-fallback Herdr registration fails, Grove should mark the worktree as degraded or unattached.
- In standalone mode, Grove should preserve existing direct Git worktree behavior.
- Grove should introduce a terminal runtime abstraction rather than continuing to add terminal-specific branches directly into launch functions.
- The terminal runtime abstraction should support opening shells, running Sandcastle-managed commands in panes, focusing sessions, and listing known sessions. It should not make Grove the owner of agent process launch.
- A local terminal runtime should preserve current terminal-launch behavior.
- A Herdr runtime should use Herdr commands for pane creation, pane execution, pane focus, and reporting pane-local agent presence. Those observations do not transfer agent lifecycle ownership from Sandcastle to Herdr.
- Active sessions should evolve from PID-first tracking to runtime-reference tracking.
- PID-based session health remains valid for local runtime sessions.
- Herdr-backed session health should use Herdr pane or agent state rather than launcher process IDs.
- Historical direct agent launchers have been removed.
- Pi Agent is the main/default agent for Sandcastle-orchestrated coding workflows.
- Claude Code, GitHub Copilot CLI, Aider, and other agents may remain useful as
  specialist Sandcastle agent kinds, but Grove does not own their process
  launch path.
- Agent-related configuration should move toward Sandcastle workflow defaults rather than per-agent Grove launcher toggles.
- Herdr-backed agent visibility should come from Herdr and Sandcastle state, not from Grove launching the agent itself.
- Grove should show externally launched Herdr or Sandcastle agents read-only when it can discover them.
- Externally launched agents should be labeled by their reported source and must not be treated as Grove-owned.
- Grove should expose only reliable phase-1 write actions: start Sandcastle workflow, open or jump Herdr pane, and create or open Herdr-managed worktree.
- Stop, pause, and retry workflow controls are deferred.
- GitHub mutation actions such as labeling, assignment, and comments are deferred.
- Grove should continue using GitHub metadata for display and correlation in phase 1.
- Grove should surface integration failures explicitly rather than swallowing them.
- Missing Herdr should result in standalone mode, not a fatal startup error.
- Missing Sandcastle should result in Sandcastle status being unavailable, not a fatal startup error.
- Malformed Sandcastle JSON should show a clear integration error in the dashboard.
- Herdr command failures should preserve stderr or structured error details for user-facing diagnostics.
- Sandcastle command failures should preserve stderr or structured error details for user-facing diagnostics.
- Settings should eventually expose integration availability and enablement for Herdr and Sandcastle.
- Documentation should describe the ownership model so future contributors do not blur responsibilities.

### Suggested mission-control state shape

The exact Go structs can evolve, but the state model should look conceptually like this:

```go
type MissionControlState struct {
    WorkItems    []WorkItem
    Worktrees    []WorktreeRef
    Agents       []AgentRef
    WorkflowRuns []WorkflowRunRef
    Panes        []PaneRef
    GitHubRefs   []GitHubRef
    Integrations IntegrationStatus
}

type WorkItem struct {
    ID           string
    Kind         string
    Repo         string
    Worktree     *WorktreeRef
    GitHubRef    *GitHubRef
    WorkflowRuns []WorkflowRunRef
    Agents       []AgentRef
    Panes        []PaneRef
    Status       string
}
```

This is not intended as final code. It captures the architectural decision that Grove should correlate external systems into Grove-owned mission-control state instead of rendering raw adapter responses directly.

## Testing Decisions

- The highest-value testing seam is the mission-control state builder: given Grove worktrees, GitHub refs, Herdr panes/agents, and Sandcastle workflow runs, it should produce the dashboard state Grove renders.
- Tests should focus on external behavior: what the dashboard shows, what action is triggered, and what error is surfaced.
- Tests should avoid asserting internal adapter implementation details such as exact helper function names.
- Adapter tests should use fake command runners rather than invoking real Herdr or Sandcastle binaries.
- Herdr adapter tests should cover successful JSON parsing, command failure, missing binary, missing capability, stale pane references, and worktree fallback prompts.
- Sandcastle adapter tests should cover successful workflow list parsing, active run parsing, missing binary, malformed JSON, command failure, and empty state.
- Mission-control correlation tests should cover worktree-to-issue matching, worktree-to-pull-request matching, workflow-to-worktree matching, agent-to-worktree matching, and pane-to-agent matching.
- Dashboard tests should verify that global overview state includes worktrees, agents, issues, pull requests, workflows, and integration status.
- Drill-down tests should verify that selecting a worktree shows its linked GitHub ref, workflow run, agents, panes, and progress.
- Drill-down tests should verify that selecting an agent shows its kind, lifecycle status, worktree, workflow, pane, and recent summary.
- Degraded-state tests should verify that failed Herdr fallback registration is visible.
- Standalone-mode tests should verify that Grove still works when Herdr is unavailable.
- Sandcastle-unavailable tests should verify that Grove still works while marking workflow status unavailable.
- Sandcastle workflow-start tests should verify that Grove requests Pi Agent by default when no alternate agent kind is selected.
- Sandcastle status parsing tests should verify that Pi-backed workflow runs render as native Grove workflow and agent status, not as raw command output.
- Sandcastle workflow-start behavior replaces direct agent process-launch tests.
- Existing modal and app update tests are good prior art for testing UI behavior at a high seam.
- Existing config tests are good prior art for testing TOML defaults and persistence.
- Existing session tests are good prior art for extending persistence from PID-only sessions to runtime references.

## Proposed Test Seams

The preferred seam is a single high-level mission-control state seam:

1. Input: existing Grove worktrees, GitHub issues, GitHub pull requests, active sessions, Herdr runtime snapshot, and Sandcastle workflow snapshot.
2. Output: Grove mission-control dashboard state with correlated work items, statuses, warnings, and actions.

This seam keeps most tests independent of rendering details and adapter internals. Bubble Tea update tests can then verify that key user actions select the correct work item, open the correct drill-down dashboard, and dispatch the correct adapter command.

Secondary seams are only needed where external process boundaries exist:

- Herdr command adapter seam.
- Sandcastle command adapter seam.
- Terminal runtime abstraction seam.
- Persistence seam for runtime references.

## Out of Scope

- Raw Herdr socket subscriptions in phase 1.
- Real-time push updates from Herdr in phase 1.
- Real-time push updates from Sandcastle in phase 1.
- Stop, pause, or retry Sandcastle workflow controls in phase 1.
- GitHub mutation actions in phase 1, including labels, assignment, comments, and review actions.
- Full Herdr plugin packaging in phase 1.
- Remote Herdr machine support in phase 1.
- New direct Grove agent launchers.
- Making Grove the owner of agent process lifecycle.
- Treating Pi Agent as a Grove-spawned process instead of a Sandcastle default.
- Sandcastle workflow graph visualization as the primary top-level model in phase 1.
- A separate web UI or external dashboard for Sandcastle status.
- Replacing Grove's existing standalone mode.
- Replacing Herdr as terminal runtime.
- Replacing Sandcastle as workflow executor.
- Implementing security sandboxing for agents.
- Storing credentials or secrets in Grove integration metadata.

## Further Notes

- The product name in user-facing discussion is Grove, while some existing documentation and code comments still refer to Nexus. This ADR uses Grove for the intended product direction.
- Herdr should be treated as the terminal/runtime layer, not as the whole dashboard.
- Sandcastle should be treated as the workflow execution layer, not as the whole dashboard.
- Pi Agent should be treated as Sandcastle's primary/default coding agent for Grove-started workflows.
- Grove should be the place where users understand and navigate the combined system.
- Phase 1 should bias toward reliable visibility and navigation over broad mutation controls.
- The major design risk is split-brain ownership between Grove, Herdr, Sandcastle, and GitHub. The explicit ownership model in this ADR is intended to prevent that.
- The second major risk is overbuilding the first release. Stop/pause/retry and GitHub mutation actions are valuable, but they should follow after the mission-control model and visual dashboard are stable.
- The third major risk is rendering external system output directly. Sandcastle must provide structured state, and Grove must render it as native TUI components.
