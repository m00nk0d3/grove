# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Claude Code agent backend** — `default_agent = "claude"` launches workflow
  specialists through Claude Code instead of OpenCode or Pi, using Herdr's
  existing `claude` agent kind. Artifact instructions use the native `Write`
  tool rather than shell redirection, and no local model server is required.
  Configurable via `AGENT_FLOW_CLAUDE_MODEL`,
  `AGENT_FLOW_CLAUDE_PERMISSION_MODE`, and `AGENT_FLOW_CLAUDE_TOOLS`.
- **`address` workflow: act on review feedback left on your pull request** —
  Grove could review someone else's pull request but had nothing for the
  reverse. `address <pr>` collects the unresolved review threads, the
  "changes requested" review bodies, and the pull request comments, checks out
  the pull request branch, and makes the changes the reviewers asked for before
  validating, committing, and pushing.
  - Feedback the author cannot act on is filtered out: resolved threads and
    their own comments. Outdated threads are included but labelled, so the
    agent checks the current code instead of redoing landed work.
  - Nothing is written to the conversation. Replying to reviewers and resolving
    threads stays with the author, and the agent reports what it did for each
    item, including what it chose not to change.
  - A pull request whose feedback is only questions produces no commit and says
    so, rather than inventing a change to look productive.
  - **Suggested changes** are extracted from the comment that carries them and
    shown as the reviewer's literal replacement for the lines the thread sits
    on. They are never truncated, and the agent is told to apply them as
    written unless they are wrong or unsafe.
  - **A review body is not truncated to a paragraph.** A reviewer who leaves no
    line comments puts the whole review in the body, where several thousand
    characters of findings is ordinary, so review bodies carry a much larger
    budget than the shorter remarks left on a line or under the pull request.
  - **An approved pull request is still addressed.** Only open/closed state
    gates the workflow, and review bodies are collected whatever their state,
    so a reviewer who approves and still leaves notes is not discarded. The run
    warns when pushing may dismiss an existing approval.
  - **`--continue` resumes a run whose delivery gate failed.** The pull request
    branch can only be checked out in one worktree, so a failed validation
    leaves its changes there and every retry is refused. Rerunning with
    `--continue` picks that work up, and the agent is told to finish it rather
    than start again. Without the flag the refusal stands, because the changes
    may equally be the author's own.
  - **A failing delivery gate is handed back to the agent until it passes.**
    The gate runs the repository own tests and waits for them, and a failure
    caused by the agent change is the agent to fix. The output is returned to
    it, the gate is re-run, and this repeats up to two repair attempts, because
    a first fix often reveals the next failure behind it. The agent is told to
    reproduce the failure itself, to distinguish its own breakage from what was
    already broken on the branch, and never to weaken a test to get past the
    gate. If it still fails, the command stops with the real output and names
    the worktree to rerun with `--continue`.
  - Available from the pull request Actions panel as "Address review feedback".
- **Pull request titles describe the change, not the request** — the title was
  the issue title verbatim, so every pull request restated what was asked
  rather than what was done. The implementation reporter, which has already
  read the whole diff, now also writes a one-line title in the repository own
  commit convention. A missing, over-long, or merely restated title falls back
  to the issue title. The delivery commit carries a placeholder subject
  because it is made before the reporter runs, so the same title also
  replaces it at publish time, which is what release tooling reads when it
  builds a changelog. The rewrite happens only while the branch has never
  been pushed, only when the subject is still the placeholder, and never when
  it cannot be proven unpublished.
- **`resolve` no longer calls your own pull requests forks** — the fork check
  compared the head repository name `gh pr view` returns, which is an empty
  string, so every same-repository pull request was rejected with "comes from a
  fork". `ci` and `address` were fixed earlier; `resolve` now shares that check
  instead of keeping its own copy. `isCrossRepository` is the authority, and a
  head repository is only compared by name when GitHub actually identified one.
  A pull request whose fork has since been deleted reports as a fork rather
  than as malformed metadata.
