import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useAppStore, emptyContextQueue } from '../store/appStore'
import type { AlertEvent, CorrelationGroup, Entity, QueueItem } from '../types'
import { Button, Chip, SeverityBadge } from './ui'
import { clsx, formatTime, kindLabel } from '../lib/utils'
import { hasGroupValueSelection, incidentTypeLabelRu, parseQueuePdql, queueSelectFields } from '../lib/pdql'
import { alertIsInContext, contextEventKeys } from '../lib/queueContext'
import {
  alertMatchesQueueText,
  correlationMatchesQueueText,
} from '../lib/queueTextSearch'
import {
  alertTableColumnLabel,
  alertTableSearchColumns,
  resolveAlertTableSearchColumn,
  type AlertTableColumnId,
} from './alertTableColumns'
import { ChevronDown, ChevronRight, Layers, Plus } from 'lucide-react'

const COL_FIT = 'max-w-0 overflow-hidden whitespace-nowrap'
const COL_TITLE = 'min-w-0 max-w-0 overflow-hidden'
const TABLE_CLASS = 'border-collapse table-fixed text-left'

const COL_WIDTHS_STORAGE_KEY = 'ir.alertTable.colWidths'
const MIN_COL_WIDTH = 48
const MIN_TITLE_WIDTH = 120

const DEFAULT_COL_WIDTHS: Record<string, number> = {
  select: 40,
  severity: 96,
  time: 148,
  title: 360,
  category: 280,
  source: 160,
  actions: 48,
}

function fieldColKey(field: string) {
  return `field:${field}`
}

function alertTableColKeys(
  selectFields: string[],
  hasActions: boolean,
  showCategory: boolean,
): string[] {
  return [
    'select',
    'severity',
    'time',
    'title',
    ...(showCategory ? ['category'] : []),
    ...selectFields.map(fieldColKey),
    'source',
    ...(hasActions ? ['actions'] : []),
  ]
}

function defaultWidthForCol(key: string): number {
  if (key.startsWith('field:')) return 144
  return DEFAULT_COL_WIDTHS[key] ?? 120
}

function minWidthForCol(key: string): number {
  if (key === 'title') return MIN_TITLE_WIDTH
  if (key === 'select' || key === 'actions') return 36
  return MIN_COL_WIDTH
}

function loadStoredColWidths(): Record<string, number> {
  try {
    const raw = localStorage.getItem(COL_WIDTHS_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const out: Record<string, number> = {}
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value === 'number' && Number.isFinite(value) && value >= 24) {
        out[key] = value
      }
    }
    return out
  } catch {
    return {}
  }
}

function ColumnResizeHandle({
  leftWidth,
  rightWidth,
  leftMin,
  rightMin,
  onWidthsChange,
  onWidthsCommit,
}: {
  leftWidth: number
  rightWidth: number
  leftMin: number
  rightMin: number
  onWidthsChange: (left: number, right: number) => void
  onWidthsCommit: (left: number, right: number) => void
}) {
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Изменить ширину колонки"
      title="Потяните, чтобы изменить ширину"
      className="group/resize absolute top-0 right-0 z-20 flex h-full w-2 translate-x-1/2 cursor-col-resize touch-none select-none items-center justify-center"
      onPointerDown={(e) => {
        e.preventDefault()
        e.stopPropagation()
        const startX = e.clientX
        const startLeft = leftWidth
        const startRight = rightWidth
        let latestLeft = startLeft
        let latestRight = startRight

        const onMove = (ev: PointerEvent) => {
          const rawDelta = Math.round(ev.clientX - startX)
          const delta = Math.max(leftMin - startLeft, Math.min(startRight - rightMin, rawDelta))
          latestLeft = startLeft + delta
          latestRight = startRight - delta
          onWidthsChange(latestLeft, latestRight)
        }
        const onUp = () => {
          onWidthsCommit(latestLeft, latestRight)
          window.removeEventListener('pointermove', onMove)
          window.removeEventListener('pointerup', onUp)
          window.removeEventListener('pointercancel', onUp)
        }

        window.addEventListener('pointermove', onMove)
        window.addEventListener('pointerup', onUp)
        window.addEventListener('pointercancel', onUp)
      }}
    >
      <div className="h-full w-px bg-transparent transition-colors group-hover/resize:bg-border-strong group-active/resize:bg-fg/50" />
    </div>
  )
}

