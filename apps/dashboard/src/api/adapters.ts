import type {
  AlertEvent,
  ContextEvent,
  CorrelationGroup,
  Entity,
  EntityKind,
  EventOrigin,
  GraphEdge,
  GraphNode,
  Investigation,
  QueueItem,
  ReviewState,
  Severity,
} from '../types'
import type { components as Ir } from '@ir/contract'
import type { components as Gw } from '@ir/contract/gateway'

type GwEvent = Gw['schemas']['Event']
type GwEntity = Gw['schemas']['Entity']
type IrEvent = Ir['schemas']['EventSummary']
type IrEntity = Ir['schemas']['Entity']
type IrNode = Ir['schemas']['GraphNode']
type IrEdge = Ir['schemas']['GraphEdge']
type IrInvestigation = Ir['schemas']['Investigation']

const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info']

export function gatewayEventId(event: Pick<GwEvent, 'source_code' | 'source_event_id'>): string {
  return `${event.source_code}/${event.source_event_id}`
}

export function parseGatewayEventId(
  id: string,
): { source_code: string; source_event_id: string } | null {
  const idx = id.indexOf('/')
  if (idx <= 0) return null
  return { source_code: id.slice(0, idx), source_event_id: id.slice(idx + 1) }
}

export function gatewayEntityId(entity: { type: string; value: string }): string {
  return `${entity.type}:${entity.value}`
}

export function mapSeverity(value: string | undefined | null): Severity {
  if (value === 'critical' || value === 'high' || value === 'medium' || value === 'low' || value === 'info') {
    return value
  }
  return 'info'
}

export function mapEntityKind(value: string | undefined | null): EntityKind {
  switch (value) {
    case 'host':
    case 'user':
    case 'process':
    case 'file_hash':
    case 'ip':
    case 'domain':
    case 'email':
    case 'account':
    case 'url':
      return value
    case 'file':
      return 'file_hash'
    default:
      return 'host'
  }
}

export function mapOrigin(value: string | undefined | null): EventOrigin {
  if (value === 'agent' || value === 'analyst' || value === 'rule' || value === 'seed') return value
  if (value === 'system') return 'seed'
  return 'seed'
}

function stringifyAttrs(value: Record<string, unknown> | undefined): Record<string, string> {
  if (!value) return {}
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(value)) {
    if (v == null) continue
    out[k] = typeof v === 'string' ? v : JSON.stringify(v)
  }
  return out
}

function putRaw(target: Record<string, string>, key: string, value: unknown) {
  if (value == null || value === '') return
  target[key] = typeof value === 'string' ? value : String(value)
}

function rawFromNormalized(data: Record<string, unknown> | undefined): Record<string, string> {
  if (!data) return {}
  const attrs = data.attributes
  if (attrs && typeof attrs === 'object' && !Array.isArray(attrs)) {
    return stringifyAttrs(attrs as Record<string, unknown>)
  }
  const skip = new Set(['attributes', 'source_code', 'source_event_id', 'provenance'])
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(data)) {
    if (skip.has(key) || value == null || typeof value === 'object') continue
    out[key] = String(value)
  }
  return out
}

function flattenFindingRaw(finding: Gw['schemas']['Finding']): Record<string, string> {
  const raw: Record<string, string> = { finding_kind: finding.kind }
  putRaw(raw, 'status', finding.status)
  putRaw(raw, 'rule.name', finding.rule?.name)
  const correlation = finding.correlation
  if (correlation) {
    putRaw(raw, 'correlation_type', correlation.correlation_type)
    putRaw(raw, 'count.subevents', correlation.subevent_count)
  }
  const incident = finding.incident
  if (incident) {
    putRaw(raw, 'incident.key', incident.key)
    putRaw(raw, 'incident.external_key', incident.external_key)
    putRaw(raw, 'incident.verdict', incident.verdict)
    putRaw(raw, 'incident.damage', incident.damage)
    putRaw(raw, 'incident.recommendation', incident.recommendation)
    putRaw(raw, 'incident.assigned_to', incident.assigned_to)
  }
  const attack = finding.nad_attack
  if (attack) {
    putRaw(raw, 'nad.class', attack.class)
    putRaw(raw, 'nad.gid', attack.gid)
    putRaw(raw, 'nad.sid', attack.sid)
    putRaw(raw, 'nad.revision', attack.revision)
  }
  return raw
}

