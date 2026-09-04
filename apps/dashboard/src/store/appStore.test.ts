import { afterEach, describe, expect, it } from 'vitest'
import { findingUuidQuery, pdqlToChips, parseQueuePdql } from '../lib/pdql'
import { filterFingerprint } from '../lib/queryFingerprint'
import { emptyContextQueue, useAppStore } from './appStore'
import type { AlertEvent, QueueItem } from '../types'

const initial = {
  queuePdql: useAppStore.getState().queuePdql,
  queueSource: useAppStore.getState().queueSource,
  queueSourceCache: useAppStore.getState().queueSourceCache,
  groupValues: useAppStore.getState().groupValues,
  eventGroups: useAppStore.getState().eventGroups,
  findingFilterWarnAt: useAppStore.getState().findingFilterWarnAt,
  executedFingerprint: useAppStore.getState().executedFingerprint,
  alerts: useAppStore.getState().alerts,
  correlations: useAppStore.getState().correlations,
  queueOrder: useAppStore.getState().queueOrder,
  mockSources: useAppStore.getState().mockSources,
  contextQueue: useAppStore.getState().contextQueue,
}

afterEach(() => {
  useAppStore.setState(initial)
})

function alertStub(id: string): AlertEvent {
  return {
    id,
    time: '2025-10-23T12:00:00.000Z',
    severity: 'high',
    title: id,
    rule: 'r',
    source: 'pt-maxpatrol-siem',
    status: 'new',
    entityIds: [],
    description: '',
    sourceEventId: id,
    raw: {},
  }
}

describe('filterByFindingUuid', () => {
  it('sets events source, drops other filters, and keeps an Incident/Correlation chip', () => {
    useAppStore.setState({
      queuePdql: 'filter(action = "login") | select(time) | sort(time desc)',
      queueSource: 'siem_incident',
      groupValues: ['dc01'],
      eventGroups: [{ source_code: 'pt-maxpatrol-siem', values: ['dc01'], count: 3 }],
    })

    useAppStore.getState().filterByFindingUuid(null, '  corr-uuid  ', 'siem_correlation')

    const state = useAppStore.getState()
    expect(state.queueSource).toBe('events')
    expect(state.queuePdql).toBe(findingUuidQuery('corr-uuid', 'siem_correlation'))
    expect(state.groupValues).toEqual([])
    expect(state.eventGroups).toEqual([])
    const parsed = parseQueuePdql(state.queuePdql)
    expect(parsed.ok).toBe(true)
    if (!parsed.ok) return
    expect(pdqlToChips(parsed.ast).filter((chip) => chip.kind === 'filter').map((chip) => chip.label)).toEqual([
      'Корреляция = "corr-uuid"',
    ])
  })

  it('updates the investigation context queue the same way', () => {
    useAppStore.getState().filterByFindingUuid('inv-1', 'inc-9', 'siem_incident')

    const queue = useAppStore.getState().contextQueue['inv-1']
    expect(queue?.queueSource).toBe('events')
    expect(queue?.pdql).toBe(findingUuidQuery('inc-9', 'siem_incident'))
    expect(queue?.groupValues).toEqual([])
    expect(queue?.timeInterval).toEqual(emptyContextQueue.timeInterval)
  })

  it('rejects extra filters and warns while a finding chip is set', () => {
    useAppStore.getState().filterByFindingUuid(null, 'inc-1', 'siem_incident')
    const pdql = useAppStore.getState().queuePdql
    const before = useAppStore.getState().findingFilterWarnAt

    useAppStore.getState().appendPdqlFilter(null, 'action', 'login')

    const state = useAppStore.getState()
    expect(state.queuePdql).toBe(pdql)
    expect(state.findingFilterWarnAt).toBeGreaterThan(before)
  })
})

