import { motion } from 'framer-motion'

const FEATURES = [
  { icon: '🖥️', title: '3-Pane TUI', desc: 'Worktree list, GitHub context panel, and detail view — all in one terminal window.' },
  { icon: '🌿', title: 'Full worktree management', desc: 'Create, delete, switch shell, lock/unlock, and prune worktrees without leaving the terminal.' },
  { icon: '🔄', title: 'GitHub sync', desc: 'PRs and issues fetched via the gh CLI and kept fresh in the background automatically.' },
  { icon: '🔍', title: 'Global fuzzy finder', desc: 'Press / or Ctrl+F to search worktrees, issues, PRs, files, branches, and agent history in real-time.' },
  { icon: '🤖', title: 'AI agent launchers', desc: 'Spawn Claude Code, GitHub Copilot, or Aider in the correct worktree directory with a single keypress.' },
  { icon: '🔬', title: 'AI-assisted PR review', desc: 'Press Ctrl+R on any PR — Nexus provisions a review worktree and pre-seeds your agent with a structured review prompt.' },
  { icon: '🌳', title: 'Issue hierarchy', desc: 'Navigate parent/child issue trees and spin up a worktree for any sub-issue in one move.' },
  { icon: '📡', title: 'Active sessions dashboard', desc: 'See which agents are running, which shells are alive, and which worktrees are just sitting there pretending.' },
  { icon: '⬆️', title: 'Auto-update', desc: 'Nexus checks for new versions on startup and self-updates in-app. No more brew upgrade guilt-trips.' },
  { icon: '🎨', title: '9 built-in themes', desc: 'Digital Noir, Matrix, Light, Everforest, Tokyo Night, Catppuccin, Kanagawa, Rosé Pine, One Dark.' },
  { icon: '❓', title: 'In-app help', desc: 'Press f1 or ? at any time for a searchable keybindings and troubleshooting reference.' },
  { icon: '💾', title: 'Local persistence', desc: 'Config in ~/.nexus/config.toml, metadata cached in SQLite — Nexus starts fast, every time.' },
]

export function Features() {
  return (
    <section id="features" className="px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// feature set</p>
          <h2 className="text-4xl font-bold tracking-tight">Everything you need</h2>
          <p className="mt-4 text-[#4a5568]">
            Twelve features that eliminate the glue work between you, your git, and your AI.
          </p>
        </motion.div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {FEATURES.map((f, i) => (
            <motion.div
              key={f.title}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.4, delay: (i % 4) * 0.08 }}
              className="group rounded-xl border border-[#1e2a3a] bg-[#0d1117] p-5 transition-all duration-300 hover:border-[#00d9ff]/20 hover:bg-[#0d1117]"
              style={{ '--hover-glow': 'rgba(0,217,255,0.05)' } as React.CSSProperties}
            >
              <div className="mb-3 text-2xl">{f.icon}</div>
              <h3 className="mb-1.5 text-sm font-semibold text-[#e2e8f0] group-hover:text-[#00d9ff] transition-colors">
                {f.title}
              </h3>
              <p className="text-xs leading-relaxed text-[#4a5568]">{f.desc}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
