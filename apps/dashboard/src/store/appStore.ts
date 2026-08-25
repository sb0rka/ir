import { create } from 'zustand'
import type {
  ActionResult,
  AlertEvent,
  ContextEvent,
  ContextQueueState,
  CorrelationGroup,
  Entity,
  FilterChip,
  FilterField,
  Finding,
  GraphEdge,
  GraphNode,
  Investigation,
  Issue,
  QueueItem,
  QueueSource,
  QueryHistoryEntry,
  ReviewState,
} from '../types'
import { uid } from '../lib/utils'
import { defaultFilterValueOptions, issueTemplates } from '../lib/catalog'
import { parseGatewayEventId, saveLayout } from '../api/adapters'
import { errorMessage, isNotImplemented, isUnauthorized } from '../api/error'
import {
  analyzeArtifact,
  lookupEntity,
  searchQueue,
} from '../api/search'
import { appendCondition, astToFilterChips, defaultQuery, entityKindForField, parseQueuePdql, pdqlToSearchParts, serialize } from '../lib/pdql'
import { pdqlFieldForFilterField } from '../lib/filters'
import { filterFingerprint } from '../lib/queryFingerprint'
import { demoDayInterval, type TimeInterval } from '../components/time-interval'
import {
  addContext,
  countProposedAgentEdges,
  createEntity,
  createInvestigation,
  getEntityCard,
  getSomEnvironment,
  listInvestigations,
  loadInvestigationBundle,
  patchInvestigation,
  resolveSomCatalog,
  reviewEdges,
  runSomIssue,
  type SomCatalog,
} from '../api/ir'
import type { components as Ir } from '@ir/contract'

export type TabId = 'queue' | string

const DEFAULT_QUEUE_PDQL = serialize(defaultQuery())
const DEFAULT_TIME_INTERVAL = demoDayInterval()
const HISTORY_LIMIT = 8

export const emptyContextQueue: ContextQueueState = {
  chips: [],
  pdql: DEFAULT_QUEUE_PDQL,
  timeInterval: DEFAULT_TIME_INTERVAL,
  queueSource: 'findings',
  executedFingerprint: null,
  queryHistory: [],
  selectedIds: [],
  hideAdded: false,
  originFilter: 'all',
  reviewFilter: 'all',
  alerts: {},
  queueOrder: [],
  loading: false,
}

function pushQueryHistory(
  history: QueryHistoryEntry[],
  entry: QueryHistoryEntry,
): QueryHistoryEntry[] {
  const key = filterFingerprint(entry.pdql, entry.timeInterval, entry.queueSource)
  return [
    entry,
    ...history.filter(
      (item) => filterFingerprint(item.pdql, item.timeInterval, item.queueSource) !== key,
    ),
  ].slice(0, HISTORY_LIMIT)
}

interface AppState {
  chips: FilterChip[]
  timeInterval: TimeInterval
  queuePdql: string
  queueSource: QueueSource
  executedFingerprint: string | null
  queryHistory: QueryHistoryEntry[]
  selectedAlertIds: string[]
  expandedCorrelationIds: string[]
  inspectedQueueItem: QueueItem | null

  tabs: TabId[]
  activeTab: TabId
  investigations: Record<string, Investigation>
  issues: Record<string, Issue>
  eventReviews: Record<string, ReviewState>
  nodeReviews: Record<string, ReviewState>
  edgeReviews: Record<string, ReviewState>
  findingReviews: Record<string, ReviewState>
  actionResults: Record<string, ActionResult[]>
  contextQueue: Record<string, ContextQueueState>
  agentPanelOpen: boolean
  detailPanelOpen: boolean

  alerts: Record<string, AlertEvent>
  correlations: Record<string, CorrelationGroup>
  queueOrder: QueueItem[]
  entities: Record<string, Entity>
  contextEvents: Record<string, ContextEvent>
  graphNodes: Record<string, GraphNode>
  graphEdges: Record<string, GraphEdge>
  findings: Record<string, Finding>
  filterValueOptions: Record<string, string[]>
  mockSources: string[]

  queueLoading: boolean
  investigationLoading: boolean
  lastError: string | null
  lastNotImplemented: string | null
  somHint: string | null
  somCatalog: SomCatalog | null

