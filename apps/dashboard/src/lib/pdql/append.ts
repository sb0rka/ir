import { defaultQuery, emptyQuery, newId, type CompareOp, type Condition } from './model'
import { parse } from './parse'
import { serialize } from './serialize'

/** Append `field op value` to an existing PDQL filter (AND). Broken input is replaced. */
export function appendCondition(
  pdql: string,
  field: string,
  op: CompareOp,
  value: string,
): string {
  const trimmed = pdql.trim()
  const parsed = trimmed ? parse(trimmed) : { ok: true as const, ast: emptyQuery() }
  const ast = parsed.ok ? parsed.ast : emptyQuery()
  const condition: Condition = {
    id: newId('c'),
    field,
    op,
    value,
    values: [],
    negated: false,
  }
  return serialize({
    ...ast,
    filter: [...ast.filter, condition],
    joiners: ast.filter.length > 0 ? [...ast.joiners, 'and'] : [...ast.joiners],
  })
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
