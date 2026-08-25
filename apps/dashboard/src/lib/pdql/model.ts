export type FieldType = 'string' | 'number' | 'ip' | 'datetime' | 'enum'
export type LogicalJoiner = 'and' | 'or'
export type CompareOp =
  | '='
  | '!='
  | '>'
  | '<'
  | '>='
  | '<='
  | 'contains'
  | 'startswith'
  | 'in'
  | 'is_null'
  | 'is_not_null'
export type AggregateFn = 'count' | 'uniq' | 'min' | 'max' | 'avg'
export type SortDir = 'asc' | 'desc'
export type ActiveSection = 'filter' | 'columns' | 'groups'

export interface EventFieldDef {
  name: string
  type: FieldType
  description: string
  enumValues?: string[]
}

export interface Condition {
  id: string
  field: string
  op: CompareOp
  value: string
  values: string[]
  negated: boolean
}

export interface Column {
  id: string
  field: string
  aggregate?: AggregateFn
  sort?: { dir: SortDir; priority: number }
}

export interface Group {
  id: string
  field: string
}

export interface QueryAst {
  filter: Condition[]
  joiners: LogicalJoiner[]
  columns: Column[]
  groups: Group[]
}

export interface ParseError {
  message: string
  position: number
}

export type ParseResult = { ok: true; ast: QueryAst } | { ok: false; error: ParseError }

let nextId = 0

export function newId(prefix: string): string {
  nextId += 1
  return `${prefix}-${nextId}`
}

export function defaultQuery(): QueryAst {
  return {
    filter: [],
    joiners: [],
    columns: [{ id: newId('col'), field: 'time', sort: { dir: 'desc', priority: 1 } }],
    groups: [],
  }
}

export function emptyQuery(): QueryAst {
  return { filter: [], joiners: [], columns: [], groups: [] }
}

export function defaultOpForType(_type: FieldType): CompareOp {
  return '='
}

export function operatorsForType(type: FieldType): CompareOp[] {
  switch (type) {
    case 'string':
      return ['=', '!=', 'contains', 'startswith', 'in', 'is_null', 'is_not_null']
    case 'enum':
    case 'ip':
      return ['=', '!=', 'in', 'is_null', 'is_not_null']
    case 'number':
    case 'datetime':
      return ['=', '!=', '>', '<', '>=', '<=', 'is_null', 'is_not_null']
  }
}

export function fieldPrefix(name: string): string {
  const dot = name.indexOf('.')
  return dot === -1 ? 'общее' : name.slice(0, dot)
}

export function withoutIds(ast: QueryAst): unknown {
  return {
    filter: ast.filter.map(({ field, op, value, values, negated }) => ({
      field,
      op,
      value,
      values,
      negated,
    })),
    joiners: ast.joiners,
    columns: ast.columns.map(({ field, aggregate, sort }) => ({ field, aggregate, sort })),
    groups: ast.groups.map(({ field }) => ({ field })),
  }
}

export const AGGREGATES: AggregateFn[] = ['count', 'uniq', 'min', 'max', 'avg']
