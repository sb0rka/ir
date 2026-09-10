import { describe, expect, it } from 'vitest'
import { eventNodeLabel } from './analystWhy'

describe('eventNodeLabel', () => {
  it('uses why only for analyst-added nodes', () => {
    expect(
      eventNodeLabel({
        why: 'suspicious login',
        fallback: 'login failed',
        origin: 'analyst',
      }),
    ).toBe('suspicious login')
  })

  it('keeps the event title for agent, seed, and rule nodes even when why is set', () => {
    expect(
      eventNodeLabel({
        why: 'agent rationale',
        fallback: 'login failed',
        origin: 'agent',
      }),
    ).toBe('login failed')
    expect(
      eventNodeLabel({
        why: 'seed note',
        fallback: 'login failed',
        origin: 'seed',
      }),
    ).toBe('login failed')
    expect(
      eventNodeLabel({
        why: 'rule note',
        fallback: 'login failed',
        origin: 'rule',
      }),
    ).toBe('login failed')
  })

  it('falls back to the event title when analyst why is blank', () => {
    expect(
      eventNodeLabel({
        why: '  ',
        fallback: 'login failed',
        origin: 'analyst',
      }),
    ).toBe('login failed')
    expect(eventNodeLabel({ fallback: 'login failed' })).toBe('login failed')
  })
})
