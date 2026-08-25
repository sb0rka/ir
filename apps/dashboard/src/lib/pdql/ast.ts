import type { ActiveSection, EventFieldDef, ParseResult, QueryAst } from './model'
import { defaultOpForType, defaultQuery, emptyQuery, newId } from './model'
import { parse } from './parse'
import { serialize } from './serialize'

function fieldType(fields: EventFieldDef[], name: string) {
  return fields.find((field) => field.name === name)?.type ?? 'string'
}

export function applyGroupInvariant(query: QueryAst): QueryAst {
  const groupFields = new Set(query.groups.map((group) => group.field))
  if (groupFields.size === 0) {
    return {
      ...query,
      columns: query.columns.map((column) => ({ ...column, aggregate: undefined })),
    }
  }
  return {
    ...query,
    columns: query.columns.map((column) =>
      groupFields.has(column.field)
        ? { ...column, aggregate: undefined }
        : { ...column, aggregate: column.aggregate ?? 'count' },
    ),
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
  const withGroup = { ...query, groups: [...query.groups, { id: newId('grp'), field }] }
  const withColumn = query.columns.some((column) => column.field === field)
    ? withGroup
    : { ...withGroup, columns: [{ id: newId('col'), field }, ...withGroup.columns] }
  return applyGroupInvariant(withColumn)
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