export function mapGatewayEntity(entity: GwEntity): Entity {
  const id = gatewayEntityId(entity)
  return {
    id,
    kind: mapEntityKind(entity.type),
    label: entity.value,
    attributes: stringifyAttrs(entity.attributes as Record<string, unknown> | undefined),
  }
}

export function mapIrEntity(entity: IrEntity): Entity {
  return {
    id: entity.id,
    kind: mapEntityKind(entity.type_code),
    label: entity.display_name || entity.canonical_key,
    attributes: {
      canonical_key: entity.canonical_key,
      type_code: entity.type_code,
      ...stringifyAttrs(entity.metadata as Record<string, unknown> | undefined),
    },
    firstSeen: entity.first_seen ?? undefined,
    lastSeen: entity.last_seen ?? undefined,
  }
}

export function gatewayFindingId(ref: {
  source_code: string
  source_instance?: string
  record_type: string
  external_id: string
}): string {
  const instance = ref.source_instance ? `${ref.source_instance}/` : ''
  return `${ref.source_code}/${instance}${ref.record_type}/${ref.external_id}`
}

export function mapGatewayFinding(
  finding: Gw['schemas']['Finding'],
): { alert: AlertEvent; entities: Entity[] } {
  const ref = finding.ref
  const entityIds: string[] = []
  const entities: Entity[] = []
  for (const mention of finding.entities ?? []) {
    if (!mention.type || !mention.value) continue
    const mapped = mapGatewayEntity({
      type: mention.type,
      value: mention.value,
      attributes: {},
      sources: [],
    })
    entityIds.push(mapped.id)
    entities.push(mapped)
  }
  const ruleName = finding.rule?.name ?? finding.kind
  const recordType = finding.ref.record_type
  const findingRef =
    recordType === 'siem_incident' ||
    recordType === 'siem_correlation' ||
    recordType === 'nad_attack'
      ? {
          source_code: ref.source_code,
          source_instance: ref.source_instance,
          record_type: recordType,
          external_id: ref.external_id,
          time_range: ref.time_range,
        }
      : undefined
  const alert: AlertEvent = {
    id: gatewayFindingId(ref),
    time: finding.occurred_at,
    severity: mapSeverity(finding.severity),
    title: finding.title,
    rule: ruleName,
    source: ref.source_code,
    status: 'new',
    entityIds,
    description: finding.description ?? finding.title,
    raw: flattenFindingRaw(finding),
    sourceEventId: ref.external_id,
    findingRef,
  }
  return { alert, entities }
}

export function mapGatewayEvent(
  event: GwEvent,
  entityIds: string[],
): AlertEvent {
  const attrs = stringifyAttrs(event.attributes as Record<string, unknown> | undefined)
  return {
    id: gatewayEventId(event),
    time: event.occurred_at,
    severity: mapSeverity(event.severity),
    title: event.title,
    rule: attrs.correlation_name || event.type,
    source: event.source_code,
    status: 'new',
    entityIds,
    description: event.title,
    raw: attrs,
    sourceEventId: event.source_event_id,
  }
}

export function mapGatewayContextEvent(
  event: GwEvent,
  entityIds: string[],
): ContextEvent {
  const alert = mapGatewayEvent(event, entityIds)
  return {
    id: alert.id,
    time: alert.time,
    severity: alert.severity,
    title: alert.title,
    type: event.type,
    source: alert.source,
    entityIds,
    origin: event.type === 'correlation_alert' ? 'seed' : 'seed',
    review: 'confirmed',
    description: alert.description,
    sourceEventId: event.source_event_id,
    raw: alert.raw,
  }
}

function severityFromNormalized(data: Record<string, unknown> | undefined): Severity {
  const raw = data?.severity
  return mapSeverity(typeof raw === 'string' ? raw : undefined)
}

