import { useState } from 'react'
import { Clock } from 'lucide-react'
import { useAppStore } from '../../store/appStore'
import { TimeIntervalPopover } from './TimeIntervalPopover'
import {
  activeTimeZone,
  intervalButtonLabel,
  timeZoneLabel,
  type TimeInterval,
} from './model'

export function TimeIntervalButton({
  value,
  onChange,
  onExecute,
}: {
  value: TimeInterval
  onChange: (value: TimeInterval) => void
  onExecute: (value: TimeInterval) => void
}) {
  const [open, setOpen] = useState(false)
  const display = useAppStore((s) => s.displayTimeZone)
  const workingTimeZone = useAppStore((s) => s.workingTimeZone)
  const setDisplay = useAppStore((s) => s.setDisplayTimeZone)
  const setWorkingTimeZone = useAppStore((s) => s.setWorkingTimeZone)
  const zone = activeTimeZone(display, workingTimeZone)

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
        <TimeIntervalPopover
          value={value}
          onApply={onChange}
          onExecute={onExecute}
          onClose={() => setOpen(false)}
          display={display}
          onDisplayChange={setDisplay}
          workingTimeZone={workingTimeZone}
          onWorkingTimeZoneChange={setWorkingTimeZone}
          className="absolute left-0 top-full z-40 mt-2 w-[min(40rem,calc(100vw-2rem))]"
        />
      )}
    </div>
  )
}
