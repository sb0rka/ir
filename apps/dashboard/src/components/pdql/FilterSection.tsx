import { useLayoutEffect, useMemo } from 'react'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ChevronDown, ChevronUp, X } from 'lucide-react'
import {
  isFilterGroup,
  operatorsForType,
  type CompareOp,
  type Condition,
  type FilterGroup,
  type FilterNode,
  type LogicalJoiner,
} from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'
import {
  DateTimeParts,
  defaultWorkingTimeZone,
  formatInstant,
  parseTimestamp,
} from '../time-interval'
import { Button } from '../ui'
import { FieldSearchPopover } from './FieldSearchPopover'
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

function FilterDatetimeInput({ id, value }: { id: string; value: string }) {
  const updateCondition = usePdqlStore((s) => s.updateCondition)
  const fallbackIso = usePdqlStore((s) => s.defaultDatetimeIso)
  const zone = useMemo(() => defaultWorkingTimeZone(), [])
  const iso = parseTimestamp(value, zone) ?? fallbackIso

  useLayoutEffect(() => {
    if (value.trim()) return
    updateCondition(id, { value: formatInstant(fallbackIso, zone) })
  }, [fallbackIso, id, updateCondition, value, zone])

  return (
    <div className="rounded border border-border bg-surface-1 px-1.5 py-0.5">
      <DateTimeParts
        size="sm"
        iso={iso}
        zone={zone}
        onCommit={(next) => updateCondition(id, { value: formatInstant(next, zone) })}
      />
    </div>
  )
}

function JoinerRow({
  parentId,
  joinerIndex,
  joiner,
}: {
  parentId: string | null
  joinerIndex: number
  joiner: LogicalJoiner
}) {
  const setJoiner = usePdqlStore((s) => s.setJoiner)
  const wrapAdjacent = usePdqlStore((s) => s.wrapAdjacent)
  return (
    <div className="flex items-center gap-1 self-start pl-7">
      <button
        type="button"
        onClick={() => setJoiner(parentId, joinerIndex, joiner === 'and' ? 'or' : 'and')}
        className="rounded border border-border px-2 py-0.5 font-mono text-[11px] uppercase text-fg-muted hover:text-fg"
      >
        {joiner}
      </button>
      <button
        type="button"
        title="Сгруппировать соседние условия"
        onClick={() => wrapAdjacent(parentId, joinerIndex)}
        className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-fg-dim hover:text-fg"
      >
        ()
      </button>
    </div>
  )
}

function ConditionRow({
  condition,
  parentId,
  index,
}: {
  condition: Condition
  parentId: string | null
  index: number
}) {
  const fields = usePdqlStore((s) => s.fields)
  const updateCondition = usePdqlStore((s) => s.updateCondition)
  const removeCondition = usePdqlStore((s) => s.removeCondition)
  const moveCondition = usePdqlStore((s) => s.moveCondition)
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
    <SortableRow id={condition.id} section="filter" index={index} parentId={parentId}>
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
          onChange={(e) => {
            const nextOp = e.target.value as CompareOp
            const prevWasIn = condition.op === 'in'
            const nextIsIn = nextOp === 'in'
            if (prevWasIn === nextIsIn) {
              updateCondition(condition.id, { op: nextOp })
              return
            }
            if (nextIsIn) {
              updateCondition(condition.id, {
                op: nextOp,
                values: condition.value.trim() ? [condition.value.trim()] : [],
                value: '',
              })
              return
            }
            updateCondition(condition.id, {
              op: nextOp,
              value: condition.values[0] ?? '',
              values: [],
            })
          }}
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
          <FilterDatetimeInput id={condition.id} value={condition.value} />
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
          <Button size="sm" variant="ghost" title="Вверх" onClick={() => moveCondition(parentId, index, -1)}>
            <ChevronUp className="h-3.5 w-3.5" />
          </Button>
          <Button size="sm" variant="ghost" title="Вниз" onClick={() => moveCondition(parentId, index, 1)}>
            <ChevronDown className="h-3.5 w-3.5" />
          </Button>
          <Button size="sm" variant="ghost" title="Удалить" onClick={() => removeCondition(condition.id)}>
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      {ipInvalid && <div className="text-[11px] text-critical">Ожидается IPv4</div>}
    </SortableRow>
  )
}

function GroupRow({
  group,
  parentId,
  index,
}: {
  group: FilterGroup
  parentId: string | null
  index: number
}) {
  const removeCondition = usePdqlStore((s) => s.removeCondition)
  const moveCondition = usePdqlStore((s) => s.moveCondition)
  const ungroup = usePdqlStore((s) => s.ungroup)
  const toggleGroupNegated = usePdqlStore((s) => s.toggleGroupNegated)
  return (
    <SortableRow id={group.id} section="filter" index={index} parentId={parentId}>
      <div className="rounded border border-dashed border-fg/30 bg-surface-2/40 p-1.5">
        <div className="mb-1 flex items-center gap-1">
          <button
            type="button"
            onClick={() => toggleGroupNegated(group.id)}
            className={`rounded border px-1.5 py-0.5 text-[11px] ${
              group.negated
                ? 'border-critical/40 bg-critical/10 text-critical'
                : 'border-border text-fg-dim'
            }`}
          >
            NOT
          </button>
          <span className="font-mono text-[11px] text-fg-dim">(</span>
          <div className="ml-auto flex items-center gap-0.5">
            <FieldSearchPopover section="filter" parentId={group.id} />
            <Button
              size="sm"
              variant="ghost"
              title="Разгруппировать"
              disabled={group.negated && group.children.length > 1}
              onClick={() => ungroup(group.id)}
            >
              разгр.
            </Button>
            <Button size="sm" variant="ghost" title="Вверх" onClick={() => moveCondition(parentId, index, -1)}>
              <ChevronUp className="h-3.5 w-3.5" />
            </Button>
            <Button size="sm" variant="ghost" title="Вниз" onClick={() => moveCondition(parentId, index, 1)}>
              <ChevronDown className="h-3.5 w-3.5" />
            </Button>
            <Button size="sm" variant="ghost" title="Удалить группу" onClick={() => removeCondition(group.id)}>
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
        <FilterNodeList nodes={group.children} joiners={group.joiners} parentId={group.id} />
      </div>
    </SortableRow>
  )
}

function FilterNodeList({
  nodes,
  joiners,
  parentId,
}: {
  nodes: FilterNode[]
  joiners: LogicalJoiner[]
  parentId: string | null
}) {
  return (
    <SortableContext items={nodes.map((item) => item.id)} strategy={verticalListSortingStrategy}>
      {nodes.map((node, index) => (
        <div key={node.id} className="flex flex-col gap-1">
          {index > 0 && (
            <JoinerRow
              parentId={parentId}
              joinerIndex={index - 1}
              joiner={joiners[index - 1] ?? 'and'}
            />
          )}
          {isFilterGroup(node) ? (
            <GroupRow group={node} parentId={parentId} index={index} />
          ) : (
            <ConditionRow condition={node} parentId={parentId} index={index} />
          )}
        </div>
      ))}
    </SortableContext>
  )
}

export function FilterSection() {
  const query = usePdqlStore((s) => s.query)

  return (
    <SectionShell section="filter" title="Фильтр">
      {query.filter.length === 0 && (
        <div className="px-1 py-2 text-xs text-fg-dim">Нет условий. Добавьте поле из каталога.</div>
      )}
      <FilterNodeList nodes={query.filter} joiners={query.joiners} parentId={null} />
    </SectionShell>
  )
}
