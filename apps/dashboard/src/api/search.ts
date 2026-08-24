import type { FilterChip } from '../types'
import { getProjectId, resolveTimeRange } from './env'
import { gatewayClient } from './clients'
import { unwrapError } from './error'
import { mapGatewayFinding } from './adapters'
import { matchesChips } from '../lib/filters'
import type { AlertEvent, ContextEvent, CorrelationGroup, Entity, QueueItem } from '../types'
import type { components as Gw } from '@ir/contract/gateway'

type FindingsBody = Gw['schemas']['SearchFindingsRequest']
type FindingKind = Gw['schemas']['FindingKind']

const FINDING_KINDS: FindingKind[] = ['siem_incident', 'siem_correlation', 'nad_attack']
const PAGE_LIMIT = 100
const MAX_PAGES = 4

function projectHeader() {
  const projectId = getProjectId()
  if (!projectId) throw new Error('Проект не выбран')
  return { header: { 'X-Project-ID': projectId } } as const
}

export interface QueueSearchResult {
  alerts: Record<string, AlertEvent>
  correlations: Record<string, CorrelationGroup>
  queueOrder: QueueItem[]
  entities: Record<string, Entity>
  contextEvents: Record<string, ContextEvent>
  sourceErrors: string[]
  availableSources: string[]
  /** @deprecated Mock sources removed from Gateway; always empty. */
  mockSources: string[]
}

function buildFindingsBody(
  chips: FilterChip[],
  timePreset: string,
  timeFrom: string,
  timeTo: string,
  kinds: FindingKind[],
): FindingsBody {
  const body: FindingsBody = {
    time_range: resolveTimeRange(timePreset, timeFrom, timeTo),
    limit: PAGE_LIMIT,
    kinds,
  }
  const sources = chips.find((c) => c.field === 'source')?.values
  if (sources?.length) body.sources = sources
  return body
}

function matchesQuery(alert: AlertEvent, query?: string): boolean {
  if (!query?.trim()) return true
  const needle = query.trim().toLowerCase()
  const haystack = [alert.title, alert.rule, alert.description, alert.raw?.finding_kind]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  return haystack.includes(needle)
}

async function findingSources(): Promise<{ defaults: string[]; available: string[] }> {
  const { data, error, response } = await gatewayClient.GET('/api/v1/sources', {
    params: projectHeader(),
  })
  if (error || !data) throw unwrapError(error, response.status)
  const findingCapable = (data.items ?? []).filter((item) =>
    item.capabilities?.includes('findings'),
  )
  const online = findingCapable.filter((item) => item.status === 'online')
  return {
    defaults: online.map((item) => item.code),
    available: findingCapable.map((item) => item.code),
  }
}

async function searchFindingKind(body: FindingsBody): Promise<{
  findings: Gw['schemas']['Finding'][]
  sourceErrors: string[]
}> {
  const findings: Gw['schemas']['Finding'][] = []
  const sourceErrors: string[] = []
  let cursor: string | undefined
  for (let page = 0; page < MAX_PAGES; page++) {
    const { data, error, response } = await gatewayClient.POST('/api/v1/findings/search', {
      params: projectHeader(),
      body: { ...body, cursor },
    })
    if (error || !data) throw unwrapError(error, response.status)
    findings.push(...(data.findings ?? []))
    for (const err of data.source_errors ?? []) {
      sourceErrors.push(`${err.source}: ${err.message}`)
    }
    if (!data.next_cursor) break
    cursor = data.next_cursor
  }
  return { findings, sourceErrors }
}

export async function searchQueue(
  chips: FilterChip[],
  timePreset: string,
  timeFrom: string,
  timeTo: string,
  query?: string,
): Promise<QueueSearchResult> {
  const sources = await findingSources()
  const sourceErrors: string[] = []
  const selectedSources = chips.find((c) => c.field === 'source')?.values
  const allowedSources =
    selectedSources?.length ? selectedSources : sources.defaults.length ? sources.defaults : sources.available

  if (!allowedSources.length) {
    return {
      alerts: {},
      correlations: {},
      queueOrder: [],
      entities: {},
      contextEvents: {},
      sourceErrors: ['Нет доступных online-источников findings'],
      availableSources: sources.available,
      mockSources: [],
    }
  }

  const merged = new Map<string, Gw['schemas']['Finding']>()
  for (const kind of FINDING_KINDS) {
    const body = buildFindingsBody(chips, timePreset, timeFrom, timeTo, [kind])
    body.sources = allowedSources
    const page = await searchFindingKind(body)
    sourceErrors.push(...page.sourceErrors)
    for (const finding of page.findings) {
      const id = `${finding.ref.source_code}/${finding.ref.record_type}/${finding.ref.external_id}`
      merged.set(id, finding)
    }
  }

  const alerts: Record<string, AlertEvent> = {}
  const entities: Record<string, Entity> = {}
  const alertList: AlertEvent[] = []
  for (const finding of merged.values()) {
    const mapped = mapGatewayFinding(finding)
    for (const entity of mapped.entities) entities[entity.id] = entity
    alerts[mapped.alert.id] = mapped.alert
    alertList.push(mapped.alert)
  }

  const filtered = alertList
    .filter((alert) => matchesQuery(alert, query))
    .filter((alert) =>
      matchesChips(alert.entityIds, alert.severity, alert.source, alert.status, chips, entities),
    )
    .sort((a, b) => b.time.localeCompare(a.time))

  const queueAlerts: Record<string, AlertEvent> = {}
  const queueOrder: QueueItem[] = []
  for (const alert of filtered) {
    queueAlerts[alert.id] = alert
    queueOrder.push({ kind: 'alert', id: alert.id })
  }

  return {
    alerts: queueAlerts,
    correlations: {},
    queueOrder,
    entities,
    contextEvents: {},
    sourceErrors: [...new Set(sourceErrors)],
    availableSources: sources.available,
    mockSources: [],
  }
}

export async function lookupEntity(type: string, value: string): Promise<string> {
  const { data, error, response } = await gatewayClient.POST('/api/v1/entities/lookup', {
    params: projectHeader(),
    body: {
      entity: { type, value },
      time_range: resolveTimeRange('90d', '', ''),
    },
  })
  if (error || !data) throw unwrapError(error, response.status)
  if (data.verdicts?.length) {
    return data.verdicts
      .map((v) => `${v.provider}: ${v.value}${v.confidence != null ? ` (${v.confidence})` : ''}`)
      .join('\n')
  }
  if (data.entities?.length) {
    return data.entities
      .map((e) => `${e.type}:${e.value} · ${JSON.stringify(e.attributes ?? {})}`)
      .join('\n')
  }
  const errors = (data.source_errors ?? []).map((e) => `${e.source}: ${e.message}`)
  return errors.length ? errors.join('\n') : 'Нет данных репутации'
}

export async function analyzeArtifact(name: string, sha256?: string): Promise<string> {
  const { data, error, response } = await gatewayClient.POST('/api/v1/artifact-analyses', {
    params: projectHeader(),
    body: {
      source: 'pt-sandbox',
      artifact: {
        name,
        hashes: sha256 ? { sha256 } : undefined,
      },
    },
  })
  if (error || !data) throw unwrapError(error, response.status)
  return `Песочница ${data.status}: ${data.verdict.value} (${data.verdict.provider}, ${data.verdict.confidence}) · ${data.artifact.name}`
}
