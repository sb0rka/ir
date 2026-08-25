import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { clsx } from '../../lib/utils'
import {
  eventHeaderMeta,
  groupEventFields,
  type FieldColumn,
  type FieldGroup,
  type FieldRow,
} from '../../lib/pdql'
import type { Severity } from '../../types'
import { SeverityBadge } from '../ui'

export interface EventCardModel {
  id: string
  time: string
  title: string
  description?: string
  source: string
  severity?: Severity
  raw: Record<string, string>
}

export function EventFields({
  source,
  raw,
  onValueClick,
}: {
  source: string
  raw: Record<string, string>
  onValueClick: (field: string, value: string) => void
}) {
  const groups = groupEventFields(source, raw)
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set())
  if (groups.length === 0) return null

  return (
    <div className="space-y-2">
      {groups.map((group) => (
        <FieldGroupBlock
          key={group.id}
          group={group}
          collapsed={collapsed.has(group.id)}
          onToggle={() => {
            setCollapsed((current) => {
              const next = new Set(current)
              if (next.has(group.id)) next.delete(group.id)
              else next.add(group.id)
              return next
            })
          }}
          onValueClick={onValueClick}
        />
      ))}
    </div>
  )
}

export function EventHeaderFacts({
  source,
  raw,
  severity,
  onValueClick,
}: {
  source: string
  raw: Record<string, string>
  severity?: Severity
  onValueClick: (field: string, value: string) => void
}) {
  const meta = eventHeaderMeta(raw, source)
  return (
    <div className="space-y-2">
      {severity && <SeverityBadge severity={severity} />}
      <div className="flex flex-wrap gap-x-5 gap-y-1 text-[11px]">
        <MetaFact label="Источник" rows={meta.source} onValueClick={onValueClick} />
        <MetaFact label="Идентификатор" rows={meta.identifier} onValueClick={onValueClick} />
        <MetaFact label="Категория" rows={meta.category} joiner=" / " onValueClick={onValueClick} />
      </div>
    </div>
  )
}

function MetaFact({
  label,
  rows,
  joiner = ' ',
  onValueClick,
}: {
  label: string
  rows: FieldRow[]
  joiner?: string
  onValueClick: (field: string, value: string) => void
}) {
  if (rows.length === 0) return null
  return (
    <div>
      <div className="text-[11px] text-fg-dim">{label}</div>
      <div className="flex flex-wrap items-baseline">
        {rows.map((row, index) => (
          <span key={row.field} className="flex items-baseline">
            {index > 0 && <span className="text-fg-dim">{joiner}</span>}
            <ValueButton row={row} onValueClick={onValueClick} />
          </span>
        ))}
      </div>
    </div>
  )
}

function FieldGroupBlock({
  group,
  collapsed,
  onToggle,
  onValueClick,
}: {
  group: FieldGroup
  collapsed: boolean
  onToggle: () => void
  onValueClick: (field: string, value: string) => void
}) {
  const twoCol = group.columns.length > 1
  return (
    <div>
      <button
        type="button"
        className="flex w-full items-center gap-1 py-0.5 text-left text-xs font-semibold text-fg"
        onClick={onToggle}
      >
        {collapsed ? (
          <ChevronRight className="h-3.5 w-3.5 text-fg-dim" />
        ) : (
          <ChevronDown className="h-3.5 w-3.5 text-fg-dim" />
        )}
        {group.title}
      </button>
      {!collapsed && (
        <div className={clsx('mt-1 pl-1', twoCol ? 'grid grid-cols-2 gap-3' : 'space-y-2')}>
          {group.columns.map((column) => (
            <FieldColumnBlock
              key={column.title || group.id}
              column={column}
              onValueClick={onValueClick}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function FieldColumnBlock({
  column,
  onValueClick,
}: {
  column: FieldColumn
  onValueClick: (field: string, value: string) => void
}) {
  return (
    <div>
      {column.title && (
        <div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-fg-dim">
          {column.title}
        </div>
      )}
      <dl>
        {column.rows.map((row) => (
          <FieldValueRow key={row.field} row={row} onValueClick={onValueClick} />
        ))}
      </dl>
    </div>
  )
}

function FieldValueRow({
  row,
  onValueClick,
}: {
  row: FieldRow
  onValueClick: (field: string, value: string) => void
}) {
  return (
    <div className="flex items-baseline justify-between gap-2 text-xs">
      <dt className="min-w-0 shrink truncate text-fg-dim" title={row.field}>
        {row.field}
      </dt>
      <dd className="min-w-0 text-right">
        {row.value ? (
          <ValueButton row={row} onValueClick={onValueClick} />
        ) : (
          <span className="text-fg-dim">&nbsp;</span>
        )}
      </dd>
    </div>
  )
}

function ValueButton({
  row,
  onValueClick,
}: {
  row: FieldRow
  onValueClick: (field: string, value: string) => void
}) {
  const display = formatFieldValue(row.field, row.value)
  return (
    <button
      type="button"
      className="max-w-full truncate text-right font-mono text-low hover:underline"
      title={row.value}
      onClick={() => onValueClick(row.field, row.value)}
    >
      {display}
    </button>
  )
}

function formatFieldValue(field: string, value: string): string {
  if (!value) return value
  if (
    field === 'time' ||
    field === 'original_time' ||
    field.endsWith('_time') ||
    field.endsWith('.time')
  ) {
    const parsed = Date.parse(value)
    if (!Number.isNaN(parsed)) {
      return new Date(parsed).toLocaleString('ru-RU', {
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      })
    }
  }
  return value
}
