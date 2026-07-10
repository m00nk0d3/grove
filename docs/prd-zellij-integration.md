# PRD: Zellij Tab Integration for Grove Worktree Switching

**Status:** Draft  
**Author:** Principal Systems Architect (Post-Grill Session)  
**Date:** 2026-07-10  
**Priority:** P0 (Breaking Change in AI Agent Focus)  

---

## 1. Executive Summary

Grove will optionally spawn new Zellij tabs with a predefined layout when users switch between git worktrees, providing an integrated workspace for code editing (`bash`/`nvim`) and Grove CLI context without manual tab management. This feature is **opt-in** by default to avoid state explosion issues identified during architectural stress-testing.

---

## 2. Problem Statement

### Current Pain Point
When developers use Groove's worktree switching features (e.g., `[s] Open shell in worktree`), they end up in plain terminal directories without any persistent workspace context. This forces manual setup of:
- Zellij tab creation
- Pane layout configuration
- Shell initialization
- Context window setup

### Why This Matters
> "Grove positions itself as an AI-powered Git worktree command center. If switching worktrees doesn't automatically provide a productive workspace, users must manually configure every time — defeating the purpose of a 'command center.'"

---

## 3. Goals & Success Metrics

### Primary Goal
Provide seamless, one-command access to a structured development environment when switching between git worktrees.

### Success Metrics
- **Adoption Rate:** >60% of active users enable zellij integration via config toggle
- **User Satisfaction:** >8/10 NPS score in user surveys ("Did the zellij tab improve your workflow?")
- **Tab Count Stability:** No more than 20 concurrent tabs per user (enforced by cleanup policy)
- **Fallback Success Rate:** 100% of worktree switches succeed even if Zellij is unavailable (graceful degradation)

---

## 4. User Stories

| ID | User Story | Priority |
|----|------------|----------|
| US-01 | As a developer, I want to spawn a new Zellij tab with my preferred layout when switching worktrees so that I can immediately start coding in context. | Must-Have |
| US-02 | As a user who doesn't use Zellij, I don't want Groove to fail if Zellij is not installed, so that I can still switch worktrees normally. | Must-Have |
| US-03 | As an advanced user, I want to customize the layout configuration via `~/.config/grove/layouts/default.kdl` so that I can tailor panes to my workflow. | Should-Have |
| US-04 | As a long-running session user, I want stale Zellij tabs to be automatically cleaned up after X minutes of inactivity so that my terminal doesn't become cluttered. | Could-Have |
| US-05 | As a power user, I want the ability to toggle Zellij integration on/off via config file so that I can use this feature only when I need it. | Must-Have |

---

## 5. Functional Requirements

### 5.1 Core Feature: Worktree Switch with Zellij Tab Spawning

#### FR-01: Spawn New Tab on Worktree Switch
**Description:** When a user switches to a worktree (via `[s]` key or automatic switching), if zellij integration is enabled, spawn a new tab with predefined layout.

**Acceptance Criteria:**
- [ ] Zellij tab spawns with name "Grove: <worktree-path>"
- [ ] Layout matches `~/.config/grove/layouts/default.kdl` (or system default)
- [ ] Left pane contains shell (`bash` by default) or neovim (if configured)
- [ ] Right pane contains Groove CLI in compact mode (`grove -c`)
- [ ] Tab focuses on right pane (Grove context) by default

#### FR-02: Graceful Fallback When Zellij Unavailable
**Description:** If Zellij is not running or zellij command fails, fall back to plain `cd` without erroring the worktree switch.

**Acceptance Criteria:**
- [ ] No error logged if zellij spawn fails silently
- [ ] User still receives confirmation that worktree switched successfully
- [ ] Status bar shows "Zellij unavailable" message (optional)

#### FR-03: External Layout Configuration File
**Description:** Layout configuration stored externally at `~/.config/grove/layouts/default.kdl` rather than hardcoded in Go code.

**Acceptance Criteria:**
- [ ] Layout file loaded on Groove startup
- [ ] Users can edit layout without rebuilding or restarting Groove
- [ ] Supports multiple layout definitions (e.g., "nvim-only", "bash-fullscreen")
- [ ] Invalid layout files fail gracefully with helpful error message

### 5.2 Lifecycle Management

#### FR-04: Opt-In Configuration Toggle
**Description:** Zellij integration is disabled by default; users must explicitly enable it in config file.

**Acceptance Criteria:**
- [ ] Config key `zellij.enabled = false` in `~/.grove/config.toml`
- [ ] Documentation clearly states this is opt-in behavior
- [ ] No automatic tab spawning without explicit config change

