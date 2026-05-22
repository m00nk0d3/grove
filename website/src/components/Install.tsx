import { useState } from 'react'
import { motion } from 'framer-motion'
import { cn } from '@/lib/utils'

const TABS = [
  {
    id: 'linux',
    label: 'Linux / macOS',
    steps: [
      {
        comment: '# One-line install',
        code: 'curl -sSL https://raw.githubusercontent.com/m00nk0d3/nexus/main/install.sh | bash',
      },
      {
        comment: '# Verify installation',
        code: 'nexus --version',
      },
    ],
  },
  {
    id: 'windows',
    label: 'Windows (PowerShell)',
    steps: [
      {
        comment: '# One-line install',
        code: 'irm https://raw.githubusercontent.com/m00nk0d3/nexus/main/install.ps1 | iex',
      },
      {
        comment: '# Or with winget',
        code: 'winget install m00nk0d3.nexus',
      },
    ],
  },
  {
    id: 'go',
    label: 'go install',
    steps: [
      {
        comment: '# Requires Go 1.22+',
        code: 'go install github.com/m00nk0d3/nexus/cmd/nexus@latest',
      },
    ],
  },
  {
    id: 'source',
    label: 'Build from source',
    steps: [
      {
        comment: '# Clone and build',
        code: `git clone https://github.com/m00nk0d3/nexus
cd nexus
go build -o nexus ./cmd/nexus
./nexus`,
      },
    ],
  },
]

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  const copy = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <button
      onClick={copy}
      className="rounded px-2 py-1 font-mono text-xs transition-all hover:bg-[#1e2a3a]"
      style={{ color: copied ? '#00ff88' : '#4a5568' }}
    >
      {copied ? '✓ copied' : 'copy'}
    </button>
  )
}

export function Install() {
  const [active, setActive] = useState('linux')
  const tab = TABS.find((t) => t.id === active)!

  return (
    <section id="install" className="px-6 py-24">
      <div className="mx-auto max-w-3xl">
        <motion.div
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.6 }}
          className="mb-14 text-center"
        >
          <p className="mb-3 font-mono text-sm text-[#00d9ff]">// quick start</p>
          <h2 className="text-4xl font-bold tracking-tight">Get started in 30 seconds</h2>
          <p className="mt-4 text-[#4a5568]">
            Requires Go 1.22+, git, and the <code className="font-mono text-xs text-[#00d9ff]">gh</code> CLI.
          </p>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true }}
          transition={{ duration: 0.5 }}
          className="overflow-hidden rounded-xl border border-[#1e2a3a] bg-[#0d1117]"
        >
          {/* Tabs */}
          <div className="flex overflow-x-auto border-b border-[#1e2a3a] bg-[#0a0e27]/50">
            {TABS.map((t) => (
              <button
                key={t.id}
                onClick={() => setActive(t.id)}
                className={cn(
                  'shrink-0 px-5 py-3.5 font-mono text-xs transition-all',
                  active === t.id
                    ? 'border-b-2 border-[#00d9ff] text-[#00d9ff]'
                    : 'text-[#4a5568] hover:text-[#e2e8f0]',
                )}
              >
                {t.label}
              </button>
            ))}
          </div>

          {/* Code blocks */}
          <div className="p-6 space-y-4">
            {tab.steps.map((step, i) => (
              <div key={i} className="rounded-lg border border-[#1e2a3a] bg-[#0a0e27]">
                <div className="flex items-center justify-between border-b border-[#1e2a3a] px-4 py-2">
                  <span className="font-mono text-xs text-[#4a5568]">{step.comment}</span>
                  <CopyButton text={step.code} />
                </div>
                <pre className="overflow-x-auto p-4 font-mono text-sm leading-relaxed text-[#e2e8f0] whitespace-pre-wrap break-all">
                  {step.code}
                </pre>
              </div>
            ))}
          </div>

          {/* Config hint */}
          <div className="border-t border-[#1e2a3a] bg-[#0a0e27]/30 px-6 py-4">
            <p className="font-mono text-xs text-[#4a5568]">
              Config lives at{' '}
              <code className="text-[#00d9ff]">~/.nexus/config.toml</code>. Set{' '}
              <code className="text-[#00d9ff]">worktree_root</code> to your repos folder and you're done.
            </p>
          </div>
        </motion.div>
      </div>
    </section>
  )
}
