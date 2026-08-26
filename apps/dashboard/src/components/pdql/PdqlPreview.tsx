import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { usePdqlStore } from '../../store/pdqlStore'
import { Button } from '../ui'

export function PdqlPreview() {
  const pdqlDraft = usePdqlStore((s) => s.pdqlDraft)
  const parseError = usePdqlStore((s) => s.parseError)
  const setPdqlDraft = usePdqlStore((s) => s.setPdqlDraft)
  const applyPdql = usePdqlStore((s) => s.applyPdql)
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(pdqlDraft)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1200)
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="border-t border-border bg-surface-1 p-3">
      <div className="mb-1.5 flex items-center justify-between">
        <div className="text-xs font-semibold uppercase tracking-wider text-fg-muted">PDQL</div>
        <div className="flex items-center gap-1">
          <Button size="sm" variant="ghost" onClick={() => void copy()}>
            {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
            Копировать
          </Button>
          <Button size="sm" onClick={() => applyPdql()}>
            Применить
          </Button>
        </div>
      </div>
      <textarea
        value={pdqlDraft}
        onChange={(e) => setPdqlDraft(e.target.value)}
        onBlur={() => applyPdql()}
        spellCheck={false}
        className="h-24 w-full resize-none rounded border border-border bg-surface-0 px-2 py-1.5 font-mono text-xs text-fg outline-none focus:border-fg/40"
      />
      {parseError && (
        <div className="mt-1 text-[11px] text-critical">
          {parseError.message}
          {parseError.position > 0 ? ` (позиция ${parseError.position})` : ''}
        </div>
      )}
    </div>
  )
}