  addChip: (field: FilterField, value: string) => void
  setQueuePdql: (pdql: string) => void
  setTimeInterval: (interval: TimeInterval) => void
  setQueueSource: (source: QueueSource) => void
  applyQueueHistory: (entry: QueryHistoryEntry) => void
  toggleAlertSelect: (id: string) => void
  clearAlertSelection: () => void
  toggleCorrelationExpand: (id: string) => void
  inspectQueueItem: (item: QueueItem | null) => void

  setActiveTab: (tab: TabId) => void
  closeTab: (tab: TabId) => void
  startInvestigation: (alertOrCorrIds: string[]) => Promise<string>
  createChildInvestigation: (parentId: string, entityIds: string[]) => Promise<string>
  updateInvestigation: (id: string, patch: Partial<Investigation>) => void
  persistInvestigation: (id: string, patch: Partial<Investigation>) => Promise<void>
  loadQueue: () => Promise<void>
  bootstrap: () => Promise<void>
  loadInvestigation: (id: string) => Promise<void>
  clearError: () => void

  setReview: (
    kind: 'event' | 'node' | 'edge' | 'finding',
    id: string,
    review: ReviewState,
    investigationId?: string,
  ) => void
  setContextQueue: (investigationId: string, patch: Partial<ContextQueueState>) => void
  addContextChip: (investigationId: string, field: FilterField, value: string) => void
  executeContextQuery: (investigationId: string) => Promise<boolean>
  addEventsToContext: (investigationId: string, eventIds: string[]) => Promise<void>
  appendPdqlFilter: (investigationId: string | null, field: string, value: string) => void
  addFieldToContext: (
    investigationId: string,
    input: { field: string; value: string; eventId: string; includeEvent: boolean },
  ) => Promise<void>
  setAgentPanelOpen: (open: boolean) => void
  setDetailPanelOpen: (open: boolean) => void
  loadSomCatalog: () => Promise<void>
  openAgentPanel: () => Promise<void>

  runEnrichment: (investigationId: string, issueId: string) => Promise<void>
  createIssue: (
    investigationId: string,
    templateId: string,
    entityIds: string[],
    parentId?: string,
  ) => Promise<void>
  cancelIssue: (issueId: string) => void
  addIssueComment: (issueId: string, text: string) => void
  runEntityAction: (entityId: string, action: string) => Promise<void>
  addFindingFromEntity: (investigationId: string, entityId: string) => void
}

function mergeEntities(
  current: Record<string, Entity>,
  incoming: Record<string, Entity>,
): Record<string, Entity> {
  return { ...current, ...incoming }
}

function contextRefsFromIds(
  ids: string[],
  alerts: Record<string, AlertEvent>,
  correlations: Record<string, CorrelationGroup>,
  contextEvents: Record<string, ContextEvent>,
): { events: Ir['schemas']['EventSourceRef'][]; findings: Ir['schemas']['SourceObjectRef'][] } {
  const events: Ir['schemas']['EventSourceRef'][] = []
  const findings: Ir['schemas']['SourceObjectRef'][] = []
  const seenEvents = new Set<string>()
  const seenFindings = new Set<string>()
  const pushEvent = (source: string, sourceEventId: string | undefined, fallbackId: string) => {
    const parsed = parseGatewayEventId(fallbackId)
    const source_code = source || parsed?.source_code
    const source_event_id = sourceEventId || parsed?.source_event_id
    if (!source_code || !source_event_id) return
    const key = `${source_code}/${source_event_id}`
    if (seenEvents.has(key)) return
    seenEvents.add(key)
    events.push({ source_code, source_event_id })
  }
  const pushFinding = (alert: AlertEvent) => {
    if (!alert.findingRef) return
    const key = `${alert.findingRef.source_code}/${alert.findingRef.record_type}/${alert.findingRef.external_id}`
    if (seenFindings.has(key)) return
    seenFindings.add(key)
    findings.push({
      source_code: alert.findingRef.source_code,
      source_instance: alert.findingRef.source_instance,
      record_type: alert.findingRef.record_type,
      external_id: alert.findingRef.external_id,
      time_range: alert.findingRef.time_range,
    })
  }
  for (const id of ids) {
    if (correlations[id]) {
      for (const eid of correlations[id].eventIds) {
        const a = alerts[eid] ?? contextEvents[eid]
        if (a) pushEvent(a.source, a.sourceEventId, a.id)
      }
      continue
    }
    const a = alerts[id] ?? contextEvents[id]
    if (!a) continue
    if ('findingRef' in a && a.findingRef) {
      pushFinding(a as AlertEvent)
      continue
    }
    pushEvent(a.source, a.sourceEventId, a.id)
  }
  return { events, findings }
}

