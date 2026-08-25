import { useEffect, useMemo, useState } from 'react'
import { ChevronLeft, Loader2 } from 'lucide-react'
import { resolveFindingEvents } from '../../api/search'
import { errorMessage } from '../../api/error'
import { findingResolveKey } from '../../lib/correlationSubevents'
import { isCorrelationRecord } from '../../lib/pdql'
import { formatTime } from '../../lib/utils'
import type { AlertEvent } from '../../types'
import { resolve, type TimeInterval } from '../time-interval'
import { SeverityBadge } from '../ui'
import { EventFieldModal } from './EventFieldModal'
import {
  EventFields,
  EventHeaderFacts,
  eventCardModelFromAlert,
  type EventCardModel,
} from './EventFields'
import { EventTimeButton } from './EventTimeButton'

export function EventCard({
  event,
  investigationId,
  eventInContext = false,
  timeInterval,
  onTimeChange,
  onTimeExecute,
  onAddFilter,
  onAddToContext,
}: {
  event: EventCardModel
  investigationId?: string
  eventInContext?: boolean
  timeInterval: TimeInterval
  onTimeChange: (value: TimeInterval) => void
  onTimeExecute: (value: TimeInterval) => void
  onAddFilter: (field: string, value: string) => void
  onAddToContext?: (field: string, value: string, includeEvent: boolean) => Promise<void>
}) {
  const [picked, setPicked] = useState<{ field: string; value: string } | null>(null)
  const [openSubevent, setOpenSubevent] = useState<EventCardModel | null>(null)
  const raw = event.raw ?? {}

  useEffect(() => {
    setOpenSubevent(null)
  }, [event.id])

  if (openSubevent) {
    return (
      <div className="space-y-3">
        <button
          type="button"
          className="inline-flex items-center gap-1 text-xs text-fg-muted hover:text-fg"
          onClick={() => setOpenSubevent(null)}
        >
          <ChevronLeft className="h-3.5 w-3.5" />
          К корреляции
        </button>
        <EventCard
          event={openSubevent}
          investigationId={investigationId}
          eventInContext={eventInContext}
          timeInterval={timeInterval}
          onTimeChange={onTimeChange}
          onTimeExecute={onTimeExecute}
          onAddFilter={onAddFilter}
          onAddToContext={onAddToContext}
        />
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <EventTimeButton
          time={event.time}
          current={timeInterval}
          onChange={onTimeChange}
          onExecute={onTimeExecute}
        />
        <div className="text-sm font-medium leading-snug">{event.title}</div>
        {event.description && event.description !== event.title && (
          <p className="text-xs leading-relaxed text-fg-muted">{event.description}</p>
        )}
      </div>
      <EventHeaderFacts
        source={event.source}
        raw={raw}
        severity={event.severity}
        onValueClick={(field, value) => setPicked({ field, value })}
      />
      <EventFields
        source={event.source}
        raw={raw}
        onValueClick={(field, value) => setPicked({ field, value })}
      />
      <CorrelationSubevents
        event={event}
        timeInterval={timeInterval}
        onOpen={(subevent) => setOpenSubevent(eventCardModelFromAlert(subevent))}
      />
      {picked && (
        <EventFieldModal
          field={picked.field}
          value={picked.value}
          investigationId={investigationId}
          eventInContext={eventInContext}
          onClose={() => setPicked(null)}
          onAddFilter={onAddFilter}
          onAddToContext={
            onAddToContext
              ? (includeEvent) => onAddToContext(picked.field, picked.value, includeEvent)
              : undefined
          }
        />
      )}
    </div>
  )
}

function CorrelationSubevents({
  event,
  timeInterval,
  onOpen,
}: {
  event: EventCardModel
  timeInterval: TimeInterval
  onOpen: (event: AlertEvent) => void
}) {
  const raw = event.raw ?? {}
  const source = event.source
  const sourceEventId = event.sourceEventId
  const refSource = event.findingRef?.source_code
  const refInstance = event.findingRef?.source_instance
  const refType = event.findingRef?.record_type
  const refId = event.findingRef?.external_id
  const refFrom = event.findingRef?.time_range.from
  const refTo = event.findingRef?.time_range.to
  const rawUuid = raw.uuid ?? ''
  const fallbackFrom = resolve(timeInterval).from
  const fallbackTo = resolve(timeInterval).to
  const findingKind = raw.finding_kind ?? ''
  const shouldLoad =
    findingKind === 'siem_incident' ||
    findingKind === 'siem_correlation' ||
    isCorrelationRecord({
      finding_kind: findingKind,
      correlation_name: raw.correlation_name ?? '',
    })
  const key = useMemo(() => {
    if (!shouldLoad) return null
    return findingResolveKey(
      {
        source,
        sourceEventId,
        raw: {
          uuid: rawUuid,
          finding_kind: findingKind,
          correlation_name: raw.correlation_name ?? '',
        },
        findingRef:
          refType && refSource && refId && refFrom && refTo
            ? {
                source_code: refSource,
                source_instance: refInstance,
                record_type: refType,
                external_id: refId,
                time_range: { from: refFrom, to: refTo },
              }
            : undefined,
      },
      { from: fallbackFrom, to: fallbackTo },
    )
  }, [
    shouldLoad,
    source,
    sourceEventId,
    rawUuid,
    findingKind,
    raw.correlation_name,
    refSource,
    refInstance,
    refType,
    refId,
    refFrom,
    refTo,
    fallbackFrom,
    fallbackTo,
  ])

  const [state, setState] = useState<
    | { status: 'idle' }
    | { status: 'loading' }
    | { status: 'ready'; events: AlertEvent[]; errors: string[] }
    | { status: 'error'; message: string }
  >({ status: 'idle' })

  useEffect(() => {
    if (!key) {
      setState({ status: 'idle' })
      return
    }
    let cancelled = false
    setState({ status: 'loading' })
    void resolveFindingEvents(key)
      .then((result) => {
        if (cancelled) return
        setState({ status: 'ready', events: result.events, errors: result.errors })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setState({ status: 'error', message: errorMessage(err) })
      })
    return () => {
      cancelled = true
    }
  }, [key])

  if (!shouldLoad || !key) return null

  return (
    <div>
      <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">
        {key.record_type === 'siem_incident' ? 'События' : 'Субевенты'}
      </div>
      {state.status === 'idle' || state.status === 'loading' ? (
        <div className="flex items-center gap-1.5 text-xs text-fg-dim">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          Загрузка…
        </div>
      ) : null}
      {state.status === 'error' && <div className="text-xs text-critical">{state.message}</div>}
      {state.status === 'ready' && (
        <>
          {state.errors.length > 0 && (
            <div className="mb-1.5 text-xs text-fg-muted">{state.errors.join(' · ')}</div>
          )}
          {state.events.length === 0 ? (
            <div className="text-xs text-fg-dim">
              {key.record_type === 'siem_incident' ? 'Нет событий' : 'Нет субевентов'}
            </div>
          ) : (
            <div className="space-y-1">
              {state.events.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  className="flex w-full items-start justify-between gap-2 rounded border border-border px-2 py-1.5 text-left text-xs hover:bg-surface-2"
                  onClick={() => onOpen(item)}
                >
                  <span className="min-w-0">
                    <span className="block truncate text-fg">{item.title}</span>
                    <span className="text-[11px] text-fg-dim">
                      {formatTime(item.time)}
                      {item.source ? ` · ${item.source}` : ''}
                    </span>
                  </span>
                  <SeverityBadge severity={item.severity} />
                </button>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
