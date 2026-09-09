/** Event-node caption: IR why, else the event title. */
export function eventNodeLabel(input: { why?: string | null; fallback: string }): string {
  return input.why?.trim() || input.fallback
}
