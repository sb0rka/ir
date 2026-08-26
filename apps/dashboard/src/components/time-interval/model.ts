import { CalendarDateTime, fromAbsolute, parseAbsolute, parseDateTime, toZoned } from '@internationalized/date'

export type PresetId = '1s' | '1m' | '5m' | '15m' | '1h' | '4h' | '12h' | '24h' | '7d'
export type Direction = 'before' | 'after' | 'around'
export type DisplayZone = 'utc' | 'working'

export type Duration =
  | { kind: 'preset'; id: PresetId }
  | { kind: 'custom'; ms: number }

export type TimeInterval =
  | {
      kind: 'relative'
      live: boolean
      /** UTC ISO. Ignored by resolve() while live is true. */
      anchor: string
      direction: Direction
      duration: Duration
    }
  | { kind: 'range'; from: string; to: string }

export type ResolvedInterval = { from: string; to: string }

export const PRESET_IDS: readonly PresetId[] = ['1s', '1m', '5m', '15m', '1h', '4h', '12h', '24h', '7d']

export const PRESET_MS: Record<PresetId, number> = {
  '1s': 1000,
  '1m': 60 * 1000,
  '5m': 5 * 60 * 1000,
  '15m': 15 * 60 * 1000,
  '1h': 60 * 60 * 1000,
  '4h': 4 * 60 * 60 * 1000,
  '12h': 12 * 60 * 60 * 1000,
  '24h': 24 * 60 * 60 * 1000,
  '7d': 7 * 24 * 60 * 1000,
}

export const PRESET_LABELS: Record<PresetId, string> = {
  '1s': '1с',
  '1m': '1м',
  '5m': '5м',
  '15m': '15м',
  '1h': '1ч',
  '4h': '4ч',
  '12h': '12ч',
  '24h': '24ч',
  '7d': '7д',
}

const SECOND_MS = 1000
const MINUTE_MS = 60 * SECOND_MS

export function defaultWorkingTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
}

/** IANA zones in Russia, west to east. */
export const RUSSIAN_TIME_ZONES = [
  'Europe/Kaliningrad',
  'Europe/Moscow',
  'Europe/Kirov',
  'Europe/Volgograd',
  'Europe/Saratov',
  'Europe/Ulyanovsk',
  'Europe/Astrakhan',
  'Europe/Samara',
  'Asia/Yekaterinburg',
  'Asia/Omsk',
  'Asia/Novosibirsk',
  'Asia/Barnaul',
  'Asia/Tomsk',
  'Asia/Novokuznetsk',
  'Asia/Krasnoyarsk',
  'Asia/Irkutsk',
  'Asia/Chita',
  'Asia/Yakutsk',
  'Asia/Khandyga',
  'Asia/Vladivostok',
  'Asia/Ust-Nera',
  'Asia/Magadan',
  'Asia/Sakhalin',
  'Asia/Srednekolymsk',
  'Asia/Kamchatka',
  'Asia/Anadyr',
] as const

export const RUSSIAN_TIME_ZONE_LABELS = {
  'Europe/Kaliningrad': 'Калининград',
  'Europe/Moscow': 'Москва',
  'Europe/Kirov': 'Киров',
  'Europe/Volgograd': 'Волгоград',
  'Europe/Saratov': 'Саратов',
  'Europe/Ulyanovsk': 'Ульяновск',
  'Europe/Astrakhan': 'Астрахань',
  'Europe/Samara': 'Самара',
  'Asia/Yekaterinburg': 'Екатеринбург',
  'Asia/Omsk': 'Омск',
  'Asia/Novosibirsk': 'Новосибирск',
  'Asia/Barnaul': 'Барнаул',
  'Asia/Tomsk': 'Томск',
  'Asia/Novokuznetsk': 'Новокузнецк',
  'Asia/Krasnoyarsk': 'Красноярск',
  'Asia/Irkutsk': 'Иркутск',
  'Asia/Chita': 'Чита',
  'Asia/Yakutsk': 'Якутск',
  'Asia/Khandyga': 'Хандыга',
  'Asia/Vladivostok': 'Владивосток',
  'Asia/Ust-Nera': 'Усть-Нера',
  'Asia/Magadan': 'Магадан',
  'Asia/Sakhalin': 'Сахалин',
  'Asia/Srednekolymsk': 'Среднеколымск',
  'Asia/Kamchatka': 'Камчатка',
  'Asia/Anadyr': 'Анадырь',
} as const satisfies Record<(typeof RUSSIAN_TIME_ZONES)[number], string>

