import { LoaderCircle, RefreshCw } from 'lucide-react'
import type { ProjectSource } from '../api/integrations'
import { clsx } from '../lib/utils'
import { Button } from './ui'

export function SourceStatusPanel({ names, sources, loading, error, canRefresh, onRefresh }: {
  names: string[]
  sources: ProjectSource[]
  loading: boolean
  error: string | null
  canRefresh: boolean
  onRefresh: () => void
}) {
  return <section className="space-y-2 rounded border border-border p-2">
    <div className="flex items-center justify-between gap-3">
      <h2 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">Статус источников</h2>
      <Button size="icon" variant="ghost" title="Проверить" aria-label="Проверить источники" disabled={loading || !canRefresh || !names.length} onClick={onRefresh}>
        <RefreshCw className={clsx('h-3.5 w-3.5', loading && 'animate-spin')} />
      </Button>
    </div>
    <div className="space-y-2 rounded border border-border bg-surface-0 px-2 py-1.5">
    {!names.length && <p className="text-xs text-fg-dim">Нет настроенных источников</p>}
    {names.map((name) => {
      const source = sources.find((item) => item.code === name)
      const label = error ? 'статус не проверен'
        : !source ? 'не подключён'
        : source.status === 'offline' ? 'недоступен'
        : source.status === 'degraded' ? 'частично доступен' : 'онлайн'
      const statusClass = error || source?.status === 'degraded' ? 'text-medium'
        : source?.status === 'online' ? 'text-confirmed' : 'text-critical'
      return <div key={name}>
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-fg">{source?.name ?? name}</span>
          {loading ? <LoaderCircle aria-label="Проверка источника" className="h-3.5 w-3.5 animate-spin text-fg-dim" />
            : <span className={clsx('shrink-0 text-[11px] font-medium', statusClass)}>{label}</span>}
        </div>
        {source && source.name !== name && <p className="text-[11px] text-fg-dim">{name}</p>}
        {!loading && !error && !source && <p className="text-[11px] text-critical">Проверьте имя источника, поля cookie в JSON и настройку backend в Gateway.</p>}
      </div>
    })}
    {error && <p role="alert" className="text-[11px] text-critical">{error}</p>}
    </div>
  </section>
}
