# Contributing to Grove

Grove is two programs in one repository: a terminal UI written in Go, and the
Sandcastle runtime, written in TypeScript, that runs agent workflows. Most
changes touch one of them; this guide covers both.

## Setting up

You need Git, the [GitHub CLI](https://cli.github.com/) (`gh auth login`),
Go 1.25+, and Node.js 22+.

```bash
git clone https://github.com/m00nk0d3/grove
cd grove
npm ci --prefix runtime/sandcastle   # runtime dependencies
make build                           # builds ./grove
make test                            # Go and runtime tests
```

To try a change end to end, install the runtime and run Grove from inside a
Git repository, preferably in a Herdr pane so workflows can start:

```bash
make install-runtime   # grove-sandcastle, imp, review, address, ci, resolve, clean
go run ./cmd/grove
```

## Project layout

| Path | Contents |
|---|---|
| `cmd/grove/` | The TUI application: the Bubble Tea model (`app.go`), the screen renderer (`renderer.go`), and how workflow runs, reports, and integrations are wired in |
| `internal/domain/` | Shared types: configuration, worktrees, issues, pull requests, sessions, mission control state |
| `internal/data/` | Loading and saving `config.toml`, the SQLite cache, sessions |
| `internal/exec/` | Wrappers around `git` and `gh` |
| `internal/herdr/` | The Herdr client and detection of a Herdr pane |
| `internal/sandcastle/` | The client for the Sandcastle runtime's JSON interface |
| `internal/mission/` | Building mission control state, correlating runs with worktrees, reading workflow reports |
| `internal/fuzzy/` | The fuzzy finder's index and matching |
| `internal/tui/styles/` | Themes and shared component styles |
| `internal/tui/modal/` | Modals: settings, help, mission inspector, worktree creation and deletion, confirmations |
| `internal/tui/markdown/` | The Markdown renderer used for workflow reports |
| `internal/updater/` | Checking for and applying self-updates |
| `runtime/sandcastle/src/` | The workflow runtime: the `imp` orchestrator, `review`, `address`, `ci`, `resolve`, `clean`, specialists, validation, and the `grove-sandcastle` telemetry command |
| `docs/` | The runbook, the Sandcastle JSON contract, and historical design records |
| `website/` | The project website, deployed to GitHub Pages |

The JSON the runtime writes and Grove reads is specified in
[docs/SANDCASTLE_JSON_CONTRACT.md](../docs/SANDCASTLE_JSON_CONTRACT.md). A
change on either side of that boundary updates the contract.

## Make targets

| Target | Does |
|---|---|
| `make build` | Build `./grove`, stamped with `git describe` |
| `make test` | `go test ./...` and the runtime's `npm test` |
| `make lint` | `golangci-lint run` |
| `make runtime-build` / `make runtime-test` | Build or test only the runtime |
| `make install-runtime` | Build the runtime and install it globally with npm |
| `make install` | `install-runtime`, then `go install` Grove |
| `make snapshot` | A local GoReleaser build without publishing |
| `make demo` | Regenerate the website's live demo (`website/public/demo/frames.json`) from Grove's renderer |

## Tests

Tests are expected with every change: a fix comes with a test that fails
without it, and a feature with tests for its behaviour.

- **Go** tests use [Testify](https://github.com/stretchr/testify) `assert` and
  `require`, and are named `TestSubject_Scenario`, for example
  `TestSettingsModal_RejectsAnInvalidNumberAndKeepsEditing`. Prefer table
  tests for variations of one behaviour.
- **Rendering** tests check the rendered text. Tests that depend on colour
  set the colour profile explicitly, because the default profile in tests
  emits no escape sequences.
- **Runtime** tests run with `npm test` in `runtime/sandcastle`.

Run `make test` before opening a pull request. CI runs `make test` and
`make lint` on every pull request to `main`.

## Branches, commits, and pull requests

- Open an issue before starting anything non-trivial, so the approach can be
  agreed first.
- Name branches `<type>/issue-<number>-<short-description>`, for example
  `feat/issue-42-worktree-root` — the same form Grove uses for the worktrees
  it creates.
- Write commit messages in the
  [Conventional Commits](https://www.conventionalcommits.org/) form, with a
  scope where one fits: `fix(tui): keep the theme background behind every
  styled span`. The body explains why the change was made and what it
  affects, not only what it does.
- Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes, and the
  README, runbook, or contract when behaviour they describe changes.
- Open the pull request against `main`. Its description says what changed,
  why, and how it was tested; the PR check rejects an empty or very short
  description.

## Releases

Pushing a `v<major>.<minor>.<patch>` tag (or a pre-release tag such as
`v1.2.3-rc.1`) runs the release workflow: the tests, a runtime build, and
GoReleaser, which publishes the archives the installers download. The runbook
has the full procedure.
