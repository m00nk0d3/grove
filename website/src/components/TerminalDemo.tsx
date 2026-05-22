import { useEffect, useRef, useState } from 'react'

const FRAMES: { prompt: string; output: string[] }[] = [
  {
    prompt: 'nexus',
    output: [
      '  NEXUS v0.5.0  ·  m00nk0d3/nexus',
      '',
      '  Worktrees                    Sessions',
      '  ─────────────────────────    ─────────────',
      '  ▶ main          [main]       ● shell · PID 8421',
      '    feat/fuzzy    [feat/...]   ● copilot running',
      '    feat/pr-rev   [feat/...]   ○ idle',
      '    fix/cache     [fix/...]    ○ idle',
      '',
      '  4 worktrees · 2 active sessions',
    ],
  },
  {
    prompt: '/ fuzzy',
    output: [
      '  ╭─ FUZZY FINDER ───────────────────────────╮',
      '  │  🔍 fuzzy                                 │',
      '  ├──────────────────────────────────────────┤',
      '  │  ▶ [worktree]  feat/fuzzy-finder          │',
      '  │    [issue]     #99 · Global fuzzy search  │',
      '  │    [pr]        #99 · feat: fuzzy finder   │',
      '  │    [file]      internal/fuzzy/fuzzy.go    │',
      '  │    [branch]    feat/issue-99-fuzzy        │',
      '  ╰──────────────────────────────────────────╯',
    ],
  },
  {
    prompt: 'c  ← spawn copilot',
    output: [
      '  Enter Copilot prompt: review this PR',
      '',
      '  Launching GitHub Copilot in',
      '  /worktrees/feat/fuzzy-finder...',
      '',
      '  ✓ Copilot session started (PID 9182)',
      '  ✓ Session recorded',
    ],
  },
  {
    prompt: 'Ctrl+R  ← AI PR review',
    output: [
      '  Provisioning review worktree...',
      '  ✓ Checked out PR #99 → /worktrees/pr-99-review',
      '',
      '  ╭─ AGENT LAUNCHER ─────────────────────────╮',
      '  │  Pre-seeded: PR code review prompt        │',
      '  │  ▶ [c]  GitHub Copilot    ● available     │',
      '  │    [a]  Claude Code       ● available     │',
      '  │    [f]  Aider             ○ disabled       │',
      '  ╰──────────────────────────────────────────╯',
    ],
  },
]

function sleep(ms: number) {
  return new Promise<void>((r) => setTimeout(r, ms))
}

export function TerminalDemo() {
  const [frameIdx, setFrameIdx] = useState(0)
  const [prompt, setPrompt] = useState('')
  const [outputLines, setOutputLines] = useState<string[]>([])
  const [cursor, setCursor] = useState(true)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  useEffect(() => {
    let cancelled = false

    async function run() {
      while (!cancelled) {
        const frame = FRAMES[frameIdx % FRAMES.length]

        // Type prompt char by char
        setOutputLines([])
        for (let i = 0; i <= frame.prompt.length; i++) {
          if (cancelled) return
          setPrompt(frame.prompt.slice(0, i))
          await sleep(50)
        }

        await sleep(300)

        // Print output lines
        for (const line of frame.output) {
          if (cancelled) return
          setOutputLines((prev) => [...prev, line])
          await sleep(60)
        }

        await sleep(2800)

        // Advance frame
        if (!cancelled) {
          setFrameIdx((f) => f + 1)
          setPrompt('')
          setOutputLines([])
        }
      }
    }

    run()
    return () => { cancelled = true }
  }, [frameIdx])

  // Cursor blink
  useEffect(() => {
    const id = setInterval(() => setCursor((c) => !c), 530)
    return () => clearInterval(id)
  }, [])

  return (
    <div
      className="relative overflow-hidden rounded-xl border border-[#1e2a3a] bg-[#0d1117] shadow-2xl scanline"
      style={{ boxShadow: '0 0 0 1px rgba(0,217,255,0.1), 0 40px 80px rgba(0,0,0,0.6)' }}
    >
      {/* Window chrome */}
      <div className="flex items-center gap-2 border-b border-[#1e2a3a] px-4 py-3">
        <span className="h-3 w-3 rounded-full bg-[#ff4757]" />
        <span className="h-3 w-3 rounded-full bg-[#ffd700]" />
        <span className="h-3 w-3 rounded-full bg-[#00ff88]" />
        <span className="ml-3 font-mono text-xs text-[#4a5568]">nexus — ~/repos/myproject</span>
        <div className="ml-auto flex items-center gap-1">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-[#00ff88]" />
          <span className="font-mono text-[10px] text-[#00ff88]/70">live</span>
        </div>
      </div>

      {/* Terminal body */}
      <div className="min-h-[320px] p-6 font-mono text-sm">
        {/* Previous output lines */}
        {outputLines.map((line, i) => (
          <div
            key={i}
            className="leading-6"
            style={{ color: line.startsWith('  ✓') ? '#00ff88' : line.startsWith('  ▶') ? '#00d9ff' : '#e2e8f0' }}
          >
            {line || '\u00A0'}
          </div>
        ))}

        {/* Current prompt line */}
        <div className="mt-1 flex items-center leading-6">
          <span className="text-[#00d9ff]">❯&nbsp;</span>
          <span className="text-[#e2e8f0]">{prompt}</span>
          <span
            className="ml-px inline-block h-[1.1em] w-[2px] bg-[#00d9ff]"
            style={{ opacity: cursor ? 1 : 0 }}
          />
        </div>
      </div>

      {/* Frame indicator */}
      <div className="flex items-center justify-center gap-1.5 border-t border-[#1e2a3a] py-3">
        {FRAMES.map((_, i) => (
          <span
            key={i}
            className="h-1 rounded-full transition-all duration-300"
            style={{
              width: i === frameIdx % FRAMES.length ? '16px' : '6px',
              background: i === frameIdx % FRAMES.length ? '#00d9ff' : '#1e2a3a',
            }}
          />
        ))}
      </div>
    </div>
  )
}
