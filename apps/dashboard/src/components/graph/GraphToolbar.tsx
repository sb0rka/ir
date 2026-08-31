import type { ReactNode } from 'react'
import { Lightbulb, RotateCcw } from 'lucide-react'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import { useAppStore } from '../../store/appStore'
import {
  ALL_ENTITY_TYPES,
  DEFAULT_ENTITY_TYPES,
  ALL_SEVERITIES,
  SEVERITY_COLOR,
} from './constants'
import { toMs } from './time'
import type { EdgeOrigin, EntityTypeCode, Severity } from './types'

function sameEntityTypes(active: Set<string>, defaults: readonly string[]): boolean {
  if (active.size !== defaults.length) return false
  return defaults.every((type) => active.has(type))
}

/** Compact filter strip above the graph. Investigation identity lives in the page header. */
export function GraphToolbar() {
  const {
    activeInvestigation,
    toggleEntityType,
    toggleSeverity,
    toggleEdgeOrigin,
    resetGraphFilters,
  } = useWorkspaceStore()
  const investigationId = activeInvestigation?.id
  const activeHypothesis = useAppStore((s) => {
    if (!investigationId) return null
    const id = s.activeHypothesisId[investigationId]
    return id ? s.hypotheses[id] ?? null : null
  })
  const selectedCount = useAppStore((s) =>
    investigationId ? (s.investigations[investigationId]?.selectedEntityIds.length ?? 0) : 0,
  )
  const addSelection = useAppStore((s) => s.addSelectionToHypothesis)
  const setActiveHypothesis = useAppStore((s) => s.setActiveHypothesis)

  if (!activeInvestigation) return null

  const filters = activeInvestigation.filters
  const entityTypes = new Set(filters.entityTypes)
  const severities = new Set(filters.severities)
  const edgeOrigins = new Set(filters.edgeOrigins)

  const fullTimeRange =
    !filters.timeRange ||
    (filters.timeRange.start <= toMs(activeInvestigation.windowStart) &&
      filters.timeRange.end >= toMs(activeInvestigation.windowEnd))
  const filtered =
    !sameEntityTypes(entityTypes, DEFAULT_ENTITY_TYPES) ||
    severities.size !== ALL_SEVERITIES.length ||
    edgeOrigins.size !== 2 ||
    !fullTimeRange

  return (
    <header className="flex flex-wrap items-center gap-x-4 gap-y-1.5 border-b border-[var(--border)] bg-[var(--bg-panel)] px-4 py-2">
      <FilterGroup label="Сущности">
        {ALL_ENTITY_TYPES.map((type) => (
          <Chip
            key={type}
            active={entityTypes.has(type)}
            onClick={() => toggleEntityType(type as EntityTypeCode)}
          >
            {type}
          </Chip>
        ))}
      </FilterGroup>

      <FilterGroup label="Критичность">
        {ALL_SEVERITIES.map((sev) => (
          <Chip
            key={sev}
            active={severities.has(sev)}
            onClick={() => toggleSeverity(sev as Severity)}
            accent={SEVERITY_COLOR[sev]}
          >
            {sev}
          </Chip>
        ))}
      </FilterGroup>

      <FilterGroup label="Происхождение">
        {(['agent', 'analyst'] as EdgeOrigin[]).map((origin) => (
          <Chip
            key={origin}
            active={edgeOrigins.has(origin)}
            onClick={() => toggleEdgeOrigin(origin)}
          >
            {origin === 'agent' ? 'агент' : 'аналитик'}
          </Chip>
        ))}
      </FilterGroup>

      {activeHypothesis && investigationId && (
        <FilterGroup label="Гипотеза">
          <span className="max-w-[220px] truncate rounded-md border border-[var(--border-strong)] bg-[var(--accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--accent)]">
            H: {activeHypothesis.statement}
          </span>
          {activeHypothesis.status !== 'resolved' && selectedCount > 0 && (
            <button
              type="button"
              onClick={() => void addSelection(investigationId, activeHypothesis.id)}
              className="inline-flex items-center gap-1 rounded-md border border-[var(--border)] px-1.5 py-0.5 text-[10px] text-[var(--text)] hover:border-[var(--border-strong)]"
            >
              <Lightbulb size={10} /> В гипотезу ({selectedCount})
            </button>
          )}
          <button
            type="button"
            onClick={() => void setActiveHypothesis(investigationId, null)}
            className="rounded-md px-1 text-[11px] text-[var(--text-muted)] hover:text-[var(--text)]"
            title="Снять линзу"
          >
            ×
          </button>
        </FilterGroup>
      )}

      <button
        type="button"
        onClick={resetGraphFilters}
        disabled={!filtered}
        className="ml-auto inline-flex items-center gap-1 rounded-md border border-[var(--border)] bg-transparent px-2 py-1 text-[11px] text-[var(--text-muted)] transition-colors hover:border-[var(--border-strong)] hover:text-[var(--text)] disabled:opacity-35"
      >
        <RotateCcw size={12} /> Сбросить фильтры
      </button>
    </header>
  )
}

function FilterGroup({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className="flex items-center gap-1">
      <span className="mr-1 text-[10px] uppercase tracking-wide text-[var(--text-dim)]">
        {label}
      </span>
      {children}
    </div>
  )
}

function Chip({
  active,
  onClick,
  children,
  accent,
}: {
  active: boolean
  onClick: () => void
  children: ReactNode
  accent?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded-md border px-1.5 py-0.5 text-[10px] transition-colors"
      style={{
        borderColor: active
          ? (accent ?? 'var(--border-strong)')
          : 'var(--border)',
        background: active
          ? accent
            ? `color-mix(in srgb, ${accent} 18%, transparent)`
            : 'var(--accent-soft)'
          : 'transparent',
        color: active ? (accent ?? 'var(--accent)') : 'var(--text-dim)',
        opacity: active ? 1 : 0.55,
      }}
    >
      {children}
    </button>
  )
}
