import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ChevronDown, ChevronUp, X } from 'lucide-react'
import { operatorsForType, type CompareOp } from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'
import { SectionShell } from './SectionShell'
import { SortableRow } from './SortableRow'

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

const IP_RE = /^(\d{1,3}\.){3}\d{1,3}$/

export function FilterSection() {
  const query = usePdqlStore((s) => s.query)
  const fields = usePdqlStore((s) => s.fields)
  const updateCondition = usePdqlStore((s) => s.updateCondition)
  const removeCondition = usePdqlStore((s) => s.removeCondition)
  const setJoiner = usePdqlStore((s) => s.setJoiner)
  const moveCondition = usePdqlStore((s) => s.moveCondition)

  return (
    <SectionShell section="filter" title="Фильтр">
      {query.filter.length === 0 && (
        <div className="px-1 py-2 text-xs text-fg-dim">Нет условий. Добавьте поле из каталога.</div>
      )}
      <SortableContext items={query.filter.map((item) => item.id)} strategy={verticalListSortingStrategy}>
        {query.filter.map((condition, index) => {
          const field = fields.find((item) => item.name === condition.field)
          const type = field?.type ?? 'string'
          const ops = operatorsForType(type)
          const needsValue = condition.op !== 'is_null' && condition.op !== 'is_not_null'
          const ipInvalid =
            type === 'ip' &&
            needsValue &&
            condition.op !== 'in' &&
            condition.value.trim() !== '' &&
            !IP_RE.test(condition.value.trim())
          return (
            <div key={condition.id} className="flex flex-col gap-1">
              {index > 0 && (
                <button
                  type="button"
                  onClick={() => setJoiner(index - 1, query.joiners[index - 1] === 'and' ? 'or' : 'and')}
                  className="self-start rounded border border-border px-2 py-0.5 font-mono text-[11px] uppercase text-fg-muted hover:text-fg"
                >
                  {query.joiners[index - 1] ?? 'and'}
                </button>
              )}
              <SortableRow id={condition.id} section="filter" index={index}>
                <div className="flex flex-wrap items-center gap-1.5">
                  <button
                    type="button"
                    onClick={() => updateCondition(condition.id, { negated: !condition.negated })}
                    className={`rounded border px-1.5 py-0.5 text-[11px] ${
                      condition.negated
                        ? 'border-critical/40 bg-critical/10 text-critical'
                        : 'border-border text-fg-dim'
                    }`}
                  >
                    NOT
                  </button>
                  <span className="font-mono text-xs text-fg">{condition.field}</span>
                  <select
                    value={ops.includes(condition.op) ? condition.op : ops[0]}
                    onChange={(e) =>
                      updateCondition(condition.id, { op: e.target.value as CompareOp, value: '', values: [] })
                    }
                    className="rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg"
                  >
                    {ops.map((op) => (
                      <option key={op} value={op}>
                        {OP_LABELS[op]}
                      </option>
                    ))}
                  </select>
                  {needsValue && condition.op === 'in' && (
                    <input
                      value={condition.values.join(', ')}
                      onChange={(e) =>
                        updateCondition(condition.id, {
                          values: e.target.value
                            .split(',')
                            .map((item) => item.trim())
                            .filter(Boolean),
                        })
                      }
                      placeholder="a, b, c"
                      className="min-w-40 flex-1 rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg"
                    />
                  )}
                  {needsValue && condition.op !== 'in' && type === 'enum' && (
                    <select
                      value={condition.value}
                      onChange={(e) => updateCondition(condition.id, { value: e.target.value })}
                      className="min-w-32 rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg"
                    >
                      <option value="">—</option>
                      {(field?.enumValues ?? []).map((value) => (
                        <option key={value} value={value}>
                          {value}
                        </option>
                      ))}
                    </select>
                  )}
                  {needsValue && condition.op !== 'in' && type === 'number' && (
                    <input
                      type="number"
                      value={condition.value}
                      onChange={(e) => updateCondition(condition.id, { value: e.target.value })}
                      className="w-28 rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg"
                    />
                  )}
                  {needsValue && condition.op !== 'in' && type === 'datetime' && (
                    <input
                      type="datetime-local"
                      step={1}
                      value={condition.value}
                      onChange={(e) => updateCondition(condition.id, { value: e.target.value })}
                      className="rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg [color-scheme:dark]"
                    />
                  )}
                  {needsValue && condition.op !== 'in' && (type === 'string' || type === 'ip') && (
                    <input
                      value={condition.value}
                      onChange={(e) => updateCondition(condition.id, { value: e.target.value })}
                      className={`min-w-40 flex-1 rounded border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg ${
                        ipInvalid ? 'border-critical' : 'border-border'
                      }`}
                    />
                  )}
                  <div className="ml-auto flex items-center gap-0.5">
                    <Button size="sm" variant="ghost" title="Вверх" onClick={() => moveCondition(index, -1)}>
                      <ChevronUp className="h-3.5 w-3.5" />
                    </Button>
                    <Button size="sm" variant="ghost" title="Вниз" onClick={() => moveCondition(index, 1)}>
                      <ChevronDown className="h-3.5 w-3.5" />
                    </Button>
                    <Button size="sm" variant="ghost" title="Удалить" onClick={() => removeCondition(condition.id)}>
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
                {ipInvalid && <div className="text-[11px] text-critical">Ожидается IPv4</div>}
              </SortableRow>
            </div>
          )
        })}
      </SortableContext>
    </SectionShell>
  )
}
