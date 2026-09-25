import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent, ReactNode } from 'react'

// The demo is a scripted walk through Grove, rendered frame by frame by
// Grove's own renderer (`make demo`, cmd/grove/demo_export_test.go) into
// public/demo/frames.json, and played back here as text, so it stays sharp at
// any size.

interface DemoStyle {
  fg?: string
  bg?: string
  b?: boolean
  i?: boolean
  u?: boolean
}

interface DemoFrame {
  chapter: string
  keys?: string
  caption: string
  hold: number
  bg: number
  lines: [string, number][][]
}

interface DemoData {
  version: string
  cols: number
  rows: number
  styles: DemoStyle[]
  frames: DemoFrame[]
}

// Text with no colour of its own is drawn in the terminal's default colour.
const DEFAULT_FG = '#e2e8f0'
const FONT_SIZE = 13
const LINE_HEIGHT = 1.25

// cssFor styles one run. A run is an inline block the height of its row, so
// its background fills the row instead of only the text's height, which
// would leave thin gaps between rows.
function cssFor(style: DemoStyle): CSSProperties {
  return {
    display: 'inline-block',
    height: `${LINE_HEIGHT}em`,
    verticalAlign: 'top',
    color: style.fg ?? DEFAULT_FG,
    background: style.bg,
    fontWeight: style.b ? 700 : undefined,
    fontStyle: style.i ? 'italic' : undefined,
    textDecoration: style.u ? 'underline' : undefined,
  }
}

// cells keeps the terminal grid exact: a monospace font does not always draw
// box-drawing and symbol characters one column wide, so each non-ASCII
// character gets a cell of exactly one column.
function cells(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  let ascii = ''
  let n = 0
  for (const ch of text) {
    if (ch.charCodeAt(0) < 128) {
      ascii += ch
      continue
    }
    if (ascii) {
      out.push(ascii)
      ascii = ''
    }
    out.push(
      <span key={`${key}-${n++}`} style={{ display: 'inline-block', width: '1ch', overflow: 'visible' }}>
        {ch}
      </span>,
    )
  }
  if (ascii) out.push(ascii)
  return out
}

// keyName maps a browser key to the name the demo's key labels use.
function keyName(e: KeyboardEvent): string {
  switch (e.key) {
    case 'Enter':
      return 'enter'
    case 'Escape':
      return 'esc'
    case 'PageDown':
      return 'pgdn'
    default:
      return e.key.length === 1 ? e.key : ''
  }
}

function Terminal({ data, frame }: { data: DemoData; frame: DemoFrame }) {
  const outer = useRef<HTMLDivElement>(null)
  const inner = useRef<HTMLDivElement>(null)
  const [scale, setScale] = useState(1)
  const [natural, setNatural] = useState({ width: 0, height: 0 })

  useLayoutEffect(() => {
    const fit = () => {
      if (!outer.current || !inner.current) return
      const width = inner.current.scrollWidth
      const height = inner.current.scrollHeight
      setNatural({ width, height })
      setScale(Math.min(1, outer.current.clientWidth / width))
    }
    fit()
    const observer = new ResizeObserver(fit)
    if (outer.current) observer.observe(outer.current)
    return () => observer.disconnect()
  }, [])

  const bg = data.styles[frame.bg]?.bg ?? '#0a0e27'
  const lines = useMemo(
    () =>
      frame.lines.map((line, li) => (
        <div key={li} style={{ height: `${LINE_HEIGHT}em`, whiteSpace: 'pre' }}>
          {line.map(([text, style], ri) => (
            <span key={ri} style={cssFor(data.styles[style] ?? {})}>
              {cells(text, `${li}-${ri}`)}
            </span>
          ))}
        </div>
      )),
    [frame, data.styles],
  )

  return (
    <div ref={outer} className="w-full overflow-hidden" style={{ background: bg, height: natural.height ? natural.height * scale : undefined }}>
      <div
        ref={inner}
        className="font-mono"
        style={{
          width: `${data.cols}ch`,
          minHeight: `${data.rows * LINE_HEIGHT}em`,
          fontSize: FONT_SIZE,
          lineHeight: LINE_HEIGHT,
          background: bg,
          transform: `scale(${scale})`,
          transformOrigin: 'top left',
        }}
        aria-hidden="true"
      >
        {lines}
      </div>
    </div>
  )
}

