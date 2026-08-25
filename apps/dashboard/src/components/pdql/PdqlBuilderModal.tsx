import { useEffect } from 'react'
import { serialize } from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'
import { PdqlBuilder } from './PdqlBuilder'

export function PdqlBuilderModal({
  open,
  initialPdql,
  onClose,
  onApply,
  onExecute,
}: {
  open: boolean
  initialPdql: string
  onClose: () => void
  onApply: (pdql: string) => void
  onExecute: (pdql: string) => void
}) {
  const initFrom = usePdqlStore((s) => s.initFrom)
  const applyPdql = usePdqlStore((s) => s.applyPdql)

  useEffect(() => {
    if (!open) return
    initFrom(initialPdql)
  }, [open, initialPdql, initFrom])

  if (!open) return null

  const commit = (): string | null => {
    if (!applyPdql()) return null
    return serialize(usePdqlStore.getState().query)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div
        role="dialog"
        aria-label="Конструктор PDQL"
        className="relative flex h-[85vh] w-[95vw] max-w-7xl flex-col overflow-hidden rounded border border-border bg-surface-1 shadow-xl"
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
      >
        <div className="min-h-0 flex-1">
          <PdqlBuilder />
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-border px-3 py-2">
          <Button size="sm" variant="ghost" onClick={onClose}>
            Отмена
          </Button>
          <Button
            size="sm"
            onClick={() => {
              const text = commit()
              if (text != null) onApply(text)
            }}
          >
            Применить
          </Button>
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              const text = commit()
              if (text != null) onExecute(text)
            }}
          >
            Выполнить
          </Button>
        </div>
      </div>
    </div>
  )
}
