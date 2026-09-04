import { useRef, useState, type ReactNode } from 'react'

const DEFAULT_MIN = 280
const DEFAULT_MAX = 900

function readStoredWidth(storageKey: string, fallback: number, min: number, max: number): number {
  try {
    const saved = localStorage.getItem(storageKey)
    if (saved) {
      const parsed = Number(saved)
      if (parsed >= min && parsed <= max) return parsed
    }
  } catch {
    /* ignore */
  }
  return fallback
}

/** Side panel shell with a drag handle on the inner edge (toward the main content). */
export function ResizablePanelFrame({
  storageKey,
  defaultWidth,
  minWidth = DEFAULT_MIN,
  maxWidth = DEFAULT_MAX,
  /** Which side of the layout this panel sits on — handle is on the opposite (inner) edge. */
  side,
  children,
}: {
  storageKey: string
  defaultWidth: number
  minWidth?: number
  maxWidth?: number
  side: 'left' | 'right'
  children: ReactNode
}) {
  const [width, setWidth] = useState(() =>
    readStoredWidth(storageKey, defaultWidth, minWidth, maxWidth),
  )
  const dragRef = useRef<{ startX: number; startWidth: number; latest: number } | null>(null)

  const clamp = (value: number) => {
    const maxW = Math.min(maxWidth, window.innerWidth - 360)
    return Math.min(Math.max(minWidth, value), Math.max(minWidth, maxW))
  }

  const commit = (next: number) => {
    try {
      localStorage.setItem(storageKey, String(next))
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="relative flex h-full shrink-0 flex-col" style={{ width }}>
      {children}
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Изменить ширину панели"
        title="Потяните, чтобы изменить ширину"
        className={
          side === 'left'
            ? 'group absolute top-0 -right-1 z-20 flex h-full w-2 cursor-col-resize touch-none select-none items-center justify-center'
            : 'group absolute top-0 -left-1 z-20 flex h-full w-2 cursor-col-resize touch-none select-none items-center justify-center'
        }
        onPointerDown={(e) => {
          e.preventDefault()
          e.currentTarget.setPointerCapture(e.pointerId)
          dragRef.current = { startX: e.clientX, startWidth: width, latest: width }
        }}
        onPointerMove={(e) => {
          if (!dragRef.current) return
          const rawDelta = e.clientX - dragRef.current.startX
          const delta = side === 'left' ? rawDelta : -rawDelta
          const next = clamp(dragRef.current.startWidth + delta)
          dragRef.current.latest = next
          setWidth(next)
        }}
        onPointerUp={(e) => {
          if (dragRef.current) commit(dragRef.current.latest)
          dragRef.current = null
          try {
            e.currentTarget.releasePointerCapture(e.pointerId)
          } catch {
            /* ignore */
          }
        }}
        onPointerCancel={() => {
          if (dragRef.current) commit(dragRef.current.latest)
          dragRef.current = null
        }}
      >
        <div className="h-8 w-1 rounded-full bg-border-strong/60 transition-colors group-hover:bg-proposed group-active:bg-proposed" />
      </div>
    </div>
  )
}