- **A prompt engineer writes the specialists for each repository** — the
  runtime shipped four hand-written personas and picked one, so a repository in
  any other language was told it was being edited by a "Senior TypeScript
  Engineer" and validated with `npm test`. Only six of the fifteen specialists
  knew the stack at all; the planner, test engineer, verifier, reviewer and
  documentation specialist were given the issue and left to infer the language.

  A `prompt-engineer` agent now reads the repository and writes the specialist
  for every stage of the workflow, together with the commands that validate
  each project and the review concerns its own vocabulary raises. It adds no
  workflow step and appears in no checkpoint.

  - **Detection runs on every workflow** and is itself the staleness check. It
    is a filesystem walk, so it costs nothing beside an agent turn; when what
    it finds still matches the profile, no agent runs at all.
  - **The specialist follows the code the step will touch.** Before a diff
    exists that comes from the issue's own paths and area labels; afterwards
    from the diff. A pull request confined to one project is reviewed by that
    project's specialist rather than by a composite naming trees it never
    touches.
  - **A generated command must be proven.** The prompt engineer runs each test
    command once and records the evidence; an unverified command never
    displaces one that already works, so the four built-in stacks behave
    exactly as they did.
  - **A project nothing can validate stops the run before any agent starts**,
    naming the project and `--refresh-profile`, rather than failing inside a
    repair loop that cannot fix a missing test command.
  - **Dependency setup is no longer npm-only.** A profile names the command
    that prepares a fresh checkout and the path whose presence means it has
    already been done.
  - The profile lives in the git common directory, alongside the workflow
    checkpoints. **Nothing is added to the repositories sandcastle is pointed
    at.**

  `seedlookups` is gone from the database-review gate: it was one repository's
  idiom sitting in a shared default, and it belongs in that repository's
  profile.
- **Claude specialists no longer stall on PowerShell** — Windows exposes
  PowerShell as a tool separate from Bash, and it was missing from the tools a
  Claude specialist launches with. Leaving it out did not stop an agent using
  it; it made every call wait for a person, which ends the run with
  `agent_blocked`. A specialist working in a .NET or Windows repository reaches
  for PowerShell unprompted, so the step that inspects or builds the solution
  was the one that stalled. PowerShell is now allowed alongside Bash.
- **A JavaScript project is no longer tested without its dependencies** — a
  git worktree is populated from tracked files alone, so `node_modules` never
  came with it and `npm test` failed before running a single test. Every
  workflow on a repository with a JavaScript project hit this, and the agent
  had to notice and install by hand. Verification now installs them first when
  they are missing, from the lockfile when the project has one, and says so.
  A project that already has them is untouched, so repair cycles pay nothing.
- **A failing test command reports the failure, not its whole transcript** — a
  failing command handed the agent everything it had printed: the restore log,
  every compiler warning, and the failure somewhere inside. The lines that name
  a failure are now selected, along with the three lines after each one so an
  assertion keeps its expected and actual values, and the rest is dropped. A
  15,000-character .NET transcript becomes about 500 characters without losing
  the failing test, its assertion, or the closing tally. When nothing in the
  output names a failure, the end of it is shown instead.
- **Installing the runtime a second time actually replaces it** — the
  installers and `make install-runtime` handed npm a package whose version had
  not changed, so npm reported success and left the previously installed files
  in place. Every upgrade that did not bump the runtime version silently kept
  the old runtime, and the installed commands went on running it. The install
  now removes the installed package before writing the new one.
- **Cross-compiled binaries are no longer committable** — a 20 MB
  `grove-linux-amd64` build artifact was tracked in the repository. It is
  removed, and the Linux and macOS build outputs are ignored alongside the
  Windows one that already was.
