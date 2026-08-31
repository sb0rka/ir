import type { components as Ir } from '@ir/contract'
import { irClient } from './clients'
import { unwrapError } from './error'
import { getProjectId } from './env'

export type Hypothesis = Ir['schemas']['Hypothesis']
export type HypothesisStatus = Ir['schemas']['HypothesisStatus']
export type HypothesisPatch = Ir['schemas']['HypothesisPatch']
export type HypothesisGraph = Ir['schemas']['HypothesisGraph']

type EventSourceRef = Ir['schemas']['EventSourceRef']
type SourceObjectRef = Ir['schemas']['SourceObjectRef']

function projectParams() {
  const projectId = getProjectId()
  if (!projectId) throw new Error('Проект не выбран')
  return { header: { 'X-Project-ID': projectId } }
}

async function throwIfError<T>(result: {
  data?: T
  error?: unknown
  response: Response
}): Promise<NonNullable<T>> {
  if (result.error || result.data == null) {
    throw unwrapError(result.error, result.response.status)
  }
  return result.data
}

async function throwIfFailed(result: { error?: unknown; response: Response }): Promise<void> {
  if (result.error || !result.response.ok) {
    throw unwrapError(result.error, result.response.status)
  }
}

function hypothesisPath(investigationId: string, hypothesisId: string) {
  return {
    ...projectParams(),
    path: { investigation_id: investigationId, hypothesis_id: hypothesisId },
  }
}

export async function listHypotheses(investigationId: string): Promise<Hypothesis[]> {
  const items: Hypothesis[] = []
  let cursor: string | undefined
  for (let i = 0; i < 8; i++) {
    const page = await throwIfError(
      await irClient.GET('/investigations/{investigation_id}/hypotheses', {
        params: {
          ...projectParams(),
          path: { investigation_id: investigationId },
          query: { limit: 100, cursor },
        },
      }),
    )
    items.push(...page.hypotheses)
    if (!page.next_cursor) break
    cursor = page.next_cursor
  }
  return items
}

export async function createHypothesis(
  investigationId: string,
  body: Ir['schemas']['HypothesisCreate'],
): Promise<Hypothesis> {
  return throwIfError(
    await irClient.POST('/investigations/{investigation_id}/hypotheses', {
      params: { ...projectParams(), path: { investigation_id: investigationId } },
      body,
    }),
  )
}

export async function getHypothesis(
  investigationId: string,
  hypothesisId: string,
): Promise<Hypothesis> {
  return throwIfError(
    await irClient.GET('/investigations/{investigation_id}/hypotheses/{hypothesis_id}', {
      params: hypothesisPath(investigationId, hypothesisId),
    }),
  )
}

export async function patchHypothesis(
  investigationId: string,
  hypothesisId: string,
  body: HypothesisPatch,
): Promise<Hypothesis> {
  return throwIfError(
    await irClient.PATCH('/investigations/{investigation_id}/hypotheses/{hypothesis_id}', {
      params: hypothesisPath(investigationId, hypothesisId),
      body,
    }),
  )
}

export async function deleteHypothesis(
  investigationId: string,
  hypothesisId: string,
): Promise<void> {
  return throwIfFailed(
    await irClient.DELETE('/investigations/{investigation_id}/hypotheses/{hypothesis_id}', {
      params: hypothesisPath(investigationId, hypothesisId),
    }),
  )
}

export async function getHypothesisGraph(
  investigationId: string,
  hypothesisId: string,
): Promise<HypothesisGraph> {
  return throwIfError(
    await irClient.GET('/investigations/{investigation_id}/hypotheses/{hypothesis_id}/graph', {
      params: hypothesisPath(investigationId, hypothesisId),
    }),
  )
}

export async function addHypothesisContext(
  investigationId: string,
  hypothesisId: string,
  input: {
    events?: EventSourceRef[]
    findings?: SourceObjectRef[]
  },
): Promise<Ir['schemas']['ContextImportResult'] | undefined> {
  const events = input.events ?? []
  const findings = input.findings ?? []
  if (events.length === 0 && findings.length === 0) return undefined
  return throwIfError(
    await irClient.POST('/investigations/{investigation_id}/hypotheses/{hypothesis_id}/context', {
      params: hypothesisPath(investigationId, hypothesisId),
      body: { findings, sessions: [], events, entities: [] },
    }),
  )
}

export async function addHypothesisNode(
  investigationId: string,
  hypothesisId: string,
  nodeId: string,
): Promise<void> {
  return throwIfFailed(
    await irClient.PUT(
      '/investigations/{investigation_id}/hypotheses/{hypothesis_id}/nodes/{node_id}',
      {
        params: {
          ...projectParams(),
          path: {
            investigation_id: investigationId,
            hypothesis_id: hypothesisId,
            node_id: nodeId,
          },
        },
      },
    ),
  )
}

export async function removeHypothesisNode(
  investigationId: string,
  hypothesisId: string,
  nodeId: string,
): Promise<void> {
  return throwIfFailed(
    await irClient.DELETE(
      '/investigations/{investigation_id}/hypotheses/{hypothesis_id}/nodes/{node_id}',
      {
        params: {
          ...projectParams(),
          path: {
            investigation_id: investigationId,
            hypothesis_id: hypothesisId,
            node_id: nodeId,
          },
        },
      },
    ),
  )
}

export async function addHypothesisEdge(
  investigationId: string,
  hypothesisId: string,
  edgeId: string,
): Promise<void> {
  return throwIfFailed(
    await irClient.PUT(
      '/investigations/{investigation_id}/hypotheses/{hypothesis_id}/edges/{edge_id}',
      {
        params: {
          ...projectParams(),
          path: {
            investigation_id: investigationId,
            hypothesis_id: hypothesisId,
            edge_id: edgeId,
          },
        },
      },
    ),
  )
}

export async function removeHypothesisEdge(
  investigationId: string,
  hypothesisId: string,
  edgeId: string,
): Promise<void> {
  return throwIfFailed(
    await irClient.DELETE(
      '/investigations/{investigation_id}/hypotheses/{hypothesis_id}/edges/{edge_id}',
      {
        params: {
          ...projectParams(),
          path: {
            investigation_id: investigationId,
            hypothesis_id: hypothesisId,
            edge_id: edgeId,
          },
        },
      },
    ),
  )
}
