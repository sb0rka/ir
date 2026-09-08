import {
  isFilterGroup,
  newId,
  type Condition,
  type FilterGroup,
  type FilterNode,
  type LogicalJoiner,
  type QueryAst,
} from './model'

export interface FilterList {
  nodes: FilterNode[]
  joiners: LogicalJoiner[]
}

export function filterListOf(ast: QueryAst): FilterList {
  return { nodes: ast.filter, joiners: ast.joiners }
}

export function withFilterList(ast: QueryAst, list: FilterList): QueryAst {
  return { ...ast, filter: list.nodes, joiners: list.joiners }
}

export function walkConditions(nodes: FilterNode[], visit: (condition: Condition) => void): void {
  for (const node of nodes) {
    if (isFilterGroup(node)) walkConditions(node.children, visit)
    else visit(node)
  }
}

export function collectConditions(nodes: FilterNode[]): Condition[] {
  const out: Condition[] = []
  walkConditions(nodes, (condition) => out.push(condition))
  return out
}

function mapGroups(nodes: FilterNode[], fn: (group: FilterGroup) => FilterGroup): FilterNode[] {
  return nodes.map((node) => {
    if (!isFilterGroup(node)) return node
    const mapped: FilterGroup = {
      ...node,
      children: mapGroups(node.children, fn),
    }
    return fn(mapped)
  })
}

function patchChildList(
  list: FilterList,
  childId: string,
  patch: (list: FilterList, index: number) => FilterList,
): FilterList | null {
  const index = list.nodes.findIndex((node) => node.id === childId)
  if (index >= 0) return patch(list, index)
  let changed = false
  const nodes = list.nodes.map((node) => {
    if (!isFilterGroup(node)) return node
    const next = patchChildList({ nodes: node.children, joiners: node.joiners }, childId, patch)
    if (!next) return node
    changed = true
    return { ...node, children: next.nodes, joiners: next.joiners }
  })
  return changed ? { ...list, nodes } : null
}

function patchNamedList(
  list: FilterList,
  parentId: string | null,
  patch: (list: FilterList) => FilterList,
): FilterList | null {
  if (parentId === null) return patch(list)
  let changed = false
  const nodes = list.nodes.map((node) => {
    if (!isFilterGroup(node)) return node
    if (node.id === parentId) {
      changed = true
      const next = patch({ nodes: node.children, joiners: node.joiners })
      return { ...node, children: next.nodes, joiners: next.joiners }
    }
    const next = patchNamedList({ nodes: node.children, joiners: node.joiners }, parentId, patch)
    if (!next) return node
    changed = true
    return { ...node, children: next.nodes, joiners: next.joiners }
  })
  return changed ? { ...list, nodes } : null
}

function removeAt(list: FilterList, index: number): FilterList {
  const nodes = list.nodes.filter((_, nodeIndex) => nodeIndex !== index)
  const joiners = list.joiners.filter((_, joinerIndex) =>
    index === 0 ? joinerIndex !== 0 : joinerIndex !== index - 1,
  )
  return { nodes, joiners }
}

export function pruneFilterList(list: FilterList): FilterList {
  const nodes: FilterNode[] = []
  const joiners: LogicalJoiner[] = []
  for (let index = 0; index < list.nodes.length; index++) {
    let node = list.nodes[index]!
    if (isFilterGroup(node)) {
      const inner = pruneFilterList({ nodes: node.children, joiners: node.joiners })
      if (inner.nodes.length === 0) continue
      if (inner.nodes.length === 1 && !node.negated) {
        node = inner.nodes[0]!
      } else {
        node = { ...node, children: inner.nodes, joiners: inner.joiners }
      }
    }
    if (nodes.length > 0) joiners.push(list.joiners[index - 1] ?? 'and')
    nodes.push(node)
  }
  return { nodes, joiners }
}

export function pruneFilterBy(list: FilterList, drop: (condition: Condition) => boolean): FilterList {
  const nodes: FilterNode[] = []
  const joiners: LogicalJoiner[] = []
  for (let index = 0; index < list.nodes.length; index++) {
    let node = list.nodes[index]!
    if (isFilterGroup(node)) {
      const inner = pruneFilterBy({ nodes: node.children, joiners: node.joiners }, drop)
      if (inner.nodes.length === 0) continue
      if (inner.nodes.length === 1 && !node.negated) {
        node = inner.nodes[0]!
      } else {
        node = { ...node, children: inner.nodes, joiners: inner.joiners }
      }
    } else if (drop(node)) {
      continue
    }
    if (nodes.length > 0) joiners.push(list.joiners[index - 1] ?? 'and')
    nodes.push(node)
  }
  return { nodes, joiners }
}

export function updateCondition(ast: QueryAst, id: string, patch: Partial<Condition>): QueryAst {
  const nodes = mapNodes(ast.filter, (node) => {
    if (isFilterGroup(node) || node.id !== id) return node
    return { ...node, ...patch }
  })
  return { ...ast, filter: nodes }
}

