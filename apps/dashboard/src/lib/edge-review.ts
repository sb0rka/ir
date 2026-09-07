import type { EventOrigin, GraphEdge, GraphNode, ReviewState } from '../types'

export type EdgeQueueFilters = {
  originFilter: EventOrigin | 'all'
  reviewFilter: ReviewState | 'all'
}

export function edgeReviewState(
  edge: GraphEdge,
  edgeReviews: Record<string, ReviewState>,
): ReviewState {
  return edgeReviews[edge.id] ?? edge.review
}

export function matchesEdgeOriginFilter(
  edge: GraphEdge,
  filter: EventOrigin | 'all',
): boolean {
  if (filter === 'all') return true
  if (filter === 'seed') return false
  return (edge.origin ?? 'agent') === filter
}

export function filterInvestigationEdges(
  edgeIds: string[],
  graphEdges: Record<string, GraphEdge>,
  edgeReviews: Record<string, ReviewState>,
  filters: EdgeQueueFilters,
): GraphEdge[] {
  return edgeIds
    .map((id) => graphEdges[id])
    .filter(Boolean)
    .filter((edge) => matchesEdgeOriginFilter(edge, filters.originFilter))
    .filter((edge) => {
      const review = edgeReviewState(edge, edgeReviews)
      if (filters.reviewFilter === 'all') return true
      return review === filters.reviewFilter
    })
}

export function proposedInvestigationEdgeIds(
  edgeIds: string[],
  graphEdges: Record<string, GraphEdge>,
  edgeReviews: Record<string, ReviewState>,
  filters: EdgeQueueFilters,
): string[] {
  return filterInvestigationEdges(edgeIds, graphEdges, edgeReviews, filters)
    .filter((edge) => edgeReviewState(edge, edgeReviews) === 'proposed')
    .map((edge) => edge.id)
}

type SetReview = (
  kind: 'edge' | 'node',
  id: string,
  review: ReviewState,
  investigationId?: string,
) => void

/** Confirm edge on the server and locally mark proposed endpoint nodes confirmed. */
export function acceptEdgeWithNodes(
  setReview: SetReview,
  edge: GraphEdge,
  investigationId: string,
  graphNodes: Record<string, GraphNode>,
  nodeReviews: Record<string, ReviewState>,
): void {
  setReview('edge', edge.id, 'confirmed', investigationId)
  for (const nodeId of [edge.source, edge.target]) {
    const node = graphNodes[nodeId]
    if (!node) continue
    if ((nodeReviews[nodeId] ?? node.review) === 'proposed') {
      setReview('node', nodeId, 'confirmed')
    }
  }
}

export function acceptEdgesWithNodes(
  setReview: SetReview,
  edges: GraphEdge[],
  investigationId: string,
  graphNodes: Record<string, GraphNode>,
  nodeReviews: Record<string, ReviewState>,
): void {
  for (const edge of edges) {
    acceptEdgeWithNodes(setReview, edge, investigationId, graphNodes, nodeReviews)
  }
}

export function confirmProposedNodes(
  setReview: SetReview,
  nodeIds: Iterable<string>,
  graphNodes: Record<string, GraphNode>,
  nodeReviews: Record<string, ReviewState>,
): void {
  for (const nodeId of nodeIds) {
    const node = graphNodes[nodeId]
    if (!node) continue
    if ((nodeReviews[nodeId] ?? node.review) === 'proposed') {
      setReview('node', nodeId, 'confirmed')
    }
  }
}
