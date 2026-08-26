import { describe, expect, it } from 'vitest'
import type { AlertEvent, ContextEvent } from '../types'
import {
  alertIsInContext,
  contextEventKeys,
  eventIdentityKey,
  findingIdentityKey,
} from './queueContext'

function alert(partial: Partial<AlertEvent> & Pick<AlertEvent, 'id'>): AlertEvent {
  return {
    time: '2025-10-23T12:00:00.000Z',
    severity: 'high',
    title: 't',
    rule: 'r',
    source: 'pt-maxpatrol-siem',
    status: 'new',
    entityIds: [],
    description: 'd',
    ...partial,
  }
}

describe('queueContext identity', () => {
  it('matches a finding by source ref, not IR uuid', () => {
    const findingAlert = alert({
      id: 'pt-maxpatrol-siem/siem_incident/42',
      findingRef: {
        source_code: 'pt-maxpatrol-siem',
        record_type: 'siem_incident',
        external_id: '42',
        time_range: { from: '2025-10-23T00:00:00.000Z', to: '2025-10-23T23:59:59.000Z' },
      },
    })
    const key = findingIdentityKey(findingAlert.findingRef!)
    expect(alertIsInContext(findingAlert, [key], new Set())).toBe(true)
    expect(alertIsInContext(findingAlert, ['other'], new Set())).toBe(false)
  })

  it('matches an event by source_code + source_event_id', () => {
    const ev = alert({
      id: 'pt-maxpatrol-siem/evt-1',
      source: 'pt-maxpatrol-siem',
      sourceEventId: 'evt-1',
    })
    const events: Record<string, ContextEvent> = {
      'uuid-1': {
        id: 'uuid-1',
        time: ev.time,
        severity: 'high',
        title: 't',
        type: 'correlation_alert',
        source: 'pt-maxpatrol-siem',
        entityIds: [],
        origin: 'seed',
        review: 'confirmed',
        description: 'd',
        sourceEventId: 'evt-1',
      },
    }
    const keys = contextEventKeys(['uuid-1'], events)
    expect(keys.has(eventIdentityKey('pt-maxpatrol-siem', 'evt-1'))).toBe(true)
    expect(alertIsInContext(ev, [], keys)).toBe(true)
  })
})
