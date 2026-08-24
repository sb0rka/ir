import type { AlertEvent, ContextEvent, CorrelationGroup, Entity, FilterChip, QueueItem } from '../types'
import { getProjectId } from './env'
import { gatewayClient } from './clients'
import { unwrapError } from './error'
import {
  groupQueue,
  mapGatewayContextEvent,
  mapGatewayEntity,
  mapGatewayEvent,
} from './adapters'
import type { components as Gw } from '@ir/contract/gateway'
import { resolve, type TimeInterval } from '../components/time-interval'

type SearchBody = Gw['schemas']['SearchEventsRequest']

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
  mockSources: string[]
}

function chipsToSearch(chips: FilterChip[], interval: TimeInterval, query?: string): SearchBody {
  const body: SearchBody = {
    time_range: resolve(interval),
    limit: 100,
  }
  const sources = chips.find((c) => c.field === 'source')?.values
  if (sources?.length) body.sources = sources

  const entities: NonNullable<SearchBody['entities']> = []
  for (const chip of chips) {
    const type =
      chip.field === 'host'
        ? 'host'
        : chip.field === 'user'
          ? 'user'
          : chip.field === 'process'
            ? 'process'
            : chip.field === 'ip'
              ? 'ip'
              : chip.field === 'domain'
                ? 'domain'
                : null
    if (!type) continue
    for (const value of chip.values) {
      entities.push({ type, value })
    }
  }
  if (entities.length) body.entities = entities

  const queryBits = [
    ...(query ? [query] : []),
    ...(chips.find((c) => c.field === 'hash')?.values ?? []),
  ]
  if (queryBits.length) body.query = queryBits.join(' ')
  return body
}

async function eventSources(): Promise<{
  defaults: string[]
  available: string[]
  activeMocks: Set<string>
}> {
  const { data, error, response } = await gatewayClient.GET('/api/v1/sources', {
    params: projectHeader(),
  })
  if (error || !data) throw unwrapError(error, response.status)
  const eventCapable = (data.items ?? []).filter((item) =>
    item.capabilities?.includes('events'),
  )
  const online = eventCapable.filter((item) => item.status === 'online')
  return {
    defaults: online.map((item) => item.code),
    available: eventCapable.map((item) => item.code),
    activeMocks: new Set(
      online.filter((item) => item.mode === 'mock').map((item) => item.code),
    ),
  }
}

async function searchPages(body: SearchBody) {
  const events: Gw['schemas']['Event'][] = []
  const entities: Gw['schemas']['Entity'][] = []
  const sourceErrors: string[] = []
  let cursor: string | undefined
  for (let i = 0; i < 8; i++) {
    const { data, error, response } = await gatewayClient.POST('/api/v1/events/search', {
      params: projectHeader(),
      body: { ...body, cursor },
    })
    if (error || !data) throw unwrapError(error, response.status)
    events.push(...(data.events ?? []))
    entities.push(...(data.entities ?? []))
    for (const err of data.source_errors ?? []) {
      sourceErrors.push(`${err.source}: ${err.message}`)
    }
    if (!data.next_cursor) break
    cursor = data.next_cursor
  }
  return { events, entities, sourceErrors }
}

export async function searchQueue(
  chips: FilterChip[],
  interval: TimeInterval,
  query?: string,
): Promise<QueueSearchResult> {
  const body = chipsToSearch(chips, interval, query)
  const sources = await eventSources()
  if (!body.sources?.length) {
    if (!sources.defaults.length) {
      return {
        alerts: {},
        correlations: {},
        queueOrder: [],
        entities: {},
        contextEvents: {},
        sourceErrors: ['Нет доступных online-источников событий'],
        availableSources: sources.available,
        mockSources: [],
      }
    }
    body.sources = sources.defaults
  }
  const mockSources = [
    ...new Set(body.sources.filter((source) => sources.activeMocks.has(source))),
  ]
  const { events, entities: gwEntities, sourceErrors } = await searchPages(body)
  const entities: Record<string, Entity> = {}
  for (const entity of gwEntities) {
    const mapped = mapGatewayEntity(entity)
    entities[mapped.id] = mapped
  }
  const alerts: Record<string, AlertEvent> = {}
  const contextEvents: Record<string, ContextEvent> = {}
  const alertList: AlertEvent[] = []
  for (const event of events) {
    const entityIds = event.entities.map((ref) => {
      const id = `${ref.type}:${ref.value}`
      if (!entities[id]) {
        entities[id] = {
          id,
          kind: ref.type as Entity['kind'],
          label: ref.value,
          attributes: {},
        }
      }
      return id
    })
    const alert = mapGatewayEvent(event, entityIds)
    alerts[alert.id] = alert
    alertList.push(alert)
    const ctx = mapGatewayContextEvent(event, entityIds)
    contextEvents[ctx.id] = ctx
  }
  const { correlations, queueOrder } = groupQueue(alertList, entities)
  return {
    alerts,
    correlations,
    queueOrder,
    entities,
    contextEvents,
    sourceErrors,
    availableSources: sources.available,
    mockSources,
  }
}

export async function lookupEntity(type: string, value: string): Promise<string> {
  const { data, error, response } = await gatewayClient.POST('/api/v1/entities/lookup', {
    params: projectHeader(),
    body: { entity: { type, value } },
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
