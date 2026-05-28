# Grove: Git Worktree Orchestrator & AI Agent Hub - Development Plan v1.0

## Problem Statement
Developers managing multiple context-heavy development streams need a unified terminal interface to:
1. Manage Git worktrees efficiently (create, delete, switch, list, prune, lock, unlock)
2. Sync with GitHub metadata in real-time (PRs, issues, branch data)
3. Launch AI agents (Copilot, Claude, Aider) with the correct filesystem context
4. Persist configuration and work context across sessions

## Proposed Approach
Build a Go TUI application using Charm.sh ecosystem (bubbletea, lipgloss, bubbles) that serves as a "command center" for orchestrating git worktrees and AI agents. The app will:
- Use MVU architecture for reactive state management
- Wrap `git worktree` CLI commands + GitHub API
- Provide dashboard views for worktrees, GitHub data, and agent spawning
- Store config in `~/.grove/` with SQLite for context persistence
- Suspend itself to spawn external AI tools with proper context

---

## Architecture Overview

### Core Layers
1. **TUI Layer** - Bubbletea components (Models, Views, Updates)
2. **Domain Layer** - Business logic (worktree ops, GitHub sync, agent spawning)
3. **Data Layer** - SQLite persistence + config file management
4. **External Integrations** - Git CLI wrapping, GitHub API client, AI tool launchers

### Directory Structure
```
grove/
├── cmd/
│   └── grove/
│       └── main.go
├── internal/
│   ├── tui/
│   │   ├── models/
│   │   │   ├── app.go (root model)
│   │   │   ├── dashboard.go
│   │   │   ├── worktree_list.go
│   │   │   ├── github_view.go
│   │   │   └── agent_launcher.go
│   │   ├── styles/
│   │   │   └── theme.go (Digital Noir aesthetic)
│   │   └── components/
│   │       ├── table.go (reusable table)
│   │       └── modals.go (create/delete/confirm)
│   ├── domain/
│   │   ├── worktree.go (worktree business logic)
│   │   ├── github.go (GitHub API wrapper)
│   │   ├── agent.go (AI agent launchers)
│   │   └── config.go (config schema + loading)
│   ├── data/
│   │   ├── sqlite.go (DB init, queries)
│   │   ├── models.go (data structs)
│   │   └── migrations/
│   │       └── 001_init_schema.sql
│   └── exec/
│       ├── git.go (git command wrapper)
│       └── github.go (gh CLI wrapper or go-github)
├── go.mod
├── go.sum
└── README.md
```

---

## Feature Breakdown

### 1. Git Worktree Management
**Scope:** Full CRUD operations on git worktrees

#### Operations
- **List** - Display all worktrees with branch, status, path
- **Create** - Interactive modal to create new worktree (branch name, base branch)
- **Switch** - Suspend Grove, cd to worktree, resume
- **Delete** - Remove worktree with confirmation
- **Prune** - Clean up broken worktrees
- **Lock/Unlock** - Lock worktrees to prevent accidental deletion

#### Data to Display
- Worktree path
- Current branch
- Commit SHA (short)
- Clean/dirty status
- Lock status
- Linked PR (if any)

---

### 2. GitHub Integration (Real-Time Sync)
**Scope:** Fetch PRs, issues, branches; sync metadata with worktrees

#### Features
- **Authenticate** - Handle GitHub token (gh CLI or PAT)
- **Fetch Repository Data** - Issues, PRs, branches (paginated)
- **Link PRs to Worktrees** - Match PR branch names to worktree branches
- **Real-Time Updates** - Background goroutine polling GitHub API (configurable interval, e.g., 5min)
- **Filter/Search** - Filter PRs by status, author, labels

#### Data to Sync
- PR: title, branch, author, status (draft/open), labels, reviewers
- Issue: title, number, assignees, labels
- Branch: name, default branch flag, protected flag

#### Real-Time Strategy
- Background goroutine updating GitHub data on interval
- Use channels to signal UI updates
- Debounce frequent updates to prevent flicker

---

### 3. AI Agent Integration (3 Tools)
**Scope:** Spawn GitHub Copilot, Claude, and Aider with worktree context

#### Agent: GitHub Copilot CLI
- Command: `gh copilot suggest [context]`
- Context: Current worktree path, git log, changed files
- When: User selects "Copilot" + provides prompt

#### Agent: Claude Code (CLI wrapper)
- Command: `claude "[prompt]" --context [path]`
- Context: Worktree path, file list, git diff
- When: User selects "Claude" + provides prompt

#### Agent: Aider (Multi-file Editing)
- Command: `aider --read-only [file_list]`
- Context: Pass specific files from worktree
- When: User selects "Aider" + selects files to edit

