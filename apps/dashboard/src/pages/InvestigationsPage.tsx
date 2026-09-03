import { InvestigationsTable } from '../components/InvestigationsTable'
import { Button, Select } from '../components/ui'
import { DEFAULT_INVESTIGATION_FILTER, useAppStore } from '../store/appStore'
import type { InvestigationListFilter } from '../types'
import { clsx } from '../lib/utils'
import { X } from 'lucide-react'

const STATUS_OPTIONS = [
  { value: 'all', label: 'Все статусы' },
  { value: 'open', label: 'Открыто' },
  { value: 'in_progress', label: 'В работе' },
  { value: 'closed', label: 'Закрыто' },
] as const satisfies ReadonlyArray<{
  value: InvestigationListFilter['status']
  label: string
}>

const SEVERITY_OPTIONS = [
  { value: 'all', label: 'Любая важность' },
  { value: 'critical', label: 'Критическая' },
  { value: 'high', label: 'Высокая' },
  { value: 'medium', label: 'Средняя' },
  { value: 'low', label: 'Низкая' },
] as const satisfies ReadonlyArray<{
  value: InvestigationListFilter['severity']
  label: string
}>

export function InvestigationsPage() {
  const filters = useAppStore((s) => s.investigationFilters)
  const setInvestigationFilter = useAppStore((s) => s.setInvestigationFilter)
  const rootIds = useAppStore((s) => s.investigationRootIds)
  const loading = useAppStore((s) => s.investigationsLoading)

  const filtersActive =
    filters.status !== 'all' || filters.severity !== 'all' || filters.q.trim().length > 0

  const resetFilters = () => {
    void setInvestigationFilter(DEFAULT_INVESTIGATION_FILTER)
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-surface-1 px-5 py-2">
        <span className={clsx('shrink-0 text-xs text-fg-dim', loading && 'opacity-60')}>
          {rootIds.length} в списке
        </span>
        <div className="flex flex-wrap items-center gap-2">
          {filtersActive ? (
            <Button
              size="icon"
              variant="ghost"
              title="Сбросить"
              aria-label="Сбросить"
              onClick={resetFilters}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          ) : null}
          <Select
            aria-label="Статус"
            value={filters.status}
            options={STATUS_OPTIONS}
            onChange={(status) => void setInvestigationFilter({ status })}
          />
          <Select
            aria-label="Важность"
            value={filters.severity}
            options={SEVERITY_OPTIONS}
            onChange={(severity) => void setInvestigationFilter({ severity })}
          />
        </div>
      </div>
      <div className="min-h-0 flex-1">
        {!loading && rootIds.length === 0 ? (
          <div className="flex h-full items-center justify-center px-4 text-center">
            <div>
              <div className="text-sm text-fg-muted">Нет расследований</div>
              <div className="mt-1 text-xs text-fg-dim">
                Выберите находки и начните разбор
              </div>
            </div>
          </div>
        ) : (
          <InvestigationsTable />
        )}
      </div>
    </div>
  )
}
