import type { Severity } from './types'

export const SEVERITY_COLOR: Record<Severity, string> = {
  critical: 'var(--severity-critical)',
  high: 'var(--severity-high)',
  medium: 'var(--severity-medium)',
  low: 'var(--severity-low)',
}

export const ALL_ENTITY_TYPES = [
  'host',
  'user',
  'process',
  'ip',
  'file_hash',
  'domain',
  'url',
] as const

export const DEFAULT_ENTITY_TYPES = ALL_ENTITY_TYPES

export const ALL_SEVERITIES = ['critical', 'high', 'medium', 'low'] as const
