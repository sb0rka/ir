import { afterEach, describe, expect, it, vi } from 'vitest'
import { findingUuidQuery, pdqlToChips, parseQueuePdql } from '../lib/pdql'
import { filterFingerprint } from '../lib/queryFingerprint'
import { emptyContextQueue, useAppStore } from './appStore'
import type { AlertEvent, ContextEvent, Investigation, QueueItem } from '../types'
import * as irApi from '../api/ir'
import * as snapshots from '../api/eventQueueSnapshots'

const initial = {
  queuePdql: useAppStore.getState().queuePdql,
  queueSource: useAppStore.getState().queueSource,
  queueSourceCache: useAppStore.getState().queueSourceCache,
  groupValues: useAppStore.getState().groupValues,
  eventGroups: useAppStore.getState().eventGroups,
  executedFingerprint: useAppStore.getState().executedFingerprint,
  alerts: useAppStore.getState().alerts,
  correlations: useAppStore.getState().correlations,
  queueOrder: useAppStore.getState().queueOrder,
  mockSources: useAppStore.getState().mockSources,
  contextQueue: useAppStore.getState().contextQueue,
  investigations: useAppStore.getState().investigations,
  contextEvents: useAppStore.getState().contextEvents,
}

afterEach(() => {
  vi.restoreAllMocks()
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

  it('keeps current table rows when switching to events for a uuid chip', () => {
    const incidentAlert = alertStub('inc-1')
    const incidentOrder: QueueItem[] = [{ kind: 'alert', id: 'inc-1' }]
    useAppStore.setState({
      queueSource: 'siem_incident',
      alerts: { 'inc-1': incidentAlert },
      queueOrder: incidentOrder,
      inspectedQueueItem: { kind: 'alert', id: 'inc-1' },
      selectedAlertIds: ['inc-1'],
      executedFingerprint: 'stale-fp',
    })

    useAppStore.getState().filterByFindingUuid(null, 'inc-1', 'siem_incident')

    const state = useAppStore.getState()
    expect(state.queueSource).toBe('events')
    expect(state.queueOrder).toEqual(incidentOrder)
    expect(state.alerts['inc-1']?.id).toBe('inc-1')
    expect(state.inspectedQueueItem).toEqual({ kind: 'alert', id: 'inc-1' })
    expect(state.selectedAlertIds).toEqual(['inc-1'])
    expect(state.executedFingerprint).toBeNull()
    expect(state.queueSourceCache.siem_incident?.queueOrder).toEqual(incidentOrder)
  })

  it('keeps context queue rows when switching to events for a uuid chip', () => {
    const incidentAlert = alertStub('ctx-inc')
    useAppStore.setState({
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          queueSource: 'siem_incident',
          alerts: { 'ctx-inc': incidentAlert },
          queueOrder: [{ kind: 'alert', id: 'ctx-inc' }],
          selectedIds: ['ctx-inc'],
          executedFingerprint: 'stale-fp',
        },
      },
    })

    useAppStore.getState().filterByFindingUuid('inv-1', 'inc-9', 'siem_incident')

    const queue = useAppStore.getState().contextQueue['inv-1']
    expect(queue?.queueSource).toBe('events')
    expect(queue?.queueOrder).toEqual([{ kind: 'alert', id: 'ctx-inc' }])
    expect(queue?.alerts['ctx-inc']?.id).toBe('ctx-inc')
    expect(queue?.selectedIds).toEqual(['ctx-inc'])
    expect(queue?.executedFingerprint).toBeNull()
    expect(queue?.sourceResults.siem_incident?.queueOrder).toEqual([{ kind: 'alert', id: 'ctx-inc' }])
  })

  it('appends extra filters while a finding chip is set', () => {
    useAppStore.getState().filterByFindingUuid(null, 'inc-1', 'siem_incident')

    useAppStore.getState().appendPdqlFilter(null, 'action', 'login')

    const state = useAppStore.getState()
    expect(state.queueSource).toBe('events')
    expect(state.queuePdql).toContain('siem_incident = "inc-1"')
    expect(state.queuePdql).toContain('action = "login"')
  })

  it('keeps events source when appending a host filter beside a finding chip', () => {
    useAppStore.getState().filterByFindingUuid(null, 'inc-1', 'siem_incident')

    useAppStore.getState().appendPdqlFilter(null, 'host', 'aamelina')

    const state = useAppStore.getState()
    expect(state.queueSource).toBe('events')
    expect(state.queuePdql).toContain('siem_incident = "inc-1"')
    expect(state.queuePdql).toContain('host = "aamelina"')
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

describe('group selection vs PDQL grouping', () => {
  it('clears the selected group when grouping fields change', () => {
    useAppStore.setState({
      queuePdql: 'group(event_src.host) | select(event_src.host, count(), time)',
      groupValues: ['dc01'],
    })

    useAppStore.getState().setQueuePdql('group(action) | select(action, count(), time)')

    expect(useAppStore.getState().groupValues).toEqual([])
  })

  it('keeps the selected group when only filters change', () => {
    useAppStore.setState({
      queuePdql: 'group(event_src.host) | select(event_src.host, count(), time)',
      groupValues: ['dc01'],
    })

    useAppStore.getState().setQueuePdql(
      'filter(action = "login") | group(event_src.host) | select(event_src.host, count(), time)',
    )

    expect(useAppStore.getState().groupValues).toEqual(['dc01'])
  })

  it('clears the context queue selection when grouping fields change', () => {
    useAppStore.setState({
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          pdql: 'group(event_src.host) | select(event_src.host, count(), time)',
          groupValues: ['dc01'],
        },
      },
    })

    useAppStore.getState().setContextQueue('inv-1', {
      pdql: 'group(action) | select(action, count(), time)',
    })

    expect(useAppStore.getState().contextQueue['inv-1']?.groupValues).toEqual([])
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

describe('rememberQueueAlerts', () => {
  it('merges nested events into the global queue so actions can resolve them', () => {
    const nested = alertStub('pt-maxpatrol-siem/evt-9')
    useAppStore.setState({ alerts: { 'inc-1': alertStub('inc-1') } })

    useAppStore.getState().rememberQueueAlerts([nested])

    expect(useAppStore.getState().alerts['inc-1']?.id).toBe('inc-1')
    expect(useAppStore.getState().alerts[nested.id]).toEqual(nested)
  })

  it('also merges into the investigation context queue', () => {
    const nested = alertStub('pt-maxpatrol-siem/evt-9')
    useAppStore.setState({
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          alerts: { 'inc-1': alertStub('inc-1') },
        },
      },
    })

    useAppStore.getState().rememberQueueAlerts([nested], 'inv-1')

    expect(useAppStore.getState().alerts[nested.id]).toEqual(nested)
    expect(useAppStore.getState().contextQueue['inv-1']?.alerts[nested.id]).toEqual(nested)
    expect(useAppStore.getState().contextQueue['inv-1']?.alerts['inc-1']?.id).toBe('inc-1')
  })
})

function investigationStub(overrides: Partial<Investigation> = {}): Investigation {
  return {
    id: 'inv-1',
    title: 'Case',
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
    view: 'graph',
    selectedEntityIds: [],
    ...overrides,
  }
}

describe('event queue snapshots', () => {
  it('remembers the current context queue when adding events', async () => {
    const remember = vi.spyOn(snapshots, 'rememberEventQueueSnapshots')
    vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ eventIds: ['evt-1'] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    const alert = alertStub('evt-1')
    const pdql = 'filter(action = "login") | select(time) | sort(time desc)'
    useAppStore.setState({
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          pdql,
          queueSource: 'events',
          groupValues: ['host-a'],
          alerts: { 'evt-1': alert },
        },
      },
    })

    await useAppStore.getState().addEventsToContext('inv-1', ['evt-1'])

    expect(remember).toHaveBeenCalledWith(
      'inv-1',
      [alert],
      expect.objectContaining({
        pdql,
        queueSource: 'events',
        groupValues: ['host-a'],
      }),
    )
  })

  it('does not remember when there is nothing to add', async () => {
    const remember = vi.spyOn(snapshots, 'rememberEventQueueSnapshots')
    const addContext = vi.spyOn(irApi, 'addContext')

    await useAppStore.getState().addEventsToContext('inv-1', ['missing'])

    expect(remember).not.toHaveBeenCalled()
    expect(addContext).not.toHaveBeenCalled()
  })

  it('restores saved queue settings and switches the investigation to queue view', () => {
    vi.spyOn(snapshots, 'readEventQueueSnapshot').mockReturnValue({
      pdql: 'filter(action = "login") | select(time) | sort(time desc)',
      timeInterval: emptyContextQueue.timeInterval,
      queueSource: 'events',
      groupValues: ['host-a'],
    })
    useAppStore.setState({
      investigations: { 'inv-1': investigationStub() },
      contextQueue: { 'inv-1': { ...emptyContextQueue, pdql: 'select(time)' } },
    })

    const restored = useAppStore.getState().restoreEventQueue('inv-1', {
      source: 'pt-maxpatrol-siem',
      sourceEventId: 'evt-1',
    })

    expect(restored).toBe(true)
    const queue = useAppStore.getState().contextQueue['inv-1']
    expect(queue?.pdql).toBe('filter(action = "login") | select(time) | sort(time desc)')
    expect(queue?.queueSource).toBe('events')
    expect(queue?.groupValues).toEqual(['host-a'])
    expect(useAppStore.getState().investigations['inv-1']?.view).toBe('queue')
  })

  it('returns false when no snapshot exists', () => {
    vi.spyOn(snapshots, 'readEventQueueSnapshot').mockReturnValue(null)
    useAppStore.setState({ investigations: { 'inv-1': investigationStub() } })

    expect(
      useAppStore.getState().restoreEventQueue('inv-1', {
        source: 'pt-maxpatrol-siem',
        sourceEventId: 'evt-1',
      }),
    ).toBe(false)
    expect(useAppStore.getState().investigations['inv-1']?.view).toBe('graph')
  })

  it('remembers the global queue when starting an investigation from events', async () => {
    const remember = vi.spyOn(snapshots, 'rememberEventQueueSnapshots')
    vi.spyOn(irApi, 'createInvestigation').mockResolvedValue(investigationStub({ id: 'inv-new' }))
    vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ id: 'inv-new', eventIds: ['evt-1'] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    const alert = alertStub('evt-1')
    const pdql = 'filter(host = "dc01") | select(time) | sort(time desc)'
    useAppStore.setState({
      alerts: { 'evt-1': alert },
      queuePdql: pdql,
      queueSource: 'events',
      groupValues: ['dc01'],
    })

    await useAppStore.getState().startInvestigation(['evt-1'], 'Case')

    expect(remember).toHaveBeenCalledWith(
      'inv-new',
      [alert],
      expect.objectContaining({
        pdql,
        queueSource: 'events',
        groupValues: ['dc01'],
      }),
    )
  })
})

