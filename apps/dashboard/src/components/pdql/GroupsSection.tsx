import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { ChevronDown, ChevronUp, X } from 'lucide-react'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'
import { SectionShell } from './SectionShell'
import { SortableRow } from './SortableRow'

export function GroupsSection() {
  const query = usePdqlStore((s) => s.query)
  const removeGroup = usePdqlStore((s) => s.removeGroup)
  const moveGroup = usePdqlStore((s) => s.moveGroup)

  return (
    <SectionShell section="groups" title="Группировка">
      {query.groups.length === 0 && (
        <div className="px-1 py-2 text-xs text-fg-dim">
          Нет группировки. При добавлении поля колонки получат агрегатные функции.
        </div>
      )}
      <SortableContext items={query.groups.map((item) => item.id)} strategy={verticalListSortingStrategy}>
        {query.groups.map((group, index) => (
          <SortableRow key={group.id} id={group.id} section="groups" index={index}>
            <div className="flex items-center gap-1.5">
              <span className="font-mono text-xs text-fg">{group.field}</span>
              <div className="ml-auto flex items-center gap-0.5">
                <Button size="sm" variant="ghost" title="Вверх" onClick={() => moveGroup(index, -1)}>
                  <ChevronUp className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="ghost" title="Вниз" onClick={() => moveGroup(index, 1)}>
                  <ChevronDown className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="ghost" title="Удалить" onClick={() => removeGroup(group.id)}>
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </SortableRow>
        ))}
      </SortableContext>
    </SectionShell>
  )
}
