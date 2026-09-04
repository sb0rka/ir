import { useEffect, useLayoutEffect } from 'react'
import { ContextTable } from '../components/InvestigationHeader'
import { ContextQueuePage } from '../components/ContextQueue'
import { InvestigationGraph } from '../components/graph'
import { DetailPanel } from '../components/DetailPanel'
import { QueueDetailPanel } from '../components/QueueDetailPanel'
import { WorkspaceSidebar } from '../components/WorkspaceSidebar'
import { useAppStore } from '../store/appStore'
import { useWorkspaceStore } from '../state/useWorkspaceStore'

export function InvestigationPage({ investigationId }: { investigationId: string }) {
  const inv = useAppStore((s) => s.investigations[investigationId])
  const detailPanelOpen = useAppStore((s) => s.detailPanelOpen)
  const loadInvestigation = useAppStore((s) => s.loadInvestigation)
  const loading = useAppStore((s) => s.investigationLoading)

  useEffect(() => {
    void loadInvestigation(investigationId)
  }, [investigationId, loadInvestigation])

  // Layout (not paint): children see session on the first frame that has size.
  // Cleanup must not clear a newer tab's binding — InvestigationPage stays mounted A→B.
  useLayoutEffect(() => {
    useWorkspaceStore.getState().bindInvestigation(investigationId)
    return () => {
      const ws = useWorkspaceStore.getState()
      if (ws.boundInvestigationId === investigationId) {
        ws.bindInvestigation(null)
      }
    }
  }, [investigationId])

  if (!inv) {
    return (
      <div className="flex h-full items-center justify-center text-fg-dim">
        {loading ? 'Загрузка расследования…' : 'Расследование не найдено'}
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-1">
      <WorkspaceSidebar investigationId={investigationId} />
      <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
        {inv.view === 'table' ? (
          <ContextTable investigationId={investigationId} />
        ) : inv.view === 'queue' ? (
          <div className="flex min-h-0 flex-1">
            <div className="min-w-0 flex-1">
              <ContextQueuePage investigationId={investigationId} />
            </div>
            <QueueDetailPanel investigationId={investigationId} />
          </div>
        ) : loading && inv.nodeIds.length === 0 ? (
          <div className="flex h-full items-center justify-center text-fg-dim">
            Загрузка расследования…
          </div>
        ) : (
          <div className="relative flex min-h-0 flex-1 flex-col">
            <InvestigationGraph fitNonce={investigationId} />
          </div>
        )}
      </div>
      {detailPanelOpen && inv.view !== 'queue' && (
        <DetailPanel investigationId={investigationId} />
      )}
    </div>
  )
}
