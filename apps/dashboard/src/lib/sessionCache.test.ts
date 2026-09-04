import { describe, expect, it, vi } from 'vitest'
import { SessionCache } from './sessionCache'

describe('SessionCache', () => {
  it('returns cached value on hit', async () => {
    const cache = new SessionCache<string>()
    const loader = vi.fn(async () => 'a')
    await expect(cache.getOrLoad('k', loader)).resolves.toBe('a')
    await expect(cache.getOrLoad('k', loader)).resolves.toBe('a')
    expect(loader).toHaveBeenCalledTimes(1)
  })

  it('dedupes parallel loads for the same key', async () => {
    const cache = new SessionCache<string>()
    let resolveLoader!: (value: string) => void
    const loader = vi.fn(
      () =>
        new Promise<string>((resolve) => {
          resolveLoader = resolve
        }),
    )
    const p1 = cache.getOrLoad('k', loader)
    const p2 = cache.getOrLoad('k', loader)
    expect(loader).toHaveBeenCalledTimes(1)
    resolveLoader('shared')
    await expect(Promise.all([p1, p2])).resolves.toEqual(['shared', 'shared'])
  })

  it('force bypasses cache and reloads', async () => {
    const cache = new SessionCache<string>()
    const loader = vi.fn(async () => 'v1')
    await cache.getOrLoad('k', loader)
    loader.mockResolvedValueOnce('v2')
    await expect(cache.getOrLoad('k', loader, { force: true })).resolves.toBe('v2')
    expect(loader).toHaveBeenCalledTimes(2)
    await expect(cache.getOrLoad('k', loader)).resolves.toBe('v2')
    expect(loader).toHaveBeenCalledTimes(2)
  })

  it('does not cache rejected loads', async () => {
    const cache = new SessionCache<string>()
    const loader = vi
      .fn()
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce('ok')
    await expect(cache.getOrLoad('k', loader)).rejects.toThrow('boom')
    await expect(cache.getOrLoad('k', loader)).resolves.toBe('ok')
    expect(loader).toHaveBeenCalledTimes(2)
  })

  it('skips storing when shouldCache returns false', async () => {
    const cache = new SessionCache<{ ok: boolean }>()
    const loader = vi.fn(async () => ({ ok: false }))
    await expect(
      cache.getOrLoad('k', loader, { shouldCache: (v) => v.ok }),
    ).resolves.toEqual({ ok: false })
    expect(cache.get('k')).toBeUndefined()
    await cache.getOrLoad('k', loader, { shouldCache: (v) => v.ok })
    expect(loader).toHaveBeenCalledTimes(2)
  })

  it('evicts least-recently-used entries', async () => {
    const cache = new SessionCache<string>(2)
    await cache.getOrLoad('a', async () => 'A')
    await cache.getOrLoad('b', async () => 'B')
    // Touch a so b is older for next insert.
    expect(cache.get('a')).toBe('A')
    await cache.getOrLoad('c', async () => 'C')
    expect(cache.get('b')).toBeUndefined()
    expect(cache.get('a')).toBe('A')
    expect(cache.get('c')).toBe('C')
    expect(cache.size()).toBe(2)
  })

  it('clear removes values and inflight', async () => {
    const cache = new SessionCache<string>()
    await cache.getOrLoad('k', async () => 'v')
    cache.clear()
    expect(cache.get('k')).toBeUndefined()
    expect(cache.size()).toBe(0)
  })
})
