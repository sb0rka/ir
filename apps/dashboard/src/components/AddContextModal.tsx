import { useEffect, useRef, useState, type ClipboardEvent } from 'react'
import { createPortal } from 'react-dom'
import {
  applyPaste,
  nextTypeDelay,
  typedPasteCaret,
  typedPasteValue,
  type PasteSplit,
} from './typewriterPaste'
import { Button, Chip } from './ui'

type TypingJob = PasteSplit & { index: number }

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export type AddContextModalMode = 'start' | 'add' | 'hypothesis' | 'create-hypothesis'

export type AddContextModalResult = {
  title?: string
  why: string
  expandFindings: boolean
}

const MODE_COPY: Record<
  AddContextModalMode,
  { kicker: string; heading: string; submit: string; aria: string }
> = {
  start: {
    kicker: 'Новое расследование',
    heading: 'Название',
    submit: 'Создать',
    aria: 'Новое расследование',
  },
  add: {
    kicker: 'Контекст',
    heading: 'Добавить в расследование',
    submit: 'Добавить',
    aria: 'Добавить в расследование',
  },
  hypothesis: {
    kicker: 'Гипотеза',
    heading: 'Добавить в гипотезу',
    submit: 'Добавить',
    aria: 'Добавить в гипотезу',
  },
  'create-hypothesis': {
    kicker: 'Гипотеза',
    heading: 'Создать гипотезу',
    submit: 'Создать',
    aria: 'Создать гипотезу',
  },
}

