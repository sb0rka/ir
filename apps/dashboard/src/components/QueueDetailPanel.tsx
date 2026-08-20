import { useEffect } from 'react'
import {
  ArrowUpRight,
  Play,
  Sparkles,
  UserPlus,
  X,
  XCircle,
} from 'lucide-react'
import { useAppStore } from '../store/appStore'
import type { AlertEvent, CorrelationGroup, Entity, FilterField } from '../types'
import { Button, Chip, Panel, SeverityBadge } from './ui'
import { formatTime, kindLabel, statusLabel } from '../lib/utils'
import { fieldForEntityKind } from '../lib/filters'

type AddChip = (field: FilterField, value: string) => void

export function QueueDetailPanel() {
  const item = useAppStore((s) => s.inspectedQueueItem)
  const inspect = useAppStore((s) => s.inspectQueueItem)
  const start = useAppStore((s) => s.startInvestigation)
  const addChip = useAppStore((s) => s.addChip)
  const alerts = useAppStore((s) => s.alerts)
  const correlations = useAppStore((s) => s.correlations)
  const entities = useAppStore((s) => s.entities)
  const loading = useAppStore((s) => s.investigationLoading)

  useEffect(() => {
    if (!item) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') inspect(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [item, inspect])

  if (!item) return null

  const alert = item.kind === 'alert' ? alerts[item.id] : undefined
  const group = item.kind === 'correlation' ? correlations[item.id] : undefined
  if (!alert && !group) return null

  return (
    <Panel
      title={alert ? 'Событие' : 'Корреляция'}
      className="w-[22rem] shrink-0"
      actions={
        <button type="button" onClick={() => inspect(null)} title="Закрыть">
          <X className="h-3.5 w-3.5 text-fg-dim" />
        </button>
      }
    >
      <div className="flex min-h-full flex-col">
        <div className="flex-1 space-y-4 p-3">
          {alert && <AlertDetails alert={alert} entities={entities} addChip={addChip} />}
          {group && (
            <CorrelationDetails
              group={group}
              alerts={alerts}
              entities={entities}
              addChip={addChip}
              onOpenAlert={(id) => inspect({ kind: 'alert', id })}
            />
          )}
        </div>

        <div className="sticky bottom-0 space-y-2 border-t border-border bg-surface-1 p-3">
          <Button
            size="md"
            variant="primary"
            className="w-full"
            disabled={loading}
            onClick={() => void start([item.id])}
          >
            <Play className="h-3.5 w-3.5" />
            Начать расследование
          </Button>
          <div className="grid grid-cols-2 gap-1.5">
            <DecoButton icon={<UserPlus className="h-3 w-3" />}>Назначить</DecoButton>
            <DecoButton icon={<Sparkles className="h-3 w-3" />}>Обогатить</DecoButton>
            <DecoButton icon={<ArrowUpRight className="h-3 w-3" />}>
              Эскалировать
            </DecoButton>
            <DecoButton icon={<XCircle className="h-3 w-3" />}>Закрыть</DecoButton>
          </div>
        </div>
      </div>
    </Panel>
  )
}

function DecoButton({
  children,
  icon,
}: {
  children: React.ReactNode
  icon: React.ReactNode
}) {
  return (
    <Button size="sm" variant="ghost" className="cursor-default justify-start" tabIndex={-1}>
      {icon}
      {children}
    </Button>
  )
}

function AlertDetails({
  alert,
  entities,
  addChip,
}: {
  alert: AlertEvent
  entities: Record<string, Entity>
  addChip: AddChip
}) {
  return (
    <>
      <div>
        <div className="flex items-center gap-2">
          <SeverityBadge severity={alert.severity} />
          <span className="text-xs text-fg-dim">{statusLabel[alert.status]}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-snug">{alert.title}</div>
        {alert.description && (
          <p className="mt-1.5 text-xs leading-relaxed text-fg-muted">{alert.description}</p>
        )}
      </div>

      <MetaList
        rows={[
          ['Время', formatTime(alert.time)],
          ['Правило', alert.rule],
          ['Источник', alert.source],
          ...(alert.sourceEventId ? ([['ID источника', alert.sourceEventId]] as const) : []),
        ]}
      />

      <EntityList entityIds={alert.entityIds} entities={entities} addChip={addChip} />

      {alert.raw && Object.keys(alert.raw).length > 0 && (
        <div>
          <div className="mb-1 text-[10px] uppercase tracking-wider text-fg-dim">Поля</div>
          <dl className="space-y-1">
            {Object.entries(alert.raw).map(([k, v]) => (
              <div key={k} className="flex justify-between gap-2 text-xs">
                <dt className="shrink-0 text-fg-dim">{k}</dt>
                <dd className="max-w-[180px] truncate text-right font-mono text-fg-muted" title={v}>
                  {v}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      )}
    </>
  )
}

function CorrelationDetails({
  group,
  alerts,
  entities,
  addChip,
  onOpenAlert,
}: {
  group: CorrelationGroup
  alerts: Record<string, AlertEvent>
  entities: Record<string, Entity>
  addChip: AddChip
  onOpenAlert: (id: string) => void
}) {
  const eventCount = group.eventIds.length
  const sourceCount = Object.keys(group.sourceCounts).length

  return (
    <>
      <div>
        <div className="flex items-center gap-2">
          <SeverityBadge severity={group.severity} />
          <span className="text-xs text-fg-dim">{statusLabel[group.status]}</span>
        </div>
        <div className="mt-2 text-sm font-medium leading-snug">{group.title}</div>
        {group.reason && (
          <p className="mt-1.5 text-xs leading-relaxed text-fg-muted">{group.reason}</p>
        )}
        <div className="mt-2 flex flex-wrap gap-1.5">
          <Chip>
            {eventCount} соб. / {sourceCount} ист.
          </Chip>
          <Chip>{formatTime(group.time)}</Chip>
        </div>
      </div>

      <div>
        <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">Источники</div>
        <div className="flex flex-wrap gap-1">
          {Object.entries(group.sourceCounts).map(([src, n]) => (
            <Chip key={src}>
              {src}:{n}
            </Chip>
          ))}
        </div>
      </div>

      <EntityList entityIds={group.entityIds} entities={entities} addChip={addChip} />

      <div>
        <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">События</div>
        <div className="space-y-1">
          {group.eventIds.map((eid) => {
            const a = alerts[eid]
            if (!a) return null
            return (
              <button
                key={eid}
                type="button"
                className="flex w-full items-start justify-between gap-2 rounded border border-border px-2 py-1.5 text-left text-xs hover:bg-surface-2"
                onClick={() => onOpenAlert(eid)}
              >
                <span className="min-w-0">
                  <span className="block truncate text-fg">{a.title}</span>
                  <span className="text-[11px] text-fg-dim">{a.source}</span>
                </span>
                <SeverityBadge severity={a.severity} />
              </button>
            )
          })}
        </div>
      </div>
    </>
  )
}

function MetaList({ rows }: { rows: ReadonlyArray<readonly [string, string]> }) {
  return (
    <div>
      <div className="mb-1 text-[10px] uppercase tracking-wider text-fg-dim">Мета</div>
      <dl className="space-y-1">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-2 text-xs">
            <dt className="shrink-0 text-fg-dim">{k}</dt>
            <dd className="max-w-[180px] truncate text-right font-mono text-fg-muted" title={v}>
              {v}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function EntityList({
  entityIds,
  entities,
  addChip,
}: {
  entityIds: string[]
  entities: Record<string, Entity>
  addChip: AddChip
}) {
  if (entityIds.length === 0) return null
  return (
    <div>
      <div className="mb-1.5 text-[10px] uppercase tracking-wider text-fg-dim">Сущности</div>
      <div className="space-y-1">
        {entityIds.map((id) => {
          const e = entities[id]
          if (!e) return null
          const field = fieldForEntityKind(e.kind)
          return (
            <button
              key={id}
              type="button"
              className="flex w-full items-center justify-between rounded border border-border px-2 py-1.5 text-left text-xs hover:bg-surface-2"
              title={field ? 'Найти связанные' : undefined}
              onClick={() => {
                if (field) addChip(field, e.label.replaceAll('[', '').replaceAll(']', ''))
              }}
            >
              <span className="text-fg-dim">{kindLabel[e.kind] ?? e.kind}</span>
              <span className="font-mono text-fg">{e.label}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