#### Spawning Mechanism
- Use `tea.ExecProcess` to suspend Grove
- Pass context as env vars or temp files
- Resume Grove when agent finishes
- Option to log agent interactions

---

### 4. Persistent Storage & Configuration
**Scope:** Store config, recent worktrees, context history

#### Config File (`~/.grove/config.toml`)
```toml
[github]
token = "ghp_xxxx"  # or read from gh CLI
auto_sync = true
sync_interval_minutes = 5

[appearance]
theme = "digital_noir"
color_scheme = "high_contrast"

[ai_agents]
copilot_enabled = true
claude_enabled = true
aider_enabled = true
claude_binary = "claude"  # custom path

[worktrees]
root_dir = "/path/to/repo"  # for relative paths
auto_link_prs = true
```

#### SQLite Schema
- **worktrees** - id, path, branch, linked_pr_id, locked, last_used
- **github_prs** - id, number, title, branch, author, status, labels
- **github_issues** - id, number, title, assignees, labels
- **agent_history** - id, agent_name, worktree_id, prompt, output, timestamp
- **context_snapshots** - id, worktree_id, git_state, file_list, timestamp

---

### 5. User Interface (TUI)
**Aesthetic:** Digital Noir / High-Contrast Technical Precision (with Matrix and Light theme variants)

#### Main Dashboard Layout
**Three-column responsive layout:**
1. **Left Panel (5%)** - Navigation rail with mode selectors
   - W: WORKTREES (list view)
   - I: ISSUES (GitHub issues)
   - P: PRS (GitHub pull requests)
   - T: THEMES (appearance settings)

2. **Middle Panel (55%)** - Content area (context-dependent)
   - **Worktree List Mode**: Table with columns: NAME, PATH, STATUS, UPDATED, GH:ID
     - Status colors: Cyan=Checked, Green=Idle, Yellow=Created
     - Inline display of commit SHAs, lock status
   - **Issues Mode**: Table with GitHub issues linked to worktrees
   - **PRs Mode**: Table with GitHub PRs linked to worktrees

3. **Right Panel (40%)** - Context panel (dynamic)
   - When worktree selected: Show linked PR/Issue details
     - Title, author, status (with status dot), labels
     - Quick actions (AI agent spawning)
   - When PR selected: Show PR details + related worktrees
   - When issue selected: Show issue details + related worktrees

#### Key Screens
1. **Worktree Dashboard** - Primary interface (as shown in mockup)
2. **Issue Browser** - Issues mode in middle panel
3. **PR Browser** - PRs mode in middle panel
4. **Theme Selector** - Dark/Light/Matrix theme picker
5. **Help Popup** - Modal with keybindings and usage guide

#### Help Popup Design
**Trigger:** Press `f1` or `?` from anywhere
**Layout:** Centered modal (60-70% width, 80% height) with tabs or sections
**Content:**
- **Keybindings Tab** - All shortcuts organized by category (navigation, worktree ops, AI agents, global)
- **Quick Tips Tab** - Common workflows (create worktree → switch → spawn agent)
- **Troubleshooting Tab** - FAQ, common errors, how to restart sync
- **About Tab** - Version, repo link, credits
**Interaction:**
- Tab navigation with arrow keys or h/l
- Scrollable content with j/k
- Press `esc` or `q` to close
- Visually styled with border matching main app theme

