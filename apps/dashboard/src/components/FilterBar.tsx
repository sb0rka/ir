import { useState } from 'react'
import { savedViews, useAppStore } from '../store/appStore'
import { filterFieldLabels } from '../lib/catalog'
import type { FilterChip as FilterChipModel, FilterField } from '../types'
import { Button, Chip } from './ui'
import { clsx } from '../lib/utils'
import { Clock, Filter, History, Search, X } from 'lucide-react'

const FIELDS = Object.keys(filterFieldLabels) as FilterField[]
const TIME_PRESETS = [
  { id: '1h', label: '1ч' },
  { id: '6h', label: '6ч' },
  { id: '24h', label: '24ч' },
  { id: '7d', label: '7д' },
  { id: '30d', label: '30д' },
]

export interface FilterBarProps {
  chips: FilterChipModel[]
  timePreset: string
  onAddChip: (field: FilterField, value: string) => void
  onRemoveChip: (id: string) => void
  onRemoveChipValue: (id: string, value: string) => void
  onClearChips: () => void
  onTimePresetChange: (preset: string) => void
  /** Saved views block; omitted where views make no sense. */
  onApplySavedView?: (id: string) => void
  /** Recently applied filters, most recent first. Click re-applies. */
  history?: Array<{ field: FilterField; value: string }>
  extra?: React.ReactNode
  query?: string
  onQueryChange?: (query: string) => void
}

function useFilterOptions() {
  return useAppStore((s) => s.filterValueOptions)
}

/**
 * The single search/filter row used everywhere: field list → value
 * autocomplete → chip. Chips AND, values inside a chip OR.
 */
