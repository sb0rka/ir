import { useState } from 'react'
import { Clock } from 'lucide-react'
import type { Severity } from '../../types'
import { SeverityBadge } from '../ui'
import {
  TimeIntervalPopover,
  activeTimeZone,
  defaultWorkingTimeZone,
  intervalAroundInstant,
  type DisplayZone,
  type TimeInterval,
} from '../time-interval'

export function EventTimeButton({
  time,
  current,
  onChange,
  onExecute,
  severity,
}: {
  time: string
  current: TimeInterval
  onChange: (value: TimeInterval) => void
  onExecute: (value: TimeInterval) => void
  severity?: Severity
}) {
  const [open, setOpen] = useState(false)
  const [display, setDisplay] = useState<DisplayZone>('working')
  const [workingTimeZone, setWorkingTimeZone] = useState(defaultWorkingTimeZone)
  const zone = activeTimeZone(display, workingTimeZone)

  return (
    <div className="relative flex items-center justify-between gap-2">
      <button
        type="button"
        title="Задать окно времени вокруг события"
        aria-expanded={open}
        aria-haspopup="dialog"
        className="inline-flex items-center gap-1 font-mono text-xs text-fg hover:underline"
        onClick={() => setOpen((value) => !value)}
      >
        <Clock className="h-3 w-3 text-fg-dim" />
        {formatEventTime(time)}
      </button>
      {severity ? <SeverityBadge severity={severity} /> : null}
      {open && (
        <TimeIntervalPopover
          value={intervalAroundInstant(time, current)}
          onApply={onChange}
          onExecute={onExecute}
          onClose={() => setOpen(false)}
          display={display}
          onDisplayChange={setDisplay}
          workingTimeZone={workingTimeZone}
          onWorkingTimeZoneChange={setWorkingTimeZone}
          className="fixed right-4 top-16 z-40 w-[min(40rem,calc(100vw-2rem))]"
        />
      )}
      <span className="sr-only">{zone}</span>
    </div>
  )
}

function formatEventTime(iso: string): string {
  return new Date(iso).toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}
