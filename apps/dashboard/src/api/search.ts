import {
  DEFAULT_QUEUE_SOURCE,
  type AlertEvent,
  type ContextEvent,
  type CorrelationGroup,
  type Entity,
  type EventGroupItem,
  type FilterChip,
  type QueueItem,
  type QueueSource,
} from '../types'
import { getProjectId, resolveTimeRange } from './env'
import { gatewayClient } from './clients'
import { unwrapError } from './error'
import { mapGatewayEntity, mapGatewayEvent, mapGatewayFinding } from './adapters'
import {
  findingResolveCache,
  findingResolveCacheKey,
  isFindingResolveSoftFail,
  type FindingResolveResult,
} from './findingResolveCache'
import { pickFindingAccounts, pickFindingChildEvents, pickFindingHosts, type FindingResolveKey } from '../lib/correlationSubevents'
import { matchesChips } from '../lib/filters'
import {
  astToEventAggregate,
  astToEventSearch,
  astToFilterChips,
  findingUuidFromAst,
  pdqlToSearchParts,
  timeIntervalFromAst,
  type QueryAst,
  type PdqlSearchEntity,
} from '../lib/pdql'
import { sortQueueAlerts, type QueueSort } from '../lib/queueSort'
import { inResolvedInterval, resolve, type TimeInterval } from '../components/time-interval/model'
import type { components as Gw } from '@ir/contract/gateway'

type FindingsBody = Gw['schemas']['SearchFindingsRequest']
type FindingKind = Gw['schemas']['FindingKind']
type EventsBody = Gw['schemas']['SearchEventsRequest']
type AggregateBody = Gw['schemas']['AggregateEventsRequest']
type Capability = Gw['schemas']['Capability']

const PAGE_LIMIT = 100
const MAX_PAGES = 4
const NAD_SOURCE = 'pt-nad'

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
  eventGroups: EventGroupItem[]
  sourceErrors: string[]
  availableSources: string[]
  /** @deprecated Mock sources removed from Gateway; always empty. */
  mockSources: string[]
  /** Wide range ∩ PDQL time — when set, UI time button should adopt this range. */
  effectiveTimeInterval?: TimeInterval
}

function emptyQueue(sourceErrors: string[], availableSources: string[]): QueueSearchResult {
  return {
    alerts: {},
    correlations: {},
    queueOrder: [],
    entities: {},
    contextEvents: {},
    eventGroups: [],
    sourceErrors,
    availableSources,
    mockSources: [],
  }
}

