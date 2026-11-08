import type { components as Gw } from '@ir/contract/gateway'
import type { AppliedEventLanguage, EventQuery } from '../types'
import { formatDql, parseDql } from './dql/dql'
import { appendConditions } from './pdql/append'
import { addFieldToAst, parseQueuePdql } from './pdql/ast'
import { pdqlToChips, removePdqlChip, toggleChipSort, type PdqlChip } from './pdql/chips'
import { filterListOf, pruneFilterBy, withFilterList } from './pdql/filterTree'
import {
  clampQueueLimit,
  DEFAULT_QUEUE_LIMIT,
  defaultQuery,
  effectiveQueueLimit,
  isFilterGroup,
  newId,
  withoutIds,
  type ActiveSection,
  type CompareOp,
  type Condition,
  type EventFieldDef,
  type FilterNode,
  type LogicalJoiner,
  type QueryAst,
} from './pdql/model'
import { eventFieldLabelRu } from './pdql/fieldLabelsRu'
import { formatCondition, serialize } from './pdql/serialize'
import { groupPathPrefix, queueSelectFields, timeIntervalFromAst } from './pdql/toSearch'
import type { TimeInterval } from '../components/time-interval/model'

export type { EventQuery }
export type SentEventLanguage = 'pt_pdql' | 'wazuh_dql'

export interface EventSourceMeta {
  source_code: string
  source_name: string
  languages: string[]
  bounded: boolean
  common_mappings?: Gw['schemas']['CommonFieldMappings']
  fields: Gw['schemas']['EventFieldMetadata'][]
}

export interface DescribedEventQuery {
  hosts: string[]
  ips: string[]
  language: SentEventLanguage | null
  filter?: string
  columns?: string[]
  sort?: { field: string; direction: 'asc' | 'desc' }[]
  group_by?: string[]
  group_values?: (string | null)[]
  aggregateGroupBy?: string[]
  limit: number
  /** Compatible source codes when a language is sent. Undefined means every event source. */
  sources?: string[]
  scopeNames: string[]
  preview: string
  nativeAst: QueryAst | null
  error?: string
}

const PDQL_TIME_FIELD = 'time'
const WAZUH_TIME_FIELD = 'timestamp'

export function emptyEventQuery(): EventQuery {
  return {
    hosts: [],
    ips: [],
    pdql: serialize(defaultQuery()),
    wazuh: '',
    appliedLanguage: 'common',
  }
}

export function defaultWazuhQuery(): QueryAst {
  return {
    filter: [],
    joiners: [],
    columns: [{ id: newId('col'), field: WAZUH_TIME_FIELD, sort: { dir: 'desc', priority: 1 } }],
    groups: [],
    limit: DEFAULT_QUEUE_LIMIT,
  }
}

