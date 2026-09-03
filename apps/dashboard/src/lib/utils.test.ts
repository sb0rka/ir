import { describe, expect, it } from 'vitest'
import { eventOriginLabel, matchesOriginFilter } from './utils'

describe('matchesOriginFilter', () => {
  it('treats isSeed as исходные, not attached_by', () => {
    const seed = { origin: 'analyst', isSeed: true }
    const analyst = { origin: 'analyst', isSeed: false }
    const derived = { origin: 'seed', isSeed: false }

    expect(matchesOriginFilter(seed, 'seed')).toBe(true)
    expect(matchesOriginFilter(analyst, 'seed')).toBe(false)
    expect(matchesOriginFilter(derived, 'seed')).toBe(false)

    expect(matchesOriginFilter(seed, 'analyst')).toBe(false)
    expect(matchesOriginFilter(analyst, 'analyst')).toBe(true)
    expect(matchesOriginFilter(derived, 'analyst')).toBe(false)
  })
})

describe('eventOriginLabel', () => {
  it('labels seed membership even when attached_by is analyst', () => {
    expect(eventOriginLabel({ origin: 'analyst', isSeed: true })).toBe('исходный')
    expect(eventOriginLabel({ origin: 'analyst', isSeed: false })).toBe('аналитик')
  })
})
