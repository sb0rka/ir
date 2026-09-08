/** Columns of the investigations list table. Searchable subset drives the text filter. */
export type InvestigationTableColumnId =
  | 'severity'
  | 'status'
  | 'nodes'
  | 'hypotheses'
  | 'agents'
  | 'title'
  | 'createdAt'
  | 'updatedAt'

export type InvestigationTableColumn = {
  id: InvestigationTableColumnId
  label: string
  searchable?: boolean
}

export const INVESTIGATION_TABLE_COLUMNS: ReadonlyArray<InvestigationTableColumn> = [
  { id: 'severity', label: 'Крит.', searchable: true },
  { id: 'status', label: 'Статус', searchable: true },
  { id: 'title', label: 'Название', searchable: true },
  { id: 'nodes', label: 'Ноды' },
  { id: 'hypotheses', label: 'Гипотезы' },
  { id: 'agents', label: 'Агенты' },
  { id: 'createdAt', label: 'Создано', searchable: true },
  { id: 'updatedAt', label: 'Обновлено', searchable: true },
]

export const INVESTIGATION_TABLE_SEARCH_COLUMNS: ReadonlyArray<InvestigationTableColumn> =
  INVESTIGATION_TABLE_COLUMNS.filter((column) => column.searchable)

export const DEFAULT_INVESTIGATION_TABLE_SEARCH_COLUMN: InvestigationTableColumnId = 'title'

export function investigationTableColumnLabel(id: InvestigationTableColumnId): string {
  return INVESTIGATION_TABLE_COLUMNS.find((column) => column.id === id)?.label ?? id
}

export function resolveInvestigationTableSearchColumn(
  column: string,
): InvestigationTableColumnId {
  if (INVESTIGATION_TABLE_SEARCH_COLUMNS.some((item) => item.id === column)) {
    return column as InvestigationTableColumnId
  }
  return DEFAULT_INVESTIGATION_TABLE_SEARCH_COLUMN
}
