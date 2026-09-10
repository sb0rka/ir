import {
  defaultQuery,
  emptyQuery,
  newId,
  type CompareOp,
  type Condition,
  type FilterGroup,
  type FilterNode,
  type LogicalJoiner,
} from './model'
import { appendFilterNode } from './filterTree'
import { parse } from './parse'
import { serialize } from './serialize'

function conditionValue(op: CompareOp, value: string): { value: string; values: string[] } {
  if (op === 'is_null' || op === 'is_not_null') return { value: '', values: [] }
  if (op === 'in') {
    return {
      value: '',
      values: value
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean),
    }
  }
  return { value, values: [] }
}

function toCondition(field: string, op: CompareOp, value: string): Condition {
  const next = conditionValue(op, value)
  return {
    id: newId('c'),
    field,
    op,
    value: next.value,
    values: next.values,
    negated: false,
  }
}

function uniqueFields(fields: readonly string[]): string[] {
  const names: string[] = []
  for (const field of fields) {
    const name = field.trim()
    if (name && !names.includes(name)) names.push(name)
  }
  return names
}

/** Append `field op value` to an existing PDQL filter (AND). Broken input is replaced. */
export function appendCondition(
  pdql: string,
  field: string,
  op: CompareOp,
  value: string,
): string {
  return appendConditions(pdql, [field], op, value)
}

/** Append one condition, or a grouped `(a op v joiner b op v …)` when several fields are set. */
export function appendConditions(
  pdql: string,
  fields: readonly string[],
  op: CompareOp,
  value: string,
  joiner: LogicalJoiner = 'and',
): string {
  const names = uniqueFields(fields)
  if (names.length === 0) return pdql
  const trimmed = pdql.trim()
  const parsed = trimmed ? parse(trimmed) : { ok: true as const, ast: emptyQuery() }
  const ast = parsed.ok ? parsed.ast : emptyQuery()
  const node: FilterNode =
    names.length === 1
      ? toCondition(names[0]!, op, value)
      : ({
          kind: 'group',
          id: newId('fgrp'),
          negated: false,
          children: names.map((name) => toCondition(name, op, value)),
          joiners: Array.from({ length: names.length - 1 }, () => joiner),
        } satisfies FilterGroup)
  return serialize(appendFilterNode(ast, node))
}

export type FindingFilterField = 'siem_incident' | 'siem_correlation'

export const FINDING_FILTER_LABELS: Record<FindingFilterField, string> = {
  siem_incident: 'Инцидент',
  siem_correlation: 'Корреляция',
}

export function isFindingFilterField(field: string): field is FindingFilterField {
  return field === 'siem_incident' || field === 'siem_correlation'
}

/** Replace the current query with a single incident/correlation resolve filter. */
export function findingUuidQuery(uuid: string, recordType: FindingFilterField): string {
  return appendCondition(serialize(defaultQuery()), recordType, '=', uuid)
}
