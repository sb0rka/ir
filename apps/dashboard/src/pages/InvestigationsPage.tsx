import { useEffect, useState } from 'react'
import { Search, X } from 'lucide-react'
import { InvestigationsTable } from '../components/InvestigationsTable'
import { Button } from '../components/ui'
import { DEFAULT_INVESTIGATION_FILTER, useAppStore } from '../store/appStore'
import type { InvestigationListFilter } from '../types'
import { clsx } from '../lib/utils'

const SELECT_CLASS =
  'rounded border border-border bg-surface-0 px-2 py-1.5 text-xs text-fg outline-none focus:border-fg/40'

export function InvestigationsPage() {
  const filters = useAppStore((s) => s.investigationFilters)
  const setInvestigationFilter = useAppStore((s) => s.setInvestigationFilter)
  const rootIds = useAppStore((s) => s.investigationRootIds)
  const loading = useAppStore((s) => s.investigationsLoading)
  const [query, setQuery] = useState(filters.q)

  useEffect(() => {
    setQuery(filters.q)
  }, [filters.q])

  useEffect(() => {
    const next = query
    if (next === filters.q) return
    const timer = window.setTimeout(() => {
      void setInvestigationFilter({ q: query })
    }, 250)
    return () => window.clearTimeout(timer)
  }, [query, filters.q, setInvestigationFilter])

  const filtersActive =
    filters.status !== 'all' || filters.severity !== 'all' || filters.q.trim().length > 0

  const resetFilters = () => {
    setQuery('')
    void setInvestigationFilter(DEFAULT_INVESTIGATION_FILTER)
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-border bg-surface-1 px-3 py-2">
        <label className="relative min-w-56 flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-fg-dim" />
          <input
            className="w-full rounded border border-border bg-surface-0 py-1.5 pr-8 pl-7 text-sm outline-none focus:border-fg/40"
            placeholder="Поиск по названию"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          {query ? (
            <button
              type="button"
              className="absolute top-1/2 right-2 -translate-y-1/2 text-fg-dim hover:text-fg"
              title="Очистить поиск"
              onClick={() => setQuery('')}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          ) : null}
        </label>
        <select
          aria-label="Статус"
          className={SELECT_CLASS}
          value={filters.status}
          onChange={(event) =>
            void setInvestigationFilter({
              status: event.target.value as InvestigationListFilter['status'],
            })
          }
        >
          <option value="all">Все статусы</option>
          <option value="open">Открыто</option>
          <option value="in_progress">В работе</option>
          <option value="closed">Закрыто</option>
        </select>
        <select
          aria-label="Важность"
          className={SELECT_CLASS}
          value={filters.severity}
          onChange={(event) =>
            void setInvestigationFilter({
              severity: event.target.value as InvestigationListFilter['severity'],
            })
          }
        >
          <option value="all">Любая важность</option>
          <option value="critical">Критическая</option>
          <option value="high">Высокая</option>
          <option value="medium">Средняя</option>
          <option value="low">Низкая</option>
        </select>
        {filtersActive ? (
          <Button size="sm" variant="ghost" onClick={resetFilters}>
            Сбросить
          </Button>
        ) : null}
        <span className={clsx('ml-auto text-xs text-fg-dim', loading && 'opacity-60')}>
          {rootIds.length} в списке
        </span>
      </div>
      <div className="min-h-0 flex-1">
        {!loading && rootIds.length === 0 ? (
          <EmptyState filtersActive={filtersActive} onReset={resetFilters} />
        ) : (
          <InvestigationsTable />
        )}
      </div>
    </div>
  )
}

function EmptyState({
  filtersActive,
  onReset,
}: {
  filtersActive: boolean
  onReset: () => void
}) {
  if (filtersActive) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
        <div className="text-sm text-fg">Ничего не найдено</div>
        <div className="max-w-md text-xs text-fg-dim">
          Измените поиск или фильтры, чтобы увидеть другие расследования.
        </div>
        <Button size="sm" onClick={onReset}>
          Сбросить фильтры
        </Button>
      </div>
    )
  }
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
      <div className="text-sm text-fg">Нет расследований</div>
      <div className="max-w-md text-xs text-fg-dim">
        Откройте кейс из очереди: выберите события или находки и начните расследование.
      </div>
    </div>
  )
}
