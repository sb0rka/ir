import { describe, expect, it } from 'vitest'
import { applyPaste, nextTypeDelay, typedPasteCaret, typedPasteValue } from './typewriterPaste'

const mid = () => 0.5

describe('applyPaste', () => {
  it('inserts at the caret', () => {
    const split = applyPaste('ab', 1, 1, 'X')
    expect(typedPasteValue(split, split.pasted.length)).toBe('aXb')
    expect(typedPasteCaret(split, split.pasted.length)).toBe(2)
  })

  it('replaces the selection, including a backwards range', () => {
    const split = applyPaste('hello', 5, 1, 'i')
    expect(split).toEqual({ before: 'h', pasted: 'i', after: '' })
    expect(typedPasteValue(split, 1)).toBe('hi')
  })

  it('clamps a selection past the end', () => {
    const split = applyPaste('ab', 8, 9, 'c')
    expect(typedPasteValue(split, 1)).toBe('abc')
  })
})

describe('nextTypeDelay', () => {
  it('types long remaining text faster than a short phrase', () => {
    const short = nextTypeDelay('а', 20, mid)
    const medium = nextTypeDelay('а', 50, mid)
    const long = nextTypeDelay('а', 100, mid)
    expect(long).toBeLessThan(medium)
    expect(medium).toBeLessThan(short)
    expect(short).toBeGreaterThanOrEqual(72)
    expect(short).toBeLessThanOrEqual(128)
  })

  it('pauses after a period more than after a letter', () => {
    expect(nextTypeDelay('.', 20, mid)).toBeGreaterThan(nextTypeDelay('а', 20, mid))
  })

  it('keeps a long remainder inside the demo budget', () => {
    const remaining = 400
    expect(nextTypeDelay('а', remaining, mid)).toBeLessThanOrEqual(10000 / remaining)
  })
})
