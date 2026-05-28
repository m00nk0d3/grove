# Grove Runbook

This runbook covers day-to-day operation of Grove and the expected procedures around setup, configuration, troubleshooting, and maintenance.

---

## Developer Setup

This section is for contributors building and running Grove from source.

### Prerequisites

| Tool | Version | Notes |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.25+ | `go version` to verify |
| [GitHub CLI (`gh`)](https://cli.github.com/) | Latest | `gh auth login` before running |
| Git | Any recent | Must be in `PATH` |

### Clone the repository

```bash
git clone https://github.com/m00nk0d3/grove
cd grove
```

### Build

```bash
go build ./...
```

### Run (without installing)

```bash
go run ./cmd/grove
```

Run from inside a Git repository, otherwise Grove will warn that no worktrees are available.

### Build a versioned binary

```bash
go build \
    -ldflags "-X github.com/m00nk0d3/grove/internal/version.Version=v0.1.0" \
    -o grove \
    ./cmd/grove
```

Move the resulting `grove` binary to a directory on your `PATH`.

### Run tests

```bash
go test ./...
```

### Release process

Releases are automated via [GoReleaser](https://goreleaser.com/) and triggered by pushing a version tag.

1. Merge all changes to `main` and ensure tests pass.
2. Create and push a version tag:
   ```bash
   git tag v0.x.0 && git push origin v0.x.0
   ```
3. GoReleaser picks up the tag, builds cross-platform binaries (linux/darwin/windows, amd64/arm64), and publishes the GitHub Release automatically.
4. Pre-releases (alpha/beta/RC) are tagged as `v0.x.0-beta.1` etc. — GoReleaser marks them as pre-releases automatically.
5. Update `README.md`, `CHANGELOG.md`, and this runbook if any user-facing behaviour changed.

---

## Purpose

Grove is a terminal UI for managing Git worktrees, syncing GitHub data, and launching AI coding agents with the right repository context.

## Current status

Grove is fully implemented and shipping. The latest release is available on the [GitHub Releases page](https://github.com/m00nk0d3/grove/releases). See `CHANGELOG.md` for the full version history.

## Owners

- Primary owner: project maintainer
- Secondary owner: anyone responsible for GitHub auth, local config, or release packaging

## Prerequisites

- Git
- GitHub CLI (`gh`) authenticated to the target account
- Go toolchain
- A git repository with worktrees enabled
- Optional: Claude Code and Aider binaries if those launchers are enabled

## Setup

1. Clone the repository.
2. Ensure `gh auth status` succeeds.
3. Create the Grove config directory: `~/.grove/`
4. Add `~/.grove/config.toml`
5. Start Grove from the root of a git repository

## Configuration

Primary config file:

`~/.grove/config.toml`

Key settings:

| Setting | Purpose |
| --- | --- |
| `github.auto_sync` | Enables background sync |
| `github.sync_interval_minutes` | Refresh cadence |
| `appearance.theme` | UI theme (`digital-noir`, `matrix`, `light`, `everforest`, `tokyonight`, `catppuccin`, `kanagawa`, `rose-pine`, `onedark`) |
| `ai_agents.copilot_enabled` | Toggles Copilot launcher |
| `ai_agents.claude_enabled` | Toggles Claude launcher |
| `ai_agents.aider_enabled` | Toggles Aider launcher |
| `ai_agents.claude_binary` | Override path to the `claude` binary |
| `ai_agents.aider_binary` | Override path to the `aider` binary |
| `worktrees.base_branch` | Default branch used when creating worktrees |
| `worktrees.worktree_root` | Directory where new worktrees are created (relative to repo root) |

## Startup procedure

1. Confirm the repo is a valid git repository.
2. Confirm the current worktree list is readable.
3. Load local config and cached data.
4. Start the UI.
5. Start background GitHub sync if enabled.

## Normal operations

### Fuzzy finder

- Press `/` or `Ctrl+F` to open the overlay from any view.
- Type to filter across worktrees, issues, PRs, files, branches, and agent history.
- `Enter` dispatches the contextual action; `Esc` dismisses without acting.

### Worktree management

- Create a worktree from the dashboard.
- Switch to a worktree when you need repository context there.
- Lock worktrees before long-lived work.
- Prune stale worktrees after branch deletion or directory removal.

### GitHub sync

- Refresh manually with `r` when current data looks stale.
- Use auto-sync for steady-state updates.
- Prefer cached data during temporary API failures.

### Agent launchers

- Use Copilot for quick guidance and suggestions.
- Use Claude for broader reasoning and multi-step changes.
- Use Aider when you want file-scoped editing assistance.

## Troubleshooting

### Grove will not start

Check:

- you are inside a git repository
- `gh auth status` succeeds
- config file exists and is readable

### GitHub data is stale

Check:

- background sync is enabled
- network access is available
- the auth token still works

### Agent launcher fails

Check:

- the binary exists in `PATH`
- the launcher is enabled in config
- the worktree path is valid

### Worktree operations fail

Check:

- the repository is clean enough for the intended operation
- the branch name is valid
- the target worktree is not locked

## Incident response

### GitHub API outage

1. Fall back to cached metadata.
2. Stop relying on live refreshes.
3. Retry after connectivity recovers.

### Corrupted local config

1. Back up `~/.grove/`.
2. Remove or repair the broken TOML file.
3. Restart with a minimal config.

### Missing agent binary

1. Disable the launcher in config.
2. Install the missing tool.
3. Re-enable the launcher after verification.

## Backup and restore

Back up:

- `~/.grove/config.toml`
- SQLite cache or local database files
- logs under `~/.grove/logs/`

Restore by copying the files back into the same paths and restarting Grove.

## Logging

Operational logs should live under:

`~/.grove/logs/grove.log`

Use logs for:

- sync failures
- launch failures
- git command errors
- config validation errors

## Maintenance tasks

- Review config defaults when adding new features
- Update dependency notes after architecture changes
- Keep keybindings in sync with the UI
- Document new error states and recovery steps

## Release checklist

- README updated
- runbook updated
- keybindings documented
- config defaults documented
- edge cases documented
- license present

## Open questions

- Should local persistence use a single SQLite file or separate caches?
- Should Grove prefer `gh` auth only, or allow PAT fallback by default?
- Should the app resume to the original shell context after agent launch or keep a visible handoff screen?
