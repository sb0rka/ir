import { useDraggable } from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import { fieldPrefix, sortFields, type EventFieldDef } from '../../lib/pdql'
import { clsx } from '../../lib/utils'
import { highlightMatch } from './highlight'

export function FieldSearchList({
  fields,
  freq,
  query,
  onChoose,
  onActivate,
  idPrefix = 'catalog',
}: {
  fields: EventFieldDef[]
  freq: Record<string, number>
  query: string
  onChoose: (name: string) => void
  onActivate?: (name: string) => void
  idPrefix?: string
}) {
  const sorted = sortFields(fields, freq, query)
  const groups = new Map<string, EventFieldDef[]>()
  for (const field of sorted) {
    const prefix = fieldPrefix(field.name)
    const list = groups.get(prefix) ?? []
    list.push(field)
    groups.set(prefix, list)
  }
  const prefixes = [...groups.keys()].sort((left, right) => {
    if (left === 'общее') return -1
    if (right === 'общее') return 1
    return left.localeCompare(right)
  })

  if (sorted.length === 0) {
    return <div className="px-3 py-4 text-xs text-fg-dim">Нет полей по запросу</div>
  }

  return (
    <div className="flex flex-col">
      {prefixes.map((prefix) => (
        <div key={prefix}>
          <div className="sticky top-0 z-10 bg-surface-1 px-3 py-1 text-[10px] font-semibold uppercase tracking-wider text-fg-dim">
            {prefix}
          </div>
          {(groups.get(prefix) ?? []).map((field) => (
            <FieldRow
              key={field.name}
              idPrefix={idPrefix}
              field={field}
              query={query}
              onChoose={onChoose}
              onActivate={onActivate}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

function FieldRow({
  field,
  query,
  onChoose,
  onActivate,
  idPrefix,
}: {
  field: EventFieldDef
  query: string
  onChoose: (name: string) => void
  onActivate?: (name: string) => void
  idPrefix: string
}) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `${idPrefix}:${field.name}`,
    data: { type: 'field', name: field.name },
  })
  return (
    <button
      ref={setNodeRef}
      type="button"
      {...attributes}
      {...listeners}
      onClick={() => onActivate?.(field.name)}
      onDoubleClick={() => onChoose(field.name)}
      className={clsx(
        'flex w-full flex-col items-start gap-0.5 px-3 py-1.5 text-left hover:bg-surface-2',
        isDragging && 'opacity-40',
      )}
      style={{ transform: CSS.Translate.toString(transform) }}
    >
      <span className="font-mono text-xs text-fg">{highlightMatch(field.name, query)}</span>
      <span className="text-[11px] text-fg-dim">{highlightMatch(field.description, query)}</span>
    </button>
  )
}
