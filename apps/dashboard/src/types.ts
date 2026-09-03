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

export type Verdict = 'incident' | 'false_positive' | 'not_affected' | 'inconclusive'

export interface InvestigationCounters {
  children: number
  findings: number
  sessions: number
  events: number
  entities: number
  proposed_edges: number
}

export interface InvestigationListFilter {
  status: 'all' | InvestigationStatus
  severity: 'all' | Exclude<Severity, 'info'>
  q: string
}

export type IssueStatus = 'running' | 'completed' | 'error' | 'cancelled'

export type FilterField =
  | 'host'
  | 'user'
  | 'account'
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
  /** Gateway source code when known (e.g. pt-maxpatrol-siem). */
  source?: string
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
  kind: 'alert' | 'correlation' | 'entity'
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
  /** Investigation-event link flag (`is_seed`), not `attached_by`. */
  isSeed: boolean
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
  /** Gateway finding identities attached to this investigation (`source/instance?/kind/external_id`). */
  findingSourceKeys: string[]
  issueIds: string[]
  hypothesisIds: string[]
  createdAt: string
  updatedAt?: string
  closedAt?: string | null
  description?: string | null
  verdict?: Verdict
  verdictReason?: string | null
  counters?: InvestigationCounters
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

/** High-level queue search target: a finding kind, normalized events, or entities. */
export type QueueSource = 'siem_incident' | 'siem_correlation' | 'nad_attack' | 'events' | 'entities'

export const DEFAULT_QUEUE_SOURCE: QueueSource = 'siem_incident'

export const QUEUE_SOURCE_OPTIONS: { id: QueueSource; label: string }[] = [
  { id: 'siem_incident', label: 'Инциденты' },
  { id: 'siem_correlation', label: 'Корреляции' },
  { id: 'nad_attack', label: 'Атаки NAD' },
  { id: 'events', label: 'События' },
  { id: 'entities', label: 'Сущности' },
]

export interface QueryHistoryEntry {
  pdql: string
  timeInterval: TimeInterval
  queueSource?: QueueSource
  groupValues?: (string | null)[]
}

/** One source-local (or merged) event group from Gateway aggregate. */
export interface EventGroupItem {
  source_code: string
  values: (string | null)[]
  count: number
}

/** Cached queue table results for one QueueSource (session memory). */
export interface QueueSourceResultSnapshot {
  alerts: Record<string, AlertEvent>
  correlations: Record<string, CorrelationGroup>
  queueOrder: QueueItem[]
  eventGroups: EventGroupItem[]
  executedFingerprint: string | null
  mockSources: string[]
}

/** Context-queue subset of a source snapshot (no correlations / mockSources). */
export interface ContextSourceResultSnapshot {
  alerts: Record<string, AlertEvent>
  queueOrder: QueueItem[]
  eventGroups: EventGroupItem[]
  executedFingerprint: string | null
}

/** Per-investigation state of the context event queue (search + filters). */
export interface ContextQueueState {
  /** Last executed entity chips used to filter the table. */
  chips: FilterChip[]
  pdql: string
  timeInterval: TimeInterval
  queueSource: QueueSource
  groupValues: (string | null)[]
  eventGroups: EventGroupItem[]
  executedFingerprint: string | null
  queryHistory: QueryHistoryEntry[]
  /** Bumped when a finding resolve chip blocks adding another filter. */
  findingFilterWarnAt: number
  selectedIds: string[]
  hideAdded: boolean
  originFilter: EventOrigin | 'all'
  reviewFilter: ReviewState | 'all'
  /** Client-side text filter for the context queue AlertTable. */
  textFilter: string
  /** Which alert table column textFilter applies to. */
  textFilterColumn: string
  alerts: Record<string, AlertEvent>
  queueOrder: QueueItem[]
  loading: boolean
  /** Last executed results per source while switching QueueSourceToggle. */
  sourceResults: Partial<Record<QueueSource, ContextSourceResultSnapshot>>
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
