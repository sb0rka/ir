import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronLeft, RefreshCw } from 'lucide-react'
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
  eventCardModelFromAlert,
  type EventCardModel,
} from './EventFields'
import { EventTimeButton } from './EventTimeButton'

type ResolveBundle = Awaited<ReturnType<typeof resolveFindingEvents>>
type SectionId = 'users' | 'hosts' | 'events'

type UsersState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ready'; names: string[] }
  | { status: 'error'; message: string }

type CountState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ready'; total: number }
  | { status: 'error'; message: string }

function SectionRetryButton({
  onClick,
  busy = false,
}: {
  onClick: () => void
  busy?: boolean
}) {
  return (
    <button
      type="button"
      title="Повторить"
      aria-label="Повторить"
      disabled={busy}
      className="inline-flex shrink-0 items-center justify-center rounded p-0.5 text-fg-muted hover:text-fg disabled:opacity-40"
      onClick={onClick}
    >
      <RefreshCw className={`h-3 w-3 ${busy ? 'animate-spin' : ''}`} />
    </button>
  )
}

export function EventCard({
  event,
  investigationId,
  eventInContext = false,
  timeInterval,
  onTimeChange,
  onTimeExecute,
  onAddFilter,
  onFilterFindingUuid,
  onAddToContext,
}: {
  event: EventCardModel
  investigationId?: string
  eventInContext?: boolean
  timeInterval: TimeInterval
  onTimeChange: (value: TimeInterval) => void
  onTimeExecute: (value: TimeInterval) => void
  onAddFilter: (field: string, value: string) => void
  onFilterFindingUuid?: (uuid: string, recordType: 'siem_incident' | 'siem_correlation') => void
  onAddToContext?: (field: string, value: string, includeEvent: boolean) => Promise<void>
}) {
  const [picked, setPicked] = useState<{ field: string; value: string } | null>(null)
  const [openSubevent, setOpenSubevent] = useState<EventCardModel | null>(null)
  const raw = event.raw ?? {}
  const findingRecordType =
    event.findingRef?.record_type === 'siem_incident' ||
    event.findingRef?.record_type === 'siem_correlation'
      ? event.findingRef.record_type
      : raw.finding_kind === 'siem_incident'
        ? 'siem_incident'
        : raw.finding_kind === 'siem_correlation' || isCorrelationRecord(raw)
          ? 'siem_correlation'
          : null

  const onValueClick = (field: string, value: string) => {
    if (field === 'uuid' && findingRecordType && onFilterFindingUuid) {
      onFilterFindingUuid(value, findingRecordType)
      return
    }
    setPicked({ field, value })
  }

  useEffect(() => {
    setOpenSubevent(null)
  }, [event.id])

  const backLabel = parentBackLabel(event)

  if (openSubevent) {
    return (
      <div className="space-y-3">
        <button
          type="button"
          className="inline-flex max-w-full items-center gap-1 text-xs text-fg-muted hover:text-fg"
          onClick={() => setOpenSubevent(null)}
          title={backLabel}
        >
          <ChevronLeft className="h-3.5 w-3.5 shrink-0" />
          <span className="truncate">{backLabel}</span>
        </button>
        <EventCard
          event={openSubevent}
          investigationId={investigationId}
          eventInContext={eventInContext}
          timeInterval={timeInterval}
          onTimeChange={onTimeChange}
          onTimeExecute={onTimeExecute}
          onAddFilter={onAddFilter}
          onFilterFindingUuid={onFilterFindingUuid}
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
          severity={event.severity}
        />
        <div className="text-sm font-medium leading-snug">{event.title}</div>
        {event.description && event.description !== event.title && (
          <p className="text-xs leading-relaxed text-fg-muted">{event.description}</p>
        )}
      </div>
      <EventFields
        source={event.source}
        raw={raw}
        onValueClick={onValueClick}
      />
      <CorrelationSubevents
        event={event}
        timeInterval={timeInterval}
        onOpen={(subevent) => setOpenSubevent(eventCardModelFromAlert(subevent))}
        onValueClick={onValueClick}
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

/** ~10 rows of subevent buttons (46px + 4px gap) before internal scroll. */
const SUBEVENT_LIST_MAX_PX = 10 * 46 + 9 * 4
/** First paint + each infinite-scroll page — keep at viewport size so the panel stays scrollable. */
const SUBEVENT_PAGE = 10

function parentBackLabel(parent: EventCardModel): string {
  const named =
    parent.raw?.correlation_name?.trim() ||
    parent.raw?.['rule.name']?.trim() ||
    parent.title?.trim() ||
    ''
  if (named) {
    const short = named.length > 52 ? `${named.slice(0, 52)}…` : named
    return `К ${short}`
  }
  const kind = parent.findingRef?.record_type || parent.raw?.finding_kind
  if (kind === 'siem_incident') return 'К инциденту'
  if (kind === 'siem_correlation') return 'К корреляции'
  return 'Назад'
}

function CorrelationSubevents({
  event,
  timeInterval,
  onOpen,
  onValueClick,
}: {
  event: EventCardModel
  timeInterval: TimeInterval
  onOpen: (event: AlertEvent) => void
  onValueClick: (field: string, value: string) => void
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

  const [visibleEventCount, setVisibleEventCount] = useState(SUBEVENT_PAGE)
  const [visibleHostCount, setVisibleHostCount] = useState(SUBEVENT_PAGE)
  const [allEvents, setAllEvents] = useState<AlertEvent[]>([])
  const [allHosts, setAllHosts] = useState<{ value: string; roles: string[] }[]>([])
  const [users, setUsers] = useState<UsersState>({ status: 'idle' })
  const [hosts, setHosts] = useState<CountState>({ status: 'idle' })
  const [events, setEvents] = useState<CountState>({ status: 'idle' })
  const usersRef = useRef(users)
  const hostsRef = useRef(hosts)
  const eventsRef = useRef(events)
  usersRef.current = users
  hostsRef.current = hosts
  eventsRef.current = events
  const loadGen = useRef(0)

  const applyUsers = useCallback((result: ResolveBundle, forceError?: string) => {
    if (forceError) {
      setUsers({ status: 'error', message: forceError })
      return
    }
    setUsers({ status: 'ready', names: result.accounts })
  }, [])

  const applyHosts = useCallback((result: ResolveBundle, forceError?: string) => {
    if (forceError) {
      setHosts({ status: 'error', message: forceError })
      setAllHosts([])
      return
    }
    setHosts({ status: 'ready', total: result.hosts.length })
    setAllHosts(result.hosts)
    setVisibleHostCount(SUBEVENT_PAGE)
  }, [])

  const applyEvents = useCallback((result: ResolveBundle, forceError?: string) => {
    if (forceError) {
      setEvents({ status: 'error', message: forceError })
      setAllEvents([])
      return
    }
    setEvents({ status: 'ready', total: result.events.length })
    setAllEvents(result.events)
    setVisibleEventCount(SUBEVENT_PAGE)
  }, [])

  const revealAfterUsers = useCallback(
    (result: ResolveBundle, gen: number, forceError?: string) => {
      if (loadGen.current !== gen) return
      if (hostsRef.current.status === 'idle' || hostsRef.current.status === 'loading') {
        applyHosts(result, forceError)
      }
      queueMicrotask(() => {
        if (loadGen.current !== gen) return
        if (eventsRef.current.status === 'idle' || eventsRef.current.status === 'loading') {
          applyEvents(result, forceError)
        }
      })
    },
    [applyHosts, applyEvents],
  )

  const loadSections = useCallback(
    (
      sections: SectionId[],
      options: { attempt?: number; revealChain?: boolean } = {},
    ) => {
      if (!key || sections.length === 0) return
      const attempt = options.attempt ?? 0
      const revealChain = options.revealChain ?? false
      const incident = key.record_type === 'siem_incident'
      const gen = ++loadGen.current
      const wanted = new Set(sections)

      if (wanted.has('users') && incident) setUsers({ status: 'loading' })
      if (wanted.has('hosts') && incident) setHosts({ status: 'loading' })
      if (wanted.has('events')) setEvents({ status: 'loading' })

      void resolveFindingEvents(key)
        .then((result) => {
          if (loadGen.current !== gen) return
          const usersEmpty = !wanted.has('users') || result.accounts.length === 0
          const hostsEmpty = !wanted.has('hosts') || result.hosts.length === 0
          const eventsEmpty = !wanted.has('events') || result.events.length === 0
          const softFail =
            result.errors.length > 0 && usersEmpty && hostsEmpty && eventsEmpty
          if (softFail && attempt < 1) {
            window.setTimeout(() => {
              if (loadGen.current !== gen) return
              loadSections(sections, { attempt: attempt + 1, revealChain })
            }, 800)
            return
          }
          const forceError = softFail ? result.errors.join(' · ') : undefined
          if (!incident) {
            if (wanted.has('events')) applyEvents(result, forceError)
            return
          }
          if (wanted.has('users')) {
            applyUsers(result, forceError)
            if (revealChain || hostsRef.current.status === 'idle') {
              queueMicrotask(() => revealAfterUsers(result, gen, forceError))
            }
          }
          if (wanted.has('hosts')) applyHosts(result, forceError)
          if (wanted.has('events')) applyEvents(result, forceError)
          if (wanted.has('hosts') && !wanted.has('events') && eventsRef.current.status === 'idle') {
            queueMicrotask(() => {
              if (loadGen.current !== gen) return
              applyEvents(result, forceError)
            })
          }
        })
        .catch((err: unknown) => {
          if (loadGen.current !== gen) return
          if (attempt < 1) {
            window.setTimeout(() => {
              if (loadGen.current !== gen) return
              loadSections(sections, { attempt: attempt + 1, revealChain })
            }, 800)
            return
          }
          const message = errorMessage(err)
          if (wanted.has('users') && incident) setUsers({ status: 'error', message })
          if (wanted.has('hosts') && incident) setHosts({ status: 'error', message })
          if (wanted.has('events')) setEvents({ status: 'error', message })
        })
    },
    [key, applyUsers, applyHosts, applyEvents, revealAfterUsers],
  )

  useEffect(() => {
    if (!key) {
      loadGen.current += 1
      setUsers({ status: 'idle' })
      setHosts({ status: 'idle' })
      setEvents({ status: 'idle' })
      setAllEvents([])
      setAllHosts([])
      return
    }
    setVisibleEventCount(SUBEVENT_PAGE)
    setVisibleHostCount(SUBEVENT_PAGE)
    setAllEvents([])
    setAllHosts([])
    const incident = key.record_type === 'siem_incident'
    setUsers(incident ? { status: 'loading' } : { status: 'idle' })
    setHosts({ status: 'idle' })
    setEvents(incident ? { status: 'idle' } : { status: 'loading' })
    if (incident) loadSections(['users'], { revealChain: true })
    else loadSections(['events'])
    return () => {
      loadGen.current += 1
    }
  }, [key, loadSections])

  const retryUsers = useCallback(() => {
    loadSections(['users'], { revealChain: true })
  }, [loadSections])

  const retryHosts = useCallback(() => {
    loadSections(['hosts'])
  }, [loadSections])

  const retryEvents = useCallback(() => {
    loadSections(['events'])
  }, [loadSections])

  if (!shouldLoad || !key) return null

  const eventsLabel = key.record_type === 'siem_incident' ? 'События' : 'Субевенты'
  const emptyEventsLabel = key.record_type === 'siem_incident' ? 'Нет событий' : 'Нет субевентов'
  const showIncidentExtras = key.record_type === 'siem_incident'
  const usersReady = users.status === 'ready' || users.status === 'error'
  const hostsReady = hosts.status === 'ready' || hosts.status === 'error'
  const showHosts = showIncidentExtras && usersReady
  const showEvents = !showIncidentExtras || hostsReady
  const visibleEvents = allEvents.slice(0, visibleEventCount)
  const visibleHosts = allHosts.slice(0, visibleHostCount)
  const moreHosts = visibleHostCount < allHosts.length
  const moreEvents = visibleEventCount < allEvents.length

  return (
    <div className="space-y-3">
      {showIncidentExtras ? (
        <div>
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <div className="text-[10px] uppercase tracking-wider text-fg-dim">
              Вовлеченные пользователи
            </div>
            <div className="flex items-center gap-1.5">
              {users.status === 'ready' && users.names.length > 0 ? (
                <div className="text-[10px] tabular-nums text-fg-dim">{users.names.length}</div>
              ) : null}
              <SectionRetryButton
                onClick={retryUsers}
                busy={users.status === 'loading'}
              />
            </div>
          </div>
          {users.status === 'loading' || users.status === 'idle' ? (
            <div className="text-xs text-fg-dim">Загрузка…</div>
          ) : null}
          {users.status === 'error' ? (
            <div className="text-xs text-fg-muted">{users.message}</div>
          ) : null}
          {users.status === 'ready' && users.names.length === 0 ? (
            <div className="text-xs text-fg-dim">Нет пользователей</div>
          ) : null}
          {users.status === 'ready' && users.names.length > 0 ? (
            <div className="flex flex-wrap gap-1">
              {users.names.map((name) => (
                <button
                  key={name}
                  type="button"
                  className="rounded border border-border px-1.5 py-0.5 text-xs text-fg hover:bg-surface-2"
                  onClick={() => onValueClick('account', name)}
                >
                  {name}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
      {showHosts ? (
        <div>
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <div className="text-[10px] uppercase tracking-wider text-fg-dim">
              Вовлеченные узлы
            </div>
            <div className="flex items-center gap-1.5">
              {hosts.status === 'ready' ? (
                <div className="text-[10px] tabular-nums text-fg-dim">{hosts.total}</div>
              ) : null}
              <SectionRetryButton
                onClick={retryHosts}
                busy={hosts.status === 'loading'}
              />
            </div>
          </div>
          {hosts.status === 'loading' || hosts.status === 'idle' ? (
            <div className="text-xs text-fg-dim">Загрузка…</div>
          ) : null}
          {hosts.status === 'error' ? (
            <div className="text-xs text-fg-muted">{hosts.message}</div>
          ) : null}
          {hosts.status === 'ready' && hosts.total === 0 ? (
            <div className="text-xs text-fg-dim">Нет узлов</div>
          ) : null}
          {hosts.status === 'ready' && hosts.total > 0 ? (
            <div
              className="space-y-1 overflow-y-auto overscroll-contain pr-0.5"
              style={{ maxHeight: SUBEVENT_LIST_MAX_PX }}
            >
              {visibleHosts.map((item) => (
                <button
                  key={item.value}
                  type="button"
                  className="flex w-full items-start justify-between gap-2 rounded border border-border px-2 py-1.5 text-left text-xs hover:bg-surface-2"
                  onClick={() => onValueClick('host', item.value)}
                >
                  <span className="min-w-0 truncate font-mono text-fg">{item.value}</span>
                  {item.roles.length > 0 ? (
                    <span className="shrink-0 text-[11px] uppercase text-fg-dim">
                      {item.roles.join(' · ')}
                    </span>
                  ) : null}
                </button>
              ))}
              {moreHosts ? (
                <button
                  type="button"
                  className="w-full py-1 text-center text-[11px] text-fg-muted hover:text-fg"
                  onClick={() =>
                    setVisibleHostCount((n) => Math.min(n + SUBEVENT_PAGE, allHosts.length))
                  }
                >
                  Загрузить ещё…
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
      {showEvents ? (
        <div>
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <div className="text-[10px] uppercase tracking-wider text-fg-dim">{eventsLabel}</div>
            <div className="flex items-center gap-1.5">
              {events.status === 'ready' ? (
                <div className="text-[10px] tabular-nums text-fg-dim">{events.total}</div>
              ) : null}
              <SectionRetryButton
                onClick={retryEvents}
                busy={events.status === 'loading'}
              />
            </div>
          </div>
          {events.status === 'loading' || events.status === 'idle' ? (
            <div className="text-xs text-fg-dim">Загрузка…</div>
          ) : null}
          {events.status === 'error' ? (
            <div className="text-xs text-fg-muted">{events.message}</div>
          ) : null}
          {events.status === 'ready' && events.total === 0 ? (
            <div className="text-xs text-fg-dim">{emptyEventsLabel}</div>
          ) : null}
          {events.status === 'ready' && events.total > 0 ? (
            <div
              className="space-y-1 overflow-y-auto overscroll-contain pr-0.5"
              style={{ maxHeight: SUBEVENT_LIST_MAX_PX }}
            >
              {visibleEvents.map((item) => (
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
              {moreEvents ? (
                <button
                  type="button"
                  className="w-full py-1 text-center text-[11px] text-fg-muted hover:text-fg"
                  onClick={() =>
                    setVisibleEventCount((n) => Math.min(n + SUBEVENT_PAGE, allEvents.length))
                  }
                >
                  Загрузить ещё…
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
