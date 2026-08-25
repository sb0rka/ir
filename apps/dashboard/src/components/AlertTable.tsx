import { useAppStore } from '../store/appStore'
import type { AlertEvent, CorrelationGroup, Entity, QueueItem } from '../types'
import { Button, Chip, SeverityBadge } from './ui'
import { clsx, formatTime } from '../lib/utils'
import { fieldForEntityKind } from '../lib/filters'
import { ChevronDown, ChevronRight, Layers, Play } from 'lucide-react'

function EntityChips({
  entityIds,
  max = 4,
  entities,
}: {
  entityIds: string[]
  max?: number
  entities: Record<string, Entity>
}) {
  const addChip = useAppStore((s) => s.addChip)
  const shown = entityIds.slice(0, max)
  const rest = entityIds.length - shown.length

  return (
    <div className="flex flex-wrap gap-1">
      {shown.map((id) => {
        const e = entities[id]
        if (!e) return null
        const field = fieldForEntityKind(e.kind)
        return (
          <button
            key={id}
            type="button"
            className="rounded border border-border bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-fg-muted hover:border-fg/30 hover:text-fg"
            title="Найти связанные"
            onClick={(ev) => {
              ev.stopPropagation()
              if (field) addChip(field, e.label.replaceAll('[', '').replaceAll(']', ''))
            }}
          >
            <span className="text-fg-dim">{e.kind}:</span> {e.label}
          </button>
        )
      })}
      {rest > 0 && <span className="text-[11px] text-fg-dim">+{rest}</span>}
    </div>
  )
}

function isInspected(item: QueueItem | null, kind: QueueItem['kind'], id: string) {
  return item?.kind === kind && item.id === id
}

function AlertRow({
  alert,
  nested,
  entities,
}: {
  alert: AlertEvent
  nested?: boolean
  entities: Record<string, Entity>
}) {
  const selected = useAppStore((s) => s.selectedAlertIds.includes(alert.id))
  const toggle = useAppStore((s) => s.toggleAlertSelect)
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const inspected = useAppStore((s) => isInspected(s.inspectedQueueItem, 'alert', alert.id))

  return (
    <tr
      className={clsx(
        'cursor-pointer border-b border-border/60 hover:bg-surface-2/60',
        selected && 'bg-surface-2',
        inspected && 'bg-surface-3/70',
        nested && !inspected && 'bg-surface-0/40',
      )}
      onClick={() => inspect({ kind: 'alert', id: alert.id })}
    >
      <td className="px-3 py-2">
        <input
          type="checkbox"
          checked={selected}
          onChange={() => toggle(alert.id)}
          onClick={(ev) => ev.stopPropagation()}
          className="accent-fg"
        />
      </td>
      <td className="px-3 py-2">
        <SeverityBadge severity={alert.severity} />
      </td>
      <td className="whitespace-nowrap px-3 py-2 font-mono text-xs text-fg-muted">
        {formatTime(alert.time)}
      </td>
      <td className="px-3 py-2">
        <div className="text-sm">{alert.title}</div>
        <div className="text-xs text-fg-dim">{alert.rule}</div>
      </td>
      <td className="px-3 py-2">
        <EntityChips entityIds={alert.entityIds} entities={entities} />
      </td>
      <td className="px-3 py-2">
        <span className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px]">
          {alert.source}
        </span>
      </td>
    </tr>
  )
}

function CorrelationRow({
  group,
  alerts,
  entities,
}: {
  group: CorrelationGroup
  alerts: Record<string, AlertEvent>
  entities: Record<string, Entity>
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
        <td className="px-3 py-2.5">
          <input
            type="checkbox"
            checked={selected}
            onChange={() => toggle(group.id)}
            onClick={(ev) => ev.stopPropagation()}
            className="accent-fg"
          />
        </td>
        <td className="px-3 py-2.5">
          <SeverityBadge severity={group.severity} />
        </td>
        <td className="whitespace-nowrap px-3 py-2.5 font-mono text-xs text-fg-muted">
          {formatTime(group.time)}
        </td>
        <td className="px-3 py-2.5">
          <div className="flex items-start gap-2 text-left">
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
            <div>
              <div className="flex items-center gap-2 text-sm font-medium">
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
        <td className="px-3 py-2.5">
          <EntityChips entityIds={group.entityIds} max={5} entities={entities} />
        </td>
        <td className="px-3 py-2.5">
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
          return a ? <AlertRow key={eid} alert={a} nested entities={entities} /> : null
        })}
    </>
  )
}

export function AlertTable() {
  const selected = useAppStore((s) => s.selectedAlertIds)
  const start = useAppStore((s) => s.startInvestigation)
  const clear = useAppStore((s) => s.clearAlertSelection)
  const alerts = useAppStore((s) => s.alerts)
  const correlations = useAppStore((s) => s.correlations)
  const queueOrder = useAppStore((s) => s.queueOrder)
  const entities = useAppStore((s) => s.entities)
  const loading = useAppStore((s) => s.queueLoading)

  const rows = queueOrder.filter((item) => {
    if (item.kind === 'correlation') return Boolean(correlations[item.id])
    return Boolean(alerts[item.id])
  })

  const criticalCount = rows.filter((r) => {
    const s =
      r.kind === 'correlation'
        ? correlations[r.id]?.severity
        : alerts[r.id]?.severity
    return s === 'critical' || s === 'high'
  }).length

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <div className="flex items-center gap-3 text-sm">
          <span className="text-fg-muted">
            Срабатываний: <span className="text-fg">{rows.length}</span>
            {loading && <span className="ml-2 text-fg-dim">загрузка…</span>}
          </span>
          <span className="text-fg-muted">
            critical/high: <span className="text-high">{criticalCount}</span>
          </span>
        </div>
        {selected.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-xs text-fg-muted">Выбрано: {selected.length}</span>
            <Button size="sm" onClick={() => start(selected)}>
              <Play className="h-3 w-3" />
              Начать расследование
            </Button>
            <Button size="sm" variant="ghost" onClick={clear}>
              Сбросить
            </Button>
          </div>
        )}
      </div>
      <div className="flex-1 overflow-auto">
        <table className="w-full min-w-[880px] border-collapse text-left">
          <thead className="sticky top-0 z-10 bg-surface-1 text-[11px] uppercase tracking-wider text-fg-dim">
            <tr className="border-b border-border">
              <th className="w-10 px-3 py-2" />
              <th className="px-3 py-2">Крит.</th>
              <th className="px-3 py-2">Время</th>
              <th className="px-3 py-2">Срабатывание</th>
              <th className="px-3 py-2">Сущности</th>
              <th className="px-3 py-2">Источник</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((item) =>
              item.kind === 'correlation' ? (
                <CorrelationRow
                  key={item.id}
                  group={correlations[item.id]}
                  alerts={alerts}
                  entities={entities}
                />
              ) : (
                <AlertRow key={item.id} alert={alerts[item.id]} entities={entities} />
              ),
            )}
            {rows.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-12 text-center">
                  <div className="text-sm text-fg-muted">
                    Нет срабатываний по текущим фильтрам
                  </div>
                  <div className="mt-1 text-xs text-fg-dim">
                    Удалите часть чипов или расширьте окно времени
                  </div>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