- **A pull request that moved on is caught up, not refused** — `ci`, `address`,
  and `resolve` reuse the worktree already checked out for the branch, and
  refused outright whenever its HEAD was not the pull request head. A branch
  moving mid-review is ordinary: a suggestion committed from the web interface,
  a push from another checkout, an assessment asked for while work is in
  flight. The commands now compare the two and act on the difference:
  - **Behind and clean** — fast-forwarded to the pull request head, and the run
    continues.
  - **Ahead** — refused, naming how many commits the checkout holds that the
    pull request does not, because the run would otherwise work on code no
    reviewer has seen.
  - **Diverged** — refused with both counts. A rebase or an amend on one side
    has no safe automatic answer.
  - **Behind with uncommitted changes** — refused, because a fast-forward would
    disturb them.
  - **A head this repository has never seen** — reported as a force-push, with
    the fetch to run.

  A refusal leaves the checkout exactly as it found it.
- **Shorter workflows for the same review coverage** — the full workflow ran 18
  steps; it now runs 10, and the lean workflow 10, without dropping a single
  check:
  - **One `planning` stage** replaces `issue-analysis`, `repository-scout`, and
    `architecture`. Those were a pipeline rather than independent reviews —
    each agent booted a pane and re-read the issue and repository to produce a
    handoff only the next one consumed. The merged stage still writes the same
    three artifacts, so every downstream stage is unchanged. **Two fewer agent
    runs per issue.**
  - **One `domain-review` stage** replaces `security-audit`, `database-review`,
    and `api-contract-review`. Each concern keeps its own checklist, and the
    stage works through whichever ones the diff raised, so a change touching
    all three costs one agent run rather than three.
  - **The adversarial review folds into `verification`.** The verification
    engineer already reads the whole diff and runs the suite; it now challenges
    the change rather than only confirming it, instead of a second agent
    repeating that reading. **One fewer agent run per issue.**
  - **Git bookkeeping is no longer tracked as workflow stages.** `cleanup`
    folds into `delivery`, `push` and `pr` become one `publish`, and
    `publish-review` completes `review`. These never spawned an agent; they
    only added rows to mission control.
  - Workflow checkpoints are now version 7. Older checkpoints keep the
    completed steps that still line up from the start and rerun the rest;
    every stage is safe to repeat.
- **Conditional domain review** — workflows gained a `domain-review` stage that
  runs only when the diff raises one of its concerns, and covers every concern
  the diff raises in a single pass, so an ordinary change costs what it did
  before. The stage audits only, and hands blockers back to the implementation
  specialist. Its concerns are:
  - **security** — authorization coverage, untrusted input, secrets, transport
    and browser protections, sensitive data.
  - **database** — destructive migrations, additive-only compliance,
    backfills, index and cascade impact, rollback cost.
  - **interface contract** — an interface change staying consistent across
    every layer that declares, produces, or consumes it.

  A separate `documentation` stage, gated the same way, updates the
  documentation the repository's own rules require, and leaves generated
  changelogs to the release tooling.
- **Multi-stack repositories are identified and validated per project** — stack
  detection recognised only Go and Python and fell back to TypeScript, so a
  .NET repository was handed a "Senior TypeScript Engineer" persona, and a
  repository holding several stacks was validated as though it held one.
  Detection now reports every project it finds, with the directory that owns
  it:
  - **C# / .NET is recognised** by `*.sln`, `*.slnx`, or `*.csproj`, at the
    repository root or below it.
  - **Validation runs in each project's own directory**, and only for the
    projects the diff touches; an unattributable change validates all of them.
    Previously a single command ran at the repository root, which failed with
    `MSB1003` whenever the solution lived in a subdirectory.
  - **The implementation persona names every stack and its directory**, so a
    mixed repository no longer claims to be a single language.
- **Issue status reflects the GitHub project board** — the issues view reported
  progress purely from local state (a worktree whose branch contains
  `issue-<N>`, or a local workflow run), so work tracked on GitHub by anyone
  else always read as "Open". Grove now reads the Projects v2 `Status` field and
  displays it in the board's own wording, falling back to the local worktree
  signal for issues that are on no board. Requires the `read:project` token
  scope; without it the previous local-only behaviour applies unchanged.

### Fixed

- **clean no longer deletes a branch that has an open pull request** — cleanup
  matched branches against merged pull requests by branch name. Release
  tooling reuses one branch for every release, so a name whose earlier
  releases merged could be hosting an open release pull request; deleting that
  branch made GitHub close it. An open head ref is now excluded from every
  cleanup candidate list, whatever merged on the same name before, and the
  skip is reported rather than silent.
