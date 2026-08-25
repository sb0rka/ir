import type { TimeInterval } from '../components/time-interval/model'
import { DEFAULT_QUEUE_SOURCE, type QueueSource } from '../types'

export function filterFingerprint(
  pdql: string,
  interval: TimeInterval,
  queueSource: QueueSource = DEFAULT_QUEUE_SOURCE,
): string {
  return `${pdql.trim()}\n${JSON.stringify(interval)}\n${queueSource}`
}
