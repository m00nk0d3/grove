# Grove — Worktree and Workflow Mission Control

> Manage Git worktrees, track GitHub PRs and issues, launch Sandcastle workflows, and monitor every active session from one terminal interface.

![Latest Release](https://img.shields.io/github/v/release/m00nk0d3/grove?logo=github&label=latest)
![Go version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)

<!-- Screenshot placeholder — run `vhs demo.tape` to regenerate -->
![Grove TUI — 3-pane worktree dashboard](demo/grove.gif)

---

## Why Grove?

Modern software development means juggling multiple things at once: several features in flight, a handful of open PRs waiting for review, GitHub issues to reference, and at least one AI agent that swears it can fix everything. Keeping all of that in sync — without losing your mind or your terminal history — is genuinely painful.

**The problems Grove solves:**

- **Context-switching hell.** Stashing changes, checking out branches, losing your editor state, repeating. Git worktrees are the solution, but their CLI is clunky and easy to forget. Grove puts your entire worktree landscape on one screen and lets you jump between them instantly.

- **The five-app shuffle.** Terminal for git, browser for GitHub, another terminal for the workflow, Slack for the PR link, repeat. Grove collapses worktrees, PRs, issues, workflows, and sessions into a single pane of glass.

- **Workflow context drift.** Starting implementation or review work in the wrong checkout wastes time and produces bad results. Grove delegates execution to Sandcastle with the selected repository, issue, PR, and worktree context.

- **"Which branch had that issue again?"** Grove links GitHub issues and PRs to their worktrees so you always know what's where. No more `git branch -a | grep vague-memory`.

In short: if you work on multiple features simultaneously, Grove removes the glue work so you can focus on the actual code.

---

## Features

- **3-pane TUI** — worktree list, GitHub context panel, and detail view in one terminal window
- **Full worktree management** — create, delete, switch shell, lock/unlock, and prune worktrees without leaving the terminal
- **GitHub sync** — pull requests, issues, reviews, comments, checks, and merge state fetched via the `gh` CLI and kept fresh in the background
- **Team-aware PR attention** — highlights your PRs with requested changes, unresolved review threads, failing checks, or merge conflicts, plus teammate PRs awaiting your review
- **Issue hierarchy & sub-issue branching** — navigate parent/child issue trees and spin up a worktree for any sub-issue in one move, with smart guards so you never branch off a ghost
- **Active sessions dashboard** — mission control for your worktrees: see exactly what's alive, what's idle, and what's absolutely on fire 🔥
- **Sandcastle workflow launcher** — run `imp`, `review`, `ci`, `resolve`, and `clean` from a context-aware action menu
- **Auto-update notifications** — Grove checks for new versions on startup and can update itself; no more `brew upgrade` guilt-trips
- **Global fuzzy finder** — press `/` or `Ctrl+F` to search across worktrees, issues, PRs, files, branches, and agent history simultaneously; results update in real-time as you type
- **Workflow telemetry** — monitor Sandcastle runs and their agents without Grove owning agent process launches
- **9 built-in themes** — Digital Noir, Matrix, Light, Everforest, Tokyo Night, Catppuccin, Kanagawa, Rosé Pine, and One Dark; cycle them live with `t`
- **In-app help** — press `f1` or `?` at any time for a searchable keybindings and troubleshooting reference
- **Local persistence** — config lives in `~/.grove/config.toml`; metadata is cached in SQLite so Grove starts fast

---

## Prerequisites

| Requirement | Notes |
|---|---|
| [Git](https://git-scm.com/) | Must be in `PATH` |
| [GitHub CLI (`gh`)](https://cli.github.com/) | Run `gh auth login` before first use. For project board status, also grant the `read:project` scope: `gh auth refresh -h github.com -s read:project` |
| Go 1.25+ | Only needed if building from source |
| Node.js 22+ | Only needed when building the Sandcastle runtime from source |

---

## Installation

### Linux / macOS

```bash
curl -sSL https://raw.githubusercontent.com/m00nk0d3/grove/main/install.sh | bash
```

Auto-detects your OS and architecture and installs Grove to `/usr/local/bin`.
It also installs a private Node.js runtime and Grove's Sandcastle commands
under `~/.local/share/grove`. If Herdr is not already installed, the matching
official Herdr release is installed there as well and exposed on `PATH`.
Set `GROVE_INSTALL_DIR` or `GROVE_DATA_DIR` to override the Unix command and
private dependency locations.

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/m00nk0d3/grove/main/install.ps1 | iex
```

Downloads the latest `windows_amd64` release, installs Grove, a private Node.js
runtime, and Sandcastle under `%LOCALAPPDATA%\grove\`, and installs Herdr when
it is missing. The directory is added to your user `PATH` automatically.
Restart your terminal after running.

### go install

Works on all platforms — requires Go 1.22+.

```bash
go install github.com/m00nk0d3/grove/cmd/grove@latest
```

`go install` installs only the Grove binary. Use the platform installer above
for the self-contained Grove + Sandcastle + Herdr setup.

### Build from source

```bash
git clone https://github.com/m00nk0d3/grove
cd grove
make build
make install-runtime
./grove
```

Grove ships its own Sandcastle runtime. The platform installers provision it
automatically. For source builds, `make install-runtime` installs
`grove-sandcastle` plus the compatible `imp`, `review`, `resolve`, `ci`, and
`clean` commands. Workflow state is stored in the repository's common Git
directory and appears automatically in Grove's mission-control dashboard.

> Pre-built release archives are also available on the [GitHub Releases page](https://github.com/m00nk0d3/grove/releases) if you prefer to install manually.

---

## Quick Start

1. `cd` into any Git repository
2. Run `grove`
3. Grove opens on the **Dashboard** — use `j`/`k` to select an active workflow and `Enter` to jump to its Herdr pane or terminal
4. Press `i` to switch to the Issues view, select an issue, and press `Enter` to create a new worktree for it
5. In the Worktrees view, press `Enter` to open or focus the selected worktree
6. Focus the visible **Actions** panel to run `imp`, `review`, `ci`, `resolve`, or `clean`

---

## Keybindings

### Navigation

| Key | Action |
|---|---|
| `↑` / `↓` or `j` / `k` | Navigate within panel |
| `←` / `→` or `h` / `l` | Switch between panels / tabs |
| `Tab` | Cycle panel focus / tab |
| `Enter` | Jump to workflow / Open selected item |

### Context Actions

| Key | Action |
|---|---|
| `Enter` (Issues view) | Create worktree from selected issue |
| `Enter` (PRs view) | Checkout PR into a new worktree |
| `Enter` (Worktrees view) | Open / focus shell in worktree |
| `Enter` (Dashboard) | Focus the selected workflow's Herdr pane or terminal |
| `v` (Dashboard) | Inspect workflow progress, steps, metrics, and implementation activity |
| `x` (Dashboard/Inspector) | Confirm stopping/removing the selected workflow |
| `[` / `]` (Dashboard) | Switch between Active / Attention and Completed workflows |
| `a` | Focus the visible context Actions panel |
| Mouse | Click navigation, rows, workflow tabs, and Actions; scroll with the wheel |

### Views

| Key | Action |
|---|---|
| `w` / `W` | Worktrees view |
| `i` / `I` | Issues view (with sub-issue hierarchy!) |
| `p` / `P` | PRs view |
| `t` | Open settings |

### Global

| Key | Action |
|---|---|
| `f1` / `?` | Open help modal |
| `/` / `Ctrl+F` | Open fuzzy finder (search worktrees, issues, PRs, files, branches) |
| `r` | Force-refresh GitHub data (bypasses cache) |
| `PgDown` / `n` | Next page (issues / PRs lists) |
| `PgUp` | Previous page |
| `q` / `Esc` / `Ctrl+C` | Quit |

---

## Configuration

Grove reads its config from `~/.grove/config.toml`. The file is created with defaults on first run. All fields are optional.

```toml
[github]
# Automatically sync PRs and Issues in the background.
auto_sync = true

# How often to refresh GitHub data (in minutes).
sync_interval_minutes = 5

[appearance]
# UI theme. Options: "digital-noir", "matrix", "light", "everforest",
#            "tokyonight", "catppuccin", "kanagawa", "rose-pine", "onedark"
theme = "digital-noir"

[sandcastle]
enabled = true
binary = "grove-sandcastle"
poll_interval_seconds = 5

# Agent backend used for workflow specialists.
# Options: "opencode", "pi", "claude"
default_agent = "opencode"

[worktrees]
# The branch used as the base when creating new worktrees.
base_branch = "main"

# Where new worktree directories are created, relative to the repo root.
worktree_root = "../worktrees"
```

---

## Active Sessions

Active sessions are displayed inline in the **Worktrees** view — no separate view needed. See which agents are running, which shells are alive, and which worktrees are just sitting there pretending to be productive.

- Focus **Actions** to open a separate shell, close a tracked session, delete
  the worktree and its local branch, or run cleanup. Worktree deletion never
  deletes the remote branch and refuses to remove the default branch.

---

## Fuzzy Finder

Press `/` or `Ctrl+F` from anywhere to open the global fuzzy finder overlay. It searches across:

- **Worktrees** — jump directly to any worktree
- **Issues** — navigate to any GitHub issue
- **Pull Requests** — navigate to any PR
- **Files** — open modified/tracked files in `$EDITOR`
- **Branches** — create a worktree from any local branch
- **Agent history** — review past agent runs

Results update in real-time as you type. `Enter` dispatches a smart action per result type. `Esc` closes the overlay without acting.

---

## Sandcastle Workflows

The right-hand **Actions** panel updates with the current selection. Press
**`a`** to focus it, use **`j`/`k`** to select an action, and press **Enter**:

- Issues can start `imp`.
- Pull requests can start `review`, `address`, `ci`, or `resolve`.
- Worktrees and the dashboard can start `clean`.

`address` works the other way round from `review`: instead of reviewing someone
else's pull request, it takes the feedback left on **yours** — unresolved review
threads, "changes requested" review bodies, and pull request comments — and
makes the code changes they ask for. It checks out the pull request branch,
works through each item, validates, then commits and pushes.

A reviewer's **suggested change** is their literal replacement for the lines the
thread sits on, so it is passed through verbatim — never truncated — and applied
as written unless it is wrong. An **approved** pull request is still fair game:
notes left alongside an approval count as feedback, and the run warns you when
pushing might dismiss that approval.

It does not write to the conversation. Replying to reviewers and resolving
threads stays with you, and the agent's final response lists what it did for
each item, including anything it deliberately did not change and why. Threads
marked outdated are shown to the agent as outdated, so it checks the current
code rather than redoing work that has already landed.

Sandcastle owns agent selection and process launching. Grove supplies context,
opens the workflow in Herdr, and displays runtime status.

### Review Specialists

After the implementation is verified, a workflow runs additional review stages —
but only the ones the diff calls for, so an ordinary change is no slower than
before. The review audits only, and hands blockers back to the implementation
specialist rather than fixing them itself.

| Concern | Raised when the diff touches | Reviews |
| --- | --- | --- |
| security | auth, roles, policies, sessions, middleware, CORS, secrets, app settings | Authorization coverage, untrusted input, leaked credentials, transport and browser protections, sensitive data |
| database | migrations, `.sql`, schema definitions, entities, seed routines | Destructive operations, additive-only compliance, backfills, index and cascade impact, rollback cost |
| API contract | controllers, routes, handlers, endpoints, DTOs, API clients | An interface change staying consistent across every layer that declares, produces, or consumes it |

Whichever concerns a diff raises are reviewed together in one `domain-review`
stage, each against its own checklist, so three concerns cost one agent run
rather than three.

The `documentation` stage follows the repository's stated obligations, and
leaves a generated changelog to the release tooling that owns it.

### Stack Detection

Grove identifies **every** stack in the repository, not just one. Each project
is found by its marker file, searching the repository root and up to two levels
below it, and a directory that owns a project is not searched again — so a
solution's individual projects and a workspace's packages do not each count.

| Marker | Stack | Validated with |
| --- | --- | --- |
| `go.mod` | Go | `go test ./...` |
| `pyproject.toml` or `requirements.txt` | Python | `python -m pytest` |
| `*.sln`, `*.slnx`, or `*.csproj` | .NET | `dotnet test <solution>` |
| `package.json` | TypeScript | `npm test` |

Two things follow from this in a repository holding more than one stack:

- **Validation runs per project, where the project lives**, and only for the
  projects the diff actually touches. A backend-only change runs the backend's
  tests; a change spanning both trees runs both. A change that cannot be
  attributed to any project validates all of them rather than guessing.
- **The implementation persona names every stack and the directory that owns
  it**, so the agent follows the conventions of whichever tree it is editing
  instead of being told the repository is one language.

For example, a repository with `backend/App.sln` and `frontend/package.json`
validates a backend change with `dotnet test App.sln` run inside `backend/`,
and a frontend change with `npm test` run inside `frontend/`.

### Agent Backends

Every workflow launches its specialists through the same backend, selected by
`default_agent` in `config.toml` and forwarded to Herdr as the agent kind.

| Backend | Requires | Notes |
| --- | --- | --- |
| `opencode` | OpenCode, LM Studio (default model) | Default. Writes artifacts with shell redirection. |
| `pi` | Pi, LM Studio or Bonsai | Uses the compaction guard extension for context continuity. |
| `claude` | [Claude Code](https://claude.com/claude-code) | Writes artifacts with its native `Write` tool. No local model server needed. |

The backend is passed to the workflow process as `AGENT_FLOW_AGENT_BACKEND`.
Each backend reads its own optional overrides:

| Variable | Backend | Default |
| --- | --- | --- |
| `AGENT_FLOW_OPENCODE_MODEL` | `opencode` | `lmstudio/qwen/qwen3.5-9b` |
| `AGENT_FLOW_OPENCODE_AGENT` | `opencode` | `build` |
| `AGENT_FLOW_PI_PROVIDER` | `pi` | `lm-studio` |
| `AGENT_FLOW_PI_MODEL` | `pi` | `qwen/qwen3.5-9b` |
| `AGENT_FLOW_CLAUDE_MODEL` | `claude` | the Claude Code session default |
| `AGENT_FLOW_CLAUDE_PERMISSION_MODE` | `claude` | `acceptEdits` |
| `AGENT_FLOW_CLAUDE_TOOLS` | `claude` | `Read,Write,Edit,Bash,Glob,Grep` |
| `AGENT_FLOW_HUMAN_INPUT_TIMEOUT_MS` | all | `900000` (15 minutes) |

### When an Agent Needs You

An agent can stop for something only a person can decide — a permission prompt,
a credential, a judgement call. Rather than failing the stage, Grove focuses
that agent's Herdr pane and waits:

```
[Input Needed] af-pull-request-reviewer-1160-49 is waiting for a person.
Answer it in its Herdr pane; the workflow resumes on its own once the agent is
idle again (waiting up to 900s).
```

Answer in the pane and the workflow carries on by itself. Set
`AGENT_FLOW_HUMAN_INPUT_TIMEOUT_MS=0` to fail immediately instead, for
unattended runs where nobody is watching.

---

## Issue Hierarchy & Sub-Issue Branching

Grove understands GitHub's issue hierarchy (parent issues → sub-issues). In the **Issues** view:

- Browse nested issue trees with indentation so you always know who's the parent
- Select a sub-issue and press `Ctrl+N` — Grove creates a worktree branching off the parent's branch automatically
- If the parent issue has no branch yet, Grove will warn you instead of silently creating something broken (we've all been there)

---

## Staying Up to Date

Grove checks for new versions on startup and lets you know when there's something fresher available. No manual `go install` archaeology required. When a new version drops, you'll see a notification and can trigger the self-update from inside the app.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| Empty issues / PRs list or "gh: not logged in" | Run `gh auth login` and follow the prompts |
| Sandcastle workflows are unavailable | Re-run the platform installer; source builds can run `make install-runtime` or configure `sandcastle.binary` |
| No worktrees visible — "git not in PATH" | Ensure Git is installed: `git --version` |
| Deleted worktrees still appear | Run `git worktree prune` in your repo, then press `r` in Grove |
| Warning banner on startup — config not loading | Check `~/.grove/config.toml` for TOML syntax errors; delete to restore defaults |

For more detail, press **`f1`** inside Grove and open the **Troubleshooting** tab.

---

## Contributing

Contributions are welcome! Here's how to get set up and ship something.

### Developer setup

```bash
# 1. Clone the repo
git clone https://github.com/m00nk0d3/grove
cd grove

# 2. Verify Go 1.25+ is installed
go version

# 3. Install the GitHub CLI and authenticate
gh auth login

# 4. Build the project
make build

# 5. Run tests
make test

# 6. Run Grove locally (from inside any Git repo)
go run ./cmd/grove
```

For a full developer reference including release procedures and configuration details, see [docs/RUNBOOK.md](docs/RUNBOOK.md).

### Workflow

1. **Open an issue first** — before starting anything non-trivial, open an issue so we can align on the approach and avoid wasted effort.
2. **Branch naming** — use `feat/issue-<number>-<short-description>` (e.g. `feat/issue-42-worktree-lock-ui`).
3. **Run tests** before pushing:
   ```bash
   make test
   ```
4. **Commit messages** follow [Conventional Commits](https://www.conventionalcommits.org/) and must include a body explaining *why* the change was made (see `.copilot/` for the project commit guidelines).
5. **Open a PR** against `main` with a clear description: what changed, why, and how it was tested.

---

## License

Grove is released under the [MIT License](LICENSE).
