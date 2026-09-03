import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Investigation } from '../types'
import {
  DEFAULT_INVESTIGATION_FILTER,
  useAppStore,
} from './appStore'
import * as irApi from '../api/ir'
import * as searchApi from '../api/search'

function investigation(overrides: Partial<Investigation> = {}): Investigation {
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
    updatedAt: '2026-01-02T00:00:00Z',
    counters: {
      children: 1,
      findings: 0,
      sessions: 0,
      events: 2,
      entities: 1,
      proposed_edges: 0,
    },
    view: 'graph',
    selectedEntityIds: [],
    ...overrides,
  }
}

const snapshot = {
  tabs: ['queue', 'investigations'] as const,
  activeTab: 'queue' as const,
  investigations: {},
  investigationRootIds: [] as string[],
  investigationsNextCursor: null as string | null,
  investigationsLoading: false,
  investigationFilters: DEFAULT_INVESTIGATION_FILTER,
  expandedInvestigationIds: [] as string[],
  investigationChildren: {} as Record<string, string[]>,
  investigationChildrenLoading: {} as Record<string, boolean>,
  investigationDeletingId: null as string | null,
  lastError: null as string | null,
}

afterEach(() => {
  vi.restoreAllMocks()
  useAppStore.setState({ ...snapshot, investigations: {}, investigationChildren: {} })
})

