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
  ReviewState,
} from '../types'
import { uid } from '../lib/utils'
import { defaultFilterValueOptions, issueTemplates, savedViews } from '../lib/catalog'
import { parseGatewayEventId, saveLayout } from '../api/adapters'
import { errorMessage, isNotImplemented, isUnauthorized } from '../api/error'
import {
  analyzeArtifact,
  lookupEntity,
  searchQueue,
} from '../api/search'
import {
  addContext,
  countProposedAgentEdges,
  createInvestigation,
  getEntityCard,
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

export const emptyContextQueue: ContextQueueState = {
  chips: [],
  timePreset: '30d',
  history: [],
  selectedIds: [],
  hideAdded: false,
  originFilter: 'all',
  reviewFilter: 'all',
}

interface AppState {
  chips: FilterChip[]
  timePreset: string
  queueQuery: string
  selectedAlertIds: string[]
  expandedCorrelationIds: string[]

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

  queueLoading: boolean
  investigationLoading: boolean
  lastError: string | null
  lastNotImplemented: string | null
  somHint: string | null
  somCatalog: SomCatalog | null

  setChips: (chips: FilterChip[]) => void
  addChip: (field: FilterField, value: string) => void
  removeChip: (id: string) => void
  removeChipValue: (id: string, value: string) => void
  setTimePreset: (preset: string) => void
  setQueueQuery: (query: string) => void
  applySavedView: (viewId: string) => void
  toggleAlertSelect: (id: string) => void
  clearAlertSelection: () => void
  toggleCorrelationExpand: (id: string) => void

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
  removeContextChip: (investigationId: string, chipId: string) => void
  removeContextChipValue: (investigationId: string, chipId: string, value: string) => void
  clearContextChips: (investigationId: string) => void
  addEventsToContext: (investigationId: string, eventIds: string[]) => Promise<void>
  setAgentPanelOpen: (open: boolean) => void
  setDetailPanelOpen: (open: boolean) => void

  runEnrichment: (investigationId: string) => Promise<void>
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

function mergeChipValue(
  chips: FilterChip[],
  field: FilterField,
  value: string,
): FilterChip[] {
  const existing = chips.find((c) => c.field === field)
  if (!existing) return [...chips, { id: uid('chip'), field, values: [value] }]
  if (existing.values.includes(value)) return chips
  return chips.map((c) =>
    c.id === existing.id ? { ...c, values: [...c.values, value] } : c,
  )
}

function mergeEntities(
  current: Record<string, Entity>,
  incoming: Record<string, Entity>,
): Record<string, Entity> {
  return { ...current, ...incoming }
}

function sourceRefsFromIds(
  ids: string[],
  alerts: Record<string, AlertEvent>,
  correlations: Record<string, CorrelationGroup>,
  contextEvents: Record<string, ContextEvent>,
): Ir['schemas']['EventSourceRef'][] {
  const refs: Ir['schemas']['EventSourceRef'][] = []
  const seen = new Set<string>()
  const push = (source: string, sourceEventId: string | undefined, fallbackId: string) => {
    const parsed = parseGatewayEventId(fallbackId)
    const source_code = source || parsed?.source_code
    const source_event_id = sourceEventId || parsed?.source_event_id
    if (!source_code || !source_event_id) return
    const key = `${source_code}/${source_event_id}`
    if (seen.has(key)) return
    seen.add(key)
    refs.push({ source_code, source_event_id })
  }
  for (const id of ids) {
    if (correlations[id]) {
      for (const eid of correlations[id].eventIds) {
        const a = alerts[eid] ?? contextEvents[eid]
        if (a) push(a.source, a.sourceEventId, a.id)
      }
      continue
    }
    const a = alerts[id] ?? contextEvents[id]
    if (a) push(a.source, a.sourceEventId, a.id)
  }
  return refs
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
        findingIds: keepView?.findingIds ?? bundle.investigation.findingIds,
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
  timePreset: '30d',
  queueQuery: 'impacket_smbexec',
  selectedAlertIds: [],
  expandedCorrelationIds: [],

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

  queueLoading: false,
  investigationLoading: false,
  lastError: null,
  lastNotImplemented: null,
  somHint: null,
  somCatalog: null,

  setChips: (chips) => {
    set({ chips })
    void get().loadQueue()
  },
  addChip: (field, value) => {
    set({ chips: mergeChipValue(get().chips, field, value) })
    void get().loadQueue()
  },
  removeChip: (id) => {
    set({ chips: get().chips.filter((c) => c.id !== id) })
    void get().loadQueue()
  },
  removeChipValue: (id, value) => {
    set({
      chips: get()
        .chips.map((c) =>
          c.id === id ? { ...c, values: c.values.filter((v) => v !== value) } : c,
        )
        .filter((c) => c.values.length > 0),
    })
    void get().loadQueue()
  },
  setTimePreset: (timePreset) => {
    set({ timePreset })
    void get().loadQueue()
  },
  setQueueQuery: (queueQuery) => {
    set({ queueQuery })
    void get().loadQueue()
  },
  applySavedView: (viewId) => {
    const view = savedViews.find((v) => v.id === viewId)
    if (!view) return
    set({
      chips: view.chips.map((c) => ({ ...c, id: uid('chip') })),
      timePreset: view.timePreset,
      queueQuery: view.query ?? '',
    })
    void get().loadQueue()
  },
  toggleAlertSelect: (id) => {
    const sel = get().selectedAlertIds
    set({
      selectedAlertIds: sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id],
    })
  },
  clearAlertSelection: () => set({ selectedAlertIds: [] }),
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
    set({ queueLoading: true, lastError: null })
    try {
      const result = await searchQueue(get().chips, get().timePreset, get().queueQuery)
      const hosts = new Set(get().filterValueOptions.host ?? [])
      const ips = new Set(get().filterValueOptions.ip ?? [])
      for (const e of Object.values(result.entities)) {
        if (e.kind === 'host') hosts.add(e.label)
        if (e.kind === 'ip') ips.add(e.label)
      }
      set({
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
        },
        queueLoading: false,
        lastError: result.sourceErrors.length ? result.sourceErrors.join('; ') : null,
      })
    } catch (err) {
      set({ queueLoading: false, lastError: errorMessage(err) })
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
      const refs = sourceRefsFromIds(ids, alerts, correlations, get().contextEvents)
      if (refs.length) await addContext(created.id, refs)
      const bundle = await loadInvestigationBundle(created.id, {
        seedEventIds: ids,
        view: 'graph',
      })
      set({
        ...applyBundle(get, bundle),
        tabs: [...get().tabs, created.id],
        activeTab: created.id,
        selectedAlertIds: [],
        investigationLoading: false,
      })
      void get().runEnrichment(created.id)
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
      const refs = sourceRefsFromIds(
        relatedEvents,
        get().alerts,
        get().correlations,
        get().contextEvents,
      )
      if (refs.length) await addContext(created.id, refs)
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
      void get().runEnrichment(created.id)
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
          chips: mergeChipValue(cur.chips, field, value),
          history: [
            { field, value },
            ...cur.history.filter((h) => !(h.field === field && h.value === value)),
          ].slice(0, 8),
        },
      },
    })
    void get().loadQueue()
  },

  removeContextChip: (investigationId, chipId) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          chips: cur.chips.filter((c) => c.id !== chipId),
        },
      },
    })
  },

  removeContextChipValue: (investigationId, chipId, value) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          chips: cur.chips
            .map((c) =>
              c.id === chipId ? { ...c, values: c.values.filter((v) => v !== value) } : c,
            )
            .filter((c) => c.values.length > 0),
        },
      },
    })
  },

  clearContextChips: (investigationId) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, chips: [] },
      },
    })
  },

  addEventsToContext: async (investigationId, eventIds) => {
    const refs = sourceRefsFromIds(
      eventIds,
      get().alerts,
      get().correlations,
      get().contextEvents,
    )
    if (refs.length === 0) return
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

  setAgentPanelOpen: (agentPanelOpen) => set({ agentPanelOpen }),
  setDetailPanelOpen: (detailPanelOpen) => set({ detailPanelOpen }),

  runEnrichment: async (investigationId) => {
    const inv = get().investigations[investigationId]
    if (!inv) return
    set({ agentPanelOpen: true, lastError: null, somHint: null })
    try {
      let catalog = get().somCatalog
      if (!catalog) {
        catalog = await resolveSomCatalog()
        set({ somCatalog: catalog })
      }
      const issueDef =
        catalog.issues.find((i) => i.simple_id.toLowerCase() === 'irw-2') ??
        catalog.issues[0]
      if (!issueDef) {
        set({
          somHint: 'В выбранной доске SOM нет issue — нечего запускать',
        })
        return
      }
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
      await runSomIssue(issueDef.id, investigationId)
      const deadline = Date.now() + 60_000
      const poll = async () => {
        const currentIssue = get().issues[issue.id]
        if (!currentIssue || currentIssue.status === 'cancelled') return
        try {
          const now = await countProposedAgentEdges(investigationId)
          if (now > before || Date.now() > deadline) {
            await get().loadInvestigation(investigationId)
            const latest = get().investigations[investigationId]
            const proposed = latest
              ? latest.edgeIds.filter(
                  (eid) => (get().edgeReviews[eid] ?? get().graphEdges[eid]?.review) === 'proposed',
                ).length
              : now
            set({
              issues: {
                ...get().issues,
                [issue.id]: {
                  ...currentIssue,
                  status: now > before ? 'completed' : 'completed',
                  edgesFound: Math.max(0, now - before),
                  resultSummary:
                    now > before
                      ? `Агент предложил ${proposed} связей. Нужно ревью.`
                      : 'Запуск принят. Новых proposed-связей за минуту не появилось — проверьте SOM.',
                },
              },
            })
            return
          }
        } catch {
          /* keep polling */
        }
        window.setTimeout(() => void poll(), 2500)
      }
      window.setTimeout(() => void poll(), 2500)
    } catch (err) {
      set({
        lastError: errorMessage(err),
        somHint: isUnauthorized(err)
          ? 'Обновите SOM-токен в шапке — он живёт около часа'
          : null,
      })
    }
  },

  createIssue: async (investigationId, templateId, entityIds, parentId) => {
    const tpl = issueTemplates.find((t) => t.id === templateId) ?? issueTemplates[0]
    await get().runEnrichment(investigationId)
    const inv = get().investigations[investigationId]
    if (!inv || !parentId) return
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
  const next = { ...nodes, [match.id]: { ...match, x: position.x, y: position.y } }
  useAppStore.setState({ graphNodes: next })
  const layout: Record<string, { x: number; y: number }> = {}
  for (const n of Object.values(next)) {
    if (useAppStore.getState().investigations[investigationId]?.nodeIds.includes(n.id)) {
      layout[n.id] = { x: n.x, y: n.y }
    }
  }
  saveLayout(investigationId, layout)
}
