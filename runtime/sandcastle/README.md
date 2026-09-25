# Grove Sandcastle Runtime

Grove's workflow runtime. It provides the workflow commands and
`grove-sandcastle`, the JSON interface the Grove dashboard reads.

| Command | Does |
|---|---|
| `imp <issue>` (alias `agent-flow`) | Implement an issue: plan, write tests, implement, verify, run the domain reviews, document, open the pull request, and review it. `--lean` / `--full` override the workflow chosen for the issue, and `--refresh-profile` re-establishes the project profile |
| `review <pr>` | Review a pull request and post the review |
| `address <pr>` | Make the changes that review feedback on your own pull request asks for; `--continue` resumes a stopped run |
| `ci <pr>` | Repair a pull request's failing checks |
| `resolve <pr>` | Resolve a pull request's merge conflicts |
| `clean` | Remove merged worktrees and branches |
| `grove-sandcastle` | `status`, `workflow start`, `workflow remove`, `workflow list`, `workflow get` |

`imp` and `review` also accept `<owner/repo>` before the number. The workflows
run inside Herdr panes and drive the agent backend chosen by
`AGENT_FLOW_AGENT_BACKEND`, or `[sandcastle].default_agent` in
`~/.grove/config.toml` (`opencode`, `pi`, or `claude`).

Run records are written atomically to `grove-workflows/` in the repository's
common Git directory, and checkpoints and reports to `agent-flow/` beside it,
so every worktree of the repository shares them without adding files to a
checkout. The interface with Grove is specified in
[docs/SANDCASTLE_JSON_CONTRACT.md](../../docs/SANDCASTLE_JSON_CONTRACT.md).

## Development

```sh
npm ci
npm run build
npm test
```

From the repository root, `make install-runtime` builds this package and
installs it globally with npm, which puts every command above on `PATH`, and
`make runtime-test` runs the tests.

In Grove, workflows start from the Actions panel: select an issue or pull
request, press `a`, choose the workflow, and press `Enter`. Grove asks
`grove-sandcastle workflow start` to open the repository's Herdr workspace and
run the workflow in a new tab of it.
