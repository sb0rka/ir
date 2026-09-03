import { useState } from 'react'
import { ChevronDown, ChevronRight, Loader2, Trash2 } from 'lucide-react'
import { useAppStore } from '../store/appStore'
import type { Investigation } from '../types'
import { Button, Chip, SeverityBadge } from './ui'
import { clsx, formatTime, statusLabel, verdictLabel } from '../lib/utils'
import { investigationMatchesText } from '../lib/investigationTextSearch'
import {
  INVESTIGATION_TABLE_SEARCH_COLUMNS,
  resolveInvestigationTableSearchColumn,
} from './investigationTableColumns'

const COL_FIT = 'w-px whitespace-nowrap'

function canExpand(inv: Investigation, loaded: string[] | undefined): boolean {
  if ((inv.counters?.children ?? 0) > 0) return true
  return (loaded?.length ?? 0) > 0
}

function InvestigationRow({
  investigation,
  depth,
}: {
  investigation: Investigation
  depth: number
}) {
  const expanded = useAppStore((s) => s.expandedInvestigationIds.includes(investigation.id))
  const loaded = useAppStore((s) => s.investigationChildren[investigation.id])
  const loadingChildren = useAppStore((s) =>
    Boolean(s.investigationChildrenLoading[investigation.id]),
  )
  const toggleExpand = useAppStore((s) => s.toggleInvestigationExpand)
  const openInvestigationTab = useAppStore((s) => s.openInvestigationTab)
  const deleteInvestigation = useAppStore((s) => s.deleteInvestigation)
  const deleting = useAppStore((s) => s.investigationDeletingId === investigation.id)
  const deleteBusy = useAppStore((s) => s.investigationDeletingId != null)
  const [confirming, setConfirming] = useState(false)
  const expand = canExpand(investigation, loaded)
  const childCount = investigation.counters?.children ?? 0

  return (
    <tr
      className="cursor-pointer border-b border-border/60 hover:bg-surface-2/60"
      onClick={() => openInvestigationTab(investigation.id)}
    >
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        {expand ? (
          <button
            type="button"
            className="text-fg-muted hover:text-fg"
            title={expanded ? 'Свернуть' : 'Развернуть'}
            onClick={(event) => {
              event.stopPropagation()
              void toggleExpand(investigation.id)
            }}
          >
            {loadingChildren ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : expanded ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
          </button>
        ) : (
          <span className="inline-block w-4" />
        )}
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2 font-mono text-xs text-fg-muted')}>
        {investigation.updatedAt ? formatTime(investigation.updatedAt) : '—'}
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <SeverityBadge severity={investigation.severity} />
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <div className="flex flex-wrap items-center gap-1.5">
          <Chip>{statusLabel[investigation.status] ?? investigation.status}</Chip>
          {investigation.verdict ? (
            <Chip>{verdictLabel[investigation.verdict] ?? investigation.verdict}</Chip>
          ) : null}
        </div>
      </td>
      <td className="min-w-0 px-3 py-2">
        <div className="flex min-w-0 items-start gap-2" style={{ paddingLeft: depth * 16 }}>
          {depth > 0 && <span className="mt-0.5 text-fg-dim">↳</span>}
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">{investigation.title}</div>
            {investigation.description ? (
              <div className="mt-0.5 truncate text-xs text-fg-dim">{investigation.description}</div>
            ) : null}
          </div>
        </div>
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <div
          className="flex items-center justify-end gap-1"
          onClick={(event) => event.stopPropagation()}
        >
          {confirming ? (
            <>
              <Button
                size="sm"
                variant="danger"
                disabled={deleteBusy}
                onClick={() => void deleteInvestigation(investigation.id)}
              >
                {deleting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
                {childCount > 0 ? 'Удалить с дочерними' : 'Удалить'}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                disabled={deleting}
                onClick={() => setConfirming(false)}
              >
                Отмена
              </Button>
            </>
          ) : (
            <Button
              size="icon"
              variant="ghost"
              className="h-7 w-7 text-fg-dim hover:text-critical"
              title="Удалить"
              disabled={deleteBusy}
              onClick={() => setConfirming(true)}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>
      </td>
    </tr>
  )
}

function collectRows(
  ids: string[],
  investigations: Record<string, Investigation>,
  expanded: string[],
  children: Record<string, string[]>,
  depth: number,
): { id: string; depth: number }[] {
  const rows: { id: string; depth: number }[] = []
  for (const id of ids) {
    if (!investigations[id]) continue
    rows.push({ id, depth })
    if (!expanded.includes(id)) continue
    rows.push(
      ...collectRows(children[id] ?? [], investigations, expanded, children, depth + 1),
    )
  }
  return rows
}

export function InvestigationsTable() {
  const investigations = useAppStore((s) => s.investigations)
  const rootIds = useAppStore((s) => s.investigationRootIds)
  const expanded = useAppStore((s) => s.expandedInvestigationIds)
  const children = useAppStore((s) => s.investigationChildren)
  const loading = useAppStore((s) => s.investigationsLoading)
  const cursor = useAppStore((s) => s.investigationsNextCursor)
  const loadInvestigationList = useAppStore((s) => s.loadInvestigationList)
  const textNeedle = useAppStore((s) => s.investigationFilters.q.trim().toLowerCase())
  const searchColumn = useAppStore((s) =>
    resolveInvestigationTableSearchColumn(s.investigationTextFilterColumn),
  )

  const rows = collectRows(rootIds, investigations, expanded, children, 0).filter((row) => {
    const inv = investigations[row.id]
    if (!inv) return false
    return investigationMatchesText(inv, textNeedle, searchColumn)
  })

  if (loading && rootIds.length === 0) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-fg-dim">
        <Loader2 className="h-4 w-4 animate-spin" />
        Загрузка расследований…
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full border-collapse text-left">
          <thead className="sticky top-0 z-10 bg-surface-1 text-[10px] uppercase tracking-wider text-fg-dim">
            <tr className="border-b border-border">
              <th className={clsx(COL_FIT, 'px-3 py-2')} />
              {INVESTIGATION_TABLE_SEARCH_COLUMNS.map((column) => (
                <th
                  key={column.id}
                  className={clsx(
                    column.id === 'title' ? 'px-3 py-2' : clsx(COL_FIT, 'px-3 py-2'),
                  )}
                >
                  {column.label}
                </th>
              ))}
              <th className={clsx(COL_FIT, 'px-3 py-2')} />
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const inv = investigations[row.id]
              if (!inv) return null
              return <InvestigationRow key={row.id} investigation={inv} depth={row.depth} />
            })}
          </tbody>
        </table>
      </div>
      {cursor ? (
        <div className="border-t border-border px-3 py-2">
          <Button
            size="sm"
            disabled={loading}
            onClick={() => void loadInvestigationList(false)}
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
            Загрузить ещё
          </Button>
        </div>
      ) : null}
    </div>
  )
}
