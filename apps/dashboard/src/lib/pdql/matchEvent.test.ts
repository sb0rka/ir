import { describe, expect, it } from 'vitest'
import type { AlertEvent, Entity } from '../../types'
import { parse } from './parse'
import { alertMatchesPdql } from './matchEvent'
import type { QueryAst } from './model'

function mustParse(text: string): QueryAst {
  const result = parse(text)
  if (!result.ok) throw new Error(`${result.error.message} @${result.error.position}`)
  return result.ast
}

function alert(partial: Partial<AlertEvent> & Pick<AlertEvent, 'id'>): AlertEvent {
  return {
    time: '2025-10-23T12:00:00.000Z',
    severity: 'high',
    title: partial.id,
    rule: 'r',
    source: 'pt-maxpatrol-siem',
    status: 'new',
    entityIds: [],
    description: '',
    raw: {},
    ...partial,
  }
}

describe('alertMatchesPdql', () => {
  it('matches when the filter is empty', () => {
    expect(alertMatchesPdql(alert({ id: 'e1' }), mustParse('select(time)'))).toBe(true)
  })

  it('treats a finding UUID chip as already satisfied', () => {
    const ast = mustParse('filter(siem_incident = "inc-1" and action = "login") | select(time)')
    expect(alertMatchesPdql(alert({ id: 'ok', raw: { action: 'login' } }), ast)).toBe(true)
    expect(alertMatchesPdql(alert({ id: 'no', raw: { action: 'logout' } }), ast)).toBe(false)
  })

  it('evaluates AND / OR / NOT groups', () => {
    const ast = mustParse(
      'filter(action = "login" or (action = "fail" and not status = "closed")) | select(time)',
    )
    expect(alertMatchesPdql(alert({ id: 'a', raw: { action: 'login', status: 'closed' } }), ast)).toBe(true)
    expect(alertMatchesPdql(alert({ id: 'b', raw: { action: 'fail', status: 'new' } }), ast)).toBe(true)
    expect(alertMatchesPdql(alert({ id: 'c', raw: { action: 'fail', status: 'closed' } }), ast)).toBe(false)
  })

  it('matches entity fields against mentioned hosts', () => {
    const host: Entity = {
      id: 'host:dc01',
      kind: 'host',
      label: 'dc01',
      attributes: {},
    }
    const ast = mustParse('filter(event_src.host = "dc01") | select(time)')
    expect(
      alertMatchesPdql(alert({ id: 'hit', entityIds: ['host:dc01'] }), ast, { 'host:dc01': host }),
    ).toBe(true)
    expect(alertMatchesPdql(alert({ id: 'miss', entityIds: [] }), ast, { 'host:dc01': host })).toBe(false)
  })

  it('matches missing fields with is null', () => {
    const ast = mustParse('filter(action is null) | select(time)')
    expect(alertMatchesPdql(alert({ id: 'empty' }), ast)).toBe(true)
    expect(alertMatchesPdql(alert({ id: 'set', raw: { action: 'login' } }), ast)).toBe(false)
  })

  it('compares time against the event timestamp', () => {
    const ast = mustParse('filter(time >= "2025-10-23T12:00:00.000Z") | select(time)')
    expect(alertMatchesPdql(alert({ id: 'in', time: '2025-10-23T12:00:00.000Z' }), ast)).toBe(true)
    expect(alertMatchesPdql(alert({ id: 'out', time: '2025-10-22T12:00:00.000Z' }), ast)).toBe(false)
  })
})
