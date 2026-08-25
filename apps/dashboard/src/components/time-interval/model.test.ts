import { describe, expect, it } from 'vitest'
import {
  customDurationFromHm,
  durationFromMs,
  durationMs,
  fixate,
  formatDurationMs,
  formatInstant,
  formatRange,
  intervalButtonLabel,
  RANGE_SEPARATOR,
  intervalFromLegacyPreset,
  intervalAroundInstant,
  normalizeRange,
  parseTimestamp,
  PRESET_MS,
  prioritizeRussianTimeZones,
  timeZoneAbbrev,
  timeZoneLabel,
  resolve,
  returnToNow,
  switchToRange,
  switchToRelative,
  windowSpanMs,
  demoDayInterval,
  type TimeInterval,
} from './model'

const ANCHOR = '2026-08-24T14:00:00.000Z'
const NOW = new Date('2026-08-24T18:00:00.000Z')

function relative(partial: Partial<Extract<TimeInterval, { kind: 'relative' }>> = {}): TimeInterval {
  return {
    kind: 'relative',
    live: false,
    anchor: ANCHOR,
    direction: 'around',
    duration: { kind: 'preset', id: '1h' },
    ...partial,
  }
}

describe('resolve', () => {
  it('around ±1h is a 2h window centered on the anchor', () => {
    expect(resolve(relative({ direction: 'around', duration: { kind: 'preset', id: '1h' } }))).toEqual({
      from: '2026-08-24T13:00:00.000Z',
      to: '2026-08-24T15:00:00.000Z',
    })
    expect(windowSpanMs(relative({ direction: 'around', duration: { kind: 'preset', id: '1h' } }))).toBe(
      PRESET_MS['1h'] * 2,
    )
  })

  it('before ends at the anchor', () => {
    expect(resolve(relative({ direction: 'before', duration: { kind: 'preset', id: '4h' } }))).toEqual({
      from: '2026-08-24T10:00:00.000Z',
      to: ANCHOR,
    })
  })

  it('after starts at the anchor', () => {
    expect(resolve(relative({ direction: 'after', duration: { kind: 'preset', id: '15m' } }))).toEqual({
      from: ANCHOR,
      to: '2026-08-24T14:15:00.000Z',
    })
  })

  it('live relative uses injected now and ignores stored anchor', () => {
    expect(
      resolve(
        relative({ live: true, direction: 'before', duration: { kind: 'preset', id: '1h' } }),
        NOW,
      ),
    ).toEqual({
      from: '2026-08-24T17:00:00.000Z',
      to: NOW.toISOString(),
    })
  })

  it('frozen relative ignores injected now', () => {
    expect(
      resolve(relative({ live: false, direction: 'before', duration: { kind: 'preset', id: '1h' } }), NOW),
    ).toEqual({
      from: '2026-08-24T13:00:00.000Z',
      to: ANCHOR,
    })
  })

  it('range autoswaps inverted bounds', () => {
    expect(
      resolve({
        kind: 'range',
        from: '2026-08-24T18:00:00.000Z',
        to: '2026-08-24T14:00:00.000Z',
      }),
    ).toEqual({
      from: ANCHOR,
      to: '2026-08-24T18:00:00.000Z',
    })
  })
})

describe('duration mapping', () => {
  it('maps exact preset milliseconds back to a preset', () => {
    expect(durationFromMs(PRESET_MS['12h'])).toEqual({ kind: 'preset', id: '12h' })
  })

  it('keeps a non-preset length as custom', () => {
    const duration = durationFromMs(3 * PRESET_MS['1h'] + 17 * 60 * 1000)
    expect(duration).toEqual({ kind: 'custom', ms: 3 * PRESET_MS['1h'] + 17 * 60 * 1000 })
    expect(formatDurationMs(durationMs(duration))).toBe('3 ч 17 мин')
  })

  it('builds custom duration from hours and minutes', () => {
    expect(customDurationFromHm(2, 30)).toEqual({ kind: 'custom', ms: 2.5 * PRESET_MS['1h'] })
  })
})

