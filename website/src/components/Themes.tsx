import { motion } from 'framer-motion'

const THEMES = [
  { name: 'Digital Noir', id: 'digital-noir', bg: '#0a0e27', accent: '#00d9ff', fg: '#e2e8f0', default: true },
  { name: 'Matrix', id: 'matrix', bg: '#000000', accent: '#00ff41', fg: '#ccffcc' },
  { name: 'Tokyo Night', id: 'tokyonight', bg: '#1a1b26', accent: '#7aa2f7', fg: '#a9b1d6' },
  { name: 'Catppuccin', id: 'catppuccin', bg: '#1e1e2e', accent: '#cba6f7', fg: '#cdd6f4' },
  { name: 'Kanagawa', id: 'kanagawa', bg: '#1f1f28', accent: '#7e9cd8', fg: '#dcd7ba' },
  { name: 'Rosé Pine', id: 'rose-pine', bg: '#191724', accent: '#ebbcba', fg: '#e0def4' },
  { name: 'One Dark', id: 'onedark', bg: '#282c34', accent: '#61afef', fg: '#abb2bf' },
  { name: 'Everforest', id: 'everforest', bg: '#2d353b', accent: '#a7c080', fg: '#d3c6aa' },
  { name: 'Light', id: 'light', bg: '#fafafa', accent: '#005cc5', fg: '#24292e' },
]

export function Themes() {
  return (
    <section id="themes" className="px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// aesthetics</p>
          <h2 className="text-4xl font-bold tracking-tight">9 built-in themes</h2>
          <p className="mt-4 text-[#4a5568]">
            Press <kbd className="rounded border border-[#1e2a3a] bg-[#0d1117] px-1.5 py-0.5 font-mono text-xs text-[#e2e8f0]">t</kbd> to
            cycle through themes live, or set one in <code className="font-mono text-xs text-[#00d9ff]">~/.nexus/config.toml</code>.
          </p>
        </motion.div>

        <div className="grid gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-5">
          {THEMES.map((t, i) => (
            <motion.div
              key={t.id}
              initial={{ opacity: 0, scale: 0.95 }}
              whileInView={{ opacity: 1, scale: 1 }}
              viewport={{ once: true }}
              transition={{ duration: 0.4, delay: (i % 5) * 0.06 }}
              className="group relative overflow-hidden rounded-xl border transition-all duration-300 hover:scale-105"
              style={{ borderColor: t.default ? `${t.accent}40` : '#1e2a3a', background: t.bg }}
            >
              {t.default && (
                <div className="absolute right-2 top-2 rounded-full px-2 py-0.5 font-mono text-[9px]"
                  style={{ background: `${t.accent}20`, color: t.accent, border: `1px solid ${t.accent}40` }}>
                  default
                </div>
              )}
              {/* Mini TUI preview */}
              <div className="p-4">
                <div className="mb-3 flex gap-1.5">
                  <span className="h-2 w-2 rounded-full" style={{ background: '#ff4757' }} />
                  <span className="h-2 w-2 rounded-full" style={{ background: '#ffd700' }} />
                  <span className="h-2 w-2 rounded-full" style={{ background: '#00ff88' }} />
                </div>
                {/* Fake TUI rows */}
                <div className="space-y-1.5 font-mono text-[9px]">
                  <div className="flex items-center gap-1.5">
                    <span style={{ color: t.accent }}>▶</span>
                    <span style={{ color: t.fg, opacity: 0.9 }}>main</span>
                    <span style={{ color: t.accent, opacity: 0.5 }} className="ml-auto text-[8px]">●</span>
                  </div>
                  <div className="flex items-center gap-1.5 opacity-60">
                    <span style={{ color: t.fg, opacity: 0.3 }}>›</span>
                    <span style={{ color: t.fg, opacity: 0.6 }}>feat/foo</span>
                  </div>
                  <div className="flex items-center gap-1.5 opacity-40">
                    <span style={{ color: t.fg, opacity: 0.3 }}>›</span>
                    <span style={{ color: t.fg, opacity: 0.4 }}>fix/bar</span>
                  </div>
                  <div className="mt-2 h-px" style={{ background: t.accent, opacity: 0.15 }} />
                  <div className="pt-1 font-mono text-[8px]" style={{ color: t.accent, opacity: 0.7 }}>
                    ❯ _
                  </div>
                </div>
              </div>
              {/* Color swatches */}
              <div className="flex h-1.5">
                <div className="flex-1" style={{ background: t.bg }} />
                <div className="flex-1" style={{ background: t.accent }} />
                <div className="flex-1" style={{ background: t.fg, opacity: 0.7 }} />
              </div>
              {/* Name */}
              <div className="border-t px-4 py-2.5" style={{ borderColor: `${t.accent}20` }}>
                <p className="font-mono text-xs font-semibold" style={{ color: t.fg }}>
                  {t.name}
                </p>
                <p className="mt-0.5 font-mono text-[9px] opacity-50" style={{ color: t.fg }}>
                  {t.id}
                </p>
              </div>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  )
}
