import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import type { Severity } from '../types'
import { clsx, severityDot } from '../lib/utils'

/** Truncates long text; on hover scrolls as a marquee when content overflows. */
export function MarqueeText({ text, className }: { text: string; className?: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const [overflowing, setOverflowing] = useState(false)
  const [hovering, setHovering] = useState(false)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el || hovering) return
    const measure = () => {
      setOverflowing(el.scrollWidth > el.clientWidth + 1)
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [text, hovering])

  const run = hovering && overflowing
  // ~10 chars/sec, clamped so short overflows still read and long ones finish in time
  const durationSec = Math.min(Math.max(text.length / 10, 3), 24)

  return (
    <div
      ref={ref}
      className={clsx('min-w-0 overflow-hidden whitespace-nowrap', !run && 'truncate', className)}
      title={run ? undefined : text}
      onMouseEnter={() => setHovering(true)}
      onMouseLeave={() => setHovering(false)}
    >
      {run ? (
        <span
          className="inline-block will-change-transform"
          style={{ animation: `marquee-scroll ${durationSec}s linear infinite` }}
        >
          <span className="inline-block pr-10">{text}</span>
          <span className="inline-block pr-10" aria-hidden>
            {text}
          </span>
        </span>
      ) : (
        text
      )}
    </div>
  )
}

export type SegmentedControlOption<T extends string = string> = {
  id: T
  label: string
}

/** Labeled segmented control in QueueSourceToggle chrome. */
export function SegmentedControl<T extends string>({
  label,
  value,
  options,
  onChange,
  size = 'md',
  'aria-label': ariaLabel,
}: {
  label?: string
  value: T
  options: ReadonlyArray<SegmentedControlOption<T>>
  onChange: (value: T) => void
  size?: 'sm' | 'md'
  'aria-label'?: string
}) {
  const segmentClass = clsx(
    'inline-flex h-full items-center leading-none',
    size === 'sm' ? 'px-2 text-xs' : 'px-2.5 text-xs',
  )

  return (
    <div
      className={clsx(
        'inline-flex items-stretch overflow-hidden rounded border border-border bg-surface-0',
        size === 'sm' ? 'h-6' : 'h-9',
      )}
      role="group"
      aria-label={ariaLabel ?? label}
    >
      {label ? (
        <span className={clsx(segmentClass, 'bg-surface-1 uppercase tracking-wide text-fg-dim')}>
          {label}
        </span>
      ) : null}
      {options.map((option, index) => (
        <button
          key={option.id}
          type="button"
          aria-pressed={value === option.id}
          onClick={() => onChange(option.id)}
          className={clsx(
            segmentClass,
            (label != null || index > 0) && 'border-l border-border',
            value === option.id ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:text-fg',
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

export function SeverityBadge({ severity, label }: { severity: Severity; label?: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide">
      <span className={clsx('h-2 w-2 rounded-full', severityDot[severity])} />
      <span className="text-fg-muted">{label ?? severity}</span>
    </span>
  )
}

export function Chip({
  children,
  onRemove,
  onClick,
  title,
  tone = 'default',
  flash = false,
}: {
  children: React.ReactNode
  onRemove?: () => void
  onClick?: () => void
  title?: string
  tone?: 'default' | 'proposed' | 'confirmed' | 'rejected'
  flash?: boolean
}) {
  const tones = {
    default: 'border-border bg-surface-2 text-fg',
    proposed: 'border-proposed/40 bg-proposed/10 text-proposed',
    confirmed: 'border-confirmed/40 bg-confirmed/10 text-confirmed',
    rejected: 'border-rejected bg-surface-2 text-fg-dim line-through',
  }
  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs font-medium',
        tones[tone],
        onClick && 'cursor-pointer hover:border-fg/40',
        flash && 'chip-flash-critical',
      )}
      title={title}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onClick={onClick}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onClick()
              }
            }
          : undefined
      }
    >
      {children}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onRemove()
          }}
          className="ml-0.5 text-fg-dim hover:text-fg"
        >
          ×
        </button>
      )}
    </span>
  )
}

