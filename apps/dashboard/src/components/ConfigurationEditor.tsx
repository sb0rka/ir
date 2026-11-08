import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { Eye, EyeOff, RefreshCw } from 'lucide-react'
import { readConfiguration, saveConfiguration, type ConfigurationMetadata } from '../api/configuration'
import { getAuthEpoch } from '../api/auth'
import { parseConfiguration } from '../lib/configuration'
import { listProjectSources, type ProjectSource } from '../api/integrations'
import { clsx } from '../lib/utils'
import { Button } from './ui'
import { SourceStatusPanel } from './SourceStatusPanel'
import { AgentAccessPanel } from './AgentAccessPanel'

export function ConfigurationEditor({ projectId, canWrite, canProbe }: { projectId: string; canWrite: boolean; canProbe: boolean }) {
  const [text, setText] = useState('')
  const [metadata, setMetadata] = useState<ConfigurationMetadata | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const [reload, setReload] = useState(0)
  const [sourceNames, setSourceNames] = useState<string[]>([])
  const [sources, setSources] = useState<ProjectSource[]>([])
  const [sourcesLoading, setSourcesLoading] = useState(false)
  const [sourcesError, setSourcesError] = useState<string | null>(null)
  const [sourceReload, setSourceReload] = useState(0)
  const [revealed, setRevealed] = useState(false)
  const lifecycle = useRef({ generation: 0, controller: null as AbortController | null }).current

  useEffect(() => {
    const current = ++lifecycle.generation
    const epoch = getAuthEpoch()
    const request = new AbortController()
    lifecycle.controller = request
    setText(''); setMetadata(null); setError(null); setSaved(false); setBusy(true)
    setSourceNames([])
    void readConfiguration(projectId, request.signal).then((document) => {
      if (current !== lifecycle.generation || epoch !== getAuthEpoch()) return
      setText(document.value); setMetadata(document)
      setSourceNames(Object.keys(parseConfiguration(document.value)))
    }).catch((reason: unknown) => {
      if (!request.signal.aborted && current === lifecycle.generation && epoch === getAuthEpoch()) setError(reason instanceof Error ? reason.message : 'Не удалось прочитать конфигурацию')
    }).finally(() => {
      if (current === lifecycle.generation && epoch === getAuthEpoch()) setBusy(false)
    })
    return () => { lifecycle.generation++; request.abort(); lifecycle.controller?.abort() }
  }, [projectId, reload, lifecycle])

  useEffect(() => {
    const epoch = getAuthEpoch()
    const request = new AbortController()
    setSources([]); setSourcesError(null); setSourcesLoading(false)
    if (!sourceNames.length) return
    if (!canProbe) { setSourcesError('Разрешите Gateway доступ к выбранному проекту для проверки источников.'); return }
    setSourcesLoading(true)
    void listProjectSources(projectId, true, request.signal).then((items) => {
      if (!request.signal.aborted && epoch === getAuthEpoch()) setSources(items)
    }).catch((reason: unknown) => {
      if (!request.signal.aborted && epoch === getAuthEpoch()) setSourcesError(reason instanceof Error ? reason.message : 'Не удалось проверить источники')
    }).finally(() => {
      if (!request.signal.aborted && epoch === getAuthEpoch()) setSourcesLoading(false)
    })
    return () => request.abort()
  }, [projectId, canProbe, sourceNames, sourceReload])

  const save = async () => {
    if (!metadata || busy || !canWrite) return
    setError(null); setSaved(false)
    try { parseConfiguration(text) }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Некорректный JSON'); return }
    const current = lifecycle.generation
    const epoch = getAuthEpoch()
    const request = new AbortController()
    lifecycle.controller = request
    setBusy(true)
    try {
      const result = await saveConfiguration(projectId, text, request.signal)
      if (current === lifecycle.generation && epoch === getAuthEpoch()) {
        setMetadata(result); setSaved(true)
        setSourceNames(Object.keys(parseConfiguration(text)))
      }
    } catch (reason) {
      if (!request.signal.aborted && current === lifecycle.generation && epoch === getAuthEpoch()) {
        setError(reason instanceof Error ? reason.message : 'Не удалось сохранить конфигурацию')
      }
    } finally {
      if (current === lifecycle.generation && epoch === getAuthEpoch()) setBusy(false)
    }
  }

  return <div className="space-y-3">
    <section className="space-y-2 rounded border border-border p-2">
      <div className="flex items-center justify-between gap-3">
        <label htmlFor="ir-configuration" className="text-xs font-semibold uppercase tracking-wider text-fg-muted">JSON-конфигурация проекта</label>
        <div className="flex items-center gap-1">
          <Button size="icon" variant="ghost" title={revealed ? 'Скрыть' : 'Показать'} aria-label={revealed ? 'Скрыть JSON-конфигурацию' : 'Показать JSON-конфигурацию'} aria-pressed={revealed} onClick={() => setRevealed((value) => !value)}>
            {revealed ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </Button>
          {metadata && <Button size="icon" variant="ghost" title="Перечитать" aria-label="Перечитать" disabled={busy} onClick={() => setReload((value) => value + 1)}>
            <RefreshCw className={clsx('h-3.5 w-3.5', busy && 'animate-spin')} />
          </Button>}
        </div>
      </div>
      <p className="text-xs text-fg-dim">Массив подключаемых инструментов с учетными данными</p>
      {metadata && <p className="text-[11px] text-fg-dim">{metadata.configured ? `Версия ${metadata.version}${metadata.updated_at ? ` · ${new Date(metadata.updated_at).toLocaleString()}` : ''}` : 'Конфигурация еще не сохранена'}</p>}
      <textarea id="ir-configuration" aria-label="JSON-конфигурация проекта" value={text} onChange={(event) => { setText(event.target.value); setSaved(false) }} disabled={busy || !metadata} readOnly={!canWrite} rows={6} spellCheck={false} autoComplete="off" className="w-full resize-y rounded border border-border bg-surface-0 px-2 py-1.5 font-mono text-xs text-fg outline-none focus:border-fg/40" style={revealed ? undefined : ({ WebkitTextSecurity: 'disc' } as CSSProperties)} />
      {error && <p role="alert" className="text-[11px] text-critical">{error}</p>}
      {saved && <p role="status" className="text-xs text-confirmed">Сохранено · версия {metadata?.version}</p>}
      <div className="flex justify-end">
        <Button size="sm" disabled={busy || !metadata || !canWrite} onClick={() => { void save() }}>{busy ? 'Загрузка…' : 'Сохранить JSON'}</Button>
      </div>
    </section>
    {metadata && <SourceStatusPanel names={sourceNames} sources={sources} loading={sourcesLoading} error={sourcesError} canRefresh={canProbe && !busy} onRefresh={() => setSourceReload((value) => value + 1)} />}
    <AgentAccessPanel projectId={projectId} canWrite={canWrite} />
  </div>
}
