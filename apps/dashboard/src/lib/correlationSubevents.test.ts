import { describe, expect, it } from 'vitest'
import type { AlertEvent } from '../types'
import {
  findingResolveKey,
  pickCorrelationSubevents,
  pickFindingAccounts,
  pickFindingChildEvents,
  pickFindingHosts,
} from './correlationSubevents'

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

const RANGE = { from: '2025-10-23T00:00:00.000Z', to: '2025-10-24T00:00:00.000Z' }

describe('findingResolveKey', () => {
  it('uses the finding ref for a siem_correlation', () => {
    expect(
      findingResolveKey(
        {
          source: 'other',
          sourceEventId: 'ignored',
          raw: {},
          findingRef: {
            source_code: 'pt-maxpatrol-siem',
            record_type: 'siem_correlation',
            external_id: 'corr-uuid',
            time_range: RANGE,
          },
        },
        { from: 'x', to: 'y' },
      ),
    ).toEqual({
      source_code: 'pt-maxpatrol-siem',
      source_instance: undefined,
      record_type: 'siem_correlation',
      external_id: 'corr-uuid',
      time_range: RANGE,
    })
  })

  it('uses the finding ref for a siem_incident', () => {
    expect(
      findingResolveKey(
        {
          source: 'pt-maxpatrol-siem',
          sourceEventId: '1e3e957d-3b00-0001-0000-000000000139',
          raw: { finding_kind: 'siem_incident' },
          findingRef: {
            source_code: 'pt-maxpatrol-siem',
            record_type: 'siem_incident',
            external_id: '1e3e957d-3b00-0001-0000-000000000139',
            time_range: RANGE,
          },
        },
        { from: 'x', to: 'y' },
      ),
    ).toMatchObject({
      record_type: 'siem_incident',
      external_id: '1e3e957d-3b00-0001-0000-000000000139',
    })
  })

  it('falls back to sourceEventId and the current time window', () => {
    expect(
      findingResolveKey(
        { source: 'pt-maxpatrol-siem', sourceEventId: 'evt-9', raw: { uuid: 'other', correlation_name: 'brute' } },
        RANGE,
      ),
    ).toEqual({
      source_code: 'pt-maxpatrol-siem',
      source_instance: undefined,
      record_type: 'siem_correlation',
      external_id: 'evt-9',
      time_range: RANGE,
    })
  })

  it('returns null without a finding id', () => {
    expect(findingResolveKey({ source: 'pt-maxpatrol-siem', raw: {} }, RANGE)).toBeNull()
  })
})

describe('pickCorrelationSubevents', () => {
  it('keeps events linked as subevent_of the correlation', () => {
    const root = alert({
      id: 'root',
      sourceEventId: 'corr-1',
      raw: { correlation_name: 'brute' },
    })
    const child = alert({
      id: 'child',
      time: '2025-10-23T12:01:00.000Z',
      sourceEventId: 'evt-2',
      raw: { parent_source_event_id: 'corr-1', relation_type: 'subevent_of' },
    })
    const other = alert({
      id: 'other',
      sourceEventId: 'evt-3',
      raw: { parent_source_event_id: 'corr-9', relation_type: 'subevent_of' },
    })
    expect(pickCorrelationSubevents([root, other, child], 'corr-1').map((e) => e.id)).toEqual([
      'child',
    ])
  })

  it('drops the root event when relation metadata is missing', () => {
    const root = alert({ id: 'root', sourceEventId: 'corr-1' })
    const child = alert({
      id: 'child',
      time: '2025-10-23T11:00:00.000Z',
      sourceEventId: 'evt-2',
    })
    expect(pickCorrelationSubevents([root, child], 'corr-1').map((e) => e.id)).toEqual(['child'])
  })
})

describe('pickFindingChildEvents', () => {
  it('keeps incident child events and drops metadata', () => {
    const key = {
      source_code: 'pt-maxpatrol-siem',
      record_type: 'siem_incident' as const,
      external_id: 'inc-1',
      time_range: RANGE,
    }
    const root = alert({ id: 'root', sourceEventId: 'inc-1' })
    const corr = alert({
      id: 'corr',
      time: '2025-10-23T12:01:00.000Z',
      sourceEventId: 'corr-1',
      rule: 'Proxy_Tools_Usage',
      raw: { correlation_name: 'Proxy_Tools_Usage' },
    })
    const file = alert({
      id: 'file',
      sourceEventId: 'inc-1:siem.incident.file:abc',
      rule: 'siem.incident.file',
    })
    expect(pickFindingChildEvents([root, file, corr], key).map((e) => e.id)).toEqual(['corr'])
  })
})

describe('pickFindingAccounts', () => {
  it('keeps unique involved accounts and drops placeholders and backslashes', () => {
    expect(
      pickFindingAccounts([
        { type: 'account', value: 'administrator', roles: ['mentions'] },
        { type: 'host', value: 'ws01', roles: ['src'] },
        { type: 'account', value: '-', roles: ['mentions'] },
        { type: 'account', value: 'DKRYLOVA\\\\administrator', roles: ['mentions'] },
        { type: 'account', value: 'dkrylova\\user', roles: ['actor'] },
        { type: 'account', value: 'noise', roles: ['object'] },
      ]),
    ).toEqual(['administrator', 'dkrylovaadministrator'])
  })
})

describe('pickFindingHosts', () => {
  it('keeps unique incident hosts with roles', () => {
    expect(
      pickFindingHosts([
        { type: 'host', value: 'dc01.local', roles: ['dst'] },
        { type: 'host', value: 'WS01.LOCAL', roles: ['src'] },
        { type: 'ip', value: '10.0.0.1', roles: ['src'] },
        { type: 'host', value: 'ws01.local', roles: ['mentions'] },
        { type: 'host', value: 'noise', roles: ['actor'] },
      ]),
    ).toEqual([
      { value: 'dc01.local', roles: ['dst'] },
      { value: 'ws01.local', roles: ['mentions', 'src'] },
    ])
  })
})
