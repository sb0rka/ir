import { create } from 'zustand'
import {
  DEFAULT_QUEUE_SOURCE,
  type ActionResult,
  type AlertEvent,
  type ContextEvent,
  type ContextQueueState,
  type CorrelationGroup,
  type Entity,
  type EventGroupItem,
  type FilterChip,
  type FilterField,
  type Finding,
  type GraphEdge,
  type GraphNode,
  type Investigation,
  type InvestigationListFilter,
  type Issue,
  type QueueItem,
  type QueueSource,
  type QueryHistoryEntry,
  type ReviewState,
} from '../types'
import { uid } from '../lib/utils'
import { defaultFilterValueOptions, issueTemplates } from '../lib/catalog'
import { parseGatewayEventId, saveLayout } from '../api/adapters'
import { errorMessage, isConflict, isNotImplemented, isUnauthorized } from '../api/error'
import {
  analyzeArtifact,
  lookupEntity,
  searchQueue,
} from '../api/search'
import { appendCondition, alignGroupValues, astToFilterChips, defaultQuery, drillGroupValues, entityKindForField, findingUuidFromAst, findingUuidQuery, parseQueuePdql, serialize, type FindingFilterField } from '../lib/pdql'
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
  deleteInvestigation as deleteInvestigationRequest,
  resolveSomCatalog,
  reviewEdges,
  runSomIssue,
  type SomCatalog,
} from '../api/ir'
import {
  addHypothesisContext,
  addHypothesisEdge,
  addHypothesisNode,
  createHypothesis as createHypothesisRequest,
  deleteHypothesis as deleteHypothesisRequest,
  getHypothesis,
  getHypothesisGraph,
  listHypotheses,
  patchHypothesis as patchHypothesisRequest,
  removeHypothesisEdge,
  removeHypothesisNode,
  type Hypothesis,
  type HypothesisStatus,
} from '../api/hypotheses'
import {
  edgesBetweenNodes,
  layerItemIds,
  membershipFromGraph,
  mergeVisibleLayerIds,
  nodeIdsForEntityRefs,
  toggleLayerId,
  type HypothesisMembership,
} from '../lib/hypotheses'
import type { components as Ir } from '@ir/contract'

export type SidebarSectionId = 'agent' | 'hypotheses'

export type TabId = 'queue' | 'investigations' | string

export const PINNED_TABS = ['queue', 'investigations'] as const

export function isPinnedTab(tab: TabId): tab is (typeof PINNED_TABS)[number] {
  return tab === 'queue' || tab === 'investigations'
}

export const DEFAULT_INVESTIGATION_FILTER: InvestigationListFilter = {
  status: 'all',
  severity: 'all',
  q: '',
}

const DEFAULT_QUEUE_PDQL = serialize(defaultQuery())
const DEFAULT_TIME_INTERVAL = demoDayInterval()
const HISTORY_LIMIT = 8

export const emptyContextQueue: ContextQueueState = {
  chips: [],
  pdql: DEFAULT_QUEUE_PDQL,
  timeInterval: DEFAULT_TIME_INTERVAL,
  queueSource: DEFAULT_QUEUE_SOURCE,
  groupValues: [],
  eventGroups: [],
  executedFingerprint: null,
  queryHistory: [],
  findingFilterWarnAt: 0,
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
  const key = filterFingerprint(
    entry.pdql,
    entry.timeInterval,
    entry.queueSource,
    entry.groupValues,
  )
  return [
    entry,
    ...history.filter(
      (item) =>
        filterFingerprint(item.pdql, item.timeInterval, item.queueSource, item.groupValues) !== key,
    ),
  ].slice(0, HISTORY_LIMIT)
}

interface AppState {
  chips: FilterChip[]
  timeInterval: TimeInterval
  queuePdql: string
  queueSource: QueueSource
  groupValues: (string | null)[]
  eventGroups: EventGroupItem[]
  executedFingerprint: string | null
  queryHistory: QueryHistoryEntry[]
  findingFilterWarnAt: number
  selectedAlertIds: string[]
  expandedCorrelationIds: string[]
  inspectedQueueItem: QueueItem | null

  tabs: TabId[]
  activeTab: TabId
  investigations: Record<string, Investigation>
  investigationRootIds: string[]
  investigationsNextCursor: string | null
  investigationsLoading: boolean
  investigationDeletingId: string | null
  investigationFilters: InvestigationListFilter
  expandedInvestigationIds: string[]
  investigationChildren: Record<string, string[]>
  investigationChildrenLoading: Record<string, boolean>
  issues: Record<string, Issue>
  eventReviews: Record<string, ReviewState>
  nodeReviews: Record<string, ReviewState>
  edgeReviews: Record<string, ReviewState>
  findingReviews: Record<string, ReviewState>
  actionResults: Record<string, ActionResult[]>
  contextQueue: Record<string, ContextQueueState>
  agentPanelOpen: boolean
  sidebarSection: SidebarSectionId | null
  hypothesisDraftOpen: boolean
  hypotheses: Record<string, Hypothesis>
  hypothesisMembership: Record<string, HypothesisMembership>
  activeHypothesisId: Record<string, string | null>
  visibleHypothesisIds: Record<string, string[]>
  highlightedHypothesisIds: Record<string, string[]>
  detailPanelOpen: boolean

