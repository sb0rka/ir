import type { TimeInterval } from './components/time-interval/model'

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info'

export type AlertStatus = 'new' | 'investigating' | 'closed'

/** Gateway / ir-api source code, e.g. pt-maxpatrol-siem. */
export type Source = string

export type EntityKind =
  | 'host'
  | 'user'
  | 'account'
  | 'process'
  | 'file_hash'
  | 'ip'
  | 'domain'
  | 'email'
  | 'url'

export type EventOrigin = 'seed' | 'agent' | 'analyst' | 'rule'

export type ReviewState = 'confirmed' | 'proposed' | 'rejected'

export type InvestigationStatus = 'open' | 'in_progress' | 'closed'

export type IssueStatus = 'running' | 'completed' | 'error' | 'cancelled'

export type FilterField =
  | 'host'
  | 'user'
  | 'process'
  | 'hash'
  | 'ip'
  | 'domain'
  | 'severity'
  | 'source'
  | 'status'

export interface Entity {
  id: string
  kind: EntityKind
  label: string
  attributes: Record<string, string>
  firstSeen?: string
  lastSeen?: string
}

export interface AlertEvent {
  id: string
  time: string
  severity: Severity
  title: string
  rule: string
  source: Source
  status: AlertStatus
  entityIds: string[]
  description: string
  raw?: Record<string, string>
  sourceEventId?: string
  /** Present for queue rows loaded from Gateway findings/search. */
  findingRef?: {
    source_code: string
    source_instance?: string
    record_type: 'siem_incident' | 'siem_correlation' | 'nad_attack'
    external_id: string
    time_range: { from: string; to: string }
  }
}

export interface CorrelationGroup {
  id: string
  title: string
  reason: string
  severity: Severity
  time: string
  status: AlertStatus
  sourceCounts: Partial<Record<Source, number>>
  eventIds: string[]
  entityIds: string[]
}

export interface QueueItem {
  kind: 'alert' | 'correlation'
  id: string
}

export interface ContextEvent {
  id: string
  time: string
  severity: Severity
  title: string
  type: string
  source: Source
  entityIds: string[]
  origin: EventOrigin
  review: ReviewState
  description: string
  sourceEventId?: string
  raw?: Record<string, string>
}

export interface GraphNode {
  id: string
  kind: EntityKind | 'event'
  refId: string
  label: string
  review: ReviewState
  x: number
  y: number
  origin?: EventOrigin
  occurredAt?: string
}

export interface GraphEdge {
  id: string
  source: string
  target: string
  relation: string
  review: ReviewState
  rationale?: string
  version?: number
  origin?: EventOrigin
}

export interface Finding {
  id: string
  title: string
  severity: Severity
  entityIds: string[]
  description: string
  review: ReviewState
  origin: EventOrigin
}

export interface IssueComment {
  id: string
  author: string
  time: string
  text: string
}

export interface Issue {
  id: string
  investigationId: string
  parentId?: string
  template: string
  title: string
  description: string
  entityIds: string[]
  status: IssueStatus
  eventsFound: number
  edgesFound: number
  findingsFound: number
  resultSummary?: string
  /** Daemon environment id from runSomIssue — used to poll completion. */
  localEnvironmentId?: string
  comments: IssueComment[]
  createdAt: string
}

export interface Investigation {
  id: string
  title: string
  severity: Severity
  status: InvestigationStatus
  parentId?: string
  assignee: string
  seedEventIds: string[]
  eventIds: string[]
  entityIds: string[]
  nodeIds: string[]
  edgeIds: string[]
  findingIds: string[]
  issueIds: string[]
  createdAt: string
  view: 'table' | 'graph' | 'queue'
  selectedNodeId?: string
  selectedEventId?: string
  selectedEntityIds: string[]
  timelineRange?: [number, number]
  version?: number
  somWorkspaceIds?: string[]
}

export interface FilterChip {
  id: string
  field: FilterField
  values: string[]
}

/** Global queue search target: coarse findings or normalized events. */
export type QueueSource = 'findings' | 'events'

export interface QueryHistoryEntry {
  pdql: string
  timeInterval: TimeInterval
  queueSource?: QueueSource
}

/** Per-investigation state of the context event queue (search + filters). */
export interface ContextQueueState {
  /** Last executed entity chips used to filter the table. */
  chips: FilterChip[]
  pdql: string
  timeInterval: TimeInterval
  executedFingerprint: string | null
  queryHistory: QueryHistoryEntry[]
  selectedIds: string[]
  hideAdded: boolean
  originFilter: EventOrigin | 'all'
  reviewFilter: ReviewState | 'all'
}

export interface SavedView {
  id: string
  name: string
  chips: FilterChip[]
  timePreset: string
  timeFrom?: string
  timeTo?: string
  query?: string
}

export interface ActionResult {
  id: string
  action: string
  title: string
  body: string
  time: string
}
