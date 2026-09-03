import { describe, expect, it } from 'vitest'
import {
  alertMatchesQueueText,
  correlationMatchesQueueText,
  queueSearchColumns,
  resolveQueueSearchColumn,
} from './queueTextSearch'
import { investigationMatchesText } from './investigationTextSearch'
import { INVESTIGATION_TABLE_SEARCH_COLUMNS } from '../components/investigationTableColumns'
import type { AlertEvent, CorrelationGroup, Investigation } from '../types'

const alert: AlertEvent = {
  id: 'a1',
  time: '2025-10-23T12:34:56Z',
  severity: 'high',
  title: 'Suspicious login',
  rule: 'auth.brute_force',
  source: 'siem',
  status: 'new',
  entityIds: [],
  description: 'Multiple failures',
  raw: { 'event_src.host': 'host-42' },
}

const group: CorrelationGroup = {
  id: 'c1',
  title: 'Cluster',
  reason: 'same host',
  severity: 'critical',
  time: '2025-10-23T12:00:00Z',
  status: 'new',
  sourceCounts: { siem: 2, edr: 1 },
  eventIds: [],
  entityIds: [],
}

const investigation: Investigation = {
  id: 'inv-1',
  title: 'Phishing case',
  severity: 'high',
  status: 'open',
  assignee: 'аналитик',
  seedEventIds: [],
  eventIds: [],
  entityIds: [],
  nodeIds: [],
  edgeIds: [],
  findingIds: [],
  findingSourceKeys: [],
  issueIds: [],
  hypothesisIds: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-02T12:34:00Z',
  description: 'User reported mail',
  counters: {
    children: 0,
    findings: 0,
    sessions: 0,
    events: 0,
    entities: 0,
    proposed_edges: 0,
  },
  view: 'graph',
  selectedEntityIds: [],
}

describe('queueSearchColumns', () => {
  it('follows alert table column order and labels', () => {
    expect(queueSearchColumns(['event_src.host'])).toEqual([
      { id: 'severity', label: 'Крит.' },
      { id: 'time', label: 'Время' },
      { id: 'title', label: 'Название' },
      { id: 'field:event_src.host', label: 'event_src.host' },
      { id: 'source', label: 'Источник' },
    ])
  })

  it('inserts category between title and select fields for incidents', () => {
    expect(queueSearchColumns(['event_src.host'], { showCategory: true })).toEqual([
      { id: 'severity', label: 'Крит.' },
      { id: 'time', label: 'Время' },
      { id: 'title', label: 'Название' },
      { id: 'category', label: 'Категория' },
      { id: 'field:event_src.host', label: 'event_src.host' },
      { id: 'source', label: 'Источник' },
    ])
  })
})

describe('resolveQueueSearchColumn', () => {
  it('falls back to title when column disappears', () => {
    expect(resolveQueueSearchColumn('field:gone', queueSearchColumns([]))).toBe('title')
  })
})

describe('alertMatchesQueueText', () => {
  it('filters by selected column', () => {
    expect(alertMatchesQueueText(alert, 'host-42', 'field:event_src.host')).toBe(true)
    expect(alertMatchesQueueText(alert, 'host-42', 'title')).toBe(false)
    expect(alertMatchesQueueText(alert, 'siem', 'source')).toBe(true)
    expect(alertMatchesQueueText(alert, 'high', 'severity')).toBe(true)
  })

  it('matches incident category by code and Russian label', () => {
    const incident: AlertEvent = {
      ...alert,
      raw: { 'incident.type': 'InfectionAttempt' },
    }
    expect(alertMatchesQueueText(incident, 'infectionattempt', 'category')).toBe(true)
    expect(alertMatchesQueueText(incident, 'внедрения', 'category')).toBe(true)
    expect(alertMatchesQueueText(incident, 'ddos', 'category')).toBe(false)
  })
})

describe('correlationMatchesQueueText', () => {
  it('filters correlation rows by column', () => {
    expect(correlationMatchesQueueText(group, 'cluster', 'title')).toBe(true)
    expect(correlationMatchesQueueText(group, 'edr', 'source')).toBe(true)
    expect(correlationMatchesQueueText(group, 'host-42', 'source')).toBe(false)
  })
})

describe('investigation table search columns', () => {
  it('matches table headers', () => {
    expect(INVESTIGATION_TABLE_SEARCH_COLUMNS.map((c) => c.label)).toEqual([
      'Крит.',
      'Статус',
      'Название',
      'Вердикт',
      'Создано',
      'Обновлено',
    ])
  })

  it('filters by selected column', () => {
    expect(investigationMatchesText(investigation, 'phishing', 'title')).toBe(true)
    expect(investigationMatchesText(investigation, 'high', 'severity')).toBe(true)
    expect(investigationMatchesText(investigation, 'открыт', 'status')).toBe(true)
    expect(investigationMatchesText(investigation, 'phishing', 'severity')).toBe(false)
  })
})
