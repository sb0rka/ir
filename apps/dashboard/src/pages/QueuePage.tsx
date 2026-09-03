import { GlobalQueryComposer } from '../components/QueryComposer'
import { AlertTable } from '../components/AlertTable'
import { EventGroupFilter } from '../components/EventGroupFilter'
import { QueueDetailPanel } from '../components/QueueDetailPanel'

export function QueuePage() {

  return (
    <div className="flex h-full flex-col">
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
