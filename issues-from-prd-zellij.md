# Zellij Integration PRD → GitHub Issues Backlog

**Source:** `/home/m00nk0d3/development/grove/docs/prd-zellij-integration.md`  
**Date:** 2026-07-10  
**Workstreams:** External Config, Core Feature, Lifecycle, Tests, Docs  

---

## 📋 ISSUE BLOCK: #1 - External Layout Config File (P0 Must-Have)

### ID: ZELJ-001
### Title: Implement External Layout Configuration for Zellij Tab Spawning

### Context (PRD Section 6.1 & 7.2)
Layout configuration stored externally at `~/.config/grove/layouts/default.kdl` rather than hardcoded in Go code to support hot-reload and user customization.

### Requirements (From PRD FR-03, NFR-10)
- [ ] Create directory structure: `~/.config/grove/layouts/`
- [ ] Write default layout file at `layouts/default.kdl` (KDL format for Zellij)
- [ ] Add Go code to read layout file on startup
- [ ] Handle missing layout file gracefully (create with defaults)
- [ ] Set file permissions to 0644 (user-readable only)

### Acceptance Criteria
**AC1:** Layout file loads from `~/.config/grove/layouts/default.kdl`  
**AC2:** App does not crash if layout file is missing (creates default inline)  
**AC3:** File permissions set to 0644 on creation  
**AC4:** No network I/O — offline-first design, no remote fetch  
**AC5:** Layout can be edited without rebuilding Groove  

### Dependencies
- None

---

## 📋 ISSUE BLOCK: #2 - Core Feature: Spawn Zellij Tab (P0 Must-Have)

### ID: ZELJ-002
### Title: Implement Graceful Worktree Switch with Zellij Tab Spawning

### Context (PRD Section 5.1, US-01, US-02, NFR-01)
When user switches worktrees via `[s]` key, spawn new Zellij tab if integration enabled, fallback to `cd` if unavailable.

### Requirements (From PRD FR-01, FR-02)
- [ ] Add new message type: `type ZellijSpawnMsg struct { WorktreePath string }`
- [ ] Add dispatch handler in app.go switch statement for `ZellijSpawnMsg`
- [ ] Check if zellij binary exists before spawn (graceful fallback if not)
- [ ] Use layout from external config file or create temp layout inline
- [ ] Spawn via CLI: `zellij action new-tab --layout <layout> --name "Grove-%s"`
- [ ] Log failure silently if spawn fails (worktree switch still succeeds)

### Acceptance Criteria
**AC1:** New tab spawns with name "Grove: <worktree-path>"  
**AC2:** Layout matches external config file (or default inline layout)  
**AC3:** Left pane contains `bash` by default, right pane `grove -c`  
**AC4:** Tab focuses on right pane (Grove context) by default  
**AC5:** No crash if Zellij not installed (fallback to plain cd)  
**AC6:** Worktree switch succeeds even if tab spawn fails  

### Dependencies
- Issue #1 (External Layout Config File) — need layout file before spawn

---

## 📋 ISSUE BLOCK: #3 - Configuration Schema Updates (P0 Must-Have)

### ID: ZELJ-003
### Title: Add Zellij Integration Settings to Config Schema

### Context (PRD Section 5.2 FR-04, NFR-01, Out of Scope note)
Zellij integration is opt-in by default; users must enable via config file.

### Requirements (From PRD FR-04)
- [ ] Add `Zellij` section to `domain.Config` struct in `internal/domain/config.go`
- [ ] Add fields: `Enabled bool`, `MaxTabs int`, `CleanupIdleMinutes int`
- [ ] Update TOML parser to handle new config keys
- [ ] Set default value: `Enabled = false` (opt-in behavior)
- [ ] Update config file example in docs

### Acceptance Criteria
**AC1:** New config struct field added: `type ZellijConfig struct { Enabled bool `toml:"enabled"`; MaxTabs int `toml:"max_tabs"`; CleanupIdleMinutes int `toml:"cleanup_idle_minutes"` }`  
**AC2:** Default config has `Enabled = false` (opt-in)  
**AC3:** TOML parser accepts new keys without breaking existing configs  
**AC4:** README.md updated with zellij configuration example  

