import {
  Handle,
  NodeToolbar,
  Position,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import { Crosshair, ShieldAlert } from 'lucide-react'
import { SEVERITY_COLOR } from '../constants'
import type { GraphNodeData } from '../graph-adapters'
import { HypothesisMembershipBadge } from './HypothesisMembershipBadge'

export type AlertFlowNode = Node<GraphNodeData, 'alert'>

export function AlertNode({ id, data }: NodeProps<AlertFlowNode>) {
  const color = data.severity
    ? SEVERITY_COLOR[data.severity]
    : 'var(--text-muted)'
  const Icon = data.isSeed ? Crosshair : ShieldAlert

  const classes = [
    'graph-node',
    'group relative flex min-w-[160px] max-w-[220px] items-start gap-2 rounded-lg border px-2.5 py-2',
    'bg-[var(--bg-node)] text-[var(--text)]',
    data.isSeed ? 'is-seed' : '',
    data.dimmed ? 'is-dimmed' : '',
    data.highlighted ? 'is-highlighted' : '',
    data.selected ? 'is-selected' : '',
    data.inHypothesis ? 'is-in-hypothesis' : '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div className={classes} style={{ borderColor: color }}>
      <HypothesisMembershipBadge nodeId={id} data={data} />
      {data.tooltip && (
        <NodeToolbar
          isVisible={data.highlighted ? true : undefined}
          position={Position.Top}
          className="rounded-md border border-[var(--border)] bg-[var(--bg-panel)] px-2 py-1 text-[10px] tabular-nums text-[var(--text)] shadow-md"
        >
          {data.tooltip}
        </NodeToolbar>
      )}
      <Handle
        type="target"
        position={Position.Left}
        className="!h-1.5 !w-1.5 !border-0 !bg-[var(--text-dim)]"
      />
      <div
        className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md"
        style={{
          background: `color-mix(in srgb, ${color} 20%, transparent)`,
          color,
        }}
      >
        <Icon size={14} />
      </div>
      <div className="min-w-0">
        <div
          className="text-[10px] font-semibold uppercase tracking-wide"
          style={{ color: data.isSeed ? 'var(--text)' : color }}
        >
          {data.isSeed ? `исходный · ${data.sublabel}` : data.sublabel}
        </div>
        <div className="line-clamp-2 text-xs font-medium leading-snug">
          {data.label}
        </div>
      </div>
      <Handle
        type="source"
        position={Position.Right}
        className="!h-1.5 !w-1.5 !border-0 !bg-[var(--text-dim)]"
      />
    </div>
  )
}
