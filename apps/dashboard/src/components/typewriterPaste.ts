export type PasteSplit = {
  before: string
  pasted: string
  after: string
}

const MAX_TYPE_MS = 10000
const MIN_DELAY_MS = 16

/** Split `value` so `pasted` replaces the current selection. */
export function applyPaste(
  value: string,
  selectionStart: number,
  selectionEnd: number,
  pasted: string,
): PasteSplit {
  const rawStart = Number.isFinite(selectionStart) ? selectionStart : value.length
  const rawEnd = Number.isFinite(selectionEnd) ? selectionEnd : value.length
  const start = Math.max(0, Math.min(Math.min(rawStart, rawEnd), value.length))
  const end = Math.max(0, Math.min(Math.max(rawStart, rawEnd), value.length))
  return {
    before: value.slice(0, start),
    pasted,
    after: value.slice(end),
  }
}

export function typedPasteValue(split: PasteSplit, typedCount: number): string {
  return split.before + split.pasted.slice(0, typedCount) + split.after
}

export function typedPasteCaret(split: PasteSplit, typedCount: number): number {
  return split.before.length + typedCount
}

/**
 * Pause after `char` given how many characters are still left to type (including this one).
 * Long remaining text stays inside ~10s; short phrases sit around 72–128ms.
 */
export function nextTypeDelay(
  char: string,
  remainingCount: number,
  random: () => number = Math.random,
): number {
  const remaining = Math.max(1, remainingCount)
  const base = remaining > 80 ? 32 : remaining > 40 ? 56 : 72
  const jitter = remaining > 40 ? base * 0.8 : 56
  let delay = base + random() * jitter
  if (char === '.' || char === '!' || char === '?') delay += 320 + random() * 240
  else if (char === ',' || char === ';' || char === ':') delay += 120 + random() * 120
  else if ((char === ' ' || char === '\n') && random() < 0.08) delay += 160

  const budget = MAX_TYPE_MS / remaining
  return Math.max(MIN_DELAY_MS, Math.min(delay, Math.max(MIN_DELAY_MS, budget)))
}
