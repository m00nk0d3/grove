# NEXUS — Git Worktree Orchestrator & AI Agent Hub

> Manage Git worktrees, track GitHub PRs and Issues, launch AI coding agents, and keep tabs on every active session — all from a single terminal interface. No browser. No tab soup. Just vibes.

![Latest Release](https://img.shields.io/github/v/release/m00nk0d3/nexus?logo=github&label=latest)
![Go version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-green)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)

<!-- Screenshot placeholder — run `vhs demo.tape` to regenerate -->
![Nexus TUI — 3-pane worktree dashboard](demo/nexus.gif)

---

## Why Nexus?

Modern software development means juggling multiple things at once: several features in flight, a handful of open PRs waiting for review, GitHub issues to reference, and at least one AI agent that swears it can fix everything. Keeping all of that in sync — without losing your mind or your terminal history — is genuinely painful.

**The problems Nexus solves:**

- **Context-switching hell.** Stashing changes, checking out branches, losing your editor state, repeating. Git worktrees are the solution, but their CLI is clunky and easy to forget. Nexus puts your entire worktree landscape on one screen and lets you jump between them instantly.

- **The five-app shuffle.** Terminal for git, browser for GitHub, another terminal for the agent, Slack for the PR link, repeat. Nexus collapses all of that into a single pane of glass: worktrees, PRs, issues, and agent launchers — no browser tab required.

- **AI agents without context.** Spinning up Claude Code or Copilot in the wrong directory (or forgetting which worktree maps to which feature) wastes time and produces wrong answers. Nexus launches agents *inside the correct worktree* automatically, so your AI always has the right repo context.

- **"Which branch had that issue again?"** Nexus links GitHub issues and PRs to their worktrees so you always know what's where. No more `git branch -a | grep vague-memory`.

In short: if you work on multiple features simultaneously and use AI coding tools, Nexus removes the glue work so you can focus on the actual code.

---

## Features

- **3-pane TUI** — worktree list, GitHub context panel, and detail view in one terminal window
- **Full worktree management** — create, delete, switch shell, lock/unlock, and prune worktrees without leaving the terminal
- **GitHub sync** — pull requests and issues fetched via the `gh` CLI and kept fresh in the background
- **Issue hierarchy & sub-issue branching** — navigate parent/child issue trees and spin up a worktree for any sub-issue in one move, with smart guards so you never branch off a ghost
- **Active sessions dashboard** — mission control for your worktrees: see exactly what's alive, what's idle, and what's absolutely on fire 🔥
- **AI agent launchers** — spawn Claude Code, GitHub Copilot, or Aider in the correct worktree directory with a single keypress
- **Auto-update notifications** — Nexus checks for new versions on startup and can update itself; no more `brew upgrade` guilt-trips
- **Global fuzzy finder** — press `/` or `Ctrl+F` to search across worktrees, issues, PRs, files, branches, and agent history simultaneously; results update in real-time as you type
- **AI-assisted PR review** — press `Ctrl+R` on any PR to auto-provision a review worktree and pre-seed your AI agent with a structured code-review prompt
- **9 built-in themes** — Digital Noir, Matrix, Light, Everforest, Tokyo Night, Catppuccin, Kanagawa, Rosé Pine, and One Dark; cycle them live with `t`
- **In-app help** — press `f1` or `?` at any time for a searchable keybindings and troubleshooting reference
- **Local persistence** — config lives in `~/.nexus/config.toml`; metadata is cached in SQLite so Nexus starts fast

---

## Prerequisites

