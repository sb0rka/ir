import { GlobalFilterBar } from '../components/FilterBar'
import { AlertTable } from '../components/AlertTable'
import { QueueDetailPanel } from '../components/QueueDetailPanel'

export function QueuePage() {
  return (
    <div className="flex h-full flex-col">
      <div className="border-b border-border px-4 py-3">
        <h1 className="text-sm font-semibold tracking-wide text-fg">
          Глобальная очередь срабатываний
        </h1>
        <p className="mt-0.5 text-xs text-fg-dim">
          Триаж событий и корреляций из Gateway — откройте строку, чтобы начать расследование
        </p>
      </div>
      <GlobalFilterBar />
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1">
          <AlertTable />
        </div>
        <QueueDetailPanel />
      </div>
    </div>
  )
}
