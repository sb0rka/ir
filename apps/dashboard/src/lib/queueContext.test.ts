import { describe, expect, it } from 'vitest'
import type { AlertEvent, ContextEvent } from '../types'
import {
  alertIsInContext,
  contextEventKeys,
  contextImportOptions,
  contextSelectionFields,
  eventIdentityKey,
  findingIdentityKey,
  findingRefForImport,
  selectionHasFindings,
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

  it('matches a correlation event by inferred finding identity', () => {
    const correlation = alert({
      id: 'corr-1',
      sourceEventId: 'corr-uuid',
      raw: { correlation_name: 'WMI remote' },
    })
    const key = findingIdentityKey({
      source_code: 'pt-maxpatrol-siem',
      record_type: 'siem_correlation',
      external_id: 'corr-uuid',
      time_range: { from: '2025-10-23T00:00:00.000Z', to: '2025-10-24T00:00:00.000Z' },
    })
    expect(alertIsInContext(correlation, [key], new Set())).toBe(true)
    expect(alertIsInContext(correlation, ['other'], new Set())).toBe(false)
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
        isSeed: true,
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

describe('selectionHasFindings', () => {
  it('is true when any selected alert has a findingRef', () => {
    const finding = alert({
      id: 'inc-1',
      findingRef: {
        source_code: 'pt-maxpatrol-siem',
        record_type: 'siem_incident',
        external_id: 'inc-1',
        time_range: { from: '2025-10-23T00:00:00.000Z', to: '2025-10-23T23:59:59.000Z' },
      },
    })
    expect(selectionHasFindings(['inc-1', 'evt-1'], { 'inc-1': finding, 'evt-1': alert({ id: 'evt-1' }) })).toBe(
      true,
    )
    expect(selectionHasFindings(['evt-1'], { 'evt-1': alert({ id: 'evt-1' }) })).toBe(false)
  })

  it('is true for a correlation event that the card can resolve without findingRef', () => {
    const correlation = alert({
      id: 'corr-1',
      sourceEventId: 'corr-1',
      raw: { correlation_name: 'WMI remote' },
    })
    expect(selectionHasFindings(['corr-1'], { 'corr-1': correlation })).toBe(true)
  })
})

describe('findingRefForImport', () => {
  const range = { from: '2025-10-23T00:00:00.000Z', to: '2025-10-24T00:00:00.000Z' }

  it('keeps an explicit findingRef', () => {
    const finding = alert({
      id: 'inc-1',
      findingRef: {
        source_code: 'pt-maxpatrol-siem',
        record_type: 'siem_incident',
        external_id: 'inc-1',
        time_range: range,
      },
    })
    expect(findingRefForImport(finding, { from: 'x', to: 'y' })).toEqual(finding.findingRef)
  })

  it('synthesizes a siem_correlation ref from the queue event', () => {
    expect(
      findingRefForImport(
        alert({ id: 'corr-1', sourceEventId: 'corr-uuid', raw: { correlation_name: 'WMI remote' } }),
        range,
      ),
    ).toEqual({
      source_code: 'pt-maxpatrol-siem',
      record_type: 'siem_correlation',
      external_id: 'corr-uuid',
      time_range: range,
    })
  })
})

describe('contextImportOptions', () => {
  it('sends expandFindings only for findings and skips blank why', () => {
    expect(contextImportOptions(true, { expandFindings: false, why: '  card  ' })).toEqual({
      expandFindings: false,
      why: 'card',
    })
    expect(contextImportOptions(false, { expandFindings: true, why: '   ' })).toEqual({})
  })
})

describe('contextSelectionFields', () => {
  it('maps expandFindings and trims why for the IR body', () => {
    expect(contextSelectionFields({ expandFindings: false, why: '  card  ' })).toEqual({
      expand_findings: false,
      why: 'card',
    })
    expect(contextSelectionFields({ why: '   ' })).toEqual({})
    expect(contextSelectionFields({})).toEqual({})
  })
})
