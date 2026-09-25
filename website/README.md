# Grove website

The project site, published to GitHub Pages at `https://m00nk0d3.github.io/grove/`.
It is a single page built with React, Vite, Tailwind CSS, and Framer Motion.

## Working on it

Requires Node.js 20.19+ (22 recommended).

```bash
npm ci
npm run dev       # local server with hot reload
npm run build     # type-check and build into dist/
npm run preview   # serve the built site
npm run lint
```

Vite serves the site under `/grove/` (`base` in `vite.config.ts`), so assets in
`public/` are referenced as `/grove/<file>`.

## Structure

| Path | Section |
|---|---|
| `src/components/Hero.tsx`, `TerminalDemo.tsx` | Headline and the animated terminal |
| `src/components/Demo.tsx`, `LiveDemo.tsx` | The live demo: Grove screens played back from `public/demo/frames.json` |
| `src/components/Problems.tsx`, `Features.tsx` | Why Grove exists and what it does |
| `src/components/Themes.tsx` | The built-in themes |
| `src/components/Keybindings.tsx` | Keyboard reference |
| `src/components/Install.tsx` | Installation |
| `src/components/Changelog.tsx` | Recent releases, fetched from the GitHub Releases API |
| `src/lib/useGitHubReleases.ts` | Fetches releases once per page load and parses GoReleaser's release notes; the nav and hero show the latest version from it |

Several sections restate facts from the application — keybindings, themes,
features, install steps — so a change to Grove's behaviour updates them here
too. The keybindings mirror `cmd/grove/app.go` and the in-app help, and the
themes mirror `internal/tui/styles/theme.go`.

The live demo is rendered by Grove itself: `make demo` in the repository root
drives Grove's model through a scripted walkthrough with demonstration data
and writes every screen to `public/demo/frames.json` as text and styles (see
`cmd/grove/demo_export_test.go`, which runs only under the `demo` build tag).
Regenerate it when the screens it shows change. `public/grove-dashboard.png`,
the social preview image, is a screenshot of such a frame and is replaced by
hand.

## Deployment

`.github/workflows/website.yml` builds the site and publishes `dist/` to the
`gh-pages` branch on every push to `main` that changes `website/`, and on
manual dispatch.
