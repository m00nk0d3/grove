# Grove Runbook

How to install, configure, operate, and recover Grove and its Sandcastle
workflow runtime. For building from source and contributing, see
[.github/CONTRIBUTING.md](../.github/CONTRIBUTING.md); for the full feature and
keybinding reference, see the [README](../README.md).

---

## Components

| Component | What it is | Installed by |
|---|---|---|
| `grove` | The terminal UI (Go) | The platform installers, `go install`, or `make build` |
| Sandcastle runtime | `grove-sandcastle` and the workflow commands `imp` (alias `agent-flow`), `review`, `address`, `ci`, `resolve`, `clean` (TypeScript on a private Node.js 22) | The platform installers, or `make install-runtime` |
| Herdr | The terminal pane manager workflows run in | The platform installers, when `herdr` is not already on `PATH` |
| `gh` | The GitHub CLI Grove and the workflows use for GitHub | You; authenticate with `gh auth login` |
| An agent backend | OpenCode, Pi, or Claude Code, which the workflows drive | You; see the README's Agent Backends |

`go install` provides only `grove`; without the runtime, workflows cannot start.

---

## Installing and upgrading

- **Linux / macOS:** `curl -sSL https://raw.githubusercontent.com/m00nk0d3/grove/main/install.sh | bash`.
  Grove goes to `/usr/local/bin` (`GROVE_INSTALL_DIR`), and Node.js, the
  runtime, and Herdr to `~/.local/share/grove` (`GROVE_DATA_DIR`).
  `GROVE_VERSION` installs a specific release.
