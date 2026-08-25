import { gatewayFindingId } from '../api/adapters'
import type { AlertEvent, ContextEvent } from '../types'

export function findingIdentityKey(ref: NonNullable<AlertEvent['findingRef']>): string {
  return gatewayFindingId(ref)
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

export function alertIsInContext(
  alert: AlertEvent,
  findingKeys: Iterable<string>,
  eventKeys: Set<string>,
): boolean {
  const findings = findingKeys instanceof Set ? findingKeys : new Set(findingKeys)
  if (alert.findingRef && findings.has(findingIdentityKey(alert.findingRef))) return true
  if (alert.source && alert.sourceEventId && eventKeys.has(eventIdentityKey(alert.source, alert.sourceEventId))) {
    return true
  }
  return false
}
