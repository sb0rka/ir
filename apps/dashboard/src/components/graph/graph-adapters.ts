import type { Edge as RFEdge, Node as RFNode } from '@xyflow/react'
import type { HypothesisViewMode } from '../../lib/hypotheses'
import type {
  AlertNode,
  Edge,
  EdgeOrigin,
  Entity,
  EntityTypeCode,
  EventRef,
  Severity,
} from './types'
import { formatEventTooltip, toMs } from './time'

export type HypothesisLens = {
  nodeIds: Set<string>
  edgeIds: Set<string>
  writable: boolean
  mode: HypothesisViewMode
}

export type GraphNodeData = {
  kind: 'entity' | 'alert'
  label: string
  sublabel?: string
  entityType?: Entity['type_code']
  severity?: Severity
  dimmed: boolean
  highlighted: boolean
  selected: boolean
  entityId?: string
  alertId?: string
  isSeed?: boolean
  tooltip?: string
  inHypothesis?: boolean
  canToggleHypothesis?: boolean
}

export type GraphFilters = {
  entityTypes: Set<EntityTypeCode>
  severities: Set<Severity>
  edgeOrigins: Set<EdgeOrigin>
  timeRange: { start: number; end: number } | null
}

export function buildVisibleGraph(args: {
  entities: Entity[]
  alerts: AlertNode[]
  edges: Edge[]
  events: EventRef[]
  filters: GraphFilters
  selection: { kind: string; id: string } | null
  hoverEventId: string | null
  hypothesisLens?: HypothesisLens | null
}): { nodes: RFNode<GraphNodeData>[]; edges: RFEdge[] } {
  const { entities, alerts, edges, events, filters, selection, hoverEventId } =
    args
  const lens = args.hypothesisLens ?? null
  const isolate = lens?.mode === 'isolate'

  const range = filters.timeRange
  const inRange = (startIso?: string, endIso?: string) => {
    if (!range) return true
    const first = startIso ? toMs(startIso) : NaN
    const last = endIso ? toMs(endIso) : first
    if (!Number.isFinite(first) || !Number.isFinite(last)) return true
    return last >= range.start && first <= range.end
  }

  const entityInTime = (entity: Entity) => inRange(entity.first_seen, entity.last_seen)
  const alertInTime = (alert: AlertNode) => inRange(alert.event_ts, alert.event_ts)
  const originVisible = (origin: EdgeOrigin) => filters.edgeOrigins.has(origin)

  let visibleEntities = entities.filter(
    (e) =>
      filters.entityTypes.has(e.type_code) &&
      entityInTime(e) &&
      originVisible(e.origin),
  )
  let visibleAlerts = alerts.filter(
    (a) =>
      filters.severities.has(a.severity) &&
      alertInTime(a) &&
      originVisible(a.origin),
  )
  if (isolate && lens) {
    visibleEntities = visibleEntities.filter((e) => lens.nodeIds.has(e.id))
    visibleAlerts = visibleAlerts.filter((a) => lens.nodeIds.has(a.id))
  }

  const visibleIds = new Set([
    ...visibleEntities.map((e) => e.id),
    ...visibleAlerts.map((a) => a.id),
  ])

  const hovering = Boolean(hoverEventId)
  const selectedId = selection?.id ?? null
  const hoverEvent = hoverEventId
    ? events.find((ev) => ev.id === hoverEventId)
    : undefined
  const hoverAlertIds = new Set<string>()
  if (hoverEventId) {
    hoverAlertIds.add(hoverEventId)
    if (hoverEvent?.alert_id) hoverAlertIds.add(hoverEvent.alert_id)
  }

  const isHoveredAlert = (a: AlertNode) =>
    hovering &&
    (hoverAlertIds.has(a.id) ||
      (!!a.event_id && hoverAlertIds.has(a.event_id)))
  const hoveredAlertNodeIds = new Set(
    visibleAlerts.filter(isHoveredAlert).map((a) => a.id),
  )

  const nodes: RFNode<GraphNodeData>[] = [
    ...visibleEntities.map((e) => {
      const inHypothesis = Boolean(lens?.nodeIds.has(e.id))
      return {
        id: e.id,
        type: 'entity',
        position: e.position,
        data: {
          kind: 'entity' as const,
          label: e.display_name,
          sublabel: e.type_code,
          entityType: e.type_code,
          dimmed: hovering || Boolean(lens && !isolate && !inHypothesis),
          highlighted: false,
          selected:
            selectedId === e.id ||
            (!!e.entity_id && selectedId === e.entity_id),
          entityId: e.entity_id ?? e.id,
          inHypothesis,
          canToggleHypothesis: Boolean(lens?.writable),
        },
      }
    }),
    ...visibleAlerts.map((a) => {
      const highlighted = isHoveredAlert(a)
      const inHypothesis = Boolean(lens?.nodeIds.has(a.id))
      const dimmed =
        (hovering && !highlighted) || Boolean(lens && !isolate && !inHypothesis)
      return {
        id: a.id,
        type: 'alert',
        position: a.position,
        data: {
          kind: 'alert' as const,
          label: a.title,
          sublabel: a.severity,
          severity: a.severity,
          dimmed,
          highlighted,
          selected:
            selectedId === a.id ||
            (!!a.event_id &&
              (selectedId === a.event_id || selectedId === `alert-${a.event_id}`)),
          alertId: a.event_id ?? a.id,
          isSeed: a.isSeed,
          tooltip: `${a.isSeed ? 'исходный · ' : ''}${
            a.event_ts ? formatEventTooltip(a.event_ts, a.title) : a.title
          }`,
          inHypothesis,
          canToggleHypothesis: Boolean(lens?.writable),
        },
      }
    }),
  ]

  const curvatureByTarget = new Map<string, number>()
  const rfEdges: RFEdge[] = edges
    .filter((e) => filters.edgeOrigins.has(e.origin))
    .filter((e) => visibleIds.has(e.source_id) && visibleIds.has(e.target_id))
    .filter((e) => !isolate || Boolean(lens?.edgeIds.has(e.id)))
    .map((e) => {
      const fromAgent = e.origin === 'agent'
      const opacity =
        e.status === 'proposed' ? 0.45 : e.status === 'rejected' ? 0.2 : 0.85
      const endpointsDimmed =
        hovering &&
        !hoveredAlertNodeIds.has(e.source_id) &&
        !hoveredAlertNodeIds.has(e.target_id)
      const outsideLens = Boolean(lens && !isolate && !lens.edgeIds.has(e.id))
      const curveIndex = curvatureByTarget.get(e.target_id) ?? 0
      curvatureByTarget.set(e.target_id, curveIndex + 1)

      return {
        id: e.id,
        source: e.source_id,
        target: e.target_id,
        label: e.kind,
        animated: fromAgent,
        pathOptions: { curvature: 0.2 + (curveIndex % 5) * 0.08 },
        style: {
          stroke: fromAgent ? 'var(--edge-expanded)' : 'var(--edge-seed)',
          strokeWidth: 1.5,
          strokeDasharray: fromAgent ? '5 4' : undefined,
          opacity: endpointsDimmed || outsideLens ? 0.15 : opacity,
        },
        labelStyle: {
          fill: 'var(--text-dim)',
          fontSize: 10,
        },
        labelBgStyle: {
          fill: 'var(--bg-elevated)',
          fillOpacity: 0.85,
        },
      }
    })

  return { nodes, edges: rfEdges }
}

export function eventsInRange(
  events: EventRef[],
  range: { start: number; end: number } | null,
): EventRef[] {
  if (!range) return events
  return events.filter((e) => {
    const t = toMs(e.event_ts)
    return t >= range.start && t <= range.end
  })
}
