import { describe, expect, it } from 'vitest'
import { defaultQuery, withoutIds, type QueryAst } from './model'
import { parse } from './parse'
import { serialize } from './serialize'
import { pdqlToChips, serializeWithoutChip } from './chips'
import {
  alignGroupValues,
  astToEventAggregate,
  astToEventSearch,
  astToFilterChips,
  drillGroupValues,
  hasGroupValueSelection,
  pdqlToSearchParts,
  queueSelectFields,
} from './toSearch'
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
    const result = parse('window(10)')
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error.message).toMatch(/стадия/)
  })

  it('ignores limit and accepts a repeated sort after grouping', () => {
    const text =
      'filter(event_src.host = "dkrylova.plat.form") | select(time, event_src.host, text, object.process.cmdline) | sort(time asc) | group(key: [action], agg: COUNT(*) as Cnt) | sort(Cnt desc) | limit(10000)'
    expect(serialize(mustParse(text))).toBe(
      'filter(event_src.host = "dkrylova.plat.form") | group(action) | select(action, time, event_src.host, text, object.process.cmdline, count()) | sort(time asc, count() desc)',
    )
  })

  it('parses named group keys without brackets', () => {
    expect(serialize(mustParse('group(key: action, agg: count()) | select(action, count())'))).toBe(
      'group(action) | select(action, count())',
    )
  })

  it('treats group(key) as a field name', () => {
    expect(serialize(mustParse('group(key) | select(key, count())'))).toBe('group(key) | select(key, count())')
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

describe('astToEventSearch', () => {
  it('keeps event-level sort from a MaxPatrol grouped query', () => {
    const ast = mustParse(
      'filter(event_src.host = "dkrylova.plat.form") | select(time, event_src.host, text, object.process.cmdline) | sort(time asc) | group(key: [action], agg: COUNT(*) as Cnt) | sort(Cnt desc) | limit(10000)',
    )
    expect(astToEventSearch(ast)).toEqual({
      filter: 'event_src.host = "dkrylova.plat.form"',
      sort: [{ field: 'time', direction: 'asc' }],
      hasControls: true,
    })
    expect(
      astToEventSearch(ast, ['create']),
    ).toMatchObject({
      group_by: ['action'],
      group_values: ['create'],
    })
  })

  it('puts the full predicate in filter, including entity fields', () => {
    const ast = mustParse(
      'filter(event_src.host = "dc01" and action = "login" or not src.ip = "8.8.8.8") | select(time) | sort(time desc)',
    )
    expect(astToEventSearch(ast)).toEqual({
      filter: 'event_src.host = "dc01" and action = "login" or not src.ip = "8.8.8.8"',
      hasControls: true,
    })
  })

  it('sends non-default sort without aggregates and omits columns', () => {
    const ast = mustParse(
      'group(event_src.host) | select(event_src.host, uniq(src.ip), count(), time) | sort(time desc, event_src.host asc)',
    )
    expect(astToEventSearch(ast)).toEqual({
      sort: [
        { field: 'time', direction: 'desc' },
        { field: 'event_src.host', direction: 'asc' },
      ],
      hasControls: true,
    })
  })

  it('sends group_by only after a group value is selected', () => {
    const ast = mustParse('group(event_src.host, action) | select(event_src.host, action, time)')
    expect(astToEventSearch(ast)).toEqual({
      hasControls: false,
    })
    expect(astToEventSearch(ast, ['dc01'])).toEqual({
      group_by: ['event_src.host'],
      group_values: ['dc01'],
      hasControls: true,
    })
    expect(astToEventSearch(ast, ['dc01', 'login'])).toEqual({
      group_by: ['event_src.host', 'action'],
      group_values: ['dc01', 'login'],
      hasControls: true,
    })
  })

  it('sends JSON null as the source null group, not as unselected', () => {
    const ast = mustParse('group(event_src.host) | select(time)')
    expect(astToEventSearch(ast, [])).toEqual({ hasControls: false })
    expect(astToEventSearch(ast, [null])).toEqual({
      group_by: ['event_src.host'],
      group_values: [null],
      hasControls: true,
    })
  })
})

describe('alignGroupValues', () => {
  it('keeps an empty selection empty and preserves explicit null', () => {
    const ast = mustParse('group(event_src.host) | select(time)')
    expect(alignGroupValues(ast, undefined)).toEqual([])
    expect(alignGroupValues(ast, [])).toEqual([])
    expect(alignGroupValues(ast, ['dc01'])).toEqual(['dc01'])
    expect(alignGroupValues(ast, [null])).toEqual([null])
    expect(hasGroupValueSelection([])).toBe(false)
    expect(hasGroupValueSelection([null])).toBe(true)
  })

  it('drops values when groups are removed from the query', () => {
    const ast = mustParse('select(time)')
    expect(alignGroupValues(ast, ['dc01'])).toEqual([])
  })
})

describe('astToEventAggregate', () => {
  it('returns undefined without groups', () => {
    expect(astToEventAggregate(mustParse('select(time) | sort(time desc)'))).toBeUndefined()
  })

  it('uses the first group field, filter, and count sort', () => {
    const ast = mustParse(
      'filter(action = "login") | group(event_src.host) | select(event_src.host, count()) | sort(count() desc)',
    )
    expect(astToEventAggregate(ast)).toEqual({
      filter: 'action = "login"',
      group_by: ['event_src.host'],
      sort: [{ field: 'count', direction: 'desc' }],
    })
  })

  it('ignores extra groups beyond the first', () => {
    expect(astToEventAggregate(mustParse('group(event_src.host, action) | select(time)'))).toEqual({
      group_by: ['event_src.host'],
    })
  })
})

describe('queueSelectFields', () => {
  it('omits time from a default select', () => {
    expect(queueSelectFields(mustParse('select(time) | sort(time desc)'))).toEqual([])
  })

  it('keeps extra select fields in order', () => {
    expect(
      queueSelectFields(mustParse('select(time, event_src.host, text) | sort(time desc)')),
    ).toEqual(['event_src.host', 'text'])
  })

  it('includes group dimensions and skips aggregates', () => {
    expect(
      queueSelectFields(
        mustParse('group(event_src.host) | select(event_src.host, uniq(src.ip), count(), time)'),
      ),
    ).toEqual(['event_src.host'])
  })

  it('puts group keys before extra columns', () => {
    expect(
      queueSelectFields(
        mustParse(
          'filter(event_src.host = "dkrylova.plat.form") | select(time, event_src.host, text, object.process.cmdline) | sort(time asc) | group(key: [action], agg: COUNT(*) as Cnt)',
        ),
      ),
    ).toEqual(['action', 'event_src.host', 'text', 'object.process.cmdline'])
  })
})

describe('drillGroupValues', () => {
  it('sets the field and omits unselected deeper levels', () => {
    const ast = mustParse('group(event_src.host, action) | select(time)')
    expect(drillGroupValues(ast, ['dc01', 'login'], 'event_src.host', 'ws01')).toEqual(['ws01'])
    expect(drillGroupValues(ast, ['dc01'], 'action', 'login')).toEqual(['dc01', 'login'])
    expect(drillGroupValues(ast, [], 'src.ip', '1.1.1.1')).toBeNull()
  })
})

