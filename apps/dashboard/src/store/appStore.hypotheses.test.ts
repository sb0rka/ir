import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/error'
import type { Hypothesis } from '../api/hypotheses'
import type { Investigation } from '../types'

const {
  listHypotheses,
  createHypothesisRequest,
  patchHypothesisRequest,
  getHypothesis,
  getHypothesisGraph,
  addHypothesisContext,
  addHypothesisNode,
  removeHypothesisNode,
} = vi.hoisted(() => ({
  listHypotheses: vi.fn(),
  createHypothesisRequest: vi.fn(),
  patchHypothesisRequest: vi.fn(),
  getHypothesis: vi.fn(),
  getHypothesisGraph: vi.fn(),
  addHypothesisContext: vi.fn(),
  addHypothesisNode: vi.fn(),
  removeHypothesisNode: vi.fn(),
}))

vi.mock('../api/hypotheses', () => ({
  listHypotheses,
  createHypothesis: createHypothesisRequest,
  patchHypothesis: patchHypothesisRequest,
  getHypothesis,
  getHypothesisGraph,
  addHypothesisContext,
  addHypothesisNode,
  removeHypothesisNode,
  deleteHypothesis: vi.fn(),
  addHypothesisEdge: vi.fn(),
  removeHypothesisEdge: vi.fn(),
}))

import { useAppStore, emptyContextQueue } from './appStore'
import * as irApi from '../api/ir'

function hypothesis(overrides: Partial<Hypothesis> = {}): Hypothesis {
  return {
    id: 'h1',
    project_id: 'proj',
    investigation_id: 'inv-1',
    statement: 'lateral movement',
    description: null,
    status: 'proposed',
    reason: null,
    origin: 'analyst',
    version: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    resolved_at: null,
    ...overrides,
  }
}

function investigation(overrides: Partial<Investigation> = {}): Investigation {
  return {
    id: 'inv-1',
    title: 'Case',
    severity: 'high',
    status: 'open',
    assignee: 'аналитик',
    seedEventIds: [],
    eventIds: [],
    entityIds: ['ent-1'],
    nodeIds: ['n1', 'n2'],
    edgeIds: ['e1'],
    findingIds: [],
    findingSourceKeys: [],
    issueIds: [],
    hypothesisIds: [],
    createdAt: '2026-01-01T00:00:00Z',
    view: 'graph',
    selectedEntityIds: ['ent-1'],
    ...overrides,
  }
}

const snapshot = {
  hypotheses: {},
  hypothesisMembership: {},
  activeHypothesisId: {},
  visibleHypothesisIds: {},
  highlightedHypothesisIds: {},
  investigations: {},
  graphNodes: {},
  graphEdges: {},
  contextQueue: {},
  lastError: null as string | null,
  sidebarSection: null as null,
  agentPanelOpen: false,
  hypothesisDraftOpen: false,
}

beforeEach(() => {
  useAppStore.setState({
    ...snapshot,
    investigations: { 'inv-1': investigation() },
    graphNodes: {
      n1: {
        id: 'n1',
        kind: 'host',
        refId: 'ent-1',
        label: 'ws-1',
        review: 'confirmed',
        x: 0,
        y: 0,
      },
      n2: {
        id: 'n2',
        kind: 'user',
        refId: 'ent-2',
        label: 'alice',
        review: 'confirmed',
        x: 0,
        y: 0,
      },
    },
    graphEdges: {
      e1: {
        id: 'e1',
        source: 'n1',
        target: 'n2',
        relation: 'connected',
        review: 'confirmed',
      },
    },
  })
})

afterEach(() => {
  vi.clearAllMocks()
  vi.restoreAllMocks()
  useAppStore.setState(snapshot)
})

