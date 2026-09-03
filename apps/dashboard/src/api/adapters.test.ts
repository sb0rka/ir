import { describe, expect, it } from 'vitest'
import type { components as Ir } from '@ir/contract'
import { mapIrEvent, mapIrInvestigation } from './adapters'

type IrEvent = Ir['schemas']['EventSummary']
type IrInvestigation = Ir['schemas']['Investigation']

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
    })
    expect(mapped.updatedAt).toBe('2026-01-02T12:00:00Z')
    expect(mapped.verdict).toBe('incident')
    expect(mapped.verdictReason).toBe('confirmed C2')
    expect(mapped.description).toBe('beaconing host')
    expect(mapped.nodeIds).toEqual([])
    expect(mapped.view).toBe('graph')
  })
})
