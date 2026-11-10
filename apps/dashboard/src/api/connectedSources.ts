import { getProjectEpoch, getProjectId } from './env'
import { listProjectSources } from './integrations'

type Cache = { revision: number; epoch: number; project: string; codes: ReadonlySet<string> }
type Inflight = { revision: number; epoch: number; project: string; promise: Promise<ReadonlySet<string>> }

let cache: Cache | null = null
let inflight: Inflight | null = null
let revision = 0
const listeners = new Set<() => void>()

export function getConnectedSourceRevision(): number {
  return revision
}

export function subscribeConnectedSources(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

/** Drop the cached source list. The queue re-reads Gateway after configuration changes. */
export function invalidateConnectedSources(): void {
  cache = null
  revision += 1
  for (const listener of listeners) listener()
}

/** Sources configured for the current project. Presence means the source is connected. */
export async function loadConnectedSourceCodes(): Promise<ReadonlySet<string>> {
  const project = getProjectId()
  if (!project) throw new Error('Проект не выбран')
  const epoch = getProjectEpoch()
  if (cache && cache.revision === revision && cache.epoch === epoch && cache.project === project) {
    return cache.codes
  }
  if (
    inflight &&
    inflight.revision === revision &&
    inflight.epoch === epoch &&
    inflight.project === project
  ) {
    return inflight.promise
  }
  const requestedRevision = revision
  const promise = listProjectSources(project).then((items) => {
    const codes = new Set(items.map((item) => item.code))
    if (revision === requestedRevision && getProjectId() === project && getProjectEpoch() === epoch) {
      cache = { revision: requestedRevision, epoch, project, codes }
    }
    return codes
  })
  inflight = { revision: requestedRevision, epoch, project, promise }
  return promise
}
