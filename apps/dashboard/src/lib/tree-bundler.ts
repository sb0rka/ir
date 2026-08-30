import type { GraphEdge, GraphNode } from "../types";

export interface ProposedTreeNode {
  id: string;
  node: GraphNode;
  edges: GraphEdge[];
  children: ProposedTreeNode[];
}

export function buildEntityTree(
  proposedEdges: GraphEdge[],
  nodesById: Record<string, GraphNode>,
): ProposedTreeNode[] {
  const childrenMap = new Map<string, string[]>();
  const parentMap = new Map<string, string>();
  const edgesBySource = new Map<string, GraphEdge[]>();

  for (const edge of proposedEdges) {
    if (!edgesBySource.has(edge.source)) edgesBySource.set(edge.source, []);
    edgesBySource.get(edge.source)!.push(edge);

    if (!childrenMap.has(edge.source)) childrenMap.set(edge.source, []);
    childrenMap.get(edge.source)!.push(edge.target);
    parentMap.set(edge.target, edge.source);
  }

  const roots: ProposedTreeNode[] = [];
  const visited = new Set<string>();

  function buildNode(nodeId: string): ProposedTreeNode | null {
    if (visited.has(nodeId)) return null;
    visited.add(nodeId);

    const node = nodesById[nodeId];
    if (!node) return null;

    const childIds = childrenMap.get(nodeId) ?? [];
    const children = childIds
      .map(buildNode)
      .filter((n): n is ProposedTreeNode => n !== null);

    return {
      id: nodeId,
      node,
      edges: edgesBySource.get(nodeId) ?? [],
      children,
    };
  }

  for (const edge of proposedEdges) {
    if (!parentMap.has(edge.source)) {
      const treeNode = buildNode(edge.source);
      if (treeNode) roots.push(treeNode);
    }
  }

  return roots;
}
