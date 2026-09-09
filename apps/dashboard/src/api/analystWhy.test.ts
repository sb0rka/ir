import { describe, expect, it } from 'vitest'
import { eventNodeLabel } from './analystWhy'

describe('eventNodeLabel', () => {
  it('prefers IR why over the event title', () => {
    expect(
      eventNodeLabel({
        why: 'agent rationale',
        fallback: 'login failed',
      }),
    ).toBe('agent rationale')
  })

  it('falls back to the event title when why is blank', () => {
    expect(
      eventNodeLabel({
        why: '  ',
        fallback: 'login failed',
      }),
    ).toBe('login failed')
    expect(eventNodeLabel({ fallback: 'login failed' })).toBe('login failed')
  })
})
