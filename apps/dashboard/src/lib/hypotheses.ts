import type { HypothesisStatus } from '../api/hypotheses'

export interface HypothesisMembership {
  nodeIds: string[]
  edgeIds: string[]
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
