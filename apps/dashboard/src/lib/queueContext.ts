import { gatewayFindingId } from '../api/adapters'
import type { AlertEvent, ContextEvent } from '../types'
import { findingResolveKey } from './correlationSubevents'

export function findingIdentityKey(ref: NonNullable<AlertEvent['findingRef']>): string {
  return gatewayFindingId(ref)
}

export function findingRefForImport(
  alert: Pick<AlertEvent, 'source' | 'sourceEventId' | 'findingRef' | 'raw'>,
  fallbackRange: { from: string; to: string },
): NonNullable<AlertEvent['findingRef']> | undefined {
  if (alert.findingRef) return alert.findingRef
  const key = findingResolveKey(
    {
      source: alert.source,
      sourceEventId: alert.sourceEventId,
      raw: alert.raw ?? {},
    },
    fallbackRange,
  )
  if (!key) return undefined
  return {
    source_code: key.source_code,
    ...(key.source_instance ? { source_instance: key.source_instance } : {}),
    record_type: key.record_type,
    external_id: key.external_id,
    time_range: key.time_range,
  }
}

export function eventIdentityKey(source: string, sourceEventId: string): string {
  return `${source}/${sourceEventId}`
}

export function contextEventKeys(
  eventIds: string[],
  events: Record<string, ContextEvent>,
): Set<string> {
  const keys = new Set<string>()
  for (const id of eventIds) {
    const ev = events[id]
    if (ev?.source && ev.sourceEventId) keys.add(eventIdentityKey(ev.source, ev.sourceEventId))
  }
  return keys
}

const IMPORT_PROBE_RANGE = { from: '1970-01-01T00:00:00.000Z', to: '1970-01-01T00:00:00.001Z' }

export function selectionHasFindings(
  ids: string[],
  alerts: Record<string, AlertEvent>,
): boolean {
  return ids.some((id) => Boolean(alerts[id] && findingRefForImport(alerts[id], IMPORT_PROBE_RANGE)))
}

export function contextImportOptions(
  hasFindings: boolean,
  input: { expandFindings: boolean; why: string },
): { expandFindings?: boolean; why?: string } {
  const why = input.why.trim()
  return {
    ...(hasFindings ? { expandFindings: input.expandFindings } : {}),
    ...(why ? { why } : {}),
  }
}

export function contextSelectionFields(input: {
  expandFindings?: boolean
  why?: string
}): { expand_findings?: boolean; why?: string } {
  const why = input.why?.trim()
  return {
    ...(input.expandFindings !== undefined ? { expand_findings: input.expandFindings } : {}),
    ...(why ? { why } : {}),
  }
}

export function alertIsInContext(
  alert: AlertEvent,
  findingKeys: Iterable<string>,
  eventKeys: Set<string>,
): boolean {
  const findings = findingKeys instanceof Set ? findingKeys : new Set(findingKeys)
  const findingRef = findingRefForImport(alert, IMPORT_PROBE_RANGE)
  if (findingRef && findings.has(findingIdentityKey(findingRef))) return true
  if (alert.source && alert.sourceEventId && eventKeys.has(eventIdentityKey(alert.source, alert.sourceEventId))) {
    return true
  }
  return false
}
