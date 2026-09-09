import { describe, expect, it } from 'vitest'
import { layoutTimeline, TIMELINE_ROW_HEIGHT } from './timeline-layout'
import type { EventRef } from './types'

const start = Date.UTC(2023, 5, 8, 11, 1)
const end = start + 60_000
const events: EventRef[] = Array.from({ length: 69 }, (_, i) => ({
  id: `event-${i}`, source: 'pt-nad', source_event_id: `${i}`,
  event_class: 'network_session', entity_ids: [], title: 'NTLMSSP_AUTH',
  event_ts: new Date([start, start + 12_000, end][i % 3]).toISOString(),
}))

describe('timeline label placement', () => {
  it.each([320, 1200, 1994])('keeps every label accessible without overlap at width %i', (width) => {
    const { markers, height } = layoutTimeline(events, start, end, width)
    expect(markers).toHaveLength(events.length)
    expect(new Set(markers.map((m) => m.event.id)).size).toBe(events.length)
    expect(height).toBeGreaterThan(3 * TIMELINE_ROW_HEIGHT)
    for (let i = 0; i < markers.length; i++) {
      const a = markers[i]
      expect(a.left).toBeGreaterThanOrEqual(0)
      expect(a.left + a.width).toBeLessThanOrEqual(width)
      for (const b of markers.slice(i + 1)) {
        expect(a.top !== b.top || a.left + a.width + 6 <= b.left || b.left + b.width + 6 <= a.left).toBe(true)
      }
    }
    expect(layoutTimeline(events.slice().reverse(), start, end, width)).toEqual({ markers, height })
  })

  it('handles empty and zero-duration windows', () => {
    expect(layoutTimeline([], start, end, 500).markers).toEqual([])
    expect(layoutTimeline(events.slice(0, 1), start, start, 500).markers[0].left).toBe(2)
  })
})
