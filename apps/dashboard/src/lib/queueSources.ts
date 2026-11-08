import type { QueueSource } from '../types'

/** Gateway source that serves SIEM incidents and correlations. */
export const MAXPATROL_SIEM_SOURCE = 'pt-maxpatrol-siem'

/** Gateway source that serves NAD attacks. */
export const PT_NAD_SOURCE = 'pt-nad'

/** Same text as the findings search when no findings source is available. */
export const FINDINGS_SOURCE_UNAVAILABLE = 'Нет доступных online-источников findings'

export const QUEUE_SOURCE_GROUPS: Array<{
  category?: string
  ids: QueueSource[]
  /** Source that must be connected before this group is shown. */
  requires?: string
}> = [
  { category: 'SIEM', ids: ['siem_incident', 'siem_correlation'], requires: MAXPATROL_SIEM_SOURCE },
  { category: 'NAD', ids: ['nad_attack'], requires: PT_NAD_SOURCE },
  { ids: ['events', 'entities'] },
]

/**
 * Source-bound groups stay hidden until their source is known to be connected.
 * `connected === null` means the source list has not loaded yet.
 */
export function visibleQueueSourceGroups(connected: ReadonlySet<string> | null) {
  return QUEUE_SOURCE_GROUPS.filter((group) => !group.requires || connected?.has(group.requires) === true)
}

export function queueSourceAvailable(source: QueueSource, connected: ReadonlySet<string>): boolean {
  const required = QUEUE_SOURCE_GROUPS.find((group) => group.ids.includes(source))?.requires
  return required == null || connected.has(required)
}

/** Incidents, correlations, and attacks fall back to events when their source is absent. */
export function coerceQueueSource(source: QueueSource, connected: ReadonlySet<string>): QueueSource {
  return queueSourceAvailable(source, connected) ? source : 'events'
}

export function withoutFindingsUnavailableError(message: string | null): string | null {
  if (!message?.includes(FINDINGS_SOURCE_UNAVAILABLE)) return message
  const rest = message
    .split('; ')
    .filter((part) => part !== FINDINGS_SOURCE_UNAVAILABLE)
  return rest.length ? rest.join('; ') : null
}
