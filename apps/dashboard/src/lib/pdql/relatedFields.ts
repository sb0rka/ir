export interface RelatedFieldColumn {
  title: string
  fields: string[]
}

/** Catalog names from MaxPatrol event-fields (gateway mock + live SIEM). */
export const KNOWN_EVENT_FIELDS = new Set<string>([
  'time',
  'recv_time',
  'uuid',
  'id',
  'text',
  'importance',
  'status',
  'msgid',
  'protocol',
  'action',
  'correlation_name',
  'correlation_type',
  'correlation_event_id',
  'incident.key',
  'event_src.host',
  'event_src.ip',
  'event_src.hostname',
  'event_src.fqdn',
  'event_src.title',
  'event_src.vendor',
  'event_src.subsys',
  'event_src.asset',
  'src.ip',
  'src.host',
  'src.port',
  'src.mac',
  'src.hostname',
  'src.fqdn',
  'src.geo.country',
  'dst.ip',
  'dst.host',
  'dst.port',
  'dst.mac',
  'dst.hostname',
  'dst.fqdn',
  'subject.account.name',
  'subject.account.domain',
  'subject.account.id',
  'subject.user.name',
  'subject.process.name',
  'subject.process.id',
  'subject.process.fullpath',
  'subject.process.cmdline',
  'subject.process.hash.md5',
  'subject.process.hash.sha256',
  'object.process.name',
  'object.process.id',
  'object.process.fullpath',
  'object.process.cmdline',
  'object.process.parent.name',
  'object.process.parent.id',
  'object.process.hash.md5',
  'object.process.hash.sha256',
  'object.account.name',
  'object.account.domain',
  'object.file.name',
  'object.file.path',
  'object.file.hash.md5',
  'object.file.hash.sha256',
  'object.registry.key',
  'object.url',
  'object.value',
  'category.generic',
  'category.high',
  'category.low',
])

const DIRECTIONAL_SUFFIXES = new Set(['ip', 'host', 'hostname', 'fqdn', 'port', 'mac'])

function keep(names: string[], clicked: string): string[] {
  const out: string[] = []
  for (const name of names) {
    if ((KNOWN_EVENT_FIELDS.has(name) || name === clicked) && !out.includes(name)) {
      out.push(name)
    }
  }
  return out
}

function columnsOf(
  clicked: string,
  specs: ReadonlyArray<{ title: string; fields: string[] }>,
): RelatedFieldColumn[] {
  const columns: RelatedFieldColumn[] = []
  for (const spec of specs) {
    const fields = keep(spec.fields, clicked)
    if (fields.length > 0) columns.push({ title: spec.title, fields })
  }
  return columns.length > 0 ? columns : [{ title: 'Поле', fields: [clicked] }]
}

/** Related PDQL fields to offer when a card value is clicked. */
export function relatedFieldColumns(field: string): RelatedFieldColumn[] {
  const directional = field.match(/^(src|dst|event_src)\.(.+)$/)
  if (directional && DIRECTIONAL_SUFFIXES.has(directional[2])) {
    const suffix = directional[2]
    return columnsOf(field, [
      { title: 'Источник', fields: [`src.${suffix}`, `event_src.${suffix}`] },
      { title: 'Назначение', fields: [`dst.${suffix}`] },
    ])
  }

  const hash = field.match(/^(?:subject|object)\.(?:process|file)\.hash\.(md5|sha256)$/)
  if (hash) {
    const algo = hash[1]
    return columnsOf(field, [
      { title: 'Процесс', fields: [`subject.process.hash.${algo}`, `object.process.hash.${algo}`] },
      { title: 'Файл', fields: [`object.file.hash.${algo}`] },
    ])
  }

  const process = field.match(/^(?:object|subject)\.process\.(parent\.)?(.+)$/)
  if (process) {
    const leaf = process[2]
    if (leaf === 'name' || leaf === 'id') {
      return columnsOf(field, [
        { title: 'Процесс', fields: [`object.process.${leaf}`, `subject.process.${leaf}`] },
        { title: 'Родитель', fields: [`object.process.parent.${leaf}`] },
      ])
    }
    return columnsOf(field, [
      { title: 'Субъект', fields: [`subject.process.${leaf}`] },
      { title: 'Объект', fields: [`object.process.${leaf}`] },
    ])
  }

  const account = field.match(/^(?:subject|object)\.account\.(.+)$/)
  if (account) {
    const leaf = account[1]
    return columnsOf(field, [
      { title: 'Субъект', fields: [`subject.account.${leaf}`] },
      { title: 'Объект', fields: [`object.account.${leaf}`] },
    ])
  }

  return [{ title: 'Поле', fields: [field] }]
}
