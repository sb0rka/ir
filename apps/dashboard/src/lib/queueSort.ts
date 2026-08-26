import type { AlertEvent } from '../types'

export type QueueSort = { field: string; direction: 'asc' | 'desc' }[]

const DEFAULT_SORT: QueueSort = [{ field: 'time', direction: 'desc' }]

function fieldValue(alert: AlertEvent, field: string): string {
  if (field === 'time') return alert.time
  return alert.raw?.[field] ?? ''
}

export function sortQueueAlerts(alerts: AlertEvent[], sort?: QueueSort): AlertEvent[] {
  const rules = sort?.length ? sort : DEFAULT_SORT
  return alerts.slice().sort((a, b) => {
    for (const rule of rules) {
      const cmp = fieldValue(a, rule.field).localeCompare(fieldValue(b, rule.field))
      if (cmp !== 0) return rule.direction === 'asc' ? cmp : -cmp
    }
    return 0
  })
}
