import type { EventHeaderMeta, FieldColumn, FieldGroup, FieldRow } from './siemGroups'

const HEADER_SKIP = new Set(['time', 'text'])

function rowsOf(raw: Record<string, string>, fields: string[]): FieldRow[] {
  const rows: FieldRow[] = []
  for (const field of fields) {
    const value = raw[field]
    if (value == null || value === '') continue
    rows.push({ field, value })
  }
  return rows
}

function take(
  remaining: Map<string, string>,
  pick: (field: string) => boolean,
): FieldRow[] {
  const rows: FieldRow[] = []
  for (const [field, value] of [...remaining.entries()]) {
    if (!pick(field)) continue
    remaining.delete(field)
    rows.push({ field, value })
  }
  return rows
}

function namedGroup(
  remaining: Map<string, string>,
  id: string,
  title: string,
  pick: (field: string) => boolean,
): FieldGroup | undefined {
  const rows = take(remaining, pick)
  if (rows.length === 0) return undefined
  return { id, title, columns: [{ title: '', rows }] }
}

export function wazuhHeaderMeta(raw: Record<string, string>): EventHeaderMeta {
  return {
    source: [{ field: 'source', value: 'wazuh' }],
    identifier: rowsOf(raw, ['rule.id']),
    category: rowsOf(raw, ['rule.groups']),
  }
}

/** Group Wazuh native attributes for the event card (no MaxPatrol field names). */
export function groupWazuhFields(raw: Record<string, string>): FieldGroup[] {
  const remaining = new Map<string, string>()
  for (const [field, value] of Object.entries(raw)) {
    if (HEADER_SKIP.has(field) || value == null) continue
    remaining.set(field, value)
  }

  const header = wazuhHeaderMeta(raw)
  for (const row of [...header.source, ...header.identifier, ...header.category]) {
    remaining.delete(row.field)
  }

  const groups: FieldGroup[] = []
  const rule = namedGroup(
    remaining,
    'rule',
    'Правило',
    (field) =>
      field.startsWith('rule.') ||
      field === 'correlation_name' ||
      field === 'correlation_type',
  )
  if (rule) groups.push(rule)

  const agent = namedGroup(
    remaining,
    'agent',
    'Агент',
    (field) =>
      field.startsWith('agent.') ||
      field === 'manager.name' ||
      field === 'location' ||
      field === 'decoder.name',
  )
  if (agent) groups.push(agent)

  const network = namedGroup(
    remaining,
    'network',
    'Сеть',
    (field) =>
      field === 'data.srcip' ||
      field === 'data.dstip' ||
      field === 'data.srcport' ||
      field === 'data.dstport' ||
      field === 'data.protocol' ||
      field === 'data.url' ||
      field === 'data.status' ||
      field === 'data.id' ||
      field.startsWith('GeoLocation.'),
  )
  if (network) groups.push(network)

  const accounts = namedGroup(
    remaining,
    'accounts',
    'Учётные записи',
    (field) =>
      field === 'data.srcuser' ||
      field === 'data.dstuser' ||
      field.startsWith('data.win.eventdata.') ||
      field.startsWith('data.win.system.') ||
      field === 'data.system_name',
  )
  if (accounts) groups.push(accounts)

  const files = namedGroup(
    remaining,
    'files',
    'Файлы',
    (field) => field.startsWith('syscheck.') || field.startsWith('data.virustotal.'),
  )
  if (files) groups.push(files)

  const integrations = namedGroup(
    remaining,
    'integrations',
    'Интеграции',
    (field) =>
      field.startsWith('data.aws.') ||
      field.startsWith('data.office365.') ||
      field.startsWith('data.github.') ||
      field.startsWith('data.gcp.') ||
      field.startsWith('data.ms-graph.') ||
      field.startsWith('data.vulnerability.') ||
      field.startsWith('data.audit.') ||
      field.startsWith('data.docker.') ||
      field.startsWith('data.osquery.') ||
      field === 'data.integration',
  )
  if (integrations) groups.push(integrations)

  if (remaining.size > 0) {
    const rows = [...remaining.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([field, value]) => ({ field, value }))
    groups.push({
      id: 'other',
      title: 'Прочее',
      columns: [{ title: '', rows }],
    })
  }

  if (header.source.length || header.identifier.length || header.category.length) {
    const headerRows: FieldRow[] = [...header.source, ...header.identifier, ...header.category]
    const headerColumns: FieldColumn[] = [{ title: '', rows: headerRows }]
    groups.unshift({ id: 'header', title: 'Параметры', columns: headerColumns })
  }

  return groups
}