function isInspected(item: QueueItem | null, kind: QueueItem['kind'], id: string) {
  return item?.kind === kind && item.id === id
}

function queueFieldValue(alert: AlertEvent, field: string): string {
  if (field === 'time') return alert.time
  return alert.raw?.[field] ?? ''
}

function formatQueueFieldValue(field: string, value: string): string {
  if (!value) return value
  if (
    field === 'time' ||
    field === 'original_time' ||
    field.endsWith('_time') ||
    field.endsWith('.time')
  ) {
    const parsed = Date.parse(value)
    if (!Number.isNaN(parsed)) {
      return new Date(parsed).toLocaleString('ru-RU', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      })
    }
  }
  return value
}

function SelectFieldCell({
  field,
  value,
  investigationId,
}: {
  field: string
  value: string
  investigationId?: string
}) {
  const appendPdqlFilter = useAppStore((s) => s.appendPdqlFilter)
  const display = formatQueueFieldValue(field, value)
  return (
    <td className={clsx(COL_FIT, 'px-3 py-2')}>
      {value ? (
        <button
          type="button"
          title={display}
          className="block w-full truncate text-left font-mono text-xs text-fg hover:underline"
          onClick={(ev) => {
            ev.stopPropagation()
            appendPdqlFilter(investigationId ?? null, field, value)
          }}
        >
          {display}
        </button>
      ) : (
        <span className="text-fg-dim">&nbsp;</span>
      )}
    </td>
  )
}

function EntityRow({ entity }: { entity: Entity | undefined }) {
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const inspected = useAppStore((s) =>
    entity ? isInspected(s.inspectedQueueItem, 'entity', entity.id) : false,
  )
  if (!entity) return null
  return (
    <tr
      className={clsx(
        'cursor-pointer border-b border-border/60 hover:bg-surface-2/60',
        inspected && 'bg-surface-3/70',
      )}
      onClick={() => inspect({ kind: 'entity', id: entity.id })}
    >
      <td className={clsx(COL_FIT, 'px-3 py-2')} />
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <span className="text-[11px] uppercase tracking-wider text-fg-dim">
          {kindLabel[entity.kind] ?? entity.kind}
        </span>
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2 text-fg-dim')}>—</td>
      <td className={clsx(COL_TITLE, 'px-3 py-2')}>
        <span className="block truncate font-mono text-sm text-fg" title={entity.label}>
          {entity.label}
        </span>
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2')} title={entity.source}>
        <span className="block truncate font-mono text-[11px] text-fg-muted">
          {entity.source ?? '—'}
        </span>
      </td>
    </tr>
  )
}