export const RUSSIAN_TIME_ZONE_ABBREVS = {
  'Europe/Kaliningrad': 'EET',
  'Europe/Moscow': 'MSK',
  'Europe/Kirov': 'MSK',
  'Europe/Volgograd': 'MSK',
  'Europe/Saratov': 'SAMT',
  'Europe/Ulyanovsk': 'SAMT',
  'Europe/Astrakhan': 'SAMT',
  'Europe/Samara': 'SAMT',
  'Asia/Yekaterinburg': 'YEKT',
  'Asia/Omsk': 'OMST',
  'Asia/Novosibirsk': 'NOVT',
  'Asia/Barnaul': 'KRAT',
  'Asia/Tomsk': 'KRAT',
  'Asia/Novokuznetsk': 'KRAT',
  'Asia/Krasnoyarsk': 'KRAT',
  'Asia/Irkutsk': 'IRKT',
  'Asia/Chita': 'YAKT',
  'Asia/Yakutsk': 'YAKT',
  'Asia/Khandyga': 'YAKT',
  'Asia/Vladivostok': 'VLAT',
  'Asia/Ust-Nera': 'VLAT',
  'Asia/Magadan': 'MAGT',
  'Asia/Sakhalin': 'SAKT',
  'Asia/Srednekolymsk': 'SRET',
  'Asia/Kamchatka': 'PETT',
  'Asia/Anadyr': 'ANAT',
} as const satisfies Record<(typeof RUSSIAN_TIME_ZONES)[number], string>

export function timeZoneLabel(timeZone: string): string {
  if (timeZone in RUSSIAN_TIME_ZONE_LABELS) {
    return RUSSIAN_TIME_ZONE_LABELS[timeZone as keyof typeof RUSSIAN_TIME_ZONE_LABELS]
  }
  return timeZone
}

export function timeZoneAbbrev(timeZone: string, instant: Date | string = new Date()): string {
  if (timeZone === 'UTC') return 'UTC'
  if (Object.hasOwn(RUSSIAN_TIME_ZONE_ABBREVS, timeZone)) {
    return RUSSIAN_TIME_ZONE_ABBREVS[timeZone as keyof typeof RUSSIAN_TIME_ZONE_ABBREVS]
  }
  const date = typeof instant === 'string' ? new Date(instant) : instant
  try {
    const name = new Intl.DateTimeFormat('en-US', { timeZone, timeZoneName: 'short' })
      .formatToParts(date)
      .find((part) => part.type === 'timeZoneName')?.value
    if (name && /^[A-Z]{2,5}$/.test(name)) return name
  } catch {
    /* invalid IANA */
  }
  return timeZone
}

export function timeZoneMatchesQuery(timeZone: string, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return (
    timeZone.toLowerCase().includes(q) ||
    timeZoneLabel(timeZone).toLowerCase().includes(q) ||
    timeZoneAbbrev(timeZone).toLowerCase().includes(q)
  )
}

export function listTimeZones(): string[] {
  if (typeof Intl === 'undefined' || !('supportedValuesOf' in Intl)) return ['UTC']
  return Intl.supportedValuesOf('timeZone')
}

export function prioritizeRussianTimeZones(zones: readonly string[]): string[] {
  const ru = RUSSIAN_TIME_ZONES.filter((zone) => zones.includes(zone))
  const rest = zones.filter((zone) => !(RUSSIAN_TIME_ZONES as readonly string[]).includes(zone))
  return [...ru, ...rest]
}

