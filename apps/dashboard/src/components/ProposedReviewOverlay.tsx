import {
  graphEdges,
  graphNodes,
  useAppStore,
} from '../store/appStore'
import { Button, Chip } from './ui'
import { Check, X } from 'lucide-react'

/** Accept / reject AI-proposed nodes and edges while on the graph view. */
export function ProposedReviewOverlay({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const nodeReviews = useAppStore((s) => s.nodeReviews)
  const edgeReviews = useAppStore((s) => s.edgeReviews)
  const setReview = useAppStore((s) => s.setReview)

  if (!inv) return null

  const selectedNode = inv.selectedNodeId ? graphNodes[inv.selectedNodeId] : null
  const selectedReview = selectedNode
    ? (nodeReviews[selectedNode.id] ?? selectedNode.review)
    : null

  const proposedEdges = inv.edgeIds
    .map((id) => graphEdges[id])
    .filter(Boolean)
    .filter((e) => (edgeReviews[e.id] ?? e.review) === 'proposed')
    .filter((e) => {
      if (!inv.selectedNodeId) return true
      return e.source === inv.selectedNodeId || e.target === inv.selectedNodeId
    })
    .slice(0, 6)

  const proposedNodes = inv.nodeIds
    .map((id) => graphNodes[id])
    .filter(Boolean)
    .filter((n) => (nodeReviews[n.id] ?? n.review) === 'proposed')

  if (
    proposedEdges.length === 0 &&
    proposedNodes.length === 0 &&
    selectedReview !== 'proposed'
  ) {
    return null
  }

  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-[172px] z-20 flex flex-col gap-2 p-3">
      {selectedNode && selectedReview === 'proposed' && (
        <div className="pointer-events-auto flex w-fit items-center gap-2 rounded border border-proposed/40 bg-surface-1/95 px-2 py-1.5 text-xs shadow-lg backdrop-blur">
          <Chip tone="proposed">предложено агентом</Chip>
          <span className="text-fg-muted">{selectedNode.label}</span>
          <Button
            size="sm"
            onClick={() => setReview('node', selectedNode.id, 'confirmed')}
          >
            <Check className="h-3 w-3" /> Принять
          </Button>
          <Button
            size="sm"
            variant="danger"
            onClick={() => setReview('node', selectedNode.id, 'rejected')}
          >
            <X className="h-3 w-3" /> Отклонить
          </Button>
        </div>
      )}

      {proposedEdges.length > 0 && (
        <div className="pointer-events-auto max-w-lg space-y-1 rounded border border-border bg-surface-1/95 p-2 text-xs shadow-lg backdrop-blur">
          <div className="text-[10px] uppercase tracking-wider text-fg-dim">
            Предложенные связи
            {proposedNodes.length > 0 && (
              <span className="ml-2 text-proposed">
                · узлов: {proposedNodes.length}
              </span>
            )}
          </div>
          {proposedEdges.map((e) => (
            <div key={e.id} className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="text-fg-muted">
                  {graphNodes[e.source]?.label} —{e.relation}→{' '}
                  {graphNodes[e.target]?.label}
                </div>
                {e.rationale && (
                  <div className="text-[11px] text-fg-dim">{e.rationale}</div>
                )}
              </div>
              <div className="flex shrink-0 gap-1">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setReview('edge', e.id, 'confirmed')
                    // Also confirm endpoints if they were proposed
                    const src = graphNodes[e.source]
                    const tgt = graphNodes[e.target]
                    if (src && (nodeReviews[src.id] ?? src.review) === 'proposed') {
                      setReview('node', src.id, 'confirmed')
                    }
                    if (tgt && (nodeReviews[tgt.id] ?? tgt.review) === 'proposed') {
                      setReview('node', tgt.id, 'confirmed')
                    }
                  }}
                >
                  <Check className="h-3 w-3 text-confirmed" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setReview('edge', e.id, 'rejected')}
                >
                  <X className="h-3 w-3 text-critical" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
