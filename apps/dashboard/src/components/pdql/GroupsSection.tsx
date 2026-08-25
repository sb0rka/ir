import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp, X } from 'lucide-react'
import { AGGREGATES, groupCountColumn } from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'
import { SectionShell } from './SectionShell'
import { SortableRow } from './SortableRow'

export function GroupsSection() {
  const query = usePdqlStore((s) => s.query)
  const removeGroup = usePdqlStore((s) => s.removeGroup)
  const moveGroup = usePdqlStore((s) => s.moveGroup)
  const setGroupAggregate = usePdqlStore((s) => s.setGroupAggregate)
  const setGroupSort = usePdqlStore((s) => s.setGroupSort)
  const countCol = groupCountColumn(query)

  return (
    <SectionShell section="groups" title="Группировка">
      {query.groups.length === 0 && (
        <div className="px-1 py-2 text-xs text-fg-dim">
          Нет группировки. При добавлении поля события будут считаться через count().
        </div>
      )}
      <SortableContext items={query.groups.map((item) => item.id)} strategy={verticalListSortingStrategy}>
        {query.groups.map((group, index) => (
          <SortableRow key={group.id} id={group.id} section="groups" index={index}>
            <div className="flex flex-wrap items-center gap-1.5">
              <select
                value={countCol?.aggregate ?? 'count'}
                onChange={(e) => setGroupAggregate(e.target.value as (typeof AGGREGATES)[number])}
                className="rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg"
              >
                {AGGREGATES.map((fn) => (
                  <option key={fn} value={fn}>
                    {fn}
                  </option>
                ))}
              </select>
              <span className="font-mono text-xs text-fg">{group.field}</span>
              <button
                type="button"
                title="Сортировка"
                onClick={() => {
                  if (!countCol?.sort) {
                    setGroupSort({ dir: 'desc', priority: 99 })
                    return
                  }
                  if (countCol.sort.dir === 'desc') {
                    setGroupSort({ dir: 'asc', priority: countCol.sort.priority })
                    return
                  }
                  setGroupSort(undefined)
                }}
                className="inline-flex items-center gap-1 rounded border border-border px-1.5 py-0.5 text-[11px] text-fg-muted hover:text-fg"
              >
                {!countCol?.sort && <ArrowUpDown className="h-3 w-3" />}
                {countCol?.sort?.dir === 'desc' && <ArrowDown className="h-3 w-3" />}
                {countCol?.sort?.dir === 'asc' && <ArrowUp className="h-3 w-3" />}
                {countCol?.sort ? (countCol.sort.dir === 'desc' ? 'desc' : 'asc') : 'sort'}
              </button>
              <div className="ml-auto flex items-center gap-0.5">
                <Button size="sm" variant="ghost" title="Вверх" onClick={() => moveGroup(index, -1)}>
                  <ChevronUp className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="ghost" title="Вниз" onClick={() => moveGroup(index, 1)}>
                  <ChevronDown className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="ghost" title="Удалить" onClick={() => removeGroup(group.id)}>
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </SortableRow>
        ))}
      </SortableContext>
    </SectionShell>
  )
}
