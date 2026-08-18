import { create } from 'zustand'
import {
  ALL_ENTITY_TYPES,
  ALL_SEVERITIES,
} from '../components/graph/constants'
import type {
  AlertNode,
  Edge,
  EdgeOrigin,
  Entity,
  EntityTypeCode,
  EventClass,
  EventRef,
  GraphInvestigation,
  GraphSessionFilters,
  Selection,
  Severity,
} from '../components/graph/types'
import {
  contextEvents,
  entities as mockEntities,
  graphEdges as mockGraphEdges,
  graphNodes as mockGraphNodes,
  useAppStore,
} from '../store/appStore'
import type { Investigation } from '../types'

function mapEntityType(kind: string): EntityTypeCode {
  switch (kind) {
    case 'host':
      return 'host'
    case 'user':
      return 'user'
    case 'process':
      return 'process'
    case 'ip':
      return 'ip'
    case 'file':
      return 'file_hash'
    case 'domain':
      return 'domain'
    case 'email':
      return 'url'
    default:
      return 'host'
  }
}

function mapEventClass(type: string): EventClass {
  if (type.startsWith('network') || type.startsWith('dns')) return 'network_session'
  if (type.startsWith('process') || type.startsWith('file')) return 'endpoint'
  if (type.startsWith('email')) return 'log'
  if (type.startsWith('auth')) return 'detect'
  return 'log'
}

function mapSeverity(s: string): Severity {
  if (s === 'critical' || s === 'high' || s === 'medium' || s === 'low') return s
  return 'low'
}

function defaultFilters(windowStart: string, windowEnd: string): GraphSessionFilters {
  return {
    entityTypes: [...ALL_ENTITY_TYPES],
    severities: [...ALL_SEVERITIES],
    edgeOrigins: ['seed', 'expanded'],
    timeRange: {
      start: new Date(windowStart).getTime(),
      end: new Date(windowEnd).getTime(),
    },
  }
}

function buildFromApp(inv: Investigation): GraphInvestigation {
  const reviews = useAppStore.getState().nodeReviews
  const edgeReviews = useAppStore.getState().edgeReviews
  const eventReviews = useAppStore.getState().eventReviews

  const entityNodes = inv.nodeIds
    .map((id) => mockGraphNodes[id])
    .filter(Boolean)
    .filter((n) => n.kind !== 'event')
    .filter((n) => (reviews[n.id] ?? n.review) !== 'rejected')

  const entities: Entity[] = entityNodes.map((n) => {
    const src = mockEntities[n.refId]
    const review = reviews[n.id] ?? n.review
    return {
      id: n.refId,
      type_code: mapEntityType(n.kind),
      key: src?.attributes.hash ?? src?.attributes.ip ?? n.label,
      display_name: src?.label ?? n.label,
      first_seen: '2026-08-12T13:50:00Z',
      last_seen: '2026-08-12T14:35:00Z',
      metadata: {
        ...(src?.attributes ?? {}),
        review,
        node_id: n.id,
      },
      position: { x: n.x, y: n.y },
    }
  })

  // Alert nodes from critical/high seed context events
  const alerts: AlertNode[] = inv.eventIds
    .map((id) => contextEvents[id])
    .filter(Boolean)
    .filter((ev) => ev.severity === 'critical' || ev.severity === 'high')
    .filter((ev) => (eventReviews[ev.id] ?? ev.review) !== 'rejected')
    .slice(0, 4)
    .map((ev, i) => ({
      id: `alert-${ev.id}`,
      title: ev.title,
      severity: mapSeverity(ev.severity),
      event_ts: ev.time,
      source: ev.source,
      description: ev.description,
      position: { x: 120 + i * 160, y: -40 },
    }))

  const entityIdSet = new Set(entities.map((e) => e.id))
  const alertIdSet = new Set(alerts.map((a) => a.id))

  // Map graph node ids → entity/alert ids for edges
  const nodeToEndpoint = new Map<string, string>()
  for (const n of entityNodes) {
    nodeToEndpoint.set(n.id, n.refId)
  }

  const edges: Edge[] = inv.edgeIds
    .map((id) => mockGraphEdges[id])
    .filter(Boolean)
    .filter((e) => (edgeReviews[e.id] ?? e.review) !== 'rejected')
    .map((e) => {
      const review = edgeReviews[e.id] ?? e.review
      const source = nodeToEndpoint.get(e.source) ?? e.source
      const target = nodeToEndpoint.get(e.target) ?? e.target
      const origin: EdgeOrigin = review === 'proposed' || e.rationale ? 'expanded' : 'seed'
      return {
        id: e.id,
        source_id: source,
        target_id: target,
        kind: e.relation,
        origin,
        status: review,
        confidence: review === 'confirmed' ? 0.92 : review === 'proposed' ? 0.65 : 0.2,
        expand_from: origin === 'expanded' ? source : undefined,
      }
    })
    .filter(
      (e) =>
        (entityIdSet.has(e.source_id) || alertIdSet.has(e.source_id)) &&
        (entityIdSet.has(e.target_id) || alertIdSet.has(e.target_id)),
    )

  // Link alerts to first related entity visually via edges
  for (const alert of alerts) {
    const evId = alert.id.replace(/^alert-/, '')
    const ev = contextEvents[evId]
    const firstEnt = ev?.entityIds.find((id) => entityIdSet.has(id))
    if (!firstEnt) continue
    edges.push({
      id: `edge-alert-${alert.id}`,
      source_id: alert.id,
      target_id: firstEnt,
      kind: 'triggered',
      origin: 'seed',
      status: 'confirmed',
      confidence: 0.9,
    })
  }

  const events: EventRef[] = inv.eventIds
    .map((id) => contextEvents[id])
    .filter(Boolean)
    .filter((ev) => (eventReviews[ev.id] ?? ev.review) !== 'rejected')
    .map((ev) => ({
      id: ev.id,
      source: ev.source,
      source_event_id: ev.id,
      event_class: mapEventClass(ev.type),
      event_ts: ev.time,
      title: ev.title,
      severity: mapSeverity(ev.severity),
      summary: ev.description,
      entity_ids: ev.entityIds.filter((id) => entityIdSet.has(id)),
      alert_id:
        ev.severity === 'critical' || ev.severity === 'high'
          ? `alert-${ev.id}`
          : undefined,
    }))

  const times = events.map((e) => new Date(e.event_ts).getTime())
  const windowStart = new Date(
    (times.length ? Math.min(...times) : Date.parse('2026-08-12T13:50:00Z')) - 5 * 60_000,
  ).toISOString()
  const windowEnd = new Date(
    (times.length ? Math.max(...times) : Date.parse('2026-08-12T14:40:00Z')) + 5 * 60_000,
  ).toISOString()

  const running = inv.issueIds.some(
    (id) => useAppStore.getState().issues[id]?.status === 'running',
  )

  return {
    id: inv.id,
    title: inv.title,
    severity: mapSeverity(inv.severity),
    agentStatus: running ? 'agent: running' : 'agent: idle',
    windowStart,
    windowEnd,
    entities,
    alerts,
    edges,
    events,
    filters: defaultFilters(windowStart, windowEnd),
  }
}

