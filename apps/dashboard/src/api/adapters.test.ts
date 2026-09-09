import { describe, expect, it } from 'vitest'
import type { components as Ir } from '@ir/contract'
import { layoutGraph, mapIrEvent, mapIrInvestigation } from './adapters'
import type { GraphNode } from '../types'

type IrEvent = Ir['schemas']['EventSummary']
type IrInvestigation = Ir['schemas']['Investigation']

describe('dense investigation layout', () => {
  const events: GraphNode[] = Array.from({ length: 63 }, (_, i) => ({
    id: `event-${i}`, refId: `event-${i}`, kind: 'event', label: 'NTLMSSP_AUTH',
    review: 'confirmed', x: 0, y: 0,
    occurredAt: new Date(Date.UTC(2023, 5, 8, 11, 1, i)).toISOString(),
  }))
  const entities: GraphNode[] = Array.from({ length: 6 }, (_, i) => ({
    id: `host-${i}`, refId: `host-${i}`, kind: 'host', label: `host-${i}`,
    review: 'confirmed', x: 0, y: 0,
  }))
  const edges = events.flatMap((event) => entities.map((entity) => ({ source: event.id, target: entity.id })))

  it('keeps all 69 nodes compact and non-overlapping in chronological rows', () => {
    const input = [...events].reverse().concat(entities)
    const placed = layoutGraph('dense', input, edges, { ignoreSaved: true })
    expect(placed.map((n) => n.id)).toEqual(input.map((n) => n.id))
    expect(Math.max(...placed.map((n) => n.x)) - Math.min(...placed.map((n) => n.x))).toBeLessThan(3500)
    expect(Math.max(...placed.map((n) => n.y)) - Math.min(...placed.map((n) => n.y))).toBeLessThan(2500)
    const byId = new Map(placed.map((n) => [n.id, n]))
    for (let i = 1; i < events.length; i++) {
      const prev = byId.get(events[i - 1].id)!
      const next = byId.get(events[i].id)!
      expect(next.x > prev.x || next.y > prev.y + 100).toBe(true)
    }
    for (let i = 0; i < placed.length; i++) {
      for (const b of placed.slice(i + 1)) {
        const a = placed[i]
        const [aw, ah] = a.kind === 'event' ? [220, 72] : [180, 56]
        const [bw, bh] = b.kind === 'event' ? [220, 72] : [180, 56]
        expect(a.x + aw <= b.x || b.x + bw <= a.x || a.y + ah <= b.y || b.y + bh <= a.y,
          `overlap: ${a.id} and ${b.id}`).toBe(true)
      }
    }
  })

  it('retains the single row for small investigations', () => {
    const placed = layoutGraph('small', events.slice(0, 7), [], { ignoreSaved: true })
    expect(placed.every((n, i) => i === 0 || n.x > placed[i - 1].x)).toBe(true)
    expect(Math.max(...placed.map((n) => n.y)) - Math.min(...placed.map((n) => n.y))).toBeLessThan(50)
  })

  it('does not inflate every row for entities attached only to the first event', () => {
    const localEdges = events.flatMap((event) => entities.slice(0, 3).map((entity) => ({ source: event.id, target: entity.id })))
    localEdges.push(...entities.slice(3).map((entity) => ({ source: events[0].id, target: entity.id })))
    const placed = layoutGraph('local-fan', [...events, ...entities], localEdges, { ignoreSaved: true })
    expect(Math.max(...placed.map((n) => n.y)) - Math.min(...placed.map((n) => n.y))).toBeLessThan(1600)
  })
})

function irEvent(overrides: Partial<IrEvent>): IrEvent {
  return {
    id: '00000000-0000-0000-0000-000000000001',
    is_seed: false,
    source_code: 'pt-maxpatrol-siem',
    source_event_id: 'evt-1',
    title: 'login',
    event_type: 'auth.success',
    occurred_at: '2026-01-01T00:00:00.000Z',
    ingested_at: '2026-01-01T00:00:01.000Z',
    ...overrides,
  }
}

describe('mapIrEvent seed flag', () => {
  it('copies is_seed from the investigation-event link', () => {
    const seed = mapIrEvent(irEvent({ is_seed: true, attached_by: 'analyst' }), [])
    expect(seed.isSeed).toBe(true)
    expect(seed.origin).toBe('analyst')

    const later = mapIrEvent(irEvent({ is_seed: false, attached_by: 'analyst' }), [])
    expect(later.isSeed).toBe(false)
    expect(later.origin).toBe('analyst')
  })

  it('does not treat derived system-attached events as seed', () => {
    const derived = mapIrEvent(irEvent({ is_seed: false, attached_by: 'system' }), [])
    expect(derived.isSeed).toBe(false)
    expect(derived.origin).toBe('seed')
  })
})

function irInvestigation(overrides: Partial<IrInvestigation> = {}): IrInvestigation {
  return {
    id: '11111111-1111-1111-1111-111111111111',
    project_id: 'proj',
    title: 'Case',
    status: 'open',
    severity: 'high',
    verdict: 'incident',
    verdict_reason: 'confirmed C2',
    description: 'beaconing host',
    version: 3,
    som_workspace_ids: ['ws-1'],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T12:00:00Z',
    closed_at: null,
    counters: {
      children: 2,
      findings: 3,
      sessions: 0,
      events: 4,
      entities: 5,
      proposed_edges: 1,
      nodes: 0,
      hypotheses: 0,
      agents: 0,
    },
    ...overrides,
  }
}

describe('mapIrInvestigation catalog fields', () => {
  it('copies counters, timestamps and verdict from the API record', () => {
    const mapped = mapIrInvestigation(irInvestigation())
    expect(mapped.counters).toEqual({
      children: 2,
      findings: 3,
      sessions: 0,
      events: 4,
      entities: 5,
      proposed_edges: 1,
      nodes: 0,
      hypotheses: 0,
      agents: 0,
    })
    expect(mapped.updatedAt).toBe('2026-01-02T12:00:00Z')
    expect(mapped.verdict).toBe('incident')
    expect(mapped.verdictReason).toBe('confirmed C2')
    expect(mapped.description).toBe('beaconing host')
    expect(mapped.nodeIds).toEqual([])
    expect(mapped.view).toBe('graph')
  })
})
