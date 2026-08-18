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
  const entityInTime = (entity: Entity) => {
    if (!range) return true
    const first = toMs(entity.first_seen)
    const last = toMs(entity.last_seen)
    return last >= range.start && first <= range.end
  }

  const alertInTime = (alert: AlertNode) => {
    if (!range) return true
    const t = toMs(alert.event_ts)
    return t >= range.start && t <= range.end
  }

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
      const highlighted = hovering && hoverEntityIds.has(e.id)
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
          selected: selectedId === e.id,
          entityId: e.id,
        },
      }
    }),
    ...visibleAlerts.map((a) => {
      const linkedToHover = hovering && relatedAlertIds.has(a.id)
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
          selected: selectedId === a.id,
          alertId: a.id,
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
