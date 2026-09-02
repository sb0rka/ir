import { describe, expect, it } from 'vitest'
import {
  edgesBetweenNodes,
  graphLayerNodeSets,
  INVESTIGATION_LAYER_ID,
  investigationLayerNodeIds,
  isHypothesisWritable,
  membershipFromGraph,
  mergeVisibleLayerIds,
  nodeIdsForEntityRefs,
  nodeIdsForLayerItems,
  toggleLayerId,
  validHypothesisTransition,
} from './hypotheses'

describe('validHypothesisTransition', () => {
  it('allows proposed → active or resolved', () => {
    expect(validHypothesisTransition('proposed', 'active')).toBe(true)
    expect(validHypothesisTransition('proposed', 'resolved')).toBe(true)
    expect(validHypothesisTransition('proposed', 'proposed')).toBe(true)
  })

  it('forbids returning to proposed', () => {
    expect(validHypothesisTransition('active', 'proposed')).toBe(false)
    expect(validHypothesisTransition('resolved', 'proposed')).toBe(false)
  })

  it('allows resolved → active and active → resolved', () => {
    expect(validHypothesisTransition('resolved', 'active')).toBe(true)
    expect(validHypothesisTransition('active', 'resolved')).toBe(true)
  })
})

describe('node and edge mapping', () => {
  it('maps selected entity refs to graph node ids and skips events', () => {
    const ids = nodeIdsForEntityRefs(
      ['ent-1', 'ent-2'],
      {
        n1: { id: 'n1', kind: 'host', refId: 'ent-1' },
        n2: { id: 'n2', kind: 'event', refId: 'ent-2' },
        n3: { id: 'n3', kind: 'user', refId: 'ent-2' },
      },
    )
    expect(ids.sort()).toEqual(['n1', 'n3'])
  })

  it('keeps only edges whose both ends are selected', () => {
    const ids = edgesBetweenNodes(['n1', 'n2'], {
      e1: { id: 'e1', source: 'n1', target: 'n2' },
      e2: { id: 'e2', source: 'n1', target: 'n3' },
    })
    expect(ids).toEqual(['e1'])
  })
})

describe('membership helpers', () => {
  it('marks resolved as read-only', () => {
    expect(isHypothesisWritable('proposed')).toBe(true)
    expect(isHypothesisWritable('active')).toBe(true)
    expect(isHypothesisWritable('resolved')).toBe(false)
  })

  it('copies ids from a graph projection', () => {
    expect(
      membershipFromGraph({
        nodes: [{ id: 'n1' }, { id: 'n2' }],
        edges: [{ id: 'e1' }],
      }),
    ).toEqual({ nodeIds: ['n1', 'n2'], edgeIds: ['e1'] })
  })
})

describe('investigation layer nodes', () => {
  const hypotheses = {
    open: { status: 'active' as const },
    closed: { status: 'resolved' as const },
  }
  const membership = {
    open: { nodeIds: ['n-open', 'n-shared'], edgeIds: [] },
    closed: { nodeIds: ['n-closed', 'n-shared'], edgeIds: [] },
  }

  it('keeps orphans and resolved-only nodes, drops open membership', () => {
    expect(
      investigationLayerNodeIds(
        ['n-open', 'n-closed', 'n-shared', 'n-orphan'],
        ['open', 'closed'],
        hypotheses,
        membership,
      ).sort(),
    ).toEqual(['n-closed', 'n-orphan'])
  })

  it('unions visible items including the investigation layer', () => {
    const ids = nodeIdsForLayerItems(
      [INVESTIGATION_LAYER_ID, 'open'],
      ['n-open', 'n-closed', 'n-orphan'],
      ['open', 'closed'],
      hypotheses,
      membership,
    )
    expect([...ids].sort()).toEqual(['n-closed', 'n-open', 'n-orphan', 'n-shared'])
  })

  it('hides nodes when some eyes are off and dims when a highlight is on', () => {
    const sets = graphLayerNodeSets({
      visibleItemIds: ['open'],
      highlightedItemIds: ['open'],
      allItemIds: [INVESTIGATION_LAYER_ID, 'open', 'closed'],
      allNodeIds: ['n-open', 'n-closed', 'n-orphan'],
      hypothesisIds: ['open', 'closed'],
      hypotheses,
      membership,
    })
    expect(sets.visibleNodeIds ? [...sets.visibleNodeIds].sort() : null).toEqual([
      'n-open',
      'n-shared',
    ])
    expect(sets.highlightedNodeIds ? [...sets.highlightedNodeIds].sort() : null).toEqual([
      'n-open',
      'n-shared',
    ])
  })

  it('shows the full graph when every eye is on and does not dim without highlights', () => {
    const sets = graphLayerNodeSets({
      visibleItemIds: [INVESTIGATION_LAYER_ID, 'open', 'closed'],
      highlightedItemIds: [],
      allItemIds: [INVESTIGATION_LAYER_ID, 'open', 'closed'],
      allNodeIds: ['n-open', 'n-closed', 'n-orphan'],
      hypothesisIds: ['open', 'closed'],
      hypotheses,
      membership,
    })
    expect(sets.visibleNodeIds).toBeNull()
    expect(sets.highlightedNodeIds).toBeNull()
  })
})

describe('layer id lists', () => {
  it('defaults to all items and keeps user-hidden layers when hypotheses refresh', () => {
    expect(mergeVisibleLayerIds(undefined, ['h1'])).toEqual([INVESTIGATION_LAYER_ID, 'h1'])
    expect(mergeVisibleLayerIds(['h1'], ['h1', 'h2'])).toEqual(['h1', 'h2'])
    expect(mergeVisibleLayerIds([INVESTIGATION_LAYER_ID, 'gone'], ['h1'])).toEqual([
      INVESTIGATION_LAYER_ID,
      'h1',
    ])
  })

  it('toggles and isolates a layer id', () => {
    expect(toggleLayerId(['a', 'b'], 'a', false)).toEqual(['b'])
    expect(toggleLayerId(['a'], 'b', false)).toEqual(['a', 'b'])
    expect(toggleLayerId(['a', 'b'], 'b', true)).toEqual(['b'])
  })
})
