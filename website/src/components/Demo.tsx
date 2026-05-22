import { motion } from 'framer-motion'

export function Demo() {
  return (
    <section id="demo" className="px-6 py-24">
      <div className="mx-auto max-w-5xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// see it in action</p>
          <h2 className="text-4xl font-bold tracking-tight">Live demo</h2>
          <p className="mt-4 text-[#4a5568]">
            The real thing — no mockups, no staging server. Just a terminal.
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 32, scale: 0.98 }}
          whileInView={{ opacity: 1, y: 0, scale: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.7, ease: [0.16, 1, 0.3, 1] }}
          className="relative"
        >
          {/* Outer glow */}
          <div
            className="absolute -inset-px rounded-2xl opacity-40 blur-xl"
            style={{ background: 'linear-gradient(135deg, #00d9ff20, #00ff8810)' }}
          />

          {/* Terminal chrome wrapper */}
          <div
            className="relative overflow-hidden rounded-2xl border border-[#1e2a3a]"
            style={{ boxShadow: '0 0 0 1px rgba(0,217,255,0.08), 0 32px 80px rgba(0,0,0,0.7)' }}
          >
            {/* Title bar */}
            <div className="flex items-center gap-2 border-b border-[#1e2a3a] bg-[#0d1117] px-5 py-3.5">
              <span className="h-3 w-3 rounded-full bg-[#ff4757]" />
              <span className="h-3 w-3 rounded-full bg-[#ffd700]" />
              <span className="h-3 w-3 rounded-full bg-[#00ff88]" />
              <span className="ml-4 font-mono text-xs text-[#4a5568]">nexus — ~/repos/myproject</span>
              <div className="ml-auto flex items-center gap-1.5">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-[#00ff88]" />
                <span className="font-mono text-[10px] text-[#00ff88]/60">recording</span>
              </div>
            </div>

            {/* GIF */}
            <div className="bg-[#0a0e27]">
              <img
                src="/nexus/nexus-demo.gif"
                alt="Nexus TUI demo — worktree management, fuzzy finder, and AI agent launcher in action"
                className="w-full"
                loading="lazy"
                decoding="async"
              />
            </div>
          </div>
        </motion.div>

        {/* Caption */}
        <motion.p
          initial={{ opacity: 0 }}
          whileInView={{ opacity: 1 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5, delay: 0.3 }}
          className="mt-6 text-center font-mono text-xs text-[#4a5568]"
        >
          Worktree management · Fuzzy finder · AI agent launcher · PR review workflow
        </motion.p>
      </div>
    </section>
  )
}
