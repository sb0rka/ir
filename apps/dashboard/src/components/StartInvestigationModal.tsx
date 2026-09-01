import { useEffect, useRef, useState } from 'react'
import { Button, Chip } from './ui'

export function StartInvestigationModal({
  eventTitles,
  busy,
  onClose,
  onConfirm,
}: {
  eventTitles: string[]
  busy?: boolean
  onClose: () => void
  onConfirm: (title: string) => void | Promise<void>
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [title, setTitle] = useState(() => (eventTitles.length === 1 ? eventTitles[0] : ''))
  const canSubmit = title.trim().length > 0 && !busy

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  useEffect(() => {
    inputRef.current?.focus()
    if (eventTitles.length === 1) inputRef.current?.select()
  }, [eventTitles.length])

  const submit = () => {
    const next = title.trim()
    if (!next || busy) return
    void onConfirm(next)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={busy ? undefined : onClose} />
      <form
        role="dialog"
        aria-label="Название расследования"
        className="relative w-full max-w-lg overflow-hidden rounded border border-border bg-surface-1 shadow-xl"
        onSubmit={(event) => {
          event.preventDefault()
          submit()
        }}
      >
        <div className="border-b border-border px-4 py-3">
          <div className="text-[10px] uppercase tracking-wider text-fg-dim">Новое расследование</div>
          <div className="mt-0.5 text-sm text-fg">Название</div>
        </div>

        <div className="space-y-3 p-4">
          <input
            ref={inputRef}
            className="w-full rounded border border-border bg-surface-0 px-2 py-1.5 text-sm outline-none focus:border-fg/30"
            placeholder="Название расследования"
            maxLength={255}
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
          {eventTitles.length > 1 && (
            <div className="space-y-1.5">
              <div className="text-[10px] uppercase tracking-wider text-fg-dim">
                Вставить название события
              </div>
              <div className="flex flex-wrap gap-1.5">
                {eventTitles.map((eventTitle) => (
                  <Chip key={eventTitle} onClick={() => setTitle(eventTitle)} title={eventTitle}>
                    <span className="max-w-[18rem] truncate">{eventTitle}</span>
                  </Chip>
                ))}
              </div>
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
          <Button size="sm" variant="ghost" onClick={onClose} disabled={busy}>
            Отмена
          </Button>
          <Button size="sm" variant="primary" type="submit" disabled={!canSubmit}>
            Создать
          </Button>
        </div>
      </form>
    </div>
  )
}
