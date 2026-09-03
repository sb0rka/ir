import { useState } from 'react'
import { Loader2, Play, Plus, X } from 'lucide-react'
import { emptyContextQueue, useAppStore } from '../store/appStore'
import { titlesForQueueIds } from '../lib/investigationTitle'
import { alertIsInContext, contextEventKeys } from '../lib/queueContext'
import { Button } from './ui'
import { StartInvestigationModal } from './StartInvestigationModal'

/** Selection actions for the queue composer row (global or investigation context). */
export function AlertSelectionActions({ investigationId }: { investigationId?: string } = {}) {
  const globalSelected = useAppStore((s) => s.selectedAlertIds)
  const start = useAppStore((s) => s.startInvestigation)
  const starting = useAppStore((s) => s.investigationLoading)
  const clear = useAppStore((s) => s.clearAlertSelection)
  const globalAlerts = useAppStore((s) => s.alerts)
  const correlations = useAppStore((s) => s.correlations)
  const queue = useAppStore((s) =>
    investigationId ? (s.contextQueue[investigationId] ?? emptyContextQueue) : null,
  )
  const inv = useAppStore((s) => (investigationId ? s.investigations[investigationId] : undefined))
  const contextEvents = useAppStore((s) => s.contextEvents)
  const setContextQueue = useAppStore((s) => s.setContextQueue)
  const addEventsToContext = useAppStore((s) => s.addEventsToContext)
  const [naming, setNaming] = useState(false)

  const alerts = queue?.alerts ?? globalAlerts
  const eventKeys = inv ? contextEventKeys(inv.eventIds, contextEvents) : new Set<string>()
  const findingKeys = new Set(inv?.findingSourceKeys ?? [])
  const selected = investigationId ? (queue?.selectedIds ?? []) : globalSelected

  const inContextOf = (alertId: string) => {
    const alert = alerts[alertId]
    return Boolean(investigationId && alert && alertIsInContext(alert, findingKeys, eventKeys))
  }

  const addSelected = () => {
    if (!investigationId) return
    const ids = selected.filter((id) => !inContextOf(id))
    if (ids.length === 0) return
    void addEventsToContext(investigationId, ids)
  }

  const clearSelection = () => {
    if (investigationId) setContextQueue(investigationId, { selectedIds: [] })
    else clear()
  }

  if (selected.length === 0) return null

  return (
    <>
      <div className="ml-auto flex flex-wrap items-center gap-2">
        <span className="text-xs text-fg-muted">Выбрано: {selected.length}</span>
        <Button
          size="icon"
          variant="ghost"
          title="Сбросить"
          aria-label="Сбросить"
          onClick={clearSelection}
        >
          <X className="h-3.5 w-3.5" />
        </Button>
        {investigationId ? (
          <Button size="sm" variant="primary" onClick={addSelected}>
            <Plus className="h-3 w-3" />
            Добавить в расследование
          </Button>
        ) : (
          <Button size="sm" variant="primary" disabled={starting} onClick={() => setNaming(true)}>
            {starting ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <Play className="h-3 w-3" />
            )}
            Начать расследование
          </Button>
        )}
      </div>
      {naming && !investigationId && (
        <StartInvestigationModal
          eventTitles={titlesForQueueIds(selected, alerts, correlations)}
          busy={starting}
          onClose={() => setNaming(false)}
          onConfirm={async (title) => {
            const createdId = await start(selected, title)
            if (createdId) setNaming(false)
          }}
        />
      )}
    </>
  )
}
