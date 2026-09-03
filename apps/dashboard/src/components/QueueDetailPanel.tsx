import { useEffect, useState } from 'react'
import {
  Lightbulb,
  Play,
  Plus,
  X,
} from 'lucide-react'
import { emptyContextQueue, useAppStore } from '../store/appStore'
import type { AlertEvent, CorrelationGroup } from '../types'
import { Button, Chip, Panel, SeverityBadge } from './ui'
import { ResizablePanelFrame } from './ResizablePanelFrame'
import { StartInvestigationModal } from './StartInvestigationModal'
import { isHypothesisWritable } from '../lib/hypotheses'
import { titlesForQueueIds } from '../lib/investigationTitle'
import { formatTime, statusLabel } from '../lib/utils'
import { alertIsInContext, contextEventKeys } from '../lib/queueContext'
import { EventCard, eventCardModelFromAlert } from './event-card'
import type { TimeInterval } from './time-interval'

export function QueueDetailPanel({
  investigationId,
}: {
  investigationId?: string
} = {}) {
  const item = useAppStore((s) => s.inspectedQueueItem)
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const start = useAppStore((s) => s.startInvestigation)
  const addEventsToContext = useAppStore((s) => s.addEventsToContext)
  const addEventsToActiveHypothesis = useAppStore((s) => s.addEventsToActiveHypothesis)
  const createHypothesisFromEvents = useAppStore((s) => s.createHypothesisFromEvents)
  const appendPdqlFilter = useAppStore((s) => s.appendPdqlFilter)
  const filterByFindingUuid = useAppStore((s) => s.filterByFindingUuid)
  const globalTime = useAppStore((s) => s.timeInterval)
  const setTimeInterval = useAppStore((s) => s.setTimeInterval)
  const loadQueue = useAppStore((s) => s.loadQueue)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const executeContextQuery = useAppStore((s) => s.executeContextQuery)
  const globalAlerts = useAppStore((s) => s.alerts)
  const queueAlerts = useAppStore((s) =>
    investigationId ? s.contextQueue[investigationId]?.alerts : undefined,
  )
  const queue = useAppStore((s) =>
    investigationId ? (s.contextQueue[investigationId] ?? emptyContextQueue) : null,
  )
  const inv = useAppStore((s) => (investigationId ? s.investigations[investigationId] : undefined))
  const entities = useAppStore((s) => s.entities)
  const activeHypothesis = useAppStore((s) => {
    if (!investigationId) return null
    const id = s.activeHypothesisId[investigationId]
    return id ? (s.hypotheses[id] ?? null) : null
  })
  const contextEvents = useAppStore((s) => s.contextEvents)
  const correlations = useAppStore((s) => s.correlations)
  const loading = useAppStore((s) => s.investigationLoading)
  const [naming, setNaming] = useState(false)

  useEffect(() => {
    if (!item) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') inspect(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [item, inspect])

  if (!item) return null

  const alerts = queueAlerts ?? globalAlerts
  const alert = item.kind === 'alert' ? (queueAlerts?.[item.id] ?? globalAlerts[item.id]) : undefined
  const group = item.kind === 'correlation' ? correlations[item.id] : undefined
  const entity = item.kind === 'entity' ? entities[item.id] : undefined
  if (!alert && !group && !entity) return null

  const timeInterval = queue?.timeInterval ?? globalTime
  const eventKeys = inv ? contextEventKeys(inv.eventIds, contextEvents) : new Set<string>()
  const inContext = Boolean(
    investigationId && alert && alertIsInContext(alert, inv?.findingSourceKeys ?? [], eventKeys),
  )
  const canAddToHypothesis = Boolean(
    alert && activeHypothesis && isHypothesisWritable(activeHypothesis.status),
  )
  const addToHypothesisTitle = !activeHypothesis
    ? 'Сначала выберите гипотезу'
    : activeHypothesis.status === 'resolved'
      ? 'Гипотеза закрыта'
      : activeHypothesis.statement

  return (
    <>
    <ResizablePanelFrame storageKey="ir.detailPanel.width" defaultWidth={512} side="right">
    <Panel
      title={alert ? 'Событие' : group ? 'Корреляция' : 'Сущность'}
      className="w-full min-w-0 flex-1"
      actions={
        <button type="button" onClick={() => inspect(null)} title="Закрыть">
          <X className="h-3.5 w-3.5 text-fg-dim" />
        </button>
      }
    >
      <div className="flex min-h-full flex-col">
        <div className="flex-1 space-y-4 p-3">
          {entity && (
            <div className="space-y-2">
              <div className="text-[10px] uppercase tracking-wider text-fg-dim">{entity.kind}</div>
              <div className="break-all font-mono text-sm text-fg">{entity.label}</div>
              {entity.source ? (
                <div className="font-mono text-[11px] text-fg-muted">{entity.source}</div>
              ) : null}
            </div>
          )}
          {alert && inContext && <Chip tone="confirmed">в контексте</Chip>}
          {alert && (
            <AlertDetails
              alert={alert}
              onAddFilter={(field, value) =>
                appendPdqlFilter(investigationId ?? null, field, value)
              }
              onFilterFindingUuid={(uuid, recordType) =>
                filterByFindingUuid(investigationId ?? null, uuid, recordType)
              }
              timeInterval={timeInterval}
              onTimeChange={(interval) => {
                if (investigationId) setContextQueue(investigationId, { timeInterval: interval })
                else setTimeInterval(interval)
              }}
              onTimeExecute={(interval) => {
                if (investigationId) {
                  setContextQueue(investigationId, { timeInterval: interval })
                  void executeContextQuery(investigationId)
                  return
                }
                setTimeInterval(interval)
                void loadQueue()
              }}
            />
          )}
          {group && (
            <CorrelationDetails
              group={group}
              alerts={alerts}
              onOpenAlert={(id) => inspect({ kind: 'alert', id })}
            />
          )}
        </div>

        <div className="sticky bottom-0 space-y-2 border-t border-border bg-surface-1 p-3">
          {entity ? (
            <div className="text-xs text-fg-dim">Сущность из поиска по entities</div>
          ) : investigationId ? (
            <>
              {inContext ? (
                <div className="text-xs text-fg-dim">Уже в расследовании</div>
              ) : (
                <Button
                  size="md"
                  variant="primary"
                  className="w-full"
                  disabled={loading || !alert}
                  onClick={() => void addEventsToContext(investigationId, [item.id])}
                >
                  <Plus className="h-3.5 w-3.5" />
                  Добавить в расследование
                </Button>
              )}
              {alert && (
                <>
                  <Button
                    size="md"
                    variant={inContext && canAddToHypothesis ? 'primary' : 'default'}
                    className="w-full"
                    disabled={loading || !canAddToHypothesis}
                    title={addToHypothesisTitle}
                    onClick={() => void addEventsToActiveHypothesis(investigationId, [item.id])}
                  >
                    <Lightbulb className="h-3.5 w-3.5" />
                    Добавить в текущую гипотезу
                  </Button>
                  <Button
                    size="md"
                    variant="ghost"
                    className="w-full"
                    disabled={loading}
                    onClick={() => void createHypothesisFromEvents(investigationId, [item.id])}
                  >
                    <Lightbulb className="h-3.5 w-3.5" />
                    Создать гипотезу
                  </Button>
                </>
              )}
            </>
          ) : (
            <>
              <Button
                size="md"
                variant="primary"
                className="w-full"
                disabled={loading}
                onClick={() => setNaming(true)}
              >
                <Play className="h-3.5 w-3.5" />
                Начать расследование
              </Button>
            </>
          )}
        </div>
      </div>
    </Panel>
    </ResizablePanelFrame>
    {naming && (
      <StartInvestigationModal
        eventTitles={titlesForQueueIds([item.id], alerts, correlations)}
        busy={loading}
        onClose={() => setNaming(false)}
        onConfirm={async (title) => {
          const createdId = await start([item.id], title)
          if (createdId) setNaming(false)
        }}
      />
    )}
    </>
  )
}

function AlertDetails({
  alert,
  onAddFilter,
  onFilterFindingUuid,
  timeInterval,
  onTimeChange,
  onTimeExecute,
}: {
  alert: AlertEvent
  onAddFilter: (field: string, value: string) => void
  onFilterFindingUuid: (uuid: string, recordType: 'siem_incident' | 'siem_correlation') => void
  timeInterval: TimeInterval
  onTimeChange: (value: TimeInterval) => void
  onTimeExecute: (value: TimeInterval) => void
}) {
  return (
    <>
      <span className="text-xs text-fg-dim">{statusLabel[alert.status]}</span>
      <EventCard
        event={eventCardModelFromAlert(alert)}
        timeInterval={timeInterval}
        onTimeChange={onTimeChange}
        onTimeExecute={onTimeExecute}
        onAddFilter={onAddFilter}
        onFilterFindingUuid={onFilterFindingUuid}
      />
    </>
  )
}

function CorrelationDetails({
  group,
  alerts,
  onOpenAlert,
}: {
  group: CorrelationGroup
  alerts: Record<string, AlertEvent>
  onOpenAlert: (id: string) => void
}) {
  const eventCount = group.eventIds.length
  const sourceCount = Object.keys(group.sourceCounts).length

  return (
    <>
      <div>
        <div className="flex items-center gap-2">
          <SeverityBadge severity={group.severity} />
          <span className="text-xs text-fg-dim">{statusLabel[group.status]}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-snug">{group.title}</div>
        {group.reason && (
          <p className="mt-1.5 text-xs leading-relaxed text-fg-muted">{group.reason}</p>
        )}
        <div className="mt-2 flex flex-wrap gap-1.5">
          <Chip>
            {eventCount} соб. / {sourceCount} ист.
          </Chip>
          <Chip>{formatTime(group.time)}</Chip>
        </div>
      </div>

      <div>
        <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">Источники</div>
        <div className="flex flex-wrap gap-1">
          {Object.entries(group.sourceCounts).map(([src, n]) => (
            <Chip key={src}>
              {src}:{n}
            </Chip>
          ))}
        </div>
      </div>

      <div>
        <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">События</div>
        <div className="space-y-1">
          {group.eventIds.map((eid) => {
            const a = alerts[eid]
            if (!a) return null
            return (
              <button
                key={eid}
                type="button"
                className="flex w-full items-start justify-between gap-2 rounded border border-border px-2 py-1.5 text-left text-xs hover:bg-surface-2"
                onClick={() => onOpenAlert(eid)}
              >
                <span className="min-w-0">
                  <span className="block truncate text-fg">{a.title}</span>
                  <span className="text-[11px] text-fg-dim">{a.source}</span>
                </span>
                <SeverityBadge severity={a.severity} />
              </button>
            )
          })}
        </div>
      </div>
    </>
  )
}
