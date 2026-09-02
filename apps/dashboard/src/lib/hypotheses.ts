import type { Hypothesis, HypothesisStatus } from '../api/hypotheses'

export const INVESTIGATION_LAYER_ID = '__investigation__'
export const EMPTY_LAYER_IDS: string[] = []

export interface HypothesisMembership {
  nodeIds: string[]
  edgeIds: string[]
}

export function layerItemIds(hypothesisIds: readonly string[]): string[] {
  return [INVESTIGATION_LAYER_ID, ...hypothesisIds]
}

export function openHypothesisNodeIds(
  hypothesisIds: readonly string[],
  hypotheses: Record<string, Pick<Hypothesis, 'status'>>,
  membership: Record<string, HypothesisMembership>,
): Set<string> {
  const ids = new Set<string>()
  for (const hypothesisId of hypothesisIds) {
    const hypothesis = hypotheses[hypothesisId]
    if (!hypothesis || hypothesis.status === 'resolved') continue
    for (const nodeId of membership[hypothesisId]?.nodeIds ?? []) ids.add(nodeId)
  }
  return ids
}

export function investigationLayerNodeIds(
  allNodeIds: readonly string[],
  hypothesisIds: readonly string[],
  hypotheses: Record<string, Pick<Hypothesis, 'status'>>,
  membership: Record<string, HypothesisMembership>,
): string[] {
  const openIds = openHypothesisNodeIds(hypothesisIds, hypotheses, membership)
  return allNodeIds.filter((id) => !openIds.has(id))
}

export function nodeIdsForLayerItems(
  itemIds: readonly string[],
  allNodeIds: readonly string[],
  hypothesisIds: readonly string[],
  hypotheses: Record<string, Pick<Hypothesis, 'status'>>,
  membership: Record<string, HypothesisMembership>,
): Set<string> {
  const investigationIds = investigationLayerNodeIds(
    allNodeIds,
    hypothesisIds,
    hypotheses,
    membership,
  )
  const ids = new Set<string>()
  for (const itemId of itemIds) {
    if (itemId === INVESTIGATION_LAYER_ID) {
      for (const nodeId of investigationIds) ids.add(nodeId)
      continue
    }
    for (const nodeId of membership[itemId]?.nodeIds ?? []) ids.add(nodeId)
  }
  return ids
}

export function graphLayerNodeSets(args: {
  visibleItemIds: string[] | undefined
  highlightedItemIds: readonly string[]
  allItemIds: readonly string[]
  allNodeIds: readonly string[]
  hypothesisIds: readonly string[]
  hypotheses: Record<string, Pick<Hypothesis, 'status'>>
  membership: Record<string, HypothesisMembership>
}): { visibleNodeIds: Set<string> | null; highlightedNodeIds: Set<string> | null } {
  const visibleItems = args.visibleItemIds ?? args.allItemIds
  const allVisible =
    args.visibleItemIds == null ||
    (args.allItemIds.length === visibleItems.length &&
      args.allItemIds.every((id) => visibleItems.includes(id)))
  return {
    visibleNodeIds: allVisible
      ? null
      : nodeIdsForLayerItems(
          visibleItems,
          args.allNodeIds,
          args.hypothesisIds,
          args.hypotheses,
          args.membership,
        ),
    highlightedNodeIds:
      args.highlightedItemIds.length === 0
        ? null
        : nodeIdsForLayerItems(
            args.highlightedItemIds,
            args.allNodeIds,
            args.hypothesisIds,
            args.hypotheses,
            args.membership,
          ),
  }
}

export function mergeVisibleLayerIds(
  previous: string[] | undefined,
  hypothesisIds: readonly string[],
): string[] {
  const all = layerItemIds(hypothesisIds)
  if (!previous) return all
  const allowed = new Set(all)
  const previousSet = new Set(previous)
  const next = previous.filter((id) => allowed.has(id))
  for (const id of hypothesisIds) {
    if (!previousSet.has(id) && !next.includes(id)) next.push(id)
  }
  return next
}

export function toggleLayerId(current: readonly string[], id: string, solo: boolean): string[] {
  if (solo) return [id]
  return current.includes(id) ? current.filter((item) => item !== id) : [...current, id]
}

export function validHypothesisTransition(from: string, to: string): boolean {
  if (from === to) return from === 'proposed' || from === 'active' || from === 'resolved'
  switch (from) {
    case 'proposed':
      return to === 'active' || to === 'resolved'
    case 'active':
      return to === 'resolved'
    case 'resolved':
      return to === 'active'
    default:
      return false
  }
}

export function hypothesisStatusLabel(status: HypothesisStatus): string {
  if (status === 'proposed') return 'предложена'
  if (status === 'active') return 'активна'
  return 'закрыта'
}

export function hypothesisOriginLabel(origin: string): string {
  if (origin === 'agent') return 'агент'
  if (origin === 'rule') return 'правило'
  return 'аналитик'
}

export function isHypothesisWritable(status: HypothesisStatus): boolean {
  return status !== 'resolved'
}

export function nodeIdsForEntityRefs(
  entityIds: string[],
  graphNodes: Record<string, { id: string; kind: string; refId: string }>,
): string[] {
  const wanted = new Set(entityIds)
  return Object.values(graphNodes)
    .filter((node) => node.kind !== 'event' && wanted.has(node.refId))
    .map((node) => node.id)
}

export function edgesBetweenNodes(
  nodeIds: string[],
  edges: Record<string, { id: string; source: string; target: string }>,
): string[] {
  const members = new Set(nodeIds)
  return Object.values(edges)
    .filter((edge) => members.has(edge.source) && members.has(edge.target))
    .map((edge) => edge.id)
}

export function membershipFromGraph(graph: {
  nodes: Array<{ id: string }>
  edges: Array<{ id: string }>
}): HypothesisMembership {
  return {
    nodeIds: graph.nodes.map((node) => node.id),
    edgeIds: graph.edges.map((edge) => edge.id),
  }
}