describe('appendPdqlFilter entity fields', () => {
  it('switches queue source to entities for bare host filters', () => {
    useAppStore.setState({
      queuePdql: 'select(time) | sort(time desc)',
      queueSource: 'siem_correlation',
    })

    useAppStore.getState().appendPdqlFilter(null, 'host', 'aamelina')

    const state = useAppStore.getState()
    expect(state.queueSource).toBe('entities')
    expect(state.queuePdql).toContain('host = "aamelina"')
  })

  it('keeps events source for non-entity event fields', () => {
    useAppStore.setState({
      queuePdql: 'select(time) | sort(time desc)',
      queueSource: 'events',
    })

    useAppStore.getState().appendPdqlFilter(null, 'action', 'login')

    expect(useAppStore.getState().queueSource).toBe('events')
    expect(useAppStore.getState().queuePdql).toContain('action = "login"')
  })
})

describe('queue source result cache', () => {
  it('restores prior results when switching sources without refetch', () => {
    const incidentAlert = alertStub('inc-1')
    const corrAlert = alertStub('corr-1')
    const incidentOrder: QueueItem[] = [{ kind: 'alert', id: 'inc-1' }]
    const corrOrder: QueueItem[] = [{ kind: 'alert', id: 'corr-1' }]
    const pdql = useAppStore.getState().queuePdql
    const timeInterval = useAppStore.getState().timeInterval
    const incidentFp = filterFingerprint(pdql, timeInterval, 'siem_incident', [])
    const corrFp = filterFingerprint(pdql, timeInterval, 'siem_correlation', [])

    useAppStore.setState({
      queueSource: 'siem_incident',
      alerts: { 'inc-1': incidentAlert },
      queueOrder: incidentOrder,
      correlations: {},
      eventGroups: [],
      executedFingerprint: incidentFp,
      mockSources: [],
      queueSourceCache: {},
    })

    useAppStore.getState().setQueueSource('siem_correlation')
    expect(useAppStore.getState().queueOrder).toEqual([])
    expect(useAppStore.getState().executedFingerprint).toBeNull()
    expect(useAppStore.getState().queueSourceCache.siem_incident?.queueOrder).toEqual(incidentOrder)

    useAppStore.setState({
      alerts: { 'corr-1': corrAlert },
      queueOrder: corrOrder,
      executedFingerprint: corrFp,
      queueSourceCache: {
        ...useAppStore.getState().queueSourceCache,
        siem_correlation: {
          alerts: { 'corr-1': corrAlert },
          correlations: {},
          queueOrder: corrOrder,
          eventGroups: [],
          executedFingerprint: corrFp,
          mockSources: [],
        },
      },
    })

    useAppStore.getState().setQueueSource('siem_incident')
    const restored = useAppStore.getState()
    expect(restored.queueSource).toBe('siem_incident')
    expect(restored.queueOrder).toEqual(incidentOrder)
    expect(restored.alerts['inc-1']?.id).toBe('inc-1')
    // Cached rows stay, but fingerprint is cleared so "Выполнить · фильтр изменен" shows.
    expect(restored.executedFingerprint).toBeNull()
    expect(restored.queueSourceCache.siem_incident?.executedFingerprint).toBe(incidentFp)
  })

  it('keeps context queue results per source across toggles', () => {
    const incidentAlert = alertStub('ctx-inc')
    const fp = filterFingerprint(
      emptyContextQueue.pdql,
      emptyContextQueue.timeInterval,
      'siem_incident',
      [],
    )
    useAppStore.setState({
      contextQueue: {
        'inv-cache': {
          ...emptyContextQueue,
          alerts: { 'ctx-inc': incidentAlert },
          queueOrder: [{ kind: 'alert', id: 'ctx-inc' }],
          executedFingerprint: fp,
        },
      },
    })

    useAppStore.getState().setContextQueue('inv-cache', { queueSource: 'events' })
    let queue = useAppStore.getState().contextQueue['inv-cache']
    expect(queue?.queueSource).toBe('events')
    expect(queue?.queueOrder).toEqual([])
    expect(queue?.sourceResults.siem_incident?.queueOrder).toEqual([{ kind: 'alert', id: 'ctx-inc' }])

    useAppStore.getState().setContextQueue('inv-cache', { queueSource: 'siem_incident' })
    queue = useAppStore.getState().contextQueue['inv-cache']
    expect(queue?.queueOrder).toEqual([{ kind: 'alert', id: 'ctx-inc' }])
    expect(queue?.executedFingerprint).toBeNull()
    expect(queue?.sourceResults.siem_incident?.executedFingerprint).toBe(fp)
  })
})