export function encodeAst(ast: QueryAst): string {
  return JSON.stringify(withoutIds(ast))
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value != null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function decodeNode(value: unknown): FilterNode | null {
  const record = asRecord(value)
  if (!record) return null
  if (record.kind === 'group') {
    const children = Array.isArray(record.children)
      ? record.children.map(decodeNode).filter((node): node is FilterNode => node != null)
      : []
    const joiners = Array.isArray(record.joiners)
      ? record.joiners.filter((item): item is LogicalJoiner => item === 'and' || item === 'or')
      : []
    return { kind: 'group', id: newId('grp'), negated: record.negated === true, children, joiners }
  }
  if (typeof record.field !== 'string') return null
  return {
    id: newId('cond'),
    field: record.field,
    op: (typeof record.op === 'string' ? record.op : '=') as CompareOp,
    value: typeof record.value === 'string' ? record.value : '',
    values: Array.isArray(record.values) ? record.values.filter((item): item is string => typeof item === 'string') : [],
    negated: record.negated === true,
  }
}

export function decodeAst(raw: string): QueryAst | null {
  try {
    const parsed = JSON.parse(raw) as unknown
    const record = asRecord(parsed)
    if (!record) return null
    const filter = Array.isArray(record.filter)
      ? record.filter.map(decodeNode).filter((node): node is FilterNode => node != null)
      : []
    const joiners = Array.isArray(record.joiners)
      ? record.joiners.filter((item): item is LogicalJoiner => item === 'and' || item === 'or')
      : []
    const columns = Array.isArray(record.columns)
      ? record.columns.flatMap((item) => {
          const column = asRecord(item)
          if (!column || typeof column.field !== 'string') return []
          const sort = asRecord(column.sort)
          const rawDir = sort?.dir
          const dir: 'asc' | 'desc' | undefined = rawDir === 'asc' || rawDir === 'desc' ? rawDir : undefined
          return [{
            id: newId('col'),
            field: column.field,
            ...(typeof column.aggregate === 'string'
              ? { aggregate: column.aggregate as QueryAst['columns'][number]['aggregate'] }
              : {}),
            ...(dir
              ? { sort: { dir, priority: typeof sort?.priority === 'number' ? sort.priority : 1 } }
              : {}),
          }]
        })
      : []
    const groups = Array.isArray(record.groups)
      ? record.groups.flatMap((item) => {
          const group = asRecord(item)
          if (!group || typeof group.field !== 'string') return []
          return [{ id: newId('grp'), field: group.field }]
        })
      : []
    return {
      filter,
      joiners,
      columns,
      groups,
      ...(typeof record.limit === 'number' ? { limit: record.limit } : {}),
    }
  } catch {
    return null
  }
}

export function readWazuhAst(raw: string): { ast: QueryAst; error?: string } {
  if (!raw.trim()) return { ast: defaultWazuhQuery() }
  if (raw.trim().startsWith('{')) {
    const ast = decodeAst(raw)
    return ast ? { ast } : { ast: defaultWazuhQuery(), error: 'Не удалось прочитать запрос Wazuh' }
  }
  const parsed = parseDql(raw)
  if (!parsed.ok) return { ast: defaultWazuhQuery(), error: parsed.error.message }
  return { ast: { ...defaultWazuhQuery(), filter: parsed.filter, joiners: parsed.joiners } }
}

export function readPdqlAst(raw: string): { ast: QueryAst; error?: string } {
  const parsed = parseQueuePdql(raw)
  if (!parsed.ok) return { ast: defaultQuery(), error: parsed.error.message }
  return { ast: parsed.ast }
}

function predicate(ast: QueryAst): string {
  return formatCondition(ast).trim()
}

function isDefaultSort(sort: { field: string; direction: 'asc' | 'desc' }[], field: string): boolean {
  return sort.length === 1 && sort[0]?.field === field && sort[0]?.direction === 'desc'
}

function columnSort(ast: QueryAst): { field: string; direction: 'asc' | 'desc' }[] {
  return ast.columns
    .filter((column) => column.sort && column.field && !column.aggregate)
    .slice()
    .sort((left, right) => (left.sort?.priority ?? 0) - (right.sort?.priority ?? 0))
    .map((column) => ({ field: column.field, direction: column.sort!.dir }))
}

function hasNativeControls(ast: QueryAst, timeField: string): boolean {
  if (ast.groups.length > 0) return true
  const sort = columnSort(ast)
  if (sort.length > 0 && !isDefaultSort(sort, timeField)) return true
  return queueSelectFields(ast).some((field) => field !== timeField && field !== WAZUH_TIME_FIELD)
}

function languageSources(sources: EventSourceMeta[], language: SentEventLanguage): EventSourceMeta[] {
  return sources.filter((source) => source.languages.includes(language))
}

function speaks(sources: EventSourceMeta[], language: string): boolean {
  return sources.some((source) => source.languages.includes(language))
}

function limitClause(limit: number): string {
  return `limit(${clampQueueLimit(limit)})`
}

/** DQL has no limit syntax. The queue line still ends with the same `limit(N)` stage as PDQL. */
function withLimitClause(filter: string, limit: number): string {
  const clause = limitClause(limit)
  return filter ? `${filter} | ${clause}` : clause
}

/**
 * Split a queue line into a DQL predicate and an optional trailing `limit(N)`.
 * A bare `limit(N)` is the page size with no predicate.
 */
export function readLimitClause(text: string): { predicate: string; limit?: number; error?: string } {
  const trimmed = text.trim()
  const only = /^limit\(([^)]*)\)\s*$/i.exec(trimmed)
  const tail = /^(.*?)\s*\|\s*limit\(([^)]*)\)\s*$/i.exec(trimmed)
  const raw = (only?.[1] ?? tail?.[2])?.trim()
  const predicate = only ? '' : (tail ? tail[1].trim() : trimmed)
  if (raw == null) return { predicate }
  if (!/^\d+$/.test(raw) || Number(raw) < 1) {
    return { predicate, error: 'limit должен быть целым числом не меньше 1' }
  }
  return { predicate, limit: clampQueueLimit(Number(raw)) }
}

