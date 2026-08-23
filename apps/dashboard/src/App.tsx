import { useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { LogOut, Settings2 } from 'lucide-react'
import { TabBar } from './components/TabBar'
import { QueuePage } from './pages/QueuePage'
import { InvestigationPage } from './pages/InvestigationPage'
import { useAppStore } from './store/appStore'
import { Button, ErrorBanner } from './components/ui'
import {
  bootstrapAuth,
  getAccessToken,
  login,
  logout,
  subscribeAuth,
  type AuthSubject,
} from './api/auth'
import { getProjectId, setProjectId } from './api/env'
import { listProjects, type Project } from './api/platform'
import { LoginPage } from './components/LoginPage'
import { ConfigurationModal } from './components/ConfigurationModal'

type SessionPhase = 'loading' | 'ready' | 'signed-out' | 'error'

export default function App() {
  const token = useSyncExternalStore(subscribeAuth, getAccessToken, () => null)
  const [phase, setPhase] = useState<SessionPhase>('loading')
  const [subject, setSubject] = useState<AuthSubject | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [sessionError, setSessionError] = useState<string | null>(null)

  const loadProjectSession = async (authenticatedSubject: AuthSubject) => {
    setSubject(authenticatedSubject)
    const available = await listProjects()
    setProjects(available)
    if (available.length) {
      const stored = getProjectId()
      const selected = available.find((project) => project.id === stored) ?? available[0]
      setProjectId(selected.id)
    } else {
      setProjectId(null)
    }
    setPhase('ready')
  }

  useEffect(() => {
    let cancelled = false
    void bootstrapAuth()
      .then(async (authenticatedSubject) => {
        if (cancelled) return
        if (!authenticatedSubject) {
          setPhase('signed-out')
          return
        }
        await loadProjectSession(authenticatedSubject)
      })
      .catch((reason: unknown) => {
        if (cancelled) return
        setSessionError(reason instanceof Error ? reason.message : 'Auth недоступен')
        setPhase(getAccessToken() ? 'error' : 'signed-out')
      })
    return () => {
      cancelled = true
    }
    // Initial bootstrap is intentionally independent of token refresh notifications.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!token && phase === 'ready') {
      // Drop the module-level Zustand workspace before another user can sign in.
      window.location.reload()
    }
  }, [phase, token])

  if (phase === 'loading') return <LoadingScreen label="Проверка сессии" />

  if (phase === 'error' && token) {
    return (
      <LoadingScreen
        label={sessionError || 'Не удалось загрузить проекты'}
        action={
          <div className="flex gap-2">
            <Button onClick={() => window.location.reload()} variant="default">
              Повторить
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                void logout().finally(() => window.location.reload())
              }}
            >
              Выйти
            </Button>
          </div>
        }
      />
    )
  }

  if (!token || !subject) {
    return (
      <LoginPage
        error={sessionError}
        onLogin={async (loginValue, password) => {
          setSessionError(null)
          const authenticatedSubject = await login(loginValue, password)
          try {
            await loadProjectSession(authenticatedSubject)
          } catch (reason) {
            setSessionError(reason instanceof Error ? reason.message : 'Не удалось загрузить проекты')
            setPhase('error')
          }
        }}
      />
    )
  }

  if (!projects.length) {
    return (
      <LoadingScreen
        label="У пользователя нет активных проектов"
        action={
          <Button
            onClick={() => {
              void logout().finally(() => window.location.reload())
            }}
          >
            Выйти
          </Button>
        }
      />
    )
  }

  const currentProject = projects.find((project) => project.id === getProjectId()) ?? projects[0]
  return <Dashboard subject={subject} projects={projects} currentProject={currentProject} />
}

function Dashboard({
  subject,
  projects,
  currentProject,
}: {
  subject: AuthSubject
  projects: Project[]
  currentProject: Project
}) {
  const activeTab = useAppStore((state) => state.activeTab)
  const lastError = useAppStore((state) => state.lastError)
  const lastNotImplemented = useAppStore((state) => state.lastNotImplemented)
  const somHint = useAppStore((state) => state.somHint)
  const clearError = useAppStore((state) => state.clearError)
  const bootstrap = useAppStore((state) => state.bootstrap)
  const bootstrapped = useRef(false)
  const [configurationOpen, setConfigurationOpen] = useState(false)

  useEffect(() => {
    if (bootstrapped.current) return
    bootstrapped.current = true
    void bootstrap()
  }, [bootstrap])

  return (
    <div className="flex h-full flex-col bg-surface-0 text-fg">
      <header className="flex items-center justify-between gap-4 border-b border-border px-4 py-2">
        <div className="flex min-w-0 items-center gap-3">
          <div className="shrink-0 font-mono text-sm font-semibold tracking-widest">SB0RKA / IR</div>
          <span
            className="truncate text-[10px] uppercase tracking-wider text-fg-dim"
            title={currentProject.id}
          >
            live · {currentProject.name}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <span className="hidden max-w-48 truncate px-2 font-mono text-[11px] text-fg-dim sm:block">
            {subject.user?.username ?? subject.subject_id}
          </span>
          <Button size="sm" variant="ghost" onClick={() => setConfigurationOpen(true)}>
            <Settings2 className="h-3.5 w-3.5" />
            Конфигурация
          </Button>
          <Button
            size="sm"
            variant="ghost"
            title="Выйти"
            onClick={() => {
              void logout().finally(() => {
                setProjectId(null)
                window.location.reload()
              })
            }}
          >
            <LogOut className="h-3.5 w-3.5" />
          </Button>
        </div>
      </header>
      <ErrorBanner message={lastError} onDismiss={clearError} />
      <ErrorBanner message={lastNotImplemented} tone="warning" onDismiss={clearError} />
      <ErrorBanner message={somHint} tone="warning" onDismiss={clearError} />
      <TabBar />
      <main className="min-h-0 flex-1">
        {activeTab === 'queue' ? (
          <QueuePage />
        ) : (
          <InvestigationPage investigationId={activeTab} />
        )}
      </main>
      {configurationOpen && (
        <ConfigurationModal
          projects={projects}
          currentProjectId={currentProject.id}
          onClose={(appliedProjectId) => {
            if (appliedProjectId && appliedProjectId !== currentProject.id) {
              setProjectId(appliedProjectId)
              window.location.reload()
              return
            }
            setConfigurationOpen(false)
          }}
        />
      )}
    </div>
  )
}

function LoadingScreen({ label, action }: { label: string; action?: React.ReactNode }) {
  return (
    <main className="ir-auth-grid flex min-h-full items-center justify-center bg-surface-0 p-6 text-fg">
      <div className="rounded border border-border bg-surface-1 px-6 py-5 text-center shadow-xl">
        <div className="font-mono text-xs uppercase tracking-[0.18em] text-fg-muted">{label}</div>
        {action && <div className="mt-4 flex justify-center">{action}</div>}
      </div>
    </main>
  )
}
