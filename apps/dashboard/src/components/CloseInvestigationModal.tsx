import { useEffect, useState } from 'react'
import { Button } from './ui'
import { CLOSE_VERDICTS, clsx } from '../lib/utils'
import type { Verdict } from '../types'

export function CloseInvestigationModal({
  title,
  busy,
  onClose,
  onConfirm,
}: {
  title: string
  busy?: boolean
  onClose: () => void
  onConfirm: (input: { verdict: Verdict; reason: string }) => void | Promise<void>
}) {
  const [verdict, setVerdict] = useState<Verdict | null>(null)
  const [reason, setReason] = useState('')
  const canSubmit = verdict != null && !busy

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={busy ? undefined : onClose} />
      <form
        role="dialog"
        aria-label="Закрыть расследование"
        className="relative w-full max-w-lg overflow-hidden rounded border border-border bg-surface-1 shadow-xl"
        onSubmit={(event) => {
          event.preventDefault()
          if (!verdict || busy) return
          void onConfirm({ verdict, reason: reason.trim() })
        }}
      >
        <div className="border-b border-border px-4 py-3">
          <div className="text-[10px] uppercase tracking-wider text-fg-dim">Закрыть расследование</div>
          <div className="mt-0.5 text-sm text-fg">{title}</div>
        </div>

        <div className="space-y-3 p-4">
          <div className="space-y-1.5">
            <div className="text-[10px] uppercase tracking-wider text-fg-dim">Вердикт</div>
            <div className="grid grid-cols-2 gap-1.5" role="group" aria-label="Вердикт">
              {CLOSE_VERDICTS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  disabled={busy}
                  onClick={() => setVerdict(option.id)}
                  className={clsx(
                    'rounded border px-2 py-1.5 text-left text-xs',
                    verdict === option.id
                      ? 'border-fg/40 bg-surface-3 text-fg'
                      : 'border-border bg-surface-0 text-fg-muted hover:text-fg',
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
          <label className="block space-y-1.5">
            <span className="text-[10px] uppercase tracking-wider text-fg-dim">
              Обоснование (необязательно)
            </span>
            <textarea
              className="w-full resize-none rounded border border-border bg-surface-0 px-2 py-1.5 text-sm outline-none focus:border-fg/30"
              rows={3}
              disabled={busy}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              placeholder="Почему кейс закрывается с этим вердиктом"
            />
          </label>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
          <Button size="sm" variant="ghost" onClick={onClose} disabled={busy}>
            Отмена
          </Button>
          <Button size="sm" variant="primary" type="submit" disabled={!canSubmit}>
            {busy ? 'Закрытие…' : 'Закрыть'}
          </Button>
        </div>
      </form>
    </div>
  )
}
