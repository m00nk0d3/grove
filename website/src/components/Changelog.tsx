import { motion } from 'framer-motion'
import { useGitHubReleases } from '../lib/useGitHubReleases'

function tagFor(isLatest: boolean, isPrerelease: boolean): { label: string; color: string } {
  if (isPrerelease) return { label: 'Pre-release', color: '#ffd700' }
  if (isLatest) return { label: 'Latest', color: '#00ff88' }
  return { label: 'Stable', color: '#00d9ff' }
}

function ReleaseSkeleton() {
  return (
    <div className="space-y-8">
      {[0, 1, 2].map((i) => (
        <div key={i} className="pl-16 relative">
          <div className="absolute left-4 top-5 h-4 w-4 rounded-full bg-[#1e2a3a] animate-pulse" />
          <div className="rounded-xl border border-[#1e2a3a] bg-[#0d1117] p-6">
            <div className="mb-4 flex gap-3">
              <div className="h-5 w-16 rounded bg-[#1e2a3a] animate-pulse" />
              <div className="h-5 w-12 rounded-full bg-[#1e2a3a] animate-pulse" />
              <div className="h-5 w-20 rounded bg-[#1e2a3a] animate-pulse" />
            </div>
            <div className="space-y-2">
              {[0, 1, 2].map((j) => (
                <div key={j} className="h-4 rounded bg-[#1e2a3a] animate-pulse" style={{ width: `${70 + j * 10}%` }} />
              ))}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

export function Changelog() {
  const { releases, loading, error } = useGitHubReleases('m00nk0d3/grove', 5)

  return (
    <section id="changelog" className="px-6 py-24">
      <div className="mx-auto max-w-5xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// release history</p>
          <h2 className="text-4xl font-bold tracking-tight">Changelog</h2>
          <p className="mt-4 text-[#4a5568]">Recent releases — see the full log on GitHub.</p>
        </motion.div>

        <div className="relative">
          {/* Timeline line */}
          <div className="absolute left-6 top-0 bottom-0 w-px bg-gradient-to-b from-[#00d9ff]/40 via-[#1e2a3a] to-transparent" />

          {loading && <ReleaseSkeleton />}

          {error && (
            <div className="pl-16 text-sm text-[#4a5568]">
              Could not load releases.{' '}
              <a
                href="https://github.com/m00nk0d3/grove/releases"
                target="_blank"
                rel="noopener noreferrer"
                className="text-[#00d9ff] underline underline-offset-4"
              >
                View on GitHub →
              </a>
            </div>
          )}

          {!loading && !error && (
            <div className="space-y-8">
              {releases.map((r, i) => {
                const tag = tagFor(r.isLatest, r.isPrerelease)
                return (
                  <motion.div
                    key={r.version}
                    initial={{ opacity: 0, x: -20 }}
                    whileInView={{ opacity: 1, x: 0 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.5, delay: i * 0.1 }}
                    className="pl-16 relative"
                  >
                    {/* Timeline dot */}
                    <div
                      className="absolute left-4 top-5 h-4 w-4 rounded-full border-2 border-[#0a0e27]"
                      style={{ background: tag.color, boxShadow: `0 0 12px ${tag.color}80` }}
                    />

                    <div className="rounded-xl border border-[#1e2a3a] bg-[#0d1117] p-6">
                      <div className="mb-4 flex flex-wrap items-center gap-3">
                        <a
                          href={r.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="font-mono text-xl font-bold text-[#e2e8f0] hover:text-[#00d9ff] transition-colors"
                        >
                          {r.version}
                        </a>
                        <span
                          className="rounded-full px-2.5 py-0.5 font-mono text-xs font-semibold"
                          style={{ background: `${tag.color}20`, color: tag.color, border: `1px solid ${tag.color}40` }}
                        >
                          {tag.label}
                        </span>
                        <span className="font-mono text-xs text-[#4a5568]">{r.date}</span>
                      </div>

                      <ul className="space-y-2">
                        {r.highlights.map((h) => (
                          <li key={h} className="flex items-start gap-2 text-sm text-[#4a5568]">
                            <span className="mt-0.5 shrink-0 text-[#00d9ff]/50">▸</span>
                            <span>{h}</span>
                          </li>
                        ))}
                        {r.highlights.length === 0 && (
                          <li className="text-sm text-[#4a5568] italic">No highlights listed.</li>
                        )}
                      </ul>
                    </div>
                  </motion.div>
                )
              })}
            </div>
          )}
        </div>

        <motion.div
          initial={{ opacity: 0 }}
          whileInView={{ opacity: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="mt-8 pl-16 text-center"
        >
          <a
            href="https://github.com/m00nk0d3/grove/releases"
            target="_blank"
            rel="noopener noreferrer"
            className="font-mono text-sm text-[#4a5568] underline underline-offset-4 transition-colors hover:text-[#00d9ff]"
          >
            View all releases on GitHub →
          </a>
        </motion.div>
      </div>
    </section>
  )
}

