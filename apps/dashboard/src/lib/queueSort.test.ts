import { describe, expect, it } from 'vitest'
import type { AlertEvent } from '../types'
import { sortQueueAlerts } from './queueSort'

function alert(partial: Partial<AlertEvent> & Pick<AlertEvent, 'id'>): AlertEvent {
  return {
    time: '2025-10-23T12:00:00.000Z',
    severity: 'high',
    title: 't',
    rule: 'r',
    source: 'pt-maxpatrol-siem',
    status: 'new',
    entityIds: [],
    description: 'd',
    ...partial,
  }
}

describe('sortQueueAlerts', () => {
  const older = alert({ id: 'old', time: '2025-10-23T10:00:00.000Z', raw: { 'event_src.host': 'b' } })
  const newer = alert({ id: 'new', time: '2025-10-23T18:00:00.000Z', raw: { 'event_src.host': 'a' } })

  it('defaults to time desc', () => {
    expect(sortQueueAlerts([older, newer]).map((item) => item.id)).toEqual(['new', 'old'])
  })

  it('honors time asc', () => {
    expect(
      sortQueueAlerts([newer, older], [{ field: 'time', direction: 'asc' }]).map((item) => item.id),
    ).toEqual(['old', 'new'])
  })

  it('applies secondary keys after time', () => {
    const hostB = alert({ id: 'b', time: older.time, raw: { 'event_src.host': 'b' } })
    const hostA = alert({ id: 'a', time: older.time, raw: { 'event_src.host': 'a' } })
    expect(
      sortQueueAlerts(
        [hostB, hostA],
        [
          { field: 'time', direction: 'desc' },
          { field: 'event_src.host', direction: 'asc' },
        ],
      ).map((item) => item.id),
    ).toEqual(['a', 'b'])
  })
})
