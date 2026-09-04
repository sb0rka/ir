import {
  emptyContextQueue,
  useAppStore,
} from '../store/appStore'
import { ContextQueueToolbar } from './ContextQueue'
import { Button, Chip, SeverityBadge } from '../components/ui'
import { clsx, eventOriginLabel, formatTime, matchesOriginFilter, statusLabel, verdictLabel } from '../lib/utils'
import { Check, X } from 'lucide-react'

/** Compact investigation identity for the app header (between logo and actions). */
export function InvestigationHeader({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const parent = useAppStore((s) =>
    inv?.parentId ? s.investigations[inv.parentId] : null,
  )
  const issues = useAppStore((s) => s.issues)
  const openInvestigationTab = useAppStore((s) => s.openInvestigationTab)

  if (!inv) return null

  const running = inv.issueIds.some((id) => issues[id]?.status === 'running')

  return (
    <div className="flex min-w-0 flex-1 items-baseline gap-2">
      <SeverityBadge severity={inv.severity} />
      <span className="shrink-0 text-xs text-fg-muted">{statusLabel[inv.status]}</span>
      {inv.verdict ? (
        <span className="shrink-0 text-xs text-fg-dim">
          {verdictLabel[inv.verdict] ?? inv.verdict}
        </span>
      ) : null}
      <h1 className="min-w-0 truncate text-xs font-medium">{inv.title}</h1>
      {running && (
        <span className="inline-flex shrink-0 items-center gap-1.5 text-xs text-proposed">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-proposed" />
          фоновые задачи
        </span>
      )}
      {parent && (
        <button
          type="button"
          className="shrink-0 truncate text-xs text-fg-muted hover:text-fg"
          onClick={() => openInvestigationTab(parent.id)}
        >
          ← {parent.title}
        </button>
      )}
    </div>
  )
}

export function ContextTable({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const eventReviews = useAppStore((s) => s.eventReviews)
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const setReview = useAppStore((s) => s.setReview)
  const update = useAppStore((s) => s.updateInvestigation)
  const contextEvents = useAppStore((s) => s.contextEvents)

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
            <th className="px-3 py-2">Тип</th>
            <th className="w-[41%] px-3 py-2">Событие</th>
            <th className="px-3 py-2">Происхождение</th>
            <th className="min-w-[11rem] px-3 py-2">Статус</th>
            <th className="px-3 py-2">Действия</th>
            <th className="px-3 py-2">Произошло</th>
            <th className="px-3 py-2">Источник</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 && (
            <tr>
              <td colSpan={7} className="px-3 py-6 text-center text-sm text-fg-dim">
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
                <td className="px-3 py-2 font-mono text-xs text-fg-dim">{ev.type}</td>
                <td className="w-[41%] px-3 py-2">
                  <div className="flex items-center gap-2 text-sm">
                    <SeverityBadge severity={ev.severity} label="" />
                    {ev.title}
                  </div>
                </td>
                <td className="px-3 py-2 text-xs text-fg-muted">{eventOriginLabel(ev)}</td>
                <td className="min-w-[11rem] px-3 py-2">
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
                <td className="whitespace-nowrap px-3 py-2 font-mono text-xs text-fg-muted">
                  {formatTime(ev.time)}
                </td>
                <td
                  className="max-w-0 overflow-hidden whitespace-nowrap px-3 py-2"
                  title={ev.source}
                >
                  <span className="block truncate font-mono text-[11px] text-fg-muted">
                    {ev.source}
                  </span>
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