#### FR-05: Tab Cleanup Policy
**Description:** Automatically close tabs that exceed maximum count or have been inactive for X minutes.

**Acceptance Criteria:**
- [ ] Config key `zellij.max_tabs = 10` (default)
- [ ] Config key `zellij.cleanup_idle_minutes = 30` (default)
- [ ] Cleanup runs on worktree switch attempt
- [ ] Logs cleanup events to `~/.grove/logs/grove.log`

#### FR-06: Persistent Layout File Location
**Description:** Use `~/.config/grove/layouts/default.kdl` instead of `/tmp` for layout files.

**Acceptance Criteria:**
- [ ] Layout file created on first use if missing
- [ ] File permissions set to 0644 (readable by user only)
- [ ] No network I/O required (file-based, not remote fetch)

---

## 6. Non-Functional Requirements

### 6.1 Reliability

- **NFR-01:** All worktree switches succeed even if Zellij integration fails (no data loss, no state corruption)
- **NFR-02:** No memory leaks from accumulating tabs (tested with 50+ tab creation/destruction cycles)
- **NFR-03:** Graceful handling of SSH session disconnects (zombie processes cleaned up on next switch)

### 6.2 Performance

- **NFR-04:** Tab spawn latency <1 second (network timeouts handled gracefully)
- **NFR-05:** Layout file parsing <50ms startup time
- **NFR-06:** No more than 10 concurrent zellij processes per user (enforced by cleanup policy)

### 6.3 Security

- **NFR-07:** Layout files stored with 0644 permissions (not world-writable)
- **NFR-08:** No remote code execution via layout file (only read local file system)
- **NFR-09:** Shell commands executed in user's context (no privilege escalation)

### 6.4 Maintainability

- **NFR-10:** All layout changes require no Groove rebuild (external config)
- **NFR-11:** Clear error messages when Zellij command fails (helpful troubleshooting guide)
- **NFR-12:** No hard-to-debug race conditions between worktree switches

---

## 7. Technical Constraints & Requirements

### 7.1 Architecture Decision: CLI Wrapper (Not Plugin)

**Rationale:** Plugin integration requires socket-based IPC and shared state management, which introduces significant complexity for minimal UX benefit. CLI wrapper approach aligns with Grove's existing agent spawning patterns (Copilot, Claude, Aider).

### 7.2 Layout File Schema

Layout files must conform to Zellij KDL specification:
```kdl
layout {
    tab name="Grove (%s)" focus=true {
        pane split_direction="vertical" {
            pane command="bash" name="[Workspace]" size="70%"
            pane command="grove -c" name="[Grove Context]" size="30%"
        }
    }
}
```

### 7.3 Zellij Environment Variables

- `ZELLIJ_WORKTREE_ID`: Set to worktree basename (e.g., "PROJ-123-feature")
- Used for tab naming and potential future plugin integration

### 7.4 Error Handling Strategy

| Error Scenario | Behavior | User Feedback |
|----------------|----------|---------------|
| Zellij not installed | Fallback to `cd` only | Status bar: "Zellij unavailable (fallback)" |
| Layout file missing | Create default layout in `~/.config/grove/layouts/` | Log warning only |
| Network timeout spawning temp files | Use local file cache, no network I/O | No impact (offline-first design) |
| Tab spawn exceeds max count | Close oldest tab first | Status bar: "Tab limit reached, cleaning stale tabs" |

---

## 8. Out of Scope

The following are **explicitly not included** in this iteration to avoid scope creep:

- [ ] Plugin-based IPC integration with Zellij (requires socket protocol implementation)
- [ ] Persistent tab state management across Groove restarts (user must manually reconfigure)
- [ ] Custom pane command configuration via UI (only file-based config supported)
- [ ] Dynamic pane resizing based on terminal size
- [ ] Cross-platform layout file sharing (each user maintains own config)
- [ ] Integration with other terminal multiplexers (tmux, screen — Zellij-only for now)

---

## 9. User Documentation Requirements

### 9.1 README Update
Add to README.md under "Advanced Configuration":
```markdown
## Zellij Tab Integration (Opt-In)

Grove can automatically spawn new Zellij tabs when switching worktrees:

1. **Enable:** Add to `~/.grove/config.toml`:
   ```toml
   [zellij]
   enabled = true              # Default: false (opt-in)
   max_tabs = 10              # Close tabs beyond this count
   cleanup_idle_minutes = 30  # Auto-close inactive tabs
   layout_file = "~/.config/grove/layouts/default.kdl"
   ```

2. **Customize Layout:** Edit `~/.config/grove/layouts/default.kdl` to match your workflow. See example layouts in `/home/m00nk0d3/development/grove/layouts/`.

3. **Fallback Behavior:** If Zellij is unavailable, Grove falls back to plain `cd` without erroring the worktree switch.
```

