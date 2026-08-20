import { useEffect, useState } from 'react'
import { TabBar } from './components/TabBar'
import { QueuePage } from './pages/QueuePage'
import { InvestigationPage } from './pages/InvestigationPage'
import { useAppStore } from './store/appStore'
import { ErrorBanner } from './components/ui'
import { env, getSomToken, setSomToken } from './api/env'

export default function App() {
  const activeTab = useAppStore((s) => s.activeTab)
  const lastError = useAppStore((s) => s.lastError)
  const lastNotImplemented = useAppStore((s) => s.lastNotImplemented)
  const somHint = useAppStore((s) => s.somHint)
  const clearError = useAppStore((s) => s.clearError)
  const bootstrap = useAppStore((s) => s.bootstrap)
  const [tokenDraft, setTokenDraft] = useState(() => getSomToken() ?? '')

  useEffect(() => {
    void bootstrap()
  }, [bootstrap])

  return (
    <div className="flex h-full flex-col bg-surface-0 text-fg">
      <header className="flex items-center justify-between gap-4 border-b border-border px-4 py-2">
        <div className="flex items-center gap-3">
          <div className="font-mono text-sm font-semibold tracking-widest">SB0RKA / IR</div>
          <span className="text-[10px] uppercase tracking-wider text-fg-dim">
            live · {env.projectId}
          </span>
        </div>
        <label className="flex min-w-0 max-w-md flex-1 items-center gap-2 text-[11px] text-fg-dim">
          SOM token
          <input
            className="min-w-0 flex-1 rounded border border-border bg-surface-1 px-2 py-1 font-mono text-[11px] text-fg outline-none focus:border-fg/40"
            type="password"
            placeholder="вставьте JWT, если VITE_SOM_TOKEN истёк"
            value={tokenDraft}
            onChange={(e) => setTokenDraft(e.target.value)}
            onBlur={() => setSomToken(tokenDraft.trim() || null)}
          />
        </label>
      </header>
      <ErrorBanner message={lastError} onDismiss={clearError} />
      <ErrorBanner
        message={lastNotImplemented}
        tone="warning"
        onDismiss={clearError}
      />
      <ErrorBanner message={somHint} tone="warning" onDismiss={clearError} />
      <TabBar />
      <main className="min-h-0 flex-1">
        {activeTab === 'queue' ? (
          <QueuePage />
        ) : (
          <InvestigationPage investigationId={activeTab} />
        )}
      </main>
    </div>
  )
}