### Dependencies
- None (can be implemented in isolation)

---

## 📋 ISSUE BLOCK: #4 - Tab Cleanup Policy Implementation (P2 Could-Have)

### ID: ZELJ-004
### Title: Implement Automatic Tab Cleanup for Stale Tabs

### Context (PRD Section 5.2 FR-05, NFR-02, Critical Failure Point #1 from grill session)
Prevent tab count explosion by auto-closing tabs beyond max count or after idle timeout.

### Requirements (From PRD FR-05)
- [ ] Add config key: `zellij.max_tabs = 10` (default)
- [ ] Add config key: `zellij.cleanup_idle_minutes = 30` (default)
- [ ] Implement cleanup logic that runs on each worktree switch attempt
- [ ] Close oldest tab first when limit exceeded
- [ ] Log cleanup events to `~/.grove/logs/grove.log`

### Acceptance Criteria
**AC1:** Tab count never exceeds `max_tabs` configuration value  
**AC2:** Cleanup runs when user attempts to spawn new worktree tab  
**AC3:** Logs cleanup events with timestamp and tab name  
**AC4:** Status bar shows message: "Tab limit reached, cleaning stale tabs"  

### Dependencies
- Issue #1 (External Layout Config) — need layout file for cleanup
- Issue #2 (Core Feature) — spawn handler must check tab count before spawning

---

## 📋 ISSUE BLOCK: #5 - Tests for Zellij Error Paths (P0 Must-Have)

### ID: ZELJ-005
### Title: Write Unit Tests for Zellij Spawn Failure Scenarios

### Context (PRD Section 6.1 NFR-02, Critical Failure Points from grill session)
Ensure no regressions in agent spawning and handle all error paths gracefully.

### Requirements (From PRD NFR-01, NFR-02, NFR-03)
- [ ] Create test file: `cmd/grove/app_zellij_test.go`
- [ ] Test case: Zellij binary not found → fallback to cd
- [ ] Test case: Layout file missing → use inline default layout
- [ ] Test case: Network timeout on spawn → handle gracefully (offline-first)
- [ ] Test case: Max tabs exceeded → cleanup logic works correctly
- [ ] Verify no memory leaks from accumulated tab processes

### Acceptance Criteria
**AC1:** All existing tests pass after Zellij integration code is added  
**AC2:** New test cases cover all error paths identified in grill session  
**AC3:** No regression in Copilot/Claude/Aider spawning logic  

### Dependencies
- Issue #1 (External Layout Config) — need layout file for tests
- Issue #4 (Tab Cleanup) — need cleanup logic to test

---

## 📋 ISSUE BLOCK: #6 - Documentation Updates (P0 Must-Have)

### ID: ZELJ-006
### Title: Update README and Docs with Zellij Integration Guide

### Context (PRD Section 8 User Documentation Requirements)
Document how users enable, configure, and troubleshoot Zellij integration.

### Requirements (From PRD Section 9)
- [ ] Add "Zellij Tab Integration" section to README.md
- [ ] Include config example: `[zellij] enabled = true`
- [ ] Link to layout file examples in `/home/m00nk0d3/development/grove/layouts/`
- [ ] Explain fallback behavior when Zellij unavailable
- [ ] Update CHANGELOG.md with breaking change note (if removing Copilot/Claude)

### Acceptance Criteria
**AC1:** README.md contains zellij integration documentation  
**AC2:** Config file example updated with zellij section  
**AC3:** Layout file examples included in docs repository  

### Dependencies
- Issue #3 (Config Schema Updates) — need config fields before documenting

---

## 📋 ISSUE BLOCK: #7 - CHANGELOG Entry (P0 Must-Have / Breaking Change)

### ID: ZELJ-007
### Title: Update CHANGELOG.md for Zellij Integration Release

### Context (PRD Section 9.2 Migration Note, Critical Failure Point from grill session)
Document breaking change in multi-agent support and new zellij feature.

