import { TabBar } from './components/TabBar'
import { QueuePage } from './pages/QueuePage'
import { InvestigationPage } from './pages/InvestigationPage'
import { useAppStore } from './store/appStore'

export default function App() {
  const activeTab = useAppStore((s) => s.activeTab)

  return (
    <div className="flex h-full flex-col bg-surface-0 text-fg">
      <header className="flex items-center justify-between border-b border-border px-4 py-2">
        <div className="flex items-center gap-3">
          <div className="font-mono text-sm font-semibold tracking-widest">SB0RKA / IR</div>
          <span className="text-[10px] uppercase tracking-wider text-fg-dim">
            прототип v1 · mock data
          </span>
        </div>
        <div className="text-xs text-fg-dim">проект: finance-corp · а.соколов</div>
      </header>
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