- **Same-repository pull requests are no longer rejected as forks** — `gh pr
  view` returns `headRepository.nameWithOwner` as an empty string, unlike
  `gh pr list`, and the fork guard compared that empty value with the current
  checkout and refused every pull request with "comes from a fork". The head
  repository is now rebuilt from its owner and name, `isCrossRepository`
  decides on its own when the head repository cannot be identified, and a real
  fork is still refused. This affected `ci` as well as the new `address`.
- **A conditional audit is judged on its own schema** — the security,
  database and API contract specialists were validated against the pull
  request reviewer schema, which requires `fixes` and `validation` fields
  those audits are never asked to produce. A sound audit was therefore
  rejected with "PR review verdict has an invalid schema" and failed the
  stage. They now have their own reader, which also tolerates a fenced object
  and treats an "approved" verdict that still lists blockers as blockers.
- **A prompt Herdr did not see start no longer fails the stage** — Herdr accepts
  and delivers a prompt, then requires the agent to be observed working or
  blocked within a fixed five seconds, reporting `agent_prompt_stalled`
  otherwise. A large prompt, or an agent that simply takes a moment to begin,
  misses that window even though the instruction landed. Grove now waits for
  the turn the agent has already started rather than re-sending the prompt,
  which would hand it the same work twice.
- **An agent that needs a person no longer fails the workflow** — when an agent
  stops for something only a person can give (a permission decision, a
  credential, a judgement call), Herdr reports `agent_blocked` and the stage
  used to end there. Grove now focuses that agent's pane, says what it is
  waiting for, and resumes on its own once the agent is idle again. The wait is
  bounded by `AGENT_FLOW_HUMAN_INPUT_TIMEOUT_MS` (default 15 minutes); setting
  it to `0` restores the previous fail-fast behaviour. Unrelated failures are
  still raised immediately. The wait is reported to Grove, so a stage waiting
  on a person shows as `blocked` on the dashboard and counts toward its BLOCKED
  indicator instead of looking like a slow stage, with the agent's summary
  naming the pane to answer in.
- **Claude agents no longer block on the workspace trust dialog** — Claude Code
  asks for confirmation the first time it runs in a directory and records the
  answer per exact path, so trusting a repository does not extend to its
  worktrees. Every workflow creates a new worktree, and pull request review
  creates one per review, so the agent started and then blocked on its first
  prompt with `agent_blocked ... requires interactive input`. Grove now records
  that trust for the worktree before launching a Claude agent. The entry is
  written only when the existing configuration parses, through a temporary file
  and a rename, and any failure leaves the file untouched and logs a warning
  rather than failing the workflow. Note that the permission mode does not
  affect this: `bypassPermissions` still shows the dialog.
- **Conditional review verdicts are written inside the worktree** — the
  security, database, and API contract specialists wrote their verdict to the
  Git common directory, which is outside the agent's working directory and
  needs separate approval under Claude. They now write under `.agent/`, which
  is already removed at delivery.
- **Transient agent startup failures no longer fail the step** — an agent that
  is still blocked when `herdr agent start` returns (for example while its
  splash or notice screen is on the terminal) is now given a bounded wait to
  reach an idle state. The original startup error is surfaced only if the agent
  never settles.
- **Issue assignees are no longer discarded by the cache** — `github_issues`
  already had an `assignees` column, but the upsert never wrote it and the read
  never selected it, so any issue served from cache reported no assignees.
  Assignees now round-trip.
- **Issue bodies are cached** — the cache had no `body` column, so cached issues
  rendered as "(no description)" until the next live sync.
- **Issue state reflects GitHub** — the upsert hardcoded an empty `state`.
  `gh issue list` now requests `state` and the value is persisted.
- **Workflow branch slugs no longer push worktrees past the Windows path
  limit** — `slugifyIssueTitle` cut the title at a hard 60 characters, mid-word,
  producing branch and worktree directory names long enough that deeply nested
  files exceeded the 260-character limit and failed the checkout part-way
  through. Slugs are now capped at 40 characters and cut on a word boundary,
  and `imp` warns before creating a worktree when `core.longpaths` is not
  enabled on Windows.
