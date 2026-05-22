import { motion } from 'framer-motion'

const PROBLEMS = [
  {
    icon: '🔀',
    title: 'Context-switching hell',
    body: 'Stashing changes, checking out branches, losing your editor state. Git worktrees solve this — but the CLI is clunky. Nexus puts your entire worktree landscape on one screen.',
  },
  {
    icon: '🪟',
    title: 'The five-app shuffle',
    body: 'Terminal for git, browser for GitHub, another terminal for the agent, Slack for the PR link. Nexus collapses all of that into a single pane of glass.',
  },
  {
    icon: '🤖',
    title: 'AI agents without context',
    body: 'Spinning up Claude or Copilot in the wrong directory wastes time and produces wrong answers. Nexus launches agents inside the correct worktree automatically.',
  },
  {
    icon: '🔍',
    title: '"Which branch was that?"',
    body: 'Nexus links GitHub issues and PRs to their worktrees. No more git branch -a | grep vague-memory at 11pm.',
  },
]

export function Problems() {
  return (
    <section className="px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// why it exists</p>
          <h2 className="text-4xl font-bold tracking-tight">The problems Nexus solves</h2>
          <p className="mt-4 text-[#4a5568]">
            Modern multi-feature development is painful. Here's where Nexus intervenes.
          </p>
        </motion.div>

        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          {PROBLEMS.map((p, i) => (
            <motion.div
              key={p.title}
              initial={{ opacity: 0, y: 30 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.5, delay: i * 0.1 }}
              className="group rounded-xl border border-[#1e2a3a] bg-[#0d1117] p-6 transition-all duration-300 glow-box-hover"
            >
              <div className="mb-4 text-3xl">{p.icon}</div>
              <h3 className="mb-2 font-semibold text-[#e2e8f0] group-hover:text-[#00d9ff] transition-colors">
                {p.title}
              </h3>
              <p className="text-sm leading-relaxed text-[#4a5568]">{p.body}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
