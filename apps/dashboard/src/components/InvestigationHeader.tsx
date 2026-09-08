import {
  emptyContextQueue,
  useAppStore,
} from '../store/appStore'
import { ContextQueueToolbar } from './ContextQueue'
import { Button, Chip } from './ui'
import {
  acceptEdgeWithNodes,
  edgeReviewState,
  filterInvestigationEdges,
} from '../lib/edge-review'
import { clsx, kindLabel, statusLabel } from '../lib/utils'
import { Check, Eye, EyeOff, X } from 'lucide-react'

const EMPTY_HIDDEN_NODE_IDS: string[] = []

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
  const edgeReviews = useAppStore((s) => s.edgeReviews)
  const nodeReviews = useAppStore((s) => s.nodeReviews)
  const queue = useAppStore((s) => s.contextQueue[investigationId]) ?? emptyContextQueue
  const setReview = useAppStore((s) => s.setReview)
  const update = useAppStore((s) => s.updateInvestigation)
  const graphNodes = useAppStore((s) => s.graphNodes)
  const graphEdges = useAppStore((s) => s.graphEdges)
  const hiddenGraphNodeIds = useAppStore(
    (s) => s.hiddenGraphNodeIds[investigationId] ?? EMPTY_HIDDEN_NODE_IDS,
  )
  const toggleGraphNodeHidden = useAppStore((s) => s.toggleGraphNodeHidden)

  if (!inv) return null

  const rows = filterInvestigationEdges(
    inv.edgeIds,
    graphEdges,
    edgeReviews,
    queue,
  ).sort((a, b) => {
    const sourceA = graphNodes[a.source]?.label ?? ''
    const sourceB = graphNodes[b.source]?.label ?? ''
    return sourceA.localeCompare(sourceB, 'ru')
  })

  const nodeLabel = (nodeId: string) => {
    const node = graphNodes[nodeId]
    if (!node) return nodeId
    const kind = kindLabel[node.kind] ?? node.kind
    return `${node.label} · ${kind}`
  }

  const clampCell =
    'line-clamp-3 break-words [overflow-wrap:anywhere] [word-break:break-word]'

  return (
    <div className="flex h-full min-h-0 flex-col">
      <ContextQueueToolbar investigationId={investigationId} />
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full min-w-[900px] table-fixed border-collapse text-left">
          <thead className="sticky top-0 z-10 bg-surface-1 text-[11px] uppercase tracking-wider text-fg-muted">
            <tr className="border-b border-border">
              <th className="w-[10rem] max-w-[10rem] px-3 py-2">Цель</th>
              <th className="w-[8rem] px-3 py-2">Связь</th>
              <th className="px-3 py-2">Источник</th>
              <th className="w-[8rem] px-3 py-2">Происхождение</th>
              <th className="w-[11rem] px-3 py-2">Статус</th>
              <th className="w-[5rem] px-3 py-2 text-center">Действия</th>
              <th className="w-[20rem] max-w-[20rem] px-3 py-2">Обоснование</th>
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 && (
              <tr>
                <td colSpan={7} className="px-3 py-6 text-center text-sm text-fg-dim">
                  Нет связей под выбранные фильтры
                </td>
              </tr>
            )}
            {rows.map((edge) => {
              const review = edgeReviewState(edge, edgeReviews)
              const origin = edge.origin ?? 'agent'
              return (
                <tr
                  key={edge.id}
                  className={clsx(
                    'cursor-pointer border-b border-border/60 hover:bg-surface-2/50',
                    review === 'proposed' && 'bg-proposed/5',
                    review === 'rejected' && 'opacity-40',
                    inv.selectedNodeId === edge.target && 'bg-surface-2',
                  )}
                  onClick={() =>
                    update(investigationId, {
                      selectedNodeId: edge.target,
                      selectedEventId: undefined,
                    })
                  }
                >
                  <td className="max-w-[18rem] px-3 py-2 align-middle text-xs text-fg line-clamp-3">
                    {nodeLabel(edge.target)}
                  </td>
                  <td className="px-3 py-2 align-middle text-xs text-proposed">
                    {edge.relation}
                  </td>
                  <td className="max-w-[18rem] px-3 py-2 align-middle">
                    <div
                      className={clsx(clampCell, 'text-xs text-fg')}
                      title={nodeLabel(edge.source)}
                    >
                      {nodeLabel(edge.source)}
                    </div>
                  </td>
                  <td className="px-3 py-2 align-middle text-xs text-fg-muted">
                    {statusLabel[origin] ?? origin}
                  </td>
                  <td className="px-3 py-2 align-middle">
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
                  <td className="px-3 py-2 align-middle text-center">
                    {review === 'proposed' ? (
                      <div className="flex justify-center gap-1">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={(e) => {
                            e.stopPropagation()
                            acceptEdgeWithNodes(
                              setReview,
                              edge,
                              investigationId,
                              graphNodes,
                              nodeReviews,
                            )
                          }}
                        >
                          <Check className="h-3 w-3 text-confirmed" />
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={(e) => {
                            e.stopPropagation()
                            setReview('edge', edge.id, 'rejected', investigationId)
                          }}
                        >
                          <X className="h-3 w-3 text-critical" />
                        </Button>
                      </div>
                    ) : (
                      <Button
                        size="sm"
                        variant="ghost"
                        title={
                          hiddenGraphNodeIds.includes(edge.target)
                            ? 'Показать на графе'
                            : 'Скрыть на графе'
                        }
                        aria-label={
                          hiddenGraphNodeIds.includes(edge.target)
                            ? 'Показать на графе'
                            : 'Скрыть на графе'
                        }
                        onClick={(e) => {
                          e.stopPropagation()
                          toggleGraphNodeHidden(investigationId, edge.target)
                        }}
                      >
                        {hiddenGraphNodeIds.includes(edge.target) ? (
                          <EyeOff className="h-3 w-3 text-fg-dim" />
                        ) : (
                          <Eye className="h-3 w-3 text-fg-muted" />
                        )}
                      </Button>
                    )}
                  </td>
                  <td className="max-w-[20rem] px-3 py-2 align-middle">
                    {edge.rationale ? (
                      <div
                        className={clsx(clampCell, 'text-xs text-fg-muted')}
                        title={edge.rationale}
                      >
                        {edge.rationale}
                      </div>
                    ) : null}
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
