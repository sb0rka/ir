import { useCallback, useEffect, useMemo } from 'react'
import {
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Node,
  type NodeMouseHandler,
  type OnNodeDrag,
} from '@xyflow/react'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import { SEVERITY_COLOR } from './constants'
import { buildVisibleGraph, type GraphNodeData } from './graph-adapters'
import { AlertNode } from './nodes/AlertNode'
import { EntityNode } from './nodes/EntityNode'

const nodeTypes = {
  entity: EntityNode,
  alert: AlertNode,
}

function GraphInner({ fitToken }: { fitToken: number }) {
  const {
    activeInvestigation,
    selection,
    hoverEntityIds,
    select,
    expandRelated,
    collapseRelated,
    canExpand,
    isExpanded,
    updateNodePosition,
  } = useWorkspaceStore()

  const { fitView } = useReactFlow()

  const session = activeInvestigation

  const { nodes: derivedNodes, edges: derivedEdges } = useMemo(() => {
    if (!session) return { nodes: [], edges: [] }
    return buildVisibleGraph({
      entities: session.entities,
      alerts: session.alerts,
      edges: session.edges,
      events: session.events,
      filters: {
        entityTypes: new Set(session.filters.entityTypes),
        severities: new Set(session.filters.severities),
        edgeOrigins: new Set(session.filters.edgeOrigins),
        timeRange: session.filters.timeRange,
      },
      selection,
      hoverEntityIds,
    })
  }, [session, selection, hoverEntityIds])

  const [nodes, setNodes, onNodesChange] = useNodesState(derivedNodes as Node[])
  const [rfEdges, setEdges, onEdgesChange] = useEdgesState(derivedEdges)

  useEffect(() => {
    setNodes((current) => {
      const currentById = new Map(current.map((n) => [n.id, n]))
      return (derivedNodes as Node[]).map((n) => {
        const existing = currentById.get(n.id)
        if (existing?.dragging) {
          return { ...n, position: existing.position, dragging: true }
        }
        return n
      })
    })
  }, [derivedNodes, setNodes])

  useEffect(() => {
    setEdges(derivedEdges)
  }, [derivedEdges, setEdges])

  useEffect(() => {
    // Delay until React Flow has measured node dimensions, otherwise the
    // viewport fits an empty bounding box and clips the top rows.
    const t = window.setTimeout(() => {
      fitView({ padding: 0.15, duration: 300 })
    }, 180)
    return () => window.clearTimeout(t)
  }, [fitToken, fitView, derivedNodes.length])

  const onNodeClick: NodeMouseHandler = useCallback(
    (_evt, node) => {
      const data = node.data as GraphNodeData
      if (data.kind === 'entity' && data.entityId) {
        select({ kind: 'entity', id: data.entityId })
      } else if (data.kind === 'alert' && data.alertId) {
        select({ kind: 'alert', id: data.alertId })
      }
    },
    [select],
  )

  const onNodeContextMenu: NodeMouseHandler = useCallback(
    (evt, node) => {
      evt.preventDefault()
      const data = node.data as GraphNodeData
      if (data.kind !== 'entity' || !data.entityId) return
      if (isExpanded(data.entityId)) {
        collapseRelated(data.entityId)
      } else if (canExpand(data.entityId)) {
        expandRelated(data.entityId)
      }
    },
    [canExpand, collapseRelated, expandRelated, isExpanded],
  )

  const onNodeDragStop: OnNodeDrag = useCallback(
    (_evt, node) => {
      updateNodePosition(node.id, node.position)
    },
    [updateNodePosition],
  )

  const onPaneClick = useCallback(() => select(null), [select])

  if (!session) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-[var(--text-dim)]">
        Расследование не выбрано
      </div>
    )
  }

  return (
    <ReactFlow
      nodes={nodes}
      edges={rfEdges}
      nodeTypes={nodeTypes}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onNodeClick={onNodeClick}
      onNodeContextMenu={onNodeContextMenu}
      onNodeDragStop={onNodeDragStop}
      onPaneClick={onPaneClick}
      nodesDraggable
      fitView
      minZoom={0.3}
      maxZoom={1.8}
      proOptions={{ hideAttribution: true }}
      colorMode="dark"
    >
      <Background
        variant={BackgroundVariant.Dots}
        gap={18}
        size={1}
        color="var(--grid-dot)"
      />
      <Controls showInteractive={false} position="bottom-left" />
      <MiniMap
        pannable
        zoomable
        position="bottom-right"
        style={{ width: 140, height: 92 }}
        nodeColor={(n) => {
          const d = n.data as GraphNodeData
          if (d.kind === 'alert' && d.severity) return SEVERITY_COLOR[d.severity]
          return 'var(--border-strong)'
        }}
        maskColor="rgba(0, 0, 0, 0.7)"
      />
    </ReactFlow>
  )
}

export function GraphCanvas({ fitToken }: { fitToken: number }) {
  return (
    <div className="relative h-full w-full">
      <ReactFlowProvider>
        <GraphInner fitToken={fitToken} />
      </ReactFlowProvider>
      <div className="pointer-events-none absolute bottom-3 left-12 rounded-md border border-[var(--border)] bg-[var(--bg-panel)]/90 px-2 py-1 text-[10px] text-[var(--text-dim)]">
        Узлы можно перетаскивать · правый клик по сущности — развернуть или
        свернуть связанные
      </div>
    </div>
  )
}
