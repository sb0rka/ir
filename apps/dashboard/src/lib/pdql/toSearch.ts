import type { FilterChip } from '../../types'
import type { Condition, QueryAst } from './model'
import { formatCondition } from './serialize'

export type SearchEntityType = 'host' | 'user' | 'process' | 'ip'

export interface PdqlSearchEntity {
  type: SearchEntityType
  value: string
}

export interface PdqlSearchParts {
  entities: PdqlSearchEntity[]
  query: string
}

const ENTITY_FIELDS: Record<string, SearchEntityType> = {
  'event_src.host': 'host',
  'src.host': 'host',
  'dst.host': 'host',
  'src.ip': 'ip',
  'dst.ip': 'ip',
  'event_src.ip': 'ip',
  'subject.account.name': 'user',
  'object.account.name': 'user',
  'object.process.name': 'process',
  'subject.process.name': 'process',
}

function quote(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

function isNumericLiteral(value: string): boolean {
  return /^-?\d+(\.\d+)?$/.test(value)
}

function formatValue(value: string): string {
  return isNumericLiteral(value) ? value : quote(value)
}

function formatPredicate(condition: Condition): string {
  const prefix = condition.negated ? 'not ' : ''
  switch (condition.op) {
    case 'is_null':
      return `${prefix}${condition.field} is null`
    case 'is_not_null':
      return `${prefix}${condition.field} is not null`
    case 'in':
      return `${prefix}${condition.field} in (${condition.values.map(formatValue).join(', ')})`
    default:
      return `${prefix}${condition.field} ${condition.op} ${formatValue(condition.value)}`
  }
}

function isMappedEntity(condition: Condition): boolean {
  if (condition.negated) return false
  if (condition.op !== '=' && condition.op !== 'in') return false
  return condition.field in ENTITY_FIELDS
}

function conditionValues(condition: Condition): string[] {
  if (condition.op === 'in') return condition.values.filter(Boolean)
  return condition.value ? [condition.value] : []
}

export function pdqlToSearchParts(ast: QueryAst): PdqlSearchParts {
  const entities: PdqlSearchEntity[] = []
  const queryBits: string[] = []

  ast.filter.forEach((condition, index) => {
    if (isMappedEntity(condition)) {
      const type = ENTITY_FIELDS[condition.field]
      for (const value of conditionValues(condition)) {
        entities.push({ type, value })
      }
      return
    }
    const predicate = formatPredicate(condition)
    if (queryBits.length === 0) {
      queryBits.push(predicate)
      return
    }
    const joiner = ast.joiners[index - 1] ?? 'and'
    queryBits.push(`${joiner} ${predicate}`)
  })

  return { entities, query: queryBits.join(' ') }
}

export function astToFilterChips(ast: QueryAst): FilterChip[] {
  const grouped = new Map<PdqlSearchEntity['type'], string[]>()
  for (const entity of pdqlToSearchParts(ast).entities) {
    const values = grouped.get(entity.type) ?? []
    if (!values.includes(entity.value)) values.push(entity.value)
    grouped.set(entity.type, values)
  }
  return [...grouped.entries()].map(([field, values]) => ({
    id: `pdql-${field}`,
    field,
    values,
  }))
}

export interface EventSearchParts {
  filter?: string
  columns?: string[]
  sort?: { field: string; direction: 'asc' | 'desc' }[]
  group_by?: string[]
  group_values?: (string | null)[]
  hasControls: boolean
}

function uniqueFields(fields: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const field of fields) {
    if (!field || seen.has(field)) continue
    seen.add(field)
    out.push(field)
  }
  return out
}

function isDefaultSort(sort: { field: string; direction: 'asc' | 'desc' }[]): boolean {
  return sort.length === 1 && sort[0]?.field === 'time' && sort[0]?.direction === 'desc'
}

function isDefaultColumns(columns: string[]): boolean {
  return columns.length === 0 || (columns.length === 1 && columns[0] === 'time')
}

export function alignGroupValues(
  ast: QueryAst,
  values: (string | null)[] | undefined,
): (string | null)[] {
  return ast.groups.map((_, index) => {
    const value = values?.[index]
    return value == null || value === '' ? null : value
  })
}

/**
 * Selected MaxPatrol group path. Unselected steps are omitted: JSON null is
 * the source null group ("Нет данных"), not "no group chosen yet".
 */
export function groupPathPrefix(
  ast: QueryAst,
  values: (string | null)[] | undefined,
): { group_by: string[]; group_values: (string | null)[] } | undefined {
  if (ast.groups.length === 0) return undefined
  const group_by: string[] = []
  const group_values: (string | null)[] = []
  for (let index = 0; index < ast.groups.length; index++) {
    const value = values?.[index]
    if (value == null || value === '') break
    group_by.push(ast.groups[index]!.field)
    group_values.push(value)
  }
  if (group_by.length === 0) return undefined
  return { group_by, group_values }
}

export function drillGroupValues(
  ast: QueryAst,
  values: (string | null)[] | undefined,
  field: string,
  value: string,
): (string | null)[] | null {
  const index = ast.groups.findIndex((group) => group.field === field)
  if (index < 0) return null
  return ast.groups.map((_, i) => {
    if (i < index) return values?.[i] ?? null
    if (i === index) return value
    return null
  })
}

export function astToEventSearch(
  ast: QueryAst,
  groupValues?: (string | null)[],
): EventSearchParts {
  const filter = formatCondition(ast).trim()
  const columns = uniqueFields([
    ...ast.groups.map((group) => group.field),
    ...ast.columns.filter((column) => column.field && !column.aggregate).map((column) => column.field),
  ])
  const sort = ast.columns
    .filter((column) => column.sort && column.field && !column.aggregate)
    .slice()
    .sort((left, right) => (left.sort?.priority ?? 0) - (right.sort?.priority ?? 0))
    .map((column) => ({ field: column.field, direction: column.sort!.dir }))

  const parts: EventSearchParts = { hasControls: false }
  if (filter) parts.filter = filter
  if (!isDefaultColumns(columns)) parts.columns = columns
  if (sort.length && !isDefaultSort(sort)) parts.sort = sort
  const path = groupPathPrefix(ast, groupValues)
  if (path) {
    parts.group_by = path.group_by
    parts.group_values = path.group_values
  }
  parts.hasControls = Boolean(parts.filter || parts.columns || parts.sort || parts.group_by)
  return parts
}
