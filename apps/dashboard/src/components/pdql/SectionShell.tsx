import { useDroppable } from '@dnd-kit/core'
import type { ReactNode } from 'react'
import type { ActiveSection } from '../../lib/pdql'
import { usePdqlStore } from '../../store/pdqlStore'
import { clsx } from '../../lib/utils'
import { FieldSearchPopover } from './FieldSearchPopover'

export function SectionShell({
  section,
  title,
  children,
}: {
  section: ActiveSection
  title: string
  children: ReactNode
}) {
  const activeSection = usePdqlStore((s) => s.activeSection)
  const setActiveSection = usePdqlStore((s) => s.setActiveSection)
  const { setNodeRef, isOver } = useDroppable({
    id: `drop-${section}`,
    data: { type: 'section', section },
  })
  const active = activeSection === section
  return (
    <section
      ref={setNodeRef}
      onClick={() => setActiveSection(section)}
      className={clsx(
        'rounded border p-2',
        active ? 'border-fg/40 bg-surface-2/60' : 'border-border bg-surface-1',
        isOver && 'border-proposed/60',
      )}
    >
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">{title}</h2>
        <FieldSearchPopover section={section} />
      </div>
      <div className="flex flex-col gap-1.5">{children}</div>
    </section>
  )
}
