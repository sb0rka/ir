import type { AlertEvent, Entity, EntityKind } from '../../types'
import { isFindingFilterField } from './append'
import { entityKindForField } from './entityKind'
import { isFilterGroup, type CompareOp, type Condition, type FilterNode, type LogicalJoiner, type QueryAst } from './model'

function entityKindMatches(entityKind: EntityKind, wanted: EntityKind): boolean {
  if (wanted === 'account' || wanted === 'user') return entityKind === 'account' || entityKind === 'user'
  if (wanted === 'domain' || wanted === 'url' || wanted === 'email') {
    return entityKind === 'domain' || entityKind === 'email' || entityKind === 'url'
  }
  return entityKind === wanted
}

function fieldValues(alert: AlertEvent, field: string, entities: Record<string, Entity>): string[] {
  if (field === 'time') return alert.time ? [alert.time] : []
  const out: string[] = []
  const raw = alert.raw?.[field]
  if (raw) out.push(raw)
  const wanted = entityKindForField(field)
  if (!wanted) return out
  for (const id of alert.entityIds) {
    const entity = entities[id]
    if (!entity || !entityKindMatches(entity.kind, wanted)) continue
    if (entity.label) out.push(entity.label)
  }
  return out
}

function normalize(value: string): string {
  return value.trim().toLowerCase()
}

function parseInstant(value: string): number | undefined {
  const ms = Date.parse(value)
  return Number.isFinite(ms) ? ms : undefined
}

function compareOrdered(actual: string, expected: string, op: '>' | '<' | '>=' | '<='): boolean | undefined {
  const actualNum = Number(actual)
  const expectedNum = Number(expected)
  if (actual.trim() !== '' && expected.trim() !== '' && Number.isFinite(actualNum) && Number.isFinite(expectedNum)) {
    switch (op) {
      case '>':
        return actualNum > expectedNum
      case '<':
        return actualNum < expectedNum
      case '>=':
        return actualNum >= expectedNum
      case '<=':
        return actualNum <= expectedNum
    }
  }
  const actualMs = parseInstant(actual)
  const expectedMs = parseInstant(expected)
  if (actualMs != null && expectedMs != null) {
    switch (op) {
      case '>':
        return actualMs > expectedMs
      case '<':
        return actualMs < expectedMs
      case '>=':
        return actualMs >= expectedMs
      case '<=':
        return actualMs <= expectedMs
    }
  }
  const left = normalize(actual)
  const right = normalize(expected)
  switch (op) {
    case '>':
      return left > right
    case '<':
      return left < right
    case '>=':
      return left >= right
    case '<=':
      return left <= right
  }
}

function valueMatches(actual: string, op: CompareOp, expected: string, expectedList: string[]): boolean {
  const needle = normalize(expected)
  const haystack = normalize(actual)
  switch (op) {
    case '=':
      return haystack === needle
    case '!=':
      return haystack !== needle
    case 'contains':
      return haystack.includes(needle)
    case 'startswith':
      return haystack.startsWith(needle)
    case 'in':
      return expectedList.some((item) => haystack === normalize(item))
    case '>':
    case '<':
    case '>=':
    case '<=':
      return compareOrdered(actual, expected, op) === true
    case 'is_null':
    case 'is_not_null':
      return false
  }
}

function someValue(values: string[], pred: (value: string) => boolean): boolean {
  return values.some(pred)
}

function matchCondition(alert: AlertEvent, condition: Condition, entities: Record<string, Entity>): boolean {
  if (isFindingFilterField(condition.field)) return true
  const values = fieldValues(alert, condition.field, entities)
  let hit: boolean
  switch (condition.op) {
    case 'is_null':
      hit = values.length === 0
      break
    case 'is_not_null':
      hit = values.length > 0
      break
    case '!=':
      hit = values.length === 0 || values.every((value) => valueMatches(value, '!=', condition.value, condition.values))
      break
    default:
      hit = someValue(values, (value) => valueMatches(value, condition.op, condition.value, condition.values))
      break
  }
  return condition.negated ? !hit : hit
}

function evalList(
  nodes: FilterNode[],
  joiners: LogicalJoiner[],
  alert: AlertEvent,
  entities: Record<string, Entity>,
): boolean {
  if (nodes.length === 0) return true
  let acc = evalNode(nodes[0]!, alert, entities)
  for (let index = 1; index < nodes.length; index++) {
    const right = evalNode(nodes[index]!, alert, entities)
    acc = (joiners[index - 1] ?? 'and') === 'or' ? acc || right : acc && right
  }
  return acc
}

function evalNode(node: FilterNode, alert: AlertEvent, entities: Record<string, Entity>): boolean {
  if (!isFilterGroup(node)) return matchCondition(alert, node, entities)
  const inner = evalList(node.children, node.joiners, alert, entities)
  return node.negated ? !inner : inner
}

/** Client-side PDQL extras after a finding UUID has already selected the resolve set. */
export function alertMatchesPdql(
  alert: AlertEvent,
  ast: QueryAst,
  entities: Record<string, Entity> = {},
): boolean {
  return evalList(ast.filter, ast.joiners, alert, entities)
}
