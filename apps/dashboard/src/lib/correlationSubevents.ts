import type { AlertEvent } from '../types'

export type FindingRecordType = 'siem_incident' | 'siem_correlation'

export interface FindingResolveKey {
  source_code: string
  source_instance?: string
  record_type: FindingRecordType
  external_id: string
  time_range: { from: string; to: string }
}

const INCIDENT_METADATA_RULES = new Set([
  'siem.incident.file',
  'siem.incident.link',
  'siem.incident.asset_group',
])

export function findingResolveKey(
  input: {
    source: string
    sourceEventId?: string
    findingRef?: AlertEvent['findingRef']
    raw: Record<string, string>
  },
  fallbackRange: { from: string; to: string },
): FindingResolveKey | null {
  const ref = input.findingRef
  if (ref?.record_type === 'siem_incident' || ref?.record_type === 'siem_correlation') {
    const external_id = ref.external_id.trim()
    const source_code = ref.source_code.trim()
    if (!external_id || !source_code) return null
    return {
      source_code,
      source_instance: ref.source_instance,
      record_type: ref.record_type,
      external_id,
      time_range: ref.time_range,
    }
  }
  const record_type = recordTypeFromRaw(input.raw)
  const external_id = (input.sourceEventId || input.raw.uuid || '').trim()
  const source_code = (ref?.source_code || input.source || '').trim()
  if (!record_type || !external_id || !source_code) return null
  return {
    source_code,
    source_instance: ref?.source_instance,
    record_type,
    external_id,
    time_range: ref?.time_range ?? fallbackRange,
  }
}

export function pickFindingChildEvents(
  events: AlertEvent[],
  key: FindingResolveKey,
): AlertEvent[] {
  if (key.record_type === 'siem_correlation') {
    return pickCorrelationSubevents(events, key.external_id)
  }
  const id = key.external_id.trim()
  return events
    .filter((event) => {
      if ((event.sourceEventId ?? '').trim() === id) return false
      if (INCIDENT_METADATA_RULES.has(event.rule)) return false
      return true
    })
    .sort((a, b) => a.time.localeCompare(b.time))
}

export function pickCorrelationSubevents(
  events: AlertEvent[],
  correlationId: string,
): AlertEvent[] {
  const id = correlationId.trim()
  if (!id) return []
  const children = events.filter((event) => {
    const parent = event.raw?.parent_source_event_id?.trim()
    if (parent) return parent === id
    return event.raw?.relation_type?.trim() === 'subevent_of'
  })
  const picked = children.length
    ? children
    : events.filter((event) => (event.sourceEventId ?? '').trim() !== id)
  return picked.slice().sort((a, b) => a.time.localeCompare(b.time))
}

function recordTypeFromRaw(raw: Record<string, string>): FindingRecordType | null {
  if (raw.finding_kind === 'siem_incident') return 'siem_incident'
  if (raw.finding_kind === 'siem_correlation' || raw.correlation_name) return 'siem_correlation'
  return null
}
