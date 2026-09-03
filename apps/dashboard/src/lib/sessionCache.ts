type CacheEntry<T> = { value: T; at: number }

export type GetOrLoadOptions<T> = {
  force?: boolean
  /** When false, the loaded value is returned but not stored. Default: always cache. */
  shouldCache?: (value: T) => boolean
}

/**
 * In-memory session cache with LRU eviction and in-flight request dedup.
 * No TTL — callers invalidate via force/delete/clear.
 */
export class SessionCache<T> {
  private readonly values = new Map<string, CacheEntry<T>>()
  private readonly inflight = new Map<string, Promise<T>>()
  private readonly maxEntries: number

  constructor(maxEntries = 50) {
    this.maxEntries = Math.max(1, maxEntries)
  }

  get(key: string): T | undefined {
    const entry = this.values.get(key)
    if (!entry) return undefined
    // Refresh LRU order.
    this.values.delete(key)
    this.values.set(key, entry)
    return entry.value
  }

  set(key: string, value: T): void {
    if (this.values.has(key)) this.values.delete(key)
    this.values.set(key, { value, at: Date.now() })
    this.evict()
  }

  delete(key: string): void {
    this.values.delete(key)
    this.inflight.delete(key)
  }

  clear(): void {
    this.values.clear()
    this.inflight.clear()
  }

  size(): number {
    return this.values.size
  }

  async getOrLoad(
    key: string,
    loader: () => Promise<T>,
    options: GetOrLoadOptions<T> = {},
  ): Promise<T> {
    if (!options.force) {
      const hit = this.get(key)
      if (hit !== undefined) return hit
      const pending = this.inflight.get(key)
      if (pending) return pending
    } else {
      this.values.delete(key)
    }

    const promise = loader()
      .then((value) => {
        if (options.shouldCache?.(value) !== false) this.set(key, value)
        return value
      })
      .finally(() => {
        if (this.inflight.get(key) === promise) this.inflight.delete(key)
      })

    this.inflight.set(key, promise)
    return promise
  }

  private evict(): void {
    while (this.values.size > this.maxEntries) {
      const oldest = this.values.keys().next().value
      if (oldest === undefined) break
      this.values.delete(oldest)
    }
  }
}