function AlertRow({
  alert,
  nested,
  investigationId,
  inContext,
  selected,
  selectFields,
  showCategory,
  onToggle,
}: {
  alert: AlertEvent
  nested?: boolean
  investigationId?: string
  inContext?: boolean
  selected: boolean
  selectFields: string[]
  showCategory: boolean
  onToggle: () => void
}) {
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const addEventsToContext = useAppStore((s) => s.addEventsToContext)
  const inspected = useAppStore((s) => isInspected(s.inspectedQueueItem, 'alert', alert.id))
  const categoryCode = alert.raw?.['incident.type'] ?? ''
  const categoryLabel = incidentTypeLabelRu(categoryCode)

  return (
    <tr
      className={clsx(
        'cursor-pointer border-b border-border/60 hover:bg-surface-2/60',
        inContext && 'bg-surface-0/40',
        selected && 'bg-surface-2',
        inspected && 'bg-surface-3/70',
        nested && !inspected && 'bg-surface-0/40',
      )}
      onClick={() => inspect({ kind: 'alert', id: alert.id })}
    >
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <input
          type="checkbox"
          checked={selected}
          disabled={Boolean(inContext)}
          onChange={() => onToggle()}
          onClick={(ev) => ev.stopPropagation()}
          className="accent-fg"
        />
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <div className="truncate">
          <SeverityBadge severity={alert.severity} />
        </div>
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2 font-mono text-xs text-fg-muted')} title={formatTime(alert.time)}>
        <span className="block truncate">{formatTime(alert.time)}</span>
      </td>
      <td className={clsx(COL_TITLE, 'px-3 py-2')}>
        <div className="flex min-w-0 items-center gap-2">
          <div
            className={clsx('min-w-0 flex-1 truncate text-sm', inContext && 'text-fg-muted')}
            title={alert.title}
          >
            {alert.title}
          </div>
          {inContext && (
            <Chip tone="confirmed">в контексте</Chip>
          )}
        </div>
        <div className="truncate text-xs text-fg-dim" title={alert.rule}>
          {alert.rule}
        </div>
      </td>
      {showCategory && (
        <td className="max-w-0 overflow-hidden px-3 py-2 align-top" title={categoryLabel || categoryCode}>
          <span className="line-clamp-2 text-xs leading-snug text-fg-muted">
            {categoryLabel || <span className="text-fg-dim">&nbsp;</span>}
          </span>
        </td>
      )}
      {selectFields.map((field) => (
        <SelectFieldCell
          key={field}
          field={field}
          value={queueFieldValue(alert, field)}
          investigationId={investigationId}
        />
      ))}
      <td className={clsx(COL_FIT, 'px-3 py-2')} title={alert.source}>
        <span className="block truncate font-mono text-[11px] text-fg-muted">
          {alert.source}
        </span>
      </td>
      {investigationId && (
        <td className={clsx(COL_FIT, 'px-3 py-2')}>
          {!inContext && (
            <Button
              size="sm"
              variant="ghost"
              title="Добавить в расследование"
              onClick={(e) => {
                e.stopPropagation()
                void addEventsToContext(investigationId, [alert.id])
              }}
            >
              <Plus className="h-3 w-3" />
            </Button>
          )}
        </td>
      )}
    </tr>
  )
}