function mapNodes(nodes: FilterNode[], fn: (node: FilterNode) => FilterNode): FilterNode[] {
  return nodes.map((node) => {
    if (isFilterGroup(node)) {
      return fn({ ...node, children: mapNodes(node.children, fn) })
    }
    return fn(node)
  })
}

export function removeFilterNode(ast: QueryAst, id: string): QueryAst {
  const patched = patchChildList(filterListOf(ast), id, removeAt)
  if (!patched) return ast
  return withFilterList(ast, pruneFilterList(patched))
}

export function setFilterJoiner(
  ast: QueryAst,
  parentId: string | null,
  index: number,
  joiner: LogicalJoiner,
): QueryAst {
  const patched = patchNamedList(filterListOf(ast), parentId, (list) => {
    if (!list.joiners[index]) return list
    const joiners = list.joiners.slice()
    joiners[index] = joiner
    return { ...list, joiners }
  })
  return patched ? withFilterList(ast, patched) : ast
}

export function wrapFilterAdjacent(ast: QueryAst, parentId: string | null, joinerIndex: number): QueryAst {
  const patched = patchNamedList(filterListOf(ast), parentId, (list) => {
    if (joinerIndex < 0 || joinerIndex >= list.joiners.length) return list
    const left = list.nodes[joinerIndex]
    const right = list.nodes[joinerIndex + 1]
    if (!left || !right) return list
    const group: FilterGroup = {
      kind: 'group',
      id: newId('fgrp'),
      negated: false,
      children: [left, right],
      joiners: [list.joiners[joinerIndex]!],
    }
    return {
      nodes: [...list.nodes.slice(0, joinerIndex), group, ...list.nodes.slice(joinerIndex + 2)],
      joiners: [...list.joiners.slice(0, joinerIndex), ...list.joiners.slice(joinerIndex + 1)],
    }
  })
  return patched ? withFilterList(ast, patched) : ast
}

export function ungroupFilter(ast: QueryAst, groupId: string): QueryAst {
  const patched = patchChildList(filterListOf(ast), groupId, (list, index) => {
    const node = list.nodes[index]
    if (!node || !isFilterGroup(node)) return list
    if (node.negated && node.children.length > 1) return list
    if (node.negated && node.children.length === 1) {
      const child = node.children[0]!
      const lifted = isFilterGroup(child)
        ? { ...child, negated: child.negated || node.negated }
        : { ...child, negated: child.negated || node.negated }
      return {
        nodes: [...list.nodes.slice(0, index), lifted, ...list.nodes.slice(index + 1)],
        joiners: list.joiners,
      }
    }
    return {
      nodes: [...list.nodes.slice(0, index), ...node.children, ...list.nodes.slice(index + 1)],
      joiners: [...list.joiners.slice(0, index), ...node.joiners, ...list.joiners.slice(index)],
    }
  })
  return patched ? withFilterList(ast, pruneFilterList(patched)) : ast
}

export function toggleFilterGroupNegated(ast: QueryAst, groupId: string): QueryAst {
  const nodes = mapGroups(ast.filter, (group) =>
    group.id === groupId ? { ...group, negated: !group.negated } : group,
  )
  return { ...ast, filter: nodes }
}

export function appendFilterNode(
  ast: QueryAst,
  node: FilterNode,
  parentId: string | null = null,
): QueryAst {
  const patched = patchNamedList(filterListOf(ast), parentId, (list) => ({
    nodes: [...list.nodes, node],
    joiners: list.nodes.length === 0 ? list.joiners : [...list.joiners, 'and' as const],
  }))
  return patched ? withFilterList(ast, patched) : ast
}

function moveItem<T>(items: T[], index: number, delta: number): T[] {
  const next = index + delta
  if (next < 0 || next >= items.length) return items
  const copy = items.slice()
  const [item] = copy.splice(index, 1)
  copy.splice(next, 0, item)
  return copy
}

export function moveFilterNode(
  ast: QueryAst,
  parentId: string | null,
  index: number,
  delta: number,
): QueryAst {
  const patched = patchNamedList(filterListOf(ast), parentId, (list) => {
    const nodes = moveItem(list.nodes, index, delta)
    if (nodes === list.nodes) return list
    return { ...list, nodes }
  })
  return patched ? withFilterList(ast, patched) : ast
}

export function reorderFilterNodes(
  ast: QueryAst,
  parentId: string | null,
  from: number,
  to: number,
): QueryAst {
  const patched = patchNamedList(filterListOf(ast), parentId, (list) => {
    if (from === to || from < 0 || to < 0 || from >= list.nodes.length || to >= list.nodes.length) {
      return list
    }
    const nodes = list.nodes.slice()
    const [item] = nodes.splice(from, 1)
    nodes.splice(to, 0, item)
    return { ...list, nodes }
  })
  return patched ? withFilterList(ast, patched) : ast
}
