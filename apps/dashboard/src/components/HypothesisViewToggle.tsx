import {
  hypothesisViewModeLabel,
  type HypothesisViewMode,
} from '../lib/hypotheses'
import { useAppStore } from '../store/appStore'
import { clsx } from '../lib/utils'

const MODES: HypothesisViewMode[] = ['dim', 'isolate']

export function HypothesisViewToggle({ investigationId }: { investigationId: string }) {
  const mode = useAppStore((s) => s.hypothesisViewMode[investigationId] ?? 'dim')
  const setMode = useAppStore((s) => s.setHypothesisViewMode)

  return (
    <div className="flex rounded border border-border p-0.5" title="Как показать гипотезу на графе">
      {MODES.map((id) => (
        <button
          key={id}
          type="button"
          className={clsx(
            'rounded px-1.5 py-0.5 text-[10px]',
            mode === id ? 'bg-surface-3 text-fg' : 'text-fg-muted',
          )}
          onClick={() => setMode(investigationId, id)}
        >
          {hypothesisViewModeLabel(id)}
        </button>
      ))}
    </div>
  )
}
