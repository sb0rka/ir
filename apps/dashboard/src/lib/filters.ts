import type { Entity, EntityKind, FilterChip, FilterField } from '../types'

/** Chip filter semantics shared by the global queue and investigation queues. */
export function matchesChips(
  entityIds: string[],
  severity: string,
  source: string,
  status: string,
  chips: FilterChip[],
  entities: Record<string, Entity>,
): boolean {
  if (chips.length === 0) return true
  const ents = entityIds.map((id) => entities[id]).filter(Boolean)

  return chips.every((chip) => {
    const vals = chip.values.map((v) => v.toLowerCase())
    switch (chip.field) {
      case 'severity':
        return vals.includes(severity.toLowerCase())
      case 'source':
        return vals.includes(source.toLowerCase())
      case 'status':
        return vals.includes(status.toLowerCase())
      case 'host':
        return ents.some(
          (e) => e.kind === 'host' && vals.includes(e.label.toLowerCase()),
        )
      case 'account':
      case 'user':
        return ents.some(
          (e) =>
            (e.kind === 'user' || e.kind === 'account') &&
            vals.includes(e.label.toLowerCase()),
        )
      case 'process':
        return ents.some(
          (e) => e.kind === 'process' && vals.includes(e.label.toLowerCase()),
        )
      case 'ip':
        return ents.some(
          (e) => e.kind === 'ip' && vals.includes(e.label.toLowerCase()),
        )
      case 'domain':
        return ents.some(
          (e) =>
            (e.kind === 'domain' || e.kind === 'email' || e.kind === 'url') &&
            vals.some((v) => e.label.toLowerCase().includes(v.replace(/[\[\]]/g, ''))),
        )
      case 'hash':
        return ents.some((e) =>
          vals.some(
            (v) =>
              e.kind === 'file_hash' ||
              e.attributes.hash?.toLowerCase().includes(v) ||
              e.attributes.hash?.toLowerCase() === v ||
              e.label.toLowerCase().includes(v),
          ),
        )
      default:
        return true
    }
  })
}

/** Which filter field an entity chip click maps to («Найти связанные»). */
export function fieldForEntityKind(kind: EntityKind): FilterField | null {
  switch (kind) {
    case 'host':
      return 'host'
    case 'user':
    case 'account':
      return 'user'
    case 'process':
      return 'process'
    case 'ip':
      return 'ip'
    case 'domain':
    case 'url':
    case 'email':
      return 'domain'
    case 'file_hash':
      return 'hash'
    default:
      return null
  }
}

/** PDQL field used when a queue chip/entity click is appended to the query. */
export function pdqlFieldForFilterField(field: FilterField): string {
  switch (field) {
    case 'host':
      return 'event_src.host'
    case 'user':
    case 'account':
      return 'subject.account.name'
    case 'process':
      return 'object.process.name'
    case 'ip':
      return 'src.ip'
    case 'hash':
      return 'object.file.hash.sha256'
    case 'domain':
      return 'object.url'
    case 'severity':
      return 'importance'
    case 'source':
      return 'event_src.vendor'
    case 'status':
      return 'status'
  }
}