export function LiveDemo() {
  const [data, setData] = useState<DemoData | null>(null)
  const [index, setIndex] = useState(0)
  // Visitors who ask for reduced motion start paused and step through by hand.
  const [playing, setPlaying] = useState(() => !window.matchMedia('(prefers-reduced-motion: reduce)').matches)
  const [visible, setVisible] = useState(false)
  const [failed, setFailed] = useState(false)
  const root = useRef<HTMLDivElement>(null)

  useEffect(() => {
    fetch(`${import.meta.env.BASE_URL}demo/frames.json`)
      .then((res) => (res.ok ? (res.json() as Promise<DemoData>) : Promise.reject(new Error(String(res.status)))))
      .then(setData)
      .catch(() => setFailed(true))
  }, [])

  // Play only while the demo is on screen.
  useEffect(() => {
    if (!root.current) return
    const observer = new IntersectionObserver(([entry]) => setVisible(entry.isIntersecting), { threshold: 0.3 })
    observer.observe(root.current)
    return () => observer.disconnect()
  }, [])

  const count = data?.frames.length ?? 0
  const go = useCallback((i: number) => count && setIndex(((i % count) + count) % count), [count])

  useEffect(() => {
    if (!data || !playing || !visible) return
    const id = setTimeout(() => go(index + 1), data.frames[index].hold)
    return () => clearTimeout(id)
  }, [data, index, playing, visible, go])

  const chapters = useMemo(() => {
    const seen: { name: string; first: number }[] = []
    data?.frames.forEach((f, i) => {
      if (!seen.some((c) => c.name === f.chapter)) seen.push({ name: f.chapter, first: i })
    })
    return seen
  }, [data])

  if (!data) {
    return (
      <div ref={root} className="flex h-[420px] items-center justify-center rounded-2xl border border-[#1e2a3a] bg-[#0a0e27] font-mono text-xs text-[#4a5568]">
        {failed ? 'The demo could not be loaded.' : 'loading demo…'}
      </div>
    )
  }

  const frame = data.frames[index]
  const next = data.frames[(index + 1) % count]
  const nextKey = next.keys?.split(' ')[0]?.toLowerCase()

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'ArrowRight') {
      e.preventDefault()
      go(index + 1)
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault()
      go(index - 1)
    } else if (e.key === ' ') {
      e.preventDefault()
      setPlaying((p) => !p)
    } else if (nextKey && keyName(e).toLowerCase() === nextKey) {
      // Pressing the key the next step uses performs it, as in Grove.
      e.preventDefault()
      setPlaying(false)
      go(index + 1)
    }
  }

  const current = chapters.findLast((c) => c.first <= index)?.name

  return (
    <div ref={root} className="relative">
      <div
        className="absolute -inset-px rounded-2xl opacity-40 blur-xl"
        style={{ background: 'linear-gradient(135deg, #00d9ff20, #00ff8810)' }}
      />
      <div
        tabIndex={0}
        onKeyDown={onKeyDown}
        role="region"
        aria-roledescription="interactive demo"
        aria-label={`Grove demo, step ${index + 1} of ${count}: ${frame.caption}`}
        className="relative overflow-hidden rounded-2xl border border-[#1e2a3a] outline-none focus-visible:border-[#00d9ff]/60"
        style={{ boxShadow: '0 0 0 1px rgba(0,217,255,0.08), 0 32px 80px rgba(0,0,0,0.7)' }}
      >
        {/* Title bar */}
        <div className="flex items-center gap-2 border-b border-[#1e2a3a] bg-[#0d1117] px-5 py-3">
          <span className="h-3 w-3 rounded-full bg-[#ff4757]" />
          <span className="h-3 w-3 rounded-full bg-[#ffd700]" />
          <span className="h-3 w-3 rounded-full bg-[#00ff88]" />
          <span className="ml-4 font-mono text-xs text-[#4a5568]">herdr — grove {data.version}</span>
          <div className="ml-auto flex items-center gap-2">
            {frame.keys?.split(' ').map((k, i) => (
              <kbd
                key={`${index}-${i}`}
                className="rounded border border-[#00d9ff]/40 bg-[#00d9ff]/10 px-2 py-0.5 font-mono text-[11px] text-[#00d9ff]"
              >
                {k}
              </kbd>
            ))}
          </div>
        </div>

        <Terminal data={data} frame={frame} />

        {/* Caption and controls */}
        <div className="border-t border-[#1e2a3a] bg-[#0d1117] px-5 py-4">
          <p className="mb-3 min-h-[1.5rem] font-mono text-sm text-[#e2e8f0]" aria-live="polite">
            {frame.caption}
          </p>
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-1">
              {[
                { label: '⏮', title: 'Restart', action: () => go(0) },
                { label: '◀', title: 'Previous step (←)', action: () => go(index - 1) },
                { label: playing ? '❚❚' : '▶', title: playing ? 'Pause (space)' : 'Play (space)', action: () => setPlaying((p) => !p) },
                { label: '▶▶', title: 'Next step (→)', action: () => go(index + 1) },
              ].map((b) => (
                <button
                  key={b.title}
                  type="button"
                  title={b.title}
                  aria-label={b.title}
                  onClick={b.action}
                  className="rounded border border-[#1e2a3a] px-2.5 py-1 font-mono text-xs text-[#4a5568] transition-colors hover:border-[#00d9ff]/40 hover:text-[#00d9ff]"
                >
                  {b.label}
                </button>
              ))}
            </div>
            <div className="flex flex-wrap items-center gap-1.5">
              {chapters.map((c) => (
                <button
                  key={c.name}
                  type="button"
                  onClick={() => go(c.first)}
                  className="rounded-full border px-3 py-1 font-mono text-[11px] transition-colors"
                  style={
                    c.name === current
                      ? { borderColor: '#00d9ff80', color: '#00d9ff', background: '#00d9ff14' }
                      : { borderColor: '#1e2a3a', color: '#4a5568' }
                  }
                >
                  {c.name}
                </button>
              ))}
            </div>
            <span className="ml-auto font-mono text-[11px] text-[#4a5568]">
              {index + 1} / {count}
            </span>
          </div>
          <div className="mt-3 flex gap-0.5" aria-hidden="true">
            {data.frames.map((_, i) => (
              <span
                key={i}
                className="h-0.5 flex-1 rounded-full transition-colors duration-300"
                style={{ background: i <= index ? '#00d9ff' : '#1e2a3a' }}
              />
            ))}
          </div>
        </div>
      </div>
      <p className="mt-4 text-center font-mono text-[11px] text-[#4a5568]">
        Click the demo, then press the key it shows — or use ← → and space.
      </p>
    </div>
  )
}
