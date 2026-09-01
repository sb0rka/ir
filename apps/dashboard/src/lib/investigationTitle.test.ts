import { describe, expect, it } from 'vitest'
import { titlesForQueueIds } from './investigationTitle'

describe('titlesForQueueIds', () => {
  it('returns a single alert title', () => {
    expect(
      titlesForQueueIds(['a1'], { a1: { title: 'Encoded PowerShell' } }, {}),
    ).toEqual(['Encoded PowerShell'])
  })

  it('prefers correlation title over alert with the same id', () => {
    expect(
      titlesForQueueIds(
        ['c1'],
        { c1: { title: 'alert title' } },
        { c1: { title: 'Компрометация WS-1042' } },
      ),
    ).toEqual(['Компрометация WS-1042'])
  })

  it('collapses duplicate titles and skips missing or blank ones', () => {
    expect(
      titlesForQueueIds(
        ['a1', 'a2', 'missing', 'a3', 'c1'],
        {
          a1: { title: 'login' },
          a2: { title: 'login' },
          a3: { title: '   ' },
        },
        { c1: { title: 'C2-соединение' } },
      ),
    ).toEqual(['login', 'C2-соединение'])
  })
})
