import { motion } from 'framer-motion'
import { TerminalDemo } from './TerminalDemo'

export function Hero() {
  return (
    <section className="relative flex min-h-screen flex-col items-center justify-center overflow-hidden px-6 pt-24 pb-16">
      {/* Background grid */}
      <div
        className="pointer-events-none absolute inset-0 opacity-20"
        style={{
          backgroundImage:
            'linear-gradient(rgba(0,217,255,0.05) 1px, transparent 1px), linear-gradient(90deg, rgba(0,217,255,0.05) 1px, transparent 1px)',
          backgroundSize: '60px 60px',
        }}
      />

      {/* Radial glow */}
      <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
        <div
          className="h-[600px] w-[800px] rounded-full opacity-20"
          style={{
            background: 'radial-gradient(ellipse, rgba(0,217,255,0.3) 0%, transparent 70%)',
          }}
        />
      </div>

      {/* Badge */}
      <motion.div
        initial={{ opacity: 0, y: -10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5 }}
        className="mb-6 flex items-center gap-2 rounded-full border border-[#00d9ff]/20 bg-[#00d9ff]/5 px-4 py-1.5"
      >
        <span className="h-2 w-2 animate-pulse rounded-full bg-[#00ff88]" />
        <span className="font-mono text-xs text-[#00d9ff]/80">v0.5.0 — Now with global fuzzy finder</span>
      </motion.div>

      {/* Headline */}
      <motion.h1
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.6, delay: 0.1 }}
        className="mb-6 max-w-4xl text-center text-5xl font-bold leading-tight tracking-tight md:text-7xl"
      >
        Git Worktrees.{' '}
        <span className="text-[#00d9ff] glow-accent">AI Agents.</span>
        <br />
        One Terminal.
      </motion.h1>

      {/* Tagline */}
      <motion.p
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.6, delay: 0.2 }}
        className="mb-10 max-w-2xl text-center text-lg text-[#4a5568] md:text-xl"
      >
        Manage Git worktrees, track GitHub PRs and issues, launch AI coding agents,
        and monitor every active session — all from a single terminal interface.{' '}
        <span className="text-[#e2e8f0]/60">No browser. No tab soup. Just vibes.</span>
      </motion.p>

      {/* CTAs */}
      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.6, delay: 0.3 }}
        className="mb-16 flex flex-col items-center gap-4 sm:flex-row"
      >
        <a
          href="#install"
          className="group relative overflow-hidden rounded border border-[#00d9ff] bg-[#00d9ff]/10 px-8 py-3 font-mono text-sm font-medium text-[#00d9ff] transition-all hover:bg-[#00d9ff]/20"
          style={{ boxShadow: '0 0 30px rgba(0,217,255,0.2)' }}
        >
          <span className="relative z-10">↓ Download v0.5.0</span>
          <div className="absolute inset-0 translate-x-[-100%] bg-gradient-to-r from-transparent via-[#00d9ff]/10 to-transparent transition-transform duration-700 group-hover:translate-x-[100%]" />
        </a>
        <a
          href="https://github.com/m00nk0d3/nexus"
          target="_blank"
          rel="noopener noreferrer"
          className="rounded border border-[#1e2a3a] px-8 py-3 font-mono text-sm font-medium text-[#4a5568] transition-all hover:border-[#00d9ff]/30 hover:text-[#e2e8f0]"
        >
          View on GitHub →
        </a>
      </motion.div>

      {/* Terminal demo */}
      <motion.div
        initial={{ opacity: 0, y: 40 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.8, delay: 0.4 }}
        className="w-full max-w-4xl"
      >
        <TerminalDemo />
      </motion.div>

      {/* Platform badges */}
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.6, delay: 0.8 }}
        className="mt-8 flex items-center gap-6 text-xs text-[#4a5568]"
      >
        {['macOS', 'Linux', 'Windows'].map((p) => (
          <span key={p} className="flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-[#00ff88]" />
            {p}
          </span>
        ))}
        <span className="text-[#1e2a3a]">·</span>
        <span className="font-mono">Go 1.25+</span>
        <span className="text-[#1e2a3a]">·</span>
        <span>MIT License</span>
      </motion.div>
    </section>
  )
}
