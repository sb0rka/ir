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

/** Account names from a resolved finding's entity mentions (incident /accounts). */
export function pickFindingAccounts(
  mentions: ReadonlyArray<{ type?: string; value?: string; roles?: readonly string[] }>,
): string[] {
  const accountMentions = mentions.filter((mention) => mention.type === 'account')
  const preferInvolved = accountMentions.some((mention) => mention.roles?.includes('mentions'))
  const names = new Set<string>()
  for (const mention of accountMentions) {
    if (preferInvolved && !mention.roles?.includes('mentions')) continue
    const name = normalizeAccountName(mention.value)
    if (!name) continue
    names.add(name)
  }
  return [...names].sort((a, b) => a.localeCompare(b))
}

export interface FindingHost {
  value: string
  roles: string[]
}

const INCIDENT_HOST_ROLES = new Set(['src', 'dst', 'mentions'])

/** Hosts from a resolved finding's entity mentions (incident /hosts). */
export function pickFindingHosts(
  mentions: ReadonlyArray<{ type?: string; value?: string; roles?: readonly string[] }>,
): FindingHost[] {
  const byValue = new Map<string, Set<string>>()
  for (const mention of mentions) {
    if (mention.type !== 'host') continue
    const value = mention.value?.trim().toLowerCase() ?? ''
    if (!value || value === '-') continue
    const roles = (mention.roles ?? []).filter((role) => INCIDENT_HOST_ROLES.has(role))
    if ((mention.roles?.length ?? 0) > 0 && roles.length === 0) continue
    const bucket = byValue.get(value) ?? new Set<string>()
    for (const role of roles.length ? roles : ['mentions']) bucket.add(role)
    byValue.set(value, bucket)
  }
  return [...byValue.entries()]
    .map(([value, roles]) => ({
      value,
      roles: [...roles].sort((a, b) => a.localeCompare(b)),
    }))
    .sort((a, b) => a.value.localeCompare(b.value))
}

/** Collapse escapes and drop backslashes for chip display (`domain\\user` → `domainuser`). */
export function normalizeAccountName(value: string | undefined): string {
  let name = value?.trim().toLowerCase() ?? ''
  if (!name || name === '-') return ''
  while (name.includes('\\\\')) {
    name = name.replaceAll('\\\\', '\\')
  }
  return name.replaceAll('\\', '')
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
