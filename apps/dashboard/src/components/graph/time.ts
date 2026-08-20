export function toMs(iso: string): number {
  return new Date(iso).getTime()
}

export function formatClock(isoOrMs: string | number): string {
  const d = typeof isoOrMs === 'number' ? new Date(isoOrMs) : new Date(isoOrMs)
  return d.toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: 'UTC',
  })
}

export function formatEventTooltip(ts: string | number, title: string): string {
  return `${formatClock(ts)} — ${title}`
}

export function formatShortDate(isoOrMs: string | number): string {
  const d = typeof isoOrMs === 'number' ? new Date(isoOrMs) : new Date(isoOrMs)
  return (
    d.toLocaleString('en-GB', {
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
      timeZone: 'UTC',
    }) + ' UTC'
  )
}

export function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n))
}

export function nowIso(): string {
  return new Date().toISOString()
}
