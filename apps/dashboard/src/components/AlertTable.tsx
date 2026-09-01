import { useAppStore, emptyContextQueue } from '../store/appStore'
import type { AlertEvent, CorrelationGroup, QueueItem } from '../types'
import { Button, Chip, SeverityBadge } from './ui'
import { clsx, formatTime } from '../lib/utils'
import { hasGroupValueSelection, parseQueuePdql, queueSelectFields } from '../lib/pdql'
import { alertIsInContext, contextEventKeys } from '../lib/queueContext'
import { ChevronDown, ChevronRight, Layers, Loader2, Play, Plus } from 'lucide-react'

const COL_FIT = 'w-px whitespace-nowrap'
const COL_TITLE = 'min-w-0 w-full'

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
    <td className={clsx(COL_FIT, 'max-w-[16rem] px-3 py-2')}>
      {value ? (
        <button
          type="button"
          title={display}
          className="block max-w-[16rem] truncate text-left font-mono text-xs text-fg hover:underline"
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
        <SeverityBadge severity={alert.severity} />
      </td>
      <td className={clsx(COL_FIT, 'px-3 py-2 font-mono text-xs text-fg-muted')}>
        {formatTime(alert.time)}
      </td>
      <td className={clsx(COL_TITLE, 'px-3 py-2')}>
        <div className="flex min-w-0 items-center gap-2">
          <div className={clsx('min-w-0 text-sm', inContext && 'text-fg-muted')}>{alert.title}</div>
          {inContext && (
            <Chip tone="confirmed">в контексте</Chip>
          )}
        </div>
        <div className="text-xs text-fg-dim">{alert.rule}</div>
      </td>
      {selectFields.map((field) => (
        <SelectFieldCell
          key={field}
          field={field}
          value={queueFieldValue(alert, field)}
          investigationId={investigationId}
        />
      ))}
      <td className={clsx(COL_FIT, 'px-3 py-2')}>
        <span className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px]">
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
          <SeverityBadge severity={group.severity} />
        </td>
        <td className={clsx(COL_FIT, 'px-3 py-2.5 font-mono text-xs text-fg-muted')}>
          {formatTime(group.time)}
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
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                <Layers className="h-3.5 w-3.5 text-proposed" />
                {group.title}
                <Chip>
                  {eventCount} соб. / {sourceCount} ист.
                </Chip>
              </div>
              <div className="mt-0.5 text-xs text-fg-dim">{group.reason}</div>
            </div>
          </div>
        </td>
        {selectFields.map((field) => (
          <td key={field} className={clsx(COL_FIT, 'px-3 py-2.5')} />
        ))}
        <td className={clsx(COL_FIT, 'px-3 py-2.5')}>
          <div className="flex flex-wrap gap-1">
            {Object.entries(group.sourceCounts).map(([src, n]) => (
              <span
                key={src}
                className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px]"
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
  const start = useAppStore((s) => s.startInvestigation)
  const starting = useAppStore((s) => s.investigationLoading)
  const clear = useAppStore((s) => s.clearAlertSelection)
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
  const addEventsToContext = useAppStore((s) => s.addEventsToContext)
  const toggleAlertSelect = useAppStore((s) => s.toggleAlertSelect)

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

  const rows = queueOrder.filter((item) => {
    if (item.kind === 'correlation') {
      if (investigationId) return false
      return Boolean(correlations[item.id])
    }
    const alert = alerts[item.id]
    if (!alert) return false
    if (investigationId && queue?.hideAdded && inContextOf(alert)) return false
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

  const addSelected = () => {
    if (!investigationId) return
    const ids = selected.filter((id) => {
      const alert = alerts[id]
      return alert && !inContextOf(alert)
    })
    if (ids.length === 0) return
    void addEventsToContext(investigationId, ids)
  }

  const clearSelection = () => {
    if (investigationId) setContextQueue(investigationId, { selectedIds: [] })
    else clear()
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <div className="flex items-center gap-3 text-sm">
          <span className="text-fg-muted">
            Срабатываний: <span className="text-fg">{rows.length}</span>
            {loading && <span className="ml-2 text-fg-dim">загрузка…</span>}
          </span>
          <span className="text-fg-muted">
            critical/high: <span className="text-high">{criticalCount}</span>
          </span>
          {investigationId && !queue?.hideAdded && (
            <span className="text-fg-muted">
              в контексте: <span className="text-fg">{inContextCount}</span>
            </span>
          )}
        </div>
        {selected.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-xs text-fg-muted">Выбрано: {selected.length}</span>
            {investigationId ? (
              <Button
                size="sm"
                variant="primary"
                onClick={addSelected}
              >
                <Plus className="h-3 w-3" />
                Добавить в расследование
              </Button>
            ) : (
              <Button
                size="sm"
                disabled={starting}
                onClick={() => void start(selected)}
              >
                {starting ? (
                  <Loader2 className="h-3 w-3 animate-spin" />
                ) : (
                  <Play className="h-3 w-3" />
                )}
                Начать расследование
              </Button>
            )}
            <Button size="sm" variant="ghost" onClick={clearSelection}>
              Сбросить
            </Button>
          </div>
        )}
      </div>
      <div className="flex-1 overflow-auto">
        <table className="w-full min-w-[880px] border-collapse text-left">
          <thead className="sticky top-0 z-10 bg-surface-1 text-[11px] uppercase tracking-wider text-fg-dim">
            <tr className="border-b border-border">
              <th className={clsx(COL_FIT, 'px-3 py-2')} />
              <th className={clsx(COL_FIT, 'px-3 py-2')}>Крит.</th>
              <th className={clsx(COL_FIT, 'px-3 py-2')}>Время</th>
              <th className={clsx(COL_TITLE, 'px-3 py-2')}>Срабатывание</th>
              {selectFields.map((field) => (
                <th key={field} className={clsx(COL_FIT, 'px-3 py-2 font-mono normal-case tracking-normal')}>
                  {field}
                </th>
              ))}
              <th className={clsx(COL_FIT, 'px-3 py-2')}>Источник</th>
              {investigationId && <th className={clsx(COL_FIT, 'px-3 py-2')} />}
            </tr>
          </thead>
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