#### Visual Elements
- **Header Bar**: App version, repo info, local path
- **Status Bar (Footer)**: Keybinding hints, sync status, timestamp (UTC)
- **Color Scheme**: 
  - Primary accent: Cyan (#00D9FF)
  - Success: Bright Green
  - Warning: Yellow
  - Error: Red/Magenta
  - Background: Deep black (#000000 or #0a0e27)
  - Text: High contrast white/cyan
- **Borders**: Rounded rectangles with accent colors
- **Status Indicators**: Colored dots (● for status), emoji (⚙️ for agents, 🔒 for locked)

#### Interactive Elements
- **Table Component** - Reusable for worktrees, PRs, issues
- **Modal Dialogs** - Create worktree, confirm delete, input prompt
- **Status Indicators** - Sync status, process running, error states
- **Input Fields** - Branch name, PR filter, agent prompts

#### Keybindings
```
NAVIGATION
j/k or ↑↓    - Navigate within panel
h/l or ←→    - Switch between left/middle/right panels
Enter        - Select/Open

WORKTREE OPERATIONS
[c-n]        - Create new worktree
[c-d]        - Delete selected worktree
[c-l]        - Lock/unlock worktree
[s]          - Open shell in worktree

AI AGENTS (from context panel)
[a]          - Spawn Claude Code
[c]          - Spawn Copilot
[?]          - Show more agent options (Aider, etc.)

MODE SWITCHING (left panel)
W            - Switch to Worktrees view
I            - Switch to Issues view
P            - Switch to PRs view
T            - Switch to Themes view

GLOBAL
[f1] or [?]  - Open help popup (tabs: keybindings, tips, troubleshooting, about)
[t]          - Open theme selector (Digital Noir / Matrix / Light)
[g]          - Open selected item in GitHub
[r]          - Refresh GitHub data
[esc]        - Close popups / Quit from main view
```

---

## Implementation Phases

### Phase 1: Foundation (Core TUI + Git Integration)
**Deliverable:** Basic worktree management in TUI
- [ ] Project setup (go.mod, Charm dependencies)
- [ ] Basic app structure (bubbletea model, update, view)
- [ ] Implement git wrapper (exec + parsing worktree list)
- [ ] Implement worktree list screen
- [ ] Implement create/delete modals
- [ ] Implement switch worktree (suspend + resume)
- [ ] Styling (Digital Noir theme)

### Phase 2: GitHub Integration
**Deliverable:** Real-time GitHub sync + PR linking
- [ ] GitHub client setup (go-github or gh CLI wrapper)
- [ ] Config file parsing (TOML)
- [ ] SQLite setup (schema, migrations, queries)
- [ ] Implement GitHub data fetching (PRs, issues, branches)
- [ ] Background sync goroutine
- [ ] Implement PR linking algorithm (match branches to worktrees)
- [ ] GitHub view screen (list PRs/issues)
- [ ] Display linked PR in worktree list

### Phase 3: AI Agent Integration
**Deliverable:** Spawn Copilot, Claude, Aider with context
- [ ] Implement GitHub Copilot launcher
- [ ] Implement Claude launcher (with custom binary path support)
- [ ] Implement Aider launcher (file selection)
- [ ] Context builder (collect git state, file list)
- [ ] Agent history tracking (SQLite)
- [ ] Agent launcher screen
- [ ] suspend/resume Grove for agent execution

### Phase 4: Polish & Refinement
**Deliverable:** Production-ready v1.0
- [ ] Error handling & logging
- [ ] Help system (in-app keybinding reference)
- [ ] Settings screen (config editor in TUI)
- [ ] Performance optimization (caching, debouncing)
- [ ] README + installation guide
- [ ] Build system (Makefile or Go build script)
- [ ] Testing (unit tests for domain logic)

---

## Technical Considerations

### Dependencies
- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/lipgloss` - Styling
- `github.com/charmbracelet/bubbles` - Components (Table, List, Spinner)
- `github.com/google/go-github/v60` - GitHub API client (or use `gh` CLI)
- `github.com/pelletier/go-toml` - TOML parsing
- `github.com/mattn/go-sqlite3` - SQLite driver
- Standard lib: `os/exec`, `os`, `path/filepath`, `time`, `sync`

### Concurrency Model
- Main TUI goroutine (bubbletea event loop)
- Background sync goroutine (GitHub updates on interval)
- Communication via channels (for non-blocking updates)
- Mutex protection for shared state (worktree list, GitHub data cache)

### Error Handling
- Graceful degradation if GitHub API fails (use cached data)
- Display errors in status bar + error modal
- Log errors to `~/.grove/logs/grove.log`
- Validate user input before executing git commands

### Performance
- Cache GitHub data in SQLite (query before API call)
- Debounce UI updates from background goroutines (100ms)
- Lazy load worktree details (don't fetch all on startup)
- Pagination for PR/issue lists (load first 50, then paginate)

### Security
- Store GitHub token securely (prefer `gh` CLI auth over PAT)
- Never log sensitive data (tokens, passwords)
- Validate git commands to prevent injection
- File permissions on `~/.grove/` (0600)

---

## Edge Cases & Special Handling

1. **Orphaned Worktrees** - Git worktree dir deleted but git metadata remains → offer prune
2. **Stale PR Sync** - PR branch deleted but Grove still shows it → mark as stale, offer cleanup
3. **Network Failures** - GitHub API unreachable → show cached data, retry with backoff
4. **Agent Not Found** - User doesn't have Claude/Aider installed → show helpful error + install hint
5. **Git Repo Not Found** - Grove launched outside git repo → friendly error message
6. **Multiple Repos** - Grove open in one repo, user cd's to another → handle gracefully (warn or reload)

---

## Definition of Done (v1.0)
- [ ] All Phase 1-4 todos complete
- [ ] Manual testing of all workflows
- [ ] README with installation + usage guide
- [ ] Graceful error handling for edge cases
- [ ] Digital Noir aesthetic applied to all screens
- [ ] All keybindings documented
- [ ] Code organized, no dead code
