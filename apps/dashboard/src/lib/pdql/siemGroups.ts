export interface FieldRow {
  field: string
  value: string
}

export interface FieldColumn {
  title: string
  rows: FieldRow[]
}

export interface FieldGroup {
  id: string
  title: string
  columns: FieldColumn[]
}

export interface EventHeaderMeta {
  source: FieldRow[]
  identifier: FieldRow[]
  category: FieldRow[]
}

const HEADER_SKIP = new Set(['time', 'text'])

const CATEGORY_FIELDS = ['category.generic', 'category.high', 'category.low'] as const

const INTERACTION_FIELDS = new Set([
  'importance',
  'action',
  'status',
  'reason',
  'protocol',
  'logon_type',
  'logon_service',
  'logon_auth_method',
])

const SERVICE_FIELDS = new Set([
  'id',
  'uuid',
  'vendor_event_id',
  'agent_id',
  'input_id',
  'task_id',
  'normalized',
  'tag',
  'mime',
  'taxonomy_version',
  'historical',
  'original_time',
  'incorrect_time',
  'scope_id',
  'tenant_id',
  'job_id',
  'labels',
  'remote',
  'siem_id',
  'siem_alias',
  'site_alias',
  'site_name',
  'site_address',
  'site_id',
])

const RAW_FIELDS = new Set(['raw', 'raw_event', 'original', 'original_event', '_raw'])

export function isSiemSource(source: string): boolean {
  const lower = source.toLowerCase()
  return lower.includes('siem') || lower.includes('maxpatrol')
}

export function isCorrelationRecord(raw: Record<string, string>): boolean {
  if (raw.finding_kind === 'siem_correlation') return true
  return Boolean(raw.correlation_name)
}

export function isFindingRecord(raw: Record<string, string>, recordType?: string): boolean {
  const kind = recordType || raw.finding_kind || ''
  return kind === 'siem_incident' || kind === 'siem_correlation' || isCorrelationRecord(raw)
}

export function eventHeaderMeta(raw: Record<string, string>, source: string): EventHeaderMeta {
  const sourceRows = rowsOf(raw, ['event_src.vendor', 'event_src.title'])
  if (sourceRows.length === 0 && source) sourceRows.push({ field: 'source', value: source })
  return {
    source: sourceRows,
    identifier: rowsOf(raw, ['msgid', 'event_src.subsys']),
    category: rowsOf(raw, CATEGORY_FIELDS),
  }
}

function headerParameterRows(raw: Record<string, string>, source: string): FieldRow[] {
  const meta = eventHeaderMeta(raw, source)
  return [...meta.source, ...meta.identifier, ...meta.category]
}

export function groupEventFields(source: string, raw: Record<string, string>): FieldGroup[] {
  const remaining = new Map<string, string>()
  for (const [field, value] of Object.entries(raw)) {
    if (HEADER_SKIP.has(field) || value == null) continue
    remaining.set(field, value)
  }
  if (!isSiemSource(source) && !looksLikeSiem(remaining)) {
    return leftoverGroup(remaining)
  }

  const headerRows = headerParameterRows(raw, source)
  for (const row of headerRows) remaining.delete(row.field)

  const incident = isIncidentRecord(raw)
  const correlation = isCorrelationRecord(raw)
  const incidentFields = incident ? incidentGroup(remaining) : undefined
  const correlationFields = correlation
    ? namedGroup(remaining, 'correlation', 'Параметры корреляции', isCorrelationField)
    : undefined
  const roles = pairGroup(remaining, 'roles', 'Роли во взаимодействии', [
    { title: 'Субъект', pick: (field) => field === 'subject' || field.startsWith('subject.') },
    { title: 'Объект', pick: (field) => field === 'object' || field.startsWith('object.') },
  ])
  const addresses = pairGroup(remaining, 'addresses', 'Адресаты', [
    { title: 'Отправитель', pick: (field) => field.startsWith('src.') },
    { title: 'Получатель', pick: (field) => field.startsWith('dst.') || field.startsWith('external_dst.') },
  ])
  const interactionRows = take(
    remaining,
    (field) => INTERACTION_FIELDS.has(field) || field.startsWith('logon_'),
  )
  const interaction =
    headerRows.length > 0 || interactionRows.length > 0
      ? {
          id: 'interaction',
          title: 'Параметры',
          columns: [{ title: '', rows: [...headerRows, ...interactionRows] }],
        }
      : undefined
  const eventSrc = namedGroup(remaining, 'event-src', 'Источник событий', isEventSourceField)
  const collection = namedGroup(remaining, 'collection', 'Точка сбора', (field) =>
    field === 'recv_host' || field === 'recv_time' || field.startsWith('recv_'),
  )
  const serviceRows = take(remaining, isServiceField)
  const rawEvent = namedGroup(remaining, 'raw', 'Исходное событие', (field) => RAW_FIELDS.has(field))
  const extra = mergeExtra(leftoverGroup(remaining)[0], serviceRows)

  const groups: FieldGroup[] = []
  if (incident) {
    pushGroup(groups, incidentFields)
    pushGroup(groups, roles)
    pushGroup(groups, addresses)
    pushGroup(groups, interaction)
    pushGroup(groups, eventSrc)
    pushGroup(groups, collection)
    pushGroup(groups, extra)
    pushGroup(groups, rawEvent)
    return groups
  }
  if (correlation) {
    pushGroup(groups, correlationFields)
    pushGroup(groups, roles)
    pushGroup(groups, addresses)
    pushGroup(groups, interaction)
    pushGroup(groups, eventSrc)
    pushGroup(groups, collection)
    pushGroup(groups, extra)
    pushGroup(groups, rawEvent)
    return groups
  }

  pushGroup(groups, roles)
  pushGroup(groups, addresses)
  pushGroup(groups, interaction)
  pushGroup(groups, extra)
  pushGroup(groups, eventSrc)
  pushGroup(groups, collection)
  pushGroup(groups, rawEvent)
  return groups
}

