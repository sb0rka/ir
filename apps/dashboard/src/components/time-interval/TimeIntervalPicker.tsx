import { useEffect, useMemo, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { clsx } from '../../lib/utils'
import { DateTimeParts } from './DateTimeParts'
import {
  PRESET_IDS,
  PRESET_LABELS,
  RANGE_READOUT_WIDTH_EM,
  activeTimeZone,
  customDurationFromHm,
  defaultWorkingTimeZone,
  durationLabel,
  formatRange,
  listTimeZones,
  normalizeRange,
  prioritizeRussianTimeZones,
  resolve,
  returnToNow,
  setRelativeAnchor,
  splitDurationHm,
  switchToRange,
  switchToRelative,
  timeZoneAbbrev,
  timeZoneLabel,
  timeZoneMatchesQuery,
  type DisplayZone,
  type TimeInterval,
} from './model'

export interface TimeIntervalPickerProps {
  value: TimeInterval
  onChange: (value: TimeInterval) => void
  display: DisplayZone
  onDisplayChange: (display: DisplayZone) => void
  workingTimeZone: string
  onWorkingTimeZoneChange: (timeZone: string) => void
}

export function TimeIntervalPicker({
  value,
  onChange,
  display,
  onDisplayChange,
  workingTimeZone,
  onWorkingTimeZoneChange,
}: TimeIntervalPickerProps) {
  const zone = activeTimeZone(display, workingTimeZone)
  const [now, setNow] = useState(() => new Date())
  const [flash, setFlash] = useState(false)
  const [customOpen, setCustomOpen] = useState(value.kind === 'relative' && value.duration.kind === 'custom')
  const [zoneOpen, setZoneOpen] = useState(false)
  const [zoneQuery, setZoneQuery] = useState('')
  const [focusNonce, setFocusNonce] = useState(0)
  const live = value.kind === 'relative' && value.live
  const flashTimer = useRef<number | null>(null)
  const rootRef = useRef<HTMLElement>(null)

  useEffect(() => {
    if (!live) return
    const id = window.setInterval(() => setNow(new Date()), 1000)
    return () => window.clearInterval(id)
  }, [live])

  useEffect(() => {
    if (value.kind === 'relative' && value.duration.kind === 'custom') setCustomOpen(true)
  }, [value])

  useEffect(() => {
    if (focusNonce === 0) return
    const id = window.requestAnimationFrame(() => {
      const el = rootRef.current?.querySelector<HTMLInputElement>('input[aria-label="ДД"]')
      el?.focus()
      el?.select()
    })
    return () => window.cancelAnimationFrame(id)
  }, [focusNonce, value.kind])

  const focusFirstStamp = () => setFocusNonce((n) => n + 1)

  const onPickerClick = (event: { target: EventTarget | null }) => {
    const button = (event.target as HTMLElement | null)?.closest('button')
    if (!button || !rootRef.current?.contains(button)) return
    if (button.getAttribute('aria-haspopup') === 'listbox') {
      if (zoneOpen) focusFirstStamp()
      return
    }
    focusFirstStamp()
  }

  const resolved = resolve(value, now)

  const triggerFlash = () => {
    setFlash(true)
    if (flashTimer.current) window.clearTimeout(flashTimer.current)
    flashTimer.current = window.setTimeout(() => setFlash(false), 400)
  }

  const zones = useMemo(() => listTimeZones(), [])
  const localZone = useMemo(() => defaultWorkingTimeZone(), [])
  const filteredZones = useMemo(() => {
    const q = zoneQuery.trim().toLowerCase()
    const catalog = zones.filter((item) => item !== 'UTC')
    const matches = q ? catalog.filter((item) => timeZoneMatchesQuery(item, q)) : catalog
    return prioritizeRussianTimeZones(matches)
  }, [zoneQuery, zones])

  return (
    <section
      ref={rootRef}
      className="space-y-5"
      onClick={onPickerClick}
    >
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-fg-dim">
          Окно времени
        </div>
        <div className="relative">
          <div className="flex rounded border border-border bg-surface-0 p-0.5">
            <ZoneToggle
              active={display === 'utc'}
              onClick={() => {
                onDisplayChange('utc')
                setZoneOpen(false)
                setZoneQuery('')
              }}
            >
              UTC
            </ZoneToggle>
            <ZoneToggle
              active={display === 'working'}
              aria-expanded={zoneOpen}
              aria-haspopup="listbox"
              onClick={() => {
                onDisplayChange('working')
                setZoneOpen((open) => !open)
              }}
            >
              <span className="max-w-[14rem] truncate">{timeZoneLabel(workingTimeZone)}</span>
              <ChevronDown className="h-3 w-3 shrink-0 text-fg-dim" />
            </ZoneToggle>
          </div>
          {zoneOpen && (
            <div className="absolute right-0 top-full z-50 mt-1 w-[min(20rem,calc(100vw-4rem))] rounded-md border border-border bg-surface-0 p-2 shadow-xl">
              <input
                autoFocus
                value={zoneQuery}
                onChange={(e) => setZoneQuery(e.target.value)}
                placeholder="Москва или Europe/Moscow"
                className="w-full rounded border border-border bg-surface-1 px-3 py-2 font-mono text-sm outline-none placeholder:text-fg-dim"
              />
              <div className="mt-2 max-h-48 overflow-auto" role="listbox">
                {filteredZones.map((item) => (
                  <button
                    key={item}
                    type="button"
                    role="option"
                    aria-selected={item === workingTimeZone}
                    className={clsx(
                      'flex w-full items-baseline justify-between gap-2 rounded px-2 py-1.5 text-left font-mono text-sm hover:bg-surface-2',
                      item === workingTimeZone ? 'text-interval' : 'text-fg',
                    )}
                    onClick={() => {
                      onWorkingTimeZoneChange(item)
                      onDisplayChange('working')
                      setZoneOpen(false)
                      setZoneQuery('')
                    }}
                  >
                    <span className="truncate" title={item}>
                      {timeZoneLabel(item)}
                    </span>
                    {item === localZone ? (
                      <span className="shrink-0 text-[10px] uppercase tracking-wider text-fg-dim">локальная</span>
                    ) : null}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </header>

      <div className="space-y-2">
        <div className="text-sm text-fg-muted">Интервал расследования</div>
        <Readout
          from={resolved.from}
          to={resolved.to}
          zone={zone}
          abbrev={timeZoneAbbrev(zone, resolved.from)}
          flash={flash}
        />
      </div>

      <div className="flex flex-wrap gap-2">
        <ModeButton
          active={value.kind === 'relative'}
          onClick={() => onChange(switchToRelative(value))}
        >
          Относительно точки
        </ModeButton>
        <ModeButton
          active={value.kind === 'range'}
          onClick={() => onChange(switchToRange(value, now))}
        >
          От — до
        </ModeButton>
      </div>

      {value.kind === 'relative' ? (
        <RelativeControls
          value={value}
          zone={zone}
          now={now}
          customOpen={customOpen}
          onCustomOpen={setCustomOpen}
          onChange={onChange}
        />
      ) : (
        <RangeControls
          value={value}
          zone={zone}
          onCommit={(next, swapped) => {
            onChange(next)
            if (swapped) triggerFlash()
          }}
        />
      )}
    </section>
  )
}

function Readout({
  from,
  to,
  zone,
  abbrev,
  flash,
}: {
  from: string
  to: string
  zone: string
  abbrev: string
  flash: boolean
}) {
  return (
    <div
      className={clsx(
        'whitespace-nowrap rounded-md border bg-surface-0 px-4 py-4 font-mono text-sm tabular-nums leading-none text-fg',
        flash ? 'border-interval' : 'border-border',
      )}
      style={{ transition: 'border-color 180ms ease' }}
    >
      <span className="inline-block" style={{ width: `${RANGE_READOUT_WIDTH_EM}em` }}>
        {formatRange(from, to, zone)}
      </span>
      {' · '}
      {abbrev}
    </div>
  )
}

function RelativeControls({
  value,
  zone,
  now,
  customOpen,
  onCustomOpen,
  onChange,
}: {
  value: Extract<TimeInterval, { kind: 'relative' }>
  zone: string
  now: Date
  customOpen: boolean
  onCustomOpen: (open: boolean) => void
  onChange: (value: TimeInterval) => void
}) {
  const custom = splitDurationHm(value.duration)
  const showCustom = customOpen || value.duration.kind === 'custom'
  const live = value.live

  useEffect(() => {
    if (!live || value.direction === 'before') return
    onChange({ ...value, direction: 'before' })
  }, [live, onChange, value])

  return (
    <div className="space-y-5">
      <div className="space-y-2">
        <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-fg-dim">Якорь</div>
        <AnchorField
          live={value.live}
          iso={value.live ? now.toISOString() : value.anchor}
          zone={zone}
          onCommit={(iso) => onChange(setRelativeAnchor(value, iso))}
          onNow={() => onChange(returnToNow(value))}
        />
      </div>

      <div className="space-y-2">
        <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-fg-dim">Длительность</div>
        <div className="flex flex-wrap gap-2">
          {PRESET_IDS.map((id) => (
            <ChipToggle
              key={id}
              active={value.duration.kind === 'preset' && value.duration.id === id}
              onClick={() => {
                onCustomOpen(false)
                onChange({ ...value, duration: { kind: 'preset', id } })
              }}
            >
              {PRESET_LABELS[id]}
            </ChipToggle>
          ))}
          <ChipToggle active={value.duration.kind === 'custom'} onClick={() => onCustomOpen(true)}>
            {value.duration.kind === 'custom' ? durationLabel(value.duration) : 'Свой'}
          </ChipToggle>
        </div>
        {showCustom && (
          <div className="flex items-center gap-3 pt-1">
            <NumberField
              label="ч"
              value={custom.hours}
              onChange={(hours) => onChange({ ...value, duration: customDurationFromHm(hours, custom.minutes) })}
            />
            <NumberField
              label="мин"
              value={custom.minutes}
              onChange={(minutes) => onChange({ ...value, duration: customDurationFromHm(custom.hours, minutes) })}
            />
          </div>
        )}
      </div>

      <div className="space-y-2">
        <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-fg-dim">Направление</div>
        <div className="flex flex-wrap gap-2">
          <ChipToggle
            active={value.direction === 'before'}
            onClick={() => onChange({ ...value, direction: 'before' })}
          >
            ◀ До
          </ChipToggle>
          <ChipToggle
            active={value.direction === 'around'}
            disabled={live}
            onClick={() => onChange({ ...value, direction: 'around' })}
          >
            ◆ Вокруг ±
          </ChipToggle>
          <ChipToggle
            active={value.direction === 'after'}
            disabled={live}
            onClick={() => onChange({ ...value, direction: 'after' })}
          >
            После ▶
          </ChipToggle>
        </div>
      </div>
    </div>
  )
}

function RangeControls({
  value,
  zone,
  onCommit,
}: {
  value: Extract<TimeInterval, { kind: 'range' }>
  zone: string
  onCommit: (value: Extract<TimeInterval, { kind: 'range' }>, swapped: boolean) => void
}) {
  const commitIso = (from: string, to: string) => {
    const next = normalizeRange(from, to)
    onCommit({ kind: 'range', from: next.from, to: next.to }, next.swapped)
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <StampField label="От" iso={value.from} zone={zone} onCommit={(iso) => commitIso(iso, value.to)} />
      <StampField label="До" iso={value.to} zone={zone} onCommit={(iso) => commitIso(value.from, iso)} />
    </div>
  )
}

function AnchorField({
  live,
  iso,
  zone,
  onCommit,
  onNow,
}: {
  live: boolean
  iso: string
  zone: string
  onCommit: (iso: string) => void
  onNow: () => void
}) {
  return (
    <div className="flex items-center gap-3">
      <div className="flex min-w-0 flex-1 items-center rounded-md border border-border bg-surface-0 px-3 py-2.5">
        <DateTimeParts iso={iso} zone={zone} onCommit={onCommit} onEdit={live ? () => onCommit(iso) : undefined} />
      </div>
      <button
        type="button"
        onClick={onNow}
        className={clsx(
          'shrink-0 rounded-md border px-3 py-2 text-sm',
          live
            ? 'border-interval/50 bg-interval/10 text-interval'
            : 'border-border bg-surface-0 text-fg-muted hover:text-fg',
        )}
      >
        Сейчас
      </button>
    </div>
  )
}

function StampField({
  label,
  iso,
  zone,
  onCommit,
}: {
  label: string
  iso: string
  zone: string
  onCommit: (iso: string) => void
}) {
  return (
    <div className="block space-y-2">
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-fg-dim">{label}</div>
      <div className="rounded-md border border-border bg-surface-0 px-3 py-2.5">
        <DateTimeParts iso={iso} zone={zone} onCommit={onCommit} />
      </div>
    </div>
  )
}

function NumberField({
  label,
  value,
  onChange,
}: {
  label: string
  value: number
  onChange: (value: number) => void
}) {
  return (
    <label className="flex items-center gap-2 rounded-md border border-border bg-surface-0 px-3 py-2">
      <input
        type="number"
        min={0}
        value={value}
        onChange={(e) => onChange(Number(e.target.value) || 0)}
        className="w-16 bg-transparent font-mono text-sm tabular-nums outline-none"
      />
      <span className="text-xs text-fg-muted">{label}</span>
    </label>
  )
}

function ZoneToggle({
  active,
  onClick,
  children,
  ...rest
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={clsx(
        'inline-flex items-center gap-1 rounded px-2.5 py-1.5 font-mono text-xs tabular-nums',
        active ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:text-fg',
      )}
      {...rest}
    >
      {children}
    </button>
  )
}

function ModeButton({
  active,
  onClick,
  children,
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={clsx(
        'rounded-md border px-3 py-2 text-sm',
        active
          ? 'border-fg/25 bg-surface-3 text-fg'
          : 'border-border bg-surface-0 text-fg-muted hover:text-fg',
      )}
    >
      {children}
    </button>
  )
}

function ChipToggle({
  active,
  onClick,
  disabled,
  children,
}: {
  active: boolean
  onClick: () => void
  disabled?: boolean
  children: ReactNode
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={clsx(
        'rounded-md border px-3 py-2 text-sm',
        disabled && 'cursor-not-allowed opacity-40 hover:text-fg-muted',
        !disabled && active && 'border-interval/50 bg-interval/10 text-interval',
        !disabled && !active && 'border-border bg-surface-0 text-fg-muted hover:text-fg',
      )}
    >
      {children}
    </button>
  )
}
