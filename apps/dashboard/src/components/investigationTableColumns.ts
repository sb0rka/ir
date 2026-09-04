/** Searchable columns of the investigations list table — single source for headers + search. */
export type InvestigationTableColumnId =
  | 'severity'
  | 'status'
  | 'title'
  | 'verdict'
  | 'createdAt'
  | 'updatedAt'

export type InvestigationTableColumn = {
  id: InvestigationTableColumnId
  label: string
}

export const INVESTIGATION_TABLE_SEARCH_COLUMNS: ReadonlyArray<InvestigationTableColumn> = [
  { id: 'severity', label: 'Крит.' },
  { id: 'status', label: 'Статус' },
  { id: 'title', label: 'Название' },
  { id: 'verdict', label: 'Вердикт' },
  { id: 'createdAt', label: 'Создано' },
  { id: 'updatedAt', label: 'Обновлено' },
]

export const DEFAULT_INVESTIGATION_TABLE_SEARCH_COLUMN: InvestigationTableColumnId = 'title'

export function investigationTableColumnLabel(id: InvestigationTableColumnId): string {
  return INVESTIGATION_TABLE_SEARCH_COLUMNS.find((column) => column.id === id)?.label ?? id
}

export function resolveInvestigationTableSearchColumn(
  column: string,
): InvestigationTableColumnId {
  if (INVESTIGATION_TABLE_SEARCH_COLUMNS.some((item) => item.id === column)) {
    return column as InvestigationTableColumnId
  }
  return DEFAULT_INVESTIGATION_TABLE_SEARCH_COLUMN
}