/**
 * Text shown on the event-query line.
 * PDQL keeps `select | sort | limit` in one string. Without a PDQL source the
 * same string is the DQL predicate plus `limit(N)`, edited by the same pencil.
 * An empty predicate is just `limit(N)`; the search stays a common time range.
 */
export function eventQueryLine(
  query: EventQuery,
  sources: EventSourceMeta[] = [],
  groupValues?: (string | null)[],
): string {
  const described = describeEventQuery(query, sources, groupValues)
  const pdqlPipeline =
    (speaks(sources, 'pt_pdql') || described.language === 'pt_pdql') && described.language !== 'wazuh_dql'
  if (pdqlPipeline) return described.preview
  if (sources.length === 0 && described.language !== 'wazuh_dql') return ''
  const filter = described.language === 'wazuh_dql' ? (described.filter ?? '') : ''
  return withLimitClause(filter, described.limit)
}

export function describeEventQuery(
  query: EventQuery,
  sources: EventSourceMeta[] = [],
  groupValues?: (string | null)[],
): DescribedEventQuery {
  const hosts = query.hosts.map((item) => item.trim()).filter(Boolean)
  const ips = query.ips.map((item) => item.trim()).filter(Boolean)
  const base: DescribedEventQuery = {
    hosts,
    ips,
    language: null,
    limit: DEFAULT_QUEUE_LIMIT,
    scopeNames: sources.map((source) => source.source_name),
    preview: serialize(defaultQuery()),
    nativeAst: null,
  }
  if (query.appliedLanguage === 'common') {
    const read = readPdqlAst(query.pdql)
    // The line keeps select/sort/limit. An unapplied PDQL filter stays a draft.
    if (!read.error && !predicate(read.ast) && !hasNativeControls(read.ast, PDQL_TIME_FIELD)) {
      return {
        ...base,
        limit: effectiveQueueLimit(read.ast),
        preview: serialize(read.ast),
      }
    }
    return base
  }

  const language = query.appliedLanguage
  const read = language === 'pt_pdql' ? readPdqlAst(query.pdql) : readWazuhAst(query.wazuh)
  const timeField = language === 'pt_pdql' ? PDQL_TIME_FIELD : WAZUH_TIME_FIELD
  const compatible = languageSources(sources, language)
  const names = compatible.map((source) => source.source_name)
  if (read.error) {
    return { ...base, error: read.error, scopeNames: names, nativeAst: read.ast }
  }
  const filterText = language === 'wazuh_dql' ? formatDql(read.ast.filter, read.ast.joiners) : predicate(read.ast)
  if (language === 'wazuh_dql' && filterText == null) {
    return {
      ...base,
      error: 'Условие Wazuh нельзя записать в DQL без смены смысла',
      scopeNames: names,
      nativeAst: read.ast,
    }
  }
  const filter = filterText?.trim() ?? ''
  const active = Boolean(filter) || hasNativeControls(read.ast, timeField)
  // An empty predicate stays a common search. The page size still follows the
  // stored limit, so raising it does not require a native filter.
  if (!active) return { ...base, limit: effectiveQueueLimit(read.ast) }

  const sort = columnSort(read.ast)
  const columns = queueSelectFields(read.ast).filter((field) => field !== timeField && field !== WAZUH_TIME_FIELD)
  const path = groupPathPrefix(read.ast, groupValues)
  const described: DescribedEventQuery = {
    ...base,
    language,
    limit: effectiveQueueLimit(read.ast),
    sources: compatible.map((source) => source.source_code),
    scopeNames: names,
    preview: language === 'pt_pdql' ? serialize(read.ast) : filter,
    nativeAst: read.ast,
  }
  if (filter) described.filter = filter
  if (columns.length) described.columns = columns
  if (sort.length && !isDefaultSort(sort, timeField)) described.sort = sort
  if (path) {
    described.group_by = path.group_by
    described.group_values = path.group_values
  }
  if (read.ast.groups.length > 0) described.aggregateGroupBy = [read.ast.groups[0]!.field]
  return described
}