export function FilterBar({
  chips,
  timePreset,
  onAddChip,
  onRemoveChip,
  onRemoveChipValue,
  onClearChips,
  onTimePresetChange,
  onApplySavedView,
  history,
  extra,
  query,
  onQueryChange,
}: FilterBarProps) {
  const filterValueOptions = useFilterOptions()
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchField, setSearchFieldRaw] = useState<FilterField | null>(null)
  const [searchQuery, setSearchQuery] = useState('')

  const setSearchField = (field: FilterField | null) => {
    setSearchFieldRaw(field)
    setSearchQuery('')
  }

  const pickValue = (field: FilterField, value: string) => {
    onAddChip(field, value)
    setSearchOpen(false)
    setSearchFieldRaw(null)
    setSearchQuery('')
  }

  const values = searchField
    ? (filterValueOptions[searchField] ?? []).filter((v) =>
        v.toLowerCase().includes(searchQuery.toLowerCase()),
      )
    : []

  return (
    <div className="border-b border-border bg-surface-1 px-4 py-3">
      <div className="relative flex flex-wrap items-center gap-2">
        <div
          className="flex min-h-9 min-w-[320px] flex-1 flex-wrap items-center gap-1.5 rounded border border-border bg-surface-0 px-2 py-1.5"
          onClick={() => setSearchOpen(true)}
        >
          <Search className="h-3.5 w-3.5 text-fg-dim" />
          {query && onQueryChange && (
            <Chip onRemove={() => onQueryChange('')}>
              <span className="text-fg-muted">q:</span> {query}
            </Chip>
          )}
          {chips.map((chip) => (
            <Chip key={chip.id} onRemove={() => onRemoveChip(chip.id)}>
              <span className="text-fg-muted">{filterFieldLabels[chip.field]}:</span>{' '}
              {chip.values.map((v, i) => (
                <span key={v}>
                  {i > 0 && <span className="text-fg-dim"> | </span>}
                  <button
                    type="button"
                    className="hover:underline"
                    onClick={(e) => {
                      e.stopPropagation()
                      onRemoveChipValue(chip.id, v)
                    }}
                    title="Убрать значение"
                  >
                    {v.length > 24 ? `${v.slice(0, 12)}…${v.slice(-6)}` : v}
                  </button>
                </span>
              ))}
            </Chip>
          ))}
          <input
            className="min-w-[120px] flex-1 bg-transparent text-sm outline-none placeholder:text-fg-dim"
            placeholder={chips.length ? 'Добавить фильтр…' : 'Фильтр: хост, IP, хеш, источник…'}
            value={searchQuery}
            onChange={(e) => {
              setSearchOpen(true)
              setSearchQuery(e.target.value)
            }}
            onFocus={() => setSearchOpen(true)}
            onKeyDown={(e) => {
              if (e.key !== 'Enter' || !searchQuery.trim()) return
              if (!searchField && onQueryChange) {
                onQueryChange(searchQuery.trim())
                setSearchQuery('')
                setSearchOpen(false)
              }
            }}
          />
          {(chips.length > 0 || query) && (
            <button
              type="button"
              className="text-fg-dim hover:text-fg"
              onClick={(e) => {
                e.stopPropagation()
                onClearChips()
                onQueryChange?.('')
              }}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>

        <div className="flex items-center gap-1 rounded border border-border bg-surface-0 p-0.5">
          <Clock className="ml-1.5 h-3.5 w-3.5 text-fg-dim" />
          {TIME_PRESETS.map((p) => (
            <button
              key={p.id}
              type="button"
              onClick={() => onTimePresetChange(p.id)}
              className={clsx(
                'rounded px-2 py-1 text-xs',
                timePreset === p.id
                  ? 'bg-surface-3 text-fg'
                  : 'text-fg-muted hover:text-fg',
              )}
            >
              {p.label}
            </button>
          ))}
        </div>

        {onApplySavedView && (
          <div className="flex items-center gap-1">
            <Filter className="h-3.5 w-3.5 text-fg-dim" />
            {savedViews.map((v) => (
              <Button
                key={v.id}
                size="sm"
                variant="ghost"
                onClick={() => onApplySavedView(v.id)}
              >
                {v.name}
              </Button>
            ))}
          </div>
        )}

        {extra}

        {searchOpen && (
          <>
            <div className="fixed inset-0 z-20" onClick={() => setSearchOpen(false)} />
            <div className="absolute left-0 top-full z-30 mt-1 w-[420px] rounded border border-border bg-surface-2 shadow-xl">
              {!searchField ? (
                <div className="p-2">
                  <div className="px-2 py-1 text-[10px] uppercase tracking-wider text-fg-dim">
                    Поле фильтра
                  </div>
                  {FIELDS.map((f) => (
                    <button
                      key={f}
                      type="button"
                      className="flex w-full items-center rounded px-2 py-1.5 text-left text-sm hover:bg-surface-3"
                      onClick={() => setSearchField(f)}
                    >
                      <span className="font-mono text-fg-muted">{filterFieldLabels[f]}</span>
                    </button>
                  ))}
                </div>
              ) : (
                <div className="p-2">
                  <div className="mb-1 flex items-center justify-between px-2">
                    <span className="text-[10px] uppercase tracking-wider text-fg-dim">
                      {filterFieldLabels[searchField]} — значение
                    </span>
                    <button
                      type="button"
                      className="text-xs text-fg-muted hover:text-fg"
                      onClick={() => setSearchField(null)}
                    >
                      ← назад
                    </button>
                  </div>
                  {values.length === 0 && (
                    <div className="px-2 py-2 text-sm text-fg-dim">Нет совпадений</div>
                  )}
                  {values.map((v) => (
                    <button
                      key={v}
                      type="button"
                      className="flex w-full items-center rounded px-2 py-1.5 text-left font-mono text-sm hover:bg-surface-3"
                      onClick={() => pickValue(searchField, v)}
                    >
                      {v.length > 48 ? `${v.slice(0, 24)}…${v.slice(-12)}` : v}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </>
        )}
      </div>

      {history && history.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <History className="h-3 w-3 text-fg-dim" />
          {history.map((h) => (
            <button
              key={`${h.field}:${h.value}`}
              type="button"
              className="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-fg-muted hover:text-fg"
              title="Применить фильтр из истории"
              onClick={() => onAddChip(h.field, h.value)}
            >
              {filterFieldLabels[h.field]}:{' '}
              {h.value.length > 24 ? `${h.value.slice(0, 12)}…${h.value.slice(-6)}` : h.value}
            </button>
          ))}
        </div>
      )}

      <div className="mt-2 text-xs text-fg-dim">
        Чипы через AND · значения внутри чипа через OR · клик по сущности добавит фильтр
      </div>
    </div>
  )
}

/** The global queue's filter row, wired to the app-wide chip state. */
export function GlobalFilterBar() {
  const chips = useAppStore((s) => s.chips)
  const timePreset = useAppStore((s) => s.timePreset)
  const addChip = useAppStore((s) => s.addChip)
  const removeChip = useAppStore((s) => s.removeChip)
  const removeChipValue = useAppStore((s) => s.removeChipValue)
  const setChips = useAppStore((s) => s.setChips)
  const setTimePreset = useAppStore((s) => s.setTimePreset)
  const applySavedView = useAppStore((s) => s.applySavedView)
  const queueQuery = useAppStore((s) => s.queueQuery)
  const setQueueQuery = useAppStore((s) => s.setQueueQuery)

  return (
    <FilterBar
      chips={chips}
      timePreset={timePreset}
      query={queueQuery}
      onQueryChange={setQueueQuery}
      onAddChip={addChip}
      onRemoveChip={removeChip}
      onRemoveChipValue={removeChipValue}
      onClearChips={() => setChips([])}
      onTimePresetChange={setTimePreset}
      onApplySavedView={applySavedView}
    />
  )
}
