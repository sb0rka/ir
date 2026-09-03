import { describe, expect, it } from 'vitest'
import { entityKindForField, roleForField } from './entityKind'
import {
  eventHeaderMeta,
  groupEventFields,
  isCorrelationRecord,
  isFindingRecord,
  isSiemSource,
} from './siemGroups'

describe('entityKindForField', () => {
  it('maps directional host/ip and account/process/hash fields', () => {
    expect(entityKindForField('src.host')).toBe('host')
    expect(entityKindForField('dst.fqdn')).toBe('host')
    expect(entityKindForField('event_src.ip')).toBe('ip')
    expect(entityKindForField('subject.account.name')).toBe('account')
    expect(entityKindForField('object.process.name')).toBe('process')
    expect(entityKindForField('object.file.hash.sha256')).toBe('file_hash')
    expect(entityKindForField('object.url')).toBe('url')
    expect(entityKindForField('action')).toBeNull()
    expect(entityKindForField('importance')).toBeNull()
  })

  it('derives edge roles from the field prefix', () => {
    expect(roleForField('src.host')).toBe('src')
    expect(roleForField('dst.ip')).toBe('dst')
    expect(roleForField('subject.account.name')).toBe('actor')
    expect(roleForField('object.process.name')).toBe('object')
    expect(roleForField('correlation_name')).toBe('mentions')
  })
})

