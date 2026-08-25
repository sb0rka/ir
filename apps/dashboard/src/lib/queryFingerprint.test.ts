import { describe, expect, it } from 'vitest'
import { filterFingerprint } from './queryFingerprint'
import { demoDayInterval } from '../components/time-interval/model'

describe('filterFingerprint', () => {
  it('changes when pdql, the interval, or queue source changes', () => {
    const day = demoDayInterval('Europe/Moscow')
    const hour = {
      kind: 'relative' as const,
      live: true,
      anchor: '2025-10-23T00:00:00.000Z',
      direction: 'before' as const,
      duration: { kind: 'preset' as const, id: '1h' as const },
    }
    const base = filterFingerprint('select(time)', day)
    expect(filterFingerprint('select(time)', day)).toBe(base)
    expect(filterFingerprint('select(time)', day, 'findings')).toBe(base)
    expect(filterFingerprint('select(time) | sort(time desc)', day)).not.toBe(base)
    expect(filterFingerprint('select(time)', hour)).not.toBe(base)
    expect(filterFingerprint('select(time)', day, 'events')).not.toBe(base)
  })
})
