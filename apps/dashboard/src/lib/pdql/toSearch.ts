import { isFindingFilterField, type FindingFilterField } from './append'
import type { FilterChip } from '../../types'
import type { Condition, QueryAst } from './model'
import { formatCondition } from './serialize'
import {
  defaultWorkingTimeZone,
  parseTimestamp,
  resolve,
  type TimeInterval,
} from '../../components/time-interval/model'

export type SearchEntityType = 'host' | 'user' | 'account' | 'process' | 'ip'

export interface PdqlSearchEntity {
  type: SearchEntityType
  value: string
}

export interface PdqlSearchParts {
  entities: PdqlSearchEntity[]
  query: string
}

const ENTITY_FIELDS: Record<string, SearchEntityType> = {
  // Canonical entity chips (queue «Сущности» / involved hosts & accounts).
  host: 'host',
  account: 'account',
  user: 'user',
  ip: 'ip',
  process: 'process',
  // SIEM PDQL fields that map to the same entity types.
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

/** Bare entity field names used as queue filters (not SIEM PDQL paths). */
export function isEntityQueueField(field: string): boolean {
  return field === 'host' || field === 'account' || field === 'user' || field === 'ip' || field === 'process'
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

function isTimeBound(condition: Condition): boolean {
  if (condition.field !== 'time' || condition.negated) return false
  return (
    condition.op === '=' ||
    condition.op === '>' ||
    condition.op === '>=' ||
    condition.op === '<' ||
    condition.op === '<='
  )
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
    // Time bounds go to gateway time_range (findings) / SIEM filter (events), not title text search.
    if (isTimeBound(condition)) return
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

const SECOND_MS = 1000

export type TimeIntervalFromAstResult = {
  /** Effective range sent as gateway time_range (wide ∩ PDQL). */
  interval: TimeInterval
  /** True when PDQL time bounds narrowed the wide range. */
  rewritten: boolean
}

function assertSingleTimeWindow(ast: QueryAst): void {
  for (let index = 0; index < ast.filter.length; index++) {
    const condition = ast.filter[index]!
    if (condition.field !== 'time') continue
    if (condition.negated) {
      throw new Error('NOT time в PDQL не поддерживается для окна времени')
    }
    if (!isTimeBound(condition)) {
      throw new Error(`Оператор time ${condition.op} не поддерживается для окна времени`)
    }
    if (index > 0 && (ast.joiners[index - 1] ?? 'and') === 'or') {
      throw new Error(
        'Несколько окон времени в PDQL не поддерживаются — используйте одно AND-окно (time >= … and time <= …)',
      )
    }
    if (index < ast.filter.length - 1 && (ast.joiners[index] ?? 'and') === 'or') {
      throw new Error(
        'Несколько окон времени в PDQL не поддерживаются — используйте одно AND-окно (time >= … and time <= …)',
      )
    }
  }
}

/**
 * Intersect the UI wide-range time filter with PDQL `time` AND-bounds.
 * Findings have no PDQL filter on the wire — only time_range → IM detectedAt.
 * Complex OR time windows are rejected (gateway accepts a single range).
 */
export function timeIntervalFromAst(
  ast: QueryAst,
  fallback: TimeInterval,
  timeZone = defaultWorkingTimeZone(),
): TimeIntervalFromAstResult {
  assertSingleTimeWindow(ast)

  const fallbackRange = resolve(fallback)
  let fromMs = Date.parse(fallbackRange.from)
  let toMs = Date.parse(fallbackRange.to)
  let touched = false

  for (const condition of ast.filter) {
    if (!isTimeBound(condition)) continue
    const iso = parseTimestamp(condition.value.trim(), timeZone)
    if (!iso) {
      throw new Error(`Не удалось разобрать время в PDQL: ${condition.value}`)
    }
    const boundMs = Date.parse(iso)
    if (!Number.isFinite(boundMs)) {
      throw new Error(`Некорректное время в PDQL: ${condition.value}`)
    }
    touched = true
    switch (condition.op) {
      case '=':
        // Gateway requires from < to — represent equality as a 1s window, then clamp.
        fromMs = Math.max(fromMs, boundMs)
        toMs = Math.min(toMs, boundMs + SECOND_MS)
        break
      case '>':
        fromMs = Math.max(fromMs, boundMs + SECOND_MS)
        break
      case '>=':
        fromMs = Math.max(fromMs, boundMs)
        break
      case '<':
        toMs = Math.min(toMs, boundMs)
        break
      case '<=':
        // Inclusive upper second: end 1s after the bound so gateway from < to holds
        // even when wide.from == bound. Never expand past the UI wide-range end.
        toMs = Math.min(toMs, boundMs + SECOND_MS)
        break
    }
  }

  if (!touched) return { interval: fallback, rewritten: false }

  if (!Number.isFinite(fromMs) || !Number.isFinite(toMs) || fromMs >= toMs) {
    throw new Error(
      'Окно времени пустое: PDQL time отсёк весь интервал кнопки (нужно from < to). Расширьте окно или ослабьте условие time.',
    )
  }

  const interval: TimeInterval = {
    kind: 'range',
    from: new Date(fromMs).toISOString(),
    to: new Date(toMs).toISOString(),
  }
  const rewritten =
    interval.from !== fallbackRange.from || interval.to !== fallbackRange.to
  return { interval, rewritten }
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

/** Select fields to show as extra queue columns. `time` and group keys already have UI. */
export function queueSelectFields(ast: QueryAst): string[] {
  const groupFields = new Set(ast.groups.map((group) => group.field))
  return uniqueFields(
    ast.columns
      .filter((column) => column.field && !column.aggregate && !groupFields.has(column.field))
      .map((column) => column.field),
  ).filter((field) => field !== 'time')
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

/** Finding resolve chip, even when other filters are also present (they are ignored). */
export function findingUuidFromAst(ast: QueryAst): {
  uuid: string
  recordType: FindingFilterField
} | null {
  for (const condition of ast.filter) {
    if (!isFindingFilterField(condition.field) || condition.op !== '=' || condition.negated) continue
    const value = condition.value.trim()
    if (!value) continue
    return { uuid: value, recordType: condition.field }
  }
  return null
}

export function astToEventSearch(
  ast: QueryAst,
  groupValues?: (string | null)[],
): EventSearchParts {
  // Entity predicates go in gateway `entities`, not MaxPatrol PDQL `filter`
  // (bare `host = "…"` is invalid SIEM syntax and fails all sources).
  const kept: Condition[] = []
  const joiners: QueryAst['joiners'] = []
  for (let index = 0; index < ast.filter.length; index++) {
    const condition = ast.filter[index]!
    if (isMappedEntity(condition)) continue
    if (kept.length > 0) joiners.push(ast.joiners[index - 1] ?? 'and')
    kept.push(condition)
  }
  const filter = formatCondition({ ...ast, filter: kept, joiners }).trim()
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
  const entityParts = pdqlToSearchParts(ast)
  parts.hasControls = Boolean(
    parts.filter || parts.sort || parts.group_by || entityParts.entities.length > 0,
  )
  return parts
}