describe('normalizeRange', () => {
  it('reports a swap when from is after to', () => {
    expect(
      normalizeRange('2026-08-24T18:00:00.000Z', '2026-08-24T14:00:00.000Z'),
    ).toEqual({
      from: ANCHOR,
      to: '2026-08-24T18:00:00.000Z',
      swapped: true,
    })
  })

  it('leaves an ordered range in place', () => {
    expect(normalizeRange(ANCHOR, '2026-08-24T18:00:00.000Z').swapped).toBe(false)
  })
})

describe('parseTimestamp', () => {
  it('parses ISO with an explicit offset as an instant', () => {
    expect(parseTimestamp('2026-08-24T14:00:00.000Z', 'Europe/Moscow')).toBe(ANCHOR)
  })

  it('parses unix seconds and milliseconds', () => {
    const ms = Date.parse(ANCHOR)
    expect(parseTimestamp(String(Math.floor(ms / 1000)), 'UTC')).toBe(ANCHOR)
    expect(parseTimestamp(String(ms), 'UTC')).toBe(ANCHOR)
  })

  it('interprets ДДММГГГГ:ЧЧММСС in the given IANA zone', () => {
    expect(parseTimestamp('24082026:170000', 'Europe/Moscow')).toBe(ANCHOR)
    expect(parseTimestamp('24 08 2026 : 14 00 00', 'UTC')).toBe(ANCHOR)
  })

  it('interprets wall-clock text in the given IANA zone', () => {
    expect(parseTimestamp('2026-08-24 17:00:00', 'Europe/Moscow')).toBe(ANCHOR)
    expect(parseTimestamp('2026-08-24 14:00:00', 'UTC')).toBe(ANCHOR)
  })

  it('uses DST offset for America/New_York winter vs summer', () => {
    expect(parseTimestamp('2025-01-15 12:00:00', 'America/New_York')).toBe('2025-01-15T17:00:00.000Z')
    expect(parseTimestamp('2025-07-15 12:00:00', 'America/New_York')).toBe('2025-07-15T16:00:00.000Z')
  })

  it('returns null for invalid input without changing meaning', () => {
    expect(parseTimestamp('not a time', 'UTC')).toBeNull()
    expect(parseTimestamp('', 'UTC')).toBeNull()
  })
})

describe('timezone display', () => {
  it('reformats an instant without moving it', () => {
    const iso = ANCHOR
    expect(formatInstant(iso, 'UTC')).toBe('2026-08-24 14:00:00')
    expect(formatInstant(iso, 'Europe/Moscow')).toBe('2026-08-24 17:00:00')
    expect(parseTimestamp(formatInstant(iso, 'Europe/Moscow'), 'Europe/Moscow')).toBe(iso)
    expect(parseTimestamp(formatInstant(iso, 'UTC'), 'UTC')).toBe(iso)
  })

  it('keeps the date on the left for a same-day range', () => {
    expect(formatRange('2026-08-24T13:00:00.000Z', '2026-08-24T15:00:00.000Z', 'UTC')).toBe(
      '2026-08-24 13:00:00 → 15:00:00',
    )
  })

  it('shows both dates when the range crosses midnight', () => {
    const label = formatRange('2026-08-23T22:00:00.000Z', '2026-08-24T02:00:00.000Z', 'UTC')
    expect(label).toBe('2026-08-23 22:00:00 → 2026-08-24 02:00:00')
    expect(label.length).toBe('YYYY-MM-DD HH:MM:SS'.length * 2 + RANGE_SEPARATOR.length)
  })

  it('includes the date on a frozen button label', () => {
    expect(
      intervalButtonLabel({
        kind: 'range',
        from: '2026-08-24T13:00:00.000Z',
        to: '2026-08-24T15:00:00.000Z',
      }),
    ).toBe('2026-08-24 13:00:00–15:00:00')
  })

  it('puts Russian zones first, west to east', () => {
    expect(prioritizeRussianTimeZones(['UTC', 'Asia/Tokyo', 'Asia/Anadyr', 'Europe/Moscow'])).toEqual([
      'Europe/Moscow',
      'Asia/Anadyr',
      'UTC',
      'Asia/Tokyo',
    ])
  })

  it('labels Russian zones in Russian and leaves others as IANA', () => {
    expect(timeZoneLabel('Europe/Moscow')).toBe('Москва')
    expect(timeZoneLabel('Asia/Yekaterinburg')).toBe('Екатеринбург')
    expect(timeZoneLabel('Asia/Tokyo')).toBe('Asia/Tokyo')
    expect(timeZoneLabel('UTC')).toBe('UTC')
  })

  it('uses international abbreviations in the interval readout', () => {
    expect(timeZoneAbbrev('UTC')).toBe('UTC')
    expect(timeZoneAbbrev('Europe/Moscow')).toBe('MSK')
    expect(timeZoneAbbrev('Asia/Yekaterinburg')).toBe('YEKT')
    expect(timeZoneAbbrev('Asia/Anadyr')).toBe('ANAT')
    expect(timeZoneAbbrev('America/New_York', '2026-02-02T18:50:47.000Z')).toBe('EST')
  })
})

