import { describe, expect, it, vi } from 'vitest'
import type { GraphEdge, GraphNode } from '../types'
import {
  acceptEdgeWithNodes,
  edgeReviewState,
  filterInvestigationEdges,
  proposedInvestigationEdgeIds,
} from './edge-review'

const edge = (id: string, review: GraphEdge['review'], origin: GraphEdge['origin'] = 'agent'): GraphEdge => ({
  id,
  source: 'n1',
  target: 'n2',
  relation: 'mentions',
  review,
  origin,
  version: 1,
})

describe('edgeReviewState', () => {
  it('prefers edgeReviews map over edge.review', () => {
    expect(edgeReviewState(edge('e1', 'proposed'), { e1: 'confirmed' })).toBe('confirmed')
  })
})

describe('filterInvestigationEdges', () => {
  const graphEdges = {
    e1: edge('e1', 'proposed', 'agent'),
    e2: edge('e2', 'confirmed', 'analyst'),
  }

  it('filters by origin and review', () => {
    const rows = filterInvestigationEdges(
      ['e1', 'e2'],
      graphEdges,
      {},
      { originFilter: 'agent', reviewFilter: 'proposed' },
    )
    expect(rows.map((r) => r.id)).toEqual(['e1'])
  })

  it('excludes all edges for seed origin filter', () => {
    const rows = filterInvestigationEdges(
      ['e1', 'e2'],
      graphEdges,
      {},
      { originFilter: 'seed', reviewFilter: 'all' },
    )
    expect(rows).toEqual([])
  })
})

describe('proposedInvestigationEdgeIds', () => {
  it('returns only visible proposed edges', () => {
    const ids = proposedInvestigationEdgeIds(
      ['e1', 'e2'],
      { e1: edge('e1', 'proposed'), e2: edge('e2', 'confirmed') },
      {},
      { originFilter: 'all', reviewFilter: 'all' },
    )
    expect(ids).toEqual(['e1'])
  })
})

describe('acceptEdgeWithNodes', () => {
  it('confirms edge and proposed endpoint nodes', () => {
    const setReview = vi.fn()
    const graphNodes: Record<string, GraphNode> = {
      n1: { id: 'n1', kind: 'host', refId: 'h1', label: 'host', review: 'proposed', x: 0, y: 0 },
      n2: { id: 'n2', kind: 'event', refId: 'ev1', label: 'ev', review: 'confirmed', x: 0, y: 0 },
    }
    acceptEdgeWithNodes(
      setReview,
      edge('e1', 'proposed'),
      'inv-1',
      graphNodes,
      {},
    )
    expect(setReview).toHaveBeenCalledWith('edge', 'e1', 'confirmed', 'inv-1')
    expect(setReview).toHaveBeenCalledWith('node', 'n1', 'confirmed')
    expect(setReview).not.toHaveBeenCalledWith('node', 'n2', 'confirmed')
  })
})
