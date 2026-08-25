import { describe, expect, it } from 'vitest'
import { defaultQuery, withoutIds, type QueryAst } from './model'
import { parse } from './parse'
import { serialize } from './serialize'
import { pdqlToChips, serializeWithoutChip } from './chips'
import { astToFilterChips, pdqlToSearchParts } from './toSearch'
import { addFieldToAst, addFieldToPdql, setGroupAggregate } from './ast'
import { appendCondition } from './append'
import { relatedFieldColumns } from './relatedFields'

function mustParse(text: string): QueryAst {
  const result = parse(text)
  if (!result.ok) throw new Error(`${result.error.message} @${result.error.position}`)
  return result.ast
}

function roundTrip(ast: QueryAst) {
  const text = serialize(ast)
  const again = mustParse(text)
  expect(withoutIds(again)).toEqual(withoutIds(ast))
  expect(serialize(again)).toBe(text)
}

describe('serialize', () => {
  it('emits default time column and sort', () => {
    expect(serialize(defaultQuery())).toBe('select(time) | sort(time desc)')
  })

  it('emits filter joiners, not, in, and null checks', () => {
    const ast: QueryAst = {
      filter: [
        { id: '1', field: 'action', op: '=', value: 'login', values: [], negated: false },
        { id: '2', field: 'event_src.host', op: 'contains', value: 'dc', values: [], negated: true },
        { id: '3', field: 'src.ip', op: 'in', value: '', values: ['10.0.0.1', '10.0.0.2'], negated: false },
        { id: '4', field: 'dst.port', op: 'is_null', value: '', values: [], negated: false },
      ],
      joiners: ['and', 'or', 'and'],
      columns: [{ id: 'c', field: 'time' }],
      groups: [],
    }
    expect(serialize(ast)).toBe(
      'filter(action = "login" and not event_src.host contains "dc" or src.ip in ("10.0.0.1", "10.0.0.2") and dst.port is null) | select(time)',
    )
  })

  it('emits group, aggregates, and sort priorities', () => {
    const ast: QueryAst = {
      filter: [],
      joiners: [],
      columns: [
        { id: '1', field: 'event_src.host' },
        { id: '2', field: 'src.ip', aggregate: 'uniq', sort: { dir: 'desc', priority: 2 } },
        { id: '3', field: 'time', aggregate: 'count', sort: { dir: 'desc', priority: 1 } },
      ],
      groups: [{ id: 'g', field: 'event_src.host' }],
    }
    expect(serialize(ast)).toBe(
      'group(event_src.host) | select(event_src.host, uniq(src.ip), count(time)) | sort(count(time) desc, uniq(src.ip) desc)',
    )
  })

  it('does not duplicate group fields already present in select', () => {
    expect(
      serialize({
        filter: [],
        joiners: [],
        columns: [
          { id: '1', field: 'action' },
          { id: '2', field: 'time', aggregate: 'count' },
        ],
        groups: [{ id: 'g', field: 'action' }],
      }),
    ).toBe('group(action) | select(action, count(time))')
  })
})

describe('parse', () => {
  it('round-trips the default query', () => {
    roundTrip(defaultQuery())
  })

  it('round-trips a grouped aggregate query', () => {
    roundTrip({
      filter: [
        { id: 'a', field: 'action', op: '=', value: 'login', values: [], negated: false },
        { id: 'b', field: 'text', op: 'startswith', value: 'fail', values: [], negated: true },
      ],
      joiners: ['and'],
      columns: [
        { id: 'c1', field: 'event_src.host' },
        { id: 'c2', field: 'src.ip', aggregate: 'uniq' },
        { id: 'c3', field: 'time', aggregate: 'count', sort: { dir: 'desc', priority: 1 } },
      ],
      groups: [{ id: 'g', field: 'event_src.host' }],
    })
  })

  it('round-trips group fields injected into select without duplicating them', () => {
    const text = 'group(action) | select(action, count(time)) | sort(count(time) desc)'
    expect(serialize(mustParse(text))).toBe(text)
  })

  it('round-trips count() without a field', () => {
    const text = 'group(action) | select(action, count()) | sort(count() desc)'
    expect(serialize(mustParse(text))).toBe(text)
  })

  it('accepts whitespace and attaches sort to existing columns', () => {
    const ast = mustParse(`
      filter(importance = "high")
      | select(time, event_src.host, text)
      | sort(time desc)
    `)
    expect(ast.columns.map((column) => column.field)).toEqual(['time', 'event_src.host', 'text'])
    expect(ast.columns[0]?.sort).toEqual({ dir: 'desc', priority: 1 })
  })

  it('returns a positioned error for a broken filter', () => {
    const result = parse('filter(action = )')
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.position).toBeGreaterThan(0)
    expect(result.error.message).toMatch(/значение/)
  })

  it('rejects an unknown stage', () => {
    const result = parse('limit(10)')
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.message).toMatch(/стадия/)
  })
})

