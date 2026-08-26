import type { EventFieldDef, FieldType } from './model'
import { KNOWN_EVENT_FIELDS } from './relatedFields'

const FREQ_KEY = 'ir.pdql.fieldFreq'

export const DEFAULT_FIELD_FREQ: Record<string, number> = {
  time: 100,
  'event_src.host': 90,
  text: 85,
  'src.ip': 80,
  'dst.ip': 75,
  action: 70,
  correlation_name: 65,
  'event_src.ip': 60,
  'subject.account.name': 55,
  'object.process.cmdline': 50,
  importance: 45,
}

const ENUM_VALUES: Record<string, string[]> = {
  importance: ['low', 'medium', 'high'],
  status: ['new', 'investigating', 'closed'],
  action: [
    'start',
    'access',
    'elevate',
    'login',
    'open',
    'create',
    'remove',
    'modify',
    'assign',
    'logout',
    'rename',
    'deelevate',
    'execute',
    'stop',
    'configure',
    'initiate',
  ],
}

function fieldType(name: string): FieldType {
  if (name === 'time' || name === 'recv_time' || name.endsWith('.time')) return 'datetime'
  if (name.endsWith('.ip')) return 'ip'
  if (name.endsWith('.port') || name.endsWith('.id')) return 'number'
  if (name in ENUM_VALUES) return 'enum'
  return 'string'
}

function toFieldDef(name: string): EventFieldDef {
  return {
    name,
    type: fieldType(name),
    description: name,
    enumValues: ENUM_VALUES[name],
  }
}

/** Local MaxPatrol-shaped catalog — Gateway has no event-fields path in the current contract. */
export async function fetchEventFields(): Promise<EventFieldDef[]> {
  return [...KNOWN_EVENT_FIELDS].map(toFieldDef)
}

export function loadFieldFreq(): Record<string, number> {
  try {
    const raw = localStorage.getItem(FREQ_KEY)
    if (!raw) return { ...DEFAULT_FIELD_FREQ }
    const parsed = JSON.parse(raw) as Record<string, number>
    return { ...DEFAULT_FIELD_FREQ, ...parsed }
  } catch {
    return { ...DEFAULT_FIELD_FREQ }
  }
}

export function saveFieldFreq(freq: Record<string, number>): void {
  try {
    localStorage.setItem(FREQ_KEY, JSON.stringify(freq))
  } catch {
    /* ignore quota / private mode */
  }
}

export function bumpFieldFreq(
  freq: Record<string, number>,
  name: string,
): Record<string, number> {
  const next = { ...freq, [name]: (freq[name] ?? 0) + 1 }
  saveFieldFreq(next)
  return next
}

export function sortFields(
  fields: EventFieldDef[],
  freq: Record<string, number>,
  query: string,
): EventFieldDef[] {
  const needle = query.trim().toLowerCase()
  const matched = needle
    ? fields.filter(
        (field) =>
          field.name.toLowerCase().includes(needle) ||
          field.description.toLowerCase().includes(needle),
      )
    : fields.slice()
  return matched.sort((left, right) => {
    const freqDelta = (freq[right.name] ?? 0) - (freq[left.name] ?? 0)
    if (freqDelta !== 0) return freqDelta
    return left.name.localeCompare(right.name)
  })
}