/** Host, IP, or a native predicate the entity queue can send as an event search. */
export function eventQueryHasEntityScope(query: EventQuery, sources: EventSourceMeta[] = []): boolean {
  const described = describeEventQuery(query, sources)
  return Boolean(described.filter) || described.hosts.length > 0 || described.ips.length > 0
}

export function eventQueryFingerprint(
  query: EventQuery,
  interval: TimeInterval,
  groupValues: (string | null)[] = [],
  sources: EventSourceMeta[] = [],
): string {
  const described = describeEventQuery(query, sources, groupValues)
  return JSON.stringify({
    hosts: described.hosts,
    ips: described.ips,
    language: described.language,
    filter: described.filter ?? '',
    columns: described.columns ?? [],
    sort: described.sort ?? [],
    group_by: described.group_by ?? [],
    group_values: described.group_values ?? [],
    limit: described.limit,
    interval,
    groupValues,
  })
}

/** Parse assigns fresh node ids, so chips address a condition by its place in the text. */
function stableNativeChips(ast: QueryAst, language: SentEventLanguage): PdqlChip[] {
  const seen = { filter: 0, group: 0, column: 0 }
  return pdqlToChips(ast).map((chip) => {
    const index = seen[chip.kind]
    seen[chip.kind] += 1
    let label = chip.label
    if (chip.kind === 'filter' && language === 'wazuh_dql') {
      const condition = findCondition(ast, chip.id)
      const formatted = condition ? formatDql([condition], []) : null
      if (formatted) label = formatted
    }
    return { ...chip, id: `native:${chip.kind}:${index}`, label }
  })
}

function nativeNodeId(ast: QueryAst, id: string): string | null {
  const match = /^native:(filter|group|column):(\d+)$/.exec(id)
  if (!match) return null
  const kind = match[1] as PdqlChip['kind']
  const index = Number(match[2])
  return pdqlToChips(ast).filter((chip) => chip.kind === kind)[index]?.id ?? null
}

/** Pipeline select/sort stays on the line after the last predicate is removed. */
function pipelineAst(query: EventQuery): QueryAst | null {
  if (query.appliedLanguage !== 'common') return null
  const read = readPdqlAst(query.pdql)
  if (read.error || predicate(read.ast) || hasNativeControls(read.ast, PDQL_TIME_FIELD)) return null
  return read.ast
}

function pipelineChips(ast: QueryAst): PdqlChip[] {
  return stableNativeChips(ast, 'pt_pdql').filter((chip) => chip.kind !== 'filter')
}

export function eventQueryChips(query: EventQuery, sources: EventSourceMeta[] = []): PdqlChip[] {
  const described = describeEventQuery(query, sources)
  const chips: PdqlChip[] = []
  described.hosts.forEach((value, index) => {
    chips.push({ id: `common:host:${index}`, kind: 'filter', field: 'host', label: `host = "${value}"` })
  })
  described.ips.forEach((value, index) => {
    chips.push({ id: `common:ip:${index}`, kind: 'filter', field: 'ip', label: `ip = "${value}"` })
  })
  if (described.nativeAst && described.language) {
    return [...chips, ...stableNativeChips(described.nativeAst, described.language)]
  }
  const pipeline = pipelineAst(query)
  return pipeline && speaks(sources, 'pt_pdql') ? [...chips, ...pipelineChips(pipeline)] : chips
}

