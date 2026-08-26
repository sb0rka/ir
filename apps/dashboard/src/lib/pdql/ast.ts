import type { ActiveSection, AggregateFn, EventFieldDef, ParseResult, QueryAst } from './model'
import {
  defaultOpForType,
  defaultQuery,
  emptyQuery,
  groupCountColumn,
  isGroupCountColumn,
  newId,
} from './model'
import { parse } from './parse'
import { serialize } from './serialize'

function fieldType(fields: EventFieldDef[], name: string) {
  return fields.find((field) => field.name === name)?.type ?? 'string'
}

export function applyGroupInvariant(query: QueryAst): QueryAst {
  const groupFields = new Set(query.groups.map((group) => group.field))
  if (groupFields.size === 0) {
    const hasTime = query.columns.some((column) => column.field === 'time')
    return {
      ...query,
      columns: query.columns
        .filter((column) => !(isGroupCountColumn(column) && hasTime))
        .map((column) => {
          if (isGroupCountColumn(column)) {
            return { ...column, field: 'time', aggregate: undefined }
          }
          return { ...column, aggregate: undefined }
        }),
    }
  }

  let columns = query.columns.filter((column) => !groupFields.has(column.field) || Boolean(column.aggregate))
  if (!columns.some(isGroupCountColumn)) {
    columns = [{ id: newId('col'), field: '', aggregate: 'count' as const }, ...columns]
  }
  return {
    ...query,
    columns: columns.map((column) => {
      if (column.aggregate || column.field === 'time') return column
      return { ...column, aggregate: 'count' as const }
    }),
  }
}

export function setGroupAggregate(query: QueryAst, aggregate: AggregateFn): QueryAst {
  if (query.groups.length === 0) return query
  const existing = groupCountColumn(query)
  if (!existing) {
    return {
      ...query,
      columns: [
        { id: newId('col'), field: '', aggregate, sort: { dir: 'desc', priority: 1 } },
        ...query.columns,
      ],
    }
  }
  return {
    ...query,
    columns: query.columns.map((column) => (column.id === existing.id ? { ...column, aggregate } : column)),
  }
}

export function addColumn(query: QueryAst, field: string): QueryAst {
  if (query.columns.some((column) => column.field === field && !column.aggregate)) return query
  const groupFields = new Set(query.groups.map((group) => group.field))
  const aggregate = query.groups.length > 0 && !groupFields.has(field) ? ('count' as const) : undefined
  return { ...query, columns: [...query.columns, { id: newId('col'), field, aggregate }] }
}

export function addGroup(query: QueryAst, field: string): QueryAst {
  if (query.groups.some((group) => group.field === field)) return query
  return applyGroupInvariant({
    ...query,
    groups: [...query.groups, { id: newId('grp'), field }],
  })
}

export function removeGroup(query: QueryAst, id: string): QueryAst {
  const removed = query.groups.find((group) => group.id === id)
  if (!removed) return query
  const groups = query.groups.filter((group) => group.id !== id)
  const hasColumn = query.columns.some((column) => column.field === removed.field && !column.aggregate)
  const columns = hasColumn
    ? query.columns
    : [{ id: newId('col'), field: removed.field }, ...query.columns]
  return applyGroupInvariant({ ...query, groups, columns })
}

export function addFilter(query: QueryAst, field: string, fields: EventFieldDef[] = []): QueryAst {
  const joiners = query.filter.length === 0 ? query.joiners : [...query.joiners, 'and' as const]
  return {
    ...query,
    filter: [
      ...query.filter,
      {
        id: newId('cond'),
        field,
        op: defaultOpForType(fieldType(fields, field)),
        value: '',
        values: [],
        negated: false,
      },
    ],
    joiners,
  }
}

export function addFieldToAst(
  query: QueryAst,
  name: string,
  section: ActiveSection,
  fields: EventFieldDef[] = [],
): QueryAst {
  if (section === 'filter') return addFilter(query, name, fields)
  if (section === 'columns') return addColumn(query, name)
  return addGroup(query, name)
}

export function parseQueuePdql(pdql: string): ParseResult {
  const trimmed = pdql.trim()
  if (!trimmed) return { ok: true, ast: defaultQuery() }
  return parse(trimmed)
}

export function addFieldToPdql(
  pdql: string,
  name: string,
  section: ActiveSection,
  fields: EventFieldDef[] = [],
): string {
  const parsed = parseQueuePdql(pdql)
  const ast = parsed.ok ? parsed.ast : emptyQuery()
  return serialize(addFieldToAst(ast, name, section, fields))
}
