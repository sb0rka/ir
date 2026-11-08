import type { components as Gw } from '@ir/contract/gateway'
import type { EventSourceMeta } from '../lib/eventQuery'
import { gatewayClient } from './clients'
import { getProjectEpoch, getProjectId } from './env'
import { unwrapError } from './error'

let cache: { epoch: number; project: string; sources: EventSourceMeta[] } | null = null

export function clearEventFieldsCache(): void {
  cache = null
}

export function peekEventFields(): EventSourceMeta[] {
  return cache?.sources ?? []
}

function mapSource(source: Gw['schemas']['SourceEventFields']): EventSourceMeta {
  return {
    source_code: source.source_code,
    source_name: source.source_name,
    languages: source.languages ?? [],
    bounded: source.bounded,
    fields: source.fields ?? [],
    ...(source.common_mappings ? { common_mappings: source.common_mappings } : {}),
  }
}

export async function loadEventFields(): Promise<EventSourceMeta[]> {
  const project = getProjectId()
  if (!project) throw new Error('Проект не выбран')
  const epoch = getProjectEpoch()
  if (cache && cache.epoch === epoch && cache.project === project) return cache.sources
  const { data, error, response } = await gatewayClient.GET('/api/v1/events/fields', {
    params: { header: { 'X-Project-ID': project } },
  })
  if (error || !data) throw unwrapError(error, response.status)
  const sources = (data.sources ?? []).map(mapSource)
  cache = { epoch, project, sources }
  return sources
}