function findCondition(ast: QueryAst, id: string): Condition | null {
  const walk = (nodes: FilterNode[]): Condition | null => {
    for (const node of nodes) {
      if (isFilterGroup(node)) {
        const found = walk(node.children)
        if (found) return found
        continue
      }
      if (node.id === id) return node
    }
    return null
  }
  return walk(ast.filter)
}

function editPipelineChip(query: EventQuery, id: string, edit: (ast: QueryAst, nodeId: string) => QueryAst): EventQuery | null {
  const pipeline = pipelineAst(query)
  if (!pipeline) return null
  const nodeId = nativeNodeId(pipeline, id)
  if (!nodeId) return null
  const kind = pdqlToChips(pipeline).find((chip) => chip.id === nodeId)?.kind
  if (!kind || kind === 'filter') return null
  return replacePdqlText(query, serialize(edit(pipeline, nodeId))).query
}

export function removeEventQueryChip(query: EventQuery, id: string, sources: EventSourceMeta[] = []): EventQuery {
  if (id.startsWith('common:host:')) {
    const index = Number(id.slice('common:host:'.length))
    return { ...query, hosts: query.hosts.filter((_, item) => item !== index) }
  }
  if (id.startsWith('common:ip:')) {
    const index = Number(id.slice('common:ip:'.length))
    return { ...query, ips: query.ips.filter((_, item) => item !== index) }
  }
  const described = describeEventQuery(query, sources)
  if (!described.nativeAst || !described.language) {
    return editPipelineChip(query, id, removePdqlChip) ?? query
  }
  const nodeId = nativeNodeId(described.nativeAst, id)
  if (!nodeId) return query
  const next = removePdqlChip(described.nativeAst, nodeId)
  if (described.language === 'pt_pdql') return replacePdqlText(query, serialize(next)).query
  const filter = formatDql(next.filter, next.joiners)
  const active = Boolean(filter?.trim()) || hasNativeControls(next, WAZUH_TIME_FIELD)
  return { ...query, wazuh: encodeAst(next), appliedLanguage: active ? 'wazuh_dql' : 'common' }
}

export function toggleEventQuerySort(query: EventQuery, id: string, sources: EventSourceMeta[] = []): EventQuery {
  const described = describeEventQuery(query, sources)
  if (!described.nativeAst || !described.language) {
    return editPipelineChip(query, id, toggleChipSort) ?? query
  }
  const nodeId = nativeNodeId(described.nativeAst, id)
  if (!nodeId) return query
  const next = toggleChipSort(described.nativeAst, nodeId)
  if (described.language === 'pt_pdql') return replacePdqlText(query, serialize(next)).query
  return { ...query, wazuh: encodeAst(next) }
}

function namesOf(fields: readonly string[]): string[] {
  const names: string[] = []
  for (const field of fields) {
    const name = field.trim()
    if (name && !names.includes(name)) names.push(name)
  }
  return names
}

function catalogNames(sources: EventSourceMeta[], language: SentEventLanguage): Set<string> {
  return new Set(
    sources
      .filter((source) => source.languages.includes(language))
      .flatMap((source) => source.fields.map((field) => field.name)),
  )
}

/** A condition belongs to the language that can actually compile its fields. */
export function conditionLanguage(
  fields: readonly string[],
  sources: EventSourceMeta[],
  eventSource?: string,
): SentEventLanguage {
  const names = namesOf(fields)
  const wazuh = catalogNames(sources, 'wazuh_dql')
  const pdql = catalogNames(sources, 'pt_pdql')
  if (names.length > 0 && (wazuh.size > 0 || pdql.size > 0)) {
    if (names.every((name) => wazuh.has(name) && !pdql.has(name))) return 'wazuh_dql'
    if (names.every((name) => pdql.has(name) && !wazuh.has(name))) return 'pt_pdql'
  }
  const named = eventSource ? sources.find((source) => source.source_code === eventSource) : undefined
  if (named?.languages.includes('wazuh_dql') && !named.languages.includes('pt_pdql')) return 'wazuh_dql'
  if (named?.languages.includes('pt_pdql')) return 'pt_pdql'
  if (eventSource === 'wazuh') return 'wazuh_dql'
  return 'pt_pdql'
}

