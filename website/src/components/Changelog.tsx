import { motion } from 'framer-motion'

const RELEASES = [
  {
    version: 'v0.5.0',
    date: '2025',
    tag: 'Latest',
    tagColor: '#00ff88',
    highlights: [
      'Global fuzzy finder (/ or Ctrl+F) — search worktrees, issues, PRs, branches, and agent sessions',
      'Fuzzy results navigate directly to relevant worktrees and GitHub context',
      'Performance improvements across list rendering and theme cycling',
    ],
  },
  {
    version: 'v0.4.3',
    date: '2025',
    tag: 'Stable',
    tagColor: '#00d9ff',
    highlights: [
      'Settings modal — cycle themes and configure editor/agent paths in-app',
      'Pagination with PgUp/PgDn for long worktree and issue lists',
      'Refresh binding (r) to pull latest data from GitHub',
    ],
  },
  {
    version: 'v0.4.0',
    date: '2025',
    tag: 'Feature',
    tagColor: '#ffd700',
    highlights: [
      'AI-assisted PR review: Ctrl+R provisions a review worktree and pre-seeds agent prompt',
      'Agent Launcher modal with per-agent availability indicators',
      'Claude Code, GitHub Copilot, and Aider supported out of the box',
    ],
  },
]

export function Changelog() {
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

          <div className="space-y-8">
            {RELEASES.map((r, i) => (
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
                  style={{ background: r.tagColor, boxShadow: `0 0 12px ${r.tagColor}80` }}
                />

                <div className="rounded-xl border border-[#1e2a3a] bg-[#0d1117] p-6">
                  <div className="mb-4 flex flex-wrap items-center gap-3">
                    <h3 className="font-mono text-xl font-bold text-[#e2e8f0]">{r.version}</h3>
                    <span
                      className="rounded-full px-2.5 py-0.5 font-mono text-xs font-semibold"
                      style={{ background: `${r.tagColor}20`, color: r.tagColor, border: `1px solid ${r.tagColor}40` }}
                    >
                      {r.tag}
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
                  </ul>
                </div>
              </motion.div>
            ))}
          </div>
        </div>

        <motion.div
          initial={{ opacity: 0 }}
          whileInView={{ opacity: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="mt-8 pl-16 text-center"
        >
          <a
            href="https://github.com/m00nk0d3/nexus/blob/main/CHANGELOG.md"
            target="_blank"
            rel="noopener noreferrer"
            className="font-mono text-sm text-[#4a5568] underline underline-offset-4 transition-colors hover:text-[#00d9ff]"
          >
            View full CHANGELOG.md →
          </a>
        </motion.div>
      </div>
    </section>
  )
}
