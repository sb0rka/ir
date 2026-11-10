import { describe, expect, it } from 'vitest'
import { parseConfiguration } from './configuration'

describe('source JSON configuration', () => {
  it('preserves names and string values including whitespace and prototype names', () => {
    const result = parseConfiguration('[{"name":"pt-maxpatrol-siem","CorePortalCookie":"  exact  ","idsrv.session":"value","__proto__":"safe"},{"name":"pt-nad","sessionid":"nad"},{"name":"__proto__","custom":"safe"}]')
    expect(result['pt-maxpatrol-siem'].CorePortalCookie).toBe('  exact  ')
    expect(result['pt-maxpatrol-siem']['idsrv.session']).toBe('value')
    expect(result['pt-maxpatrol-siem'].__proto__).toBe('safe')
    expect(result['pt-nad'].sessionid).toBe('nad')
    expect(result.__proto__.custom).toBe('safe')
    expect(parseConfiguration('[]')).toEqual({})
  })
  it.each(['null','{}','[null]','[[]]','[{}]','[{"name":""}]','[{"name":"  "}]','[{"name":1}]','[{"name":"pt-nad","a":null}]','[{"name":"pt-nad","a":1}]','[{"name":"pt-nad","a":{}}]','[{"name":"pt-nad","a":[]}]','[{"name":"pt-nad","a":"x","\\u0061":"y"}]','[{"name":"pt-nad","\\u006eame":"other"}]','[{"name":"pt-nad"},{"name":"pt-\\u006ead"}]','[{"name":"pt-nad"},]','[{"name":"pt-nad",}]','[{"name":"pt-nad"}] {}'])('rejects invalid or ambiguous configuration: %s', (raw) => {
    expect(() => parseConfiguration(raw)).toThrow()
  })
  it('enforces the compact UTF-8 limit including escaped characters', () => {
    const prefix = '[{"name":"pt-nad","a":"'
    const suffix = '"}]'
    const available = 65536 - prefix.length - suffix.length
    expect(() => parseConfiguration(prefix + 'x'.repeat(available) + suffix)).not.toThrow()
    expect(() => parseConfiguration(prefix + 'x'.repeat(available + 1) + suffix)).toThrow('64 КиБ')
    expect(() => parseConfiguration(prefix + '&'.repeat(12000) + suffix)).not.toThrow()
    expect(() => parseConfiguration(prefix + '\\u0061'.repeat(12000) + suffix)).toThrow('64 КиБ')
    expect(() => parseConfiguration(prefix + 'я'.repeat(32768) + suffix)).toThrow('64 КиБ')
    expect(() => parseConfiguration('  \n[\n\t{"name":"pt-nad", "a" : " x y "}\n]  \n')).not.toThrow()
  })
  it('does not include secret fragments in validation errors', () => {
    expect(() => parseConfiguration('[{"name":"pt-nad","a":"private-fragment\\x"}]')).toThrow('Некорректная JSON-строка')
  })
})
