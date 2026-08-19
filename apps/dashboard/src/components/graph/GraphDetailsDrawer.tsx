import { useEffect, useMemo } from 'react'
import { X } from 'lucide-react'
import { useWorkspaceStore } from '../../state/useWorkspaceStore'
import { SEVERITY_COLOR } from './constants'
import { formatShortDate } from './time'

export function GraphDetailsDrawer() {
  const {
    selection,
    select,
    activeInvestigation,
    expandRelated,
    collapseRelated,
    canExpand,
    isExpanded,
  } = useWorkspaceStore()

  useEffect(() => {
    if (!selection) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') select(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selection, select])

  const content = useMemo(() => {
    if (!selection || !activeInvestigation) return null
    const { entities, alerts, events, edges } = activeInvestigation

    if (selection.kind === 'entity') {
      const entity = entities.find(
        (e) => e.id === selection.id || e.entity_id === selection.id,
      )
      if (!entity) return null
      const relatedEvents = events.filter((ev) =>
        ev.entity_ids.includes(entity.id),
      )
      const relatedEdges = edges.filter(
        (e) => e.source_id === entity.id || e.target_id === entity.id,
      )
      return {
        title: entity.display_name,
        badge: entity.type_code,
        rows: [
          ['Key', entity.key],
          ...(entity.first_seen
            ? ([['First seen', formatShortDate(entity.first_seen)]] as [string, string][])
            : []),
          ...(entity.last_seen
            ? ([['Last seen', formatShortDate(entity.last_seen)]] as [string, string][])
            : []),
          ...Object.entries(entity.metadata ?? {}).map(
            ([k, v]) => [k, v] as [string, string],
          ),
        ],
        events: relatedEvents,
        edges: relatedEdges,
        entityId: entity.entity_id ?? entity.id,
      }
    }

    if (selection.kind === 'alert') {
      const alert = alerts.find(
        (a) =>
          a.id === selection.id ||
          a.event_id === selection.id ||
          selection.id === `alert-${a.event_id}`,
      )
      if (!alert) return null
      const relatedEvents = events.filter((ev) => ev.alert_id === alert.id)
      const relatedEdges = edges.filter(
        (e) => e.source_id === alert.id || e.target_id === alert.id,
      )
      return {
        title: alert.title,
        badge: alert.severity,
        badgeColor: SEVERITY_COLOR[alert.severity],
        rows: [
          ['Source', alert.source],
          ['Time', formatShortDate(alert.event_ts)],
          ['Description', alert.description],
        ],
        events: relatedEvents,
        edges: relatedEdges,
      }
    }

    if (selection.kind === 'event') {
      const event = events.find((e) => e.id === selection.id)
      if (!event) return null
      return {
        title: event.title,
        badge: event.event_class,
        badgeColor: event.severity
          ? SEVERITY_COLOR[event.severity]
          : undefined,
        rows: [
          ['Source', String(event.source)],
          ['Event ID', event.source_event_id],
          ['Time', formatShortDate(event.event_ts)],
          ['Class', event.event_class],
          ...(event.severity
            ? ([['Severity', event.severity]] as [string, string][])
            : []),
          ['Entities', event.entity_ids.length.toString()],
        ],
        events: [] as typeof events,
        edges: edges.filter(
          (e) =>
            event.entity_ids.includes(e.source_id) ||
            event.entity_ids.includes(e.target_id) ||
            e.source_id === event.alert_id ||
            e.target_id === event.alert_id,
        ),
        linkedEntityIds: event.entity_ids,
      }
    }

    return null
  }, [selection, activeInvestigation])

  if (!selection || !content || !activeInvestigation) return null

  return (
    <>
      <button
        type="button"
        aria-label="Close details"
        className="absolute inset-0 z-20 bg-black/30"
        onClick={() => select(null)}
      />
      <aside className="absolute bottom-0 right-0 top-0 z-30 flex w-[340px] flex-col border-l border-[var(--border)] bg-[var(--bg-panel)] shadow-xl">
        <div className="flex items-start justify-between gap-2 border-b border-[var(--border)] px-4 py-3">
          <div className="min-w-0">
            <div
              className="mb-1 inline-block rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
              style={{
                color: content.badgeColor ?? 'var(--accent)',
                background: content.badgeColor
                  ? `color-mix(in srgb, ${content.badgeColor} 18%, transparent)`
                  : 'var(--accent-soft)',
              }}
            >
              {content.badge}
            </div>
            <h2 className="text-sm font-semibold leading-snug text-[var(--text)]">
              {content.title}
            </h2>
          </div>
          <button
            type="button"
            onClick={() => select(null)}
            className="rounded-md p-1 text-[var(--text-dim)] hover:bg-[var(--bg-node)] hover:text-[var(--text)]"
          >
            <X size={16} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-4 py-3">
          <dl className="space-y-2">
            {content.rows.map(([k, v]) => (
              <div key={k}>
                <dt className="text-[10px] uppercase tracking-wide text-[var(--text-dim)]">
                  {k}
                </dt>
                <dd className="break-all text-xs text-[var(--text)]">{v}</dd>
              </div>
            ))}
          </dl>

          {'entityId' in content && content.entityId ? (
            <div className="mt-4 flex gap-2">
              {canExpand(content.entityId) ? (
                <button
                  type="button"
                  onClick={() => expandRelated(content.entityId!)}
                  className="rounded-md border border-[var(--border-strong)] bg-[var(--bg-node)] px-2 py-1 text-[11px] text-[var(--accent)] hover:border-[var(--accent)]"
                >
                  Expand related
                </button>
              ) : null}
              {isExpanded(content.entityId) ? (
                <button
                  type="button"
                  onClick={() => collapseRelated(content.entityId!)}
                  className="rounded-md border border-[var(--border)] px-2 py-1 text-[11px] text-[var(--text-muted)] hover:text-[var(--text)]"
                >
                  Collapse related
                </button>
              ) : null}
            </div>
          ) : null}

          {content.edges.length > 0 ? (
            <section className="mt-5">
              <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--text-dim)]">
                Edges ({content.edges.length})
              </h3>
              <ul className="space-y-1.5">
                {content.edges.map((e) => (
                  <li
                    key={e.id}
                    className="rounded-md border border-[var(--border)] bg-[var(--bg)] px-2 py-1.5 text-[11px]"
                  >
                    <div className="text-[var(--text)]">
                      {e.kind}{' '}
                      <span className="text-[var(--text-dim)]">
                        ({e.origin}, {e.status})
                      </span>
                    </div>
                    <div className="truncate text-[var(--text-dim)]">
                      {e.source_id} → {e.target_id}
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          {content.events.length > 0 ? (
            <section className="mt-5">
              <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--text-dim)]">
                Related events ({content.events.length})
              </h3>
              <ul className="space-y-1.5">
                {content.events.map((ev) => (
                  <li key={ev.id}>
                    <button
                      type="button"
                      onClick={() => select({ kind: 'event', id: ev.id })}
                      className="w-full rounded-md border border-[var(--border)] bg-[var(--bg)] px-2 py-1.5 text-left text-[11px] hover:border-[var(--border-strong)]"
                    >
                      <div className="text-[var(--text-dim)]">
                        {formatShortDate(ev.event_ts)}
                      </div>
                      <div className="text-[var(--text)]">{ev.title}</div>
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}

          {'linkedEntityIds' in content &&
          content.linkedEntityIds &&
          content.linkedEntityIds.length > 0 ? (
            <section className="mt-5">
              <h3 className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--text-dim)]">
                Linked entities
              </h3>
              <ul className="space-y-1">
                {content.linkedEntityIds.map((id) => {
                  const ent = activeInvestigation.entities.find(
                    (e) => e.id === id,
                  )
                  return (
                    <li key={id}>
                      <button
                        type="button"
                        onClick={() => select({ kind: 'entity', id })}
                        className="w-full rounded-md border border-[var(--border)] px-2 py-1 text-left text-[11px] text-[var(--accent)] hover:bg-[var(--bg-node)]"
                      >
                        {ent?.display_name ?? id}
                      </button>
                    </li>
                  )
                })}
              </ul>
            </section>
          ) : null}
        </div>
      </aside>
    </>
  )
}