/** Page size shared by the common pipeline and a Wazuh predicate. */
function pipelineLimit(query: EventQuery): number {
  if (query.appliedLanguage === 'wazuh_dql' && query.wazuh.trim()) {
    return effectiveQueueLimit(readWazuhAst(query.wazuh).ast)
  }
  return effectiveQueueLimit(readPdqlAst(query.pdql).ast)
}

function withStoredLimit(ast: QueryAst, limit: number): QueryAst {
  return { ...ast, limit }
}

/**
 * How many rows the next search loads. DQL has no limit clause, so the number
 * is stored beside the predicate and sent as the search `limit` (1–1000).
 */
export function setEventQueryLimit(query: EventQuery, raw: number): EventQuery {
  const limit = clampQueueLimit(raw)
  const pdql = serialize(withStoredLimit(readPdqlAst(query.pdql).ast, limit))
  if (query.appliedLanguage !== 'wazuh_dql' && !query.wazuh.trim()) return { ...query, pdql }
  return {
    ...query,
    pdql,
    wazuh: encodeAst(withStoredLimit(readWazuhAst(query.wazuh).ast, limit)),
  }
}

function pdqlBase(query: EventQuery): string {
  if (query.appliedLanguage === 'pt_pdql') return query.pdql
  const parsed = parseQueuePdql(query.pdql)
  if (parsed.ok && parsed.ast.filter.length === 0) return serialize(parsed.ast)
  return serialize(defaultQuery())
}

/** Card and table clicks land in the language that owns the field. One request keeps one language. */
export function appendEventQueryCondition(
  query: EventQuery | undefined,
  fields: readonly string[],
  op: CompareOp,
  value: string,
  joiner: LogicalJoiner,
  sources: EventSourceMeta[] = [],
  eventSource?: string,
): EventQuery {
  const current = query ?? emptyEventQuery()
  if (conditionLanguage(fields, sources, eventSource) === 'wazuh_dql') {
    const base = current.appliedLanguage === 'wazuh_dql' ? readWazuhAst(current.wazuh).ast : defaultWazuhQuery()
    const limited = withStoredLimit(base, pipelineLimit(current))
    const parsed = parseQueuePdql(appendConditions(serialize(limited), fields, op, value, joiner))
    const next = parsed.ok ? parsed.ast : limited
    const limit = effectiveQueueLimit(next)
    return {
      ...current,
      pdql: serialize(withStoredLimit(readPdqlAst(current.pdql).ast, limit)),
      wazuh: encodeAst(withStoredLimit(next, limit)),
      appliedLanguage: 'wazuh_dql',
    }
  }
  return {
    ...current,
    pdql: appendConditions(pdqlBase(current), fields, op, value, joiner),
    appliedLanguage: 'pt_pdql',
  }
}

function conditionValues(condition: Condition): string[] {
  if (condition.op === 'in') return condition.values.map((item) => item.trim()).filter(Boolean)
  return condition.value.trim() ? [condition.value.trim()] : []
}

/** Old history without a language opens as PDQL when the predicate is non-empty. */
export function eventQueryFromLegacy(pdql: string): EventQuery {
  const read = readPdqlAst(pdql)
  const hosts: string[] = []
  const ips: string[] = []
  const pruned = pruneFilterBy(filterListOf(read.ast), (condition) => {
    if (condition.negated || (condition.op !== '=' && condition.op !== 'in')) return false
    if (condition.field !== 'host' && condition.field !== 'ip') return false
    const values = conditionValues(condition)
    if (condition.field === 'host') hosts.push(...values)
    else ips.push(...values)
    return values.length > 0
  })
  const ast = withFilterList(read.ast, pruned)
  const filter = predicate(ast)
  const native = Boolean(filter) || hasNativeControls(ast, PDQL_TIME_FIELD)
  return {
    hosts,
    ips,
    pdql: serialize(ast),
    wazuh: '',
    appliedLanguage: native ? 'pt_pdql' : 'common',
  }
}