- **Windows:** `irm https://raw.githubusercontent.com/m00nk0d3/grove/main/install.ps1 | iex`.
  Everything goes to `%LOCALAPPDATA%\grove\`, which is added to the user `PATH`.

Grove's in-app update replaces only the `grove` binary. To update the runtime,
re-run the installer (or `make install-runtime` for a source install). A
runtime and a Grove from different releases may disagree about the JSON
contract, so upgrade both together.

Check the installed version with `grove --version`.

---

## Configuration

`~/.grove/config.toml`. Every field is optional and a missing file means the
defaults. The settings screen (`t`) edits the same file, writing it when a
setting changes; each setting takes effect immediately.

| Key | Default | Effect |
| --- | --- | --- |
| `appearance.theme` | `digital-noir` | UI theme; one of the 23 listed in the settings screen |
| `github.auto_sync` | `true` | Refresh GitHub data in the background. When `false`, only at startup and on `r` |
| `github.sync_interval_minutes` | `5` | Minutes between background refreshes |
| `worktrees.base_branch` | `main` | Base of a new worktree when no parent branch is picked. When the repository has no such branch, its default branch is used |
| `worktrees.worktree_root` | `../worktrees` | Where worktrees are created, as `<root>/<repository>/<branch>`; a relative path starts at the repository root |
| `sandcastle.enabled` | `true` | Track and start workflows |
| `sandcastle.binary` | `grove-sandcastle` | The runtime's command |
| `sandcastle.default_agent` | `opencode` | Agent backend for workflow specialists: `opencode`, `pi`, or `claude` |
| `herdr.enabled` | `true` | Use Herdr panes and show Herdr status |
| `herdr.prefer_worktree_api` | `true` | Create worktrees through Herdr when Grove runs inside it |
| `herdr.poll_interval_seconds` | `5` | Seconds between checks of Herdr and Sandcastle status |

The Herdr integration is active only when Grove runs inside a Herdr pane, which
Grove detects from the `HERDR_*` environment variables Herdr sets.

Workflows also read `AGENT_FLOW_*` environment variables — model and backend
overrides, the human-input wait, review passes, and validation limits. The
README's Agent Backends section lists them with their defaults.

---

## Where state lives

| Path | Contents | Safe to delete? |
|---|---|---|
| `~/.grove/config.toml` | Configuration | Yes; the defaults apply |
| `~/.grove/grove.db` | SQLite cache of GitHub data and sessions | Yes, with Grove closed; it is rebuilt |
| `~/.grove/logs/grove.log` | Grove's log | Yes |
| `~/.grove/dismissed.json` | Workflows marked done on the dashboard | Yes; they reappear under *Active* |
| `<common-git-dir>/grove-workflows/*.json` | One record per workflow run; the newest 100 are listed | Per run, through Grove (`x`) |
| `<common-git-dir>/agent-flow/` | Workflow checkpoints (`issue-<N>.json`), reports, and the project profile | Checkpoints only when no run of that issue is in progress |

`<common-git-dir>` is the repository's shared git directory (`git rev-parse
--git-common-dir`), the `.git` directory of the main worktree. Every worktree of
the repository shares it, so all of them see the same workflows and reports,
and a run's reports survive its worktree's removal.

---

## Normal operation

### Worktrees

- **Create:** in Issues, select an issue and press `Enter`; in PRs, press
  `Enter` to check the pull request out; in the fuzzy finder, select a branch.
- **Open:** in Worktrees, `Enter` jumps to the worktree's workflow pane or
  opens its shell. *Open separate shell* in Actions opens another one.
- **Delete:** Actions → *Delete worktree*, then confirm. The local branch is
  force-deleted; the remote branch is kept, and the default branch is never
  deleted.
- **Clean up merged work:** Actions → *Clean merged work* runs `clean`.

### GitHub sync

Grove syncs at startup and, with auto sync on, every
`sync_interval_minutes`. `r` refreshes worktrees, GitHub, and Herdr and
Sandcastle status at once. During an outage Grove keeps showing the cached
data and the footer shows the error.

### Workflows

1. Select an issue or pull request, press `a` to focus Actions, choose the
   workflow, and press `Enter`.
2. Grove asks `grove-sandcastle` to start it. The runtime opens (or reuses) the
   repository's Herdr workspace, creates a tab for the run, and starts the
   workflow command there with the chosen agent backend.
3. The run appears on the dashboard. `Enter` jumps to its pane, `v` opens the
   mission inspector, and `[` / `]` switch between *Active / Attention* and
   *Completed*.
4. When an agent needs a person — a permission prompt, a credential, a
   decision — its pane is focused and the workflow waits (15 minutes by
   default, `AGENT_FLOW_HUMAN_INPUT_TIMEOUT_MS`). Answer in the pane.
5. When the run finishes, read its reports in the mission inspector (`v`, then
   `5`), and press `m` to mark a succeeded run done.

Workflows can also be started directly from a Herdr pane, for example
`imp 42`, `review 17`, or `address 17`.

---

## Recovery

### A workflow shows *running* but nothing is happening

The runtime marks a run whose process has exited as *failed* the next time its
status is read. Press `r`. If the process is alive but stuck, jump to its pane
(`Enter`) to see what it is waiting for, or stop it with `x`, which ends the
process and closes its agents' Herdr panes.

### A failed run should be retried

Mission inspector `t`, or Actions → *Retry workflow*. An implementation run
resumes after its last completed stage from its checkpoint in
`<common-git-dir>/agent-flow/issue-<N>.json`. Delete that checkpoint only to
start the issue over from the beginning.

### `address` or `ci` refuses to touch a worktree with uncommitted changes

The pull request's worktree has changes, from your own work or an earlier
failed run. Commit or stash your own work; if the changes are from an earlier
run of the same command, rerun it with `address <pr> --continue` or
`ci <pr> --continue`.

### The header shows *Sandcastle: unavailable*

`grove-sandcastle` was not found. Re-run the platform installer, or run
`make install-runtime`. If the runtime is installed somewhere unusual, set
`sandcastle.binary` to its path.

### The header shows *Herdr: unavailable*, or workflows do not start

Workflows run in Herdr. Start Grove from a Herdr pane, and check that `herdr` is
on `PATH`. The error from a failed start is shown in Grove's status line.

### GitHub data is empty or stale

Check `gh auth status`. For project board status, the token also needs
`read:project`: `gh auth refresh -h github.com -s read:project`. Check that
auto sync is on, then press `r`.

### Worktree operations fail

Check that the branch name is valid, that the worktree is not locked
(`git worktree list` shows locked worktrees), and that the directory under
`worktree_root` is writable. `git worktree prune` removes records of
worktrees whose directories were deleted by hand.

### The config does not load

A warning banner appears on startup. Fix the TOML syntax in
`~/.grove/config.toml`, or move the file aside to start from the defaults.

### Resetting the local cache

Close Grove, delete `~/.grove/grove.db`, and start Grove again.

---

## Logging

Grove logs to `~/.grove/logs/grove.log`: sync failures, git and `gh` command
errors, integration errors, and config problems. Workflow output is in the
run's Herdr pane.

---

## Releases

Releases are built by [GoReleaser](https://goreleaser.com/) when a version tag
is pushed:

1. Merge to `main` with CI green.
2. Tag and push: `git tag v1.2.3 && git push origin v1.2.3`. A tag such as
   `v1.2.3-rc.1` publishes a pre-release.
3. The release workflow runs the tests, builds the runtime, and publishes the
   archives for linux and darwin (amd64 and arm64) and windows (amd64), which
   the installers download.

### Release checklist

- `CHANGELOG.md` describes the release
- README, this runbook, and the JSON contract match the released behaviour
- Keybindings in the README and the in-app help match the code
- New configuration keys are documented with their defaults
