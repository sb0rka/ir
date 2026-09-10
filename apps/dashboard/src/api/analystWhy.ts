import type { EventOrigin } from '../types'

/** Event-node caption: analyst why only for analyst-added nodes, else the event title. */
export function eventNodeLabel(input: {
  why?: string | null
  fallback: string
  origin?: EventOrigin | null
}): string {
  if (input.origin === 'analyst') {
    return input.why?.trim() || input.fallback
  }
  return input.fallback
}