- **The same repository is no longer cached twice** — the repository path is
  derived from both `os.Getwd()` and `git worktree list`, which disagree on path
  separators on Windows, so each repository was cached under two keys. Cache
  keys are now normalized, including in the staleness check.

### Changed

- Agent backend validation is centralised in `resolveAgentBackend`, replacing
  the per-call `pi`/`opencode` ternaries in workflow telemetry so unknown
  backends fail fast with a consistent error.
- The completion-artifact retry path now covers every non-Pi backend instead of
  OpenCode alone, and its guidance adapts to the active backend.

## [v0.7.1] - 2026-09-20

### Fixed

- **Issue implementation from another Herdr workspace** — Sandcastle now passes
  the target repository as Herdr's worktree source, preventing
  `worktree_not_found` when Grove is opened for a project other than the
  currently active Herdr workspace.

## [v0.7.0] - 2026-09-20

### Added

- **Sandcastle mission control dashboard** — navigate active, attention, and completed workflows directly from Grove, jump to their Herdr panes, and review completed missions without leaving the TUI.
- **Mission Inspector** — inspect live workflow progress, step timing, execution metrics, implementation activity, and structured agent telemetry in a full-screen interface.
- **Mouse support** — click navigation, dashboard tabs, list rows, actions, and Mission Inspector controls; use the wheel to navigate and double-click to activate rows.
- **Workflow lifecycle controls** — stop and remove active Sandcastle workflows or remove completed workflow history with explicit confirmation.
- **Self-contained release installers** — Linux, macOS, and Windows installers provision private Node.js and Sandcastle runtimes and install Herdr when it is not already available.

### Changed

- **Context Actions redesign** — actions now use a clearer, polished panel with keyboard and mouse focus.
- **Completed workflow review** — dashboard workflows are separated into Active / Attention and Completed tabs.
- **Worktree deletion removes its local branch** — confirmed deletion now removes both the worktree and its local branch while preserving the remote branch and protecting the default branch.
- **Release archives include Sandcastle** — GoReleaser packages the compiled runtime and package metadata required by the self-contained installers.

### Fixed

- **Workflow dashboard counts and labels** — actionable counts match visible rows, successful runs no longer appear as active, and human-readable workflow titles replace raw run IDs when available.
- **Long workflow lists remain navigable** — dashboard selection now scrolls to keep the selected mission visible.

## [v0.6.5] - 2026-05-30

### Fixed

- **CI build restored** — the `MkdirAll` call added in v0.6.3 was placed inside the git command wrappers, which caused unit tests to fail on CI (fake paths like `/repo/feature` triggered real filesystem operations). Moved to the app layer where runtime paths are used. `fix(exec)` (76df59f)

## [v0.6.4] - 2026-05-30

### Fixed

- **Ctrl+B cleanup no longer fails on unmerged branches** — the cleanup modal used `git branch -d` (safe delete) which refuses branches that haven't been merged into HEAD. Since the user explicitly selects branches for deletion, `git branch -D` (force delete) is now used so cleanup always succeeds. `fix(tui)` (bd42a8d)

## [v0.6.3] - 2026-05-29

### Fixed

- **First worktree creation no longer fails with "Git operation failed"** — `git worktree add` creates the leaf directory but requires its parent to exist. When `worktrees/<repo>/` hadn't been created yet, git exited fatally. `os.MkdirAll` is now called on the parent path before every `git worktree add`. `fix(exec)` (03770e7)

## [v0.6.2] - 2026-05-29

### Fixed

- **Bulk delete now also removes the branch of a stale worktree** — selecting a worktree in the Ctrl+B cleanup modal only queued the worktree path for deletion; the associated branch was left dangling. The branch is now also deleted after the worktree is removed. `fix(tui)` (2a69733)
- **Header and help modal rebranded from NEXUS to GROVE** — four lingering references to the old project name have been updated. `fix(tui)` (3754330)

