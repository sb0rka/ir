export type Severity = 'critical' | 'high' | 'medium' | 'low'

export type EntityTypeCode =
  | 'host'
  | 'user'
  | 'process'
  | 'ip'
  | 'file_hash'
  | 'domain'
  | 'url'

export type EventClass =
  | 'log'
  | 'correlation'
  | 'detect'
  | 'network_session'
  | 'verdict'
  | 'endpoint'

export type EventSource = 'SIEM' | 'EDR' | 'NDR' | 'LOGS'

export type EdgeOrigin = 'seed' | 'expanded'
export type EdgeStatus = 'proposed' | 'confirmed' | 'rejected'

export interface Entity {
  /** Graph node id — React Flow node id and edge endpoint. */
  id: string
  /** Underlying IR entity id, for the detail panel. */
  entity_id?: string
  type_code: EntityTypeCode
  key: string
  display_name: string
  first_seen?: string
  last_seen?: string
  metadata?: Record<string, string>
  position: { x: number; y: number }
}

export interface AlertNode {
  /** Graph node id — React Flow node id and edge endpoint. */
  id: string
  /** Underlying IR event id, for selection / the detail panel. */
  event_id?: string
  title: string
  severity: Severity
  event_ts: string
  source: string
  description: string
  position: { x: number; y: number }
}

export interface Edge {
  id: string
  source_id: string
  target_id: string
  kind: string
  origin: EdgeOrigin
  status: EdgeStatus
  confidence: number
  expand_from?: string
}

export interface EventRef {
  id: string
  source: EventSource | string
  source_event_id: string
  event_class: EventClass
  event_ts: string
  title: string
  severity?: Severity
  summary?: string
  entity_ids: string[]
  alert_id?: string
}

/** Per-investigation graph filter state */
export interface GraphSessionFilters {
  entityTypes: EntityTypeCode[]
  severities: Severity[]
  edgeOrigins: EdgeOrigin[]
  timeRange: { start: number; end: number } | null
}

/** Minimal investigation shape the graph UI reads */
export interface GraphInvestigation {
  id: string
  title: string
  severity: Severity
  agentStatus: string
  windowStart: string
  windowEnd: string
  entities: Entity[]
  alerts: AlertNode[]
  edges: Edge[]
  events: EventRef[]
  filters: GraphSessionFilters
}

export type Selection =
  | { kind: 'event'; id: string }
  | { kind: 'entity'; id: string }
  | { kind: 'finding'; id: string }
  | { kind: 'alert'; id: string }
  | null