| Requirement | Notes |
|---|---|
| [Git](https://git-scm.com/) | Must be in `PATH` |
| [GitHub CLI (`gh`)](https://cli.github.com/) | Run `gh auth login` before first use |
| Go 1.25+ | Only needed if building from source |
| Claude Code | Optional — enable with `claude_enabled = true` |
| GitHub Copilot CLI | Optional — `gh extension install github/gh-copilot` |
| Aider | Optional — `pip install aider-chat` |

---

## Installation

### Download a pre-built binary (recommended)

Grab the latest release from the [GitHub Releases page](https://github.com/m00nk0d3/nexus/releases).

Assets follow the naming pattern `nexus_{version}_{os}_{arch}[.tar.gz|.zip]`:

| Platform | Archive |
|---|---|
| Linux (amd64) | `nexus_{version}_linux_amd64.tar.gz` |
| Linux (arm64) | `nexus_{version}_linux_arm64.tar.gz` |
| macOS (Intel) | `nexus_{version}_darwin_amd64.tar.gz` |
| macOS (Apple Silicon) | `nexus_{version}_darwin_arm64.tar.gz` |
| Windows (amd64) | `nexus_{version}_windows_amd64.zip` |

> **Pre-releases** (alpha / beta / RC) are also published on the Releases page and clearly marked. Use them to try upcoming features early.

#### Linux / macOS

The snippet below fetches the latest version automatically — no manual version hunting required:

```bash
VERSION=$(curl -s https://api.github.com/repos/m00nk0d3/nexus/releases/latest \
  | grep '"tag_name"' | cut -d'"' -f4 | tr -d 'v')
curl -L "https://github.com/m00nk0d3/nexus/releases/latest/download/nexus_${VERSION}_linux_amd64.tar.gz" \
  | tar -xz
sudo mv nexus /usr/local/bin/
```

> Replace `linux_amd64` with `darwin_amd64` or `darwin_arm64` for macOS.

#### Windows (PowerShell)

```powershell
$version = (Invoke-RestMethod https://api.github.com/repos/m00nk0d3/nexus/releases/latest).tag_name.TrimStart('v')
Invoke-WebRequest "https://github.com/m00nk0d3/nexus/releases/latest/download/nexus_${version}_windows_amd64.zip" `
  -OutFile nexus.zip
Expand-Archive nexus.zip -DestinationPath "$env:USERPROFILE\nexus"
# Add to PATH for the current user (run once):
[System.Environment]::SetEnvironmentVariable(
  "PATH",
  "$env:USERPROFILE\nexus;$([System.Environment]::GetEnvironmentVariable('PATH','User'))",
  "User"
)
```

Restart your terminal after updating the PATH.

### Using go install

Works on all platforms — requires Go 1.25+.

```bash
go install github.com/m00nk0d3/nexus/cmd/nexus@latest
```

The binary is placed in `$GOPATH/bin` (Linux/macOS) or `%GOPATH%\bin` (Windows), which should already be on your `PATH` if Go is set up correctly.

### Build from source

#### Linux / macOS

```bash
git clone https://github.com/m00nk0d3/nexus
cd nexus
make build
sudo mv nexus /usr/local/bin/
```

#### Windows (PowerShell)

```powershell
git clone https://github.com/m00nk0d3/nexus
cd nexus
go build -o nexus.exe ./cmd/nexus
# Move nexus.exe to a folder that's on your PATH, e.g.:
Move-Item nexus.exe "$env:USERPROFILE\bin\nexus.exe"
```

> `make` targets are available via Git Bash or WSL on Windows.

---

## Quick Start

1. `cd` into any Git repository
2. Run `nexus`
3. Nexus opens in the **Worktrees** view — use `j`/`k` to navigate the list
4. Press `i` to switch to the Issues view, select an issue, and press `Enter` to create a new worktree for it
5. Press `Enter` or `s` to open a shell inside the selected worktree
6. Press `a`, `c`, `f`, or `Space` to launch an AI agent in that worktree's context

---

## Keybindings

### Navigation

| Key | Action |
|---|---|
| `↑` / `↓` or `j` / `k` | Navigate within panel |
| `←` / `→` or `h` / `l` | Switch between panels / tabs |
| `Tab` | Cycle panel focus / tab |
| `Enter` | Open shell in worktree / Select |

### Worktree Operations

| Key | Action |
|---|---|
| `Enter` (Issues view) | Create worktree from selected issue |
| `Enter` (PRs view) | Checkout PR into a new worktree |
| `Enter` (Worktrees view) | Open / focus shell in worktree |
| `Ctrl+D` | Delete selected worktree |
| `s` | Open shell in worktree |
| `x` | Kill session and close terminal tab |

### AI Agents

| Key | Action |
|---|---|
| `a` | Spawn Claude Code (with optional inline prompt) |
| `c` | Spawn GitHub Copilot (with optional inline prompt) |
| `f` | Spawn Aider (opens file picker first) |
| `Space` | Unified agent launcher (shows all agents and availability) |
| `Ctrl+R` | AI-assisted PR review (PRs view only — auto-provisions worktree + pre-seeded prompt) |

### Views

| Key | Action |
|---|---|
| `w` / `W` | Worktrees view |
| `i` / `I` | Issues view (with sub-issue hierarchy!) |
| `p` / `P` | PRs view |
| `t` | Open settings (theme, sync config, AI agents) |

### Global

| Key | Action |
|---|---|
| `f1` / `?` | Open help modal |
| `/` / `Ctrl+F` | Open fuzzy finder (search worktrees, issues, PRs, files, branches) |
| `g` | Open selected item in GitHub |
| `r` | Force-refresh GitHub data (bypasses cache) |
| `PgDown` / `n` | Next page (issues / PRs lists) |
| `PgUp` | Previous page |
| `q` / `Esc` / `Ctrl+C` | Quit |

---

## Configuration

Nexus reads its config from `~/.nexus/config.toml`. The file is created with defaults on first run. All fields are optional.

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

[ai_agents]
# Enable or disable individual agent launchers.
copilot_enabled = true
claude_enabled  = true
aider_enabled   = false

# Override the binary name/path if it differs from the default.
claude_binary = "claude"
aider_binary  = "aider"

[worktrees]
# The branch used as the base when creating new worktrees.
base_branch = "main"

# Where new worktree directories are created, relative to the repo root.
worktree_root = "../worktrees"
```

---

## Active Sessions

Active sessions are displayed inline in the **Worktrees** view — no separate view needed. See which agents are running, which shells are alive, and which worktrees are just sitting there pretending to be productive.

- **`s`** — open a new shell session in the selected worktree
- **`x`** — kill the active session for the selected worktree

No more `ps aux | grep claude` crimes. It's all in one place.

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

## AI-Assisted PR Review

In the **PRs** view, press `Ctrl+R` on any pull request to trigger an AI-assisted review:

1. Nexus auto-provisions a dedicated review worktree for the PR branch
2. Opens the unified agent launcher pre-seeded with a structured code-review prompt covering correctness, security, performance, maintainability, and breaking changes
3. Your AI agent starts reviewing with full repository context — no copy-pasting, no browser tab required

---

## Issue Hierarchy & Sub-Issue Branching

Nexus understands GitHub's issue hierarchy (parent issues → sub-issues). In the **Issues** view:

- Browse nested issue trees with indentation so you always know who's the parent
- Select a sub-issue and press `Ctrl+N` — Nexus creates a worktree branching off the parent's branch automatically
- If the parent issue has no branch yet, Nexus will warn you instead of silently creating something broken (we've all been there)

---

## Staying Up to Date

Nexus checks for new versions on startup and lets you know when there's something fresher available. No manual `go install` archaeology required. When a new version drops, you'll see a notification and can trigger the self-update from inside the app.

---

## AI Agent Setup

### GitHub Copilot

```bash
# Install the Copilot CLI extension
gh extension install github/gh-copilot

# Authenticate (if not already done)
gh auth login
```

Enable in config (default: `true`). Trigger in Nexus: **`c`**.

### Claude Code

Install via the official guide: <https://docs.anthropic.com/en/docs/claude-code>

The binary must be named `claude` (or set `claude_binary` to its full path). Enable in config (default: `true`). Trigger in Nexus: **`a`**.

### Aider

```bash
pip install aider-chat
```

Set `aider_enabled = true` in `~/.nexus/config.toml`. Trigger in Nexus: **`f`** for direct launch, or **`Space`** → select Aider in the unified launcher.

---

## Troubleshooting

| Symptom | Fix |
|---|---|
| Empty issues / PRs list or "gh: not logged in" | Run `gh auth login` and follow the prompts |
| "binary not found" when spawning an agent | Install the missing tool or set `*_enabled = false` in config |
| No worktrees visible — "git not in PATH" | Ensure Git is installed: `git --version` |
| Deleted worktrees still appear | Run `git worktree prune` in your repo, then press `r` in Nexus |
| Warning banner on startup — config not loading | Check `~/.nexus/config.toml` for TOML syntax errors; delete to restore defaults |

For more detail, press **`f1`** inside Nexus and open the **Troubleshooting** tab.

---

## Contributing

Contributions are welcome! Here's how to get set up and ship something.

### Developer setup

```bash
# 1. Clone the repo
git clone https://github.com/m00nk0d3/nexus
cd nexus

# 2. Verify Go 1.25+ is installed
go version

# 3. Install the GitHub CLI and authenticate
gh auth login

# 4. Build the project
make build

# 5. Run tests
make test

# 6. Run Nexus locally (from inside any Git repo)
go run ./cmd/nexus
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

Nexus is released under the [MIT License](LICENSE).
