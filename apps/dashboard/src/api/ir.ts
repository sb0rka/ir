import type { components as Ir } from '@ir/contract'
import { env } from './env'
import { irClient } from './clients'
import { unwrapError } from './error'
import {
  layoutGraph,
  mapGraphEdge,
  mapGraphNode,
  mapIrEntity,
  mapIrEvent,
  mapIrInvestigation,
} from './adapters'
import type { ContextEvent, Entity, GraphEdge, GraphNode, Investigation } from '../types'

type EventSourceRef = Ir['schemas']['EventSourceRef']

const FALLBACK_WORKSPACE = '00000000-0000-0000-0000-000000000001'
const projectParams = { header: { 'X-Project-ID': env.projectId } }

export interface InvestigationBundle {
  investigation: Investigation
  events: Record<string, ContextEvent>
  entities: Record<string, Entity>
  nodes: Record<string, GraphNode>
  edges: Record<string, GraphEdge>
}

export interface SomCatalog {
  workspaceId: string
  boardId: string | null
  issues: Ir['schemas']['SomIssue'][]
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

export async function listInvestigations(): Promise<Investigation[]> {
  const items: Investigation[] = []
  let cursor: string | undefined
  for (let i = 0; i < 8; i++) {
    const page = await throwIfError(
      await irClient.GET('/investigations', {
        params: { ...projectParams, query: { limit: 100, cursor } },
      }),
    )
    items.push(...page.investigations.map((inv) => mapIrInvestigation(inv)))
    if (!page.next_cursor) break
    cursor = page.next_cursor
  }
  return items
}

export async function resolveSomCatalog(): Promise<SomCatalog> {
  const workspaces = await throwIfError(await irClient.GET('/som/workspaces'))
  const selector = env.somWorkspaceSelector.toLowerCase()
  const workspace =
    workspaces.find(
      (w) => w.name.toLowerCase() === selector || w.slug.toLowerCase() === selector,
    ) ?? workspaces[0]
  if (!workspace) {
    return { workspaceId: FALLBACK_WORKSPACE, boardId: null, issues: [] }
  }
  const boards = await throwIfError(
    await irClient.GET('/som/workspaces/{workspace_id}/boards', {
      params: { path: { workspace_id: workspace.id } },
    }),
  )
  const boardSelector = env.somBoardSelector.toLowerCase()
  const board =
    boards.find((b) => b.name.toLowerCase() === boardSelector) ?? boards[0] ?? null
  if (!board) return { workspaceId: workspace.id, boardId: null, issues: [] }
  const listed = await throwIfError(
    await irClient.GET('/som/boards/{board_id}/issues', {
      params: { path: { board_id: board.id } },
    }),
  )
  return { workspaceId: workspace.id, boardId: board.id, issues: listed.issues }
}

export async function resolveWorkspaceId(): Promise<string> {
  try {
    const catalog = await resolveSomCatalog()
    return catalog.workspaceId
  } catch {
    return FALLBACK_WORKSPACE
  }
}

export async function createInvestigation(input: {
  title: string
  severity: Investigation['severity']
  description?: string
  parentId?: string
  workspaceId?: string
}): Promise<Investigation> {
  const workspaceId = input.workspaceId ?? (await resolveWorkspaceId())
  const created = await throwIfError(
    await irClient.POST('/investigations', {
      params: { ...projectParams },
      body: {
        title: input.title,
        description: input.description,
        severity: input.severity === 'info' ? 'low' : input.severity,
        parent_id: input.parentId,
        som_workspace_ids: [workspaceId],
      },
    }),
  )
  return mapIrInvestigation(created)
}

export async function addContext(
  investigationId: string,
  events: EventSourceRef[],
): Promise<Ir['schemas']['ContextImportResult'] | undefined> {
  if (events.length === 0) return undefined
  return throwIfError(
    await irClient.POST('/investigations/{investigation_id}/context', {
      params: { ...projectParams, path: { investigation_id: investigationId } },
      body: { events, entities: [] },
    }),
  )
}

export async function loadInvestigationBundle(
  investigationId: string,
  extras?: Partial<Investigation>,
): Promise<InvestigationBundle> {
  const [inv, eventsPage, entitiesPage, graph] = await Promise.all([
    throwIfError(
      await irClient.GET('/investigations/{investigation_id}', {
        params: { ...projectParams, path: { investigation_id: investigationId } },
      }),
    ),
    loadAllEvents(investigationId),
    loadAllEntities(investigationId),
    throwIfError(
      await irClient.GET('/investigations/{investigation_id}/graph', {
        params: {
          ...projectParams,
          path: { investigation_id: investigationId },
          query: { statuses: ['proposed', 'confirmed', 'rejected'] as const },
        },
      }),
    ),
  ])

  const entities: Record<string, Entity> = {}
  for (const entity of entitiesPage) {
    const mapped = mapIrEntity(entity)
    entities[mapped.id] = mapped
  }

  const events: Record<string, ContextEvent> = {}
  for (const event of eventsPage) {
    const entityIds = (event.entities ?? []).map((rel) => rel.entity_id)
    events[event.id] = mapIrEvent(event, entityIds)
  }

  const nodes: Record<string, GraphNode> = {}
  const mappedEdges = (graph.edges ?? []).map(mapGraphEdge)
  const mappedNodes = layoutGraph(
    investigationId,
    (graph.nodes ?? []).map(mapGraphNode),
    mappedEdges,
  )
  for (const node of mappedNodes) nodes[node.id] = node

  const edges: Record<string, GraphEdge> = {}
  for (const edge of mappedEdges) edges[edge.id] = edge

  return {
    investigation: mapIrInvestigation(inv, {
      ...extras,
      eventIds: Object.keys(events),
      entityIds: Object.keys(entities),
      nodeIds: Object.keys(nodes),
      edgeIds: Object.keys(edges),
    }),
    events,
    entities,
    nodes,
    edges,
  }
}

async function loadAllEvents(investigationId: string) {
  const items: Ir['schemas']['EventSummary'][] = []
  let cursor: string | undefined
  for (let i = 0; i < 10; i++) {
    const page = await throwIfError(
      await irClient.GET('/investigations/{investigation_id}/events', {
        params: {
          ...projectParams,
          path: { investigation_id: investigationId },
          query: { limit: 100, cursor },
        },
      }),
    )
    items.push(...page.events)
    if (!page.next_cursor) break
    cursor = page.next_cursor
  }
  return items
}

async function loadAllEntities(investigationId: string) {
  const items: Ir['schemas']['Entity'][] = []
  let cursor: string | undefined
  for (let i = 0; i < 10; i++) {
    const page = await throwIfError(
      await irClient.GET('/investigations/{investigation_id}/entities', {
        params: {
          ...projectParams,
          path: { investigation_id: investigationId },
          query: { limit: 100, cursor },
        },
      }),
    )
    items.push(...page.entities)
    if (!page.next_cursor) break
    cursor = page.next_cursor
  }
  return items
}

export async function reviewEdges(
  investigationId: string,
  body: Ir['schemas']['ReviewRequest'],
) {
  return throwIfError(
    await irClient.POST('/investigations/{investigation_id}/review', {
      params: { ...projectParams, path: { investigation_id: investigationId } },
      body,
    }),
  )
}

export async function patchInvestigation(
  investigationId: string,
  body: Ir['schemas']['InvestigationPatch'],
) {
  return throwIfError(
    await irClient.PATCH('/investigations/{investigation_id}', {
      params: { ...projectParams, path: { investigation_id: investigationId } },
      body,
    }),
  )
}

export async function runSomIssue(issueId: string, investigationId: string) {
  return throwIfError(
    await irClient.POST('/som/issues/{issue_id}/run', {
      params: { path: { issue_id: issueId } },
      body: {
        investigation_id: investigationId,
        variant: env.somVariant,
        model_id: env.somModelId,
      },
    }),
  )
}

export async function countProposedAgentEdges(investigationId: string): Promise<number> {
  const page = await throwIfError(
    await irClient.GET('/investigations/{investigation_id}/edges', {
      params: {
        ...projectParams,
        path: { investigation_id: investigationId },
        query: { statuses: ['proposed'] as const, origin: 'agent' as const, limit: 100 },
      },
    }),
  )
  return page.edges.length
}

export async function getEntityCard(entityId: string): Promise<Ir['schemas']['EntityCard']> {
  return throwIfError(
    await irClient.GET('/entities/{entity_id}', {
      params: { ...projectParams, path: { entity_id: entityId } },
    }),
  )
}
