import { describe, expect, it } from 'vitest'
import type { components as Ir } from '@ir/contract'
import { mapIrEvent } from './adapters'

type IrEvent = Ir['schemas']['EventSummary']

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
