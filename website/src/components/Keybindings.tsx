import { motion } from 'framer-motion'

const GROUPS = [
  {
    name: 'Navigation',
    bindings: [
      { key: '↑ / k', desc: 'Move up' },
      { key: '↓ / j', desc: 'Move down' },
      { key: 'Tab', desc: 'Next panel' },
      { key: 'Shift+Tab', desc: 'Previous panel' },
      { key: 'PgUp / PgDn', desc: 'Paginate long lists' },
    ],
  },
  {
    name: 'Worktrees',
    bindings: [
      { key: 'n', desc: 'New worktree' },
      { key: 'Enter / s', desc: 'Switch shell to worktree' },
      { key: 'd', desc: 'Delete worktree' },
      { key: 'l', desc: 'Lock / unlock' },
      { key: 'p', desc: 'Prune stale worktrees' },
      { key: 'r', desc: 'Refresh from remote' },
    ],
  },
  {
    name: 'Search',
    bindings: [
      { key: '/ or Ctrl+F', desc: 'Open global fuzzy finder' },
      { key: 'Type to filter', desc: 'Real-time multi-source search' },
      { key: 'Enter', desc: 'Navigate to result' },
      { key: 'Esc', desc: 'Close finder' },
    ],
  },
  {
    name: 'AI Agents',
    bindings: [
      { key: 'c', desc: 'Spawn GitHub Copilot' },
      { key: 'a', desc: 'Spawn Claude Code' },
      { key: 'f', desc: 'Spawn Aider' },
      { key: 'Ctrl+R', desc: 'AI-assisted PR review' },
    ],
  },
  {
    name: 'Global',
    bindings: [
      { key: 't', desc: 'Open settings / cycle theme' },
      { key: 'u', desc: 'Check for updates' },
      { key: 'f1 / ?', desc: 'Open help modal' },
      { key: 'q / Ctrl+C', desc: 'Quit Nexus' },
    ],
  },
]

export function Keybindings() {
  return (
    <section id="keybindings" className="px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// keyboard first</p>
          <h2 className="text-4xl font-bold tracking-tight">Keybindings</h2>
          <p className="mt-4 text-[#4a5568]">
            Press <kbd className="rounded border border-[#1e2a3a] bg-[#0d1117] px-1.5 py-0.5 font-mono text-xs text-[#e2e8f0]">f1</kbd> or{' '}
            <kbd className="rounded border border-[#1e2a3a] bg-[#0d1117] px-1.5 py-0.5 font-mono text-xs text-[#e2e8f0]">?</kbd>{' '}
            inside Nexus to view these live.
          </p>
        </motion.div>

        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {GROUPS.map((g, gi) => (
            <motion.div
              key={g.name}
              initial={{ opacity: 0, y: 24 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: gi * 0.08 }}
              className="rounded-xl border border-[#1e2a3a] bg-[#0d1117] overflow-hidden"
            >
              <div className="border-b border-[#1e2a3a] bg-[#0a0e27]/50 px-5 py-3">
                <h3 className="font-mono text-xs font-semibold tracking-widest text-[#00d9ff] uppercase">
                  {g.name}
                </h3>
              </div>
              <div className="divide-y divide-[#1e2a3a]">
                {g.bindings.map((b) => (
                  <div key={b.key} className="flex items-center justify-between px-5 py-3 gap-4">
                    <kbd className="shrink-0 rounded border border-[#1e2a3a] bg-[#0a0e27] px-2 py-1 font-mono text-xs text-[#00d9ff]">
                      {b.key}
                    </kbd>
                    <span className="text-right text-sm text-[#4a5568]">{b.desc}</span>
                  </div>
                ))}
              </div>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
