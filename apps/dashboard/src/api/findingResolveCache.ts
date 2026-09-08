import { SessionCache } from '../lib/sessionCache'
import type { AlertEvent } from '../types'
import type { FindingResolveKey } from '../lib/correlationSubevents'

export type FindingResolveResult = {
  events: AlertEvent[]
  accounts: string[]
  hosts: { value: string; roles: string[] }[]
  errors: string[]
}

/** Isolated from search.ts so env.ts can clear without an import cycle. */
export const findingResolveCache = new SessionCache<FindingResolveResult>(50)

export function clearFindingResolveCache(): void {
  findingResolveCache.clear()
}

/**
 * Identity of a finding in this project. time_range stays on the wire but is
 * not part of the key: the card sends findingRef timestamps (often without
 * millis) while uuid search uses the UI interval (Date.toISOString).
 */
export function findingResolveCacheKey(
  projectId: string,
  key: FindingResolveKey,
): string {
  return JSON.stringify({
    projectId,
    source_code: key.source_code,
    source_instance: key.source_instance ?? null,
    record_type: key.record_type,
    external_id: key.external_id,
  })
}

export function isFindingResolveSoftFail(result: FindingResolveResult): boolean {
  return (
    result.errors.length > 0 &&
    result.accounts.length === 0 &&
    result.hosts.length === 0 &&
    result.events.length === 0
  )
}