  alerts: Record<string, AlertEvent>
  correlations: Record<string, CorrelationGroup>
  queueOrder: QueueItem[]
  /** Client-side text filter for the global queue list (header search). */
  queueTextFilter: string
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
  setQueueTextFilter: (q: string) => void
  setTimeInterval: (interval: TimeInterval) => void
  setQueueSource: (source: QueueSource) => void
  applyQueueHistory: (entry: QueryHistoryEntry) => void
  drillGroupValue: (investigationId: string | null, field: string, value: string) => void
  selectGroupValue: (investigationId: string | null, value: string | null) => void
  clearGroupSelection: (investigationId: string | null) => void
  clearGroupPathFrom: (investigationId: string | null, index: number) => void
  toggleAlertSelect: (id: string) => void
  clearAlertSelection: () => void
  toggleCorrelationExpand: (id: string) => void
  inspectQueueItem: (item: QueueItem | null) => void

  setActiveTab: (tab: TabId) => void
  closeTab: (tab: TabId) => void
  openInvestigationTab: (id: string) => void
  startInvestigation: (alertOrCorrIds: string[], title: string) => Promise<string>
  createChildInvestigation: (parentId: string, entityIds: string[]) => Promise<string>
  updateInvestigation: (id: string, patch: Partial<Investigation>) => void
  persistInvestigation: (id: string, patch: Partial<Investigation>) => Promise<boolean>
  loadQueue: () => Promise<void>
  bootstrap: () => Promise<void>
  loadInvestigation: (id: string) => Promise<void>
  loadInvestigationList: (reset?: boolean) => Promise<void>
  setInvestigationFilter: (patch: Partial<InvestigationListFilter>) => Promise<void>
  toggleInvestigationExpand: (id: string) => Promise<void>
  deleteInvestigation: (id: string) => Promise<void>
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
  filterByFindingUuid: (
    investigationId: string | null,
    uuid: string,
    recordType: FindingFilterField,
  ) => void
  warnFindingFilterExclusive: (investigationId: string | null) => void
  addFieldToContext: (
    investigationId: string,
    input: { field: string; value: string; eventId: string; includeEvent: boolean },
  ) => Promise<void>
  setAgentPanelOpen: (open: boolean) => void
  setSidebarSection: (section: SidebarSectionId | null) => void
  setHypothesisDraftOpen: (open: boolean) => void
  setDetailPanelOpen: (open: boolean) => void
  loadSomCatalog: () => Promise<void>
  openAgentPanel: () => Promise<void>
  loadHypotheses: (investigationId: string) => Promise<void>
  createHypothesis: (
    investigationId: string,
    input: { statement: string; description?: string; includeSelection?: boolean },
  ) => Promise<Hypothesis | null>
  createHypothesisFromEvents: (investigationId: string, eventIds: string[]) => Promise<Hypothesis | null>
  patchHypothesis: (
    investigationId: string,
    hypothesisId: string,
    patch: { statement?: string; description?: string; status?: HypothesisStatus; reason?: string },
  ) => Promise<Hypothesis | null>
  deleteHypothesis: (investigationId: string, hypothesisId: string) => Promise<void>
  setActiveHypothesis: (investigationId: string, hypothesisId: string | null) => Promise<void>
  toggleHypothesisVisible: (investigationId: string, itemId: string, solo?: boolean) => void
  toggleHypothesisHighlight: (investigationId: string, itemId: string, solo?: boolean) => void
  addSelectionToHypothesis: (investigationId: string, hypothesisId: string) => Promise<void>
  addEventsToActiveHypothesis: (investigationId: string, eventIds: string[]) => Promise<void>
  toggleHypothesisNode: (investigationId: string, nodeId: string) => Promise<void>
  toggleHypothesisEdge: (investigationId: string, edgeId: string) => Promise<void>

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
        seedEventIds: bundle.investigation.seedEventIds,
        issueIds: keepView?.issueIds ?? bundle.investigation.issueIds,
        hypothesisIds:
          keepView?.hypothesisIds ??
          get().investigations[bundle.investigation.id]?.hypothesisIds ??
          bundle.investigation.hypothesisIds,
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

function listQueryFromFilters(
  filters: InvestigationListFilter,
  extra: { parentId?: string; cursor?: string } = {},
) {
  return {
    parentId: extra.parentId,
    cursor: extra.cursor,
    status: filters.status === 'all' ? undefined : filters.status,
    severity: filters.severity === 'all' ? undefined : filters.severity,
    q: filters.q,
    limit: 100,
  }
}

function mergeListedInvestigation(
  existing: Investigation | undefined,
  listed: Investigation,
): Investigation {
  if (!existing) return listed
  return {
    ...existing,
    title: listed.title,
    severity: listed.severity,
    status: listed.status,
    parentId: listed.parentId,
    createdAt: listed.createdAt,
    updatedAt: listed.updatedAt,
    closedAt: listed.closedAt,
    description: listed.description,
    verdict: listed.verdict,
    verdictReason: listed.verdictReason,
    counters: listed.counters,
    version: listed.version,
    somWorkspaceIds: listed.somWorkspaceIds,
  }
}

function mergeListed(
  current: Record<string, Investigation>,
  items: Investigation[],
): Record<string, Investigation> {
  const next = { ...current }
  for (const item of items) next[item.id] = mergeListedInvestigation(next[item.id], item)
  return next
}

function uniqueAppend(ids: string[], extra: string[]): string[] {
  const seen = new Set(ids)
  const next = [...ids]
  for (const id of extra) {
    if (seen.has(id)) continue
    seen.add(id)
    next.push(id)
  }
  return next
}

function prependId(ids: string[], id: string): string[] {
  return [id, ...ids.filter((item) => item !== id)]
}

function collectSubtreeIds(
  rootId: string,
  investigations: Record<string, Investigation>,
  children: Record<string, string[]>,
): string[] {
  const ids = new Set<string>([rootId])
  const queue = [rootId]
  while (queue.length) {
    const current = queue.pop()!
    for (const childId of children[current] ?? []) {
      if (ids.has(childId)) continue
      ids.add(childId)
      queue.push(childId)
    }
    for (const inv of Object.values(investigations)) {
      if (inv.parentId !== current || ids.has(inv.id)) continue
      ids.add(inv.id)
      queue.push(inv.id)
    }
  }
  return [...ids]
}

export const useAppStore = create<AppState>((set, get) => ({
  chips: [],
  timeInterval: DEFAULT_TIME_INTERVAL,
  queuePdql: DEFAULT_QUEUE_PDQL,
  queueSource: DEFAULT_QUEUE_SOURCE,
  groupValues: [],
  eventGroups: [],
  executedFingerprint: null,
  queryHistory: [],
  findingFilterWarnAt: 0,
  selectedAlertIds: [],
  expandedCorrelationIds: [],
  inspectedQueueItem: null,

  tabs: ['queue', 'investigations'],
  activeTab: 'queue',
  investigations: {},
  investigationRootIds: [],
  investigationsNextCursor: null,
  investigationsLoading: false,
  investigationDeletingId: null,
  investigationFilters: DEFAULT_INVESTIGATION_FILTER,
  expandedInvestigationIds: [],
  investigationChildren: {},
  investigationChildrenLoading: {},
  issues: {},
  eventReviews: {},
  nodeReviews: {},
  edgeReviews: {},
  findingReviews: {},
  actionResults: {},
  contextQueue: {},
  agentPanelOpen: false,
  sidebarSection: null,
  hypothesisDraftOpen: false,
  hypotheses: {},
  hypothesisMembership: {},
  activeHypothesisId: {},
  visibleHypothesisIds: {},
  highlightedHypothesisIds: {},
  detailPanelOpen: false,

  alerts: {},
  correlations: {},
  queueOrder: [],
  queueTextFilter: '',
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
    const parsed = parseQueuePdql(get().queuePdql)
    if (parsed.ok && findingUuidFromAst(parsed.ast)) {
      get().warnFindingFilterExclusive(null)
      return
    }
    set({
      queuePdql: appendCondition(get().queuePdql, pdqlFieldForFilterField(field), '=', value),
    })
  },
  setQueuePdql: (queuePdql) => {
    const parsed = parseQueuePdql(queuePdql)
    set({
      queuePdql,
      groupValues: parsed.ok ? alignGroupValues(parsed.ast, get().groupValues) : get().groupValues,
    })
  },
  setQueueTextFilter: (queueTextFilter) => set({ queueTextFilter }),
  setTimeInterval: (timeInterval) => set({ timeInterval }),
  setQueueSource: (queueSource) => set({ queueSource }),
  applyQueueHistory: (entry) =>
    set({
      queuePdql: entry.pdql,
      timeInterval: entry.timeInterval,
      queueSource: entry.queueSource ?? DEFAULT_QUEUE_SOURCE,
      groupValues: entry.groupValues ?? [],
    }),
  drillGroupValue: (investigationId, field, value) => {
    if (!investigationId) {
      const parsed = parseQueuePdql(get().queuePdql)
      if (parsed.ok === false) return
      const next = drillGroupValues(parsed.ast, get().groupValues, field, value)
      if (!next) return
      set({ groupValues: next })
      void get().loadQueue()
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    const parsed = parseQueuePdql(cur.pdql)
    if (parsed.ok === false) return
    const next = drillGroupValues(parsed.ast, cur.groupValues, field, value)
    if (!next) return
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, groupValues: next },
      },
    })
    void get().executeContextQuery(investigationId)
  },
  selectGroupValue: (investigationId, value) => {
    const nextFor = (current: (string | null)[]) =>
      current.length === 1 && current[0] === value ? [] : [value]
    if (!investigationId) {
      set({ groupValues: nextFor(get().groupValues) })
      void get().loadQueue()
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, groupValues: nextFor(cur.groupValues) },
      },
    })
    void get().executeContextQuery(investigationId)
  },
  clearGroupSelection: (investigationId) => {
    if (!investigationId) {
      set({ groupValues: [] })
      void get().loadQueue()
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, groupValues: [] },
      },
    })
    void get().executeContextQuery(investigationId)
  },
  clearGroupPathFrom: (investigationId, index) => {
    if (!investigationId) {
      set({ groupValues: get().groupValues.slice(0, index) })
      void get().loadQueue()
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          groupValues: cur.groupValues.slice(0, index),
        },
      },
    })
    void get().executeContextQuery(investigationId)
  },
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
    if (isPinnedTab(tab)) return
    const tabs = get().tabs.filter((t) => t !== tab)
    const activeTab =
      get().activeTab === tab ? (tabs[tabs.length - 1] ?? 'investigations') : get().activeTab
    set({ tabs, activeTab })
  },
  openInvestigationTab: (id) => {
    if (isPinnedTab(id)) {
      set({ activeTab: id })
      return
    }
    const tabs = get().tabs
    if (tabs.includes(id)) {
      set({ activeTab: id })
      return
    }
    set({ tabs: [...tabs, id], activeTab: id })
  },

  clearError: () => set({ lastError: null, lastNotImplemented: null }),

  bootstrap: async () => {
    await get().loadInvestigationList(true)
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
    const timeInterval = get().timeInterval
    const queueSource = get().queueSource
    const groupValues = alignGroupValues(parsed.ast, get().groupValues)
    set({ queueLoading: true, lastError: null, mockSources: [], groupValues })
    try {
      const result = await searchQueue(parsed.ast, timeInterval, queueSource, groupValues)
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
        groupValues,
        executedFingerprint: filterFingerprint(canonical, timeInterval, queueSource, groupValues),
        queryHistory: pushQueryHistory(get().queryHistory, {
          pdql: canonical,
          timeInterval,
          queueSource,
          groupValues,
        }),
        alerts: result.alerts,
        correlations: result.correlations,
        queueOrder: result.queueOrder,
        eventGroups: result.eventGroups,
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
      void get().loadHypotheses(id)
    } catch (err) {
      set({ investigationLoading: false, lastError: errorMessage(err) })
    }
  },

  loadInvestigationList: async (reset = true) => {
    const filters = get().investigationFilters
    const cursor = reset ? undefined : (get().investigationsNextCursor ?? undefined)
    if (!reset && !cursor) return
    set({ investigationsLoading: true, lastError: null })
    try {
      const page = await listInvestigations(listQueryFromFilters(filters, { cursor }))
      const ids = page.items.map((item) => item.id)
      set({
        investigations: mergeListed(get().investigations, page.items),
        investigationRootIds: reset
          ? ids
          : uniqueAppend(get().investigationRootIds, ids),
        investigationsNextCursor: page.nextCursor,
        investigationsLoading: false,
        ...(reset
          ? { expandedInvestigationIds: [], investigationChildren: {}, investigationChildrenLoading: {} }
          : {}),
      })
    } catch (err) {
      set({ investigationsLoading: false, lastError: errorMessage(err) })
    }
  },

  setInvestigationFilter: async (patch) => {
    set({ investigationFilters: { ...get().investigationFilters, ...patch } })
    await get().loadInvestigationList(true)
  },

  toggleInvestigationExpand: async (id) => {
    const expanded = get().expandedInvestigationIds
    if (expanded.includes(id)) {
      set({ expandedInvestigationIds: expanded.filter((item) => item !== id) })
      return
    }
    set({ expandedInvestigationIds: [...expanded, id] })
    if (get().investigationChildren[id]) return
    set({
      investigationChildrenLoading: { ...get().investigationChildrenLoading, [id]: true },
    })
    try {
      const page = await listInvestigations(
        listQueryFromFilters(get().investigationFilters, { parentId: id }),
      )
      set({
        investigations: mergeListed(get().investigations, page.items),
        investigationChildren: {
          ...get().investigationChildren,
          [id]: page.items.map((item) => item.id),
        },
        investigationChildrenLoading: { ...get().investigationChildrenLoading, [id]: false },
      })
    } catch (err) {
      set({
        investigationChildrenLoading: { ...get().investigationChildrenLoading, [id]: false },
        lastError: errorMessage(err),
      })
    }
  },

  deleteInvestigation: async (id) => {
    if (isPinnedTab(id) || get().investigationDeletingId) return
    const target = get().investigations[id]
    if (!target) return
    const parentId = target.parentId
    const removedIds = collectSubtreeIds(id, get().investigations, get().investigationChildren)
    const removed = new Set(removedIds)
    set({ investigationDeletingId: id, lastError: null, lastNotImplemented: null })
    try {
      await deleteInvestigationRequest(id)
      const investigations = { ...get().investigations }
      for (const removedId of removedIds) delete investigations[removedId]
      if (parentId && investigations[parentId]?.counters) {
        const parent = investigations[parentId]
        investigations[parentId] = {
          ...parent,
          counters: {
            ...parent.counters!,
            children: Math.max(0, parent.counters!.children - 1),
          },
        }
      }
      const investigationChildren: Record<string, string[]> = {}
      for (const [parent, childIds] of Object.entries(get().investigationChildren)) {
        if (removed.has(parent)) continue
        investigationChildren[parent] = childIds.filter((childId) => !removed.has(childId))
      }
      const investigationChildrenLoading = { ...get().investigationChildrenLoading }
      for (const removedId of removedIds) delete investigationChildrenLoading[removedId]
      const contextQueue = { ...get().contextQueue }
      for (const removedId of removedIds) delete contextQueue[removedId]
      const tabs = get().tabs.filter((tab) => !removed.has(tab))
      const activeTab = removed.has(get().activeTab)
        ? (tabs[tabs.length - 1] ?? 'investigations')
        : get().activeTab
      set({
        investigations,
        investigationRootIds: get().investigationRootIds.filter((rootId) => !removed.has(rootId)),
        investigationChildren,
        investigationChildrenLoading,
        expandedInvestigationIds: get().expandedInvestigationIds.filter(
          (expandedId) => !removed.has(expandedId),
        ),
        contextQueue,
        tabs,
        activeTab,
        investigationDeletingId: null,
      })
    } catch (err) {
      if (isNotImplemented(err)) {
        set({ investigationDeletingId: null, lastNotImplemented: errorMessage(err) })
        return
      }
      set({ investigationDeletingId: null, lastError: errorMessage(err) })
    }
  },

  startInvestigation: async (ids, title) => {
    const trimmed = title.trim().slice(0, 255)
    if (!trimmed) return ''
    const { alerts, correlations } = get()
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
      const created = await createInvestigation({ title: trimmed, severity })
      const refs = contextRefsFromIds(ids, alerts, correlations, get().contextEvents)
      if (refs.events.length || refs.findings.length)
        await addContext(created.id, { ...refs, seed: true })
      const bundle = await loadInvestigationBundle(created.id, {
        seedEventIds: ids,
        view: 'graph',
      })
      set({
        ...applyBundle(get, bundle),
        tabs: get().tabs.includes(created.id) ? get().tabs : [...get().tabs, created.id],
        activeTab: created.id,
        investigationRootIds: prependId(get().investigationRootIds, created.id),
        selectedAlertIds: [],
        inspectedQueueItem: null,
        investigationLoading: false,
        contextQueue: {
          ...get().contextQueue,
          [created.id]: {
            ...emptyContextQueue,
            chips: get().chips,
            pdql: get().queuePdql,
            timeInterval: get().timeInterval,
            queueSource: get().queueSource,
            groupValues: get().groupValues,
            queryHistory: get().queryHistory,
          },
        },
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
      const parentQueue = get().contextQueue[parentId]
      const inheritedQueue = parentQueue ?? {
        ...emptyContextQueue,
        chips: get().chips,
        pdql: get().queuePdql,
        timeInterval: get().timeInterval,
        queueSource: get().queueSource,
        groupValues: get().groupValues,
        queryHistory: get().queryHistory,
      }
      const children = get().investigationChildren[parentId]
      const bundled = applyBundle(get, bundle)
      const parentInv = bundled.investigations[parentId]
      set({
        ...bundled,
        tabs: get().tabs.includes(created.id) ? get().tabs : [...get().tabs, created.id],
        activeTab: created.id,
        investigationLoading: false,
        investigationChildren: children
          ? {
              ...get().investigationChildren,
              [parentId]: prependId(children, created.id),
            }
          : get().investigationChildren,
        investigations: parentInv
          ? {
              ...bundled.investigations,
              [parentId]: {
                ...parentInv,
                counters: parentInv.counters
                  ? { ...parentInv.counters, children: parentInv.counters.children + 1 }
                  : parentInv.counters,
              },
            }
          : bundled.investigations,
        contextQueue: {
          ...get().contextQueue,
          [created.id]: {
            ...emptyContextQueue,
            chips: inheritedQueue.chips,
            pdql: inheritedQueue.pdql,
            timeInterval: inheritedQueue.timeInterval,
            queueSource: inheritedQueue.queueSource,
            groupValues: inheritedQueue.groupValues,
            queryHistory: inheritedQueue.queryHistory,
          },
        },
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
      return true
    }
    try {
      const status =
        patch.status === 'closed' || patch.status === 'in_progress' || patch.status === 'open'
          ? patch.status
          : undefined
      const updated = await patchInvestigation(id, {
        version: inv.version,
        title: patch.title,
        status,
        severity: patch.severity && patch.severity !== 'info' ? patch.severity : undefined,
        verdict: patch.verdict,
        verdict_reason: patch.verdictReason ?? undefined,
      })
      get().updateInvestigation(id, {
        status: updated.status,
        verdict: updated.verdict,
        verdictReason: updated.verdict_reason,
        closedAt: updated.closed_at,
        updatedAt: updated.updated_at,
        version: updated.version,
        ...(patch.title != null ? { title: patch.title } : {}),
        ...(patch.severity != null ? { severity: patch.severity } : {}),
      })
      return true
    } catch (err) {
      if (isNotImplemented(err)) {
        set({ lastNotImplemented: errorMessage(err) })
        return false
      }
      set({ lastError: errorMessage(err) })
      return false
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
    const next = { ...cur, ...patch }
    if (patch.pdql != null && patch.groupValues == null) {
      const parsed = parseQueuePdql(next.pdql)
      if (parsed.ok) next.groupValues = alignGroupValues(parsed.ast, next.groupValues)
    }
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: next,
      },
    })
  },

  addContextChip: (investigationId, field, value) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    const parsed = parseQueuePdql(cur.pdql)
    if (parsed.ok && findingUuidFromAst(parsed.ast)) {
      get().warnFindingFilterExclusive(investigationId)
      return
    }
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
    const timeInterval = cur.timeInterval
    const queueSource = cur.queueSource
    const groupValues = alignGroupValues(parsed.ast, cur.groupValues)
    const canonical = serialize(parsed.ast)
    set({
      lastError: null,
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, chips, pdql: canonical, groupValues, loading: true },
      },
    })
    try {
      const result = await searchQueue(parsed.ast, timeInterval, queueSource, groupValues)
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
            groupValues,
            eventGroups: result.eventGroups,
            alerts: result.alerts,
            queueOrder: result.queueOrder,
            loading: false,
            executedFingerprint: filterFingerprint(
              canonical,
              timeInterval,
              queueSource,
              groupValues,
            ),
            queryHistory: pushQueryHistory(latest.queryHistory, {
              pdql: canonical,
              timeInterval,
              queueSource,
              groupValues,
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
    const pdql = investigationId
      ? (get().contextQueue[investigationId] ?? emptyContextQueue).pdql
      : get().queuePdql
    const queueSource = investigationId
      ? (get().contextQueue[investigationId] ?? emptyContextQueue).queueSource
      : get().queueSource
    const parsed = parseQueuePdql(pdql)
    if (parsed.ok && findingUuidFromAst(parsed.ast)) {
      get().warnFindingFilterExclusive(investigationId)
      return
    }
    if (queueSource === 'events' && parsed.ok && parsed.ast.groups.some((group) => group.field === field)) {
      get().drillGroupValue(investigationId, field, value)
      return
    }
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

  filterByFindingUuid: (investigationId, uuid, recordType) => {
    const value = uuid.trim()
    if (!value) return
    const pdql = findingUuidQuery(value, recordType)
    if (!investigationId) {
      set({ queuePdql: pdql, queueSource: 'events', groupValues: [], eventGroups: [] })
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          pdql,
          queueSource: 'events',
          groupValues: [],
          eventGroups: [],
        },
      },
    })
  },

  warnFindingFilterExclusive: (investigationId) => {
    const now = Date.now()
    if (!investigationId) {
      set({ findingFilterWarnAt: now })
      return
    }
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, findingFilterWarnAt: now },
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

  setAgentPanelOpen: (open) => {
    if (open) {
      set({ sidebarSection: 'agent', agentPanelOpen: true })
      return
    }
    if (get().sidebarSection === 'agent') {
      set({ sidebarSection: null, agentPanelOpen: false })
    }
  },
  setSidebarSection: (section) => {
    set({
      sidebarSection: section,
      agentPanelOpen: section === 'agent',
    })
    if (section === 'agent') void get().loadSomCatalog()
    if (section === 'hypotheses') {
      const id = get().activeTab
      if (id !== 'queue') void get().loadHypotheses(id)
    }
  },
  setHypothesisDraftOpen: (hypothesisDraftOpen) => set({ hypothesisDraftOpen }),
  setDetailPanelOpen: (detailPanelOpen) => set({ detailPanelOpen }),

  loadSomCatalog: async () => {
    try {
      set({ somCatalog: await resolveSomCatalog() })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  openAgentPanel: async () => {
    set({ sidebarSection: 'agent', agentPanelOpen: true })
    if (!get().somCatalog) await get().loadSomCatalog()
  },

  runEnrichment: async (investigationId, issueId) => {
    const inv = get().investigations[investigationId]
    if (!inv || !issueId) return
    set({ sidebarSection: 'agent', agentPanelOpen: true, lastError: null, somHint: null })
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

  loadHypotheses: async (investigationId) => {
    try {
      const items = await listHypotheses(investigationId)
      const hypotheses = { ...get().hypotheses }
      for (const item of items) hypotheses[item.id] = item
      const inv = get().investigations[investigationId]
      const hypothesisIds = items.map((item) => item.id)
      set({
        hypotheses,
        investigations: inv
          ? {
              ...get().investigations,
              [investigationId]: {
                ...inv,
                hypothesisIds,
              },
            }
          : get().investigations,
        visibleHypothesisIds: {
          ...get().visibleHypothesisIds,
          [investigationId]: mergeVisibleLayerIds(
            get().visibleHypothesisIds[investigationId],
            hypothesisIds,
          ),
        },
        highlightedHypothesisIds: {
          ...get().highlightedHypothesisIds,
          [investigationId]: (get().highlightedHypothesisIds[investigationId] ?? []).filter((id) =>
            layerItemIds(hypothesisIds).includes(id),
          ),
        },
      })
      const graphs = await Promise.all(
        items.map(async (item) => {
          try {
            return [item.id, membershipFromGraph(await getHypothesisGraph(investigationId, item.id))] as const
          } catch {
            return null
          }
        }),
      )
      const membership = { ...get().hypothesisMembership }
      for (const entry of graphs) {
        if (!entry) continue
        membership[entry[0]] = entry[1]
      }
      set({ hypothesisMembership: membership })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  createHypothesis: async (investigationId, input) => {
    const statement = input.statement.trim()
    if (!statement) return null
    set({ lastError: null })
    try {
      const created = await createHypothesisRequest(investigationId, {
        statement,
        description: input.description?.trim() || undefined,
      })
      const inv = get().investigations[investigationId]
      const hypothesisIds = inv
        ? [created.id, ...inv.hypothesisIds.filter((id) => id !== created.id)]
        : [created.id]
      const prevVisible = get().visibleHypothesisIds[investigationId]
      set({
        hypotheses: { ...get().hypotheses, [created.id]: created },
        investigations: inv
          ? {
              ...get().investigations,
              [investigationId]: {
                ...inv,
                hypothesisIds,
              },
            }
          : get().investigations,
        visibleHypothesisIds: {
          ...get().visibleHypothesisIds,
          [investigationId]: prevVisible
            ? prevVisible.includes(created.id)
              ? prevVisible
              : [...prevVisible, created.id]
            : layerItemIds(hypothesisIds),
        },
        hypothesisDraftOpen: false,
        sidebarSection: 'hypotheses',
        agentPanelOpen: false,
      })
      const activated = await get().patchHypothesis(investigationId, created.id, {
        status: 'active',
      })
      if (input.includeSelection) {
        await get().addSelectionToHypothesis(investigationId, created.id)
      }
      await get().setActiveHypothesis(investigationId, created.id)
      return activated ?? get().hypotheses[created.id] ?? created
    } catch (err) {
      set({ lastError: errorMessage(err) })
      return null
    }
  },

  createHypothesisFromEvents: async (investigationId, eventIds) => {
    const queue = get().contextQueue[investigationId]
    const alerts = { ...get().alerts, ...queue?.alerts }
    const first = eventIds
      .map((id) => alerts[id] ?? get().contextEvents[id])
      .find((item) => item != null)
    const statement = (first?.title ?? '').trim().slice(0, 255) || 'Новая гипотеза'
    const created = await get().createHypothesis(investigationId, { statement })
    if (!created) return null
    await get().addEventsToActiveHypothesis(investigationId, eventIds)
    return created
  },

  patchHypothesis: async (investigationId, hypothesisId, patch) => {
    const current = get().hypotheses[hypothesisId]
    if (!current) return null
    set({ lastError: null })
    try {
      const updated = await patchHypothesisRequest(investigationId, hypothesisId, {
        version: current.version,
        ...patch,
      })
      set({ hypotheses: { ...get().hypotheses, [updated.id]: updated } })
      return updated
    } catch (err) {
      if (isConflict(err)) {
        try {
          const fresh = await getHypothesis(investigationId, hypothesisId)
          set({
            hypotheses: { ...get().hypotheses, [fresh.id]: fresh },
            lastError: 'Карточка гипотезы изменилась — обновите и повторите',
          })
        } catch (refreshErr) {
          set({ lastError: errorMessage(refreshErr) })
        }
        return null
      }
      set({ lastError: errorMessage(err) })
      return null
    }
  },

  deleteHypothesis: async (investigationId, hypothesisId) => {
    set({ lastError: null })
    try {
      await deleteHypothesisRequest(investigationId, hypothesisId)
      const hypotheses = { ...get().hypotheses }
      delete hypotheses[hypothesisId]
      const membership = { ...get().hypothesisMembership }
      delete membership[hypothesisId]
      const inv = get().investigations[investigationId]
      const active = { ...get().activeHypothesisId }
      if (active[investigationId] === hypothesisId) active[investigationId] = null
      const visible = { ...get().visibleHypothesisIds }
      if (visible[investigationId]) {
        visible[investigationId] = visible[investigationId].filter((id) => id !== hypothesisId)
      }
      const highlighted = { ...get().highlightedHypothesisIds }
      if (highlighted[investigationId]) {
        highlighted[investigationId] = highlighted[investigationId].filter(
          (id) => id !== hypothesisId,
        )
      }
      set({
        hypotheses,
        hypothesisMembership: membership,
        activeHypothesisId: active,
        visibleHypothesisIds: visible,
        highlightedHypothesisIds: highlighted,
        investigations: inv
          ? {
              ...get().investigations,
              [investigationId]: {
                ...inv,
                hypothesisIds: inv.hypothesisIds.filter((id) => id !== hypothesisId),
              },
            }
          : get().investigations,
      })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  toggleHypothesisVisible: (investigationId, itemId, solo = false) => {
    const inv = get().investigations[investigationId]
    const allIds = layerItemIds(inv?.hypothesisIds ?? [])
    const current = get().visibleHypothesisIds[investigationId] ?? allIds
    set({
      visibleHypothesisIds: {
        ...get().visibleHypothesisIds,
        [investigationId]: toggleLayerId(current, itemId, solo, allIds),
      },
    })
  },

  toggleHypothesisHighlight: (investigationId, itemId, solo = false) => {
    const current = get().highlightedHypothesisIds[investigationId] ?? []
    set({
      highlightedHypothesisIds: {
        ...get().highlightedHypothesisIds,
        [investigationId]: toggleLayerId(current, itemId, solo),
      },
    })
  },

  setActiveHypothesis: async (investigationId, hypothesisId) => {
    const current = get().activeHypothesisId[investigationId] ?? null
    if (current === hypothesisId) return
    set({
      activeHypothesisId: { ...get().activeHypothesisId, [investigationId]: hypothesisId },
    })
    if (!hypothesisId) return
    try {
      const graph = await getHypothesisGraph(investigationId, hypothesisId)
      set({
        hypothesisMembership: {
          ...get().hypothesisMembership,
          [hypothesisId]: membershipFromGraph(graph),
        },
      })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  addEventsToActiveHypothesis: async (investigationId, eventIds) => {
    const hypothesisId = get().activeHypothesisId[investigationId]
    const hypothesis = hypothesisId ? get().hypotheses[hypothesisId] : null
    if (!hypothesisId || !hypothesis || hypothesis.status === 'resolved') return
    const queue = get().contextQueue[investigationId]
    const refs = contextRefsFromIds(
      eventIds,
      { ...get().alerts, ...queue?.alerts },
      get().correlations,
      get().contextEvents,
    )
    if (refs.events.length === 0 && refs.findings.length === 0) return
    set({ lastError: null })
    try {
      await addHypothesisContext(investigationId, hypothesisId, refs)
      const cur = get().contextQueue[investigationId] ?? emptyContextQueue
      set({
        contextQueue: {
          ...get().contextQueue,
          [investigationId]: { ...cur, selectedIds: [] },
        },
      })
      await get().loadInvestigation(investigationId)
      const graph = await getHypothesisGraph(investigationId, hypothesisId)
      set({
        hypothesisMembership: {
          ...get().hypothesisMembership,
          [hypothesisId]: membershipFromGraph(graph),
        },
      })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  addSelectionToHypothesis: async (investigationId, hypothesisId) => {
    const inv = get().investigations[investigationId]
    const hypothesis = get().hypotheses[hypothesisId]
    if (!inv || !hypothesis || hypothesis.status === 'resolved') return
    const nodeIds = nodeIdsForEntityRefs(inv.selectedEntityIds, get().graphNodes)
    const edgeIds = edgesBetweenNodes(nodeIds, get().graphEdges)
    set({ lastError: null })
    try {
      for (const nodeId of nodeIds) {
        await addHypothesisNode(investigationId, hypothesisId, nodeId)
      }
      for (const edgeId of edgeIds) {
        await addHypothesisEdge(investigationId, hypothesisId, edgeId)
      }
      const graph = await getHypothesisGraph(investigationId, hypothesisId)
      set({
        hypothesisMembership: {
          ...get().hypothesisMembership,
          [hypothesisId]: membershipFromGraph(graph),
        },
      })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  toggleHypothesisNode: async (investigationId, nodeId) => {
    const hypothesisId = get().activeHypothesisId[investigationId]
    const hypothesis = hypothesisId ? get().hypotheses[hypothesisId] : null
    if (!hypothesisId || !hypothesis || hypothesis.status === 'resolved') return
    const member = new Set(get().hypothesisMembership[hypothesisId]?.nodeIds ?? [])
    set({ lastError: null })
    try {
      if (member.has(nodeId)) {
        await removeHypothesisNode(investigationId, hypothesisId, nodeId)
      } else {
        await addHypothesisNode(investigationId, hypothesisId, nodeId)
      }
      const graph = await getHypothesisGraph(investigationId, hypothesisId)
      set({
        hypothesisMembership: {
          ...get().hypothesisMembership,
          [hypothesisId]: membershipFromGraph(graph),
        },
      })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
  },

  toggleHypothesisEdge: async (investigationId, edgeId) => {
    const hypothesisId = get().activeHypothesisId[investigationId]
    const hypothesis = hypothesisId ? get().hypotheses[hypothesisId] : null
    if (!hypothesisId || !hypothesis || hypothesis.status === 'resolved') return
    const member = new Set(get().hypothesisMembership[hypothesisId]?.edgeIds ?? [])
    set({ lastError: null })
    try {
      if (member.has(edgeId)) {
        await removeHypothesisEdge(investigationId, hypothesisId, edgeId)
      } else {
        await addHypothesisEdge(investigationId, hypothesisId, edgeId)
      }
      const graph = await getHypothesisGraph(investigationId, hypothesisId)
      set({
        hypothesisMembership: {
          ...get().hypothesisMembership,
          [hypothesisId]: membershipFromGraph(graph),
        },
      })
    } catch (err) {
      set({ lastError: errorMessage(err) })
    }
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
