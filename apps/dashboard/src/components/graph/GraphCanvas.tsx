import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  Background,
  BackgroundVariant,
  ControlButton,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
  useStore,
  type Node,
  type NodeMouseHandler,
  type OnNodeDrag,
} from '@xyflow/react'
import { LayoutGrid } from 'lucide-react'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import { useAppStore } from '../../store/appStore'
import { SEVERITY_COLOR } from './constants'
import { buildVisibleGraph, type GraphNodeData } from './graph-adapters'
import { AlertNode } from './nodes/AlertNode'
import { EntityNode } from './nodes/EntityNode'

const nodeTypes = {
  entity: EntityNode,
  alert: AlertNode,
}

type FitToken = number | string

function GraphInner({ fitToken }: { fitToken: FitToken }) {
  const {
    activeInvestigation: session,
    selection,
    hoverEventId,
    select,
    expandRelated,
    collapseRelated,
    canExpand,
    isExpanded,
    updateNodePosition,
    arrangeNodes,
  } = useWorkspaceStore()
  const hypothesisId = useAppStore((s) =>
    session?.id ? (s.activeHypothesisId[session.id] ?? null) : null,
  )
  const membership = useAppStore((s) =>
    hypothesisId ? (s.hypothesisMembership[hypothesisId] ?? null) : null,
  )
  const lensWritable = useAppStore((s) =>
    hypothesisId ? s.hypotheses[hypothesisId]?.status !== 'resolved' : false,
  )
  const viewMode = useAppStore((s) =>
    session?.id ? (s.hypothesisViewMode[session.id] ?? 'dim') : 'dim',
  )
  const hypothesisLens = useMemo(() => {
    if (!hypothesisId) return null
    return {
      nodeIds: new Set(membership?.nodeIds ?? []),
      edgeIds: new Set(membership?.edgeIds ?? []),
      writable: lensWritable,
      mode: viewMode,
    }
  }, [hypothesisId, membership, lensWritable, viewMode])

  const { fitView } = useReactFlow()
  const paneWidth = useStore((s) => s.width)
  const paneHeight = useStore((s) => s.height)
  const hasSize = paneWidth > 0 && paneHeight > 0
  const nodesMeasured = useStore((s) => {
    if (s.nodes.length === 0) return false
    return s.nodes.every((n) => (n.measured?.width ?? 0) > 0)
  })
  const fittedSizeKey = useRef<string | null>(null)

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
      hoverEventId,
      hypothesisLens,
    })
  }, [session, selection, hoverEventId, hypothesisLens])

  // Length alone misses “same count, new ids” after agent enrichment.
  const graphSig = useMemo(() => {
    const nodeIds = derivedNodes.map((n) => n.id).sort().join(',')
    const edgeIds = derivedEdges.map((e) => e.id).sort().join(',')
    return `${nodeIds}|${edgeIds}`
  }, [derivedNodes, derivedEdges])

  const [nodes, setNodes, onNodesChange] = useNodesState(derivedNodes as Node[])
  const [rfEdges, setEdges, onEdgesChange] = useEdgesState(derivedEdges)

  useEffect(() => {
    setNodes((current) => {
      const currentById = new Map(current.map((n) => [n.id, n]))
      return (derivedNodes as Node[]).map((n) => {
        const existing = currentById.get(n.id)
        if (!existing) return n
        // Keep measured size so edges stay attached across selection/hover updates.
        return {
          ...n,
          position: existing.dragging ? existing.position : n.position,
          dragging: existing.dragging,
          measured: existing.measured,
          width: existing.width,
          height: existing.height,
        }
      })
    })
  }, [derivedNodes, setNodes])

  useEffect(() => {
    setEdges(derivedEdges)
  }, [derivedEdges, setEdges])

  useEffect(() => {
    if (!hasSize || !nodesMeasured || derivedNodes.length === 0) return
    const key = `${fitToken}:${paneWidth}x${paneHeight}:${graphSig}`
    if (fittedSizeKey.current === key) return
    const duration = fittedSizeKey.current === null ? 0 : 200
    fittedSizeKey.current = key
    void fitView({ padding: 0.15, duration })
  }, [
    fitToken,
    paneWidth,
    paneHeight,
    fitView,
    graphSig,
    derivedNodes.length,
    hasSize,
    nodesMeasured,
  ])

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

  const onArrange = useCallback(() => {
    arrangeNodes()
    const next = useWorkspaceStore.getState().activeInvestigation
    if (!next) return
    const posById = new Map<string, { x: number; y: number }>([
      ...next.entities.map((e) => [e.id, e.position] as const),
      ...next.alerts.map((a) => [a.id, a.position] as const),
    ])
    setNodes((current) =>
      current.map((n) => ({
        ...n,
        position: posById.get(n.id) ?? n.position,
      })),
    )
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => {
        void fitView({ padding: 0.15, duration: 200 })
      })
    })
  }, [arrangeNodes, fitView, setNodes])

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
      minZoom={0.3}
      maxZoom={1.8}
      proOptions={{ hideAttribution: true }}
      colorMode="dark"
      style={{ width: '100%', height: '100%' }}
    >
      <Background
        variant={BackgroundVariant.Dots}
        gap={18}
        size={1}
        color="var(--grid-dot)"
      />
      <Controls
        showInteractive={false}
        position="top-right"
        orientation="horizontal"
      >
        <ControlButton onClick={onArrange} title="Arrange nodes" aria-label="Arrange nodes">
          <LayoutGrid size={16} />
        </ControlButton>
      </Controls>
      <MiniMap
        pannable
        zoomable
        position="bottom-right"
        style={{ width: 140, height: 92 }}
        nodeColor={(n) => {
          const d = n.data as GraphNodeData
          if (d.kind === 'alert' && d.isSeed) return 'var(--accent)'
          if (d.kind === 'alert' && d.severity) return SEVERITY_COLOR[d.severity]
          return 'var(--border-strong)'
        }}
        maskColor="rgba(0, 0, 0, 0.7)"
      />
    </ReactFlow>
  )
}

export function GraphCanvas({ fitToken }: { fitToken: FitToken }) {
  const session = useWorkspaceStore((s) => s.activeInvestigation)
  const wrapRef = useRef<HTMLDivElement>(null)
  const [ready, setReady] = useState(false)

  // Mount React Flow only after the flex slot has a real box. On first tab open
  // the parent is often 0×0 for a frame; initializing then leaves the viewport empty.
  useLayoutEffect(() => {
    const el = wrapRef.current
    if (!el) return
    let cancelled = false
    const update = () => {
      if (cancelled) return
      const r = el.getBoundingClientRect()
      if (r.width > 1 && r.height > 1) setReady(true)
    }
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    const raf = window.requestAnimationFrame(() => {
      window.requestAnimationFrame(update)
    })
    return () => {
      cancelled = true
      ro.disconnect()
      window.cancelAnimationFrame(raf)
    }
  }, [session?.id])

  return (
    <div ref={wrapRef} className="absolute inset-0">
      {session && ready ? (
        <ReactFlowProvider key={session.id}>
          <GraphInner fitToken={fitToken} />
        </ReactFlowProvider>
      ) : (
        <div className="flex h-full items-center justify-center text-sm text-[var(--text-dim)]">
          {session ? '' : 'Расследование не выбрано'}
        </div>
      )}
      <div className="pointer-events-none absolute bottom-3 left-3 rounded-md border border-[var(--border)] bg-[var(--bg-panel)]/90 px-2 py-1 text-[10px] text-[var(--text-dim)]">
        Узлы можно перетаскивать · правый клик по сущности — развернуть или
        свернуть связанные
      </div>
    </div>
  )
}
