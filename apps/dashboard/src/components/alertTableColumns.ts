/** Fixed searchable columns of the queue alert table — single source for headers + search. */
export type AlertTableFixedColumnId = 'severity' | 'time' | 'title' | 'category' | 'source'

export type AlertTableColumnId = AlertTableFixedColumnId | `field:${string}`

export type AlertTableColumn = {
  id: AlertTableColumnId
  label: string
}

export const ALERT_TABLE_FIXED_COLUMNS: ReadonlyArray<{
  id: AlertTableFixedColumnId
  label: string
}> = [
  { id: 'severity', label: 'Крит.' },
  { id: 'time', label: 'Время' },
  { id: 'title', label: 'Название' },
  { id: 'category', label: 'Категория' },
  { id: 'source', label: 'Источник' },
]

export const DEFAULT_ALERT_TABLE_SEARCH_COLUMN: AlertTableFixedColumnId = 'title'

export function fieldColumnId(field: string): `field:${string}` {
  return `field:${field}`
}

export function alertTableColumnLabel(id: AlertTableFixedColumnId): string {
  return ALERT_TABLE_FIXED_COLUMNS.find((column) => column.id === id)?.label ?? id
}

/** Columns currently visible in the queue table (searchable), in header order. */
export function alertTableSearchColumns(
  selectFields: string[],
  options: { showCategory?: boolean } = {},
): AlertTableColumn[] {
  const [severity, time, title, category, source] = ALERT_TABLE_FIXED_COLUMNS
  return [
    severity,
    time,
    title,
    ...(options.showCategory ? [category] : []),
    ...selectFields.map((field) => ({ id: fieldColumnId(field), label: field })),
    source,
  ]
}

export function resolveAlertTableSearchColumn(
  column: string,
  columns: AlertTableColumn[],
): AlertTableColumnId {
  if (columns.some((item) => item.id === column)) return column as AlertTableColumnId
  return DEFAULT_ALERT_TABLE_SEARCH_COLUMN
}
