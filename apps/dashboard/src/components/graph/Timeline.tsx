import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from 'react'
import { EMPTY_LAYER_IDS } from '../../lib/hypotheses'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import { Button } from '../ui'
import { SEVERITY_COLOR } from './constants'
import { eventsInRange } from './graph-adapters'
import { useHypothesisGraphView } from './useHypothesisGraphView'
import { clamp, formatClock, formatEventTooltip, formatShortDate, toMs } from './time'
import type { EventRef, Selection } from './types'

export function Timeline() {
  const {
    activeInvestigation,
    selection,
    setHoverEvent,
    select,
    setTimeRange,
  } = useWorkspaceStore()

  const windowStart = toMs(
    activeInvestigation?.windowStart ?? '2026-07-17T08:00:00.000Z',
  )
  const windowEnd = toMs(
    activeInvestigation?.windowEnd ?? '2026-07-17T12:30:00.000Z',
  )
  const investigationId = activeInvestigation?.id
  const allNodeIds = useMemo(
    () =>
      activeInvestigation
        ? [
            ...activeInvestigation.entities.map((e) => e.id),
            ...activeInvestigation.alerts.map((a) => a.id),
          ]
        : EMPTY_LAYER_IDS,
    [activeInvestigation],
  )
  const { visibleNodeIds } = useHypothesisGraphView(investigationId, allNodeIds)
  const events = useMemo(() => {
    const all = activeInvestigation?.events ?? []
    if (!visibleNodeIds) return all
    return all.filter((ev) => ev.alert_id != null && visibleNodeIds.has(ev.alert_id))
  }, [activeInvestigation?.events, visibleNodeIds])
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
      setHoverEvent={setHoverEvent}
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
  setHoverEvent,
  select,
  setTimeRange,
}: {
  windowStart: number
  windowEnd: number
  range: { start: number; end: number }
  events: EventRef[]
  selectedEventId: string | null
  setHoverEvent: (eventId: string | null) => void
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

  const rangeActive =
    range.start > windowStart + span * 0.001 ||
    range.end < windowEnd - span * 0.001

  return (
    <div className="flex h-[176px] flex-col border-t border-[var(--border)] bg-[var(--bg-panel)] px-4 py-3">
      <div
        ref={trackRef}
        className="relative h-20 flex-1 cursor-crosshair select-none overflow-hidden rounded-md border border-[var(--border)] bg-[var(--bg)]"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
      >
        <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between gap-3 px-2.5 pt-1.5">
          <div className="flex items-center gap-2">
            <div className="text-[11px] text-[var(--text-muted)]">Таймлайн</div>
            {rangeActive && (
              <Button
                size="sm"
                variant="primary"
                className="pointer-events-auto"
                onClick={(e) => {
                  e.stopPropagation()
                  setTimeRange({ start: windowStart, end: windowEnd })
                }}
              >
                Сбросить диапазон
              </Button>
            )}
          </div>
          <div className="text-[11px] tabular-nums text-[var(--text-muted)]">
            {formatShortDate(range.start)} — {formatShortDate(range.end)}
          </div>
        </div>

        <div
          className="pointer-events-none absolute inset-y-0 border-x border-[var(--border-strong)] bg-[var(--timeline-brush)]"
          style={{ left: `${brushPct.left}%`, width: `${brushPct.width}%` }}
        />

        {ticks.map((tick, i) => {
          const isLast = i === ticks.length - 1
          return (
            <div
              key={tick.t}
              className="pointer-events-none absolute bottom-0 top-0 border-l border-[var(--border)]/60"
              style={{ left: `${tick.pct}%` }}
            >
              <span
                className={
                  isLast
                    ? 'absolute bottom-1 right-1.5 text-[10px] tabular-nums text-[var(--text-muted)]'
                    : 'absolute bottom-1 left-1.5 text-[10px] tabular-nums text-[var(--text-muted)]'
                }
              >
                {formatClock(tick.t)}
              </span>
            </div>
          )
        })}

        {visible.map((ev, idx) => {
          const t = toMs(ev.event_ts)
          const pct = timeToPct(t)
          const inRange = t >= range.start && t <= range.end
          const color = ev.severity
            ? SEVERITY_COLOR[ev.severity]
            : 'var(--accent)'
          const selected = selectedEventId === ev.id
          const lane = (idx % 3) * 16 + 26

          return (
            <button
              key={ev.id}
              type="button"
              data-marker
              title={`${ev.isSeed ? 'исходный · ' : ''}${formatEventTooltip(ev.event_ts, ev.title)}`}
              className="absolute z-10 -translate-x-1/2 rounded-sm border px-1.5 py-0.5 text-left transition-opacity"
              style={{
                left: `${pct}%`,
                top: lane,
                borderColor: selected ? 'var(--accent)' : color,
                background: selected
                  ? 'var(--accent-soft)'
                  : `color-mix(in srgb, ${color} 12%, var(--bg-node))`,
                opacity: inRange ? 1 : 0.25,
                maxWidth: 136,
                boxShadow: ev.isSeed
                  ? `0 0 0 1px var(--bg), 0 0 0 2px var(--text)`
                  : undefined,
              }}
              onMouseEnter={() => setHoverEvent(ev.id)}
              onMouseLeave={() => setHoverEvent(null)}
              onClick={(e) => {
                e.stopPropagation()
                select({ kind: 'event', id: ev.id })
              }}
            >
              <div
                className="flex items-center gap-1 truncate text-[10px] font-medium leading-tight"
                style={{ color }}
              >
                {ev.isSeed && (
                  <span
                    className="inline-block h-1.5 w-1.5 shrink-0 rotate-45 border"
                    style={{ borderColor: color, background: color }}
                    aria-hidden
                  />
                )}
                <span className="truncate">{ev.title}</span>
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}
