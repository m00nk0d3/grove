import { useEffect, useState } from 'react'

export interface Release {
  version: string
  date: string
  isLatest: boolean
  isPrerelease: boolean
  url: string
  highlights: string[]
}

interface GitHubRelease {
  tag_name: string
  published_at: string
  prerelease: boolean
  draft: boolean
  html_url: string
  body: string | null
}

// Conventional commit types worth showing, most interesting first. Anything
// else (chore, ci, test, …) is left out of a release's highlights.
const HIGHLIGHT_ORDER = ['feat', 'fix', 'perf', 'refactor']

// parseHighlights turns a GoReleaser release body into short highlights.
// GoReleaser lists each commit as "* <sha> <type>(<scope>): <subject> (#PR)";
// the SHA, the pull request reference, and the type prefix are dropped, and
// features are listed before fixes.
export function parseHighlights(body: string | null): string[] {
  if (!body) return []

  const entries = body
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.startsWith('* ') || line.startsWith('- '))
    .map((line) => {
      const text = line
        .slice(2)
        .replace(/^[0-9a-f]{7,40}\s+/i, '') // commit SHA
        .replace(/\s*\(#\d+\)\s*$/, '') // trailing "(#123)"
        .replace(/\*\*([^*]+)\*\*/g, '$1')
        .replace(/`([^`]+)`/g, '$1')
        .trim()
      const match = text.match(/^(\w+)(?:\([^)]*\))?!?:\s*(.+)$/)
      const type = match ? match[1].toLowerCase() : 'other'
      const subject = match ? match[2] : text
      return { type, subject: subject.charAt(0).toUpperCase() + subject.slice(1) }
    })
    .filter((entry) => entry.subject.length > 0)

  const rank = (type: string) => {
    const i = HIGHLIGHT_ORDER.indexOf(type)
    return i < 0 ? HIGHLIGHT_ORDER.length : i
  }
  return entries
    .filter((entry) => rank(entry.type) < HIGHLIGHT_ORDER.length || entry.type === 'other')
    .sort((a, b) => rank(a.type) - rank(b.type))
    .map((entry) => entry.subject)
    .slice(0, 5)
}

function formatDate(iso: string): string {
  return iso.slice(0, 10) // "2026-09-22T18:46:43Z" → "2026-09-22"
}

// One request per repository serves every component on the page: the
// unauthenticated GitHub API allows each visitor 60 requests an hour.
const cache = new Map<string, Promise<Release[]>>()

const RELEASES_PER_PAGE = 10

function loadReleases(repo: string): Promise<Release[]> {
  let pending = cache.get(repo)
  if (!pending) {
    pending = fetch(`https://api.github.com/repos/${repo}/releases?per_page=${RELEASES_PER_PAGE}`, {
      headers: { Accept: 'application/vnd.github+json' },
    })
      .then((res) => {
        if (!res.ok) throw new Error(`GitHub API ${res.status}: ${res.statusText}`)
        return res.json() as Promise<GitHubRelease[]>
      })
      .then((data) =>
        data
          .filter((r) => !r.draft)
          .map((r, i) => ({
            version: r.tag_name,
            date: formatDate(r.published_at),
            isLatest: i === 0,
            isPrerelease: r.prerelease,
            url: r.html_url,
            highlights: parseHighlights(r.body),
          })),
      )
    // A failed request is not cached, so a later render can try again.
    pending.catch(() => cache.delete(repo))
    cache.set(repo, pending)
  }
  return pending
}

export function useGitHubReleases(repo: string, limit = 5) {
  const [releases, setReleases] = useState<Release[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    loadReleases(repo)
      .then((all) => {
        if (!cancelled) setReleases(all.slice(0, limit))
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [repo, limit])

  return { releases, loading, error }
}

// useLatestVersion returns the newest published release's tag, or null until
// it is known or when it cannot be fetched.
export function useLatestVersion(repo: string): string | null {
  const { releases } = useGitHubReleases(repo, 1)
  return releases[0]?.version ?? null
}
