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

function eventSearchFields(ast: QueryAst): string[] {
  return uniqueFields([
    ...ast.groups.map((group) => group.field),
    ...ast.columns.filter((column) => column.field && !column.aggregate).map((column) => column.field),
  ])
}

/** Select fields to show as extra queue columns. `time` already has a dedicated column. */
export function queueSelectFields(ast: QueryAst): string[] {
  return eventSearchFields(ast).filter((field) => field !== 'time')
}

function isDefaultSort(sort: { field: string; direction: 'asc' | 'desc' }[]): boolean {
  return sort.length === 1 && sort[0]?.field === 'time' && sort[0]?.direction === 'desc'
}

/**
 * Align selected group values to the current PDQL groups.
 * A missing or empty slot means "not chosen yet". JSON null is the source
 * null group ("Нет данных") and must be kept as an explicit selection.
 */
export function alignGroupValues(
  ast: QueryAst,
  values: (string | null)[] | undefined,
): (string | null)[] {
  if (ast.groups.length === 0) return []
  const aligned: (string | null)[] = []
  for (let index = 0; index < ast.groups.length; index++) {
    if (index >= (values?.length ?? 0)) break
    const value = values![index]
    if (value === '') break
    aligned.push(value ?? null)
  }
  return aligned
}

export function hasGroupValueSelection(values: (string | null)[] | undefined): boolean {
  return (values?.length ?? 0) > 0
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
    if (index >= (values?.length ?? 0)) break
    const value = values![index]
    if (value === '') break
    group_by.push(ast.groups[index]!.field)
    group_values.push(value ?? null)
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
  const next = ast.groups.slice(0, index).map((_, i) => values?.[i] ?? null)
  next.push(value)
  return next
}

export interface EventAggregateParts {
  group_by: string[]
  filter?: string
  sort?: { field: string; direction: 'asc' | 'desc' }[]
}

/** First group only: dashboard aggregate filter assumes a single group field. */
export function astToEventAggregate(ast: QueryAst): EventAggregateParts | undefined {
  const field = ast.groups[0]?.field
  if (!field) return undefined
  const filter = formatCondition(ast).trim()
  const parts: EventAggregateParts = { group_by: [field] }
  if (filter) parts.filter = filter

  const countSort = ast.columns.find((column) => column.aggregate && !column.field && column.sort)
  const groupSort = ast.columns.find(
    (column) => column.field === field && column.sort && !column.aggregate,
  )
  const sort: { field: string; direction: 'asc' | 'desc' }[] = []
  if (countSort?.sort) sort.push({ field: 'count', direction: countSort.sort.dir })
  if (groupSort?.sort) sort.push({ field, direction: groupSort.sort.dir })
  if (sort.length) parts.sort = sort
  return parts
}

export function astToEventSearch(
  ast: QueryAst,
  groupValues?: (string | null)[],
): EventSearchParts {
  const filter = formatCondition(ast).trim()
  const sort = ast.columns
    .filter((column) => column.sort && column.field && !column.aggregate)
    .slice()
    .sort((left, right) => (left.sort?.priority ?? 0) - (right.sort?.priority ?? 0))
    .map((column) => ({ field: column.field, direction: column.sort!.dir }))

  const parts: EventSearchParts = { hasControls: false }
  if (filter) parts.filter = filter
  if (sort.length && !isDefaultSort(sort)) parts.sort = sort
  const path = groupPathPrefix(ast, groupValues)
  if (path) {
    parts.group_by = path.group_by
    parts.group_values = path.group_values
  }
  parts.hasControls = Boolean(parts.filter || parts.sort || parts.group_by)
  return parts
}
