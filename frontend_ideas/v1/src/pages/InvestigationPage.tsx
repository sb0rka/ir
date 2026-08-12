import { useEffect } from 'react'
import { InvestigationHeader, ContextTable } from '../components/InvestigationHeader'
import { ContextQueuePage } from '../components/ContextQueue'
import { InvestigationGraph } from '../components/graph'
import { DetailPanel } from '../components/DetailPanel'
import { AgentPanel } from '../components/AgentPanel'
import { ProposedReviewOverlay } from '../components/ProposedReviewOverlay'
import { useAppStore } from '../store/appStore'
import { useWorkspaceStore } from '../state/useWorkspaceStore'

export function InvestigationPage({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const detailPanelOpen = useAppStore((s) => s.detailPanelOpen)
  const agentPanelOpen = useAppStore((s) => s.agentPanelOpen)
  const bindInvestigation = useWorkspaceStore((s) => s.bindInvestigation)

  useEffect(() => {
    bindInvestigation(investigationId)
    return () => bindInvestigation(null)
  }, [investigationId, bindInvestigation])

  if (!inv) {
    return (
      <div className="flex h-full items-center justify-center text-fg-dim">
        Расследование не найдено
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <InvestigationHeader investigationId={investigationId} />
      <div className="flex min-h-0 flex-1">
        <div className="relative flex min-w-0 flex-1 flex-col">
          {inv.view === 'table' ? (
            <ContextTable investigationId={investigationId} />
          ) : inv.view === 'queue' ? (
            <ContextQueuePage investigationId={investigationId} />
          ) : (
            <div className="relative flex min-h-0 flex-1 flex-col">
              <InvestigationGraph fitNonce={investigationId.length} />
              <ProposedReviewOverlay investigationId={investigationId} />
            </div>
          )}
        </div>
        {detailPanelOpen && <DetailPanel investigationId={investigationId} />}
        {agentPanelOpen && <AgentPanel investigationId={investigationId} />}
      </div>
    </div>
  )
}