export function mapIrEvent(event: IrEvent, entityIds: string[]): ContextEvent {
  const origin = mapOrigin(event.attached_by)
  const review: ReviewState = origin === 'agent' || origin === 'rule' ? 'proposed' : 'confirmed'
  const normalized = event.normalized_data as Record<string, unknown> | undefined
  return {
    id: event.id,
    time: event.occurred_at,
    severity: severityFromNormalized(normalized),
    title: event.title,
    type: event.event_type,
    source: event.source_code,
    entityIds,
    origin,
    review,
    description: event.reason || event.title,
    sourceEventId: event.source_event_id,
    raw: rawFromNormalized(normalized),
  }
}

export function mapIrInvestigation(
  inv: IrInvestigation,
  extras?: Partial<Investigation>,
): Investigation {
  return {
    id: inv.id,
    title: inv.title,
    severity: mapSeverity(inv.severity),
    status: inv.status === 'closed' ? 'closed' : 'open',
    parentId: inv.parent_id ?? undefined,
    assignee: 'аналитик',
    seedEventIds: extras?.seedEventIds ?? [],
    eventIds: extras?.eventIds ?? [],
    entityIds: extras?.entityIds ?? [],
    nodeIds: extras?.nodeIds ?? [],
    edgeIds: extras?.edgeIds ?? [],
    findingIds: extras?.findingIds ?? [],
    issueIds: extras?.issueIds ?? [],
    createdAt: inv.created_at,
    view: extras?.view ?? 'graph',
    selectedEntityIds: extras?.selectedEntityIds ?? [],
    version: inv.version,
    somWorkspaceIds: inv.som_workspace_ids,
  }
}

const LAYOUT_KEY = (investigationId: string) => `ir.layout.${investigationId}`

export function loadLayout(investigationId: string): Record<string, { x: number; y: number }> {
  try {
    const raw = localStorage.getItem(LAYOUT_KEY(investigationId))
    if (!raw) return {}
    return JSON.parse(raw) as Record<string, { x: number; y: number }>
  } catch {
    return {}
  }
}

export function saveLayout(
  investigationId: string,
  positions: Record<string, { x: number; y: number }>,
): void {
  try {
    localStorage.setItem(LAYOUT_KEY(investigationId), JSON.stringify(positions))
  } catch {
    /* ignore */
  }
}

function hashOffset(id: string): number {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0
  return (Math.abs(h) % 80) - 40
}

function eventTimeMs(node: GraphNode): number {
  if (!node.occurredAt) return Number.POSITIVE_INFINITY
  const t = new Date(node.occurredAt).getTime()
  return Number.isFinite(t) ? t : Number.POSITIVE_INFINITY
}

function compareEventsByTime(a: GraphNode, b: GraphNode): number {
  const dt = eventTimeMs(a) - eventTimeMs(b)
  if (dt !== 0) return dt
  return a.id.localeCompare(b.id)
}

type LayoutEdge = Pick<GraphEdge, 'source' | 'target'>
type Point = { x: number; y: number }

const EVENT_ORIGIN_X = 80
const EVENT_STEP_X = 256
const EVENT_BASE_Y = 260
const EVENT_WAVE_Y = 20
const NODE_GAP_X = 32
const NODE_GAP_Y = 24
const COL_GAP = 48
const BAND_GAP = 36
const STAGGER_X = 22
const EVENT_SIZE = { w: 220, h: 72 }
const ENTITY_SIZE = { w: 180, h: 56 }
const ENTITY_ROW = ENTITY_SIZE.h + NODE_GAP_Y
const SEPARATE_ITERS = 8

function nodeBox(kind: GraphNode['kind']) {
  return kind === 'event' ? EVENT_SIZE : ENTITY_SIZE
}

function eventWaveY(index: number): number {
  return EVENT_BASE_Y + Math.round(Math.sin(index * (Math.PI / 2.2)) * EVENT_WAVE_Y)
}

function compareEntities(a: GraphNode, b: GraphNode): number {
  if (a.kind !== b.kind) return a.kind.localeCompare(b.kind)
  return a.id.localeCompare(b.id)
}

function buildNeighbors(
  ids: Set<string>,
  edges: LayoutEdge[],
): Map<string, string[]> {
  const neighbors = new Map<string, string[]>()
  for (const id of ids) neighbors.set(id, [])
  for (const edge of edges) {
    if (edge.source === edge.target) continue
    if (!ids.has(edge.source) || !ids.has(edge.target)) continue
    neighbors.get(edge.source)?.push(edge.target)
    neighbors.get(edge.target)?.push(edge.source)
  }
  return neighbors
}

