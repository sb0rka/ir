import { describe, expect, it } from 'vitest'
import { groupWazuhFields, wazuhHeaderMeta } from './wazuhGroups'

describe('groupWazuhFields', () => {
  it('groups native Wazuh attributes into rule/agent/network sections', () => {
    const groups = groupWazuhFields({
      'rule.id': '5758',
      'rule.level': '8',
      'rule.groups': 'sshd,authentication_failed',
      'rule.description': 'Maximum authentication attempts exceeded.',
      correlation_type: 'wazuh_frequency',
      correlation_name: 'Maximum authentication attempts exceeded.',
      'agent.name': 'Windows',
      'agent.ip': '10.0.0.1',
      'data.srcip': '134.87.21.47',
      'data.srcport': '26874',
      'data.dstuser': 'SYSTEM',
      'syscheck.path': '/etc/passwd',
    })
    const ids = groups.map((g) => g.id)
    expect(ids).toContain('header')
    expect(ids).toContain('rule')
    expect(ids).toContain('agent')
    expect(ids).toContain('network')
    expect(ids).toContain('accounts')
    expect(ids).toContain('files')
    const rule = groups.find((g) => g.id === 'rule')!
    expect(rule.columns[0].rows.map((r) => r.field)).toEqual(
      expect.arrayContaining(['rule.level', 'correlation_type']),
    )
  })

  it('puts leftover fields into Прочее', () => {
    const groups = groupWazuhFields({ 'custom.field': 'x', 'rule.id': '1' })
    const other = groups.find((g) => g.id === 'other')
    expect(other?.columns[0].rows.map((r) => r.field)).toEqual(['custom.field'])
  })

  it('builds header meta from rule.id and rule.groups', () => {
    const meta = wazuhHeaderMeta({
      'rule.id': '550',
      'rule.groups': 'syscheck',
    })
    expect(meta.source[0]).toEqual({ field: 'source', value: 'wazuh' })
    expect(meta.identifier[0]?.value).toBe('550')
    expect(meta.category[0]?.value).toBe('syscheck')
  })
})