function buildFindingsBody(
  chips: FilterChip[],
  timeInterval: TimeInterval,
  kinds: FindingKind[],
): FindingsBody {
  const body: FindingsBody = {
    time_range: resolve(timeInterval),
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
  const haystack = [
    alert.title,
    alert.rule,
    alert.description,
    ...Object.values(alert.raw ?? {}),
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  return haystack.includes(needle)
}

async function capableSources(capability: Capability): Promise<{ defaults: string[]; available: string[] }> {
  const { data, error, response } = await gatewayClient.GET('/api/v1/sources', {
    params: projectHeader(),
  })
  if (error || !data) throw unwrapError(error, response.status)
  const capable = (data.items ?? []).filter((item) => item.capabilities?.includes(capability))
  const online = capable.filter((item) => item.status === 'online')
  return {
    defaults: online.map((item) => item.code),
    available: capable.map((item) => item.code),
  }
}

function resolveAllowedSources(
  chips: FilterChip[],
  sources: { defaults: string[]; available: string[] },
): string[] {
  const selectedSources = chips.find((c) => c.field === 'source')?.values
  if (selectedSources?.length) return selectedSources
  return sources.defaults.length ? sources.defaults : sources.available
}

function finishQueue(
  alertList: AlertEvent[],
  entities: Record<string, Entity>,
  chips: FilterChip[],
  query: string | undefined,
  sourceErrors: string[],
  availableSources: string[],
  sort?: QueueSort,
  eventGroups: EventGroupItem[] = [],
): QueueSearchResult {
  const filtered = sortQueueAlerts(
    alertList
      .filter((alert) => matchesQuery(alert, query))
      .filter((alert) =>
        matchesChips(alert.entityIds, alert.severity, alert.source, alert.status, chips, entities),
      ),
    sort,
  )

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
    availableSources,
    mockSources: [],
    eventGroups,
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

async function searchFindingsQueue(
  chips: FilterChip[],
  timeInterval: TimeInterval,
  query: string | undefined,
  kind: FindingKind,
  sort?: QueueSort,
): Promise<QueueSearchResult> {
  const sources = await capableSources('findings')
  const sourceErrors: string[] = []
  const allowedSources = resolveAllowedSources(chips, sources)

  if (!allowedSources.length) {
    return emptyQueue(['Нет доступных online-источников findings'], sources.available)
  }

  const merged = new Map<string, Gw['schemas']['Finding']>()
  const body = buildFindingsBody(chips, timeInterval, [kind])
  body.sources = allowedSources
  const page = await searchFindingKind(body)
  sourceErrors.push(...page.sourceErrors)
  for (const finding of page.findings) {
    const id = `${finding.ref.source_code}/${finding.ref.record_type}/${finding.ref.external_id}`
    merged.set(id, finding)
  }

  const entities: Record<string, Entity> = {}
  const alertList: AlertEvent[] = []
  for (const finding of merged.values()) {
    const mapped = mapGatewayFinding(finding)
    for (const entity of mapped.entities) entities[entity.id] = entity
    alertList.push(mapped.alert)
  }
  return finishQueue(alertList, entities, chips, query, sourceErrors, sources.available, sort)
}

function sourcesForEventSearch(allowed: string[], hasControls: boolean): string[] {
  if (!hasControls) return allowed
  return allowed.filter((code) => code !== NAD_SOURCE)
}

function mergeEventGroups(groups: Gw['schemas']['EventGroup'][]): EventGroupItem[] {
  const merged = new Map<string, EventGroupItem>()
  for (const group of groups) {
    const values = group.values ?? []
    const key = JSON.stringify(values)
    const prev = merged.get(key)
    if (prev) {
      prev.count += group.count
      continue
    }
    merged.set(key, {
      source_code: group.source_code,
      values,
      count: group.count,
    })
  }
  return [...merged.values()].sort((a, b) => b.count - a.count)
}

async function aggregateEventsQueue(
  ast: QueryAst,
  timeInterval: TimeInterval,
  allowedSources: string[],
): Promise<{ groups: EventGroupItem[]; sourceErrors: string[] }> {
  const parts = astToEventAggregate(ast)
  if (!parts) return { groups: [], sourceErrors: [] }
  const body: AggregateBody = {
    time_range: resolve(timeInterval),
    limit: PAGE_LIMIT,
    sources: allowedSources,
    group_by: parts.group_by,
  }
  if (parts.filter) body.filter = parts.filter
  if (parts.sort) body.sort = parts.sort
  const { data, error, response } = await gatewayClient.POST('/api/v1/events/aggregate', {
    params: projectHeader(),
    body,
  })
  if (error || !data) throw unwrapError(error, response.status)
  return {
    groups: mergeEventGroups(data.groups ?? []),
    sourceErrors: (data.source_errors ?? []).map((err) => `${err.source}: ${err.message}`),
  }
}

function findingUuidResolveKeys(
  uuid: string,
  timeRange: { from: string; to: string },
  sources: string[],
  recordType: FindingResolveKey['record_type'],
): FindingResolveKey[] {
  return sources.map((source_code) => ({
    source_code,
    record_type: recordType,
    external_id: uuid,
    time_range: timeRange,
  }))
}

function findingRefBody(key: FindingResolveKey): Gw['schemas']['SourceObjectRef'] {
  return {
    source_code: key.source_code,
    ...(key.source_instance ? { source_instance: key.source_instance } : {}),
    record_type: key.record_type,
    external_id: key.external_id,
    time_range: key.time_range,
  }
}

function entitiesFromGateway(events: Gw['schemas']['Event'][], extra: Gw['schemas']['Entity'][]) {
  const entities: Record<string, Entity> = {}
  for (const entity of extra) {
    const mapped = mapGatewayEntity(entity)
    entities[mapped.id] = mapped
  }
  const alertList: AlertEvent[] = []
  const seen = new Set<string>()
  for (const event of events) {
    const entityIds: string[] = []
    for (const mention of event.entities ?? []) {
      if (!mention.type || !mention.value) continue
      const mapped = mapGatewayEntity({
        type: mention.type,
        value: mention.value,
        attributes: {},
        sources: event.source_code
          ? [
              {
                source_code: event.source_code,
                source_entity_id: `${mention.type}:${mention.value}`,
                fetched_at: event.fetched_at,
              },
            ]
          : [],
      })
      const prev = entities[mapped.id]
      entities[mapped.id] = prev
        ? { ...prev, source: prev.source ?? mapped.source }
        : mapped
      entityIds.push(mapped.id)
    }
    const alert = mapGatewayEvent(event, entityIds)
    if (seen.has(alert.id)) continue
    seen.add(alert.id)
    alertList.push(alert)
  }
  return { alertList, entities }
}

async function resolveUuidFindingQueue(
  uuid: string,
  recordType: FindingResolveKey['record_type'],
  timeInterval: TimeInterval,
  allowedSources: string[],
  availableSources: string[],
): Promise<QueueSearchResult | null> {
  const time_range = resolve(timeInterval)
  const keys = findingUuidResolveKeys(uuid, time_range, allowedSources, recordType)
  if (keys.length === 0) return null

  const { data, error, response } = await gatewayClient.POST('/api/v1/context/resolve', {
    params: projectHeader(),
    body: {
      findings: keys.map(findingRefBody),
      events: allowedSources.map((source_code) => ({
        source_code,
        source_event_id: uuid,
      })),
    },
  })
  if (error || !data) throw unwrapError(error, response.status)

  const { alertList, entities } = entitiesFromGateway(data.events ?? [], data.entities ?? [])
  let picked: AlertEvent[] = []
  let usedKey: FindingResolveKey | null = null
  for (const key of keys) {
    const children = pickFindingChildEvents(alertList, key)
    if (children.length > picked.length) {
      picked = children
      usedKey = key
    }
  }
  if (picked.length === 0 || !usedKey) return null

  return finishQueue(
    picked.filter((alert) => inResolvedInterval(alert.time, time_range)),
    entities,
    [],
    undefined,
    contextErrorMessagesForKey(data, usedKey),
    availableSources,
  )
}

function gatewayEntityRefs(entities: PdqlSearchEntity[]): Gw['schemas']['EntityRef'][] {
  const out: Gw['schemas']['EntityRef'][] = []
  const seen = new Set<string>()
  for (const entity of entities) {
    // MaxPatrol eventWhere supports host / ip / account only.
    const type =
      entity.type === 'user' || entity.type === 'account'
        ? 'account'
        : entity.type === 'host' || entity.type === 'ip'
          ? entity.type
          : null
    if (!type) continue
    const key = `${type}\0${entity.value}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push({ type, value: entity.value })
  }
  return out
}

function finishEntityQueue(
  entities: Record<string, Entity>,
  sourceErrors: string[],
  availableSources: string[],
): QueueSearchResult {
  const order = Object.values(entities)
    .slice()
    .sort((a, b) => a.kind.localeCompare(b.kind) || a.label.localeCompare(b.label))
  return {
    alerts: {},
    correlations: {},
    queueOrder: order.map((entity) => ({ kind: 'entity' as const, id: entity.id })),
    entities,
    contextEvents: {},
    eventGroups: [],
    sourceErrors: [...new Set(sourceErrors)],
    availableSources,
    mockSources: [],
  }
}

async function searchEntitiesQueue(
  ast: QueryAst,
  timeInterval: TimeInterval,
): Promise<QueueSearchResult> {
  const sources = await capableSources('events')
  const allowedSources = sources.defaults.length ? sources.defaults : sources.available
  if (!allowedSources.length) {
    return emptyQueue(['Нет доступных online-источников events'], sources.available)
  }

  const entityParts = pdqlToSearchParts(ast)
  const eventParts = astToEventSearch(ast)
  const gatewayEntities = gatewayEntityRefs(entityParts.entities)
  if (gatewayEntities.length === 0 && !eventParts.filter) {
    return emptyQueue(
      ['Добавьте фильтр сущности (host / account / ip)'],
      sources.available,
    )
  }

  const body: EventsBody = {
    time_range: resolve(timeInterval),
    limit: PAGE_LIMIT,
    sources: allowedSources,
  }
  if (eventParts.filter) body.filter = eventParts.filter
  if (gatewayEntities.length) body.entities = gatewayEntities

  const sourceErrors: string[] = []
  const events: Gw['schemas']['Event'][] = []
  const pageEntities: Gw['schemas']['Entity'][] = []
  let cursor: string | undefined
  for (let page = 0; page < MAX_PAGES; page++) {
    const { data, error, response } = await gatewayClient.POST('/api/v1/events/search', {
      params: projectHeader(),
      body: { ...body, cursor },
    })
    if (error || !data) throw unwrapError(error, response.status)
    events.push(...(data.events ?? []))
    pageEntities.push(...(data.entities ?? []))
    for (const err of data.source_errors ?? []) {
      sourceErrors.push(`${err.source}: ${err.message}`)
    }
    if (!data.next_cursor) break
    cursor = data.next_cursor
  }

  const { entities } = entitiesFromGateway(events, pageEntities)
  const wanted = entityParts.entities
  const filtered: Record<string, Entity> = {}
  for (const entity of Object.values(entities)) {
    if (wanted.length === 0) {
      filtered[entity.id] = entity
      continue
    }
    const match = wanted.some((item) => {
      const kind =
        item.type === 'user' || item.type === 'account'
          ? entity.kind === 'user' || entity.kind === 'account'
          : entity.kind === item.type
      return kind && entity.label.toLowerCase() === item.value.toLowerCase()
    })
    if (match) filtered[entity.id] = entity
  }
  // Always include the explicitly requested entities even if the page had no hits.
  for (const item of wanted) {
    if (item.type === 'process') continue
    const type = item.type === 'user' ? 'account' : item.type
    const mapped = mapGatewayEntity({
      type,
      value: item.value,
      attributes: {},
      sources: allowedSources.map((source_code) => ({
        source_code,
        source_entity_id: `${type}:${item.value}`,
        fetched_at: new Date().toISOString(),
      })),
    })
    const prev = filtered[mapped.id]
    filtered[mapped.id] = prev
      ? { ...prev, source: prev.source ?? mapped.source }
      : mapped
  }

  return finishEntityQueue(filtered, sourceErrors, sources.available)
}

async function searchEventsQueue(
  ast: QueryAst,
  timeInterval: TimeInterval,
  groupValues?: (string | null)[],
): Promise<QueueSearchResult> {
  const sources = await capableSources('events')
  const parts = astToEventSearch(ast, groupValues)
  const entityRefs = gatewayEntityRefs(pdqlToSearchParts(ast).entities)
  const hasGroups = ast.groups.length > 0
  const allowedSources = sourcesForEventSearch(
    sources.defaults.length ? sources.defaults : sources.available,
    parts.hasControls || hasGroups,
  )
  if (!allowedSources.length) {
    return emptyQueue(
      [
        parts.hasControls || hasGroups
          ? 'Нет SIEM-источников для PDQL-поиска событий'
          : 'Нет доступных online-источников events',
      ],
      sources.available,
    )
  }

  const finding = findingUuidFromAst(ast)
  if (finding) {
    const resolved = await resolveUuidFindingQueue(
      finding.uuid,
      finding.recordType,
      timeInterval,
      allowedSources,
      sources.available,
    )
    if (resolved) return resolved
  }

  const sourceErrors: string[] = []
  let eventGroups: EventGroupItem[] = []
  if (hasGroups) {
    const aggregated = await aggregateEventsQueue(ast, timeInterval, allowedSources)
    eventGroups = aggregated.groups
    sourceErrors.push(...aggregated.sourceErrors)
    if (!parts.group_by) {
      return {
        ...emptyQueue([...new Set(sourceErrors)], sources.available),
        eventGroups,
      }
    }
  }

  const body: EventsBody = {
    time_range: resolve(timeInterval),
    limit: PAGE_LIMIT,
    sources: allowedSources,
  }
  if (parts.filter) body.filter = parts.filter
  if (parts.sort) body.sort = parts.sort
  if (entityRefs.length) body.entities = entityRefs
  if (parts.group_by && parts.group_values) {
    body.group_by = parts.group_by
    body.group_values = parts.group_values
  }

  const events: Gw['schemas']['Event'][] = []
  const gatewayEntities: Gw['schemas']['Entity'][] = []
  let cursor: string | undefined
  for (let page = 0; page < MAX_PAGES; page++) {
    const { data, error, response } = await gatewayClient.POST('/api/v1/events/search', {
      params: projectHeader(),
      body: { ...body, cursor },
    })
    if (error || !data) throw unwrapError(error, response.status)
    events.push(...(data.events ?? []))
    gatewayEntities.push(...(data.entities ?? []))
    for (const err of data.source_errors ?? []) {
      sourceErrors.push(`${err.source}: ${err.message}`)
    }
    if (!data.next_cursor) break
    cursor = data.next_cursor
  }

  const { alertList, entities } = entitiesFromGateway(events, gatewayEntities)
  return finishQueue(
    alertList,
    entities,
    [],
    undefined,
    sourceErrors,
    sources.available,
    parts.sort,
    eventGroups,
  )
}

export async function searchQueue(
  ast: QueryAst,
  timeInterval: TimeInterval,
  queueSource: QueueSource = DEFAULT_QUEUE_SOURCE,
  groupValues?: (string | null)[],
): Promise<QueueSearchResult> {
  const { interval: effective, rewritten } = timeIntervalFromAst(ast, timeInterval)
  if (queueSource === 'events') {
    const result = await searchEventsQueue(ast, effective, groupValues)
    return rewritten ? { ...result, effectiveTimeInterval: effective } : result
  }
  if (queueSource === 'entities') {
    const result = await searchEntitiesQueue(ast, effective)
    return rewritten ? { ...result, effectiveTimeInterval: effective } : result
  }
  const chips = astToFilterChips(ast)
  const query = pdqlToSearchParts(ast).query
  // Findings have no PDQL filter on the wire — PDQL `time` ∩ wide → time_range → IM detectedAt.
  const result = await searchFindingsQueue(
    chips,
    effective,
    query,
    queueSource,
    astToEventSearch(ast).sort,
  )
  return rewritten ? { ...result, effectiveTimeInterval: effective } : result
}

function alertFromGatewayEvent(event: Gw['schemas']['Event']): AlertEvent {
  const entityIds: string[] = []
  for (const mention of event.entities ?? []) {
    if (!mention.type || !mention.value) continue
    entityIds.push(mapGatewayEntity({
      type: mention.type,
      value: mention.value,
      attributes: {},
      sources: [],
    }).id)
  }
  return mapGatewayEvent(event, entityIds)
}

function contextErrorMessages(data: Gw['schemas']['ResolveContextResponse']): string[] {
  const out: string[] = []
  for (const err of data.source_errors ?? []) {
    out.push(`${err.source}: ${err.message}`)
  }
  for (const resolution of data.resolutions ?? []) {
    for (const err of resolution.errors ?? []) {
      out.push(`${err.source}: ${err.message}`)
    }
  }
  return [...new Set(out)]
}

function contextErrorMessagesForKey(
  data: Gw['schemas']['ResolveContextResponse'],
  key: FindingResolveKey,
): string[] {
  const out: string[] = []
  for (const resolution of data.resolutions ?? []) {
    if (
      resolution.ref.source_code !== key.source_code ||
      resolution.ref.record_type !== key.record_type ||
      resolution.ref.external_id !== key.external_id
    ) {
      continue
    }
    for (const err of resolution.errors ?? []) {
      out.push(`${err.source}: ${err.message}`)
    }
  }
  return [...new Set(out)]
}

export type { FindingResolveResult }
export { clearFindingResolveCache } from './findingResolveCache'

export async function resolveFindingEvents(
  key: FindingResolveKey,
  options: { force?: boolean } = {},
): Promise<FindingResolveResult> {
  const projectId = getProjectId()
  if (!projectId) throw new Error('Проект не выбран')
  const cacheKey = findingResolveCacheKey(projectId, key)
  return findingResolveCache.getOrLoad(
    cacheKey,
    async () => {
      const { data, error, response } = await gatewayClient.POST('/api/v1/context/resolve', {
        params: projectHeader(),
        body: {
          findings: [findingRefBody(key)],
        },
      })
      if (error || !data) throw unwrapError(error, response.status)
      const root = (data.findings ?? []).find(
        (finding) =>
          finding.ref.source_code === key.source_code &&
          finding.ref.record_type === key.record_type &&
          finding.ref.external_id === key.external_id,
      )
      const mentions = root?.entities ?? []
      return {
        events: pickFindingChildEvents((data.events ?? []).map(alertFromGatewayEvent), key),
        accounts: pickFindingAccounts(mentions),
        hosts: pickFindingHosts(mentions),
        errors: contextErrorMessages(data),
      }
    },
    {
      force: options.force,
      shouldCache: (result) => !isFindingResolveSoftFail(result),
    },
  )
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
