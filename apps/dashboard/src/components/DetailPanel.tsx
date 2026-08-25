import { useAppStore, emptyContextQueue } from '../store/appStore'
import { Button, Chip, Panel } from './ui'
import { formatTime, kindLabel, statusLabel } from '../lib/utils'
import { EventCard } from './event-card'
import {
  Binary,
  Box,
  Check,
  Fingerprint,
  Plus,
  Search,
  Sparkles,
  X,
} from 'lucide-react'

export function DetailPanel({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const detailPanelOpen = useAppStore((s) => s.detailPanelOpen)
  const setDetailPanelOpen = useAppStore((s) => s.setDetailPanelOpen)
  const actionResults = useAppStore((s) => s.actionResults)
  const runEntityAction = useAppStore((s) => s.runEntityAction)
  const addFinding = useAppStore((s) => s.addFindingFromEntity)
  const addContextChip = useAppStore((s) => s.addContextChip)
  const createIssue = useAppStore((s) => s.createIssue)
  const update = useAppStore((s) => s.updateInvestigation)
  const nodeReviews = useAppStore((s) => s.nodeReviews)
  const eventReviews = useAppStore((s) => s.eventReviews)
  const setReview = useAppStore((s) => s.setReview)
  const graphNodes = useAppStore((s) => s.graphNodes)
  const entities = useAppStore((s) => s.entities)
  const contextEvents = useAppStore((s) => s.contextEvents)
  const appendPdqlFilter = useAppStore((s) => s.appendPdqlFilter)
  const addFieldToContext = useAppStore((s) => s.addFieldToContext)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const executeContextQuery = useAppStore((s) => s.executeContextQuery)
  const contextQueue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue

  if (!inv || !detailPanelOpen) return null

  let entityId: string | undefined
  let eventId: string | undefined
  let selectedGraphNodeId: string | undefined

  if (inv.selectedNodeId) {
    selectedGraphNodeId = inv.selectedNodeId
    const node = graphNodes[inv.selectedNodeId]
    if (node?.kind === 'event') {
      eventId = node.refId
    } else {
      entityId = node?.refId
    }
  } else if (inv.selectedEventId) {
    eventId = inv.selectedEventId
  } else if (inv.selectedEntityIds[0]) {
    entityId = inv.selectedEntityIds[inv.selectedEntityIds.length - 1]
    selectedGraphNodeId = Object.values(graphNodes).find(
      (n) => n.refId === entityId,
    )?.id
  }

  const entity = entityId ? entities[entityId] : null
  const event = eventId ? contextEvents[eventId] : null

  const nodeReview = selectedGraphNodeId
    ? (nodeReviews[selectedGraphNodeId] ??
      graphNodes[selectedGraphNodeId]?.review)
    : undefined
  const eventReview = eventId
    ? (eventReviews[eventId] ?? contextEvents[eventId]?.review)
    : undefined

  if (!entity && !event) {
    return (
      <Panel
        title="Детали"
        className="w-[32rem] shrink-0"
        actions={
          <button type="button" onClick={() => setDetailPanelOpen(false)}>
            <X className="h-3.5 w-3.5 text-fg-dim" />
          </button>
        }
      >
        <div className="flex flex-col items-center gap-2 px-4 py-10 text-center">
          <Search className="h-5 w-5 text-fg-dim" />
          <div className="text-sm text-fg-muted">Здесь появятся детали</div>
          <div className="text-xs text-fg-dim">
            Кликните по узлу на графе, маркеру на таймлайне или строке в таблице
          </div>
        </div>
      </Panel>
    )
  }

  const results = entity ? (actionResults[entity.id] ?? []) : []

  return (
    <Panel
      title={entity ? `Сущность · ${kindLabel[entity.kind]}` : 'Событие'}
      className="w-[32rem] shrink-0"
      actions={
        <button type="button" onClick={() => setDetailPanelOpen(false)}>
          <X className="h-3.5 w-3.5 text-fg-dim" />
        </button>
      }
    >
      <div className="space-y-4 p-3">
        {entity && (
          <>
            <div>
              <div className="font-mono text-sm text-fg">{entity.label}</div>
              <div className="mt-1 text-xs text-fg-dim">{entity.id}</div>
              {nodeReview && (
                <div className="mt-2">
                  <Chip
                    tone={
                      nodeReview === 'proposed'
                        ? 'proposed'
                        : nodeReview === 'rejected'
                          ? 'rejected'
                          : 'confirmed'
                    }
                  >
                    {statusLabel[nodeReview]}
                  </Chip>
                </div>
              )}
            </div>

            {nodeReview === 'proposed' && selectedGraphNodeId && (
              <div className="flex gap-2 rounded border border-proposed/40 bg-proposed/10 p-2">
                <Button
                  size="sm"
                  className="flex-1"
                  onClick={() =>
                    setReview('node', selectedGraphNodeId!, 'confirmed')
                  }
                >
                  <Check className="h-3 w-3" /> Принять
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  className="flex-1"
                  onClick={() =>
                    setReview('node', selectedGraphNodeId!, 'rejected')
                  }
                >
                  <X className="h-3 w-3" /> Отклонить
                </Button>
              </div>
            )}

            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wider text-fg-dim">
                Атрибуты
              </div>
              <dl className="space-y-1">
                {Object.entries(entity.attributes).map(([k, v]) => (
                  <div key={k} className="flex justify-between gap-2 text-xs">
                    <dt className="text-fg-dim">{k}</dt>
                    <dd
                      className="max-w-[160px] truncate text-right font-mono text-fg-muted"
                      title={v}
                    >
                      {v}
                    </dd>
                  </div>
                ))}
              </dl>
            </div>

            <div>
              <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">
                Действия
              </div>
              <div className="flex flex-wrap gap-1.5">
                <Button size="sm" onClick={() => runEntityAction(entity.id, 'enrich')}>
                  <Sparkles className="h-3 w-3" /> Обогатить
                </Button>
                <Button
                  size="sm"
                  onClick={() => runEntityAction(entity.id, 'reputation')}
                >
                  <Fingerprint className="h-3 w-3" /> Репутация
                </Button>
                <Button
                  size="sm"
                  onClick={() => {
                    runEntityAction(entity.id, 'related')
                    if (entity.kind === 'host')
                      addContextChip(investigationId, 'host', entity.label)
                    if (entity.kind === 'ip')
                      addContextChip(investigationId, 'ip', entity.label)
                    if (entity.kind === 'domain')
                      addContextChip(
                        investigationId,
                        'domain',
                        entity.label.replace(/[\[\]]/g, ''),
                      )
                    if (entity.attributes.hash)
                      addContextChip(investigationId, 'hash', entity.attributes.hash)
                  }}
                >
                  <Search className="h-3 w-3" /> Найти связанные
                </Button>
                <Button
                  size="sm"
                  onClick={() => addFinding(investigationId, entity.id)}
                >
                  <Plus className="h-3 w-3" /> В находки
                </Button>
                {(entity.kind === 'process' || entity.kind === 'file_hash') && (
                  <Button
                    size="sm"
                    onClick={() => runEntityAction(entity.id, 'decode')}
                  >
                    <Binary className="h-3 w-3" /> Декодировать
                  </Button>
                )}
                {(entity.kind === 'file_hash' || entity.attributes.hash) && (
                  <Button
                    size="sm"
                    onClick={() => runEntityAction(entity.id, 'sandbox')}
                  >
                    <Box className="h-3 w-3" /> Песочница
                  </Button>
                )}
              </div>
            </div>

            <div>
              <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">
                Исследовательская задача
              </div>
              <Button
                size="sm"
                variant="default"
                onClick={() =>
                  createIssue(investigationId, 'tpl-hash-hunt', [entity.id])
                }
              >
                Создать issue по сущности
              </Button>
            </div>

            <div>
              <label className="flex items-center gap-2 text-xs text-fg-muted">
                <input
                  type="checkbox"
                  checked={inv.selectedEntityIds.includes(entity.id)}
                  onChange={() => {
                    const next = inv.selectedEntityIds.includes(entity.id)
                      ? inv.selectedEntityIds.filter((x) => x !== entity.id)
                      : [...inv.selectedEntityIds, entity.id]
                    update(investigationId, { selectedEntityIds: next })
                  }}
                  className="accent-fg"
                />
                Выбрано для дочернего расследования
              </label>
            </div>

            {results.length > 0 && (
              <div>
                <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">
                  Результаты проверок
                </div>
                <div className="space-y-2">
                  {results.map((r) => (
                    <div
                      key={r.id}
                      className="rounded border border-border bg-surface-2 p-2 text-xs"
                    >
                      <div className="flex justify-between gap-2">
                        <span className="font-medium text-fg">{r.title}</span>
                        <span className="text-fg-dim">{formatTime(r.time)}</span>
                      </div>
                      <p className="mt-1 text-fg-muted">{r.body}</p>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </>
        )}

        {event && (
          <>
            {eventReview && (
              <Chip
                tone={
                  eventReview === 'proposed'
                    ? 'proposed'
                    : eventReview === 'rejected'
                      ? 'rejected'
                      : 'confirmed'
                }
              >
                {statusLabel[eventReview]}
              </Chip>
            )}

            {eventReview === 'proposed' && (
              <div className="flex gap-2 rounded border border-proposed/40 bg-proposed/10 p-2">
                <Button
                  size="sm"
                  className="flex-1"
                  onClick={() => setReview('event', event.id, 'confirmed')}
                >
                  <Check className="h-3 w-3" /> Принять
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  className="flex-1"
                  onClick={() => setReview('event', event.id, 'rejected')}
                >
                  <X className="h-3 w-3" /> Отклонить
                </Button>
              </div>
            )}

            <EventCard
              event={{
                id: event.id,
                time: event.time,
                title: event.title,
                description: event.description,
                source: event.source,
                severity: event.severity,
                raw: event.raw ?? {},
                sourceEventId: event.sourceEventId,
              }}
              investigationId={investigationId}
              eventInContext={inv.eventIds.includes(event.id)}
              timeInterval={contextQueue.timeInterval}
              onTimeChange={(interval) =>
                setContextQueue(investigationId, { timeInterval: interval })
              }
              onTimeExecute={(interval) => {
                setContextQueue(investigationId, { timeInterval: interval })
                void executeContextQuery(investigationId)
              }}
              onAddFilter={(field, value) => appendPdqlFilter(investigationId, field, value)}
              onAddToContext={(field, value, includeEvent) =>
                addFieldToContext(investigationId, {
                  field,
                  value,
                  eventId: event.id,
                  includeEvent,
                })
              }
            />
          </>
        )}
      </div>
    </Panel>
  )
}
