import type { Severity } from './types'

export const SEVERITY_COLOR: Record<Severity, string> = {
  critical: 'var(--severity-critical)',
  high: 'var(--severity-high)',
  medium: 'var(--severity-medium)',
  low: 'var(--severity-low)',
}

export const ALL_ENTITY_TYPES = [
  'device',
  'host',
  'user',
  'process',
  'ip',
  'mac',
  'hostname',
  'file_hash',
  'domain',
  'url',
] as const

export const ALL_SEVERITIES = ['critical', 'high', 'medium', 'low'] as const