describe('investigation catalog', () => {
  beforeEach(() => {
    useAppStore.setState({ ...snapshot, investigations: {}, investigationChildren: {} })
  })

  it('bootstrap lists roots without opening workspace tabs', async () => {
    const root = investigation()
    vi.spyOn(irApi, 'listInvestigations').mockResolvedValue({
      items: [root],
      nextCursor: 'next',
    })
    vi.spyOn(searchApi, 'searchQueue').mockResolvedValue({
      alerts: {},
      correlations: {},
      queueOrder: [],
      entities: {},
      contextEvents: {},
      eventGroups: [],
      sourceErrors: [],
      availableSources: [],
      mockSources: [],
    })

    await useAppStore.getState().bootstrap()

    const state = useAppStore.getState()
    expect(state.tabs).toEqual(['queue', 'investigations'])
    expect(state.investigationRootIds).toEqual(['inv-1'])
    expect(state.investigations['inv-1']?.title).toBe('Case')
    expect(state.investigationsNextCursor).toBe('next')
  })

  it('openInvestigationTab adds a workspace tab once and activates it', () => {
    useAppStore.getState().openInvestigationTab('inv-1')
    useAppStore.getState().openInvestigationTab('inv-1')
    expect(useAppStore.getState().tabs).toEqual(['queue', 'investigations', 'inv-1'])
    expect(useAppStore.getState().activeTab).toBe('inv-1')
  })

  it('closeTab keeps pinned catalog and queue tabs', () => {
    useAppStore.setState({ tabs: ['queue', 'investigations', 'inv-1'], activeTab: 'inv-1' })
    useAppStore.getState().closeTab('investigations')
    useAppStore.getState().closeTab('queue')
    expect(useAppStore.getState().tabs).toEqual(['queue', 'investigations', 'inv-1'])
    useAppStore.getState().closeTab('inv-1')
    expect(useAppStore.getState().tabs).toEqual(['queue', 'investigations'])
    expect(useAppStore.getState().activeTab).toBe('investigations')
  })

  it('toggleInvestigationExpand loads children for a parent', async () => {
    const parent = investigation()
    const child = investigation({
      id: 'inv-2',
      title: 'Child',
      parentId: 'inv-1',
      counters: {
        children: 0,
        findings: 0,
        sessions: 0,
        events: 1,
        entities: 0,
        proposed_edges: 0,
      },
    })
    useAppStore.setState({
      investigations: { 'inv-1': parent },
      investigationRootIds: ['inv-1'],
    })
    vi.spyOn(irApi, 'listInvestigations').mockResolvedValue({
      items: [child],
      nextCursor: null,
    })

    await useAppStore.getState().toggleInvestigationExpand('inv-1')

    const state = useAppStore.getState()
    expect(state.expandedInvestigationIds).toEqual(['inv-1'])
    expect(state.investigationChildren['inv-1']).toEqual(['inv-2'])
    expect(state.investigations['inv-2']?.title).toBe('Child')
    expect(irApi.listInvestigations).toHaveBeenCalledWith(
      expect.objectContaining({ parentId: 'inv-1' }),
    )
  })

  it('loadInvestigationList preserves graph fields of an open case', async () => {
    useAppStore.setState({
      investigations: {
        'inv-1': investigation({ nodeIds: ['n1'], view: 'table', selectedEntityIds: ['e1'] }),
      },
    })
    vi.spyOn(irApi, 'listInvestigations').mockResolvedValue({
      items: [investigation({ title: 'Renamed', nodeIds: [] })],
      nextCursor: null,
    })

    await useAppStore.getState().loadInvestigationList(true)

    const inv = useAppStore.getState().investigations['inv-1']
    expect(inv?.title).toBe('Renamed')
    expect(inv?.nodeIds).toEqual(['n1'])
    expect(inv?.view).toBe('table')
    expect(inv?.selectedEntityIds).toEqual(['e1'])
  })

  it('deleteInvestigation removes the case, children and open tabs', async () => {
    const parent = investigation()
    const child = investigation({ id: 'inv-2', title: 'Child', parentId: 'inv-1' })
    useAppStore.setState({
      investigations: { 'inv-1': parent, 'inv-2': child },
      investigationRootIds: ['inv-1'],
      investigationChildren: { 'inv-1': ['inv-2'] },
      expandedInvestigationIds: ['inv-1'],
      tabs: ['queue', 'investigations', 'inv-1', 'inv-2'],
      activeTab: 'inv-2',
    })
    vi.spyOn(irApi, 'deleteInvestigation').mockResolvedValue()

    await useAppStore.getState().deleteInvestigation('inv-1')

    const state = useAppStore.getState()
    expect(irApi.deleteInvestigation).toHaveBeenCalledWith('inv-1')
    expect(state.investigations['inv-1']).toBeUndefined()
    expect(state.investigations['inv-2']).toBeUndefined()
    expect(state.investigationRootIds).toEqual([])
    expect(state.tabs).toEqual(['queue', 'investigations'])
    expect(state.activeTab).toBe('investigations')
    expect(state.investigationDeletingId).toBeNull()
  })

  it('persistInvestigation closes a case with a verdict', async () => {
    useAppStore.setState({
      investigations: { 'inv-1': investigation({ version: 1 }) },
    })
    vi.spyOn(irApi, 'patchInvestigation').mockResolvedValue({
      id: 'inv-1',
      project_id: 'proj',
      title: 'Case',
      status: 'closed',
      verdict: 'incident',
      verdict_reason: 'c2 confirmed',
      version: 2,
      som_workspace_ids: [],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-03T00:00:00Z',
      closed_at: '2026-01-03T00:00:00Z',
      counters: {
        children: 1,
        findings: 0,
        sessions: 0,
        events: 2,
        entities: 1,
        proposed_edges: 0,
      },
    })

    const ok = await useAppStore.getState().persistInvestigation('inv-1', {
      status: 'closed',
      verdict: 'incident',
      verdictReason: 'c2 confirmed',
    })

    expect(ok).toBe(true)
    expect(irApi.patchInvestigation).toHaveBeenCalledWith(
      'inv-1',
      expect.objectContaining({
        version: 1,
        status: 'closed',
        verdict: 'incident',
        verdict_reason: 'c2 confirmed',
      }),
    )
    const inv = useAppStore.getState().investigations['inv-1']
    expect(inv?.status).toBe('closed')
    expect(inv?.verdict).toBe('incident')
    expect(inv?.verdictReason).toBe('c2 confirmed')
    expect(inv?.version).toBe(2)
  })

  it('persistInvestigation reopens a closed case', async () => {
    useAppStore.setState({
      investigations: {
        'inv-1': investigation({
          version: 2,
          status: 'closed',
          verdict: 'incident',
          verdictReason: 'c2 confirmed',
        }),
      },
    })
    vi.spyOn(irApi, 'patchInvestigation').mockResolvedValue({
      id: 'inv-1',
      project_id: 'proj',
      title: 'Case',
      status: 'open',
      version: 3,
      som_workspace_ids: [],
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-04T00:00:00Z',
      closed_at: null,
      counters: {
        children: 1,
        findings: 0,
        sessions: 0,
        events: 2,
        entities: 1,
        proposed_edges: 0,
      },
    })

    await useAppStore.getState().persistInvestigation('inv-1', { status: 'open' })

    expect(irApi.patchInvestigation).toHaveBeenCalledWith(
      'inv-1',
      expect.objectContaining({ version: 2, status: 'open' }),
    )
    const inv = useAppStore.getState().investigations['inv-1']
    expect(inv?.status).toBe('open')
    expect(inv?.verdict).toBeUndefined()
    expect(inv?.closedAt).toBeNull()
  })
})
