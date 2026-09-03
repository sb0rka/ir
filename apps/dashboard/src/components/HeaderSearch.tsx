import { useEffect, useRef, useState } from 'react'
import { Search, X } from 'lucide-react'
import { Button } from './ui'
import { clsx } from '../lib/utils'

/** Collapsed magnifying-glass control that expands left into a search field. */
export function HeaderSearch({
  value,
  onChange,
  placeholder,
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
}) {
  const [open, setOpen] = useState(() => value.trim().length > 0)
  const [draft, setDraft] = useState(value)
  const inputRef = useRef<HTMLInputElement>(null)
  const hasQuery = draft.trim().length > 0

  useEffect(() => {
    setDraft(value)
  }, [value])

  useEffect(() => {
    if (draft === value) return
    const timer = window.setTimeout(() => onChange(draft), 250)
    return () => window.clearTimeout(timer)
  }, [draft, value, onChange])

  useEffect(() => {
    if (!open) return
    inputRef.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.stopImmediatePropagation()
      setOpen(false)
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [open])

  const collapse = () => setOpen(false)

  return (
    <div
      className={clsx(
        'flex items-center',
        open ? 'min-w-0 flex-1 justify-end' : 'ml-auto shrink-0',
      )}
    >
      {open ? (
        <label className="relative w-full max-w-md">
          <button
            type="button"
            className="absolute top-1/2 left-1.5 z-10 -translate-y-1/2 rounded p-0.5 text-fg-dim hover:text-fg"
            title="Свернуть поиск"
            aria-label="Свернуть поиск"
            onMouseDown={(event) => event.preventDefault()}
            onClick={collapse}
          >
            <Search className="h-3.5 w-3.5" />
          </button>
          <input
            ref={inputRef}
            className="h-8 w-full rounded border border-border bg-surface-0 py-0 pr-8 pl-7 text-xs leading-none outline-none focus:border-fg/40"
            placeholder={placeholder}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            aria-label={placeholder}
          />
          {hasQuery ? (
            <button
              type="button"
              className="absolute top-1/2 right-2 -translate-y-1/2 text-fg-dim hover:text-fg"
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
          ) : null}
        </label>
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
