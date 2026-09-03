import { useEffect, useRef, useState } from 'react'
import { ChevronDown, Search, X } from 'lucide-react'
import { Button } from './ui'
import { clsx } from '../lib/utils'

export type HeaderSearchColumn = {
  id: string
  label: string
}

/** Collapsed magnifying-glass control that expands left into a search field. */
export function HeaderSearch({
  value,
  onChange,
  placeholder = 'Поиск',
  columns,
  column,
  onColumnChange,
}: {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  columns?: HeaderSearchColumn[]
  column?: string
  onColumnChange?: (column: string) => void
}) {
  const [open, setOpen] = useState(() => value.trim().length > 0)
  const [draft, setDraft] = useState(value)
  const [columnMenuOpen, setColumnMenuOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const rootRef = useRef<HTMLDivElement>(null)
  const hasQuery = draft.trim().length > 0
  const hasColumns = Boolean(columns && columns.length > 0 && onColumnChange)
  const selectedColumn =
    columns?.find((item) => item.id === column) ?? columns?.[0] ?? null

  useEffect(() => {
    setDraft(value)
  }, [value])

  useEffect(() => {
    if (draft === value) return
    const timer = window.setTimeout(() => onChange(draft), 250)
    return () => window.clearTimeout(timer)
  }, [draft, value, onChange])

  useEffect(() => {
    if (!open) {
      setColumnMenuOpen(false)
      return
    }
    inputRef.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.stopImmediatePropagation()
      if (columnMenuOpen) {
        setColumnMenuOpen(false)
        return
      }
      setOpen(false)
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [open, columnMenuOpen])

  useEffect(() => {
    if (!columnMenuOpen) return
    const onPointer = (event: MouseEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return
      setColumnMenuOpen(false)
    }
    window.addEventListener('mousedown', onPointer)
    return () => window.removeEventListener('mousedown', onPointer)
  }, [columnMenuOpen])

  const collapse = () => {
    setColumnMenuOpen(false)
    setOpen(false)
  }

  return (
    <div
      className={clsx(
        'flex items-center',
        open ? 'min-w-0 flex-1 justify-end' : 'ml-auto shrink-0',
      )}
    >
      {open ? (
        <div ref={rootRef} className="relative w-full max-w-md">
          <div
            className={clsx(
              'flex h-8 w-full items-center rounded border border-border bg-surface-0',
              'focus-within:border-fg/40',
            )}
          >
            {hasColumns && selectedColumn ? (
              <div className="relative shrink-0 self-stretch">
                <button
                  type="button"
                  title="Колонка поиска"
                  aria-label="Колонка поиска"
                  aria-expanded={columnMenuOpen}
                  aria-haspopup="listbox"
                  className={clsx(
                    'flex h-full max-w-[9.5rem] items-center gap-1 border-r border-border px-2',
                    'text-[11px] text-fg-muted hover:bg-surface-1 hover:text-fg',
                  )}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => setColumnMenuOpen((current) => !current)}
                >
                  <span className="min-w-0 truncate">{selectedColumn.label}</span>
                  <ChevronDown
                    className={clsx(
                      'h-3 w-3 shrink-0 transition-transform',
                      columnMenuOpen && 'rotate-180',
                    )}
                    aria-hidden
                  />
                </button>
                {columnMenuOpen ? (
                  <div
                    role="listbox"
                    aria-label="Колонка поиска"
                    className="absolute top-full left-0 z-40 mt-1 max-h-64 min-w-[12rem] overflow-auto rounded border border-border bg-surface-1 py-1 shadow-xl"
                  >
                    {columns!.map((item) => {
                      const selected = item.id === selectedColumn.id
                      return (
                        <button
                          key={item.id}
                          type="button"
                          role="option"
                          aria-selected={selected}
                          title={item.label}
                          className={clsx(
                            'flex w-full px-2.5 py-1.5 text-left text-xs outline-none hover:bg-surface-2 hover:text-fg',
                            selected ? 'bg-surface-2 text-fg' : 'text-fg-muted',
                            item.id.startsWith('field:') && 'font-mono',
                          )}
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            onColumnChange!(item.id)
                            setColumnMenuOpen(false)
                            inputRef.current?.focus()
                          }}
                        >
                          <span className="truncate">{item.label}</span>
                        </button>
                      )
                    })}
                  </div>
                ) : null}
              </div>
            ) : (
              <button
                type="button"
                className="shrink-0 rounded p-1.5 text-fg-dim hover:text-fg"
                title="Свернуть поиск"
                aria-label="Свернуть поиск"
                onMouseDown={(event) => event.preventDefault()}
                onClick={collapse}
              >
                <Search className="h-3.5 w-3.5" />
              </button>
            )}
            <input
              ref={inputRef}
              className="h-full min-w-0 flex-1 bg-transparent py-0 pr-2 pl-2 text-xs leading-none outline-none"
              placeholder={placeholder}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              aria-label={placeholder}
            />
            {hasQuery ? (
              <button
                type="button"
                className="shrink-0 p-1.5 text-fg-dim hover:text-fg"
                title="Очистить поиск"
                aria-label="Очистить поиск"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  setDraft('')
                  onChange('')
                  inputRef.current?.focus()
                }}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            ) : hasColumns ? (
              <button
                type="button"
                className="shrink-0 p-1.5 text-fg-dim hover:text-fg"
                title="Свернуть поиск"
                aria-label="Свернуть поиск"
                onMouseDown={(event) => event.preventDefault()}
                onClick={collapse}
              >
                <Search className="h-3.5 w-3.5" />
              </button>
            ) : null}
          </div>
        </div>
      ) : (
        <Button
          size="icon"
          variant="ghost"
          title={placeholder}
          aria-label={placeholder}
          aria-expanded={false}
          onClick={() => setOpen(true)}
          className={hasQuery ? 'text-fg' : undefined}
        >
          <Search className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  )
}
