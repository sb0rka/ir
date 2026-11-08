import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { getAuthNotice, clearAuthNotice, connectIntegration, disconnectIntegration, integrationStatus, type IntegrationKind, type IntegrationStatus } from '../api/auth'
import { setProjectId } from '../api/env'
import type { Project } from '../api/platform'
import { Button } from './ui'
import { ConfigurationEditor } from './ConfigurationEditor'

export function AuthConnections({ projects, currentProjectId, onClose }: { projects: Project[]; currentProjectId?: string; onClose: (projectId?: string) => void }) {
  const [status, setStatus] = useState<Partial<Record<IntegrationKind, IntegrationStatus>>>({})
  const [error, setError] = useState<string | null>(getAuthNotice())
  const [busy, setBusy] = useState(false)
  const [projectId, selectProject] = useState(currentProjectId ?? projects[0]?.id ?? '')
  const configurationAllowed = ['secrets.read', 'secrets.reveal'].every((scope) => status.platform?.scopes?.includes(scope))
  useEffect(() => { void Promise.all((['platform', 'gateway'] as const).map(async (kind) => {
    const value = await integrationStatus(kind)
    setStatus((previous) => ({ ...previous, [kind]: value }))
  })).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Не удалось загрузить интеграции')) }, [])
  const close = () => {
    if (busy) return
    clearAuthNotice()
    onClose(projectId || undefined)
  }
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) {
        clearAuthNotice()
        onClose(projectId || undefined)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [busy, onClose, projectId])
  const run = (action: () => Promise<void>) => {
    setBusy(true); setError(null)
    void action().catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Не удалось изменить доступ')).finally(() => setBusy(false))
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/60" onClick={close} />
      <section
        role="dialog"
        aria-label="Подключения IR"
        className="relative flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded border border-border bg-surface-1 text-fg shadow-xl"
      >
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border px-3 py-2">
          <div className="text-sm text-fg">Настройка интеграций</div>
          <Button size="icon" variant="ghost" title="Закрыть" aria-label="Закрыть" disabled={busy} onClick={close}>
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
          {error && <p role="alert" className="text-xs text-critical">{error}</p>}
          {(['platform', 'gateway'] as const).map((kind) => (
            <section key={kind} className="space-y-2 rounded border border-border p-2">
              <h2 className="text-xs font-semibold uppercase tracking-wider text-fg-muted">{kind === 'platform' ? 'Проекты Sb0rka' : 'Gateway'}</h2>
              <p className="text-xs text-fg-muted">{status[kind]?.connected ? `Подключено · проектов: ${status[kind]?.project_ids?.length ?? 0}` : 'Доступ не предоставлен'}</p>
              <div className="flex gap-2">
                <Button size="sm" disabled={busy || !status[kind]} onClick={() => run(() => connectIntegration(kind))}>{status[kind]?.connected ? kind === 'platform' && !configurationAllowed ? 'Разрешить конфигурацию' : 'Изменить доступ' : 'Подключить'}</Button>
                {status[kind]?.connected && <Button size="sm" disabled={busy} variant="ghost" onClick={() => run(async () => { await disconnectIntegration(kind); if (kind === 'platform') setProjectId(null); window.location.reload() })}>Отключить</Button>}
              </div>
            </section>
          ))}
          {!!projects.length && <label className="block space-y-1.5">
            <span className="text-xs font-semibold uppercase tracking-wider text-fg-muted">Проект</span>
            <select aria-label="Проект" className="block w-full rounded border border-border bg-surface-0 px-2 py-1.5 text-xs text-fg outline-none focus:border-fg/40" value={projectId} onChange={(event) => selectProject(event.target.value)}>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select>
          </label>}
          {projectId && status.platform?.connected && configurationAllowed && status.platform.project_ids?.includes(projectId)
            ? <ConfigurationEditor key={projectId} projectId={projectId} canWrite={Boolean(status.platform.scopes?.includes('secrets.write'))} canProbe={Boolean(status.gateway?.connected && status.gateway.project_ids?.includes(projectId))} />
            : projectId && <p className="text-xs text-fg-muted">Для JSON-конфигурации разрешите доступ к конфигурации выбранного проекта через «Проекты Sb0rka».</p>}
        </div>
      </section>
    </div>
  )
}
