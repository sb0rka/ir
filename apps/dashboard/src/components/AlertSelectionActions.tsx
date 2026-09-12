import { useState } from 'react'
import { Play, Plus, X } from 'lucide-react'
import { emptyContextQueue, useAppStore } from '../store/appStore'
import { titlesForQueueIds } from '../lib/investigationTitle'
import {
  alertIsInContext,
  contextEventKeys,
  contextImportOptions,
  selectionHasFindings,
} from '../lib/queueContext'
import { Button } from './ui'
import { AddContextModal, type AddContextModalMode } from './AddContextModal'

/** Selection actions for the queue composer row (global or investigation context). */
export function AlertSelectionActions({ investigationId }: { investigationId?: string } = {}) {
  const globalSelected = useAppStore((s) => s.selectedAlertIds)
  const start = useAppStore((s) => s.startInvestigation)
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
  const [modal, setModal] = useState<{ mode: AddContextModalMode; ids: string[] } | null>(null)
  const [modalBusy, setModalBusy] = useState(false)

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
    setModal({ mode: 'add', ids })
  }

  const clearSelection = () => {
    if (investigationId) setContextQueue(investigationId, { selectedIds: [] })
    else clear()
  }

  if (selected.length === 0 && !modal) return null

  return (
    <>
      {selected.length > 0 && (
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
            <Button
              size="sm"
              variant="primary"
              onClick={() => setModal({ mode: 'start', ids: selected })}
            >
              <Play className="h-3 w-3" />
              Начать расследование
            </Button>
          )}
        </div>
      )}
      {modal && (
        <AddContextModal
          mode={modal.mode}
          eventTitles={titlesForQueueIds(modal.ids, alerts, correlations)}
          hasFindings={selectionHasFindings(modal.ids, alerts)}
          busy={modalBusy}
          onClose={() => {
            if (modalBusy) return
            setModal(null)
          }}
          onConfirm={async ({ title, why, expandFindings }) => {
            const options = contextImportOptions(selectionHasFindings(modal.ids, alerts), {
              why,
              expandFindings,
            })
            if (modal.mode === 'start') {
              setModal(null)
              void start(modal.ids, title ?? '', options)
              return
            }
            if (!investigationId) return
            setModalBusy(true)
            try {
              const ok = await addEventsToContext(investigationId, modal.ids, options)
              if (ok) setModal(null)
            } finally {
              setModalBusy(false)
            }
          }}
        />
      )}
    </>
  )
}
