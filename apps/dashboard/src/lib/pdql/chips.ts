import { removeGroup } from './ast'
import { isGroupCountColumn, isGroupDimensionColumn, type QueryAst, type SortDir } from './model'
import { formatConditionLabel, serialize } from './serialize'

export type PdqlChipKind = 'filter' | 'group' | 'column'

export interface PdqlChip {
  id: string
  kind: PdqlChipKind
  label: string
  sort?: SortDir
}

function formatSelectLabel(ast: QueryAst, columnId: string): string {
  const column = ast.columns.find((item) => item.id === columnId)
  if (!column) return ''
  const field = column.aggregate
    ? column.field
      ? `${column.aggregate}(${column.field})`
      : `${column.aggregate}()`
    : column.field
  const sort = column.sort ? ` ${column.sort.dir}` : ''
  return `${field}${sort}`
}

export function pdqlToChips(ast: QueryAst): PdqlChip[] {
  const chips: PdqlChip[] = ast.filter.map((condition) => ({
    id: condition.id,
    kind: 'filter',
    label: formatConditionLabel(condition),
  }))
  for (const group of ast.groups) {
    chips.push({ id: group.id, kind: 'group', label: `group ${group.field}` })
  }
  for (const column of ast.columns) {
    if (isGroupDimensionColumn(ast, column) || isGroupCountColumn(column)) continue
    chips.push({
      id: column.id,
      kind: 'column',
      label: formatSelectLabel(ast, column.id),
      sort: column.sort?.dir,
    })
  }
  return chips
}

export function toggleChipSort(ast: QueryAst, id: string): QueryAst {
  return {
    ...ast,
    columns: ast.columns.map((column) => {
      if (column.id !== id || !column.sort) return column
      return {
        ...column,
        sort: { ...column.sort, dir: column.sort.dir === 'desc' ? 'asc' : 'desc' },
      }
    }),
  }
}

export function removePdqlChip(ast: QueryAst, id: string): QueryAst {
  const filterIndex = ast.filter.findIndex((item) => item.id === id)
  if (filterIndex >= 0) {
    const filter = ast.filter.filter((item) => item.id !== id)
    const joiners = ast.joiners.filter((_, joinerIndex) =>
      filterIndex === 0 ? joinerIndex !== 0 : joinerIndex !== filterIndex - 1,
    )
    return { ...ast, filter, joiners }
  }
  if (ast.groups.some((group) => group.id === id)) {
    return removeGroup(ast, id)
  }
  return { ...ast, columns: ast.columns.filter((column) => column.id !== id) }
}

export function serializeWithoutChip(ast: QueryAst, id: string): string {
  return serialize(removePdqlChip(ast, id))
}

export function serializeToggledChipSort(ast: QueryAst, id: string): string {
  return serialize(toggleChipSort(ast, id))
}