describe('pdqlToSearchParts', () => {
  it('maps equality and in conditions on entity fields', () => {
    const ast = mustParse(
      'filter(event_src.host = "dc01" and src.ip in ("10.0.0.1", "10.0.0.2") and subject.account.name = "alice" and object.process.name = "cmd.exe")',
    )
    expect(pdqlToSearchParts(ast)).toEqual({
      entities: [
        { type: 'host', value: 'dc01' },
        { type: 'ip', value: '10.0.0.1' },
        { type: 'ip', value: '10.0.0.2' },
        { type: 'user', value: 'alice' },
        { type: 'process', value: 'cmd.exe' },
      ],
      query: '',
    })
  })

  it('leaves non-entity and negated conditions in the query string', () => {
    const ast = mustParse(
      'filter(action = "login" and not event_src.host = "skip" or text contains "fail")',
    )
    expect(pdqlToSearchParts(ast)).toEqual({
      entities: [],
      query: 'action = "login" and not event_src.host = "skip" or text contains "fail"',
    })
  })

  it('splits mapped entities from remaining filter text', () => {
    const ast = mustParse(
      'filter(event_src.host = "dc01" and action = "login" and dst.ip = "8.8.8.8")',
    )
    expect(pdqlToSearchParts(ast)).toEqual({
      entities: [
        { type: 'host', value: 'dc01' },
        { type: 'ip', value: '8.8.8.8' },
      ],
      query: 'action = "login"',
    })
  })
})

describe('pdqlToChips', () => {
  it('emits filter, group, and select chips', () => {
    const ast = mustParse(
      'filter(action = "login" and event_src.host = "dc01") | group(event_src.host) | select(event_src.host, time) | sort(time desc)',
    )
    expect(pdqlToChips(ast).map((chip) => chip.label)).toEqual([
      'action = "login"',
      'event_src.host = "dc01"',
      'group event_src.host',
      'time desc',
    ])
  })

  it('removing a filter chip keeps the remaining query', () => {
    const ast = mustParse('filter(action = "login" and event_src.host = "dc01") | select(time)')
    const host = ast.filter.find((condition) => condition.field === 'event_src.host')
    expect(host).toBeTruthy()
    expect(serializeWithoutChip(ast, host!.id)).toBe('filter(action = "login") | select(time)')
  })

  it('removing a group chip restores the field as a column', () => {
    const ast = mustParse('group(action) | select(action, count(time))')
    const group = ast.groups[0]
    expect(group).toBeTruthy()
    expect(serializeWithoutChip(ast, group!.id)).toBe('select(action, time)')
  })

  it('hides group count() from chips and restores it as time', () => {
    const ast = mustParse('group(action) | select(action, count()) | sort(count() desc)')
    expect(pdqlToChips(ast).map((chip) => chip.label)).toEqual(['group action'])
    const group = ast.groups[0]
    expect(group).toBeTruthy()
    expect(serializeWithoutChip(ast, group!.id)).toBe('select(action, time) | sort(time desc)')
  })
})

describe('appendCondition', () => {
  it('creates a filter from an empty query', () => {
    expect(appendCondition('', 'src.ip', '=', '10.0.0.1')).toBe('filter(src.ip = "10.0.0.1")')
  })

  it('ands onto an existing filter and keeps other stages', () => {
    expect(
      appendCondition('filter(action = "login") | select(time)', 'dst.ip', '!=', '8.8.8.8'),
    ).toBe('filter(action = "login" and dst.ip != "8.8.8.8") | select(time)')
  })

  it('replaces a broken query with the new condition', () => {
    expect(appendCondition('filter(action = )', 'action', '=', 'login')).toBe(
      'filter(action = "login")',
    )
  })
})

