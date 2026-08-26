import type { Severity } from '../types'
import { clsx, severityDot } from '../lib/utils'

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
}: {
  children: React.ReactNode
  onRemove?: () => void
  onClick?: () => void
  title?: string
  tone?: 'default' | 'proposed' | 'confirmed' | 'rejected'
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
        'inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs font-mono',
        tones[tone],
        onClick && 'cursor-pointer hover:border-fg/40',
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
  tabIndex,
}: {
  children: React.ReactNode
  onClick?: (e: React.MouseEvent) => void
  variant?: 'default' | 'primary' | 'ghost' | 'danger'
  size?: 'sm' | 'md'
  disabled?: boolean
  className?: string
  type?: 'button' | 'submit'
  title?: string
  tabIndex?: number
}) {
  const variants = {
    default: 'border-border bg-surface-2 text-fg hover:bg-surface-3',
    primary: 'border-fg/30 bg-fg text-surface-0 hover:bg-white',
    ghost: 'border-transparent bg-transparent text-fg-muted hover:bg-surface-2 hover:text-fg',
    danger: 'border-critical/40 bg-critical/10 text-critical hover:bg-critical/20',
  }
  const sizes = {
    sm: 'px-2 py-1 text-xs',
    md: 'px-3 py-1.5 text-sm',
  }
  return (
    <button
      type={type}
      disabled={disabled}
      title={title}
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