describe('groupEventFields', () => {
  it('treats MaxPatrol SIEM as a grouped source', () => {
    expect(isSiemSource('pt-maxpatrol-siem')).toBe(true)
    expect(isSiemSource('pt-nad')).toBe(false)
  })

  it('treats incidents and correlations as finding cards', () => {
    expect(isFindingRecord({ finding_kind: 'siem_incident' })).toBe(true)
    expect(isFindingRecord({ correlation_name: 'brute' })).toBe(true)
    expect(isFindingRecord({}, 'siem_correlation')).toBe(true)
    expect(isFindingRecord({ msgid: 'openat' })).toBe(false)
  })

  it('groups a SIEM event like the vendor card', () => {
    const groups = groupEventFields('pt-maxpatrol-siem', {
      time: '2025-04-01T12:09:58Z',
      text: 'open /etc/shadow',
      'event_src.vendor': 'unix_like',
      'event_src.title': 'unix_like',
      msgid: 'openat',
      'event_src.subsys': 'auditd',
      'category.generic': 'File System Object',
      'category.high': 'System Management',
      'category.low': 'Manipulation',
      subject: 'account',
      'subject.type': 'root',
      'subject.account.name': 'james',
      'subject.process.name': 'cat',
      object: 'file_object',
      'object.name': 'shadow',
      'object.fullpath': '/etc/shadow',
      importance: 'low',
      logon_service: 'pts/0',
      action: 'access',
      status: 'success',
      datafield1: 'cat',
      chain_id: '20183',
      'event_src.host': 'vuln-32.edtechlab.local',
      origin_app_name: 'MaxPatrol 10',
      recv_host: '',
      recv_time: '2026-06-19T19:54:41Z',
      id: 'PT_UNIX_like_auditd_syslog_structured',
      uuid: 'b5eaeea8-6c18-11f1-b241-d00d762d3dd7',
      normalized: 'true',
      raw: '{"action":"access"}',
    })
    expect(groups.map((group) => group.id)).toEqual([
      'roles',
      'interaction',
      'extra',
      'event-src',
      'collection',
      'service',
      'raw',
    ])
    const roles = groups.find((group) => group.id === 'roles')
    expect(roles?.columns.map((column) => column.title)).toEqual(['Субъект', 'Объект'])
    expect(roles?.columns[0]?.rows.map((row) => row.field)).toContain('subject.process.name')
    expect(groups.find((group) => group.id === 'extra')?.columns[0]?.rows).toEqual([
      { field: 'msgid', value: 'openat' },
      { field: 'datafield1', value: 'cat' },
      { field: 'chain_id', value: '20183' },
    ])
    expect(groups.find((group) => group.id === 'event-src')?.columns[0]?.rows.map((row) => row.field)).toEqual([
      'event_src.vendor',
      'event_src.title',
      'event_src.subsys',
      'event_src.host',
      'origin_app_name',
    ])
    expect(groups.find((group) => group.id === 'collection')?.columns[0]?.rows).toEqual([
      { field: 'recv_host', value: '' },
      { field: 'recv_time', value: '2026-06-19T19:54:41Z' },
    ])
    expect(groups.find((group) => group.id === 'service')?.columns[0]?.rows.map((row) => row.field)).toEqual([
      'id',
      'uuid',
      'normalized',
    ])
  })

  it('keeps process.parent fields on the subject/object, not a separate entity group', () => {
    const groups = groupEventFields('pt-maxpatrol-siem', {
      'object.process.name': 'cmd.exe',
      'object.process.parent.name': 'explorer.exe',
      'subject.account.name': 'apetrov$',
    })
    expect(groups.map((group) => group.id)).toEqual(['roles'])
    const roles = groups.find((group) => group.id === 'roles')
    expect(roles?.columns[1]?.rows.map((row) => row.field)).toEqual([
      'object.process.name',
      'object.process.parent.name',
    ])
  })

  it('uses a correlation schema instead of the event extra group', () => {
    expect(
      isCorrelationRecord({
        correlation_name: 'brute force',
        finding_kind: 'siem_correlation',
      }),
    ).toBe(true)
    const groups = groupEventFields('pt-maxpatrol-siem', {
      correlation_name: 'brute force',
      correlation_type: 'incident',
      'count.subevents': '4',
      'alert.key': 'k1',
      'src.host': 'kali',
      'dst.host': 'aamelina.plat.form',
      'subject.account.name': 'apetrov$',
      object: 'system',
      action: 'login',
      status: 'failure',
      'event_src.host': 'dc01',
      uuid: '11111111-1111-1111-1111-111111111111',
      datafield6: 'Network',
    })
    expect(groups.map((group) => group.id)).toEqual([
      'correlation',
      'roles',
      'addresses',
      'interaction',
      'event-src',
      'service',
      'extra',
    ])
    expect(groups.find((group) => group.id === 'correlation')?.columns[0]?.rows.map((row) => row.field)).toEqual([
      'correlation_name',
      'correlation_type',
      'count.subevents',
      'alert.key',
    ])
    expect(groups.find((group) => group.id === 'extra')?.columns[0]?.rows).toEqual([
      { field: 'datafield6', value: 'Network' },
    ])
  })

  it('lists unknown sources as a flat extra group', () => {
    const groups = groupEventFields('pt-nad', {
      'src.ip': '10.0.0.1',
      'dst.ip': '10.0.0.2',
    })
    expect(groups).toEqual([
      {
        id: 'extra',
        title: 'Дополнительная информация',
        columns: [
          {
            title: '',
            rows: [
              { field: 'src.ip', value: '10.0.0.1' },
              { field: 'dst.ip', value: '10.0.0.2' },
            ],
          },
        ],
      },
    ])
  })

  it('builds header meta as separate clickable values', () => {
    expect(
      eventHeaderMeta(
        {
          'event_src.vendor': 'unix_like',
          'event_src.title': 'unix_like',
          msgid: 'openat',
          'event_src.subsys': 'auditd',
          'category.generic': 'File System Object',
          'category.high': 'System Management',
          'category.low': 'Manipulation',
        },
        'pt-maxpatrol-siem',
      ),
    ).toEqual({
      source: [
        { field: 'event_src.vendor', value: 'unix_like' },
        { field: 'event_src.title', value: 'unix_like' },
      ],
      identifier: [
        { field: 'msgid', value: 'openat' },
        { field: 'event_src.subsys', value: 'auditd' },
      ],
      category: [
        { field: 'category.generic', value: 'File System Object' },
        { field: 'category.high', value: 'System Management' },
        { field: 'category.low', value: 'Manipulation' },
      ],
    })
  })
})
