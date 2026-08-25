import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { useEffect } from 'react'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'
import { ColumnsSection } from './ColumnsSection'
import { FieldCatalogPanel } from './FieldCatalogPanel'
import { FilterSection } from './FilterSection'
import { GroupsSection } from './GroupsSection'
import { PdqlPreview } from './PdqlPreview'

export function PdqlBuilder() {
  const loadFields = usePdqlStore((s) => s.loadFields)
  const addField = usePdqlStore((s) => s.addField)
  const reorder = usePdqlStore((s) => s.reorder)
  const resetQuery = usePdqlStore((s) => s.resetQuery)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))

  useEffect(() => {
    void loadFields()
  }, [loadFields])

  const onDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over) return
    const activeData = active.data.current as
      | { type: 'field'; name: string }
      | { type: 'row'; section: 'filter' | 'columns' | 'groups'; index: number }
      | undefined
    const overData = over.data.current as
      | { type: 'section'; section: 'filter' | 'columns' | 'groups' }
      | { type: 'row'; section: 'filter' | 'columns' | 'groups'; index: number }
      | undefined
    if (activeData?.type === 'field') {
      const section =
        overData?.section ??
        (String(over.id).startsWith('drop-')
          ? (String(over.id).slice(5) as 'filter' | 'columns' | 'groups')
          : undefined)
      if (section) addField(activeData.name, section)
      return
    }
    if (activeData?.type === 'row' && activeData.section === 'columns' && overData?.section === 'groups') {
      const column = usePdqlStore.getState().query.columns[activeData.index]
      if (column?.field) addField(column.field, 'groups')
      return
    }
    if (
      activeData?.type === 'row' &&
      overData?.type === 'row' &&
      activeData.section === overData.section &&
      active.id !== over.id
    ) {
      reorder(activeData.section, activeData.index, overData.index)
    }
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <div className="flex h-full min-h-0">
        <FieldCatalogPanel />
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex items-center justify-between gap-3 border-b border-border px-3 py-2">
            <div className="text-sm text-fg">Конструктор PDQL</div>
            <div className="flex items-center gap-2">
              <Button size="sm" variant="ghost" onClick={resetQuery}>
                Сбросить
              </Button>
            </div>
          </div>
          <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3">
            <FilterSection />
            <ColumnsSection />
            <GroupsSection />
          </div>
          <PdqlPreview />
        </div>
      </div>
    </DndContext>
  )
}
