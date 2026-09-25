import { motion } from 'framer-motion'

const FEATURES = [
  { icon: '📡', title: 'Mission control', desc: 'Live cards for worktrees, agents, workflows, and open PRs, an operational pulse, and every run split into Active / Attention and Completed.' },
  { icon: '⚡', title: 'Agent workflows', desc: 'Implement an issue, review a PR, address review feedback on your own PR, repair CI, resolve conflicts, clean merged work — each in its own Herdr pane.' },
  { icon: '🔬', title: 'Mission inspector', desc: 'Press v on a run for its steps, metrics, agents, and the reports it wrote — the implementation report and review verdict, rendered in the terminal.' },
  { icon: '🧠', title: 'Your agent, your choice', desc: 'Workflows drive OpenCode, Pi, or Claude Code. Pick the backend in settings; specialists, review passes, and validation follow it.' },
  { icon: '🌿', title: 'Worktree management', desc: 'Create a worktree from an issue or PR, open it or a separate shell, delete it with its local branch, and clean up merged work.' },
  { icon: '🔄', title: 'GitHub sync', desc: 'PRs, issues, reviews, checks, and merge state through the gh CLI, with the PRs that need you flagged first.' },
  { icon: '🌳', title: 'Issue hierarchy', desc: 'Parent and sub-issues as a tree; a sub-issue branches from its parent so its PR targets the right branch.' },
  { icon: '🔍', title: 'Global fuzzy finder', desc: 'Press / or Ctrl+F to search worktrees, issues, PRs, files, branches, and commits as you type.' },
  { icon: '⚙️', title: 'Settings screen', desc: 'Press t for every option in one fullscreen screen, with its config key and what it does. Changes apply immediately.' },
  { icon: '🎨', title: '23 built-in themes', desc: '16 dark and 7 light themes, from Digital Noir, Dracula and Nord to GitHub Light and Solarized Light, picked from a live-previewed list.' },
  { icon: '🖱️', title: 'Keyboard and mouse', desc: 'Everything has a key, and the rail, rows, tabs, and actions are clickable too. Press f1 or ? for help.' },
  { icon: '📦', title: 'One-line install', desc: 'The installers bring Grove, a private Node.js, the Sandcastle runtime, and Herdr. Grove checks for updates on startup.' },
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
            The glue between your worktrees, GitHub, and the agents doing the work.
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
