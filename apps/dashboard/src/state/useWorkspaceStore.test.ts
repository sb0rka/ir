import { describe, expect, it } from 'vitest'
import { contextTimelineWindow } from './useWorkspaceStore'

const PAD_MS = 5 * 60_000

describe('contextTimelineWindow', () => {
  it('spans earliest to latest context event with no padding', () => {
    expect(
      contextTimelineWindow([
        '2025-10-23T16:42:55.000Z',
        '2025-10-23T15:53:21.000Z',
        '2025-10-23T16:10:00.000Z',
      ]),
    ).toEqual({
      windowStart: '2025-10-23T15:53:21.000Z',
      windowEnd: '2025-10-23T16:42:55.000Z',
    })
  })

  it('pads ±5 minutes when every timestamp collapses to one instant', () => {
    const instant = '2025-10-23T15:53:21.000Z'
    const t = Date.parse(instant)
    expect(contextTimelineWindow([instant, instant])).toEqual({
      windowStart: new Date(t - PAD_MS).toISOString(),
      windowEnd: new Date(t + PAD_MS).toISOString(),
    })
  })

  it('uses an epoch-to-now stub when the context has no timestamps', () => {
    const now = Date.parse('2026-09-08T11:23:00.000Z')
    expect(contextTimelineWindow([], now)).toEqual({
      windowStart: new Date(0 - PAD_MS).toISOString(),
      windowEnd: new Date(now + PAD_MS).toISOString(),
    })
    expect(contextTimelineWindow(['not-a-date'], now)).toEqual({
      windowStart: new Date(0 - PAD_MS).toISOString(),
      windowEnd: new Date(now + PAD_MS).toISOString(),
    })
  })
})
