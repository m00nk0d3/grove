import { motion } from 'framer-motion'

// Mirrors the key handling in cmd/grove/app.go and the help modal
// (internal/tui/modal/help.go); keep the three in step.
const GROUPS = [
  {
    name: 'Navigation',
    bindings: [
      { key: '↑ ↓ / j k', desc: 'Move the selection' },
      { key: 'Tab', desc: 'Cycle rail, list, and context panel' },
      { key: 'Enter', desc: 'Open, jump, or create for the selection' },
      { key: 'J / K', desc: 'Scroll the context panel' },
      { key: 'PgDn / n, PgUp', desc: 'Page through issues and PRs' },
    ],
  },
  {
    name: 'Views',
    bindings: [
      { key: 'd', desc: 'Dashboard' },
      { key: 'w', desc: 'Worktrees' },
      { key: 'i', desc: 'Issues' },
      { key: 'p', desc: 'Pull requests' },
      { key: 't', desc: 'Settings' },
    ],
  },
  {
    name: 'Workflows',
    bindings: [
      { key: 'a', desc: 'Focus Actions: start imp, review, address, ci, resolve, clean' },
      { key: '[ / ]', desc: 'Active / Attention ↔ Completed' },
      { key: 'v', desc: 'Open the mission inspector' },
      { key: 'x', desc: 'Stop and remove a run' },
      { key: 'm', desc: 'Mark a succeeded run done' },
    ],
  },
  {
    name: 'Mission inspector',
    bindings: [
      { key: '1–5 / Tab', desc: 'Overview, Steps, Metrics, Implementation, Reports' },
      { key: '[ / ]', desc: 'Previous / next report' },
      { key: 'j k, PgUp PgDn', desc: 'Scroll a report' },
      { key: 't', desc: 'Retry a failed run' },
      { key: 'Enter', desc: "Jump to the run's Herdr pane" },
    ],
  },
  {
    name: 'Global',
    bindings: [
      { key: '/ or Ctrl+F', desc: 'Open the fuzzy finder' },
      { key: 'r', desc: 'Refresh GitHub, Herdr, and Sandcastle' },
      { key: 'f1 / ?', desc: 'Open help' },
      { key: 'q / Esc / Ctrl+C', desc: 'Quit Grove' },
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
            inside Grove to view these live.
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
