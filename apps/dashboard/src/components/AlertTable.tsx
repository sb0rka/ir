import { useLayoutEffect, useRef, useState } from 'react'
import { useAppStore, emptyContextQueue } from '../store/appStore'
import type { AlertEvent, CorrelationGroup, QueueItem } from '../types'
import { Button, Chip, SeverityBadge } from './ui'
import { clsx, formatTime } from '../lib/utils'
import { hasGroupValueSelection, parseQueuePdql, queueSelectFields } from '../lib/pdql'
import { alertIsInContext, contextEventKeys } from '../lib/queueContext'
import { ChevronDown, ChevronRight, Layers, MoveHorizontal, Plus } from 'lucide-react'

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
  source: 160,
  actions: 48,
}

function fieldColKey(field: string) {
  return `field:${field}`
}

function alertTableColKeys(selectFields: string[], hasActions: boolean): string[] {
  return [
    'select',
    'severity',
    'time',
    'title',
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

function AlertRow({
  alert,
  nested,
  investigationId,
  inContext,
  selected,
  selectFields,
  onToggle,
}: {
  alert: AlertEvent
  nested?: boolean
  investigationId?: string
  inContext?: boolean
  selected: boolean
  selectFields: string[]
  onToggle: () => void
}) {
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const addEventsToContext = useAppStore((s) => s.addEventsToContext)
  const inspected = useAppStore((s) => isInspected(s.inspectedQueueItem, 'alert', alert.id))

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
}: {
  group: CorrelationGroup
  alerts: Record<string, AlertEvent>
  selectFields: string[]
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
  const queueTextFilter = useAppStore((s) => (investigationId ? '' : s.queueTextFilter))

  const alerts = queue?.alerts ?? globalAlerts
  const queueOrder = queue?.queueOrder ?? globalOrder
  const loading = queue?.loading ?? globalLoading
  const eventKeys = inv ? contextEventKeys(inv.eventIds, contextEvents) : new Set<string>()
  const findingKeys = new Set(inv?.findingSourceKeys ?? [])
  const parsed = parseQueuePdql(queue?.pdql ?? globalPdql)
  const selectFields = parsed.ok ? queueSelectFields(parsed.ast) : []
  const colSpan = 5 + selectFields.length + (investigationId ? 1 : 0)
  const queueSource = queue?.queueSource ?? globalSource
  const groupValues = queue?.groupValues ?? globalGroupValues
  const waitingForGroup =
    queueSource === 'events' &&
    parsed.ok &&
    parsed.ast.groups.length > 0 &&
    !hasGroupValueSelection(groupValues)

  const inContextOf = (alert: AlertEvent) =>
    Boolean(investigationId && alertIsInContext(alert, findingKeys, eventKeys))

  const textNeedle = queueTextFilter.trim().toLowerCase()
  const matchesText = (haystack: string) =>
    !textNeedle || haystack.toLowerCase().includes(textNeedle)

  const rows = queueOrder.filter((item) => {
    if (item.kind === 'correlation') {
      if (investigationId) return false
      const group = correlations[item.id]
      if (!group) return false
      return matchesText([group.title, group.reason].filter(Boolean).join(' '))
    }
    const alert = alerts[item.id]
    if (!alert) return false
    if (investigationId && queue?.hideAdded && inContextOf(alert)) return false
    if (
      !matchesText(
        [alert.title, alert.rule, alert.description].filter(Boolean).join(' '),
      )
    ) {
      return false
    }
    return true
  })

  const criticalCount = rows.filter((r) => {
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

  const headerWrapRef = useRef<HTMLDivElement>(null)
  const bodyWrapRef = useRef<HTMLDivElement>(null)
  const syncingScroll = useRef(false)
  const [colWidths, setColWidths] = useState<Record<string, number>>(loadStoredColWidths)
  const [viewportWidth, setViewportWidth] = useState(0)

  const colKeys = alertTableColKeys(selectFields, Boolean(investigationId))
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
                  {renderResizeHandle('select', 0)}
                </th>
                <th className={clsx(COL_FIT, 'relative px-3 py-2')} title="Крит.">
                  <span className="flex min-w-0 items-center gap-1.5 overflow-hidden">
                    <span className="truncate">Крит.</span>
                    <span className="shrink-0 text-high">{criticalCount}</span>
                  </span>
                  {renderResizeHandle('severity', 1)}
                </th>
                <th className={clsx(COL_FIT, 'relative px-3 py-2')} title="Время">
                  <span className="block truncate">Время</span>
                  {renderResizeHandle('time', 2)}
                </th>
                <th className={clsx(COL_TITLE, 'relative px-3 py-2')} title="Срабатывание">
                  <span className="flex min-w-0 items-center gap-1.5 overflow-hidden whitespace-nowrap">
                    <span className="truncate">Срабатывание</span>
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
                {selectFields.map((field, fieldIndex) => {
                  const key = fieldColKey(field)
                  const index = 4 + fieldIndex
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
                <th className={clsx(COL_FIT, 'relative px-3 py-2')} title="Источник">
                  <span className="block truncate">Источник</span>
                  {renderResizeHandle('source', 4 + selectFields.length)}
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
        <div className="absolute top-1/2 right-2 z-30 -translate-y-1/2">
          <Button
            size="icon"
            variant="ghost"
            title="Сбросить ширину колонок"
            aria-label="Сбросить ширину колонок"
            onClick={resetColumnWidths}
          >
            <MoveHorizontal className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      <div
        ref={bodyWrapRef}
        className="min-h-0 flex-1 overflow-auto"
        onScroll={() => syncScrollLeft('body')}
      >
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
                />
              ) : (
                <AlertRow
                  key={item.id}
                  alert={alerts[item.id]}
                  investigationId={investigationId}
                  inContext={inContextOf(alerts[item.id])}
                  selected={selected.includes(item.id)}
                  selectFields={selectFields}
                  onToggle={() => toggleRow(item.id)}
                />
              ),
            )}
            {rows.length === 0 && (
              <tr>
                <td colSpan={colSpan} className="px-4 py-12 text-center">
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
                        Нет срабатываний по текущим фильтрам
                      </div>
                      <div className="mt-1 text-xs text-fg-dim">
                        Удалите часть чипов или расширьте окно времени
                      </div>
                    </>
                  )}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
