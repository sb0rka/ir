import { Check, Copy, Plus, Search } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import {
  entityKindForField,
  eventFieldLabelRu,
  fetchEventFields,
  loadFieldFreq,
  relatedFieldColumns,
  sortFields,
  type CompareOp,
  type EventFieldDef,
  type LogicalJoiner,
} from '../../lib/pdql'
import { kindLabel } from '../../lib/utils'
import { useAppStore } from '../../store/appStore'
import { highlightMatch } from '../pdql/highlight'
import { Button } from '../ui'

const OP_LABELS: Record<CompareOp, string> = {
  '=': '=',
  '!=': '≠',
  '>': '>',
  '<': '<',
  '>=': '≥',
  '<=': '≤',
  contains: 'contains',
  startswith: 'startswith',
  in: 'in',
  is_null: 'is null',
  is_not_null: 'is not null',
}

const FILTER_OPS: CompareOp[] = [
  '=',
  '!=',
  'contains',
  'startswith',
  'in',
  '>',
  '<',
  '>=',
  '<=',
  'is_null',
  'is_not_null',
]

export type AddEventFilter = (
  fields: string | readonly string[],
  value: string,
  op?: CompareOp,
  joiner?: LogicalJoiner,
) => void

function sortFieldNames(names: string[], freq: Record<string, number>): string[] {
  return sortFields(
    names.map((name) => ({
      name,
      type: 'string' as const,
      description: eventFieldLabelRu(name),
    })),
    freq,
    '',
  ).map((field) => field.name)
}

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
  onAddFilter: AddEventFilter
  onAddToContext?: (includeEvent: boolean) => Promise<void>
}) {
  const fieldFreq = useRef(loadFieldFreq()).current
  const entityKind = entityKindForField(field)
  const related = relatedFieldColumns(field).map((column) => ({
    ...column,
    fields: sortFieldNames(column.fields, fieldFreq),
  }))
  const relatedNames = new Set(related.flatMap((column) => column.fields))
  const [selected, setSelected] = useState<Set<string>>(() => new Set([field]))
  const [extraFields, setExtraFields] = useState<string[]>([])
  const [op, setOp] = useState<CompareOp>('=')
  const [joiner, setJoiner] = useState<LogicalJoiner>('or')
  const [draftValue, setDraftValue] = useState(value)
  const [addEntity, setAddEntity] = useState(Boolean(entityKind && investigationId))
  const [includeEvent, setIncludeEvent] = useState(!eventInContext)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [catalogOpen, setCatalogOpen] = useState(false)
  const [catalogQuery, setCatalogQuery] = useState('')
  const [catalog, setCatalog] = useState<EventFieldDef[]>([])
  const canContext = Boolean(investigationId && onAddToContext && entityKind)
  const needsValue = op !== 'is_null' && op !== 'is_not_null'
  const extras = sortFieldNames(
    extraFields.filter((name) => !relatedNames.has(name)),
    fieldFreq,
  )
  const listed = new Set([...related.flatMap((column) => column.fields), ...extras])
  const selectedOrdered = [...related.flatMap((column) => column.fields), ...extras].filter((name) =>
    selected.has(name),
  )
  const grouped = selectedOrdered.length > 1
  const inValues =
    op === 'in'
      ? draftValue
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean)
      : []
  const canApply = selectedOrdered.length > 0 && !busy && (op !== 'in' || inValues.length > 0)
  const somIssueDescriptionEditor = useAppStore((s) => s.somIssueDescriptionEditor)
  const insertSomIssueDescriptionSnippet = useAppStore(
    (s) => s.insertSomIssueDescriptionSnippet,
  )
  const canAddToIssue = somIssueDescriptionEditor != null

  const copyValue = async () => {
    try {
      await navigator.clipboard.writeText(draftValue)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1200)
    } catch {
      /* ignore */
    }
  }

  const toggleField = (name: string) => {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const addCatalogField = (name: string) => {
    setSelected((current) => {
      const next = new Set(current)
      next.add(name)
      return next
    })
    if (!listed.has(name)) {
      setExtraFields((current) => (current.includes(name) ? current : [...current, name]))
    }
  }

  useEffect(() => {
    let cancelled = false
    void fetchEventFields().then((fields) => {
      if (!cancelled) setCatalog(fields)
    })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      if (catalogOpen) {
        setCatalogOpen(false)
        return
      }
      onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [catalogOpen, onClose])

  const apply = async () => {
    if (!canApply) return
    onAddFilter(
      selectedOrdered,
      needsValue ? draftValue : '',
      op,
      grouped ? joiner : undefined,
    )
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

  const addToIssue = () => {
    if (!canAddToIssue || selectedOrdered.length === 0) return
    const lines = selectedOrdered.map((name) =>
      needsValue ? `${name}: ${draftValue}` : `${name}:`,
    )
    if (!insertSomIssueDescriptionSnippet(lines.join('\n'))) return
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div
        role="dialog"
        aria-label={`${field} ${OP_LABELS[op]} ${draftValue}`}
        className="relative w-full max-w-3xl rounded border border-border bg-surface-1 shadow-xl"
      >
        <div className="border-b border-border px-4 py-3">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0 flex-1 space-y-1.5">
              <div className="text-[10px] uppercase tracking-wider text-fg-dim">{field}</div>
              <div className="flex items-center gap-1.5">
                <select
                  value={op}
                  aria-label="Оператор"
                  onChange={(event) => setOp(event.target.value as CompareOp)}
                  className="shrink-0 rounded border border-border bg-surface-0 px-1.5 py-1 font-mono text-[11px] text-fg outline-none focus:border-fg/30"
                >
                  {FILTER_OPS.map((item) => (
                    <option key={item} value={item}>
                      {OP_LABELS[item]}
                    </option>
                  ))}
                </select>
                {needsValue ? (
                  <input
                    autoFocus
                    value={draftValue}
                    aria-label="Значение"
                    placeholder={op === 'in' ? 'a, b, c' : 'Значение'}
                    onChange={(event) => setDraftValue(event.target.value)}
                    className="min-w-0 flex-1 rounded border border-border bg-surface-0 px-2 py-1 font-mono text-sm text-fg outline-none focus:border-fg/30"
                  />
                ) : (
                  <div className="min-w-0 flex-1 font-mono text-sm text-fg-dim">без значения</div>
                )}
              </div>
            </div>
            <Button
              size="icon"
              variant="ghost"
              className="h-7 w-7 shrink-0"
              title={copied ? 'Скопировано' : 'Копировать значение'}
              aria-label={copied ? 'Скопировано' : 'Копировать значение'}
              onClick={() => void copyValue()}
            >
              {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
            </Button>
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
            <div className="mb-2 flex items-center justify-between gap-2">
              <div className="text-[10px] uppercase tracking-wider text-fg-dim">
                Фильтр по значению
              </div>
              <FieldCatalogPopover
                open={catalogOpen}
                query={catalogQuery}
                fields={catalog}
                freq={fieldFreq}
                onQueryChange={setCatalogQuery}
                onOpenChange={(next) => {
                  setCatalogOpen(next)
                  if (!next) setCatalogQuery('')
                }}
                onChoose={addCatalogField}
              />
            </div>
            <div
              className={
                grouped
                  ? 'rounded border border-dashed border-fg/30 bg-surface-2/40 p-1.5'
                  : undefined
              }
            >
              {grouped ? (
                <div className="mb-2 flex items-center gap-1">
                  <button
                    type="button"
                    aria-label="Оператор группы"
                    onClick={() => setJoiner((current) => (current === 'and' ? 'or' : 'and'))}
                    className="rounded border border-border px-2 py-0.5 font-mono text-[11px] uppercase text-fg-muted hover:text-fg"
                  >
                    {joiner}
                  </button>
                  <span className="font-mono text-[11px] text-fg-dim">(</span>
                </div>
              ) : null}
              <div className={related.length > 1 ? 'grid grid-cols-2 gap-3' : undefined}>
                {related.map((column) => (
                  <FieldChecks
                    key={column.title}
                    title={column.title}
                    fields={column.fields}
                    selected={selected}
                    onToggle={toggleField}
                  />
                ))}
              </div>
              {extras.length > 0 && (
                <div className="mt-3">
                  <FieldChecks
                    title="Добавленные"
                    fields={extras}
                    selected={selected}
                    onToggle={toggleField}
                  />
                </div>
              )}
              {grouped ? (
                <div className="mt-1 font-mono text-[11px] text-fg-dim">)</div>
              ) : null}
            </div>
          </section>
        </div>

        <div className="flex items-center justify-between gap-2 border-t border-border px-4 py-2">
          <Button
            size="sm"
            variant="default"
            disabled={!canAddToIssue || selectedOrdered.length === 0}
            title={
              canAddToIssue
                ? 'Вставить key: value в описание открытого issue'
                : 'Откройте редактирование описания issue (карандаш)'
            }
            onClick={addToIssue}
          >
            Добавить в Issue
          </Button>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="ghost" onClick={onClose}>
              Отмена
            </Button>
            <Button size="sm" variant="primary" disabled={!canApply} onClick={() => void apply()}>
              Применить
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function FieldChecks({
  title,
  fields,
  selected,
  onToggle,
}: {
  title: string
  fields: string[]
  selected: Set<string>
  onToggle: (name: string) => void
}) {
  return (
    <div>
      <div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-fg-dim">
        {title}
      </div>
      <div className="space-y-1">
        {fields.map((name) => (
          <label
            key={name}
            className="flex items-center gap-2 rounded border border-border px-2 py-1.5 text-xs hover:bg-surface-2"
          >
            <input
              type="checkbox"
              className="accent-fg"
              checked={selected.has(name)}
              onChange={() => onToggle(name)}
            />
            <span className="font-mono text-fg">{name}</span>
          </label>
        ))}
      </div>
    </div>
  )
}

function FieldCatalogPopover({
  open,
  query,
  fields,
  freq,
  onQueryChange,
  onOpenChange,
  onChoose,
}: {
  open: boolean
  query: string
  fields: EventFieldDef[]
  freq: Record<string, number>
  onQueryChange: (value: string) => void
  onOpenChange: (open: boolean) => void
  onChoose: (name: string) => void
}) {
  const sorted = sortFields(fields, freq, query)

  return (
    <div className="relative">
      <Button
        size="sm"
        variant="ghost"
        title="Добавить поле"
        onClick={() => onOpenChange(!open)}
      >
        <Plus className="h-3.5 w-3.5" />
        Поле
      </Button>
      {open && (
        <>
          <div className="fixed inset-0 z-[60]" onClick={() => onOpenChange(false)} />
          <div className="absolute right-0 top-full z-[70] mt-1 w-80 overflow-hidden rounded border border-border bg-surface-2 shadow-xl">
            <label className="flex items-center gap-1.5 border-b border-border px-2 py-1.5">
              <Search className="h-3.5 w-3.5 text-fg-dim" />
              <input
                autoFocus
                value={query}
                onChange={(event) => onQueryChange(event.target.value)}
                placeholder="Найти поле"
                className="w-full bg-transparent text-xs text-fg outline-none placeholder:text-fg-dim"
              />
            </label>
            <div className="max-h-72 overflow-auto">
              {sorted.length === 0 ? (
                <div className="px-3 py-4 text-xs text-fg-dim">Нет полей по запросу</div>
              ) : (
                sorted.map((item) => (
                  <button
                    key={item.name}
                    type="button"
                    className="flex w-full flex-col items-start gap-0.5 px-3 py-1.5 text-left hover:bg-surface-1"
                    onClick={() => onChoose(item.name)}
                  >
                    <span className="text-xs text-fg">
                      {highlightMatch(item.description || eventFieldLabelRu(item.name), query)}
                    </span>
                    {item.description !== item.name && (
                      <span className="font-mono text-[11px] text-fg-dim">
                        {highlightMatch(item.name, query)}
                      </span>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
