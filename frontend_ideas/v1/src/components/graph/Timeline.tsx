import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import { SEVERITY_COLOR } from './constants'
import { eventsInRange } from './graph-adapters'
import { clamp, formatClock, formatShortDate, toMs } from './time'
import type { EventRef, Selection } from './types'

export function Timeline() {
  const {
    activeInvestigation,
    selection,
    setHoverTime,
    select,
    setTimeRange,
  } = useWorkspaceStore()

  const windowStart = toMs(
    activeInvestigation?.windowStart ?? '2026-07-17T08:00:00.000Z',
  )
  const windowEnd = toMs(
    activeInvestigation?.windowEnd ?? '2026-07-17T12:30:00.000Z',
  )
  const events = activeInvestigation?.events ?? []
  const range =
    activeInvestigation?.filters.timeRange ?? {
      start: windowStart,
      end: windowEnd,
    }
  const selectedEventId = selection?.kind === 'event' ? selection.id : null

  if (!activeInvestigation) return null

  return (
    <TimelineInner
      windowStart={windowStart}
      windowEnd={windowEnd}
      range={range}
      events={events}
      selectedEventId={selectedEventId}
      setHoverTime={setHoverTime}
      select={select}
      setTimeRange={setTimeRange}
    />
  )
}

function TimelineInner({
  windowStart,
  windowEnd,
  range,
  events,
  selectedEventId,
  setHoverTime,
  select,
  setTimeRange,
}: {
  windowStart: number
  windowEnd: number
  range: { start: number; end: number }
  events: EventRef[]
  selectedEventId: string | null
  setHoverTime: (ms: number | null, entityIds?: string[]) => void
  select: (s: Selection) => void
  setTimeRange: (range: { start: number; end: number } | null) => void
}) {
  const span = Math.max(windowEnd - windowStart, 1)
  const visible = useMemo(
    () => eventsInRange(events, { start: windowStart, end: windowEnd }),
    [events, windowStart, windowEnd],
  )

  const trackRef = useRef<HTMLDivElement>(null)
  const [brushing, setBrushing] = useState<{
    origin: number
    current: number
  } | null>(null)

  const xToTime = useCallback(
    (clientX: number) => {
      const el = trackRef.current
      if (!el) return windowStart
      const rect = el.getBoundingClientRect()
      const ratio = clamp((clientX - rect.left) / rect.width, 0, 1)
      return windowStart + ratio * span
    },
    [span, windowStart],
  )

  const timeToPct = (t: number) => ((t - windowStart) / span) * 100

  const onPointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    if ((e.target as HTMLElement).closest('[data-marker]')) return
    const t = xToTime(e.clientX)
    setBrushing({ origin: t, current: t })
    e.currentTarget.setPointerCapture(e.pointerId)
  }

  const onPointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (!brushing) return
    setBrushing({ ...brushing, current: xToTime(e.clientX) })
  }

  const onPointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (!brushing) return
    const a = Math.min(brushing.origin, brushing.current)
    const b = Math.max(brushing.origin, brushing.current)
    setBrushing(null)
    if (b - a < span * 0.02) {
      setTimeRange({ start: windowStart, end: windowEnd })
      return
    }
    setTimeRange({ start: a, end: b })
    e.currentTarget.releasePointerCapture(e.pointerId)
  }

  const brushPct = brushing
    ? {
        left: timeToPct(Math.min(brushing.origin, brushing.current)),
        width: Math.abs(
          timeToPct(brushing.current) - timeToPct(brushing.origin),
        ),
      }
    : {
        left: timeToPct(range.start),
        width: timeToPct(range.end) - timeToPct(range.start),
      }

  const ticks = useMemo(() => {
    const count = 6
    return Array.from({ length: count + 1 }, (_, i) => {
      const t = windowStart + (span * i) / count
      return { t, pct: (i / count) * 100 }
    })
  }, [span, windowStart])

  return (
    <div className="flex h-[168px] flex-col border-t border-[var(--border)] bg-[var(--bg-panel)] px-4 py-3">
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="text-xs font-medium text-[var(--text)]">Timeline</div>
        <div className="text-[10px] text-[var(--text-dim)]">
          Drag to filter range · click marker · hover to highlight
        </div>
        <div className="text-[10px] tabular-nums text-[var(--text-muted)]">
          {formatShortDate(range.start)} — {formatShortDate(range.end)}
        </div>
      </div>

      <div
        ref={trackRef}
        className="relative h-20 flex-1 cursor-crosshair select-none rounded-md border border-[var(--border)] bg-[var(--bg)]"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
      >
        <div
          className="pointer-events-none absolute inset-y-0 bg-[var(--timeline-brush)]"
          style={{ left: `${brushPct.left}%`, width: `${brushPct.width}%` }}
        />

        {ticks.map((tick) => (
          <div
            key={tick.t}
            className="pointer-events-none absolute bottom-0 top-0 border-l border-[var(--border)]"
            style={{ left: `${tick.pct}%` }}
          >
            <span className="absolute bottom-1 left-1 text-[9px] tabular-nums text-[var(--text-dim)]">
              {formatClock(tick.t)}
            </span>
          </div>
        ))}

        {visible.map((ev, idx) => {
          const t = toMs(ev.event_ts)
          const pct = timeToPct(t)
          const inRange = t >= range.start && t <= range.end
          const color = ev.severity
            ? SEVERITY_COLOR[ev.severity]
            : 'var(--accent)'
          const selected = selectedEventId === ev.id
          const lane = (idx % 3) * 18 + 10

          return (
            <button
              key={ev.id}
              type="button"
              data-marker
              title={`${formatClock(ev.event_ts)} — ${ev.title}`}
              className="absolute z-10 -translate-x-1/2 rounded-sm border px-1 py-0.5 text-left transition-opacity"
              style={{
                left: `${pct}%`,
                top: lane,
                borderColor: selected ? 'var(--accent)' : color,
                background: selected
                  ? 'var(--accent-soft)'
                  : 'var(--bg-node)',
                opacity: inRange ? 1 : 0.2,
                maxWidth: 120,
              }}
              onMouseEnter={() =>
                setHoverTime(toMs(ev.event_ts), ev.entity_ids)
              }
              onMouseLeave={() => setHoverTime(null)}
              onClick={(e) => {
                e.stopPropagation()
                select({ kind: 'event', id: ev.id })
              }}
            >
              <div
                className="truncate text-[9px] font-medium leading-tight"
                style={{ color }}
              >
                {ev.title}
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}
