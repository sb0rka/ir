import { formatTime } from './utils'
import type { AlertEvent, CorrelationGroup } from '../types'
import type { AlertTableColumnId } from '../components/alertTableColumns'
import { incidentTypeLabelRu } from './pdql'

export type {
  AlertTableColumn as QueueSearchColumn,
  AlertTableColumnId as QueueSearchColumnId,
} from '../components/alertTableColumns'
export {
  DEFAULT_ALERT_TABLE_SEARCH_COLUMN as DEFAULT_QUEUE_SEARCH_COLUMN,
  alertTableSearchColumns as queueSearchColumns,
  fieldColumnId as fieldSearchColumnId,
  resolveAlertTableSearchColumn as resolveQueueSearchColumn,
} from '../components/alertTableColumns'

function includesNeedle(haystack: string, needle: string): boolean {
  return !needle || haystack.toLowerCase().includes(needle)
}

function alertColumnText(alert: AlertEvent, column: AlertTableColumnId): string {
  if (column === 'severity') return alert.severity
  if (column === 'time') return [formatTime(alert.time), alert.time].filter(Boolean).join(' ')
  if (column === 'title') {
    return [alert.title, alert.rule, alert.description].filter(Boolean).join(' ')
  }
  if (column === 'category') {
    const code = alert.raw?.['incident.type'] ?? ''
    return [incidentTypeLabelRu(code), code].filter(Boolean).join(' ')
  }
  if (column === 'source') return alert.source
  if (column.startsWith('field:')) {
    const field = column.slice('field:'.length)
    if (field === 'time') return alert.time
    return alert.raw?.[field] ?? ''
  }
  return ''
}

function correlationColumnText(group: CorrelationGroup, column: AlertTableColumnId): string {
  if (column === 'severity') return group.severity
  if (column === 'time') return [formatTime(group.time), group.time].filter(Boolean).join(' ')
  if (column === 'title') return [group.title, group.reason].filter(Boolean).join(' ')
  if (column === 'category') return ''
  if (column === 'source') {
    return Object.entries(group.sourceCounts)
      .filter(([, count]) => (count ?? 0) > 0)
      .map(([source]) => source)
      .join(' ')
  }
  return ''
}

export function alertMatchesQueueText(
  alert: AlertEvent,
  needle: string,
  column: AlertTableColumnId,
): boolean {
  return includesNeedle(alertColumnText(alert, column), needle)
}

export function correlationMatchesQueueText(
  group: CorrelationGroup,
  needle: string,
  column: AlertTableColumnId,
): boolean {
  return includesNeedle(correlationColumnText(group, column), needle)
}
