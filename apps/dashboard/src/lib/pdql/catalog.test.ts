import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { EventFieldDef } from './model'
import { bumpFieldFreq, loadFieldFreq, sortFields } from './catalog'

const memory = new Map<string, string>()

vi.stubGlobal('localStorage', {
  getItem: (key: string) => memory.get(key) ?? null,
  setItem: (key: string, value: string) => {
    memory.set(key, value)
  },
  removeItem: (key: string) => {
    memory.delete(key)
  },
  clear: () => {
    memory.clear()
  },
  key: (index: number) => [...memory.keys()][index] ?? null,
  get length() {
    return memory.size
  },
})

function field(name: string): EventFieldDef {
  return { name, type: 'string', description: name }
}

describe('sortFields', () => {
  it('orders unused fields by default frequency then name', () => {
    const names = sortFields(
      [field('importance'), field('src.port'), field('time')],
      {},
      '',
    ).map((item) => item.name)
    expect(names).toEqual(['time', 'importance', 'src.port'])
  })

  it('lifts a field with usage 1 above every default seed', () => {
    const names = sortFields(
      [field('time'), field('importance'), field('src.port')],
      { 'src.port': 1 },
      '',
    ).map((item) => item.name)
    expect(names).toEqual(['src.port', 'time', 'importance'])
  })
})

describe('field usage persistence', () => {
  beforeEach(() => {
    memory.clear()
  })

  afterEach(() => {
    memory.clear()
  })

  it('bump writes the counter and load reads it back', () => {
    expect(loadFieldFreq()).toEqual({})
    const once = bumpFieldFreq({}, 'src.port')
    expect(once).toEqual({ 'src.port': 1 })
    expect(loadFieldFreq()).toEqual({ 'src.port': 1 })
    expect(bumpFieldFreq(once, 'src.port')).toEqual({ 'src.port': 2 })
    expect(loadFieldFreq()).toEqual({ 'src.port': 2 })
  })
})
