import { useEffect, useState } from 'react'
import { clsx } from '../../lib/utils'
import { Button } from '../ui'
import { TimeIntervalPicker } from './TimeIntervalPicker'
import type { DisplayZone, TimeInterval } from './model'

export function TimeIntervalPopover({
  value,
  onApply,
  onExecute,
  onClose,
  display,
  onDisplayChange,
  workingTimeZone,
  onWorkingTimeZoneChange,
  className,
}: {
  value: TimeInterval
  onApply: (value: TimeInterval) => void
  onExecute: (value: TimeInterval) => void
  onClose: () => void
  display: DisplayZone
  onDisplayChange: (display: DisplayZone) => void
  workingTimeZone: string
  onWorkingTimeZoneChange: (timeZone: string) => void
  className?: string
}) {
  const [draft, setDraft] = useState(value)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.stopImmediatePropagation()
      onClose()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onClose])

  return (
    <>
      <div className="fixed inset-0 z-30" onClick={onClose} />
      <div
        role="dialog"
        aria-label="Окно времени"
        className={clsx(
          'rounded-lg border border-border bg-surface-1 shadow-xl',
          className,
        )}
      >
        <div className="p-5">
          <TimeIntervalPicker
            value={draft}
            onChange={setDraft}
            display={display}
            onDisplayChange={onDisplayChange}
            workingTimeZone={workingTimeZone}
            onWorkingTimeZoneChange={onWorkingTimeZoneChange}
          />
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-border px-3 py-2">
          <Button size="sm" variant="ghost" onClick={onClose}>
            Отмена
          </Button>
          <Button
            size="sm"
            onClick={() => {
              onApply(draft)
              onClose()
            }}
          >
            Применить
          </Button>
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              onExecute(draft)
              onClose()
            }}
          >
            Выполнить
          </Button>
        </div>
      </div>
    </>
  )
}
