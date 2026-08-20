import { useAppStore, emptyContextQueue } from '../store/appStore'
import type { EventOrigin, ReviewState } from '../types'
import { FilterBar } from './FilterBar'
import { Button, Chip, SeverityBadge } from './ui'
import { clsx, formatTime, statusLabel } from '../lib/utils'
import { fieldForEntityKind, matchesChips } from '../lib/filters'
import { Check, Plus, X } from 'lucide-react'

const ORIGIN_FILTERS: Array<{ id: EventOrigin | 'all'; label: string }> = [
  { id: 'all', label: 'все' },
  { id: 'seed', label: 'исходные' },
  { id: 'agent', label: 'агент' },
  { id: 'analyst', label: 'аналитик' },
  { id: 'rule', label: 'правило' },
]

const REVIEW_FILTERS: Array<{ id: ReviewState | 'all'; label: string }> = [
  { id: 'all', label: 'все' },
  { id: 'proposed', label: 'предложенные' },
  { id: 'confirmed', label: 'подтверждённые' },
  { id: 'rejected', label: 'отклонённые' },
]

/** Filter/bulk-review toolbar shown above the context table view. */
export function ContextQueueToolbar({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const eventReviews = useAppStore((s) => s.eventReviews)
  const contextEvents = useAppStore((s) => s.contextEvents)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const setReview = useAppStore((s) => s.setReview)
  const update = useAppStore((s) => s.updateInvestigation)

  if (!inv) return null

  // Proposed events among the currently visible rows — bulk review applies to them
  const visibleProposedIds = inv.eventIds.filter((id) => {
    const ev = contextEvents[id]
    if (!ev) return false
    if (queue.originFilter !== 'all' && ev.origin !== queue.originFilter) return false
    const review = eventReviews[id] ?? ev.review
    if (queue.reviewFilter !== 'all' && review !== queue.reviewFilter) return false
    return review === 'proposed'
  })

  return (
    <div className="flex flex-wrap items-center gap-3 border-b border-border bg-surface-1 px-3 py-2">
      <div className="flex items-center gap-1.5 text-xs text-fg-dim">
        происхождение:
        <div className="flex rounded border border-border p-0.5">
          {ORIGIN_FILTERS.map((f) => (
            <button
              key={f.id}
              type="button"
              className={clsx(
                'rounded px-2 py-0.5 text-xs',
                queue.originFilter === f.id
                  ? 'bg-surface-3 text-fg'
                  : 'text-fg-muted hover:text-fg',
              )}
              onClick={() => setContextQueue(investigationId, { originFilter: f.id })}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex items-center gap-1.5 text-xs text-fg-dim">
        статус:
        <div className="flex rounded border border-border p-0.5">
          {REVIEW_FILTERS.map((f) => (
            <button
              key={f.id}
              type="button"
              className={clsx(
                'rounded px-2 py-0.5 text-xs',
                queue.reviewFilter === f.id
                  ? 'bg-surface-3 text-fg'
                  : 'text-fg-muted hover:text-fg',
              )}
              onClick={() => setContextQueue(investigationId, { reviewFilter: f.id })}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      <div className="ml-auto flex items-center gap-2">
        {visibleProposedIds.length > 0 && (
          <>
            <span className="text-xs text-proposed">
              предложено: {visibleProposedIds.length}
            </span>
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                visibleProposedIds.forEach((id) => setReview('event', id, 'confirmed'))
              }
            >
              <Check className="h-3 w-3 text-confirmed" />
              Принять все
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                visibleProposedIds.forEach((id) => setReview('event', id, 'rejected'))
              }
            >
              <X className="h-3 w-3 text-critical" />
              Отклонить все
            </Button>
          </>
        )}
        <Button
          size="sm"
          variant="primary"
          onClick={() => update(investigationId, { view: 'queue' })}
        >
          <Plus className="h-3.5 w-3.5" />
          Добавить события
        </Button>
      </div>
    </div>
  )
}

/**
 * Full-page queue view of the investigation: searches project events from all
 * sources, with per-investigation search history and an add action.
 */
export function ContextQueuePage({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const eventReviews = useAppStore((s) => s.eventReviews)
  const contextEvents = useAppStore((s) => s.contextEvents)
  const entities = useAppStore((s) => s.entities)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const addContextChip = useAppStore((s) => s.addContextChip)
  const removeContextChip = useAppStore((s) => s.removeContextChip)
  const removeContextChipValue = useAppStore((s) => s.removeContextChipValue)
  const clearContextChips = useAppStore((s) => s.clearContextChips)
  const addEventsToContext = useAppStore((s) => s.addEventsToContext)
  const setReview = useAppStore((s) => s.setReview)

  if (!inv) return null

  const rows = Object.values(contextEvents)
    .filter((ev) =>
      matchesChips(ev.entityIds, ev.severity, ev.source, '', queue.chips, entities),
    )
    .filter((ev) => !queue.hideAdded || !inv.eventIds.includes(ev.id))
    .sort((a, b) => a.time.localeCompare(b.time))

  const addedCount = rows.filter((ev) => inv.eventIds.includes(ev.id)).length

  const toggleSelect = (id: string) => {
    setContextQueue(investigationId, {
      selectedIds: queue.selectedIds.includes(id)
        ? queue.selectedIds.filter((x) => x !== id)
        : [...queue.selectedIds, id],
    })
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Same search/filter row as the global queue, scoped to this investigation */}
      <FilterBar
        chips={queue.chips}
        timePreset={queue.timePreset}
        onAddChip={(field, value) => addContextChip(investigationId, field, value)}
        onRemoveChip={(chipId) => removeContextChip(investigationId, chipId)}
        onRemoveChipValue={(chipId, value) =>
          removeContextChipValue(investigationId, chipId, value)
        }
        onClearChips={() => clearContextChips(investigationId)}
        onTimePresetChange={(timePreset) =>
          setContextQueue(investigationId, { timePreset })
        }
        history={queue.history}
        extra={
          <button
            type="button"
            className={clsx(
              'rounded border px-2 py-1.5 text-xs',
              queue.hideAdded
                ? 'border-fg/30 bg-surface-3 text-fg'
                : 'border-border text-fg-muted hover:text-fg',
            )}
            onClick={() => setContextQueue(investigationId, { hideAdded: !queue.hideAdded })}
            title="Показывать только события вне контекста"
          >
            скрыть добавленные
          </button>
        }
      />

      {/* Result summary + selection actions */}
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <div className="flex items-center gap-3 text-sm">
          <span className="text-fg-muted">
            Событий: <span className="text-fg">{rows.length}</span>
          </span>
          {!queue.hideAdded && (
            <span className="text-fg-muted">
              в контексте: <span className="text-fg">{addedCount}</span>
            </span>
          )}
        </div>
        {queue.selectedIds.length > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-xs text-fg-muted">
              Выбрано: {queue.selectedIds.length}
            </span>
            <Button
              size="sm"
              variant="primary"
              onClick={() => addEventsToContext(investigationId, queue.selectedIds)}
            >
              <Plus className="h-3 w-3" />
              Добавить в расследование
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setContextQueue(investigationId, { selectedIds: [] })}
            >
              Сбросить
            </Button>
          </div>
        )}
      </div>

      {/* Full events table */}
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full min-w-[960px] border-collapse text-left">
          <thead className="sticky top-0 z-10 bg-surface-1 text-[11px] uppercase tracking-wider text-fg-dim">
            <tr className="border-b border-border">
              <th className="w-10 px-3 py-2" />
              <th className="px-3 py-2">Крит.</th>
              <th className="px-3 py-2">Время</th>
              <th className="px-3 py-2">Событие</th>
              <th className="px-3 py-2">Тип</th>
              <th className="px-3 py-2">Сущности</th>
              <th className="px-3 py-2">Источник</th>
              <th className="px-3 py-2">Статус</th>
              <th className="w-28 px-3 py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((ev) => {
              const inContext = inv.eventIds.includes(ev.id)
              const review = inContext ? (eventReviews[ev.id] ?? ev.review) : null
              const selected = queue.selectedIds.includes(ev.id)
              return (
                <tr
                  key={ev.id}
                  className={clsx(
                    'border-b border-border/60',
                    inContext ? 'bg-surface-0/40' : 'cursor-pointer hover:bg-surface-2/60',
                    selected && 'bg-surface-2',
                  )}
                  onClick={() => {
                    if (!inContext) toggleSelect(ev.id)
                  }}
                >
                  <td className="px-3 py-2">
                    <input
                      type="checkbox"
                      className="accent-fg"
                      checked={selected}
                      disabled={inContext}
                      onChange={() => toggleSelect(ev.id)}
                      onClick={(e) => e.stopPropagation()}
                    />
                  </td>
                  <td className="px-3 py-2">
                    <SeverityBadge severity={ev.severity} />
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 font-mono text-xs text-fg-muted">
                    {formatTime(ev.time)}
                  </td>
                  <td className="px-3 py-2">
                    <div className={clsx('text-sm', inContext && 'text-fg-muted')}>
                      {ev.title}
                    </div>
                    <div className="text-xs text-fg-dim">{ev.description}</div>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-fg-dim">{ev.type}</td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-1">
                      {ev.entityIds.slice(0, 4).map((id) => {
                        const e = entities[id]
                        if (!e) return null
                        const field = fieldForEntityKind(e.kind)
                        return (
                          <button
                            key={id}
                            type="button"
                            className="rounded border border-border bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-fg-muted hover:border-fg/30 hover:text-fg"
                            title="Найти связанные"
                            onClick={(evt) => {
                              evt.stopPropagation()
                              if (field)
                                addContextChip(
                                  investigationId,
                                  field,
                                  e.label.replace(/[\[\]]/g, ''),
                                )
                            }}
                          >
                            <span className="text-fg-dim">{e.kind}:</span> {e.label}
                          </button>
                        )
                      })}
                      {ev.entityIds.length > 4 && (
                        <span className="text-[11px] text-fg-dim">
                          +{ev.entityIds.length - 4}
                        </span>
                      )}
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <span className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px]">
                      {ev.source}
                    </span>
                  </td>
                  <td className="px-3 py-2">
                    {inContext ? (
                      <Chip
                        tone={
                          review === 'proposed'
                            ? 'proposed'
                            : review === 'rejected'
                              ? 'rejected'
                              : 'confirmed'
                        }
                      >
                        {review === 'confirmed' ? 'в контексте' : statusLabel[review!]}
                      </Chip>
                    ) : (
                      <span className="text-xs text-fg-dim">—</span>
                    )}
                  </td>
                  <td className="px-3 py-2">
                    {!inContext && (
                      <Button
                        size="sm"
                        variant="ghost"
                        title="Добавить в расследование"
                        onClick={(e) => {
                          e.stopPropagation()
                          addEventsToContext(investigationId, [ev.id])
                        }}
                      >
                        <Plus className="h-3 w-3" />
                      </Button>
                    )}
                    {inContext && review === 'proposed' && (
                      <div className="flex gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          title="Принять"
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
                          title="Отклонить"
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
            {rows.length === 0 && (
              <tr>
                <td colSpan={9} className="px-4 py-12 text-center text-fg-dim">
                  Ничего не найдено
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <div className="border-t border-border px-4 py-1.5 text-[11px] text-fg-dim">
        Очередь событий этого расследования · поиск по всем источникам проекта ·
        история фильтров сохраняется для этого контекста
      </div>
    </div>
  )
}