function applyBundle(
  get: () => AppState,
  bundle: Awaited<ReturnType<typeof loadInvestigationBundle>>,
  keepView?: Investigation,
) {
  const eventReviews = { ...get().eventReviews }
  const nodeReviews = { ...get().nodeReviews }
  const edgeReviews = { ...get().edgeReviews }
  for (const ev of Object.values(bundle.events)) eventReviews[ev.id] = ev.review
  for (const n of Object.values(bundle.nodes)) nodeReviews[n.id] = n.review
  for (const e of Object.values(bundle.edges)) edgeReviews[e.id] = e.review
  return {
    investigations: {
      ...get().investigations,
      [bundle.investigation.id]: {
        ...bundle.investigation,
        view: keepView?.view ?? bundle.investigation.view,
        selectedEntityIds: keepView?.selectedEntityIds ?? [],
        selectedEventId: keepView?.selectedEventId,
        selectedNodeId: keepView?.selectedNodeId,
        seedEventIds: keepView?.seedEventIds ?? bundle.investigation.seedEventIds,
        issueIds: keepView?.issueIds ?? bundle.investigation.issueIds,
        findingIds: bundle.investigation.findingIds,
        findingSourceKeys: bundle.investigation.findingSourceKeys,
      },
    },
    contextEvents: { ...get().contextEvents, ...bundle.events },
    entities: mergeEntities(get().entities, bundle.entities),
    graphNodes: { ...get().graphNodes, ...bundle.nodes },
    graphEdges: { ...get().graphEdges, ...bundle.edges },
    eventReviews,
    nodeReviews,
    edgeReviews,
  }
}

