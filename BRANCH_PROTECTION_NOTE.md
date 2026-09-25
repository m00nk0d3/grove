# Branch Protection

The rules `main` is meant to be protected by. Branch protection is configured
in the GitHub web UI (Repository Settings → Branches → rule for `main`), so
this note records the intended rules; the settings page is the source of
truth for what is actually enforced.

## Rules for `main`

```
✓ Require a pull request before merging
  ├─ Required approvals: 1 (a solo maintainer can self-review)
  └─ Dismiss stale approvals when new commits are pushed

✓ Require status checks to pass before merging
  ├─ Run Tests                  (.github/workflows/ci-tests.yml: make test, make lint)
  └─ Validate PR Description    (.github/workflows/ci-pr-check.yml)

✓ Require branches to be up to date before merging
✓ Include administrators
  ├─ Allow force pushes: No
  └─ Allow deletions: No

✓ Automatically delete head branches after merge
```

The status check names are the job names in those workflows. A required check
that no workflow produces never reports, and blocks every pull request, so
update this rule whenever a job is renamed or removed.

## See also

- [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md) — branches, commits,
  tests, and pull requests
- [docs/RUNBOOK.md](docs/RUNBOOK.md) — the release procedure