export function eventTimeInterval(
  query: EventQuery,
  interval: TimeInterval,
  sources: EventSourceMeta[] = [],
): TimeInterval {
  const described = describeEventQuery(query, sources)
  if (described.language !== 'pt_pdql' || !described.nativeAst) return interval
  return timeIntervalFromAst(described.nativeAst, interval).interval
}

export function addEventQueryField(
  query: EventQuery,
  name: string,
  section: ActiveSection,
  fields: EventFieldDef[] = [],
): EventQuery {
  if (query.appliedLanguage === 'common') {
    if (name === 'host') return { ...query, hosts: [...query.hosts, ''] }
    if (name === 'ip') return { ...query, ips: [...query.ips, ''] }
    return query
  }
  if (query.appliedLanguage === 'pt_pdql') {
    const read = readPdqlAst(query.pdql)
    return { ...query, pdql: serialize(addFieldToAst(read.ast, name, section, fields)) }
  }
  const read = readWazuhAst(query.wazuh)
  const ast = withStoredLimit(read.ast, pipelineLimit(query))
  return { ...query, wazuh: encodeAst(addFieldToAst(ast, name, section, fields)) }
}

export function replacePdqlText(query: EventQuery, text: string): { query: EventQuery; error?: string } {
  const parsed = parseQueuePdql(text)
  if (!parsed.ok) return { query, error: parsed.error.message }
  const active = Boolean(predicate(parsed.ast)) || hasNativeControls(parsed.ast, PDQL_TIME_FIELD)
  return {
    query: {
      ...query,
      pdql: serialize(parsed.ast),
      appliedLanguage: active ? 'pt_pdql' : 'common',
    },
  }
}

export function replaceWazuhPredicate(query: EventQuery, text: string): { query: EventQuery; error?: string } {
  const parsed = parseDql(text)
  if (!parsed.ok) return { query, error: parsed.error.message }
  const limit = pipelineLimit(query)
  const current = readWazuhAst(query.wazuh).ast
  return {
    query: {
      ...query,
      pdql: serialize(withStoredLimit(readPdqlAst(query.pdql).ast, limit)),
      wazuh: encodeAst({ ...current, filter: parsed.filter, joiners: parsed.joiners, limit }),
      appliedLanguage: 'wazuh_dql',
    },
  }
}

const COMPARE_OPS = new Set<CompareOp>([
  '=',
  '!=',
  '>',
  '<',
  '>=',
  '<=',
  'contains',
  'startswith',
  'in',
  'is_null',
  'is_not_null',
])

export function eventQueryTabs(sources: EventSourceMeta[]): { id: AppliedEventLanguage; label: string }[] {
  const tabs: { id: AppliedEventLanguage; label: string }[] = [{ id: 'common', label: 'Общие' }]
  if (sources.some((source) => source.languages.includes('pt_pdql'))) tabs.push({ id: 'pt_pdql', label: 'PDQL' })
  if (sources.some((source) => source.languages.includes('wazuh_dql'))) tabs.push({ id: 'wazuh_dql', label: 'Wazuh' })
  return tabs
}

export function fieldsForLanguage(sources: EventSourceMeta[], language: SentEventLanguage): EventFieldDef[] {
  const source = sources.find((item) => item.languages.includes(language))
  return (source?.fields ?? []).map((field) => ({
    name: field.name,
    type:
      field.type === 'number' || field.type === 'datetime' || field.type === 'ip' || field.type === 'enum'
        ? field.type
        : 'string',
    description: eventFieldLabelRu(field.name),
    operators: field.operators.filter((op): op is CompareOp => COMPARE_OPS.has(op as CompareOp)),
    column: field.column,
    sort: field.sort,
    group: field.group,
  }))
}
