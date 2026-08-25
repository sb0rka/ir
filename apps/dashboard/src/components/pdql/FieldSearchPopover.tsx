import { Plus, Search } from 'lucide-react'
import { useEffect, useState } from 'react'
import { usePdqlStore } from '../../store/pdqlStore'
import type { ActiveSection } from '../../lib/pdql'
import { Button } from '../ui'
import { FieldSearchList } from './FieldSearchList'

export function FieldSearchPopover({ section }: { section: ActiveSection }) {
  const fields = usePdqlStore((s) => s.fields)
  const fieldFreq = usePdqlStore((s) => s.fieldFreq)
  const addField = usePdqlStore((s) => s.addField)
  const setActiveSection = usePdqlStore((s) => s.setActiveSection)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')

  useEffect(() => {
    if (!open) setQuery('')
  }, [open])

  return (
    <div className="relative">
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
      {open && (
        <>
          <div className="fixed inset-0 z-[60]" onClick={() => setOpen(false)} />
          <div className="absolute right-0 top-full z-[70] mt-1 w-80 overflow-hidden rounded border border-border bg-surface-2 shadow-xl">
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
            <div className="max-h-72 overflow-auto">
              <FieldSearchList
                idPrefix="popover"
                fields={fields}
                freq={fieldFreq}
                query={query}
                onChoose={(name) => {
                  addField(name, section)
                  setOpen(false)
                }}
                onActivate={(name) => {
                  addField(name, section)
                  setOpen(false)
                }}
              />
            </div>
          </div>
        </>
      )}
    </div>
  )
}
