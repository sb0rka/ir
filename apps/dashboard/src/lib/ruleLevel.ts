import type { EventGroupItem } from '../types'

const BOUNDS: Record<string, [number, number]> = {
  INFO: [0, 2],
  LOW: [3, 5],
  MEDIUM: [6, 9],
  HIGH: [10, 12],
  CRITICAL: [13, 16],
}

/** Same buckets as the Gateway Wazuh adapter: rule.level → info/low/medium/high/critical. */
export function ruleLevelSeverity(value: string | null | undefined): string | null {
  if (value == null || value.trim() === '' || !/^-?\d+$/.test(value.trim())) return null
  const level = Number(value)
  if (level >= 0 && level <= 2) return 'INFO'
  if (level >= 3 && level <= 5) return 'LOW'
  if (level >= 6 && level <= 9) return 'MEDIUM'
  if (level >= 10 && level <= 12) return 'HIGH'
  if (level >= 13 && level <= 16) return 'CRITICAL'
  return null
}

/** Severity name or a numeric level inside a bucket. */
export function ruleLevelBucket(value: string | null | undefined): string | null {
  if (value == null) return null
  const name = value.trim().toUpperCase()
  if (BOUNDS[name]) return name
  return ruleLevelSeverity(value)
}

/** DQL bounds for one severity bucket. The event search uses this instead of one group value. */
export function ruleLevelBucketClause(value: string | null | undefined): string | null {
  const severity = ruleLevelBucket(value)
  if (!severity) return null
  const [lo, hi] = BOUNDS[severity]!
  return `rule.level >= ${lo} AND rule.level <= ${hi}`
}

export type RuleLevelRow = {
  key: string
  label: string
  annotation: string
  count: number
  source_code: string
  selectValue: string | null
  members: (string | null)[]
}

function sourceLabel(code: string): string {
  if (code === 'pt-maxpatrol-siem') return 'MaxPatrol'
  if (code === 'wazuh') return 'Wazuh'
  if (code === 'pt-nad') return 'NAD'
  return code
}

function valueLabel(value: string | null): string {
  return value == null || value === '' ? 'Нет данных' : value
}

function levelList(members: (string | null)[]): string {
  return members
    .filter((item): item is string => item != null && item !== '')
    .slice()
    .sort((left, right) => Number(left) - Number(right))
    .join(', ')
}

/**
 * rule.level rows that share a gateway severity are one row per source.
 * The count is the sum of those levels. Other fields stay one row per value.
 */
export function collapseRuleLevelGroups(field: string | undefined, groups: EventGroupItem[]): RuleLevelRow[] {
  if (field !== 'rule.level') {
    return groups.map((group) => {
      const value = group.values[0] ?? null
      return {
        key: `${group.source_code}:${JSON.stringify(group.values)}`,
        label: valueLabel(value),
        annotation: sourceLabel(group.source_code),
        count: group.count,
        source_code: group.source_code,
        selectValue: value,
        members: [value],
      }
    })
  }
  const multiSource = new Set(groups.map((group) => group.source_code)).size > 1
  const rows: RuleLevelRow[] = []
  const index = new Map<string, RuleLevelRow>()
  for (const group of groups) {
    const value = group.values[0] ?? null
    const severity = ruleLevelSeverity(value)
    const key = severity ? `${group.source_code}\n${severity}` : `${group.source_code}\n${JSON.stringify(group.values)}`
    const existing = index.get(key)
    if (existing) {
      existing.count += group.count
      existing.members.push(value)
      existing.annotation = levelList(existing.members)
      continue
    }
    const row: RuleLevelRow = {
      key,
      label: severity ?? valueLabel(value),
      annotation: severity ? (value ?? '') : sourceLabel(group.source_code),
      count: group.count,
      source_code: group.source_code,
      selectValue: severity ?? value,
      members: [value],
    }
    index.set(key, row)
    rows.push(row)
  }
  for (const row of rows) {
    if (!ruleLevelBucket(row.selectValue)) continue
    const levels = levelList(row.members)
    row.annotation = multiSource && levels ? `${levels} ${sourceLabel(row.source_code)}` : levels
  }
  return rows.sort((left, right) => right.count - left.count || left.label.localeCompare(right.label))
}

export function ruleLevelRowSelected(row: RuleLevelRow, selected: (string | null)[]): boolean {
  if (selected.length === 0) return false
  const current = selected[0]
  if (current === row.selectValue) return true
  if (row.members.some((member) => member === current)) return true
  return ruleLevelBucket(current) != null && ruleLevelBucket(current) === row.selectValue
}