describe('mode switch', () => {
  it('relative after 1h round-trips through range', () => {
    const start = relative({ direction: 'after', duration: { kind: 'preset', id: '1h' } })
    const range = switchToRange(start)
    expect(range).toEqual({
      kind: 'range',
      from: ANCHOR,
      to: '2026-08-24T15:00:00.000Z',
    })
    expect(switchToRelative(range)).toEqual(start)
  })

  it('around becomes after with the full span when switching to relative', () => {
    const range = switchToRange(relative({ direction: 'around', duration: { kind: 'preset', id: '1h' } }))
    expect(switchToRelative(range)).toEqual({
      kind: 'relative',
      live: false,
      anchor: '2026-08-24T13:00:00.000Z',
      direction: 'after',
      duration: { kind: 'custom', ms: PRESET_MS['1h'] * 2 },
    })
  })
})

describe('live state', () => {
  it('fixate stores now as the anchor', () => {
    const live = relative({ live: true, direction: 'before' })
    expect(fixate(live, NOW)).toEqual({
      ...live,
      live: false,
      anchor: NOW.toISOString(),
    })
  })

  it('returnToNow restores the live token without dropping duration', () => {
    const frozen = relative({ live: false, duration: { kind: 'preset', id: '4h' } })
    expect(returnToNow(frozen)).toMatchObject({ live: true, duration: { kind: 'preset', id: '4h' } })
  })

  it('returnToNow forces direction before', () => {
    const frozen = relative({ live: false, direction: 'around' })
    expect(returnToNow(frozen)).toMatchObject({ live: true, direction: 'before' })
  })
})

describe('legacy queue presets', () => {
  it('maps 24h to the live before preset', () => {
    expect(intervalFromLegacyPreset('24h', NOW)).toEqual({
      kind: 'relative',
      live: true,
      anchor: NOW.toISOString(),
      direction: 'before',
      duration: { kind: 'preset', id: '24h' },
    })
  })

  it('keeps 30d as a custom live window', () => {
    const interval = intervalFromLegacyPreset('30d', NOW)
    expect(interval).toMatchObject({
      kind: 'relative',
      live: true,
      direction: 'before',
      duration: { kind: 'custom', ms: 30 * PRESET_MS['24h'] },
    })
    expect(intervalButtonLabel(interval)).toBe('30 дней')
  })
})

describe('demoDayInterval', () => {
  it('covers 23.10.2025 from midnight to end of day in Moscow', () => {
    const interval = demoDayInterval('Europe/Moscow')
    expect(interval).toEqual({
      kind: 'range',
      from: '2025-10-22T21:00:00.000Z',
      to: '2025-10-23T20:59:59.000Z',
    })
    const resolved = resolve(interval)
    expect(formatInstant(resolved.from, 'Europe/Moscow')).toBe('2025-10-23 00:00:00')
    expect(formatInstant(resolved.to, 'Europe/Moscow')).toBe('2025-10-23 23:59:59')
  })
})

describe('intervalAroundInstant', () => {
  it('centers the current duration on the event timestamp', () => {
    expect(intervalAroundInstant('2025-10-23T15:58:37.000Z', relative())).toEqual({
      kind: 'relative',
      live: false,
      anchor: '2025-10-23T15:58:37.000Z',
      direction: 'around',
      duration: { kind: 'preset', id: '1h' },
    })
  })
})
