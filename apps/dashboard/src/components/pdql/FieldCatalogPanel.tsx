import { Search } from 'lucide-react'
import { useRef, useState } from 'react'
import { useBuilder } from './queryBuilder'
import type { ActiveSection } from '../../lib/pdql'
import { clsx } from '../../lib/utils'
import { FieldSearchList } from './FieldSearchList'

const MODES: { id: ActiveSection; label: string }[] = [
  { id: 'filter', label: 'Фильтр' },
  { id: 'columns', label: 'Колонки' },
  { id: 'groups', label: 'Группы' },
]

export function FieldCatalogPanel() {
  const builder = useBuilder()
  const fields = builder.fields.filter((field) => {
    if (builder.activeSection === 'columns' && field.column === false) return false
    if (builder.activeSection === 'groups' && field.group === false) return false
    return true
  })
  const frozenFreq = useRef(builder.fieldFreq).current
  const fieldsLoading = builder.fieldsLoading
  const fieldsError = builder.fieldsError
  const activeSection = builder.activeSection
  const setActiveSection = builder.setActiveSection
  const addField = builder.addField
  const [query, setQuery] = useState('')

  return (
    <aside className="flex h-full w-80 shrink-0 flex-col border-r border-border bg-surface-1">
      <div className="border-b border-border px-3 py-2">
        <div className="text-xs font-semibold uppercase tracking-wider text-fg-muted">Поля</div>
        <div className="mt-2 grid grid-cols-3 gap-1 rounded border border-border bg-surface-0 p-0.5">
          {MODES.map((mode) => (
            <button
              key={mode.id}
              type="button"
              onClick={() => setActiveSection(mode.id)}
              className={clsx(
                'rounded px-1.5 py-1 text-[11px]',
                activeSection === mode.id ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:text-fg',
              )}
            >
              {mode.label}
            </button>
          ))}
        </div>
        <label className="mt-2 flex items-center gap-1.5 rounded border border-border bg-surface-0 px-2 py-1">
          <Search className="h-3.5 w-3.5 text-fg-dim" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Найти поле"
            className="w-full bg-transparent text-xs text-fg outline-none placeholder:text-fg-dim"
          />
        </label>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        {fieldsLoading && <div className="px-3 py-4 text-xs text-fg-dim">Загрузка каталога…</div>}
        {fieldsError && <div className="px-3 py-4 text-xs text-critical">{fieldsError}</div>}
        {!fieldsLoading && !fieldsError && (
          <FieldSearchList
            fields={fields}
            freq={frozenFreq}
            query={query}
            onChoose={(name) => addField(name)}
          />
        )}
      </div>
    </aside>
  )
}
