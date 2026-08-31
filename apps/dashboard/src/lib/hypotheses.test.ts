import { describe, expect, it } from 'vitest'
import {
  edgesBetweenNodes,
  isHypothesisWritable,
  membershipFromGraph,
  nodeIdsForEntityRefs,
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