describe('hypothesis store', () => {
  it('creates a proposed hypothesis and turns on the lens', async () => {
    const created = hypothesis()
    createHypothesisRequest.mockResolvedValue(created)
    getHypothesisGraph.mockResolvedValue({
      hypothesis_id: created.id,
      investigation_id: 'inv-1',
      nodes: [],
      edges: [],
    })

    const result = await useAppStore.getState().createHypothesis('inv-1', {
      statement: 'lateral movement',
    })

    expect(result?.status).toBe('proposed')
    expect(useAppStore.getState().investigations['inv-1']?.hypothesisIds).toEqual(['h1'])
    expect(useAppStore.getState().activeHypothesisId['inv-1']).toBe('h1')
    expect(useAppStore.getState().sidebarSection).toBe('hypotheses')
    expect(useAppStore.getState().visibleHypothesisIds['inv-1']).toEqual([
      '__investigation__',
      'h1',
    ])
  })

  it('refreshes the card on 409 and does not keep the stale patch', async () => {
    const current = hypothesis({ version: 1 })
    const fresh = hypothesis({ version: 2, statement: 'updated elsewhere' })
    useAppStore.setState({
      hypotheses: { h1: current },
      investigations: { 'inv-1': investigation({ hypothesisIds: ['h1'] }) },
    })
    patchHypothesisRequest.mockRejectedValue(new ApiError('conflict', 'stale', 409))
    getHypothesis.mockResolvedValue(fresh)

    const result = await useAppStore.getState().patchHypothesis('inv-1', 'h1', {
      status: 'active',
    })

    expect(result).toBeNull()
    expect(useAppStore.getState().hypotheses.h1).toEqual(fresh)
    expect(useAppStore.getState().lastError).toMatch(/изменилась/)
  })

  it('reloads membership after toggling a node', async () => {
    useAppStore.setState({
      hypotheses: { h1: hypothesis({ status: 'active' }) },
      activeHypothesisId: { 'inv-1': 'h1' },
      hypothesisMembership: { h1: { nodeIds: ['n1'], edgeIds: [] } },
    })
    removeHypothesisNode.mockResolvedValue(undefined)
    getHypothesisGraph.mockResolvedValue({
      hypothesis_id: 'h1',
      investigation_id: 'inv-1',
      nodes: [],
      edges: [],
    })

    await useAppStore.getState().toggleHypothesisNode('inv-1', 'n1')

    expect(removeHypothesisNode).toHaveBeenCalledWith('inv-1', 'h1', 'n1')
    expect(useAppStore.getState().hypothesisMembership.h1).toEqual({
      nodeIds: [],
      edgeIds: [],
    })
  })

  it('adds selected entity nodes when creating with includeSelection', async () => {
    const created = hypothesis()
    createHypothesisRequest.mockResolvedValue(created)
    addHypothesisNode.mockResolvedValue(undefined)
    getHypothesisGraph.mockResolvedValue({
      hypothesis_id: created.id,
      investigation_id: 'inv-1',
      nodes: [{ id: 'n1' }],
      edges: [],
    })

    await useAppStore.getState().createHypothesis('inv-1', {
      statement: 'lateral movement',
      includeSelection: true,
    })

    expect(addHypothesisNode).toHaveBeenCalledWith('inv-1', 'h1', 'n1')
    expect(useAppStore.getState().hypothesisMembership.h1?.nodeIds).toEqual(['n1'])
  })

  it('imports a queue event into the graph and the active hypothesis', async () => {
    addHypothesisContext.mockResolvedValue({
      findings: 0,
      sessions: 0,
      events: 1,
      entities: 0,
      nodes: 1,
      edges: 0,
      warnings: [],
    })
    getHypothesisGraph.mockResolvedValue({
      hypothesis_id: 'h1',
      investigation_id: 'inv-1',
      nodes: [{ id: 'n-evt' }],
      edges: [],
    })
    listHypotheses.mockResolvedValue([hypothesis({ status: 'active' })])
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigation({ eventIds: ['evt-1'], hypothesisIds: ['h1'] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    useAppStore.setState({
      hypotheses: { h1: hypothesis({ status: 'active' }) },
      activeHypothesisId: { 'inv-1': 'h1' },
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          alerts: {
            'evt-1': {
              id: 'evt-1',
              time: '2026-01-01T00:00:00Z',
              severity: 'high',
              title: 'login',
              rule: 'r',
              source: 'pt-maxpatrol-siem',
              status: 'new',
              entityIds: [],
              description: '',
              sourceEventId: 'src-1',
            },
          },
        },
      },
    })

    await useAppStore.getState().addEventsToActiveHypothesis('inv-1', ['evt-1'])

    expect(addHypothesisContext).toHaveBeenCalledWith('inv-1', 'h1', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'src-1' }],
      findings: [],
    })
    expect(useAppStore.getState().hypothesisMembership.h1?.nodeIds).toEqual(['n-evt'])
  })

  it('does not import into a resolved or missing hypothesis', async () => {
    useAppStore.setState({
      hypotheses: { h1: hypothesis({ status: 'resolved' }) },
      activeHypothesisId: { 'inv-1': 'h1' },
    })

    await useAppStore.getState().addEventsToActiveHypothesis('inv-1', ['evt-1'])
    expect(addHypothesisContext).not.toHaveBeenCalled()

    useAppStore.setState({ activeHypothesisId: { 'inv-1': null } })
    await useAppStore.getState().addEventsToActiveHypothesis('inv-1', ['evt-1'])
    expect(addHypothesisContext).not.toHaveBeenCalled()
  })

  it('creates a hypothesis from a queue event and imports it', async () => {
    const created = hypothesis({ id: 'h2', statement: 'login' })
    createHypothesisRequest.mockResolvedValue(created)
    addHypothesisContext.mockResolvedValue({
      findings: 0,
      sessions: 0,
      events: 1,
      entities: 0,
      nodes: 1,
      edges: 0,
      warnings: [],
    })
    getHypothesisGraph.mockResolvedValue({
      hypothesis_id: created.id,
      investigation_id: 'inv-1',
      nodes: [{ id: 'n-evt' }],
      edges: [],
    })
    listHypotheses.mockResolvedValue([created])
    vi.spyOn(irApi, 'loadInvestigationBundle').mockResolvedValue({
      investigation: investigation({ eventIds: ['evt-1'], hypothesisIds: ['h2'] }),
      events: {},
      entities: {},
      nodes: {},
      edges: {},
      findingSourceKeys: [],
    })
    useAppStore.setState({
      contextQueue: {
        'inv-1': {
          ...emptyContextQueue,
          alerts: {
            'evt-1': {
              id: 'evt-1',
              time: '2026-01-01T00:00:00Z',
              severity: 'high',
              title: 'login',
              rule: 'r',
              source: 'pt-maxpatrol-siem',
              status: 'new',
              entityIds: [],
              description: '',
              sourceEventId: 'src-1',
            },
          },
        },
      },
    })

    const result = await useAppStore.getState().createHypothesisFromEvents('inv-1', ['evt-1'])

    expect(result?.id).toBe('h2')
    expect(createHypothesisRequest).toHaveBeenCalledWith('inv-1', { statement: 'login' })
    expect(addHypothesisContext).toHaveBeenCalledWith('inv-1', 'h2', {
      events: [{ source_code: 'pt-maxpatrol-siem', source_event_id: 'src-1' }],
      findings: [],
    })
    expect(useAppStore.getState().activeHypothesisId['inv-1']).toBe('h2')
    expect(useAppStore.getState().hypothesisMembership.h2?.nodeIds).toEqual(['n-evt'])
  })

  it('toggles and isolates visibility independently of the selected hypothesis', () => {
    expect(useAppStore.getState().visibleHypothesisIds['inv-1']).toBeUndefined()
    useAppStore.getState().toggleHypothesisVisible('inv-1', 'h1')
    expect(useAppStore.getState().visibleHypothesisIds['inv-1']).toEqual(['__investigation__', 'h1'])
    useAppStore.getState().toggleHypothesisVisible('inv-1', 'h1', true)
    expect(useAppStore.getState().visibleHypothesisIds['inv-1']).toEqual(['h1'])
    expect(useAppStore.getState().activeHypothesisId['inv-1']).toBeUndefined()
  })

  it('keeps the selected hypothesis when setActiveHypothesis is called with the same id', async () => {
    getHypothesisGraph.mockResolvedValue({
      hypothesis_id: 'h1',
      investigation_id: 'inv-1',
      nodes: [],
      edges: [],
    })

    await useAppStore.getState().setActiveHypothesis('inv-1', 'h1')
    expect(useAppStore.getState().activeHypothesisId['inv-1']).toBe('h1')
    expect(getHypothesisGraph).toHaveBeenCalledTimes(1)

    await useAppStore.getState().setActiveHypothesis('inv-1', 'h1')
    expect(useAppStore.getState().activeHypothesisId['inv-1']).toBe('h1')
    expect(getHypothesisGraph).toHaveBeenCalledTimes(1)
  })

  it('clears the selected hypothesis only when set to null', async () => {
    useAppStore.setState({ activeHypothesisId: { 'inv-1': 'h1' } })

    await useAppStore.getState().setActiveHypothesis('inv-1', null)
    expect(useAppStore.getState().activeHypothesisId['inv-1']).toBeNull()
  })

  it('highlights layers only when at least one bulb is on', () => {
    expect(useAppStore.getState().highlightedHypothesisIds['inv-1']).toBeUndefined()
    useAppStore.getState().toggleHypothesisHighlight('inv-1', 'h1')
    expect(useAppStore.getState().highlightedHypothesisIds['inv-1']).toEqual(['h1'])
    useAppStore.getState().toggleHypothesisHighlight('inv-1', 'h1')
    expect(useAppStore.getState().highlightedHypothesisIds['inv-1']).toEqual([])
  })
})
