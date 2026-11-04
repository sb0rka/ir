import { useEffect, useState } from 'react'
import { getAuthNotice, clearAuthNotice, connectIntegration, disconnectIntegration, integrationStatus, logout, type IntegrationKind, type IntegrationStatus } from '../api/auth'
import { setProjectId } from '../api/env'
import type { Project } from '../api/platform'
import { Button } from './ui'

export function AuthConnections({ projects, currentProjectId, onClose }: { projects: Project[]; currentProjectId?: string; onClose?: (projectId?: string) => void }) {
  const [status, setStatus] = useState<Partial<Record<IntegrationKind, IntegrationStatus>>>({})
  const [error, setError] = useState<string | null>(getAuthNotice())
  const [busy, setBusy] = useState(false)
  const [projectId, selectProject] = useState(currentProjectId ?? projects[0]?.id ?? '')
  useEffect(() => { void Promise.all((['platform', 'gateway'] as const).map(async (kind) => {
    const value = await integrationStatus(kind)
    setStatus((previous) => ({ ...previous, [kind]: value }))
  })).catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Не удалось загрузить интеграции')) }, [])
  const run = (action: () => Promise<void>) => {
    setBusy(true); setError(null)
    void action().catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'Не удалось изменить доступ')).finally(() => setBusy(false))
  }
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 px-4">
      <section role="dialog" aria-label="Подключения IR" className="w-full max-w-lg space-y-5 rounded border border-border bg-surface-0 p-6 text-fg">
        <h1 className="text-lg">Подключения IR</h1>
        <p className="text-sm text-fg-muted">Вы вошли в IR. Для работы с проектами отдельно разрешите доступ к API.</p>
        {error && <p role="alert" className="text-sm text-critical">{error}</p>}
        {(['platform', 'gateway'] as const).map((kind) => (
          <div key={kind} className="space-y-2 border-t border-border pt-3">
            <p>{kind === 'platform' ? 'Проекты Sb0rka' : 'Gateway'}</p>
            <p className="text-xs text-fg-muted">{status[kind]?.connected ? `Подключено · проектов: ${status[kind]?.project_ids?.length ?? 0}` : 'Доступ не предоставлен'}</p>
            <div className="flex gap-2">
              <Button disabled={busy || !status[kind]} onClick={() => run(() => connectIntegration(kind))}>{status[kind]?.connected ? 'Изменить доступ' : 'Подключить'}</Button>
              {status[kind]?.connected && <Button disabled={busy} variant="ghost" onClick={() => run(async () => { await disconnectIntegration(kind); if (kind === 'platform') setProjectId(null); window.location.reload() })}>Отключить</Button>}
            </div>
          </div>
        ))}
        {!!projects.length && <label className="block space-y-2 text-sm">Проект
          <select aria-label="Проект" className="block w-full rounded border border-border bg-surface-0 p-2" value={projectId} onChange={(event) => selectProject(event.target.value)}>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select>
        </label>}
        <div className="flex justify-between gap-2">
          <Button variant="ghost" disabled={busy} onClick={() => run(async () => { await logout(); window.location.reload() })}>Выйти</Button>
          {onClose && <Button onClick={() => { clearAuthNotice(); onClose(projectId || undefined) }}>Готово</Button>}
        </div>
      </section>
    </div>
  )
}
