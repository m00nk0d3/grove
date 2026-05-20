# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

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
