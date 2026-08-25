import { useEffect, useState } from 'react'
import { Clock } from 'lucide-react'
import {
  TimeIntervalPicker,
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
}: {
  time: string
  current: TimeInterval
  onChange: (value: TimeInterval) => void
}) {
  const [open, setOpen] = useState(false)
  const [display, setDisplay] = useState<DisplayZone>('working')
  const [workingTimeZone, setWorkingTimeZone] = useState(defaultWorkingTimeZone)
  const [draft, setDraft] = useState(() => intervalAroundInstant(time, current))
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
        title="Задать окно времени вокруг события"
        aria-expanded={open}
        aria-haspopup="dialog"
        className="inline-flex items-center gap-1 font-mono text-xs text-low hover:underline"
        onClick={() => {
          const next = intervalAroundInstant(time, current)
          setDraft(next)
          onChange(next)
          setOpen((value) => !value)
        }}
      >
        <Clock className="h-3 w-3 text-fg-dim" />
        {formatEventTime(time)}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-30" onClick={() => setOpen(false)} />
          <div className="fixed right-4 top-16 z-40 w-[min(40rem,calc(100vw-2rem))]">
            <TimeIntervalPicker
              value={draft}
              onChange={(value) => {
                setDraft(value)
                onChange(value)
              }}
              display={display}
              onDisplayChange={setDisplay}
              workingTimeZone={workingTimeZone}
              onWorkingTimeZoneChange={setWorkingTimeZone}
            />
          </div>
        </>
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