export function activeTimeZone(display: DisplayZone, workingTimeZone: string): string {
  return display === 'utc' ? 'UTC' : workingTimeZone
}

const LEGACY_PRESET_MS: Record<string, number> = {
  '15m': PRESET_MS['15m'],
  '1h': PRESET_MS['1h'],
  '4h': PRESET_MS['4h'],
  '6h': 6 * PRESET_MS['1h'],
  '12h': PRESET_MS['12h'],
  '24h': PRESET_MS['24h'],
  '7d': PRESET_MS['7d'],
  '30d': 30 * PRESET_MS['24h'],
  '90d': 90 * PRESET_MS['24h'],
}

export function intervalFromLegacyPreset(id: string, now: Date = new Date()): TimeInterval {
  const ms = LEGACY_PRESET_MS[id] ?? PRESET_MS['24h']
  return {
    kind: 'relative',
    live: true,
    anchor: now.toISOString(),
    direction: 'before',
    duration: durationFromMs(ms),
  }
}

export function defaultInterval(now: Date = new Date()): TimeInterval {
  return {
    kind: 'relative',
    live: true,
    anchor: now.toISOString(),
    direction: 'before',
    duration: { kind: 'preset', id: '1h' },
  }
}

export function defaultQueueInterval(now: Date = new Date()): TimeInterval {
  return intervalFromLegacyPreset('30d', now)
}

/** Hardcoded demo window: 23.10.2025, 00:00–24:00 in the given zone. */
export const DEMO_DAY = { year: 2025, month: 10, day: 23 } as const

export function demoDayInterval(timeZone = defaultWorkingTimeZone()): TimeInterval {
  const from = partsToInstant(
    { year: DEMO_DAY.year, month: DEMO_DAY.month, day: DEMO_DAY.day, hour: 0, minute: 0, second: 0 },
    timeZone,
  )
  const to = partsToInstant(
    { year: DEMO_DAY.year, month: DEMO_DAY.month, day: DEMO_DAY.day, hour: 23, minute: 59, second: 59 },
    timeZone,
  )
  if (!from || !to) {
    return {
      kind: 'range',
      from: '2025-10-22T21:00:00.000Z',
      to: '2025-10-23T20:59:59.000Z',
    }
  }
  return { kind: 'range', from, to }
}

export function intervalButtonLabel(value: TimeInterval, timeZone = 'UTC'): string {
  if (value.kind === 'relative' && value.live) {
    const duration = durationLabel(value.duration)
    if (value.direction === 'before') return duration
    if (value.direction === 'after') return `после ${duration}`
    return `±${duration}`
  }
  const { from, to } = resolve(value)
  return formatRange(from, to, timeZone, '–')
}

export function durationMs(duration: Duration): number {
  return duration.kind === 'preset' ? PRESET_MS[duration.id] : duration.ms
}

export function durationFromMs(ms: number): Duration {
  const rounded = Math.max(SECOND_MS, Math.round(ms))
  for (const id of PRESET_IDS) {
    if (PRESET_MS[id] === rounded) return { kind: 'preset', id }
  }
  return { kind: 'custom', ms: rounded }
}

export function customDurationFromHm(hours: number, minutes: number): Duration {
  const h = Number.isFinite(hours) ? Math.max(0, Math.floor(hours)) : 0
  const m = Number.isFinite(minutes) ? Math.max(0, Math.floor(minutes)) : 0
  return durationFromMs((h * 60 + m) * MINUTE_MS)
}

export function splitDurationHm(duration: Duration): { hours: number; minutes: number } {
  const totalMinutes = Math.round(durationMs(duration) / MINUTE_MS)
  return { hours: Math.floor(totalMinutes / 60), minutes: totalMinutes % 60 }
}

