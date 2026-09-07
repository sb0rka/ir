import { useEffect } from 'react'
import { useAppStore, emptyContextQueue } from '../store/appStore'
import type { EventOrigin, ReviewState } from '../types'
import { ContextQueryComposer } from './QueryComposer'
import { AlertTable } from './AlertTable'
import { EventGroupFilter } from './EventGroupFilter'
import { Button } from './ui'
import {
  acceptEdgesWithNodes,
  filterInvestigationEdges,
  proposedInvestigationEdgeIds,
} from '../lib/edge-review'
import { clsx } from '../lib/utils'
import { filterFingerprint } from '../lib/queryFingerprint'
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
  { id: 'confirmed', label: 'подтвержденные' },
  { id: 'rejected', label: 'отклоненные' },
]

/** Filter/bulk-review toolbar shown above the context table view. */
export function ContextQueueToolbar({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const edgeReviews = useAppStore((s) => s.edgeReviews)
  const nodeReviews = useAppStore((s) => s.nodeReviews)
  const graphEdges = useAppStore((s) => s.graphEdges)
  const graphNodes = useAppStore((s) => s.graphNodes)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const setReview = useAppStore((s) => s.setReview)
  const update = useAppStore((s) => s.updateInvestigation)

  if (!inv) return null

  const visibleProposedIds = proposedInvestigationEdgeIds(
    inv.edgeIds,
    graphEdges,
    edgeReviews,
    queue,
  )
  const visibleProposedEdges = filterInvestigationEdges(
    visibleProposedIds,
    graphEdges,
    edgeReviews,
    { ...queue, reviewFilter: 'proposed' },
  )

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
                acceptEdgesWithNodes(
                  setReview,
                  visibleProposedEdges,
                  investigationId,
                  graphNodes,
                  nodeReviews,
                )
              }
            >
              <Check className="h-3 w-3 text-confirmed" />
              Принять все
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                visibleProposedIds.forEach((id) =>
                  setReview('edge', id, 'rejected', investigationId),
                )
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
 * Full-page queue view of the investigation: same Gateway search as the global
 * queue, with rows already in this investigation highlighted.
 */
export function ContextQueuePage({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const executeContextQuery = useAppStore((s) => s.executeContextQuery)

  useEffect(() => {
    const current = useAppStore.getState().contextQueue[investigationId] ?? emptyContextQueue
    const fingerprint = filterFingerprint(
      current.pdql,
      current.timeInterval,
      current.queueSource,
      current.groupValues,
    )
    if (current.loading) return
    if (current.executedFingerprint === fingerprint) return
    void executeContextQuery(investigationId)
  }, [investigationId, executeContextQuery])

  if (!inv) return null

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ContextQueryComposer investigationId={investigationId} />
      <div className="flex min-h-0 flex-1">
        <EventGroupFilter investigationId={investigationId} />
        <div className="min-h-0 min-w-0 flex-1">
          <AlertTable investigationId={investigationId} />
        </div>
      </div>
    </div>
  )
}