export const useAppStore = create<AppState>((set, get) => ({
  chips: [],
  timeInterval: DEFAULT_TIME_INTERVAL,
  queuePdql: DEFAULT_QUEUE_PDQL,
  queueSource: 'findings',
  executedFingerprint: null,
  queryHistory: [],
  selectedAlertIds: [],
  expandedCorrelationIds: [],
  inspectedQueueItem: null,

  tabs: ['queue'],
  activeTab: 'queue',
  investigations: {},
  issues: {},
  eventReviews: {},
  nodeReviews: {},
  edgeReviews: {},
  findingReviews: {},
  actionResults: {},
  contextQueue: {},
  agentPanelOpen: false,
  detailPanelOpen: false,

  alerts: {},
  correlations: {},
  queueOrder: [],
  entities: {},
  contextEvents: {},
  graphNodes: {},
  graphEdges: {},
  findings: {},
  filterValueOptions: defaultFilterValueOptions,
  mockSources: [],

  queueLoading: false,
  investigationLoading: false,
  lastError: null,
  lastNotImplemented: null,
  somHint: null,
  somCatalog: null,

  addChip: (field, value) => {
    set({
      queuePdql: appendCondition(get().queuePdql, pdqlFieldForFilterField(field), '=', value),
    })
  },
  setQueuePdql: (queuePdql) => set({ queuePdql }),
  setTimeInterval: (timeInterval) => set({ timeInterval }),
  setQueueSource: (queueSource) => set({ queueSource }),
  applyQueueHistory: (entry) =>
    set({
      queuePdql: entry.pdql,
      timeInterval: entry.timeInterval,
      queueSource: entry.queueSource ?? 'findings',
    }),
  toggleAlertSelect: (id) => {
    const sel = get().selectedAlertIds
    set({
      selectedAlertIds: sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id],
    })
  },
  clearAlertSelection: () => set({ selectedAlertIds: [] }),
  inspectQueueItem: (item) => set({ inspectedQueueItem: item }),
  toggleCorrelationExpand: (id) => {
    const cur = get().expandedCorrelationIds
    set({
      expandedCorrelationIds: cur.includes(id)
        ? cur.filter((x) => x !== id)
        : [...cur, id],
    })
  },
  setActiveTab: (activeTab) => set({ activeTab }),
  closeTab: (tab) => {
    if (tab === 'queue') return
    const tabs = get().tabs.filter((t) => t !== tab)
    const activeTab = get().activeTab === tab ? tabs[tabs.length - 1] ?? 'queue' : get().activeTab
    set({ tabs, activeTab })
  },

  clearError: () => set({ lastError: null, lastNotImplemented: null }),

  bootstrap: async () => {
    try {
      const listed = await listInvestigations()
      if (listed.length) {
        const investigations = { ...get().investigations }
        const tabs = [...get().tabs]
        for (const inv of listed) {
          investigations[inv.id] = inv
          if (!tabs.includes(inv.id)) tabs.push(inv.id)
        }
        set({ investigations, tabs })
      }
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
    await get().loadQueue()
  },

  loadQueue: async () => {
    const parsed = parseQueuePdql(get().queuePdql)
    if (parsed.ok === false) {
      const { error } = parsed
      set({
        lastError: `${error.message}${error.position > 0 ? ` (позиция ${error.position})` : ''}`,
      })
      return
    }
    const chips = astToFilterChips(parsed.ast)
    const query = pdqlToSearchParts(parsed.ast).query
    const timeInterval = get().timeInterval
    const queueSource = get().queueSource
    set({ queueLoading: true, lastError: null, mockSources: [] })
    try {
      const result = await searchQueue(chips, timeInterval, query, queueSource)
      const hosts = new Set(get().filterValueOptions.host ?? [])
      const ips = new Set(get().filterValueOptions.ip ?? [])
      for (const e of Object.values(result.entities)) {
        if (e.kind === 'host') hosts.add(e.label)
        if (e.kind === 'ip') ips.add(e.label)
      }
      const canonical = serialize(parsed.ast)
      set({
        chips,
        queuePdql: canonical,
        executedFingerprint: filterFingerprint(canonical, timeInterval, queueSource),
        queryHistory: pushQueryHistory(get().queryHistory, {
          pdql: canonical,
          timeInterval,
          queueSource,
        }),
        alerts: result.alerts,
        correlations: result.correlations,
        queueOrder: result.queueOrder,
        entities: mergeEntities(get().entities, result.entities),
        contextEvents: { ...get().contextEvents, ...result.contextEvents },
        expandedCorrelationIds: result.queueOrder
          .filter((i) => i.kind === 'correlation')
          .map((i) => i.id)
          .slice(0, 3),
        filterValueOptions: {
          ...get().filterValueOptions,
          host: [...hosts],
          ip: [...ips],
          source: result.availableSources,
        },
        mockSources: result.mockSources,
        queueLoading: false,
        lastError: result.sourceErrors.length ? result.sourceErrors.join('; ') : null,
      })
    } catch (err) {
      set({ queueLoading: false, lastError: errorMessage(err), mockSources: [] })
    }
  },

  loadInvestigation: async (id) => {
    const keep = get().investigations[id]
    set({ investigationLoading: true, lastError: null })
    try {
      const bundle = await loadInvestigationBundle(id, keep)
      set({ ...applyBundle(get, bundle, keep), investigationLoading: false })
    } catch (err) {
      set({ investigationLoading: false, lastError: errorMessage(err) })
    }
  },

  startInvestigation: async (ids) => {
    const { alerts, correlations } = get()
    const title =
      ids.length === 1 && correlations[ids[0]]
        ? correlations[ids[0]].title
        : ids.length === 1 && alerts[ids[0]]
          ? alerts[ids[0]].title
          : `Расследование (${ids.length})`
    const severity =
      ids
        .map((id) => correlations[id]?.severity ?? alerts[id]?.severity)
        .filter(Boolean)
        .sort((a, b) => {
          const order = ['critical', 'high', 'medium', 'low', 'info']
          return order.indexOf(a!) - order.indexOf(b!)
        })[0] ?? 'high'

    set({ investigationLoading: true, lastError: null })
    try {
      const created = await createInvestigation({ title, severity })
      const refs = contextRefsFromIds(ids, alerts, correlations, get().contextEvents)
      if (refs.events.length || refs.findings.length) await addContext(created.id, refs)
      const bundle = await loadInvestigationBundle(created.id, {
        seedEventIds: ids,
        view: 'graph',
      })
      set({
        ...applyBundle(get, bundle),
        tabs: [...get().tabs, created.id],
        activeTab: created.id,
        selectedAlertIds: [],
        inspectedQueueItem: null,
        agentPanelOpen: true,
        investigationLoading: false,
      })
      void get().loadSomCatalog()
      return created.id
    } catch (err) {
      set({ investigationLoading: false, lastError: errorMessage(err) })
      return ''
    }
  },

  createChildInvestigation: async (parentId, entityIds) => {
    const parent = get().investigations[parentId]
    if (!parent) return ''
    set({ investigationLoading: true, lastError: null })
    try {
      const created = await createInvestigation({
        title: `${parent.title} → ветка`,
        severity: parent.severity,
        parentId,
        workspaceId: parent.somWorkspaceIds?.[0],
      })
      const relatedEvents = parent.eventIds.filter((eid) =>
        get().contextEvents[eid]?.entityIds.some((x) => entityIds.includes(x)),
      )
      const refs = contextRefsFromIds(
        relatedEvents,
        get().alerts,
        get().correlations,
        get().contextEvents,
      )
      if (refs.events.length || refs.findings.length) await addContext(created.id, refs)
      const bundle = await loadInvestigationBundle(created.id, {
        parentId,
        view: 'graph',
        selectedEntityIds: [],
      })
      set({
        ...applyBundle(get, bundle),
        tabs: [...get().tabs, created.id],
        activeTab: created.id,
        investigationLoading: false,
      })
      return created.id
    } catch (err) {
      set({ investigationLoading: false, lastError: errorMessage(err) })
      return ''
    }
  },

  updateInvestigation: (id, patch) => {
    const inv = get().investigations[id]
    if (!inv) return
    set({
      investigations: {
        ...get().investigations,
        [id]: { ...inv, ...patch },
      },
    })
  },

  persistInvestigation: async (id, patch) => {
    const inv = get().investigations[id]
    if (!inv || inv.version == null) {
      get().updateInvestigation(id, patch)
      return
    }
    try {
      await patchInvestigation(id, {
        version: inv.version,
        title: patch.title,
        status: patch.status === 'closed' ? 'closed' : patch.status === 'open' ? 'open' : undefined,
        severity:
          patch.severity && patch.severity !== 'info' ? patch.severity : undefined,
      })
      get().updateInvestigation(id, patch)
    } catch (err) {
      if (isNotImplemented(err)) {
        set({ lastNotImplemented: errorMessage(err) })
        return
      }
      set({ lastError: errorMessage(err) })
    }
  },

  setReview: (kind, id, review, investigationId) => {
    if (kind === 'event') {
      set({ eventReviews: { ...get().eventReviews, [id]: review } })
      return
    }
    if (kind === 'node') {
      set({ nodeReviews: { ...get().nodeReviews, [id]: review } })
      return
    }
    if (kind === 'finding') {
      set({ findingReviews: { ...get().findingReviews, [id]: review } })
      return
    }

    const invId =
      investigationId ??
      (get().activeTab !== 'queue' ? get().activeTab : undefined)
    const edge = get().graphEdges[id]
    const previous = get().edgeReviews[id] ?? edge?.review
    // Apply locally first so the proposed list stays interactive if /review is 501.
    set({ edgeReviews: { ...get().edgeReviews, [id]: review } })
    if (!invId || !edge || edge.version == null) return
    void (async () => {
      try {
        const body: Ir['schemas']['ReviewRequest'] =
          review === 'rejected'
            ? { reject: [{ id, version: edge.version ?? 1, reason: 'Отклонено аналитиком' }] }
            : { confirm: [{ id, version: edge.version ?? 1 }] }
        await reviewEdges(invId, body)
        await get().loadInvestigation(invId)
      } catch (err) {
        if (isNotImplemented(err)) return
        set({
          lastError: errorMessage(err),
          edgeReviews: { ...get().edgeReviews, [id]: previous ?? 'proposed' },
        })
      }
    })()
  },

  setContextQueue: (investigationId, patch) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, ...patch },
      },
    })
  },

  addContextChip: (investigationId, field, value) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          pdql: appendCondition(cur.pdql, pdqlFieldForFilterField(field), '=', value),
        },
      },
    })
  },
  executeContextQuery: async (investigationId) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    const parsed = parseQueuePdql(cur.pdql)
    if (parsed.ok === false) {
      const { error } = parsed
      set({
        lastError: `${error.message}${error.position > 0 ? ` (позиция ${error.position})` : ''}`,
      })
      return false
    }
    const chips = astToFilterChips(parsed.ast)
    const query = pdqlToSearchParts(parsed.ast).query
    const timeInterval = cur.timeInterval
    const queueSource = cur.queueSource
    const canonical = serialize(parsed.ast)
    set({
      lastError: null,
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, chips, pdql: canonical, loading: true },
      },
    })
    try {
      const result = await searchQueue(chips, timeInterval, query, queueSource)
      const hosts = new Set(get().filterValueOptions.host ?? [])
      const ips = new Set(get().filterValueOptions.ip ?? [])
      for (const e of Object.values(result.entities)) {
        if (e.kind === 'host') hosts.add(e.label)
        if (e.kind === 'ip') ips.add(e.label)
      }
      const latest = get().contextQueue[investigationId] ?? emptyContextQueue
      set({
        entities: mergeEntities(get().entities, result.entities),
        filterValueOptions: {
          ...get().filterValueOptions,
          host: [...hosts],
          ip: [...ips],
          source: result.availableSources,
        },
        lastError: result.sourceErrors.length ? result.sourceErrors.join('; ') : null,
        contextQueue: {
          ...get().contextQueue,
          [investigationId]: {
            ...latest,
            chips,
            pdql: canonical,
            queueSource,
            alerts: result.alerts,
            queueOrder: result.queueOrder,
            loading: false,
            executedFingerprint: filterFingerprint(canonical, timeInterval, queueSource),
            queryHistory: pushQueryHistory(latest.queryHistory, {
              pdql: canonical,
              timeInterval,
              queueSource,
            }),
            selectedIds: [],
          },
        },
      })
      return true
    } catch (err) {
      const latest = get().contextQueue[investigationId] ?? emptyContextQueue
      set({
        lastError: errorMessage(err),
        contextQueue: {
          ...get().contextQueue,
          [investigationId]: { ...latest, loading: false },
        },
      })
      return false
    }
  },

  addEventsToContext: async (investigationId, eventIds) => {
    const queue = get().contextQueue[investigationId]
    const refs = contextRefsFromIds(
      eventIds,
      { ...get().alerts, ...queue?.alerts },
      get().correlations,
      get().contextEvents,
    )
    if (refs.events.length === 0 && refs.findings.length === 0) return
    try {
      await addContext(investigationId, refs)
      const cur = get().contextQueue[investigationId] ?? emptyContextQueue
      set({
        contextQueue: {
          ...get().contextQueue,
          [investigationId]: { ...cur, selectedIds: [] },
        },
      })
      await get().loadInvestigation(investigationId)
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  appendPdqlFilter: (investigationId, field, value) => {
    if (!investigationId) {
      set({ queuePdql: appendCondition(get().queuePdql, field, '=', value) })
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, pdql: appendCondition(cur.pdql, field, '=', value) },
      },
    })
  },

  addFieldToContext: async (investigationId, input) => {
    const kind = entityKindForField(input.field)
    try {
      if (kind) {
        await createEntity(investigationId, {
          type_code: kind,
          canonical_key: input.value,
          display_name: input.value,
          metadata: { field: input.field },
        })
      }
      const inv = get().investigations[investigationId]
      const alreadyInContext = Boolean(inv?.eventIds.includes(input.eventId))
      if (input.includeEvent || alreadyInContext) {
        await get().addEventsToContext(investigationId, [input.eventId])
        return
      }
      await get().loadInvestigation(investigationId)
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  setAgentPanelOpen: (agentPanelOpen) => set({ agentPanelOpen }),
  setDetailPanelOpen: (detailPanelOpen) => set({ detailPanelOpen }),

  loadSomCatalog: async () => {
    try {
      set({ somCatalog: await resolveSomCatalog() })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  openAgentPanel: async () => {
    set({ agentPanelOpen: true })
    if (!get().somCatalog) await get().loadSomCatalog()
  },

  runEnrichment: async (investigationId, issueId) => {
    const inv = get().investigations[investigationId]
    if (!inv || !issueId) return
    set({ agentPanelOpen: true, lastError: null, somHint: null })
    try {
      let catalog = get().somCatalog
      if (!catalog) {
        catalog = await resolveSomCatalog()
        set({ somCatalog: catalog })
      }
      const issueDef = catalog.issues.find((i) => i.id === issueId)
      if (!issueDef) {
        set({ lastError: 'Issue не найден' })
        return
      }
      if (get().issues[issueDef.id]?.status === 'running') return
      const issue: Issue = {
        id: issueDef.id,
        investigationId,
        template: issueDef.simple_id,
        title: issueDef.title,
        description: issueDef.description || 'Насыщение контекста',
        entityIds: inv.entityIds.slice(0, 4),
        status: 'running',
        eventsFound: 0,
        edgesFound: 0,
        findingsFound: 0,
        comments: [],
        createdAt: new Date().toISOString(),
      }
      set({
        issues: { ...get().issues, [issue.id]: issue },
        investigations: {
          ...get().investigations,
          [investigationId]: {
            ...inv,
            issueIds: inv.issueIds.includes(issue.id)
              ? inv.issueIds
              : [...inv.issueIds, issue.id],
          },
        },
      })
      const before = await countProposedAgentEdges(investigationId)
      const run = await runSomIssue(issueDef.id, investigationId)
      const localEnvironmentId = run.local_environment_id
      set({
        issues: {
          ...get().issues,
          [issue.id]: { ...get().issues[issue.id]!, localEnvironmentId },
        },
      })

      const poll = async () => {
        const currentIssue = get().issues[issue.id]
        if (!currentIssue || currentIssue.status === 'cancelled') return
        try {
          await get().loadInvestigation(investigationId)
          const now = await countProposedAgentEdges(investigationId)
          const edgesFound = Math.max(0, now - before)
          const env = await getSomEnvironment(localEnvironmentId)

          if (env.status === 'running') {
            const live = get().issues[issue.id]
            if (live && live.status === 'running') {
              set({
                issues: {
                  ...get().issues,
                  [issue.id]: { ...live, edgesFound, localEnvironmentId },
                },
              })
            }
            window.setTimeout(() => void poll(), 2500)
            return
          }

          const live = get().issues[issue.id]
          if (!live || live.status === 'cancelled') return
          const latest = get().investigations[investigationId]
          const proposed = latest
            ? latest.edgeIds.filter(
                (eid) => (get().edgeReviews[eid] ?? get().graphEdges[eid]?.review) === 'proposed',
              ).length
            : now

          if (env.status === 'failed') {
            set({
              issues: {
                ...get().issues,
                [issue.id]: {
                  ...live,
                  status: 'error',
                  edgesFound,
                  localEnvironmentId,
                  resultSummary: 'Агент завершился с ошибкой.',
                },
              },
            })
            return
          }

          set({
            issues: {
              ...get().issues,
              [issue.id]: {
                ...live,
                status: 'completed',
                edgesFound,
                localEnvironmentId,
                resultSummary:
                  proposed > 0
                    ? `Агент предложил ${proposed} связей. Нужно ревью.`
                    : 'Агент завершился. Новых proposed-связей нет.',
              },
            },
          })
        } catch {
          window.setTimeout(() => void poll(), 2500)
        }
      }
      window.setTimeout(() => void poll(), 2500)
    } catch (err) {
      const failed = get().issues[issueId]
      if (failed?.status === 'running') {
        set({
          issues: {
            ...get().issues,
            [issueId]: {
              ...failed,
              status: 'error',
              resultSummary: errorMessage(err),
            },
          },
        })
      }
      set({
        lastError: errorMessage(err),
        somHint: isUnauthorized(err)
          ? 'Обновите SOM-токен в шапке — он живет около часа'
          : null,
      })
    }
  },

  createIssue: async (investigationId, templateId, entityIds, parentId) => {
    await get().openAgentPanel()
    const inv = get().investigations[investigationId]
    if (!inv || !parentId) return
    const tpl = issueTemplates.find((t) => t.id === templateId) ?? issueTemplates[0]
    const running = inv.issueIds
      .map((id) => get().issues[id])
      .find((i) => i?.status === 'running')
    if (running) {
      set({
        issues: {
          ...get().issues,
          [running.id]: { ...running, parentId, entityIds, template: tpl.title },
        },
      })
    }
  },

  cancelIssue: (issueId) => {
    const issue = get().issues[issueId]
    if (!issue || issue.status !== 'running') return
    set({
      issues: {
        ...get().issues,
        [issueId]: { ...issue, status: 'cancelled', resultSummary: 'Отменено аналитиком' },
      },
    })
  },

  addIssueComment: (issueId, text) => {
    const issue = get().issues[issueId]
    if (!issue) return
    set({
      issues: {
        ...get().issues,
        [issueId]: {
          ...issue,
          comments: [
            ...issue.comments,
            {
              id: uid('cmt'),
              author: 'аналитик',
              time: new Date().toISOString(),
              text,
            },
          ],
        },
      },
    })
  },

  runEntityAction: async (entityId, action) => {
    const entity = get().entities[entityId]
    if (!entity) return
    const results = get().actionResults[entityId] ?? []
    let body = `Действие ${action} выполнено`
    try {
      if (action === 'reputation') {
        body = await lookupEntity(entity.kind === 'file_hash' ? 'hash' : entity.kind, entity.label)
      } else if (action === 'sandbox') {
        body = await analyzeArtifact(
          entity.label,
          entity.attributes.hash || entity.attributes.sha256,
        )
      } else if (action === 'related') {
        const card = await getEntityCard(entityId).catch(() => null)
        body = card
          ? `Связанные события: ${card.events_count}. Расследований: ${card.occurrences.length}`
          : `Связанные события по ${entity.label}`
      } else if (action === 'decode') {
        body = entity.attributes.cmdline
          ? `cmdline: ${entity.attributes.cmdline}`
          : 'Декодирование недоступно: у сущности нет cmdline'
      } else if (action === 'enrich') {
        const card = await getEntityCard(entityId).catch(() => null)
        body = card
          ? `Карточка ${entity.label}: ${card.events_count} событий, соседей: ${card.neighbors?.length ?? 0}`
          : `Обогащение ${entity.label}`
      }
    } catch (err) {
      body = errorMessage(err)
    }
    const titles: Record<string, string> = {
      enrich: 'Обогащение',
      reputation: 'Репутация',
      decode: 'Декодирование',
      sandbox: 'Песочница',
      related: 'Связанные события',
    }
    const result: ActionResult = {
      id: uid('act'),
      action,
      title: titles[action] ?? action,
      body,
      time: new Date().toISOString(),
    }
    set({
      actionResults: {
        ...get().actionResults,
        [entityId]: [result, ...results],
      },
      detailPanelOpen: true,
    })
  },

  addFindingFromEntity: (investigationId, entityId) => {
    const inv = get().investigations[investigationId]
    const entity = get().entities[entityId]
    if (!inv || !entity) return
    const fid = uid('find')
    const finding: Finding = {
      id: fid,
      title: `Находка: ${entity.label}`,
      severity: 'high',
      entityIds: [entityId],
      description: `Добавлено аналитиком из карточки ${entity.kind}`,
      review: 'confirmed',
      origin: 'analyst',
    }
    set({
      findings: { ...get().findings, [fid]: finding },
      findingReviews: { ...get().findingReviews, [fid]: 'confirmed' },
      investigations: {
        ...get().investigations,
        [investigationId]: {
          ...inv,
          findingIds: [...inv.findingIds, fid],
        },
      },
    })
  },
}))

export { savedViews, issueTemplates, filterFieldLabels } from '../lib/catalog'

export function persistNodePosition(
  investigationId: string,
  nodeId: string,
  position: { x: number; y: number },
) {
  const nodes = useAppStore.getState().graphNodes
  const match = Object.values(nodes).find((n) => n.id === nodeId || n.refId === nodeId)
  if (!match) return
  persistGraphLayout(investigationId, [
    { ...match, x: position.x, y: position.y },
  ])
}

export function persistGraphLayout(investigationId: string, nodes: GraphNode[]) {
  if (nodes.length === 0) return
  const current = useAppStore.getState().graphNodes
  const next = { ...current }
  for (const node of nodes) {
    const match = next[node.id] ?? Object.values(next).find((n) => n.refId === node.id)
    if (!match) continue
    next[match.id] = { ...match, x: node.x, y: node.y }
  }
  useAppStore.setState({ graphNodes: next })
  const layout: Record<string, { x: number; y: number }> = {}
  const invNodeIds = new Set(
    useAppStore.getState().investigations[investigationId]?.nodeIds ?? [],
  )
  for (const n of Object.values(next)) {
    if (invNodeIds.has(n.id)) layout[n.id] = { x: n.x, y: n.y }
  }
  saveLayout(investigationId, layout)
}