export function formatDurationMs(ms: number): string {
  const total = Math.max(0, Math.round(ms))
  const days = Math.floor(total / PRESET_MS['24h'])
  let rest = total % PRESET_MS['24h']
  const hours = Math.floor(rest / PRESET_MS['1h'])
  rest %= PRESET_MS['1h']
  const minutes = Math.floor(rest / MINUTE_MS)
  rest %= MINUTE_MS
  const seconds = Math.floor(rest / 1000)

  const parts: string[] = []
  if (days) parts.push(`${days}д`)
  if (hours) parts.push(`${hours}ч`)
  if (minutes) parts.push(`${minutes}м`)
  if (parts.length === 0) return seconds ? `${seconds}с` : '0м'
  return parts.join(' ')
}

export function durationLabel(duration: Duration): string {
  return duration.kind === 'preset' ? PRESET_LABELS[duration.id] : formatDurationMs(duration.ms)
}

export function windowSpanMs(value: TimeInterval): number {
  if (value.kind === 'range') {
    const { from, to } = normalizeRange(value.from, value.to)
    return Math.max(0, Date.parse(to) - Date.parse(from))
  }
  const d = durationMs(value.duration)
  return value.direction === 'around' ? d * 2 : d
}

/** Center a relative window on an event timestamp, keeping the current duration. */
export function intervalAroundInstant(iso: string, current: TimeInterval): TimeInterval {
  const duration =
    current.kind === 'relative'
      ? current.duration
      : durationFromMs(Math.max(windowSpanMs(current), PRESET_MS['1h']))
  return {
    kind: 'relative',
    live: false,
    anchor: iso,
    direction: 'around',
    duration,
  }
}

export function resolve(value: TimeInterval, now: Date = new Date()): ResolvedInterval {
  if (value.kind === 'range') {
    const { from, to } = normalizeRange(value.from, value.to)
    return { from, to }
  }
  const anchorMs = value.live ? now.getTime() : Date.parse(value.anchor)
  const d = durationMs(value.duration)
  let fromMs: number
  let toMs: number
  switch (value.direction) {
    case 'before':
      fromMs = anchorMs - d
      toMs = anchorMs
      break
    case 'after':
      fromMs = anchorMs
      toMs = anchorMs + d
      break
    case 'around':
      fromMs = anchorMs - d
      toMs = anchorMs + d
      break
  }
  return { from: new Date(fromMs).toISOString(), to: new Date(toMs).toISOString() }
}

export function normalizeRange(from: string, to: string): ResolvedInterval & { swapped: boolean } {
  if (Date.parse(from) <= Date.parse(to)) return { from, to, swapped: false }
  return { from: to, to: from, swapped: true }
}

export function switchToRange(value: TimeInterval, now: Date = new Date()): TimeInterval {
  const resolved = resolve(value, now)
  return { kind: 'range', from: resolved.from, to: resolved.to }
}

export function switchToRelative(value: TimeInterval): TimeInterval {
  if (value.kind === 'relative') return value
  const { from, to } = normalizeRange(value.from, value.to)
  return {
    kind: 'relative',
    live: false,
    anchor: from,
    direction: 'after',
    duration: durationFromMs(Date.parse(to) - Date.parse(from)),
  }
}

export function fixate(value: TimeInterval, now: Date = new Date()): TimeInterval {
  if (value.kind !== 'relative' || !value.live) return value
  return { ...value, live: false, anchor: now.toISOString() }
}

export function returnToNow(value: TimeInterval): TimeInterval {
  if (value.kind !== 'relative') return value
  return { ...value, live: true, direction: 'before' }
}

export function setRelativeAnchor(value: TimeInterval, iso: string): TimeInterval {
  if (value.kind !== 'relative') return value
  return { ...value, live: false, anchor: iso }
}

export function parseTimestamp(raw: string, timeZone: string): string | null {
  const s = raw.trim()
  if (!s) return null

  if (/^\d{10}$/.test(s)) return new Date(Number(s) * 1000).toISOString()
  if (/^\d{13}$/.test(s)) {
    const date = new Date(Number(s))
    return Number.isNaN(date.getTime()) ? null : date.toISOString()
  }

  const compact = parseCompactTimestamp(s, timeZone)
  if (compact) return compact

  if (hasExplicitOffset(s)) {
    try {
      return parseAbsolute(s, timeZone).toDate().toISOString()
    } catch {
      return null
    }
  }

  const wall = normalizeWallClock(s)
  if (!wall) return null
  try {
    return toZoned(parseDateTime(wall), timeZone).toDate().toISOString()
  } catch {
    return null
  }
}