function looksLikeSiem(fields: Map<string, string>): boolean {
  for (const field of fields.keys()) {
    if (
      field.startsWith('subject.') ||
      field.startsWith('object.') ||
      field.startsWith('event_src.') ||
      field.startsWith('correlation_') ||
      field.startsWith('category.')
    ) {
      return true
    }
  }
  return false
}

function isIncidentRecord(raw: Record<string, string>): boolean {
  return raw.finding_kind === 'siem_incident'
}

const INCIDENT_FIELD_ORDER = [
  'incident.type',
  'incident.key',
  'incident.external_key',
  'incident.verdict',
  'incident.damage',
  'incident.recommendation',
  'incident.assigned_to',
  'status',
  'rule.name',
  'uuid',
  'finding_kind',
] as const

function incidentGroup(remaining: Map<string, string>): FieldGroup | undefined {
  const rows: FieldRow[] = []
  for (const field of INCIDENT_FIELD_ORDER) {
    const value = remaining.get(field)
    if (value == null || value === '') continue
    remaining.delete(field)
    rows.push({ field, value })
  }
  for (const [field, value] of [...remaining.entries()]) {
    if (!isIncidentField(field)) continue
    remaining.delete(field)
    rows.push({ field, value })
  }
  if (rows.length === 0) return undefined
  return { id: 'incident', title: 'Параметры инцидента', columns: [{ title: '', rows }] }
}

function isIncidentField(field: string): boolean {
  return (
    field.startsWith('incident.') ||
    field === 'finding_kind' ||
    field === 'rule.name' ||
    field === 'uuid' ||
    field === 'status'
  )
}

function isCorrelationField(field: string): boolean {
  return (
    field.startsWith('correlation_') ||
    field.startsWith('alert.') ||
    field.startsWith('incident.') ||
    field === 'count.subevents' ||
    field === 'subevents' ||
    field === 'finding_kind' ||
    field === 'rule.name'
  )
}

function isEventSourceField(field: string): boolean {
  return (
    field.startsWith('event_src.') ||
    field.startsWith('origin_app_') ||
    field.startsWith('storage_app_') ||
    field.startsWith('primary_siem_')
  )
}

function isServiceField(field: string): boolean {
  return SERVICE_FIELDS.has(field) || field.startsWith('_') || field.startsWith('generator.')
}

function rowsOf(raw: Record<string, string>, fields: readonly string[]): FieldRow[] {
  const rows: FieldRow[] = []
  for (const field of fields) {
    const value = raw[field]
    if (value == null || value === '') continue
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

function pairGroup(
  remaining: Map<string, string>,
  id: string,
  title: string,
  specs: ReadonlyArray<{ title: string; pick: (field: string) => boolean }>,
): FieldGroup | undefined {
  const columns: FieldColumn[] = []
  for (const spec of specs) {
    const rows = take(remaining, spec.pick)
    if (rows.length > 0) columns.push({ title: spec.title, rows })
  }
  if (columns.length === 0) return undefined
  return { id, title, columns }
}

function leftoverGroup(remaining: Map<string, string>): FieldGroup[] {
  const rows: FieldRow[] = []
  for (const [field, value] of remaining) {
    rows.push({ field, value })
  }
  remaining.clear()
  if (rows.length === 0) return []
  return [{ id: 'extra', title: 'Дополнительная информация', columns: [{ title: '', rows }] }]
}

function mergeExtra(extra: FieldGroup | undefined, serviceRows: FieldRow[]): FieldGroup | undefined {
  if (serviceRows.length === 0) return extra
  if (!extra) {
    return { id: 'extra', title: 'Дополнительная информация', columns: [{ title: '', rows: serviceRows }] }
  }
  extra.columns[0]?.rows.push(...serviceRows)
  return extra
}

function take(remaining: Map<string, string>, pick: (field: string) => boolean): FieldRow[] {
  const rows: FieldRow[] = []
  for (const [field, value] of remaining) {
    if (!pick(field)) continue
    rows.push({ field, value })
  }
  for (const row of rows) remaining.delete(row.field)
  return rows
}

function pushGroup(groups: FieldGroup[], group: FieldGroup | undefined) {
  if (group) groups.push(group)
}
