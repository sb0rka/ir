import type { Edge as RFEdge, Node as RFNode } from '@xyflow/react'
import type {
  AlertNode,
  Edge,
  EdgeOrigin,
  Entity,
  EntityTypeCode,
  EventRef,
  Severity,
} from './types'
import { toMs } from './time'

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
  hoverEntityIds: Set<string>
}): { nodes: RFNode<GraphNodeData>[]; edges: RFEdge[] } {
  const { entities, alerts, edges, events, filters, selection, hoverEntityIds } =
    args

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

  const visibleEntities = entities.filter(
    (e) => filters.entityTypes.has(e.type_code) && entityInTime(e),
  )
  const visibleAlerts = alerts.filter(
    (a) => filters.severities.has(a.severity) && alertInTime(a),
  )

  const visibleIds = new Set([
    ...visibleEntities.map((e) => e.id),
    ...visibleAlerts.map((a) => a.id),
  ])

  const hovering = hoverEntityIds.size > 0
  const selectedId = selection?.id ?? null

  const relatedAlertIds = new Set<string>()
  if (hovering) {
    for (const ev of events) {
      if (ev.entity_ids.some((id) => hoverEntityIds.has(id)) && ev.alert_id) {
        relatedAlertIds.add(ev.alert_id)
      }
    }
  }

  const nodes: RFNode<GraphNodeData>[] = [
    ...visibleEntities.map((e) => {
      const highlighted =
        hovering &&
        (hoverEntityIds.has(e.id) ||
          (!!e.entity_id && hoverEntityIds.has(e.entity_id)))
      const dimmed = hovering && !highlighted
      return {
        id: e.id,
        type: 'entity',
        position: e.position,
        data: {
          kind: 'entity' as const,
          label: e.display_name,
          sublabel: e.type_code,
          entityType: e.type_code,
          dimmed,
          highlighted,
          selected:
            selectedId === e.id ||
            (!!e.entity_id && selectedId === e.entity_id),
          entityId: e.entity_id ?? e.id,
        },
      }
    }),
    ...visibleAlerts.map((a) => {
      const linkedToHover =
        hovering &&
        (relatedAlertIds.has(a.id) ||
          (!!a.event_id && relatedAlertIds.has(a.event_id)))
      const dimmed = hovering && !linkedToHover
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
          highlighted: linkedToHover,
          selected:
            selectedId === a.id ||
            (!!a.event_id &&
              (selectedId === a.event_id || selectedId === `alert-${a.event_id}`)),
          alertId: a.event_id ?? a.id,
        },
      }
    }),
  ]

  const rfEdges: RFEdge[] = edges
    .filter((e) => filters.edgeOrigins.has(e.origin))
    .filter((e) => visibleIds.has(e.source_id) && visibleIds.has(e.target_id))
    .map((e) => {
      const isExpanded = e.origin === 'expanded'
      const opacity =
        e.status === 'proposed' ? 0.45 : e.status === 'rejected' ? 0.2 : 0.85
      const endpointsDimmed =
        hovering &&
        !hoverEntityIds.has(e.source_id) &&
        !hoverEntityIds.has(e.target_id) &&
        !relatedAlertIds.has(e.source_id) &&
        !relatedAlertIds.has(e.target_id)

      return {
        id: e.id,
        source: e.source_id,
        target: e.target_id,
        label: e.kind,
        animated: isExpanded,
        style: {
          stroke: isExpanded ? 'var(--edge-expanded)' : 'var(--edge-seed)',
          strokeWidth: 1.5,
          strokeDasharray: isExpanded ? '5 4' : undefined,
          opacity: endpointsDimmed ? 0.15 : opacity,
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