function hasExplicitOffset(s: string): boolean {
  return /[zZ]$/.test(s) || /[+-]\d{2}:?\d{2}$/.test(s)
}

function normalizeWallClock(raw: string): string | null {
  const s = raw.trim().replace(/\s+/g, ' ')
  const match = s.match(/^(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}):(\d{2})(?::(\d{2}))?)?$/)
  if (!match) return null
  const [, date, hour, minute, second] = match
  if (!hour) return `${date}T00:00:00`
  return `${date}T${hour}:${minute}:${second ?? '00'}`
}

export const RANGE_SEPARATOR = ' → '
const INSTANT_DISPLAY_CHARS = 'YYYY-MM-DD HH:MM:SS'.length
/** IBM Plex Mono advance is 600/1000em. Sized for two full stamps so the readout does not jump. */
export const RANGE_READOUT_WIDTH_EM = (INSTANT_DISPLAY_CHARS * 2 + RANGE_SEPARATOR.length) * 0.6

export function formatInstant(iso: string, timeZone: string): string {
  const z = fromAbsolute(Date.parse(iso), timeZone)
  return `${z.year}-${pad2(z.month)}-${pad2(z.day)} ${pad2(z.hour)}:${pad2(z.minute)}:${pad2(z.second)}`
}

export function formatClock(iso: string, timeZone: string): string {
  const z = fromAbsolute(Date.parse(iso), timeZone)
  return `${pad2(z.hour)}:${pad2(z.minute)}:${pad2(z.second)}`
}

/** Same calendar day keeps the date on the left only. */
export function formatRange(from: string, to: string, timeZone: string, separator = RANGE_SEPARATOR): string {
  const left = formatInstant(from, timeZone)
  const right = formatInstant(to, timeZone)
  if (left.slice(0, 10) === right.slice(0, 10)) return `${left}${separator}${formatClock(to, timeZone)}`
  return `${left}${separator}${right}`
}

export type WallParts = {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
}

export function instantToParts(iso: string, timeZone: string): WallParts {
  const z = fromAbsolute(Date.parse(iso), timeZone)
  return {
    year: z.year,
    month: z.month,
    day: z.day,
    hour: z.hour,
    minute: z.minute,
    second: z.second,
  }
}

export function partsToInstant(parts: WallParts, timeZone: string): string | null {
  if (!validWallParts(parts)) return null
  try {
    return toZoned(
      new CalendarDateTime(parts.year, parts.month, parts.day, parts.hour, parts.minute, parts.second),
      timeZone,
    )
      .toDate()
      .toISOString()
  } catch {
    return null
  }
}

/** ДДММГГГГ:ЧЧММСС, optionally with spaces around the colon. */
export function parseCompactTimestamp(raw: string, timeZone: string): string | null {
  const compact = raw.trim().replace(/\s+/g, '')
  const match = compact.match(/^(\d{2})(\d{2})(\d{4}):(\d{2})(\d{2})(\d{2})$/)
  if (!match) return null
  return partsToInstant(
    {
      day: Number(match[1]),
      month: Number(match[2]),
      year: Number(match[3]),
      hour: Number(match[4]),
      minute: Number(match[5]),
      second: Number(match[6]),
    },
    timeZone,
  )
}

function validWallParts(parts: WallParts): boolean {
  return (
    parts.year >= 1 &&
    parts.year <= 9999 &&
    parts.month >= 1 &&
    parts.month <= 12 &&
    parts.day >= 1 &&
    parts.day <= 31 &&
    parts.hour >= 0 &&
    parts.hour <= 23 &&
    parts.minute >= 0 &&
    parts.minute <= 59 &&
    parts.second >= 0 &&
    parts.second <= 59
  )
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}