function boxesTooClose(
  a: Point,
  sa: { w: number; h: number },
  b: Point,
  sb: { w: number; h: number },
): boolean {
  const overlapX = Math.min(a.x + sa.w, b.x + sb.w) - Math.max(a.x, b.x) + NODE_GAP_X
  const overlapY = Math.min(a.y + sa.h, b.y + sb.h) - Math.max(a.y, b.y) + NODE_GAP_Y
  return overlapX > 0 && overlapY > 0
}

function overlappingSavedIds(
  saved: Record<string, Point>,
  entities: GraphNode[],
  eventPos: Map<string, Point>,
): Set<string> {
  const ignored = new Set<string>()
  const withSaved = entities.filter((entity) => saved[entity.id])
  const savedPoints = Object.values(saved)
  if (savedPoints.length >= 3) {
    let minX = Infinity
    let maxX = -Infinity
    let minY = Infinity
    let maxY = -Infinity
    for (const p of savedPoints) {
      minX = Math.min(minX, p.x)
      maxX = Math.max(maxX, p.x)
      minY = Math.min(minY, p.y)
      maxY = Math.max(maxY, p.y)
    }
    const expectedW =
      EVENT_ORIGIN_X + Math.max(0, eventPos.size - 1) * EVENT_STEP_X + EVENT_SIZE.w
    if (maxX - minX > expectedW * 1.45 || maxY - minY > 720) {
      for (const entity of withSaved) ignored.add(entity.id)
      return ignored
    }
  }
  for (let i = 0; i < withSaved.length; i++) {
    const a = withSaved[i]
    const pa = saved[a.id]
    for (const [, pe] of eventPos) {
      if (boxesTooClose(pa, ENTITY_SIZE, pe, EVENT_SIZE)) ignored.add(a.id)
    }
    for (let j = i + 1; j < withSaved.length; j++) {
      const b = withSaved[j]
      if (boxesTooClose(pa, ENTITY_SIZE, saved[b.id], ENTITY_SIZE)) {
        ignored.add(a.id)
        ignored.add(b.id)
      }
    }
  }
  return ignored
}

function meanPoint(ids: string[], pos: Map<string, Point>): Point | null {
  let x = 0
  let y = 0
  let n = 0
  for (const id of ids) {
    const p = pos.get(id)
    if (!p) continue
    x += p.x
    y += p.y
    n += 1
  }
  if (n === 0) return null
  return { x: x / n, y: y / n }
}

function fanAround(
  origin: Point,
  members: GraphNode[],
  mode: 'right' | 'above',
): Map<string, Point> {
  const placed = new Map<string, Point>()
  const n = members.length
  const startY =
    origin.y + EVENT_SIZE.h / 2 - ENTITY_SIZE.h / 2 - ((n - 1) * ENTITY_ROW) / 2
  members.forEach((ent, i) => {
    if (mode === 'right') {
      placed.set(ent.id, {
        x: origin.x + EVENT_SIZE.w + COL_GAP + (i % 2 === 0 ? 0 : STAGGER_X),
        y: startY + i * ENTITY_ROW,
      })
      return
    }
    placed.set(ent.id, {
      x: origin.x + (i % 2 === 0 ? 0 : STAGGER_X),
      y: origin.y - BAND_GAP - ENTITY_SIZE.h - i * ENTITY_ROW,
    })
  })
  return placed
}

