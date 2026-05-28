# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [v0.6.0] - 2026-05-28

### Added

- **Ctrl+B cleanup modal for stale worktrees and branches** — press `Ctrl+B` from the TUI to surface a modal listing stale/prunable worktrees and their associated branches. Lets you bulk-delete the mess you left behind. `feat(tui)` (#107) (4d79ec2)
- **One-liner install scripts** — `install.sh` (Unix) and `install.ps1` (Windows) so you can get nexus running without touching `go install` or release assets manually. `feat(install)` (b5835f2)
- **GitHub Pages landing site** — nexus now has a real homepage with a live demo GIF, install instructions, and a fancy favicon. `feat(website)` (11ec084, 75c5196)

### Fixed

- **`r` key not forcing a fresh remote fetch** — pressing `r` was still returning cached data if the TTL hadn't expired. It now always hits GitHub and refreshes the cache. `fix(app)` (#105) (919ceea)

### Documentation

- Replace installation section in README with the new one-liner scripts. (f83780f)
- Sync README, RUNBOOK, and in-app help with the v0.5.0 feature set. (7611150)

### Changed

- Redesigned favicon with a git node-graph icon. `style(website)` (203805b)

## [v0.5.0] - 2026-05-22

### Added

- **Global fuzzy finder overlay** — press `/` or `Ctrl+F` from anywhere in the TUI to search across worktrees, issues, PRs, files, branches, agent history, and commits simultaneously. Results update in real-time as you type. `Enter` dispatches a smart action per result type (switch worktree, navigate to issue/PR, open file in `$EDITOR`, create worktree from branch). `feat(fuzzy)` (#99)

### Fixed

- **Worktree list went blank when a stale/prunable worktree path no longer existed on disk** — nexus tried to run `git status` on the missing directory, the error bubbled up silently, and the entire list stayed empty. Prunable worktrees are now handled gracefully. `fix(worktree)` (1995002)

### Changed

- **`r` now always fetches fresh data from GitHub** — previously it would respect the cache TTL and return stale data if the cache was still "fresh". Manual refresh now always bypasses the cache and updates it afterwards. `feat(app)` (f85fb3b)

## [v0.4.3] - 2026-05-21

### Fixed

- **Worktree creation threw a fit on repos that use `master` instead of `main`** — nexus was hardcoding `main` as the base branch like it was 2018. Now it asks the remote what the actual default branch is. `master`, `main`, `trunk`, whatever — it handles it. `fix(worktree)` (f1dc5c3)
- **Auto-updater silently died when nexus was installed in `/usr/bin`** — it tried to rename a root-owned binary, got EACCES, and just... said nothing useful. Now it stages the downloaded binary to `~/.cache/nexus/nexus.staged` and tells you exactly what `sudo` command to run. `fix(updater)` (4874f40)
- **Issues list was mixing up projects** — the GitHub cache had no repo scoping, so running nexus from nova showed nexus issues and vice versa. Cache now keyed by repo path. Your issues stay in your lane. `fix(cache)` (e5cc91e)

## [v0.4.2] - 2026-05-20

### Fixed

- **Worktrees were sharing a namespace like roommates with boundary issues** — paths weren't scoped to the repo name, so multiple repos could stomp each other's worktrees like clumsy giants. Now each repo gets its own lane. No more collisions. `fix(worktree)` (479360f)

## [v0.4.1] - 2026-05-20

### Fixed

- **Linux threw a full tantrum when nexus tried to update itself** — turns out Linux gets *real* possessive about executables that are currently running (ETXTBSY — aka "no bro you literally cannot replace yourself while you exist"). Self-update now writes to a temp file and swaps atomically so Linux stops having an existential crisis mid-update. `fix(updater)` (f2dd7c7)

## [v0.4.0] - 2026-05-20

### Added

- **Your AI bro can now review PRs like a senior dev at 2am** — nexus auto-provisions a worktree, drops into it, and fires up Copilot for an AI-assisted PR review. No more context-switching to a browser like an animal. `feat(tui)` (eda6eb8)
- **Enter actually does something useful on issues & PRs now** — pressing Enter on an issue or PR that already has a session jumps straight to it instead of making you feel lost. Muscle memory: restored. `feat(sessions)` (3063a9f)

### Fixed

- **`r` key was ghosting the refresh function** — it was wired up in some screens but not globally, so half the time you'd press `r` and nothing happened. Classic. Now it works everywhere like it always should have. `fix(app)` (120f039)

## [v0.3.2] - 2026-05-19

### Fixed

- **Error modal had a fixed-width identity crisis** — it now reads the room and resizes itself based on your terminal width (clamped 40–80). No more modal that looks like it escaped from a 1998 dial-up connection. `fix(tui)` (64d884e)
- **Self-update errors were playing dumb** — when a download failed, the error just said "download failed" with zero context. It now rats out the exact URL so you know *which* download decided to ruin your day. `fix(updater)` (0d0821f)
- **Background sync messages were getting ghosted** — the modal's early-return guard was intercepting ALL messages instead of just key presses, silently swallowing sync updates like a black hole. Fixed so non-key messages fall through properly. `fix(app)` (019ff66)

## [v0.3.1] - 2026-05-19

### Fixed
- fix(updater): strip v prefix from filename in DownloadURL (9702ace)

## [v0.3.0] - 2026-05-19

### Added
- feat: create worktree from selected issue (#88) (63ad193)

### Documentation
- docs(readme): fix keybindings to match actual app behavior (d382dad)
- docs(readme): replace version placeholders with dynamic installs (21c9966)
- docs(readme): update for v0.2.0 features (9dfd3cc)

## [v0.2.0] - 2026-05-19

### Added
- feat(updater): version detection, update notifications, and self-update (#87) (f3d6610)
- feat(sessions): active sessions - mission control for worktrees (#80) (909d33b)
- feat(app): add q as quit key binding alongside ESC (#84) (a861fce)
- feat(modal): guard sub-issue worktrees when parent has no branch (699867a)
- feat(issues): issue hierarchy + sub-issue worktree branching (#74) (127ee91)

### Documentation
- docs(readme): split installation instructions by platform (3d656ce)
