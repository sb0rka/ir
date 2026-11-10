import { useEffect, useRef, useState } from 'react'
import { Copy, RefreshCw, X } from 'lucide-react'
import { createAgentAccess, readAgentAccess, rotateAgentAccess, type AgentAccessMetadata } from '../api/agentAccess'
import { AuthError, getAuthEpoch, subscribeAuth } from '../api/auth'
import { Button } from './ui'

export function AgentAccessPanel({ projectId, canWrite }: { projectId: string; canWrite: boolean }) {
  return <ProjectAgentAccess key={projectId} projectId={projectId} canWrite={canWrite} />
}

function ProjectAgentAccess({ projectId, canWrite }: { projectId: string; canWrite: boolean }) {
  const [metadata, setMetadata] = useState<AgentAccessMetadata | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [reload, setReload] = useState(0)
  const lifecycle = useRef({ generation: 0, epoch: getAuthEpoch(), controller: null as AbortController | null, pending: false }).current

  useEffect(() => {
    return subscribeAuth(() => {
      if (lifecycle.epoch === getAuthEpoch()) return
      lifecycle.epoch = getAuthEpoch()
      lifecycle.generation++
      lifecycle.controller?.abort()
      lifecycle.pending = false
      setToken(null); setCopied(false); setMetadata(null); setBusy(false); setError(null)
    })
  }, [lifecycle])

  useEffect(() => {
    const current = ++lifecycle.generation
    const epoch = getAuthEpoch()
    const request = new AbortController()
    lifecycle.controller = request
    lifecycle.pending = true
    setBusy(true); setError(null)
    void readAgentAccess(projectId, request.signal).then((result) => {
      if (current === lifecycle.generation && epoch === getAuthEpoch()) setMetadata(result)
    }).catch((reason: unknown) => {
      if (!request.signal.aborted && current === lifecycle.generation && epoch === getAuthEpoch()) setError(reason instanceof Error ? reason.message : 'Не удалось прочитать доступ агента')
    }).finally(() => {
      if (current === lifecycle.generation && epoch === getAuthEpoch()) { lifecycle.pending = false; setBusy(false) }
    })
    return () => { lifecycle.generation++; request.abort(); lifecycle.controller?.abort() }
  }, [projectId, reload, lifecycle])

  const issue = async () => {
    if (!metadata || !canWrite || lifecycle.pending || (metadata.configured && !metadata.version)) return
    const current = ++lifecycle.generation
    const epoch = getAuthEpoch()
    const request = new AbortController()
    lifecycle.controller = request
    lifecycle.pending = true
    setBusy(true); setError(null); setToken(null); setCopied(false)
    const isCurrent = () => !request.signal.aborted && current === lifecycle.generation && epoch === getAuthEpoch()
    try {
      const { token: issued, ...result } = metadata.configured
        ? await rotateAgentAccess(projectId, metadata.version!, request.signal)
        : await createAgentAccess(projectId, request.signal)
      if (isCurrent()) { setMetadata(result); setToken(issued) }
    } catch (reason) {
      if (isCurrent()) {
        setError(reason instanceof Error ? reason.message : 'Не удалось изменить ключ доступа')
        if (reason instanceof AuthError && reason.status === 409) {
          try {
            const result = await readAgentAccess(projectId, request.signal)
            if (isCurrent()) setMetadata(result)
          } catch { /* Preserve the rotation conflict while allowing a manual reload. */ }
        }
      }
    } finally {
      if (isCurrent()) { lifecycle.pending = false; setBusy(false) }
    }
  }

  const copy = async () => {
    if (!token) return
    const current = lifecycle.generation
    const epoch = getAuthEpoch()
    try {
      await navigator.clipboard.writeText(token)
      if (current === lifecycle.generation && epoch === getAuthEpoch()) setCopied(true)
    } catch {
      if (current === lifecycle.generation && epoch === getAuthEpoch()) setError('Не удалось скопировать ключ. Скопируйте его вручную.')
    }
  }

  return <section aria-label="Доступ агента к IR" className="space-y-2 rounded border border-border p-2">
    <div className="flex items-center justify-between gap-3">
      <h2 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">Доступ агента к IR</h2>
      <Button variant="ghost" size="icon" title="Перечитать" aria-label="Перечитать доступ агента" disabled={busy} onClick={() => setReload((value) => value + 1)}>
        <RefreshCw className="h-3.5 w-3.5" />
      </Button>
    </div>
    {metadata && <p className="text-[11px] text-fg-dim">{metadata.configured
      ? `${metadata.prefix ?? '••••'}…${metadata.postfix ?? ''} · версия ${metadata.version}${metadata.updated_at ? ` · ${new Date(metadata.updated_at).toLocaleString()}` : ''}`
      : 'Ключ доступа еще не создан'}</p>}
    {token && <div className="space-y-2 rounded border border-border bg-surface-0 px-2 py-1.5">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-fg-muted">Скопируйте ключ в SB0RKA_IR_AGENT_TOKEN. После закрытия он доступен только в Sb0rka Secrets.</p>
        <Button variant="ghost" size="icon" title="Скрыть" aria-label="Скрыть ключ доступа" onClick={() => { setToken(null); setCopied(false) }}><X className="h-3.5 w-3.5" /></Button>
      </div>
      <input aria-label="Новый ключ доступа агента" className="w-full rounded border border-border bg-surface-0 px-2 py-1.5 font-mono text-xs text-fg outline-none" type="text" value={token} readOnly autoComplete="off" spellCheck={false} />
      <Button size="sm" onClick={() => { void copy() }}><Copy className="h-3.5 w-3.5" />{copied ? 'Скопировано' : 'Скопировать'}</Button>
    </div>}
    {error && <p role="alert" className="text-[11px] text-critical">{error}</p>}
    <div className="flex justify-end">
      <Button size="sm" disabled={busy || !metadata || !canWrite} onClick={() => { void issue() }}>{busy ? 'Загрузка…' : metadata?.configured ? 'Ротация ключа' : 'Сгенерировать ключ доступа агенту'}</Button>
    </div>
  </section>
}
