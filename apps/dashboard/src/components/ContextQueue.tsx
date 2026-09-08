import { useEffect } from 'react'
import { useAppStore, emptyContextQueue } from '../store/appStore'
import type { EventOrigin, ReviewState } from '../types'
import { ContextQueryComposer } from './QueryComposer'
import { AlertTable } from './AlertTable'
import { EventGroupFilter } from './EventGroupFilter'
import { Button, Select } from './ui'
import {
  acceptEdgesWithNodes,
  filterInvestigationEdges,
  proposedInvestigationEdgeIds,
} from '../lib/edge-review'
import { filterFingerprint } from '../lib/queryFingerprint'
import { Check, X } from 'lucide-react'

const ORIGIN_OPTIONS = [
  { value: 'all', label: 'Все происхождения' },
  { value: 'seed', label: 'Исходные' },
  { value: 'agent', label: 'Агент' },
  { value: 'analyst', label: 'Аналитик' },
  { value: 'rule', label: 'Правило' },
] as const satisfies ReadonlyArray<{ value: EventOrigin | 'all'; label: string }>

const REVIEW_OPTIONS = [
  { value: 'all', label: 'Все статусы' },
  { value: 'proposed', label: 'Предложенные' },
  { value: 'confirmed', label: 'Подтвержденные' },
  { value: 'rejected', label: 'Отклоненные' },
] as const satisfies ReadonlyArray<{ value: ReviewState | 'all'; label: string }>

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
      <div className="flex flex-wrap items-center gap-2">
        <Select
          aria-label="Происхождение"
          value={queue.originFilter}
          options={ORIGIN_OPTIONS}
          onChange={(originFilter) => setContextQueue(investigationId, { originFilter })}
        />
        <Select
          aria-label="Статус"
          value={queue.reviewFilter}
          options={REVIEW_OPTIONS}
          onChange={(reviewFilter) => setContextQueue(investigationId, { reviewFilter })}
        />
      </div>

      <div className="ml-auto flex items-center gap-2">
        {visibleProposedIds.length > 0 && (
          <>
            <span className="text-xs text-proposed">
              Предложено: {visibleProposedIds.length}
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
