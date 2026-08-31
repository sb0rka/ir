import { Minus, Plus } from 'lucide-react'
import { useWorkspaceStore } from '../../../state/useWorkspaceStore'
import { useAppStore } from '../../../store/appStore'
import type { GraphNodeData } from '../graph-adapters'

export function HypothesisMembershipBadge({
  nodeId,
  data,
}: {
  nodeId: string
  data: GraphNodeData
}) {
  const investigationId = useWorkspaceStore((s) => s.boundInvestigationId)
  const toggle = useAppStore((s) => s.toggleHypothesisNode)

  if (!data.canToggleHypothesis || !investigationId) return null

  return (
    <button
      type="button"
      className="nodrag nopan absolute -right-1.5 -top-1.5 z-10 flex h-5 w-5 items-center justify-center rounded-full border border-[var(--border-strong)] bg-[var(--bg-panel)] text-[var(--text)] opacity-0 shadow-sm transition-opacity group-hover:opacity-100 hover:border-[var(--accent)]"
      title={data.inHypothesis ? 'Убрать из гипотезы' : 'Добавить в гипотезу'}
      onClick={(event) => {
        event.stopPropagation()
        void toggle(investigationId, nodeId)
      }}
    >
      {data.inHypothesis ? <Minus size={10} /> : <Plus size={10} />}
    </button>
  )
}
