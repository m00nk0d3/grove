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

function parseHighlights(body: string | null): string[] {
  if (!body) return []

  return body
    .split('\n')
    .filter((line) => line.trimStart().startsWith('- '))
    .map((line) => {
      return line
        .trimStart()
        .replace(/^-\s+/, '')          // strip leading "- "
        .replace(/\*\*([^*]+)\*\*/g, '$1') // strip **bold**
        .replace(/`([^`]+)`/g, '$1')   // strip `code`
        .replace(/\s*—.*$/, '')         // drop "— long description" suffix
        .replace(/\s*\(.*?\)\s*$/, '')  // drop trailing "(commit ref)" notes
        .trim()
    })
    .filter((h) => h.length > 0)
    .slice(0, 5)
}

function formatDate(iso: string): string {
  return iso.slice(0, 10) // "2026-05-28T..." → "2026-05-28"
}

export function useGitHubReleases(repo: string, limit = 5) {
  const [releases, setReleases] = useState<Release[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    async function fetchReleases() {
      try {
        const res = await fetch(
          `https://api.github.com/repos/${repo}/releases?per_page=${limit}`,
          { headers: { Accept: 'application/vnd.github+json' } },
        )

        if (!res.ok) {
          throw new Error(`GitHub API ${res.status}: ${res.statusText}`)
        }

        const data: GitHubRelease[] = await res.json()

        if (cancelled) return

        const published = data.filter((r) => !r.draft)

        setReleases(
          published.map((r, i) => ({
            version: r.tag_name,
            date: formatDate(r.published_at),
            isLatest: i === 0,
            isPrerelease: r.prerelease,
            url: r.html_url,
            highlights: parseHighlights(r.body),
          })),
        )
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    fetchReleases()
    return () => { cancelled = true }
  }, [repo, limit])

  return { releases, loading, error }
}
