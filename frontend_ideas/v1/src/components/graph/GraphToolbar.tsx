import type { ReactNode } from 'react'
import { Focus, RotateCcw } from 'lucide-react'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import {
  ALL_ENTITY_TYPES,
  ALL_SEVERITIES,
  SEVERITY_COLOR,
} from './constants'
import type { EdgeOrigin, EntityTypeCode, Severity } from './types'

export function GraphToolbar({ onFit }: { onFit: () => void }) {
  const {
    activeInvestigation,
    toggleEntityType,
    toggleSeverity,
    toggleEdgeOrigin,
    resetGraphFilters,
  } = useWorkspaceStore()

  if (!activeInvestigation) return null

  const filters = activeInvestigation.filters
  const entityTypes = new Set(filters.entityTypes)
  const severities = new Set(filters.severities)
  const edgeOrigins = new Set(filters.edgeOrigins)

  return (
    <header className="flex flex-wrap items-center gap-3 border-b border-[var(--border)] bg-[var(--bg-panel)] px-4 py-2.5">
      <div className="mr-2 min-w-0 flex-1">
        <div className="truncate text-sm font-semibold text-[var(--text)]">
          {activeInvestigation.title}
        </div>
        <div className="flex items-center gap-2 text-[10px] text-[var(--text-dim)]">
          <span
            className="rounded px-1.5 py-0.5 font-semibold uppercase tracking-wide"
            style={{
              color: SEVERITY_COLOR[activeInvestigation.severity],
              background: `color-mix(in srgb, ${SEVERITY_COLOR[activeInvestigation.severity]} 18%, transparent)`,
            }}
          >
            {activeInvestigation.severity}
          </span>
          <span>{activeInvestigation.agentStatus}</span>
          <span>·</span>
          <span>{activeInvestigation.id}</span>
        </div>
      </div>

      <FilterGroup label="Entity">
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

      <FilterGroup label="Severity">
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

      <FilterGroup label="Edges">
        {(['seed', 'expanded'] as EdgeOrigin[]).map((origin) => (
          <Chip
            key={origin}
            active={edgeOrigins.has(origin)}
            onClick={() => toggleEdgeOrigin(origin)}
          >
            {origin}
          </Chip>
        ))}
      </FilterGroup>

      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={onFit}
          className="inline-flex items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--bg-node)] px-2 py-1 text-[11px] text-[var(--text-muted)] hover:border-[var(--border-strong)] hover:text-[var(--text)]"
        >
          <Focus size={12} /> Fit
        </button>
        <button
          type="button"
          onClick={resetGraphFilters}
          className="inline-flex items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--bg-node)] px-2 py-1 text-[11px] text-[var(--text-muted)] hover:border-[var(--border-strong)] hover:text-[var(--text)]"
        >
          <RotateCcw size={12} /> Reset
        </button>
      </div>
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
    <div className="flex max-w-full flex-wrap items-center gap-1">
      <span className="mr-0.5 text-[10px] uppercase tracking-wide text-[var(--text-dim)]">
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
      className="rounded-md border px-1.5 py-0.5 text-[10px] capitalize transition-colors"
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
