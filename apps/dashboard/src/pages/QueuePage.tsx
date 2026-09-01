import { GlobalQueryComposer } from '../components/QueryComposer'
import { AlertTable } from '../components/AlertTable'
import { EventGroupFilter } from '../components/EventGroupFilter'
import { QueueDetailPanel } from '../components/QueueDetailPanel'
import { useAppStore } from '../store/appStore'

export function QueuePage() {
  const mockSources = useAppStore((state) => state.mockSources)
  const mockSourceLabel = `Активные mock-источники: ${mockSources.join(', ')}`

  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-border px-4 py-3">
        <div className="flex items-center gap-2">
          <h1 className="text-sm font-semibold tracking-wide text-fg">
            Глобальная очередь срабатываний
          </h1>
          {mockSources.length > 0 && (
            <span
              className="rounded border border-medium/40 bg-medium/10 px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wider text-medium"
              title={mockSourceLabel}
              aria-label={mockSourceLabel}
            >
              mock
            </span>
          )}
        </div>
      </div>
      <GlobalQueryComposer />
      <div className="flex min-h-0 flex-1">
        <EventGroupFilter />
        <div className="min-h-0 min-w-0 flex-1">
          <AlertTable />
        </div>
        <QueueDetailPanel />
      </div>
    </div>
  )
}