export function AddContextModal({
  mode,
  eventTitles,
  hasFindings,
  busy,
  onClose,
  onConfirm,
}: {
  mode: AddContextModalMode
  eventTitles: string[]
  hasFindings: boolean
  busy?: boolean
  onClose: () => void
  onConfirm: (input: AddContextModalResult) => void | Promise<void>
}) {
  const titleRef = useRef<HTMLInputElement>(null)
  const whyRef = useRef<HTMLTextAreaElement>(null)
  const typingRef = useRef<TypingJob | null>(null)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [title, setTitle] = useState(() =>
    mode === 'start' && eventTitles.length === 1 ? eventTitles[0] : '',
  )
  const [why, setWhy] = useState('')
  const [expandFindings, setExpandFindings] = useState(false)
  const copy = MODE_COPY[mode]
  const canSubmit = (mode !== 'start' || title.trim().length > 0) && !busy

  const placeCaret = (offset: number) => {
    const el = whyRef.current
    if (!el) return
    el.setSelectionRange(offset, offset)
  }

  const clearTimer = () => {
    if (timerRef.current == null) return
    clearTimeout(timerRef.current)
    timerRef.current = null
  }

  const cancelTyping = () => {
    clearTimer()
    typingRef.current = null
  }

  const flushTyping = (): string => {
    const job = typingRef.current
    if (!job) return why
    cancelTyping()
    const full = typedPasteValue(job, job.pasted.length)
    setWhy(full)
    placeCaret(typedPasteCaret(job, job.pasted.length))
    return full
  }

  const tickTyping = () => {
    const job = typingRef.current
    if (!job) return
    job.index += 1
    const next = typedPasteValue(job, job.index)
    setWhy(next)
    const caret = typedPasteCaret(job, job.index)
    requestAnimationFrame(() => placeCaret(caret))
    if (job.index >= job.pasted.length) {
      typingRef.current = null
      return
    }
    const typed = job.pasted[job.index - 1] ?? ''
    const remaining = job.pasted.length - job.index
    timerRef.current = setTimeout(tickTyping, nextTypeDelay(typed, remaining))
  }

  const startTyping = (split: PasteSplit) => {
    cancelTyping()
    if (split.pasted.length === 0) return
    typingRef.current = { ...split, index: 0 }
    timerRef.current = setTimeout(tickTyping, nextTypeDelay(split.pasted[0] ?? '', split.pasted.length))
  }

  const close = () => {
    if (busy) return
    cancelTyping()
    onClose()
  }

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || busy) return
      cancelTyping()
      onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose])

  useEffect(() => {
    if (mode === 'start') {
      titleRef.current?.focus()
      if (eventTitles.length === 1) titleRef.current?.select()
      return
    }
    whyRef.current?.focus()
  }, [eventTitles.length, mode])

  useEffect(() => () => cancelTyping(), [])

  const submit = () => {
    if (!canSubmit) return
    const nextTitle = title.trim()
    if (mode === 'start' && !nextTitle) return
    const nextWhy = flushTyping()
    void onConfirm({
      title: mode === 'start' ? nextTitle : undefined,
      why: nextWhy.trim(),
      expandFindings: hasFindings ? expandFindings : false,
    })
  }

  const onWhyPaste = (event: ClipboardEvent<HTMLTextAreaElement>) => {
    const pasted = event.clipboardData.getData('text/plain')
    if (!pasted) return
    event.preventDefault()
    const el = whyRef.current
    const current = el?.value ?? why
    const split = applyPaste(
      current,
      el?.selectionStart ?? current.length,
      el?.selectionEnd ?? current.length,
      pasted,
    )
    if (prefersReducedMotion()) {
      cancelTyping()
      setWhy(typedPasteValue(split, split.pasted.length))
      requestAnimationFrame(() => placeCaret(typedPasteCaret(split, split.pasted.length)))
      return
    }
    setWhy(typedPasteValue(split, 0))
    requestAnimationFrame(() => placeCaret(typedPasteCaret(split, 0)))
    startTyping(split)
  }

  // Escape WorkspaceSidebar's z-10 stacking context so sticky table headers stay under the dim.
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={busy ? undefined : close} />
      <form
        role="dialog"
        aria-label={copy.aria}
        className="relative w-full max-w-lg overflow-hidden rounded border border-border bg-surface-1 shadow-xl"
        onSubmit={(event) => {
          event.preventDefault()
          submit()
        }}
      >
        <div className="border-b border-border px-4 py-3">
          <div className="text-[10px] uppercase tracking-wider text-fg-dim">{copy.kicker}</div>
          <div className="mt-0.5 text-sm text-fg">{copy.heading}</div>
        </div>

        <div className="space-y-3 p-4">
          {mode === 'start' && (
            <>
              <input
                ref={titleRef}
                className="w-full rounded border border-border bg-surface-0 px-2 py-1.5 text-sm outline-none focus:border-fg/30"
                placeholder="Название расследования"
                maxLength={255}
                disabled={busy}
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
              {eventTitles.length > 1 && (
                <div className="space-y-1.5">
                  <div className="text-[10px] uppercase tracking-wider text-fg-dim">
                    Вставить название события
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {eventTitles.map((eventTitle) => (
                      <Chip key={eventTitle} onClick={() => setTitle(eventTitle)} title={eventTitle}>
                        <span className="max-w-[18rem] truncate">{eventTitle}</span>
                      </Chip>
                    ))}
                  </div>
                </div>
              )}
            </>
          )}
          {mode !== 'start' && eventTitles.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {eventTitles.map((eventTitle) => (
                <Chip key={eventTitle} title={eventTitle}>
                  <span className="max-w-[18rem] truncate">{eventTitle}</span>
                </Chip>
              ))}
            </div>
          )}
          {hasFindings && (
            <label className="flex items-start gap-2 text-xs text-fg-muted">
              <input
                type="checkbox"
                className="mt-0.5 accent-fg"
                checked={expandFindings}
                disabled={busy}
                onChange={(event) => setExpandFindings(event.target.checked)}
              />
              <span>
                <span className="text-fg">Resolve</span>
                <span className="mt-0.5 block text-[11px] leading-relaxed">
                  Загрузить вложенные события, сессии и сущности
                </span>
              </span>
            </label>
          )}
          <label className="block space-y-1.5">
            <span className="text-[10px] uppercase tracking-wider text-fg-dim">Почему</span>
            <textarea
              ref={whyRef}
              className="w-full resize-none rounded border border-border bg-surface-0 px-2 py-1.5 text-sm outline-none focus:border-fg/30"
              rows={3}
              disabled={busy}
              value={why}
              onPaste={onWhyPaste}
              onChange={(event) => {
                cancelTyping()
                setWhy(event.target.value)
              }}
              placeholder="Зачем эта нода в графе"
            />
          </label>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-2">
          <Button size="sm" variant="ghost" onClick={close} disabled={busy}>
            Отмена
          </Button>
          <Button size="sm" variant="primary" type="submit" disabled={!canSubmit}>
            {copy.submit}
          </Button>
        </div>
      </form>
    </div>,
    document.body,
  )
}
