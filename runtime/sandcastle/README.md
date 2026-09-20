# Grove Sandcastle Runtime

This package is Grove's workflow runtime. It owns the `imp`, `review`,
`resolve`, `ci`, and `clean` commands and exposes the JSON telemetry API that
the Grove dashboard polls:

```sh
grove-sandcastle status --json
grove-sandcastle workflow list --json
grove-sandcastle workflow get <run-id> --json
grove-sandcastle workflow start --kind imp --issue <number> --repo <path> --json
grove-sandcastle workflow start --kind review --pr <number> --repo <path> --json
```

Workflow state is written atomically under the repository's common Git
directory at `grove-workflows/`. This keeps telemetry shared across worktrees
without adding files to the checkout.

## Development

```sh
npm install
npm test
npm link
```

`npm link` installs the Grove-owned runtime and points all compatibility
commands at this package.

Inside Grove, press `o` on an issue to launch `imp`; press `o` on a pull
request to choose `review`, `ci`, or `resolve`; press `o` on the dashboard to
launch `clean`. The launcher reuses the repository's existing Herdr workspace
when present and opens the workflow in a dedicated tab instead of splitting
the Grove pane.
