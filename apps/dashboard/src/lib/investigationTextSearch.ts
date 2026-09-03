import { formatTime, statusLabel, verdictLabel } from './utils'
import type { Investigation } from '../types'
import type { InvestigationTableColumnId } from '../components/investigationTableColumns'

function includesNeedle(haystack: string, needle: string): boolean {
  return !needle || haystack.toLowerCase().includes(needle)
}

function investigationColumnText(
  investigation: Investigation,
  column: InvestigationTableColumnId,
): string {
  if (column === 'updatedAt') {
    if (!investigation.updatedAt) return ''
    return [formatTime(investigation.updatedAt), investigation.updatedAt].join(' ')
  }
  if (column === 'severity') return investigation.severity
  if (column === 'status') {
    return [
      statusLabel[investigation.status] ?? investigation.status,
      investigation.status,
      investigation.verdict ? (verdictLabel[investigation.verdict] ?? investigation.verdict) : '',
      investigation.verdict ?? '',
    ]
      .filter(Boolean)
      .join(' ')
  }
  return [investigation.title, investigation.description].filter(Boolean).join(' ')
}

export function investigationMatchesText(
  investigation: Investigation,
  needle: string,
  column: InvestigationTableColumnId,
): boolean {
  return includesNeedle(investigationColumnText(investigation, column), needle)
}
