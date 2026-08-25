import type { TimeInterval } from '../components/time-interval/model'
import type { QueueSource } from '../types'

export function filterFingerprint(
  pdql: string,
  interval: TimeInterval,
  queueSource: QueueSource = 'findings',
): string {
  return `${pdql.trim()}\n${JSON.stringify(interval)}\n${queueSource}`
}
