import { clamp, toMs } from './time'
import type { EventRef } from './types'

export const TIMELINE_ROW_HEIGHT = 24

/** Place labels in free lanes while keeping every event individually selectable. */
export function layoutTimeline(events: EventRef[], start: number, end: number, width: number) {
  const labelWidth = Math.min(136, Math.max(0, width - 4))
  const laneEnds: number[] = []
  const markers = events.slice()
    .sort((a, b) => toMs(a.event_ts) - toMs(b.event_ts) || a.id.localeCompare(b.id))
    .map((event) => {
      const center = (toMs(event.event_ts) - start) / Math.max(end - start, 1) * width
      const left = clamp(center - labelWidth / 2, 2, Math.max(2, width - labelWidth - 2))
      let lane = laneEnds.findIndex((right) => right + 6 <= left)
      if (lane < 0) lane = laneEnds.length
      laneEnds[lane] = left + labelWidth
      return { event, left, top: lane * TIMELINE_ROW_HEIGHT + 3, width: labelWidth }
    })
  return { markers, height: laneEnds.length * TIMELINE_ROW_HEIGHT + 6 }
}
