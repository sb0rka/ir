import type { EntityKind } from '../../types'

/** Role used on event→entity graph edges when the field name encodes direction. */
export function roleForField(field: string): string {
  if (field.startsWith('src.')) return 'src'
  if (field.startsWith('dst.')) return 'dst'
  if (field.startsWith('subject.')) return 'actor'
  if (field.startsWith('object.')) return 'object'
  return 'mentions'
}

/**
 * Which entity type a SIEM/PDQL field represents. Unmapped fields (action,
 * status, importance, …) cannot be added to context as entities.
 */
export function entityKindForField(field: string): EntityKind | null {
  if (field.endsWith('.ip')) return 'ip'
  if (field.includes('.hash.')) return 'file_hash'
  if (field.includes('.account.')) return 'account'
  if (field.includes('.process.')) return 'process'
  if (field.endsWith('.url')) return 'url'
  if (
    field.endsWith('.host') ||
    field.endsWith('.hostname') ||
    field.endsWith('.fqdn')
  ) {
    return 'host'
  }
  if (field.endsWith('.domain') || field.includes('.domain')) return 'domain'
  return null
}
