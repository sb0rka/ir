import type { FilterChip } from '../../types'
import type { Condition, QueryAst } from './model'

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
