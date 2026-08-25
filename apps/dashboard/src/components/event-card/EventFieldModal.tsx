import { useEffect, useState } from 'react'
import { entityKindForField, relatedFieldColumns } from '../../lib/pdql'
import { kindLabel } from '../../lib/utils'
import { Button } from '../ui'

export function EventFieldModal({
  field,
  value,
  investigationId,
  eventInContext,
  onClose,
  onAddFilter,
  onAddToContext,
}: {
  field: string
  value: string
  investigationId?: string
  eventInContext: boolean
  onClose: () => void
  onAddFilter: (field: string, value: string) => void
  onAddToContext?: (includeEvent: boolean) => Promise<void>
}) {
  const entityKind = entityKindForField(field)
  const columns = relatedFieldColumns(field)
  const [selected, setSelected] = useState<Set<string>>(() => new Set([field]))
  const [addEntity, setAddEntity] = useState(Boolean(entityKind && investigationId))
  const [includeEvent, setIncludeEvent] = useState(!eventInContext)
  const [busy, setBusy] = useState(false)
  const canContext = Boolean(investigationId && onAddToContext && entityKind)

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const apply = async () => {
    for (const name of selected) onAddFilter(name, value)
    if (canContext && addEntity) {
      setBusy(true)
      try {
        await onAddToContext!(eventInContext || includeEvent)
      } finally {
        setBusy(false)
      }
    }
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div
        role="dialog"
        aria-label={`${field} = ${value}`}
        className="relative w-full max-w-lg overflow-hidden rounded border border-border bg-surface-1 shadow-xl"
      >
        <div className="border-b border-border px-4 py-3">
          <div className="text-[10px] uppercase tracking-wider text-fg-dim">{field}</div>
          <div className="mt-0.5 break-all font-mono text-sm text-fg" title={value}>
            {value}
          </div>
        </div>

        <div className="space-y-4 p-4">
          {canContext && (
            <section>
              <div className="mb-2 text-[10px] uppercase tracking-wider text-fg-dim">
                В контекст как сущность
              </div>
              <label className="flex items-start gap-2 text-xs text-fg-muted">
                <input
                  type="checkbox"
                  className="mt-0.5 accent-fg"
                  checked={addEntity}
                  onChange={(event) => setAddEntity(event.target.checked)}
                />
                <span>
                  Добавить {kindLabel[entityKind!] ?? entityKind}
                  {eventInContext
                    ? ' — событие уже в контексте, будет создана связь'
                    : ''}
                </span>
              </label>
              {addEntity && !eventInContext && (
                <label className="mt-2 flex items-start gap-2 text-xs text-fg-muted">
                  <input
                    type="checkbox"
                    className="mt-0.5 accent-fg"
                    checked={includeEvent}
                    onChange={(event) => setIncludeEvent(event.target.checked)}
                  />
                  <span>Также добавить событие (по умолчанию — со связью)</span>
                </label>
              )}
            </section>
          )}

          <section>
            <div className="mb-2 text-[10px] uppercase tracking-wider text-fg-dim">
              Фильтр по значению
            </div>
            <div className={columns.length > 1 ? 'grid grid-cols-2 gap-3' : undefined}>
              {columns.map((column) => (
                <div key={column.title}>
                  <div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-fg-dim">
                    {column.title}
                  </div>
                  <div className="space-y-1">
                    {column.fields.map((name) => {
                      const checked = selected.has(name)
                      return (
                        <label
                          key={name}
                          className="flex items-center gap-2 rounded border border-border px-2 py-1.5 text-xs hover:bg-surface-2"
                        >
                          <input
                            type="checkbox"
                            className="accent-fg"
                            checked={checked}
                            onChange={() => {
                              setSelected((current) => {
                                const next = new Set(current)
                                if (next.has(name)) next.delete(name)
                                else next.add(name)
                                return next
                              })
                            }}
                          />
                          <span className="font-mono text-fg">{name}</span>
                        </label>
                      )
                    })}
                  </div>
                </div>
              ))}
            </div>
          </section>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
          <Button size="sm" variant="ghost" onClick={onClose}>
            Отмена
          </Button>
          <Button size="sm" variant="primary" disabled={busy} onClick={() => void apply()}>
            Применить
          </Button>
        </div>
      </div>
    </div>
  )
}
