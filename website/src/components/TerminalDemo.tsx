import { useEffect, useRef, useState } from 'react'

// Each frame types a prompt and prints what Grove or a workflow shows for it.
// The output mirrors the real screens and log lines; keep it in step with the
// dashboard (cmd/grove/renderer.go) and the runtime's messages
// (runtime/sandcastle/src/orchestrator.ts).
const FRAMES: { prompt: string; output: string[] }[] = [
  {
    prompt: 'grove',
    output: [
      '  ◈ MISSION CONTROL // LIVE OPERATIONS',
      '  SYSTEM RUNNING   27 issues • 4 PRs • 2 PRs NEED ATTENTION',
      '',
      '  WORKTREES 05   AGENTS 03   WORKFLOWS 04   OPEN PRs 04',
      '',
      '  ⌁ WORKFLOWS  [ ACTIVE / ATTENTION 03 ]  [ COMPLETED 01 ]',
      '  ▶ ● Implement issue #218        RUNNING   1 agent • Herdr w1:p2',
      '    ● Review pull request #231    RUNNING   1 agent • Herdr w1:p3',
      '    ● Repair CI #229              BLOCKED   1 agent • Herdr w1:p4',
    ],
  },
  {
    prompt: 'i  ↵  a  ← issue #224, then Actions',
    output: [
      '  ◆ ACTIONS',
      '    ↵  Jump to workflow or create worktree',
      '  ▶ ⚡ Implement issue',
      '    ◉  Open on GitHub',
      '',
      '  ✓ Started imp workflow',
    ],
  },
  {
    prompt: 'imp 224   ← in its Herdr pane',
    output: [
      '  [Target Repository] m00nk0d3/grove | Issue #224',
      '  [Workflow Mode] FULL — touches the sync loop and its tests',
      '  [Sandcastle] ../worktrees/grove/fix-issue-224-sync-tick on fix/issue-224-sync-tick',
      '  [Stack Detector] Assigned specialist: go',
      '  ✓ planning   ✓ tests   ✓ implementation   ✓ verification',
      '  ✓ domain-review   ✓ documentation   ✓ delivery',
      '  [GitHub] Opening pull request...',
      '  [COMPLETE] PR approved: https://github.com/m00nk0d3/grove/pull/232',
    ],
  },
  {
    prompt: 'v  5   ← mission inspector, reports',
    output: [
      '  1 OVERVIEW ── 2 STEPS ── 3 METRICS ── 4 IMPLEMENTATION ── 5 REPORTS',
      '',
      '  ◆ REPORTS 2   [ Implementation report ]   Review verdict',
      '  issue-224-implementation-report.md • updated 11:42',
      '',
      '  Implementation Report',
      '  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━',
      '  • One periodic sync chain, however many syncs complete',
      '  • auto_sync = false stops the background refresh',
    ],
  },
  {
    prompt: '/ sync',
    output: [
      '  ╭─ FUZZY FINDER ─────────────────────────────╮',
      '  │  sync                                       │',
      '  ├─────────────────────────────────────────────┤',
      '  │  ▶ [worktree]  fix/issue-224-sync-tick       │',
      '  │    [issue]     #224 · Sync ticks pile up     │',
      '  │    [pr]        #232 · fix(sync): one tick    │',
      '  │    [file]      cmd/grove/app.go              │',
      '  │    [commit]    3f2a91c fix(sync): one tick   │',
      '  ╰─────────────────────────────────────────────╯',
    ],
  },
]

// lineColor picks a line's colour the way the terminal shows it: log tags in
// their own colours, successes green, the selection and headings in the accent.
function lineColor(line: string): string {
  const text = line.trimStart()
  if (text.startsWith('✓') || text.startsWith('[COMPLETE]')) return '#00ff88'
  if (text.startsWith('▶') || text.startsWith('◈') || text.startsWith('◆') || text.startsWith('━')) return '#00d9ff'
  if (text.startsWith('[GitHub]')) return '#ff7edb'
  if (text.startsWith('[Workflow Mode]')) return '#7aa2f7'
  if (text.startsWith('[Target Repository]')) return '#00ff88'
  if (text.startsWith('[')) return '#8be9fd'
  return '#e2e8f0'
}

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
        <span className="ml-3 font-mono text-xs text-[#4a5568]">herdr — grove</span>
        <div className="ml-auto flex items-center gap-1">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-[#00ff88]" />
          <span className="font-mono text-[10px] text-[#00ff88]/70">live</span>
        </div>
      </div>

      {/* Terminal body */}
      <div className="min-h-[320px] overflow-x-auto p-6 font-mono text-sm">
        {/* Previous output lines */}
        {outputLines.map((line, i) => (
          <div
            key={i}
            className="whitespace-pre leading-6"
            style={{ color: lineColor(line) }}
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
