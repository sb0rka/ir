import { create } from 'zustand'
import {
  DEFAULT_ENTITY_TYPES,
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
import { layoutGraph } from '../api/adapters'
import { persistGraphLayout, persistNodePosition, useAppStore } from '../store/appStore'
import type { Investigation } from '../types'

function mapEntityType(kind: string): EntityTypeCode {
  switch (kind) {
    case 'host':
      return 'host'
    case 'user':
    case 'account':
      return 'user'
    case 'process':
      return 'process'
    case 'ip':
      return 'ip'
    case 'file_hash':
    case 'file':
      return 'file_hash'
    case 'domain':
      return 'domain'
    case 'email':
    case 'url':
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
    entityTypes: [...DEFAULT_ENTITY_TYPES],
    severities: [...ALL_SEVERITIES],
    edgeOrigins: ['agent', 'analyst'],
    timeRange: {
      start: new Date(windowStart).getTime(),
      end: new Date(windowEnd).getTime(),
    },
  }
}

function collectTimes(values: Array<string | undefined>): number[] {
  const times: number[] = []
  for (const value of values) {
    if (!value) continue
    const t = new Date(value).getTime()
    if (Number.isFinite(t)) times.push(t)
  }
  return times
}

function isSeedEvent(
  marked: boolean | undefined,
  ids: Array<string | undefined>,
  seedEventIds: string[],
): boolean {
  if (marked) return true
  if (seedEventIds.length === 0) return false
  const seed = new Set(seedEventIds)
  return ids.some((id) => Boolean(id && seed.has(id)))
}

function graphOrigin(origin: string | undefined): EdgeOrigin {
  if (origin === 'agent' || origin === 'rule') return 'agent'
  return 'analyst'
}

function buildFromApp(inv: Investigation): GraphInvestigation {
  const app = useAppStore.getState()
  const reviews = app.nodeReviews
  const edgeReviews = app.edgeReviews
  const eventReviews = app.eventReviews
  const graphNodes = app.graphNodes
  const storeEntities = app.entities
  const storeEdges = app.graphEdges
  const contextEvents = app.contextEvents

  const visibleGraphNodes = inv.nodeIds
    .map((id) => graphNodes[id])
    .filter(Boolean)
    .filter((n) => (reviews[n.id] ?? n.review) !== 'rejected')

  const entityGraphNodes = visibleGraphNodes.filter((n) => n.kind !== 'event')
  const eventGraphNodes = visibleGraphNodes.filter((n) => n.kind === 'event')

  const entities: Entity[] = entityGraphNodes.map((n) => {
    const src = storeEntities[n.refId]
    const review = reviews[n.id] ?? n.review
    return {
      id: n.id,
      entity_id: n.refId,
      type_code: mapEntityType(n.kind),
      key: src?.attributes.canonical_key ?? src?.attributes.hash ?? src?.attributes.ip ?? n.label,
      display_name: src?.label ?? n.label,
      first_seen: src?.firstSeen,
      last_seen: src?.lastSeen,
      metadata: {
        ...(src?.attributes ?? {}),
        review,
        node_id: n.id,
        entity_id: n.refId,
      },
      position: { x: n.x, y: n.y },
      origin: graphOrigin(n.origin),
    }
  })

  const alerts: AlertNode[] = eventGraphNodes.map((n) => {
    const ev = contextEvents[n.refId]
    const isSeed = isSeedEvent(
      ev?.isSeed,
      [n.refId, n.id, ev?.id],
      inv.seedEventIds,
    )
    return {
      id: n.id,
      event_id: n.refId,
      title: ev?.title ?? n.label,
      severity: mapSeverity(ev?.severity ?? 'low'),
      event_ts: ev?.time ?? n.occurredAt ?? '',
      source: ev?.source ?? '',
      description: ev?.description ?? n.label,
      position: { x: n.x, y: n.y },
      isSeed,
      origin: graphOrigin(ev?.origin ?? n.origin),
    }
  })

  const canvasIds = new Set([
    ...entities.map((e) => e.id),
    ...alerts.map((a) => a.id),
  ])

  const edges: Edge[] = inv.edgeIds
    .map((id) => storeEdges[id])
    .filter(Boolean)
    .filter((e) => (edgeReviews[e.id] ?? e.review) !== 'rejected')
    .map((e) => {
      const review = edgeReviews[e.id] ?? e.review
      const origin = graphOrigin(e.origin)
      return {
        id: e.id,
        source_id: e.source,
        target_id: e.target,
        kind: e.relation,
        origin,
        status: review,
        confidence: review === 'confirmed' ? 0.92 : review === 'proposed' ? 0.65 : 0.2,
        expand_from: origin === 'agent' ? e.source : undefined,
      }
    })
    .filter((e) => canvasIds.has(e.source_id) && canvasIds.has(e.target_id))

  const entityNodeByRef = new Map(entityGraphNodes.map((n) => [n.refId, n.id]))
  const eventNodeByRef = new Map(eventGraphNodes.map((n) => [n.refId, n.id]))

  const entityCanvasIds = new Set(entities.map((e) => e.id))

  const events: EventRef[] = inv.eventIds
    .map((id) => contextEvents[id])
    .filter(Boolean)
    .filter((ev) => (eventReviews[ev.id] ?? ev.review) !== 'rejected')
    .map((ev) => {
      const alertId = eventNodeByRef.get(ev.id)
      const entityIds = new Set(
        ev.entityIds
          .map((id) => entityNodeByRef.get(id))
          .filter((id): id is string => Boolean(id)),
      )
      if (alertId) {
        for (const edge of edges) {
          if (edge.source_id === alertId && entityCanvasIds.has(edge.target_id)) {
            entityIds.add(edge.target_id)
          }
          if (edge.target_id === alertId && entityCanvasIds.has(edge.source_id)) {
            entityIds.add(edge.source_id)
          }
        }
      }
      return {
        id: ev.id,
        source: ev.source,
        source_event_id: ev.id,
        event_class: mapEventClass(ev.type),
        event_ts: ev.time,
        title: ev.title,
        severity: mapSeverity(ev.severity),
        summary: ev.description,
        entity_ids: [...entityIds],
        alert_id: alertId,
        isSeed: isSeedEvent(ev.isSeed, [ev.id], inv.seedEventIds),
      }
    })

  const times = collectTimes([
    ...events.map((e) => e.event_ts),
    ...entities.flatMap((e) => [e.first_seen, e.last_seen]),
    ...alerts.map((a) => a.event_ts),
  ])
  // Empty graph (list stub before bundle load) must not clamp to "now":
  // bind preserves filters, and a now±5min window hides every real node.
  const minT = times.length ? Math.min(...times) : 0
  const maxT = times.length ? Math.max(...times) : Date.now()
  const windowStart = new Date(minT - 5 * 60_000).toISOString()
  const windowEnd = new Date(maxT + 5 * 60_000).toISOString()

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
  hoverEventId: string | null
  expandedEntityIds: Set<string>
  boundInvestigationId: string | null

  /** Rebuild graph from app store for the given investigation */
  bindInvestigation: (investigationId: string | null) => void
  /** Refresh graph data after enrichment / review changes */
  refreshFromApp: () => void

  select: (selection: Selection) => void
  setHoverEvent: (eventId: string | null) => void
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
  arrangeNodes: () => void
}

function patchFilters(
  inv: GraphInvestigation,
  patch: Partial<GraphSessionFilters>,
): GraphInvestigation {
  return { ...inv, filters: { ...inv.filters, ...patch } }
}

/** Preserve chip filters/positions on refresh; expand time window unless user brushed. */
export function mergeFiltersOnGraphRefresh(
  prev: GraphInvestigation,
  built: GraphInvestigation,
): GraphSessionFilters {
  const nextRange = built.filters.timeRange
  const prevRange = prev.filters.timeRange
  const prevWindowStart = new Date(prev.windowStart).getTime()
  const prevWindowEnd = new Date(prev.windowEnd).getTime()
  // Same check GraphToolbar uses for “full window” (no timeline brush).
  const wasFullWindow =
    !prevRange ||
    (prevRange.start <= prevWindowStart && prevRange.end >= prevWindowEnd)

  if (wasFullWindow) {
    // Polling grew windowStart/End — timeline already shows new events; graph
    // must follow or new nodes stay clipped by the stale brush range.
    return { ...prev.filters, timeRange: nextRange }
  }

  const overlaps =
    !prevRange ||
    !nextRange ||
    (prevRange.end >= nextRange.start && prevRange.start <= nextRange.end)
  return overlaps
    ? { ...prev.filters, timeRange: prevRange }
    : { ...prev.filters, timeRange: nextRange }
}

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  activeInvestigation: null,
  selection: null,
  hoverEventId: null,
  expandedEntityIds: new Set(),
  boundInvestigationId: null,

  bindInvestigation: (investigationId) => {
    if (!investigationId) {
      set({
        activeInvestigation: null,
        boundInvestigationId: null,
        selection: null,
        hoverEventId: null,
        expandedEntityIds: new Set(),
      })
      return
    }
    const inv = useAppStore.getState().investigations[investigationId]
    if (!inv) {
      set({ activeInvestigation: null, boundInvestigationId: investigationId })
      return
    }
    const prev = get().activeInvestigation
    const built = buildFromApp(inv)
    const hadGraph =
      !!prev &&
      prev.id === investigationId &&
      (prev.entities.length > 0 || prev.alerts.length > 0)
    // Keep user filters/positions only after the graph has real data.
    // First bind is the list stub (no nodes); preserving its time window
    // would hide every node once the bundle arrives.
    if (hadGraph) {
      built.filters = mergeFiltersOnGraphRefresh(prev, built)
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
      const nodeId = Object.values(app.graphNodes).find(
        (n) => n.kind !== 'event' && n.refId === selection.id,
      )?.id
      app.updateInvestigation(invId, {
        selectedNodeId: nodeId,
        selectedEventId: undefined,
        selectedEntityIds: inv.selectedEntityIds.includes(selection.id)
          ? inv.selectedEntityIds
          : [...inv.selectedEntityIds, selection.id],
      })
    } else if (selection?.kind === 'event') {
      const nodeId = Object.values(app.graphNodes).find(
        (n) => n.kind === 'event' && n.refId === selection.id,
      )?.id
      app.updateInvestigation(invId, {
        selectedEventId: selection.id,
        selectedNodeId: nodeId,
      })
    } else if (selection?.kind === 'alert') {
      const eventId = selection.id.replace(/^alert-/, '')
      const nodeId = Object.values(app.graphNodes).find(
        (n) =>
          n.kind === 'event' &&
          (n.refId === eventId || n.id === selection.id || n.id === eventId),
      )?.id
      const resolvedEventId = nodeId
        ? app.graphNodes[nodeId]?.refId
        : app.contextEvents[eventId]
          ? eventId
          : undefined
      app.updateInvestigation(invId, {
        selectedEventId: resolvedEventId,
        selectedNodeId: nodeId,
      })
    } else if (selection === null) {
      app.updateInvestigation(invId, {
        selectedEventId: undefined,
        selectedNodeId: undefined,
      })
    }
  },

  setHoverEvent: (eventId) => {
    set({ hoverEventId: eventId })
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
    const present = new Set([
      ...inv.entities.map((e) => e.entity_id ?? e.id),
      ...inv.alerts.map((a) => a.event_id ?? a.id),
    ])
    const { graphNodes, graphEdges } = useAppStore.getState()
    return Object.values(graphEdges).some((e) => {
      const src = graphNodes[e.source]
      const tgt = graphNodes[e.target]
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

    const { graphNodes: mockGraphNodes, graphEdges: mockGraphEdges } = useAppStore.getState()

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
    const { graphNodes: mockGraphNodes, graphEdges: mockGraphEdges } = useAppStore.getState()
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
    persistNodePosition(inv.id, nodeId, position)
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

  arrangeNodes: () => {
    const invId = get().boundInvestigationId
    const workspaceInv = get().activeInvestigation
    if (!invId || !workspaceInv) return
    const appInv = useAppStore.getState().investigations[invId]
    if (!appInv) return

    const { graphNodes, graphEdges } = useAppStore.getState()
    const nodes = appInv.nodeIds.map((id) => graphNodes[id]).filter(Boolean)
    const edges = appInv.edgeIds.map((id) => graphEdges[id]).filter(Boolean)
    const laidOut = layoutGraph(invId, nodes, edges, { ignoreSaved: true })
    const posById = new Map(laidOut.map((n) => [n.id, { x: n.x, y: n.y }]))

    set({
      activeInvestigation: {
        ...workspaceInv,
        entities: workspaceInv.entities.map((e) => ({
          ...e,
          position: posById.get(e.id) ?? e.position,
        })),
        alerts: workspaceInv.alerts.map((a) => ({
          ...a,
          position: posById.get(a.id) ?? a.position,
        })),
      },
    })
    persistGraphLayout(invId, laidOut)
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
    state.issues !== prev.issues ||
    state.graphNodes !== prev.graphNodes ||
    state.graphEdges !== prev.graphEdges
  ) {
    useWorkspaceStore.getState().refreshFromApp()
  }
})