### Requirements (From PRD Section 9.2)
- [ ] Add entry to CHANGELOG.md under "## [Unreleased] Breaking Changes"
- [ ] Document removal of Copilot/Claude AI agent support (if applicable)
- [ ] Explain zellij integration as replacement for multi-agent workflow
- [ ] Include migration path for users relying on Copilot/Claude

### Acceptance Criteria
**AC1:** CHANGELOG.md contains unreleased entry  
**AC2:** Breaking change clearly marked  
**AC3:** Migration instructions included  

### Dependencies
- Issue #1 (External Layout Config) — need layout file to document
- Issue #5 (Tests for Error Paths) — need tests before releasing

---

## 📋 ISSUE BLOCK: #8 - Cleanup & Dead Code Removal (P0 Must-Have / Breaking Change)

### ID: ZELJ-008
### Title: Remove Copilot/Claude Support Code and Unused Dependencies

### Context (PRD Out of Scope Section, GitHub Issue #130-134 from earlier work)
After implementing zellij integration, remove deprecated AI agent code.

### Requirements (From PRD Out of Scope section)
- [ ] Delete `internal/domain/agent/copilot.go` and `claude.go`
- [ ] Remove constants from `internal/tui/modal/messages.go`
- [ ] Remove unused imports from related files
- [ ] Run `go mod tidy` to clean up dependencies

### Acceptance Criteria
**AC1:** App builds without Copilot/Claude agent code  
**AC2:** No compile errors from removed files  
**AC3:** `go mod tidy` completes without warnings  

### Dependencies
- Issue #4 (Tab Cleanup Policy) — need cleanup policy before removing old code
- Issue #5 (Tests for Error Paths) — need tests before deleting

---

## 🚀 ISSUE DEPENDENCY GRAPH

```
ZELJ-001 → ZELJ-002 → ZELJ-004, ZELJ-005, ZELJ-006, ZELJ-007, ZELJ-008
   ↓
ZELJ-003 (can be done in parallel)
```

**Critical Path:** ZELJ-001 → ZELJ-002 → (ZELJ-004, ZELJ-005, ZELJ-006, ZELJ-007, ZELJ-008 can happen in parallel)

---

## 📊 BACKLOG SUMMARY

| ID | Title | Priority | Effort | Status |
|----|-------|----------|--------|--------|
| ZELJ-001 | External Layout Config File | P0 (Must-Have) | Low | ✅ Draft |
| ZELJ-002 | Core Feature: Spawn Zellij Tab | P0 (Must-Have) | Medium | 📝 Backlog |
| ZELJ-003 | Config Schema Updates | P0 (Must-Have) | Low | 📝 Backlog |
| ZELJ-004 | Tab Cleanup Policy | P2 (Could-Have) | Medium | 📝 Backlog |
| ZELJ-005 | Tests for Error Paths | P0 (Must-Have) | High | 📝 Backlog |
| ZELJ-006 | Documentation Updates | P0 (Must-Have) | Low | 📝 Backlog |
| ZELJ-007 | CHANGELOG Entry | P0 (Breaking Change) | Low | 📝 Backlog |
| ZELJ-008 | Cleanup & Dead Code Removal | P0 (Breaking Change) | Medium | 📝 Backlog |

---

## 🎯 NEXT STEPS

**Option A: Create GitHub Issues Now**  
I'll fire each issue into GitHub with proper labels (`feature`, `breaking-change`, `chore`), assignees, and milestones.

**Option B: Write Code First, Then Issues**  
Implement issues in order (ZELJ-001 → ZELJ-002 → ZELJ-003) then create issues referencing the PRD.

**Option C: Review & Refine Backlog**  
Roast-check this backlog further — find missing acceptance criteria, edge cases, or test scenarios.

---

## 💬 BRO'S VERDICT

This backlog is solid! All 8 issues from the grill session are accounted for with:
- ✅ Clear titles and context
- ✅ Measurable acceptance criteria
- ✅ Dependencies documented
- ✅ Priority levels assigned

**Ready to fire into GitHub, dude?** Or want me to implement code first? 🔥💪
