import { useEffect, useState } from 'react'
import { GraphCanvas } from './GraphCanvas'
import { GraphToolbar } from './GraphToolbar'
import { Timeline } from './Timeline'

/** Full graph + timeline stack. Details use the host DetailPanel, not GraphDetailsDrawer. */
export function InvestigationGraph({
  /** Bump to trigger fitView (e.g. when opening the graph view). */
  fitNonce = 0,
}: {
  fitNonce?: number
} = {}) {
  const [fitToken, setFitToken] = useState(0)

  useEffect(() => {
    setFitToken((n) => n + 1)
  }, [fitNonce])

  return (
    <div className="flex h-full min-h-0 flex-1 flex-col">
      <GraphToolbar onFit={() => setFitToken((n) => n + 1)} />
      <div className="relative min-h-0 flex-1">
        <GraphCanvas fitToken={fitToken} />
      </div>
      <Timeline />
    </div>
  )
}
