import {
  emptyContextQueue,
  useAppStore,
} from '../store/appStore'
import { ContextQueueToolbar } from './ContextQueue'
import { Button, Chip, SeverityBadge } from '../components/ui'
import { clsx, eventOriginLabel, formatTime, matchesOriginFilter, statusLabel } from '../lib/utils'
import { fieldForEntityKind } from '../lib/filters'
import { Check, Inbox, Network, Table2, X } from 'lucide-react'

export function InvestigationHeader({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const parent = useAppStore((s) =>
    inv?.parentId ? s.investigations[inv.parentId] : null,
  )
  const issues = useAppStore((s) => s.issues)
  const update = useAppStore((s) => s.updateInvestigation)
  const setActiveTab = useAppStore((s) => s.setActiveTab)

  if (!inv) return null

  const running = inv.issueIds.some((id) => issues[id]?.status === 'running')

  return (
    <div className="border-b border-border bg-surface-1 px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-sm font-semibold">{inv.title}</h1>
            <SeverityBadge severity={inv.severity} />
            <Chip>{statusLabel[inv.status]}</Chip>
            <span className="text-xs text-fg-dim">аналитик: {inv.assignee}</span>
            {running && (
              <span className="inline-flex items-center gap-1.5 text-xs text-proposed">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-proposed" />
                фоновые задачи
              </span>
            )}
          </div>
          {parent && (
            <button
              type="button"
              className="mt-1 text-xs text-fg-muted hover:text-fg"
              onClick={() => setActiveTab(parent.id)}
            >
              ← родитель: {parent.title}
            </button>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="flex rounded border border-border p-0.5">
            <button
              type="button"
              className={clsx(
                'inline-flex items-center gap-1 rounded px-2 py-1 text-xs',
                inv.view === 'table' ? 'bg-surface-3 text-fg' : 'text-fg-muted',
              )}
              onClick={() => update(investigationId, { view: 'table' })}
            >
              <Table2 className="h-3 w-3" />
              Таблица
            </button>
            <button
              type="button"
              className={clsx(
                'inline-flex items-center gap-1 rounded px-2 py-1 text-xs',
                inv.view === 'graph' ? 'bg-surface-3 text-fg' : 'text-fg-muted',
              )}
              onClick={() => update(investigationId, { view: 'graph' })}
            >
              <Network className="h-3 w-3" />
              Граф + таймлайн
            </button>
            <button
              type="button"
              className={clsx(
                'inline-flex items-center gap-1 rounded px-2 py-1 text-xs',
                inv.view === 'queue' ? 'bg-surface-3 text-fg' : 'text-fg-muted',
              )}
              onClick={() => update(investigationId, { view: 'queue' })}
            >
              <Inbox className="h-3 w-3" />
              Очередь
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function ContextTable({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const eventReviews = useAppStore((s) => s.eventReviews)
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const setReview = useAppStore((s) => s.setReview)
  const update = useAppStore((s) => s.updateInvestigation)
  const addContextChip = useAppStore((s) => s.addContextChip)
  const contextEvents = useAppStore((s) => s.contextEvents)
  const entities = useAppStore((s) => s.entities)

  if (!inv) return null

  const rows = inv.eventIds
    .map((id) => contextEvents[id])
    .filter(Boolean)
    .filter((ev) => matchesOriginFilter(ev, queue.originFilter))
    .filter((ev) => {
      if (queue.reviewFilter === 'all') return true
      return (eventReviews[ev.id] ?? ev.review) === queue.reviewFilter
    })
    .sort((a, b) => a.time.localeCompare(b.time))

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ContextQueueToolbar investigationId={investigationId} />
      <div className="min-h-0 flex-1 overflow-auto">
      <table className="w-full min-w-[900px] border-collapse text-left">
        <thead className="sticky top-0 bg-surface-1 text-[11px] uppercase tracking-wider text-fg-dim">
          <tr className="border-b border-border">
            <th className="px-3 py-2">Время</th>
            <th className="px-3 py-2">Источник</th>
            <th className="px-3 py-2">Тип</th>
            <th className="px-3 py-2">Событие</th>
            <th className="px-3 py-2">Сущности</th>
            <th className="px-3 py-2">Происхождение</th>
            <th className="px-3 py-2">Статус</th>
            <th className="px-3 py-2">Действия</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 && (
            <tr>
              <td colSpan={8} className="px-3 py-6 text-center text-sm text-fg-dim">
                Нет событий под выбранные фильтры
              </td>
            </tr>
          )}
          {rows.map((ev) => {
            const review = eventReviews[ev.id] ?? ev.review
            return (
              <tr
                key={ev.id}
                className={clsx(
                  'cursor-pointer border-b border-border/60 hover:bg-surface-2/50',
                  review === 'proposed' && 'bg-proposed/5',
                  review === 'rejected' && 'opacity-40',
                  inv.selectedEventId === ev.id && 'bg-surface-2',
                )}
                onClick={() =>
                  update(investigationId, {
                    selectedEventId: ev.id,
                    selectedNodeId: undefined,
                  })
                }
              >
                <td className="whitespace-nowrap px-3 py-2 font-mono text-xs text-fg-muted">
                  {formatTime(ev.time)}
                </td>
                <td className="px-3 py-2">
                  <span className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px]">
                    {ev.source}
                  </span>
                </td>
                <td className="px-3 py-2 font-mono text-xs text-fg-dim">{ev.type}</td>
                <td className="px-3 py-2">
                  <div className="flex items-center gap-2 text-sm">
                    <SeverityBadge severity={ev.severity} label="" />
                    {ev.title}
                  </div>
                </td>
                <td className="px-3 py-2">
                  <div className="flex flex-wrap gap-1">
                    {ev.entityIds.slice(0, 3).map((id) => {
                      const e = entities[id]
                      if (!e) return null
                      return (
                        <button
                          key={id}
                          type="button"
                          className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-fg-muted hover:text-fg"
                          onClick={(evt) => {
                            evt.stopPropagation()
                            const field = fieldForEntityKind(e.kind)
                            if (field)
                              addContextChip(
                                investigationId,
                                field,
                                e.label.replace(/[\[\]]/g, ''),
                              )
                            update(investigationId, {
                              selectedEntityIds: inv.selectedEntityIds.includes(id)
                                ? inv.selectedEntityIds.filter((x) => x !== id)
                                : [...inv.selectedEntityIds, id],
                            })
                          }}
                        >
                          {e.label}
                        </button>
                      )
                    })}
                  </div>
                </td>
                <td className="px-3 py-2 text-xs text-fg-muted">{eventOriginLabel(ev)}</td>
                <td className="px-3 py-2">
                  <Chip
                    tone={
                      review === 'proposed'
                        ? 'proposed'
                        : review === 'rejected'
                          ? 'rejected'
                          : 'confirmed'
                    }
                  >
                    {statusLabel[review]}
                  </Chip>
                </td>
                <td className="px-3 py-2">
                  {review === 'proposed' && (
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={(e) => {
                          e.stopPropagation()
                          setReview('event', ev.id, 'confirmed')
                        }}
                      >
                        <Check className="h-3 w-3 text-confirmed" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={(e) => {
                          e.stopPropagation()
                          setReview('event', ev.id, 'rejected')
                        }}
                      >
                        <X className="h-3 w-3 text-critical" />
                      </Button>
                    </div>
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      </div>
    </div>
  )
}