describe('relatedFieldColumns', () => {
  it('pairs src/dst host with event_src', () => {
    expect(relatedFieldColumns('src.host')).toEqual([
      { title: 'Источник', fields: ['src.host', 'event_src.host'] },
      { title: 'Назначение', fields: ['dst.host'] },
    ])
  })

  it('pairs process name with parent', () => {
    expect(relatedFieldColumns('object.process.name')).toEqual([
      { title: 'Процесс', fields: ['object.process.name', 'subject.process.name'] },
      { title: 'Родитель', fields: ['object.process.parent.name'] },
    ])
  })

  it('pairs subject and object accounts', () => {
    expect(relatedFieldColumns('subject.account.name')).toEqual([
      { title: 'Субъект', fields: ['subject.account.name'] },
      { title: 'Объект', fields: ['object.account.name'] },
    ])
  })

  it('pairs process and file hashes', () => {
    expect(relatedFieldColumns('object.process.hash.md5')).toEqual([
      { title: 'Процесс', fields: ['subject.process.hash.md5', 'object.process.hash.md5'] },
      { title: 'Файл', fields: ['object.file.hash.md5'] },
    ])
  })

  it('falls back to the clicked field when there are no relations', () => {
    expect(relatedFieldColumns('action')).toEqual([{ title: 'Поле', fields: ['action'] }])
  })
})

describe('addFieldToPdql', () => {
  it('adds a filter condition onto the default query', () => {
    expect(addFieldToPdql('select(time)', 'src.ip', 'filter')).toBe(
      'filter(src.ip = "") | select(time)',
    )
  })

  it('adds a select column and a group field', () => {
    expect(addFieldToPdql('select(time)', 'event_src.host', 'columns')).toBe(
      'select(time, event_src.host)',
    )
    expect(addFieldToPdql('select(time)', 'event_src.host', 'groups')).toBe(
      'group(event_src.host) | select(event_src.host, count(), time)',
    )
  })

  it('moves an existing select field into group without duplicating the column', () => {
    const ast = addFieldToAst(
      {
        filter: [],
        joiners: [],
        columns: [
          { id: 'c1', field: 'time' },
          { id: 'c2', field: 'action' },
        ],
        groups: [],
      },
      'action',
      'groups',
    )
    expect(ast.groups.map((group) => group.field)).toEqual(['action'])
    expect(ast.columns.map((column) => ({ field: column.field, aggregate: column.aggregate }))).toEqual([
      { field: '', aggregate: 'count' },
      { field: 'time', aggregate: undefined },
    ])
    expect(serialize(ast)).toBe('group(action) | select(action, count(), time)')
  })

  it('keeps extra measure columns and changes the group aggregate', () => {
    expect(addFieldToPdql('select(time, src.ip)', 'action', 'groups')).toBe(
      'group(action) | select(action, count(), time, count(src.ip))',
    )
    const ast = addFieldToAst(
      {
        filter: [],
        joiners: [],
        columns: [{ id: 'c1', field: 'time', sort: { dir: 'desc', priority: 1 } }],
        groups: [],
      },
      'action',
      'groups',
    )
    expect(serialize(setGroupAggregate(ast, 'uniq'))).toBe(
      'group(action) | select(action, uniq(), time) | sort(time desc)',
    )
  })

  it('keeps the default time column when grouping', () => {
    const ast = addFieldToAst(defaultQuery(), 'action', 'groups')
    expect(serialize(ast)).toBe('group(action) | select(action, count(), time) | sort(time desc)')
    expect(serializeWithoutChip(ast, ast.groups[0]!.id)).toBe('select(action, time) | sort(time desc)')
  })
})

describe('astToFilterChips', () => {
  it('turns mapped entity conditions into queue chips', () => {
    const ast = mustParse(
      'filter(event_src.host = "dc01" and src.ip in ("10.0.0.1", "10.0.0.2")) | select(time)',
    )
    expect(astToFilterChips(ast)).toEqual([
      { id: 'pdql-host', field: 'host', values: ['dc01'] },
      { id: 'pdql-ip', field: 'ip', values: ['10.0.0.1', '10.0.0.2'] },
    ])
  })
})