function separateOverlaps(
  nodes: GraphNode[],
  pos: Map<string, Point>,
  pinned: Set<string>,
) {
  for (let pass = 0; pass < 2; pass++) {
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i]
        const b = nodes[j]
        const pa = pos.get(a.id)
        const pb = pos.get(b.id)
        if (!pa || !pb) continue

        const sa = nodeBox(a.kind)
        const sb = nodeBox(b.kind)
        const overlapX =
          Math.min(pa.x + sa.w, pb.x + sb.w) - Math.max(pa.x, pb.x) + NODE_GAP_X
        const overlapY =
          Math.min(pa.y + sa.h, pb.y + sb.h) - Math.max(pa.y, pb.y) + NODE_GAP_Y
        if (overlapX <= 0 || overlapY <= 0) continue

        const aPinned = pinned.has(a.id)
        const bPinned = pinned.has(b.id)
        if (aPinned && bPinned) continue

        const acx = pa.x + sa.w / 2
        const acy = pa.y + sa.h / 2
        const bcx = pb.x + sb.w / 2
        const bcy = pb.y + sb.h / 2

        if (overlapX < overlapY) {
          const dir = bcx === acx ? 1 : Math.sign(bcx - acx)
          const push = aPinned || bPinned ? overlapX : overlapX / 2
          if (!aPinned) pa.x -= push * dir
          if (!bPinned) pb.x += push * dir
        } else {
          const dir = bcy === acy ? 1 : Math.sign(bcy - acy)
          const push = aPinned || bPinned ? overlapY : overlapY / 2
          if (!aPinned) pa.y -= push * dir
          if (!bPinned) pb.y += push * dir
        }
      }
    }
  }
}

export function layoutGraph(
  investigationId: string,
  nodes: GraphNode[],
  edges: LayoutEdge[] = [],
  options?: { ignoreSaved?: boolean },
): GraphNode[] {
  const saved = options?.ignoreSaved ? {} : loadLayout(investigationId)
  const events = nodes
    .filter((n) => n.kind === 'event')
    .slice()
    .sort(compareEventsByTime)
  const entities = nodes.filter((n) => n.kind !== 'event')
  const eventIds = new Set(events.map((n) => n.id))
  const ids = new Set(nodes.map((n) => n.id))
  const neighbors = buildNeighbors(ids, edges)
  const pos = new Map<string, Point>()
  const pinned = new Set<string>()
  const fanMode = events.length <= 1 ? 'right' : 'above'

  events.forEach((n, i) => {
    // Event X is always chronological so a saved alphabetical layout cannot stick.
    pos.set(n.id, { x: EVENT_ORIGIN_X + i * EVENT_STEP_X, y: eventWaveY(i) })
    pinned.add(n.id)
  })

  const ignoreSaved = overlappingSavedIds(saved, entities, pos)
  const fans = new Map<string, GraphNode[]>()
  const shared: GraphNode[] = []
  for (const event of events) fans.set(event.id, [])
  for (const entity of entities) {
    if (saved[entity.id] && !ignoreSaved.has(entity.id)) continue
    const eventNeighbors = (neighbors.get(entity.id) ?? []).filter((id) =>
      eventIds.has(id),
    )
    if (eventNeighbors.length >= 2) {
      shared.push(entity)
      continue
    }
    const primary = events.find((event) => eventNeighbors.includes(event.id))
    if (primary) fans.get(primary.id)?.push(entity)
  }

  for (const event of events) {
    const members = (fans.get(event.id) ?? []).slice().sort(compareEntities)
    const origin = pos.get(event.id)
    if (!origin || members.length === 0) continue
    for (const [id, point] of fanAround(origin, members, fanMode)) {
      pos.set(id, point)
    }
  }

  for (const entity of shared) {
    const eventNeighbors = (neighbors.get(entity.id) ?? []).filter((id) =>
      eventIds.has(id),
    )
    const bary = meanPoint(eventNeighbors, pos)
    if (!bary) continue
    pos.set(entity.id, {
      x: bary.x,
      y: bary.y - BAND_GAP - ENTITY_SIZE.h,
    })
  }

  let fallbackIndex = 0
  for (const entity of entities) {
    if (pos.has(entity.id)) continue
    const savedPos = saved[entity.id]
    if (savedPos && !ignoreSaved.has(entity.id)) {
      pos.set(entity.id, { x: savedPos.x, y: savedPos.y })
      continue
    }
    const bary = meanPoint(neighbors.get(entity.id) ?? [], pos)
    if (bary) {
      pos.set(entity.id, {
        x: bary.x,
        y: bary.y - BAND_GAP - ENTITY_SIZE.h,
      })
    } else {
      pos.set(entity.id, {
        x: EVENT_ORIGIN_X + fallbackIndex * (ENTITY_SIZE.w + NODE_GAP_X) + hashOffset(entity.id),
        y: EVENT_BASE_Y - ENTITY_ROW * (1 + (fallbackIndex % 3)),
      })
      fallbackIndex += 1
    }
  }

  for (let iter = 0; iter < SEPARATE_ITERS; iter++) {
    separateOverlaps(nodes, pos, pinned)
    for (const entity of entities) {
      const current = pos.get(entity.id)
      if (!current) continue
      current.x = Math.max(24, current.x)
    }
  }

  return nodes.map((n) => {
    const p = pos.get(n.id)
    if (!p) return n
    return { ...n, x: Math.round(p.x), y: Math.round(p.y) }
  })
}