### Added

- **Dynamic changelog on the website** — the Changelog section now fetches live data from the GitHub Releases API instead of a hardcoded list with wrong dates. Highlights are parsed from release notes automatically; a skeleton loader and graceful fallback are included. `feat(website)` (4cd6a27)

## [v0.6.1] - 2026-05-28

### Fixed

- **Broken worktree git link no longer crashes the worktree list** — a worktree whose path exists on disk but has a missing/broken `.git` link caused `git status` to exit 128 with "fatal: not a git repository", aborting the entire list load. Now treated gracefully as dirty (IsClean=false) without propagating an error. `fix(exec)` (3cd6407)
- **Cross-platform path assertions in tests** — test expectations used hardcoded Unix forward-slash paths, causing failures on Windows where `filepath.Join` produces backslashes. `fix(test)` (24d9735)
- **Website asset rename** — `nexus-demo.gif` renamed to `grove-demo.gif` to match the rebrand. `fix(website)` (27acb0f, 23c9d6f)

## [v0.6.0] - 2026-05-28

### Added

- **Ctrl+B cleanup modal for stale worktrees and branches** — press `Ctrl+B` from the TUI to surface a modal listing stale/prunable worktrees and their associated branches. Lets you bulk-delete the mess you left behind. `feat(tui)` (#107) (4d79ec2)
- **One-liner install scripts** — `install.sh` (Unix) and `install.ps1` (Windows) so you can get grove running without touching `go install` or release assets manually. `feat(install)` (b5835f2)
- **GitHub Pages landing site** — grove now has a real homepage with a live demo GIF, install instructions, and a fancy favicon. `feat(website)` (11ec084, 75c5196)

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

- **Worktree list went blank when a stale/prunable worktree path no longer existed on disk** — grove tried to run `git status` on the missing directory, the error bubbled up silently, and the entire list stayed empty. Prunable worktrees are now handled gracefully. `fix(worktree)` (1995002)

### Changed

- **`r` now always fetches fresh data from GitHub** — previously it would respect the cache TTL and return stale data if the cache was still "fresh". Manual refresh now always bypasses the cache and updates it afterwards. `feat(app)` (f85fb3b)

## [v0.4.3] - 2026-05-21

### Fixed

- **Worktree creation threw a fit on repos that use `master` instead of `main`** — grove was hardcoding `main` as the base branch like it was 2018. Now it asks the remote what the actual default branch is. `master`, `main`, `trunk`, whatever — it handles it. `fix(worktree)` (f1dc5c3)
- **Auto-updater silently died when grove was installed in `/usr/bin`** — it tried to rename a root-owned binary, got EACCES, and just... said nothing useful. Now it stages the downloaded binary to `~/.cache/grove/grove.staged` and tells you exactly what `sudo` command to run. `fix(updater)` (4874f40)
- **Issues list was mixing up projects** — the GitHub cache had no repo scoping, so running grove from nova showed grove issues and vice versa. Cache now keyed by repo path. Your issues stay in your lane. `fix(cache)` (e5cc91e)

## [v0.4.2] - 2026-05-20

### Fixed

- **Worktrees were sharing a namespace like roommates with boundary issues** — paths weren't scoped to the repo name, so multiple repos could stomp each other's worktrees like clumsy giants. Now each repo gets its own lane. No more collisions. `fix(worktree)` (479360f)

## [v0.4.1] - 2026-05-20

### Fixed

- **Linux threw a full tantrum when grove tried to update itself** — turns out Linux gets *real* possessive about executables that are currently running (ETXTBSY — aka "no bro you literally cannot replace yourself while you exist"). Self-update now writes to a temp file and swaps atomically so Linux stops having an existential crisis mid-update. `fix(updater)` (f2dd7c7)

## [v0.4.0] - 2026-05-20

### Added

- **Your AI bro can now review PRs like a senior dev at 2am** — grove auto-provisions a worktree, drops into it, and fires up Copilot for an AI-assisted PR review. No more context-switching to a browser like an animal. `feat(tui)` (eda6eb8)
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
