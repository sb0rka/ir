import { useEffect, useState } from 'react'
import { Clock } from 'lucide-react'
import { TimeIntervalPicker } from './TimeIntervalPicker'
import {
  activeTimeZone,
  defaultWorkingTimeZone,
  intervalButtonLabel,
  timeZoneLabel,
  type DisplayZone,
  type TimeInterval,
} from './model'

export function TimeIntervalButton({
  value,
  onChange,
}: {
  value: TimeInterval
  onChange: (value: TimeInterval) => void
}) {
  const [open, setOpen] = useState(false)
  const [display, setDisplay] = useState<DisplayZone>('working')
  const [workingTimeZone, setWorkingTimeZone] = useState(defaultWorkingTimeZone)
  const zone = activeTimeZone(display, workingTimeZone)

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

  return (
    <div className="relative">
      <button
        type="button"
        title={`Окно времени · ${timeZoneLabel(zone)}`}
        aria-expanded={open}
        aria-haspopup="dialog"
        onClick={() => setOpen((current) => !current)}
        className="flex min-h-9 items-center gap-1.5 rounded border border-border bg-surface-0 px-2.5 py-1.5 text-xs text-fg-muted hover:text-fg"
      >
        <Clock className="h-3.5 w-3.5 text-fg-dim" />
        <span className="font-mono tabular-nums text-fg">{intervalButtonLabel(value, zone)}</span>
        <span className="font-mono text-fg-dim">{timeZoneLabel(zone)}</span>
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div className="absolute right-0 top-full z-40 mt-2 w-[min(40rem,calc(100vw-2rem))]">
            <TimeIntervalPicker
              value={value}
              onChange={onChange}
              display={display}
              onDisplayChange={setDisplay}
              workingTimeZone={workingTimeZone}
              onWorkingTimeZoneChange={setWorkingTimeZone}
            />
          </div>
        </>
      )}
    </div>
  )
}
