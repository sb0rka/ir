import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical } from 'lucide-react'
import type { ReactNode } from 'react'
import { clsx } from '../../lib/utils'

export function SortableRow({
  id,
  section,
  index,
  children,
}: {
  id: string
  section: 'filter' | 'columns' | 'groups'
  index: number
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    data: { type: 'row', section, index },
  })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={clsx(
        'flex items-start gap-2 rounded border border-border bg-surface-0 px-2 py-1.5',
        isDragging && 'opacity-60',
      )}
    >
      <button
        type="button"
        className="mt-1 cursor-grab text-fg-dim hover:text-fg"
        {...attributes}
        {...listeners}
        aria-label="Перетащить"
      >
        <GripVertical className="h-3.5 w-3.5" />
      </button>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}