function CorrelationRow({
  group,
  alerts,
  selectFields,
  showCategory,
}: {
  group: CorrelationGroup
  alerts: Record<string, AlertEvent>
  selectFields: string[]
  showCategory: boolean
}) {
  const expanded = useAppStore((s) => s.expandedCorrelationIds.includes(group.id))
  const toggleExpand = useAppStore((s) => s.toggleCorrelationExpand)
  const selected = useAppStore((s) => s.selectedAlertIds.includes(group.id))
  const toggle = useAppStore((s) => s.toggleAlertSelect)
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const inspected = useAppStore((s) => isInspected(s.inspectedQueueItem, 'correlation', group.id))
  const eventCount = group.eventIds.length
  const sourceCount = Object.keys(group.sourceCounts).length

  return (
    <>
      <tr
        className={clsx(
          'cursor-pointer border-b border-border bg-surface-2/40 hover:bg-surface-2',
          selected && 'bg-surface-3/50',
          inspected && 'bg-surface-3/80',
        )}
        onClick={() => inspect({ kind: 'correlation', id: group.id })}
      >
        <td className={clsx(COL_FIT, 'px-3 py-2.5')}>
          <input
            type="checkbox"
            checked={selected}
            onChange={() => toggle(group.id)}
            onClick={(ev) => ev.stopPropagation()}
            className="accent-fg"
          />
        </td>
        <td className={clsx(COL_FIT, 'px-3 py-2.5')}>
          <div className="truncate">
            <SeverityBadge severity={group.severity} />
          </div>
        </td>
        <td className={clsx(COL_FIT, 'px-3 py-2.5 font-mono text-xs text-fg-muted')} title={formatTime(group.time)}>
          <span className="block truncate">{formatTime(group.time)}</span>
        </td>
        <td className={clsx(COL_TITLE, 'px-3 py-2.5')}>
          <div className="flex min-w-0 items-start gap-2 text-left">
            <button
              type="button"
              className="mt-0.5 shrink-0 text-fg-muted hover:text-fg"
              title={expanded ? 'Свернуть' : 'Развернуть'}
              onClick={(ev) => {
                ev.stopPropagation()
                toggleExpand(group.id)
              }}
            >
              {expanded ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </button>
            <div className="min-w-0 flex-1 overflow-hidden">
              <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
                <Layers className="h-3.5 w-3.5 shrink-0 text-proposed" />
                <span className="min-w-0 truncate" title={group.title}>
                  {group.title}
                </span>
                <Chip>
                  {eventCount} соб. / {sourceCount} ист.
                </Chip>
              </div>
              <div className="mt-0.5 truncate text-xs text-fg-dim" title={group.reason}>
                {group.reason}
              </div>
            </div>
          </div>
        </td>
        {showCategory && <td className={clsx(COL_FIT, 'px-3 py-2.5')} />}
        {selectFields.map((field) => (
          <td key={field} className={clsx(COL_FIT, 'px-3 py-2.5')} />
        ))}
        <td className={clsx(COL_FIT, 'px-3 py-2.5')}>
          <div className="flex min-w-0 gap-1 overflow-hidden">
            {Object.entries(group.sourceCounts).map(([src, n]) => (
              <span
                key={src}
                title={`${src}:${n}`}
                className="truncate font-mono text-[11px] text-fg-muted"
              >
                {src}:{n}
              </span>
            ))}
          </div>
        </td>
      </tr>
      {expanded &&
        group.eventIds.map((eid) => {
          const a = alerts[eid]
          return a ? (
            <AlertRow
              key={eid}
              alert={a}
              nested
              selectFields={selectFields}
              showCategory={showCategory}
              selected={false}
              onToggle={() => {}}
            />
          ) : null
        })}
    </>
  )
}

export function AlertTable({ investigationId }: { investigationId?: string } = {}) {
  const globalSelected = useAppStore((s) => s.selectedAlertIds)
  const globalAlerts = useAppStore((s) => s.alerts)
  const correlations = useAppStore((s) => s.correlations)
  const globalOrder = useAppStore((s) => s.queueOrder)
  const globalLoading = useAppStore((s) => s.queueLoading)
  const globalPdql = useAppStore((s) => s.queuePdql)
  const globalSource = useAppStore((s) => s.queueSource)
  const globalGroupValues = useAppStore((s) => s.groupValues)
  const queue = useAppStore((s) =>
    investigationId ? (s.contextQueue[investigationId] ?? emptyContextQueue) : null,
  )
  const inv = useAppStore((s) => (investigationId ? s.investigations[investigationId] : undefined))
  const contextEvents = useAppStore((s) => s.contextEvents)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const toggleAlertSelect = useAppStore((s) => s.toggleAlertSelect)
  const setAlertSelection = useAppStore((s) => s.setAlertSelection)
  const queueTextFilter = useAppStore((s) => (investigationId ? '' : s.queueTextFilter))
  const queueTextFilterColumn = useAppStore((s) =>
    investigationId ? 'title' : s.queueTextFilterColumn,
  )

  const entities = useAppStore((s) => s.entities)
  const alerts = queue?.alerts ?? globalAlerts
  const queueOrder = queue?.queueOrder ?? globalOrder
  const loading = queue?.loading ?? globalLoading
  const eventKeys = inv ? contextEventKeys(inv.eventIds, contextEvents) : new Set<string>()
  const findingKeys = new Set(inv?.findingSourceKeys ?? [])
  const parsed = parseQueuePdql(queue?.pdql ?? globalPdql)
  const selectFields = parsed.ok ? queueSelectFields(parsed.ast) : []
  const queueSource = queue?.queueSource ?? globalSource
  const showCategory = queueSource === 'siem_incident'
  const searchColumn = resolveAlertTableSearchColumn(
    queueTextFilterColumn,
    alertTableSearchColumns(selectFields, { showCategory }),
  ) as AlertTableColumnId
  const groupValues = queue?.groupValues ?? globalGroupValues
  const waitingForGroup =
    queueSource === 'events' &&
    parsed.ok &&
    parsed.ast.groups.length > 0 &&
    !hasGroupValueSelection(groupValues)

  const inContextOf = (alert: AlertEvent) =>
    Boolean(investigationId && alertIsInContext(alert, findingKeys, eventKeys))

  const textNeedle = queueTextFilter.trim().toLowerCase()

  const rows = queueOrder.filter((item) => {
    if (item.kind === 'entity') {
      const entity = entities[item.id]
      if (!entity) return false
      if (!textNeedle) return true
      return (
        entity.label.toLowerCase().includes(textNeedle) ||
        entity.kind.toLowerCase().includes(textNeedle)
      )
    }
    if (item.kind === 'correlation') {
      if (investigationId) return false
      const group = correlations[item.id]
      if (!group) return false
      return correlationMatchesQueueText(group, textNeedle, searchColumn)
    }
    const alert = alerts[item.id]
    if (!alert) return false
    if (investigationId && queue?.hideAdded && inContextOf(alert)) return false
    if (!alertMatchesQueueText(alert, textNeedle, searchColumn)) return false
    return true
  })

  const criticalCount = rows.filter((r) => {
    if (r.kind === 'entity') return false
    const s =
      r.kind === 'correlation'
        ? correlations[r.id]?.severity
        : alerts[r.id]?.severity
    return s === 'critical' || s === 'high'
  }).length

  const inContextCount = investigationId
    ? rows.filter((r) => r.kind === 'alert' && alerts[r.id] && inContextOf(alerts[r.id])).length
    : 0

  const selected = investigationId ? (queue?.selectedIds ?? []) : globalSelected
  const selectableIds = rows
    .filter((item) => item.kind === 'alert')
    .map((item) => item.id)
    .filter((id) => {
      const alert = alerts[id]
      return Boolean(alert && !(investigationId && inContextOf(alert)))
    })
  const selectedSelectableCount = selectableIds.filter((id) => selected.includes(id)).length
  const allSelectableSelected =
    selectableIds.length > 0 && selectedSelectableCount === selectableIds.length
  const someSelectableSelected = selectedSelectableCount > 0 && !allSelectableSelected
  const selectAllRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!selectAllRef.current) return
    selectAllRef.current.indeterminate = someSelectableSelected
  }, [someSelectableSelected])

  const toggleRow = (id: string) => {
    if (!investigationId) {
      toggleAlertSelect(id)
      return
    }
    const alert = alerts[id]
    if (alert && inContextOf(alert)) return
    const cur = queue ?? emptyContextQueue
    setContextQueue(investigationId, {
      selectedIds: cur.selectedIds.includes(id)
        ? cur.selectedIds.filter((x) => x !== id)
        : [...cur.selectedIds, id],
    })
  }

  const toggleSelectAll = () => {
    if (selectableIds.length === 0) return
    if (allSelectableSelected) {
      if (!investigationId) {
        setAlertSelection([])
        return
      }
      setContextQueue(investigationId, { selectedIds: [] })
      return
    }
    if (!investigationId) {
      setAlertSelection(selectableIds)
      return
    }
    setContextQueue(investigationId, { selectedIds: selectableIds })
  }

  const headerWrapRef = useRef<HTMLDivElement>(null)
  const bodyWrapRef = useRef<HTMLDivElement>(null)
  const syncingScroll = useRef(false)
  const [colWidths, setColWidths] = useState<Record<string, number>>(loadStoredColWidths)
  const [viewportWidth, setViewportWidth] = useState(0)

  const colKeys = alertTableColKeys(selectFields, Boolean(investigationId), showCategory)
  const storedWidths = colKeys.map((key) => colWidths[key] ?? defaultWidthForCol(key))
  const titleIndex = colKeys.indexOf('title')
  const storedSum = storedWidths.reduce((sum, width) => sum + width, 0)
  // Fill leftover space into title so the last columns aren't cut off by an empty gap.
  const tableWidth = Math.max(storedSum, viewportWidth)
  const titleExtra = Math.max(0, tableWidth - storedSum)
  const resolvedWidths = storedWidths.map((width, index) =>
    index === titleIndex ? width + titleExtra : width,
  )

  const applyPairWidths = (
    leftKey: string,
    rightKey: string,
    left: number,
    right: number,
    persist: boolean,
  ) => {
    setColWidths((prev) => {
      const updated = { ...prev }
      for (const key of colKeys) {
        updated[key] = updated[key] ?? defaultWidthForCol(key)
      }
      updated[leftKey] = left
      updated[rightKey] = right
      if (persist) {
        try {
          localStorage.setItem(COL_WIDTHS_STORAGE_KEY, JSON.stringify(updated))
        } catch {
          /* ignore */
        }
      }
      return updated
    })
  }

  const resetColumnWidths = () => {
    const next: Record<string, number> = {}
    for (const key of colKeys) {
      next[key] = defaultWidthForCol(key)
    }
    setColWidths(next)
    try {
      localStorage.setItem(COL_WIDTHS_STORAGE_KEY, JSON.stringify(next))
    } catch {
      /* ignore */
    }
  }

  const syncScrollLeft = (source: 'header' | 'body') => {
    const header = headerWrapRef.current
    const body = bodyWrapRef.current
    if (!header || !body || syncingScroll.current) return
    syncingScroll.current = true
    if (source === 'body') header.scrollLeft = body.scrollLeft
    else body.scrollLeft = header.scrollLeft
    syncingScroll.current = false
  }

  useLayoutEffect(() => {
    const body = bodyWrapRef.current
    if (!body) return
    const update = () => {
      setViewportWidth(body.clientWidth)
      syncScrollLeft('body')
    }
    update()
    const ro = new ResizeObserver(update)
    ro.observe(body)
    return () => ro.disconnect()
  }, [])

  const colGroup = (
    <colgroup>
      {colKeys.map((key, index) => (
        <col key={key} style={{ width: resolvedWidths[index] }} />
      ))}
    </colgroup>
  )

  const tableStyle = { width: tableWidth }

  const renderResizeHandle = (leftKey: string, index: number) => {
    const rightKey = colKeys[index + 1]
    if (!rightKey) return null
    // Drag against stored widths (title without the fill extra) so pair math stays stable.
    const leftWidth = storedWidths[index]
    const rightWidth = storedWidths[index + 1]
    return (
      <ColumnResizeHandle
        leftWidth={leftWidth}
        rightWidth={rightWidth}
        leftMin={minWidthForCol(leftKey)}
        rightMin={minWidthForCol(rightKey)}
        onWidthsChange={(left, right) => applyPairWidths(leftKey, rightKey, left, right, false)}
        onWidthsCommit={(left, right) => applyPairWidths(leftKey, rightKey, left, right, true)}
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="relative flex shrink-0 border-b border-border bg-surface-1">
        <div
          ref={headerWrapRef}
          className="min-w-0 flex-1 overflow-x-auto overflow-y-hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          onScroll={() => syncScrollLeft('header')}
        >
          <table className={TABLE_CLASS} style={tableStyle}>
            {colGroup}
            <thead className="text-[11px] uppercase tracking-wider text-fg-dim">
              <tr>
                <th className={clsx(COL_FIT, 'relative px-3 py-2')}>
                  <input
                    ref={selectAllRef}
                    type="checkbox"
                    checked={allSelectableSelected}
                    disabled={selectableIds.length === 0}
                    onChange={toggleSelectAll}
                    title={allSelectableSelected ? 'Снять выбор' : 'Выбрать все'}
                    aria-label={allSelectableSelected ? 'Снять выбор' : 'Выбрать все'}
                    className="accent-fg"
                  />
                  {renderResizeHandle('select', 0)}
                </th>
                <th
                  className={clsx(COL_FIT, 'relative px-3 py-2')}
                  title={alertTableColumnLabel('severity')}
                >
                  <span className="flex min-w-0 items-center gap-1.5 overflow-hidden">
                    <span className="truncate">{alertTableColumnLabel('severity')}</span>
                    <span className="shrink-0 text-high">{criticalCount}</span>
                  </span>
                  {renderResizeHandle('severity', 1)}
                </th>
                <th
                  className={clsx(COL_FIT, 'relative px-3 py-2')}
                  title={alertTableColumnLabel('time')}
                >
                  <span className="block truncate">{alertTableColumnLabel('time')}</span>
                  {renderResizeHandle('time', 2)}
                </th>
                <th
                  className={clsx(COL_TITLE, 'relative px-3 py-2')}
                  title={alertTableColumnLabel('title')}
                >
                  <span className="flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap">
                    <span className="truncate">{alertTableColumnLabel('title')}</span>
                    <span className="shrink-0 text-fg">{rows.length}</span>
                    {loading && (
                      <span className="shrink-0 normal-case tracking-normal text-fg-dim">загрузка…</span>
                    )}
                    {investigationId && !queue?.hideAdded && (
                      <span className="shrink-0 text-fg-muted">
                        в контексте <span className="text-fg">{inContextCount}</span>
                      </span>
                    )}
                  </span>
                  {renderResizeHandle('title', 3)}
                </th>
                {showCategory && (
                  <th
                    className={clsx(COL_FIT, 'relative px-3 py-2')}
                    title={alertTableColumnLabel('category')}
                  >
                    <span className="block truncate">{alertTableColumnLabel('category')}</span>
                    {renderResizeHandle('category', 4)}
                  </th>
                )}
                {selectFields.map((field, fieldIndex) => {
                  const key = fieldColKey(field)
                  const index = 4 + (showCategory ? 1 : 0) + fieldIndex
                  return (
                    <th
                      key={field}
                      className={clsx(
                        COL_FIT,
                        'relative px-3 py-2 font-mono normal-case tracking-normal',
                      )}
                      title={field}
                    >
                      <span className="block truncate">{field}</span>
                      {renderResizeHandle(key, index)}
                    </th>
                  )
                })}
                <th
                  className={clsx(COL_FIT, 'relative px-3 py-2')}
                  title={alertTableColumnLabel('source')}
                >
                  <span className="block truncate">{alertTableColumnLabel('source')}</span>
                  {renderResizeHandle('source', 4 + (showCategory ? 1 : 0) + selectFields.length)}
                </th>
                {investigationId && (
                  <th className={clsx(COL_FIT, 'relative px-3 py-2')} />
                )}
              </tr>
            </thead>
          </table>
        </div>
        {/* Match body vertical scrollbar width so header/body columns stay aligned. */}
        <div className="w-2 shrink-0" aria-hidden />
        <button
          type="button"
          title="Сбросить ширину колонок"
          aria-label="Сбросить ширину колонок"
          onClick={resetColumnWidths}
          className="absolute top-1/2 right-0 z-30 h-8 w-8 -translate-y-1/2 rounded hover:bg-surface-2"
        />
      </div>
      <div
        ref={bodyWrapRef}
        className="relative min-h-0 flex-1 overflow-auto"
        onScroll={() => syncScrollLeft('body')}
      >
        {rows.length === 0 ? (
          <div className="flex h-full items-center justify-center px-4 text-center">
            <div>
              {waitingForGroup ? (
                <>
                  <div className="text-sm text-fg-muted">
                    Выберите группу слева для просмотра событий
                  </div>
                  <div className="mt-1 text-xs text-fg-dim">
                    Сначала загружаются агрегаты, поиск событий запускается после выбора значения
                  </div>
                </>
              ) : (
                <>
                  <div className="text-sm text-fg-muted">
                    Нет находок по текущим фильтрам
                  </div>
                  <div className="mt-1 text-xs text-fg-dim">
                    Попробуйте расширить поиск и изменить фильтры
                  </div>
                </>
              )}
            </div>
          </div>
        ) : (
          <table className={TABLE_CLASS} style={tableStyle}>
            {colGroup}
            <tbody>
              {rows.map((item) =>
                item.kind === 'correlation' ? (
                  <CorrelationRow
                    key={item.id}
                    group={correlations[item.id]}
                    alerts={alerts}
                    selectFields={selectFields}
                    showCategory={showCategory}
                  />
                ) : item.kind === 'entity' ? (
                  <EntityRow key={item.id} entity={entities[item.id]} />
                ) : (
                  <AlertRow
                    key={item.id}
                    alert={alerts[item.id]}
                    investigationId={investigationId}
                    inContext={inContextOf(alerts[item.id])}
                    selected={selected.includes(item.id)}
                    selectFields={selectFields}
                    showCategory={showCategory}
                    onToggle={() => toggleRow(item.id)}
                  />
                ),
              )}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
