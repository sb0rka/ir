import { useEffect, useRef, useState } from 'react'
import { X } from 'lucide-react'
import {
  encodeAst,
  eventQueryTabs,
  fieldsForLanguage,
  readPdqlAst,
  readWazuhAst,
  type EventSourceMeta,
} from '../../lib/eventQuery'
import { effectiveQueueLimit, serialize } from '../../lib/pdql'
import type { AppliedEventLanguage, EventQuery } from '../../types'
import { resolve, type TimeInterval } from '../time-interval'
import { Button } from '../ui'
import { clsx } from '../../lib/utils'
import { PdqlBuilder } from './PdqlBuilder'
import { QueryBuilderProvider } from './queryBuilder'
import { useNativeEditor } from './useNativeEditor'

function LanguageTabs({
  tabs,
  value,
  onChange,
}: {
  tabs: { id: AppliedEventLanguage; label: string }[]
  value: AppliedEventLanguage
  onChange: (value: AppliedEventLanguage) => void
}) {
  return (
    <div className="flex gap-1 rounded border border-border bg-surface-0 p-0.5" role="tablist">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={value === tab.id}
          onClick={() => onChange(tab.id)}
          className={clsx(
            'rounded px-2 py-1 text-[11px]',
            value === tab.id ? 'bg-surface-3 text-fg' : 'text-fg-muted hover:text-fg',
          )}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}

function ValueList({
  field,
  values,
  onChange,
}: {
  field: 'host' | 'ip'
  values: string[]
  onChange: (values: string[]) => void
}) {
  return (
    <section className="rounded border border-border bg-surface-1 p-2">
      <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-fg-muted">
        {field === 'host' ? 'Хост' : 'IP'}
      </div>
      <div className="flex flex-col gap-1.5">
        {values.map((value, index) => (
          <div key={`${field}-${index}`} className="flex items-center gap-1.5">
            <span className="font-mono text-xs text-fg">{field}</span>
            <span className="font-mono text-[11px] text-fg-dim">=</span>
            <input
              value={value}
              onChange={(event) => {
                const next = values.slice()
                next[index] = event.target.value
                onChange(next)
              }}
              className="min-w-0 flex-1 rounded border border-border bg-surface-0 px-1.5 py-0.5 font-mono text-[11px] text-fg outline-none focus:border-fg/40"
            />
            <Button
              size="sm"
              variant="ghost"
              title="Удалить"
              onClick={() => onChange(values.filter((_, item) => item !== index))}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        ))}
      </div>
      <button
        type="button"
        className="mt-2 text-[11px] text-fg-muted hover:text-fg"
        onClick={() => onChange([...values, ''])}
      >
        Добавить
      </button>
    </section>
  )
}

function Coupling({ sources }: { sources: EventSourceMeta[] }) {
  return (
    <section className="rounded border border-border bg-surface-1 p-2">
      <div className="text-xs font-semibold uppercase tracking-wider text-fg-muted">Сопряжение</div>
      <p className="mt-1 text-[11px] text-fg-dim">
        Совпадение в любом подключённом инструменте. Несколько хостов и несколько IP объединяются как «или», хост и IP — как «и».
      </p>
      <div className="mt-2 flex flex-col">
        {sources.map((source) => {
          const mapping = source.common_mappings
          const entityWindow = source.bounded || mapping?.strategy === 'entities'
          return (
            <div key={source.source_code} className="border-t border-border py-2 first:border-t-0">
              <div className="text-xs text-fg">{source.source_name}</div>
              {entityWindow && (
                <div className="text-[11px] text-fg-dim">в пределах окна, по сущностям</div>
              )}
              <PathLine label="Хост" paths={mapping?.host ?? []} />
              <PathLine label="IP" paths={mapping?.ip ?? []} />
            </div>
          )
        })}
        {sources.length === 0 && <div className="text-[11px] text-fg-dim">Нет подключённых источников событий</div>}
      </div>
    </section>
  )
}

function PathLine({
  label,
  paths,
}: {
  label: string
  paths: { path: string; case_insensitive?: boolean }[]
}) {
  if (paths.length === 0) return null
  return (
    <div className="mt-1 font-mono text-[11px] text-fg-muted">
      <span className="text-fg-dim">{label}</span>{' '}
      {paths.map((path, index) => (
        <span key={path.path}>
          {index > 0 ? ', ' : ''}
          {path.path}
          {path.case_insensitive ? ' · без учёта регистра' : ''}
        </span>
      ))}
    </div>
  )
}

export function EventQueryModal({
  open,
  query,
  sources,
  timeInterval,
  onClose,
  onApply,
  onExecute,
}: {
  open: boolean
  query: EventQuery
  sources: EventSourceMeta[]
  timeInterval: TimeInterval
  onClose: () => void
  onApply: (query: EventQuery) => void
  onExecute: (query: EventQuery) => void
}) {
  const tabs = eventQueryTabs(sources)
  const [tab, setTab] = useState<AppliedEventLanguage>(query.appliedLanguage)
  const [hosts, setHosts] = useState(query.hosts)
  const [ips, setIps] = useState(query.ips)
  const pdql = useNativeEditor('pdql', fieldsForLanguage(sources, 'pt_pdql'), readPdqlAst(query.pdql).ast)
  const wazuh = useNativeEditor('dql', fieldsForLanguage(sources, 'wazuh_dql'), readWazuhAst(query.wazuh).ast)
  const pdqlRef = useRef(pdql)
  const wazuhRef = useRef(wazuh)
  pdqlRef.current = pdql
  wazuhRef.current = wazuh
  const wasOpen = useRef(false)

  useEffect(() => {
    if (!open) {
      wasOpen.current = false
      return
    }
    if (wasOpen.current) return
    wasOpen.current = true
    // The bar and the constructor read the same eventQuery. Reopen reloads it.
    const available = eventQueryTabs(sources)
    let visible = available.some((item) => item.id === query.appliedLanguage) ? query.appliedLanguage : 'common'
    if (
      visible === 'common' &&
      !available.some((item) => item.id === 'pt_pdql') &&
      available.some((item) => item.id === 'wazuh_dql')
    ) {
      visible = 'wazuh_dql'
    }
    setTab(visible)
    setHosts(query.hosts)
    setIps(query.ips)
    const pdqlAst = readPdqlAst(query.pdql).ast
    const wazuhAst = readWazuhAst(query.wazuh).ast
    pdqlRef.current.init(pdqlAst, resolve(timeInterval).from)
    wazuhRef.current.init(
      query.wazuh.trim() ? wazuhAst : { ...wazuhAst, limit: effectiveQueueLimit(pdqlAst) },
      resolve(timeInterval).from,
    )
  }, [open, query, sources, timeInterval])

  if (!open) return null

  const activeTab = tabs.some((item) => item.id === tab) ? tab : 'common'
  const toolbar = <LanguageTabs tabs={tabs} value={activeTab} onChange={setTab} />
  const commit = (): EventQuery | null => {
    if (tabs.some((item) => item.id === 'pt_pdql') && !pdql.applyPdql()) return null
    if (tabs.some((item) => item.id === 'wazuh_dql') && !wazuh.applyPdql()) return null
    return {
      hosts: hosts.map((item) => item.trim()).filter(Boolean),
      ips: ips.map((item) => item.trim()).filter(Boolean),
      pdql: serialize(pdql.snapshot()),
      wazuh: encodeAst(wazuh.snapshot()),
      appliedLanguage: activeTab,
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div
        role="dialog"
        aria-label="Конструктор запроса"
        className="relative flex h-[85vh] w-[95vw] max-w-7xl flex-col overflow-hidden rounded border border-border bg-surface-1 shadow-xl"
        onKeyDown={(event) => {
          if (event.key === 'Escape') onClose()
        }}
      >
        <div className="min-h-0 flex-1">
          {activeTab === 'common' ? (
            <div className="flex h-full min-h-0 flex-col">
              <div className="flex items-center justify-between gap-3 border-b border-border px-3 py-2">
                <div className="text-sm text-fg">Конструктор запроса</div>
                <div className="flex items-center gap-2">
                  {toolbar}
                  <Button size="icon" variant="ghost" title="Закрыть" aria-label="Закрыть" onClick={onClose}>
                    <X className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
              <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-auto p-3">
                <ValueList field="host" values={hosts} onChange={setHosts} />
                <ValueList field="ip" values={ips} onChange={setIps} />
                <Coupling sources={sources} />
              </div>
            </div>
          ) : (
            <QueryBuilderProvider value={activeTab === 'pt_pdql' ? pdql : wazuh}>
              <PdqlBuilder
                heading={activeTab === 'pt_pdql' ? 'Конструктор PDQL' : 'Конструктор Wazuh'}
                toolbar={toolbar}
                onClose={onClose}
              />
            </QueryBuilderProvider>
          )}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-border px-3 py-2">
          <Button size="sm" variant="ghost" onClick={onClose}>
            Отмена
          </Button>
          <Button
            size="sm"
            onClick={() => {
              const next = commit()
              if (next) onApply(next)
            }}
          >
            Применить
          </Button>
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              const next = commit()
              if (next) onExecute(next)
            }}
          >
            Выполнить
          </Button>
        </div>
      </div>
    </div>
  )
}
