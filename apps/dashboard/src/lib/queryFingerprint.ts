import type { TimeInterval } from '../components/time-interval/model'

export function filterFingerprint(pdql: string, interval: TimeInterval): string {
  return `${pdql.trim()}\n${JSON.stringify(interval)}`
}