interface WorkspaceState {
  activeInvestigation: GraphInvestigation | null
  selection: Selection
  hoverEntityIds: Set<string>
  expandedEntityIds: Set<string>
  boundInvestigationId: string | null

  /** Rebuild graph from app store for the given investigation */
  bindInvestigation: (investigationId: string | null) => void
  /** Refresh graph data after enrichment / review changes */
  refreshFromApp: () => void

  select: (selection: Selection) => void
  setHoverTime: (ms: number | null, entityIds?: string[]) => void
  setTimeRange: (range: { start: number; end: number } | null) => void
  toggleEntityType: (type: EntityTypeCode) => void
  toggleSeverity: (sev: Severity) => void
  toggleEdgeOrigin: (origin: EdgeOrigin) => void
  resetGraphFilters: () => void
  expandRelated: (entityId: string) => void
  collapseRelated: (entityId: string) => void
  canExpand: (entityId: string) => boolean
  isExpanded: (entityId: string) => boolean
  updateNodePosition: (nodeId: string, position: { x: number; y: number }) => void
}

function patchFilters(
  inv: GraphInvestigation,
  patch: Partial<GraphSessionFilters>,
): GraphInvestigation {
  return { ...inv, filters: { ...inv.filters, ...patch } }
}

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  activeInvestigation: null,
  selection: null,
  hoverEntityIds: new Set(),
  expandedEntityIds: new Set(),
  boundInvestigationId: null,

  bindInvestigation: (investigationId) => {
    if (!investigationId) {
      set({
        activeInvestigation: null,
        boundInvestigationId: null,
        selection: null,
        hoverEntityIds: new Set(),
        expandedEntityIds: new Set(),
      })
      return
    }
    const inv = useAppStore.getState().investigations[investigationId]
    if (!inv) {
      set({ activeInvestigation: null, boundInvestigationId: null })
      return
    }
    const prev = get().activeInvestigation
    const built = buildFromApp(inv)
    // Preserve filters / positions if same investigation
    if (prev && prev.id === investigationId) {
      built.filters = prev.filters
      const posById = new Map(prev.entities.map((e) => [e.id, e.position]))
      built.entities = built.entities.map((e) => ({
        ...e,
        position: posById.get(e.id) ?? e.position,
      }))
      const alertPos = new Map(prev.alerts.map((a) => [a.id, a.position]))
      built.alerts = built.alerts.map((a) => ({
        ...a,
        position: alertPos.get(a.id) ?? a.position,
      }))
    }
    set({
      activeInvestigation: built,
      boundInvestigationId: investigationId,
    })
  },

  refreshFromApp: () => {
    const id = get().boundInvestigationId
    if (id) get().bindInvestigation(id)
  },

  select: (selection) => {
    set({ selection })
    const invId = get().boundInvestigationId
    if (!invId) return

    const app = useAppStore.getState()
    const inv = app.investigations[invId]
    if (!inv) return

    // Always open host DetailPanel on graph/timeline selection
    if (selection) app.setDetailPanelOpen(true)

    if (selection?.kind === 'entity') {
      const nodeId = Object.values(mockGraphNodes).find((n) => n.refId === selection.id)?.id
      app.updateInvestigation(invId, {
        selectedNodeId: nodeId,
        selectedEventId: undefined,
        selectedEntityIds: inv.selectedEntityIds.includes(selection.id)
          ? inv.selectedEntityIds
          : [...inv.selectedEntityIds, selection.id],
      })
    } else if (selection?.kind === 'event') {
      app.updateInvestigation(invId, {
        selectedEventId: selection.id,
        selectedNodeId: undefined,
      })
    } else if (selection?.kind === 'alert') {
      // Alert nodes are keyed as alert-<contextEventId>
      const eventId = selection.id.replace(/^alert-/, '')
      app.updateInvestigation(invId, {
        selectedEventId: contextEvents[eventId] ? eventId : undefined,
        selectedNodeId: undefined,
      })
    } else if (selection === null) {
      app.updateInvestigation(invId, {
        selectedEventId: undefined,
        selectedNodeId: undefined,
      })
    }
  },

  setHoverTime: (_ms, entityIds) => {
    set({ hoverEntityIds: new Set(entityIds ?? []) })
  },

  setTimeRange: (range) => {
    const inv = get().activeInvestigation
    if (!inv) return
    set({
      activeInvestigation: patchFilters(inv, {
        timeRange:
          range ?? {
            start: new Date(inv.windowStart).getTime(),
            end: new Date(inv.windowEnd).getTime(),
          },
      }),
    })
  },

  toggleEntityType: (type) => {
    const inv = get().activeInvestigation
    if (!inv) return
    const setTypes = new Set(inv.filters.entityTypes)
    if (setTypes.has(type)) setTypes.delete(type)
    else setTypes.add(type)
    set({
      activeInvestigation: patchFilters(inv, {
        entityTypes: [...setTypes] as EntityTypeCode[],
      }),
    })
  },

  toggleSeverity: (sev) => {
    const inv = get().activeInvestigation
    if (!inv) return
    const setSev = new Set(inv.filters.severities)
    if (setSev.has(sev)) setSev.delete(sev)
    else setSev.add(sev)
    set({
      activeInvestigation: patchFilters(inv, {
        severities: [...setSev] as Severity[],
      }),
    })
  },

  toggleEdgeOrigin: (origin) => {
    const inv = get().activeInvestigation
    if (!inv) return
    const setOrig = new Set(inv.filters.edgeOrigins)
    if (setOrig.has(origin)) setOrig.delete(origin)
    else setOrig.add(origin)
    set({
      activeInvestigation: patchFilters(inv, {
        edgeOrigins: [...setOrig] as EdgeOrigin[],
      }),
    })
  },

  resetGraphFilters: () => {
    const inv = get().activeInvestigation
    if (!inv) return
    set({
      activeInvestigation: {
        ...inv,
        filters: defaultFilters(inv.windowStart, inv.windowEnd),
      },
    })
  },

  canExpand: (entityId) => {
    if (get().expandedEntityIds.has(entityId)) return false
    // Expandable if any mock edge from this entity leads to a node not yet in the graph
    const inv = get().activeInvestigation
    if (!inv) return false
    const present = new Set(inv.entities.map((e) => e.id))
    return Object.values(mockGraphEdges).some((e) => {
      const src = mockGraphNodes[e.source]
      const tgt = mockGraphNodes[e.target]
      if (!src || !tgt) return false
      const touches = src.refId === entityId || tgt.refId === entityId
      if (!touches) return false
      return !present.has(src.refId) || !present.has(tgt.refId)
    })
  },

  isExpanded: (entityId) => get().expandedEntityIds.has(entityId),

  expandRelated: (entityId) => {
    const invId = get().boundInvestigationId
    const inv = get().activeInvestigation
    if (!invId || !inv) return

    const appInv = useAppStore.getState().investigations[invId]
    if (!appInv) return

    const presentNodes = new Set(appInv.nodeIds)
    const presentEdges = new Set(appInv.edgeIds)
    const addNodes: string[] = []
    const addEdges: string[] = []

    for (const e of Object.values(mockGraphEdges)) {
      const src = mockGraphNodes[e.source]
      const tgt = mockGraphNodes[e.target]
      if (!src || !tgt) continue
      if (src.refId !== entityId && tgt.refId !== entityId) continue
      if (!presentNodes.has(src.id)) addNodes.push(src.id)
      if (!presentNodes.has(tgt.id)) addNodes.push(tgt.id)
      if (!presentEdges.has(e.id)) addEdges.push(e.id)
    }

    if (addNodes.length || addEdges.length) {
      useAppStore.getState().updateInvestigation(invId, {
        nodeIds: [...appInv.nodeIds, ...addNodes.filter((id, i, a) => a.indexOf(id) === i)],
        edgeIds: [...appInv.edgeIds, ...addEdges.filter((id, i, a) => a.indexOf(id) === i)],
        entityIds: [
          ...new Set([
            ...appInv.entityIds,
            ...addNodes.map((nid) => mockGraphNodes[nid]?.refId).filter(Boolean) as string[],
          ]),
        ],
      })
      // Mark proposed reviews for newly added
      for (const nid of addNodes) {
        const n = mockGraphNodes[nid]
        if (n && (useAppStore.getState().nodeReviews[nid] ?? n.review) === 'proposed') {
          // keep proposed
        }
      }
    }

    set({
      expandedEntityIds: new Set([...get().expandedEntityIds, entityId]),
    })
    get().refreshFromApp()
  },

  collapseRelated: (entityId) => {
    const invId = get().boundInvestigationId
    const appInv = invId ? useAppStore.getState().investigations[invId] : null
    if (!invId || !appInv) return

    // Remove nodes that were only connected via expand from this entity and are proposed
    const removeNodes = new Set<string>()
    for (const eid of appInv.edgeIds) {
      const e = mockGraphEdges[eid]
      if (!e) continue
      const src = mockGraphNodes[e.source]
      const tgt = mockGraphNodes[e.target]
      if (!src || !tgt) continue
      if (src.refId !== entityId && tgt.refId !== entityId) continue
      if (e.rationale || (useAppStore.getState().edgeReviews[e.id] ?? e.review) === 'proposed') {
        if (src.refId !== entityId) removeNodes.add(src.id)
        if (tgt.refId !== entityId) removeNodes.add(tgt.id)
      }
    }

    const nextNodeIds = appInv.nodeIds.filter((id) => !removeNodes.has(id))
    const nextEdgeIds = appInv.edgeIds.filter((eid) => {
      const e = mockGraphEdges[eid]
      if (!e) return true
      return !removeNodes.has(e.source) && !removeNodes.has(e.target)
    })

    useAppStore.getState().updateInvestigation(invId, {
      nodeIds: nextNodeIds,
      edgeIds: nextEdgeIds,
    })

    const next = new Set(get().expandedEntityIds)
    next.delete(entityId)
    set({ expandedEntityIds: next })
    get().refreshFromApp()
  },

  updateNodePosition: (nodeId, position) => {
    const inv = get().activeInvestigation
    if (!inv) return
    set({
      activeInvestigation: {
        ...inv,
        entities: inv.entities.map((e) =>
          e.id === nodeId ? { ...e, position } : e,
        ),
        alerts: inv.alerts.map((a) =>
          a.id === nodeId ? { ...a, position } : a,
        ),
      },
    })
  },
}))

/** Subscribe app-store enrichment/review changes → refresh graph */
useAppStore.subscribe((state, prev) => {
  const id = useWorkspaceStore.getState().boundInvestigationId
  if (!id) return
  const inv = state.investigations[id]
  const prevInv = prev.investigations[id]
  if (!inv) return
  if (
    inv !== prevInv ||
    state.nodeReviews !== prev.nodeReviews ||
    state.edgeReviews !== prev.edgeReviews ||
    state.eventReviews !== prev.eventReviews ||
    state.issues !== prev.issues
  ) {
    useWorkspaceStore.getState().refreshFromApp()
  }
})
