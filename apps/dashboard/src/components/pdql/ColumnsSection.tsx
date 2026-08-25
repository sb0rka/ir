import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronDown, ChevronUp, X } from 'lucide-react'
import { AGGREGATES, isGroupCountColumn, isGroupDimensionColumn } from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'
import { SectionShell } from './SectionShell'
import { SortableRow } from './SortableRow'

export function ColumnsSection() {
  const query = usePdqlStore((s) => s.query)
  const removeColumn = usePdqlStore((s) => s.removeColumn)
  const setColumnAggregate = usePdqlStore((s) => s.setColumnAggregate)
  const setColumnSort = usePdqlStore((s) => s.setColumnSort)
  const moveColumn = usePdqlStore((s) => s.moveColumn)
  const grouped = query.groups.length > 0
  const visibleColumns = query.columns
    .map((column, index) => ({ column, index }))
    .filter(({ column }) => !isGroupDimensionColumn(query, column) && !isGroupCountColumn(column))

  return (
    <SectionShell section="columns" title="Колонки">
      {visibleColumns.length === 0 && (
        <div className="px-1 py-2 text-xs text-fg-dim">Нет колонок. Добавьте поле из каталога.</div>
      )}
      <SortableContext items={visibleColumns.map(({ column }) => column.id)} strategy={verticalListSortingStrategy}>
        {visibleColumns.map(({ column, index }) => {
          const needsAggregate = grouped
          return (
            <SortableRow key={column.id} id={column.id} section="columns" index={index}>
              <div className="flex flex-wrap items-center gap-1.5">
                {needsAggregate && (
                  <select
                    value={column.aggregate ?? 'count'}
                    onChange={(e) => setColumnAggregate(column.id, e.target.value as (typeof AGGREGATES)[number])}
                    className="rounded border border-border bg-surface-1 px-1.5 py-0.5 font-mono text-[11px] text-fg"
                  >
                    {AGGREGATES.map((fn) => (
                      <option key={fn} value={fn}>
                        {fn}
                      </option>
                    ))}
                  </select>
                )}
                <span className="font-mono text-xs text-fg">{column.field || '*'}</span>
                <button
                  type="button"
                  title="Сортировка"
                  onClick={() => {
                    if (!column.sort) {
                      setColumnSort(column.id, { dir: 'desc', priority: 99 })
                      return
                    }
                    if (column.sort.dir === 'desc') {
                      setColumnSort(column.id, { dir: 'asc', priority: column.sort.priority })
                      return
                    }
                    setColumnSort(column.id, undefined)
                  }}
                  className="inline-flex items-center gap-1 rounded border border-border px-1.5 py-0.5 text-[11px] text-fg-muted hover:text-fg"
                >
                  {!column.sort && <ArrowUpDown className="h-3 w-3" />}
                  {column.sort?.dir === 'desc' && <ArrowDown className="h-3 w-3" />}
                  {column.sort?.dir === 'asc' && <ArrowUp className="h-3 w-3" />}
                  {column.sort ? (column.sort.dir === 'desc' ? 'desc' : 'asc') : 'sort'}
                </button>
                <div className="ml-auto flex items-center gap-0.5">
                  <Button size="sm" variant="ghost" title="Вверх" onClick={() => moveColumn(index, -1)}>
                    <ChevronUp className="h-3.5 w-3.5" />
                  </Button>
                  <Button size="sm" variant="ghost" title="Вниз" onClick={() => moveColumn(index, 1)}>
                    <ChevronDown className="h-3.5 w-3.5" />
                  </Button>
                  <Button size="sm" variant="ghost" title="Удалить" onClick={() => removeColumn(column.id)}>
                    <X className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            </SortableRow>
          )
        })}
      </SortableContext>
    </SectionShell>
  )
}
