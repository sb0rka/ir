import { useEffect, useRef, useState } from 'react'
import { clsx } from '../../lib/utils'
import {
  instantToParts,
  parseTimestamp,
  partsToInstant,
  type WallParts,
} from './model'

const FIELDS = [
  { key: 'day', max: 2, label: 'ДД', sep: '' },
  { key: 'month', max: 2, label: 'ММ', sep: '.' },
  { key: 'year', max: 4, label: 'ГГГГ', sep: '.' },
  { key: 'hour', max: 2, label: 'ЧЧ', sep: '·' },
  { key: 'minute', max: 2, label: 'ММ', sep: ':' },
  { key: 'second', max: 2, label: 'СС', sep: ':' },
] as const

type FieldKey = (typeof FIELDS)[number]['key']

type Draft = Record<FieldKey, string>

function partsToDraft(parts: WallParts): Draft {
  return {
    day: pad(parts.day, 2),
    month: pad(parts.month, 2),
    year: pad(parts.year, 4),
    hour: pad(parts.hour, 2),
    minute: pad(parts.minute, 2),
    second: pad(parts.second, 2),
  }
}

function draftToParts(draft: Draft): WallParts | null {
  const parts = {
    day: Number(draft.day),
    month: Number(draft.month),
    year: Number(draft.year),
    hour: Number(draft.hour),
    minute: Number(draft.minute),
    second: Number(draft.second),
  }
  if (Object.values(parts).some((n) => !Number.isFinite(n))) return null
  return parts
}

function pad(n: number, size: number): string {
  return String(n).padStart(size, '0')
}

export function DateTimeParts({
  iso,
  zone,
  onCommit,
  onEdit,
  size = 'md',
  className,
}: {
  iso: string
  zone: string
  onCommit: (iso: string) => void
  onEdit?: () => void
  size?: 'md' | 'sm'
  className?: string
}) {
  const [draft, setDraft] = useState(() => partsToDraft(instantToParts(iso, zone)))
  const [focused, setFocused] = useState(false)
  const refs = useRef<Array<HTMLInputElement | null>>([])
  const compact = size === 'sm'

  useEffect(() => {
    if (!focused) setDraft(partsToDraft(instantToParts(iso, zone)))
  }, [focused, iso, zone])

  const commit = (next: Draft) => {
    const parts = draftToParts(next)
    if (!parts) {
      setDraft(partsToDraft(instantToParts(iso, zone)))
      return
    }
    const parsed = partsToInstant(parts, zone)
    if (!parsed) {
      setDraft(partsToDraft(instantToParts(iso, zone)))
      return
    }
    const current = instantToParts(iso, zone)
    const unchanged =
      parts.year === current.year &&
      parts.month === current.month &&
      parts.day === current.day &&
      parts.hour === current.hour &&
      parts.minute === current.minute &&
      parts.second === current.second
    if (!unchanged) onCommit(parsed)
  }

  const focusAt = (index: number) => {
    const el = refs.current[index]
    if (!el) return
    el.focus()
    el.select()
  }

  return (
    <div
      role="group"
      aria-label="Время ДДММГГГГ:ЧЧММСС"
      className={clsx(
        'flex min-w-0 flex-1 items-center font-mono tabular-nums',
        compact ? 'text-[11px] leading-4' : 'text-sm',
        className,
      )}
      onFocus={() => setFocused(true)}
      onBlur={(e) => {
        if (e.currentTarget.contains(e.relatedTarget as Node | null)) return
        setFocused(false)
        commit(draft)
      }}
      onPaste={(e) => {
        const text = e.clipboardData.getData('text')
        const parsed = parseTimestamp(text, zone)
        if (!parsed) return
        e.preventDefault()
        onCommit(parsed)
        setDraft(partsToDraft(instantToParts(parsed, zone)))
      }}
    >
      {FIELDS.map((field, index) => (
        <span key={field.key} className="flex items-center">
          {field.sep ? (
            <span
              className={clsx(
                'select-none text-fg-dim',
                field.sep === '·' ? (compact ? 'px-1' : 'px-1.5') : 'px-0.5',
              )}
            >
              {field.sep}
            </span>
          ) : null}
          <input
            ref={(el) => {
              refs.current[index] = el
            }}
            aria-label={field.label}
            inputMode="numeric"
            autoComplete="off"
            spellCheck={false}
            value={draft[field.key]}
            className={clsx(
              'bg-transparent text-center text-fg outline-none',
              field.max === 4 ? 'w-[4.2ch]' : 'w-[2.2ch]',
            )}
            onFocus={(e) => e.currentTarget.select()}
            onChange={(e) => {
              const digits = e.target.value.replace(/\D/g, '').slice(0, field.max)
              const next = { ...draft, [field.key]: digits }
              setDraft(next)
              onEdit?.()
              if (digits.length === field.max && index < FIELDS.length - 1) {
                focusAt(index + 1)
              }
            }}
            onKeyDown={(e) => {
              const el = e.currentTarget
              if (e.key === 'ArrowRight' && el.selectionStart === el.value.length && index < FIELDS.length - 1) {
                e.preventDefault()
                focusAt(index + 1)
              }
              if (e.key === 'ArrowLeft' && el.selectionStart === 0 && index > 0) {
                e.preventDefault()
                focusAt(index - 1)
              }
              if (e.key === 'Backspace' && !el.value && index > 0) {
                e.preventDefault()
                focusAt(index - 1)
              }
              if (e.key === 'Enter') {
                e.preventDefault()
                el.blur()
                commit(draft)
              }
            }}
          />
        </span>
      ))}
    </div>
  )
}