export function mapGraphNode(node: IrNode): GraphNode {
  const isEvent = node.node_type === 'event'
  const review: ReviewState = node.origin === 'analyst' ? 'confirmed' : 'proposed'
  return {
    id: node.id,
    kind: isEvent ? 'event' : mapEntityKind(node.type_code),
    refId: (isEvent ? node.event_id : node.entity_id) || node.id,
    label: node.label || node.canonical_key || node.id,
    review,
    x: 0,
    y: 0,
    origin: mapOrigin(node.origin),
    occurredAt: node.occurred_at ?? undefined,
  }
}

export function mapGraphEdge(edge: IrEdge): GraphEdge {
  return {
    id: edge.id,
    source: edge.source_node_id,
    target: edge.target_node_id,
    relation: edge.relation_code,
    review: edge.status,
    rationale: edge.why ?? undefined,
    version: edge.version,
    origin: mapOrigin(edge.origin),
  }
}

/** Vendor attribute for a correlation instance. Rename when the source mapping lands. */
const CORRELATION_EVENT_ID_ATTR = 'correlation_event_id'

function correlationEventId(event: AlertEvent): string | undefined {
  const value = event.raw?.[CORRELATION_EVENT_ID_ATTR]?.trim()
  return value || undefined
}

export function groupQueue(
  events: AlertEvent[],
  _entitiesById: Record<string, Entity>,
): { correlations: Record<string, CorrelationGroup>; queueOrder: QueueItem[] } {
  const alerts = new Map(events.map((e) => [e.id, e]))
  const groupedIds = new Set<string>()
  const correlations: Record<string, CorrelationGroup> = {}
  const queueOrder: QueueItem[] = []
  const membersById = new Map<string, AlertEvent[]>()

  for (const event of events) {
    const corrId = correlationEventId(event)
    if (!corrId) continue
    const members = membersById.get(corrId)
    if (members) members.push(event)
    else membersById.set(corrId, [event])
  }

  for (const [corrId, members] of membersById) {
    const id = `corr:${corrId}`
    const entityIds = [...new Set(members.flatMap((m) => m.entityIds))]
    const sourceCounts: CorrelationGroup['sourceCounts'] = {}
    for (const m of members) {
      sourceCounts[m.source] = (sourceCounts[m.source] ?? 0) + 1
    }
    const severity =
      members
        .map((m) => m.severity)
        .sort((a, b) => SEVERITY_ORDER.indexOf(a) - SEVERITY_ORDER.indexOf(b))[0] ?? 'high'
    const latest = members.slice().sort((a, b) => b.time.localeCompare(a.time))[0]
    const named = members.find((m) => m.raw?.correlation_name) ?? latest
    correlations[id] = {
      id,
      title: named?.raw?.correlation_name || named?.title || corrId,
      reason: named?.title ?? '',
      severity,
      time: latest?.time ?? '',
      status: 'new',
      sourceCounts,
      eventIds: members.map((m) => m.id),
      entityIds,
    }
    members.forEach((m) => groupedIds.add(m.id))
    queueOrder.push({ kind: 'correlation', id })
  }

  for (const event of events) {
    if (groupedIds.has(event.id)) continue
    queueOrder.push({ kind: 'alert', id: event.id })
  }

  queueOrder.sort((a, b) => {
    const timeOf = (item: QueueItem) =>
      item.kind === 'correlation'
        ? correlations[item.id]?.time ?? ''
        : alerts.get(item.id)?.time ?? ''
    return timeOf(b).localeCompare(timeOf(a))
  })

  return { correlations, queueOrder }
}
