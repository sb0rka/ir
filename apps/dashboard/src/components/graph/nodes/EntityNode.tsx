import {
  Handle,
  Position,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import {
  Globe,
  Hash,
  Link2,
  Monitor,
  Network,
  Terminal,
  User,
} from 'lucide-react'
import type { GraphNodeData } from '../graph-adapters'
import type { EntityTypeCode } from '../types'

const ICONS: Record<EntityTypeCode, typeof User> = {
  device: Monitor,
  user: User,
  host: Monitor,
  process: Terminal,
  ip: Network,
  mac: Network,
  hostname: Monitor,
  file_hash: Hash,
  domain: Globe,
  url: Link2,
}

export type EntityFlowNode = Node<GraphNodeData, 'entity'>

export function EntityNode({ data }: NodeProps<EntityFlowNode>) {
  const Icon = data.entityType ? ICONS[data.entityType] : Monitor
  const classes = [
    'graph-node',
    'flex min-w-[140px] max-w-[180px] items-center gap-2 rounded-lg border px-2.5 py-2',
    'bg-[var(--bg-node)] border-[var(--border-strong)] text-[var(--text)]',
    data.dimmed ? 'is-dimmed' : '',
    data.highlighted ? 'is-highlighted' : '',
    data.selected ? 'is-selected' : '',
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div className={classes} title={data.tooltip}>
      <Handle
        type="target"
        position={Position.Left}
        className="!h-1.5 !w-1.5 !border-0 !bg-[var(--text-dim)]"
      />
      <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-[var(--bg-panel)] text-[var(--text-muted)]">
        <Icon size={14} />
      </div>
      <div className="min-w-0">
        <div className="truncate text-xs font-medium leading-tight">
          {data.label}
        </div>
        <div className="truncate text-[10px] uppercase tracking-wide text-[var(--text-dim)]">
          {data.sublabel}
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
