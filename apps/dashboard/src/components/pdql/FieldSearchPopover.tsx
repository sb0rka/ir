import { Plus, Search } from 'lucide-react'
import { useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useBuilder } from './queryBuilder'
import type { ActiveSection } from '../../lib/pdql'
import { Button } from '../ui'
import { FieldSearchList } from './FieldSearchList'

const MENU_WIDTH = 320

export function FieldSearchPopover({
  section,
  parentId = null,
}: {
  section: ActiveSection
  parentId?: string | null
}) {
  const { fields, fieldFreq, addField, setActiveSection } = useBuilder()
  const anchorRef = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [box, setBox] = useState<{ top: number; left: number; listMaxHeight: number } | null>(null)

  useLayoutEffect(() => {
    if (!open) {
      setQuery('')
      setBox(null)
      return
    }
    const place = () => {
      const rect = anchorRef.current?.getBoundingClientRect()
      if (!rect) return
      const left = Math.max(8, Math.min(rect.right - MENU_WIDTH, window.innerWidth - MENU_WIDTH - 8))
      const top = rect.bottom + 4
      const listMaxHeight = Math.max(120, Math.min(288, window.innerHeight - top - 48))
      setBox({ top, left, listMaxHeight })
    }
    place()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [open])

  const choose = (name: string) => {
    addField(name, section, parentId)
    setOpen(false)
  }

  return (
    <div className="relative" ref={anchorRef}>
      <Button
        size="sm"
        variant="ghost"
        title="Добавить поле"
        onClick={() => {
          setActiveSection(section)
          setOpen((value) => !value)
        }}
      >
        <Plus className="h-3.5 w-3.5" />
        Добавить
      </Button>
      {open &&
        box &&
        createPortal(
          <>
            <div className="fixed inset-0 z-[80]" onClick={() => setOpen(false)} />
            <div
              className="fixed z-[90] w-80 overflow-hidden rounded border border-border bg-surface-2 shadow-xl"
              style={{ top: box.top, left: box.left }}
            >
              <label className="flex items-center gap-1.5 border-b border-border px-2 py-1.5">
                <Search className="h-3.5 w-3.5 text-fg-dim" />
                <input
                  autoFocus
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Найти поле"
                  className="w-full bg-transparent text-xs text-fg outline-none placeholder:text-fg-dim"
                />
              </label>
              <div className="overflow-auto" style={{ maxHeight: box.listMaxHeight }}>
                <FieldSearchList
                  idPrefix="popover"
                  fields={fields}
                  freq={fieldFreq}
                  query={query}
                  onChoose={choose}
                  onActivate={choose}
                />
              </div>
            </div>
          </>,
          document.body,
        )}
    </div>
  )
}
