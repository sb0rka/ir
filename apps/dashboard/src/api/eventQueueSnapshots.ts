import { eventIdentityKey } from '../lib/queueContext'
import {
  PRESET_IDS,
  type Direction,
  type Duration,
  type PresetId,
  type TimeInterval,
} from '../components/time-interval/model'
import {
  QUEUE_SOURCE_OPTIONS,
  type QueryHistoryEntry,
  type QueueSource,
} from '../types'
import { getProjectId } from './env'

const QUEUE_SOURCES = new Set<string>(QUEUE_SOURCE_OPTIONS.map((option) => option.id))
const DIRECTIONS = new Set(['before', 'after', 'around'])

function storageKey(projectId: string): string {
  return `ir.${projectId}.eventQueueSnapshots`
}

type SnapshotStore = Record<string, Record<string, QueryHistoryEntry>>

function identityKey(event: { source?: string; sourceEventId?: string }): string | null {
  if (!event.source || !event.sourceEventId) return null
  return eventIdentityKey(event.source, event.sourceEventId)
}

function isPresetId(value: string): value is PresetId {
  return (PRESET_IDS as readonly string[]).includes(value)
}

function parseDuration(value: unknown): Duration | null {
  if (!value || typeof value !== 'object') return null
  const duration = value as Record<string, unknown>
  if (duration.kind === 'preset' && typeof duration.id === 'string' && isPresetId(duration.id)) {
    return { kind: 'preset', id: duration.id }
  }
  if (duration.kind === 'custom' && typeof duration.ms === 'number' && Number.isFinite(duration.ms)) {
    return { kind: 'custom', ms: duration.ms }
  }
  return null
}

function parseTimeInterval(value: unknown): TimeInterval | null {
  if (!value || typeof value !== 'object') return null
  const interval = value as Record<string, unknown>
  if (interval.kind === 'range' && typeof interval.from === 'string' && typeof interval.to === 'string') {
    return { kind: 'range', from: interval.from, to: interval.to }
  }
  if (interval.kind !== 'relative') return null
  if (typeof interval.live !== 'boolean' || typeof interval.anchor !== 'string') return null
  if (typeof interval.direction !== 'string' || !DIRECTIONS.has(interval.direction)) return null
  const duration = parseDuration(interval.duration)
  if (!duration) return null
  return {
    kind: 'relative',
    live: interval.live,
    anchor: interval.anchor,
    direction: interval.direction as Direction,
    duration,
  }
}

function parseQueueSource(value: unknown): QueueSource | undefined {
  if (typeof value !== 'string' || !QUEUE_SOURCES.has(value)) return undefined
  return value as QueueSource
}

function parseEntry(value: unknown): QueryHistoryEntry | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Record<string, unknown>
  if (typeof raw.pdql !== 'string') return null
  const timeInterval = parseTimeInterval(raw.timeInterval)
  if (!timeInterval) return null
  const groupValues = Array.isArray(raw.groupValues)
    ? raw.groupValues.map((item) => (item == null ? null : typeof item === 'string' ? item : null))
    : undefined
  return {
    pdql: raw.pdql,
    timeInterval,
    queueSource: parseQueueSource(raw.queueSource),
    groupValues,
  }
}

function cloneEntry(entry: QueryHistoryEntry): QueryHistoryEntry {
  return {
    pdql: entry.pdql,
    timeInterval: entry.timeInterval,
    queueSource: entry.queueSource,
    groupValues: entry.groupValues ? [...entry.groupValues] : undefined,
  }
}

function readStore(): SnapshotStore {
  const projectId = getProjectId()
  if (!projectId) return {}
  try {
    const raw = localStorage.getItem(storageKey(projectId))
    if (!raw) return {}
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const store: SnapshotStore = {}
    for (const [investigationId, events] of Object.entries(parsed as Record<string, unknown>)) {
      if (!events || typeof events !== 'object' || Array.isArray(events)) continue
      const byEvent: Record<string, QueryHistoryEntry> = {}
      for (const [key, entry] of Object.entries(events as Record<string, unknown>)) {
        const parsedEntry = parseEntry(entry)
        if (parsedEntry) byEvent[key] = parsedEntry
      }
      if (Object.keys(byEvent).length > 0) store[investigationId] = byEvent
    }
    return store
  } catch {
    return {}
  }
}

function writeStore(store: SnapshotStore): void {
  const projectId = getProjectId()
  if (!projectId) return
  try {
    localStorage.setItem(storageKey(projectId), JSON.stringify(store))
  } catch {
    /* Persistence is best-effort; restoring a queue is optional. */
  }
}

export function readEventQueueSnapshot(
  investigationId: string,
  event: { source?: string; sourceEventId?: string },
): QueryHistoryEntry | null {
  const key = identityKey(event)
  if (!key) return null
  return readStore()[investigationId]?.[key] ?? null
}

/** First write wins: later adds (e.g. adding a field on an existing event) keep the original query. */
export function rememberEventQueueSnapshots(
  investigationId: string,
  events: Array<{ source?: string; sourceEventId?: string } | undefined>,
  entry: QueryHistoryEntry,
): void {
  const keys = [
    ...new Set(events.map((event) => (event ? identityKey(event) : null)).filter((key): key is string => Boolean(key))),
  ]
  if (keys.length === 0) return
  const store = readStore()
  const current = store[investigationId] ?? {}
  let changed = false
  const snapshot = cloneEntry(entry)
  for (const key of keys) {
    if (current[key]) continue
    current[key] = snapshot
    changed = true
  }
  if (!changed) return
  store[investigationId] = current
  writeStore(store)
}