function findingAlert(id: string): AlertEvent {
  return {
    ...alertStub(id),
    findingRef: {
      source_code: 'pt-maxpatrol-siem',
      record_type: 'siem_incident',
      external_id: id,
      time_range: { from: '2026-01-01T00:00:00Z', to: '2026-01-02T00:00:00Z' },
    },
  }
}

function contextEventStub(id: string, entityIds: string[]): ContextEvent {
  return {
    id,
    time: '2026-01-01T00:00:00Z',
    severity: 'high',
    title: id,
    type: 'event',
    source: 'pt-maxpatrol-siem',
    entityIds,
    origin: 'analyst',
    isSeed: false,
    review: 'confirmed',
    description: '',
    sourceEventId: `src-${id}`,
  }
}

describe('context import options', () => {
  it('forwards expandFindings and why when adding findings', async () => {
    const addContext = vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ eventIds: [] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    const alert = findingAlert('inc-1')
    useAppStore.setState({
      contextQueue: {
        'inv-1': { ...emptyContextQueue, alerts: { 'inc-1': alert } },
      },
    })

    await useAppStore.getState().addEventsToContext('inv-1', ['inc-1'], {
      expandFindings: false,
      why: 'only the card',
    })

    expect(addContext).toHaveBeenCalledWith(
      'inv-1',
      expect.objectContaining({
        findings: [
          expect.objectContaining({
            source_code: 'pt-maxpatrol-siem',
            record_type: 'siem_incident',
            external_id: 'inc-1',
          }),
        ],
        expandFindings: false,
        why: 'only the card',
      }),
    )
  })

  it('omits expandFindings for ordinary events and still sends why', async () => {
    const addContext = vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ eventIds: ['evt-1'] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    useAppStore.setState({
      contextQueue: {
        'inv-1': { ...emptyContextQueue, alerts: { 'evt-1': alertStub('evt-1') } },
      },
    })

    await useAppStore.getState().addEventsToContext('inv-1', ['evt-1'], { why: 'suspicious login' })

    expect(addContext).toHaveBeenCalledWith('inv-1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'evt-1' }],
      findings: [],
      why: 'suspicious login',
    })
  })

  it('imports a correlation queue event as a finding so expandFindings applies', async () => {
    const addContext = vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ eventIds: [] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    const range = { from: '2025-10-23T00:00:00.000Z', to: '2025-10-24T00:00:00.000Z' }
    useAppStore.setState({
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          timeInterval: { kind: 'range', from: range.from, to: range.to },
          alerts: {
            'corr-1': {
              ...alertStub('corr-1'),
              raw: { correlation_name: 'WMI remote' },
            },
          },
        },
      },
    })

    await useAppStore.getState().addEventsToContext('inv-1', ['corr-1'], {
      expandFindings: true,
      why: 'load nested',
    })

    expect(addContext).toHaveBeenCalledWith(
      'inv-1',
      expect.objectContaining({
        events: [],
        findings: [
          expect.objectContaining({
            source_code: 'pt-maxpatrol-siem',
            record_type: 'siem_correlation',
            external_id: 'corr-1',
            time_range: range,
          }),
        ],
        expandFindings: true,
        why: 'load nested',
      }),
    )
  })

  it('forwards expandFindings when starting an investigation from a finding', async () => {
    const addContext = vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'createInvestigation').mockResolvedValue(investigationStub({ id: 'inv-new' }))
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ id: 'inv-new' }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    useAppStore.setState({ alerts: { 'inc-1': findingAlert('inc-1') } })

    await useAppStore.getState().startInvestigation(['inc-1'], 'Case', { expandFindings: true, why: 'seed' })

    expect(addContext).toHaveBeenCalledWith(
      'inv-new',
      expect.objectContaining({ expandFindings: true, seed: true, why: 'seed' }),
    )
  })

  it('does not send expandFindings when creating a child investigation', async () => {
    const addContext = vi.spyOn(irApi, 'addContext').mockResolvedValue(undefined)
    vi.spyOn(irApi, 'createInvestigation').mockResolvedValue(investigationStub({ id: 'inv-child' }))
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigationStub({ id: 'inv-child' }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    useAppStore.setState({
      investigations: {
        'inv-1': investigationStub({ eventIds: ['evt-1'], entityIds: ['ent-1'] }),
      },
      contextEvents: { 'evt-1': contextEventStub('evt-1', ['ent-1']) },
    })

    await useAppStore.getState().createChildInvestigation('inv-1', ['ent-1'])

    expect(addContext).toHaveBeenCalledWith('inv-child', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'src-evt-1' }],
      findings: [],
    })
  })
})