### 9.2 Migration Note (Breaking Change Warning)
If users have existing agent history for Copilot/Claude, add migration note:
> "Note: This feature replaces multi-agent support with streamlined Aider-only focus."

---

## 10. Open Questions

| Question | Impact if Unresolved | Owner |
|----------|----------------------|-------|
| Should layout file be embedded in repo or external config? | Code size vs. flexibility tradeoff | Product Lead |
| What's the ideal max_tabs value (5, 10, 20)? | State explosion risk | UX Designer |
| Should cleanup policy run on startup or lazily? | Boot time vs. responsiveness | Backend Engineer |

---

## 11. Implementation Priority Matrix

| Feature | Effort | Value | Priority |
|---------|--------|-------|----------|
| External layout config file | Low | High | P0 (Must-Have) |
| Graceful fallback to cd | Low | High | P0 (Must-Have) |
| Opt-in config toggle | Low | Medium | P1 (Should-Have) |
| Tab cleanup policy | Medium | Medium | P2 (Could-Have) |

---

## 12. Critical Failure Points (From Grill Session)

**DO NOT IGNORE:** These failure modes identified during architectural stress-testing **must** be addressed:

1. ✅ **Tab count explosion** → Solved by `max_tabs` config + cleanup policy
2. ✅ **Zellij not running** → Solved by graceful fallback to `cd`
3. ✅ **Layout hardcoded in Go** → Solved by external `.kdl` config file
4. ❓ **No tab ownership/parentage** → Out of scope (simpler than plugin approach)
5. ❓ **Zombie processes on SSH disconnect** → Addressed by cleanup policy

---

## 13. Acceptance Criteria Summary (Definition of Done)

A Pull Request implementing this feature is ready for review when ALL are met:

- [ ] External layout config file exists at `layouts/default.kdl`
- [ ] Zellij spawn handler added to `cmd/grove/app.go`
- [ ] Graceful fallback implemented (no crash if Zellij unavailable)
- [ ] Config schema updated with zellij settings in `internal/domain/config.go`
- [ ] README.md updated with feature documentation
- [ ] All existing tests pass (no regression in agent spawning)
- [ ] New tests added for Zellij spawn error paths
- [ ] CHANGELOG.md entry drafted (if not covered by breaking change issue)

---

## 14. References & Dependencies

- **Grove GitHub Issues:** #130, #131, #132, #133
- **Zellij KDL Specification:** https://github.com/zellij-org/zellij/blob/main/docs/kdl.md
- **Previous Grill Session:** `/home/m00nk0d3/development/grove/grill-zellij-integration.md` (draft)

---

## 15. Appendix: Example Layout Files

### A. Bash + Groove Default (`default.kdl`)
```kdl
layout {
    default_tab_template {{
        pane size=1 borderless=true {{
            plugin location="zellij:tab-bar"
        }}
        children
        pane size=1 borderless=true {{
            plugin location="zellij:status-bar"
        }}
    }}
    
    tab name="Grove (%s)" focus=true {{
        pane split_direction="vertical" {{
            pane command="bash" name="[Workspace]" size="70%"
            pane command="grove -c" name="[Grove Context]" size="30%"
        }}
    }}
}
```

### B. Neovim Fullscreen (`nvim.kdl`)
```kdl
layout {
    tab name="Grove-Nvim (%s)" focus=true {{
        pane split_direction="vertical" {{
            pane command="nvim" name="[Neovim Workspace]" size="90%"
            pane command="grove -c" name="[Grove Context]" size="10%"
        }}
    }}
}
```

### C. Horizontal Split (`horizontal.kdl`)
```kdl
layout {
    tab name="Grove-Horizontal (%s)" focus=true {{
        pane split_direction="horizontal" {{
            pane command="bash" name="[Workspace]" size="50%"
            pane command="grove -c" name="[Grove Context]" size="50%"
        }}
    }}
}
```

---

**END OF PRD**

---

## 🚀 Next Steps

1. **Review this PRD** — Does it capture all requirements from the grill session?
2. **Add missing sections** — What critical features or constraints did I miss?
3. **Trigger `/to-issues`** — Break this PRD down into a GitHub issues backlog (Option A)
4. **Write implementation code** — Create the actual feature branch with changes (Option B)

What's your call, dude? 🔥💪