export function Button({
  children,
  onClick,
  variant = 'default',
  size = 'md',
  disabled,
  className,
  type = 'button',
  title,
  'aria-label': ariaLabel,
  tabIndex,
}: {
  children: React.ReactNode
  onClick?: (e: React.MouseEvent) => void
  variant?: 'default' | 'primary' | 'ghost' | 'danger'
  size?: 'sm' | 'md' | 'icon'
  disabled?: boolean
  className?: string
  type?: 'button' | 'submit'
  title?: string
  'aria-label'?: string
  tabIndex?: number
}) {
  const variants = {
    default: 'border-border bg-surface-2 text-fg hover:bg-surface-3',
    primary: 'border-fg/30 bg-fg/90 text-surface-0 hover:bg-fg',
    ghost: 'border-transparent bg-transparent text-fg-muted hover:bg-surface-2 hover:text-fg',
    danger: 'border-critical/40 bg-critical/10 text-critical hover:bg-critical/20',
  }
  const sizes = {
    sm: 'px-2 py-1 text-xs',
    md: 'px-3 py-1.5 text-sm',
    icon: 'h-8 w-8 p-0',
  }
  return (
    <button
      type={type}
      disabled={disabled}
      title={title}
      aria-label={ariaLabel}
      tabIndex={tabIndex}
      onClick={onClick}
      className={clsx(
        'inline-flex items-center justify-center gap-1.5 rounded border font-medium transition-colors disabled:opacity-40',
        variants[variant],
        sizes[size],
        className,
      )}
    >
      {children}
    </button>
  )
}

export function Select<T extends string>({
  value,
  options,
  onChange,
  'aria-label': ariaLabel,
  className,
}: {
  value: T
  options: ReadonlyArray<{ value: T; label: string }>
  onChange: (value: T) => void
  'aria-label'?: string
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const label = options.find((option) => option.value === value)?.label ?? value

  useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      event.stopImmediatePropagation()
      setOpen(false)
    }
    const onPointer = (event: MouseEvent) => {
      if (rootRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('mousedown', onPointer)
    return () => {
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('mousedown', onPointer)
    }
  }, [open])

  return (
    <div ref={rootRef} className={clsx('relative', className)}>
      <button
        type="button"
        aria-label={ariaLabel}
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() => setOpen((current) => !current)}
        className="inline-flex w-full items-center gap-1.5 rounded border border-border bg-surface-0 py-1.5 pr-2 pl-2.5 text-xs text-fg outline-none focus:border-fg/40"
      >
        <span className="min-w-0 flex-1 truncate text-left">{label}</span>
        <ChevronDown
          className={clsx('h-3.5 w-3.5 shrink-0 text-fg-dim transition-transform', open && 'rotate-180')}
          aria-hidden
        />
      </button>
      {open ? (
        <div
          role="listbox"
          aria-label={ariaLabel}
          className="absolute right-0 top-full z-40 mt-1 min-w-full overflow-hidden rounded border border-border bg-surface-1 py-1 shadow-xl"
        >
          {options.map((option) => {
            const selected = option.value === value
            return (
              <button
                key={option.value}
                type="button"
                role="option"
                aria-selected={selected}
                className={clsx(
                  'flex w-full px-2.5 py-1.5 text-left text-xs outline-none hover:bg-surface-2 hover:text-fg',
                  selected ? 'bg-surface-2 text-fg' : 'text-fg-muted',
                )}
                onClick={() => {
                  onChange(option.value)
                  setOpen(false)
                }}
              >
                {option.label}
              </button>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}

export function ErrorBanner({
  message,
  tone = 'error',
  onDismiss,
}: {
  message: string | null
  tone?: 'error' | 'warning'
  onDismiss?: () => void
}) {
  if (!message) return null
  return (
    <div
      className={clsx(
        'flex items-start justify-between gap-3 border-b px-4 py-2 text-xs',
        tone === 'warning'
          ? 'border-proposed/40 bg-proposed/10 text-fg'
          : 'border-critical/40 bg-critical/10 text-critical',
      )}
    >
      <span>{message}</span>
      {onDismiss && (
        <button type="button" className="shrink-0 text-fg-dim hover:text-fg" onClick={onDismiss}>
          ×
        </button>
      )}
    </div>
  )
}

export function Panel({
  title,
  children,
  actions,
  className,
  side = 'right',
}: {
  title?: React.ReactNode
  children: React.ReactNode
  actions?: React.ReactNode
  className?: string
  side?: 'left' | 'right'
}) {
  return (
    <div
      className={clsx(
        'flex h-full flex-col border-border bg-surface-1',
        side === 'left' ? 'border-r' : 'border-l',
        className,
      )}
    >
      {title && (
        <div className="flex items-center justify-between border-b border-border px-3 py-2">
          <div className="text-xs font-semibold uppercase tracking-wider text-fg-muted">
            {title}
          </div>
          {actions}
        </div>
      )}
      <div className="flex-1 overflow-auto">{children}</div>
    </div>
  )
}
