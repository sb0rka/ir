import { formatClock, formatInstant } from './model'
import { clsx } from '../../lib/utils'

export function TimeRail({
  from,
  to,
  anchor,
  timeZone,
  flash = false,
}: {
  from: string
  to: string
  anchor?: string | null
  timeZone: string
  flash?: boolean
}) {
  const fromMs = Date.parse(from)
  const toMs = Date.parse(to)
  const span = Math.max(toMs - fromMs, 1)
  const pad = span * 0.55
  const railStart = fromMs - pad
  const railEnd = toMs + pad
  const railSpan = railEnd - railStart

  const pct = (ms: number) => ((ms - railStart) / railSpan) * 100
  const left = pct(fromMs)
  const width = Math.max(pct(toMs) - left, 0.8)
  const sameDay = formatInstant(from, timeZone).slice(0, 10) === formatInstant(to, timeZone).slice(0, 10)
  const label = (iso: string) => (sameDay ? formatClock(iso, timeZone) : formatInstant(iso, timeZone))

  const ticks = [
    { iso: from, pct: left, key: 'from' },
    ...(anchor ? [{ iso: anchor, pct: pct(Date.parse(anchor)), key: 'anchor' }] : []),
    { iso: to, pct: left + width, key: 'to' },
  ]

  return (
    <div className="space-y-3">
      <div
        className={clsx(
          'relative h-[5.5rem] overflow-hidden rounded-md border bg-surface-0',
          flash ? 'border-interval shadow-[inset_0_0_0_1px_var(--color-interval)]' : 'border-border',
        )}
        style={{ transition: 'border-color 180ms ease, box-shadow 180ms ease' }}
      >
        <div
          className="absolute inset-y-0 bg-interval/20"
          style={{ left: `${left}%`, width: `${width}%` }}
        />
        <div
          className="absolute inset-y-0 border-x border-interval"
          style={{ left: `${left}%`, width: `${width}%` }}
        />
        {anchor && (
          <div
            className="absolute top-0 z-10 h-full w-px bg-fg"
            style={{ left: `${pct(Date.parse(anchor))}%` }}
          >
            <span className="absolute left-1/2 top-0 h-0 w-0 -translate-x-1/2 border-x-[5px] border-t-[7px] border-x-transparent border-t-fg" />
          </div>
        )}
      </div>
      <div className="relative h-5">
        {ticks.map((tick) => (
          <span
            key={tick.key}
            className="absolute -translate-x-1/2 font-mono text-[11px] tabular-nums text-fg-muted"
            style={{ left: `${tick.pct}%` }}
          >
            {label(tick.iso)}
          </span>
        ))}
      </div>
    </div>
  )
}
