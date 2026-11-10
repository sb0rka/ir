import { useMemo, useState } from 'react'
import { Search, X } from 'lucide-react'
import { emptyContextQueue, useAppStore } from '../store/appStore'
import { hasGroupValueSelection, parseQueuePdql } from '../lib/pdql'
import { describeEventQuery, emptyEventQuery } from '../lib/eventQuery'
import { collapseRuleLevelGroups, ruleLevelRowSelected } from '../lib/ruleLevel'
import { clsx } from '../lib/utils'
import { Button } from './ui'

export { ruleLevelSeverity } from '../lib/ruleLevel'

export function EventGroupFilter({ investigationId }: { investigationId?: string } = {}) {
  const [query, setQuery] = useState('')
  const globalPdql = useAppStore((s) => s.queuePdql)
  const globalEventQuery = useAppStore((s) => s.eventQuery)
  const globalSource = useAppStore((s) => s.queueSource)
  const globalGroups = useAppStore((s) => s.eventGroups)
  const globalValues = useAppStore((s) => s.groupValues)
  const globalLoading = useAppStore((s) => s.queueLoading)
  const queue = useAppStore((s) =>
    investigationId ? (s.contextQueue[investigationId] ?? emptyContextQueue) : null,
  )
  const selectGroupValue = useAppStore((s) => s.selectGroupValue)
  const clearGroupSelection = useAppStore((s) => s.clearGroupSelection)

  const pdql = queue?.pdql ?? globalPdql
  const eventQuery = investigationId ? (queue?.eventQuery ?? emptyEventQuery()) : globalEventQuery
  const queueSource = queue?.queueSource ?? globalSource
  const eventGroups = queue?.eventGroups ?? globalGroups
  const groupValues = queue?.groupValues ?? globalValues
  const loading = queue?.loading ?? globalLoading
  const parsed = parseQueuePdql(pdql)
  const groupField =
    queueSource === 'events'
      ? describeEventQuery(eventQuery).aggregateGroupBy?.[0]
      : parsed.ok
        ? parsed.ast.groups[0]?.field
        : undefined

  const visible = queueSource === 'events' && Boolean(groupField)
  const rows = useMemo(() => collapseRuleLevelGroups(groupField, eventGroups), [eventGroups, groupField])
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return rows
    return rows.filter((row) => `${row.label} ${row.annotation}`.toLowerCase().includes(needle))
  }, [query, rows])
  const maxCount = filtered.reduce((max, row) => Math.max(max, row.count), 0)
  const selected = hasGroupValueSelection(groupValues)

  if (!visible) return null

  return (
    <div className="flex h-full w-72 shrink-0 flex-col border-r border-border bg-surface-1">
      <div className="flex items-center justify-between border-b border-border px-3 py-2">
        <div className="min-w-0">
          <div className="text-xs font-semibold uppercase tracking-wider text-fg-muted">Группы</div>
          <div className="truncate font-mono text-[11px] text-fg-dim" title={groupField}>
            {groupField}
          </div>
        </div>
        {selected && (
          <Button
            size="icon"
            variant="ghost"
            title="Сбросить"
            aria-label="Сбросить"
            onClick={() => clearGroupSelection(investigationId ?? null)}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
      <label className="flex items-center gap-1.5 border-b border-border px-3 py-1.5">
        <Search className="h-3.5 w-3.5 shrink-0 text-fg-dim" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Найти значение"
          className="w-full bg-transparent text-xs text-fg outline-none placeholder:text-fg-dim"
        />
      </label>
      <div className="flex-1 overflow-auto">
        {loading && eventGroups.length === 0 && (
          <div className="space-y-2 p-3">
            {Array.from({ length: 6 }, (_, i) => (
              <div key={i} className="h-10 animate-pulse rounded bg-surface-2" />
            ))}
          </div>
        )}
        {!loading && eventGroups.length === 0 && (
          <div className="px-3 py-8 text-center text-xs text-fg-dim">Нет групп по текущему запросу</div>
        )}
        {filtered.map((row) => {
          const active = ruleLevelRowSelected(row, groupValues)
          const width = maxCount > 0 ? Math.max(4, (row.count / maxCount) * 100) : 0
          return (
            <button
              key={row.key}
              type="button"
              title={row.annotation ? `${row.label} ${row.annotation}` : row.label}
              onClick={() =>
                selectGroupValue(investigationId ?? null, active ? (groupValues[0] ?? null) : row.selectValue)
              }
              className={clsx(
                'relative flex w-full flex-col gap-1 border-b border-border/60 px-3 py-2 text-left hover:bg-surface-2/80',
                active && 'bg-surface-3/70',
              )}
            >
              <span
                className="absolute inset-y-0 left-0 bg-fg/5"
                style={{ width: `${width}%` }}
              />
              <span className="relative flex items-center justify-between gap-2">
                <span
                  className={clsx(
                    'min-w-0 truncate font-mono text-xs',
                    row.selectValue == null ? 'italic text-fg-dim' : 'text-fg',
                  )}
                >
                  {row.label}
                  {row.annotation && (
                    <span className="ml-2 font-sans text-[10px] text-fg-dim">{row.annotation}</span>
                  )}
                </span>
                <span className="shrink-0 font-mono text-[11px] tabular-nums text-fg-muted">
                  {row.count.toLocaleString('ru-RU')}
                </span>
              </span>
            </button>
          )
        })}
        {eventGroups.length > 0 && filtered.length === 0 && (
          <div className="px-3 py-8 text-center text-xs text-fg-dim">Ничего не найдено</div>
        )}
      </div>
      <div className="border-t border-border px-3 py-1.5 text-[11px] text-fg-dim">
        {rows.length} групп
        {loading && eventGroups.length > 0 ? ' · обновление…' : ''}
      </div>
    </div>
  )
}
