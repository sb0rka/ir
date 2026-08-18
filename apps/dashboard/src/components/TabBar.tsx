import { useAppStore } from '../store/appStore'
import { clsx, severityDot } from '../lib/utils'
import { LayoutList, X } from 'lucide-react'

export function TabBar() {
  const tabs = useAppStore((s) => s.tabs)
  const activeTab = useAppStore((s) => s.activeTab)
  const investigations = useAppStore((s) => s.investigations)
  const setActiveTab = useAppStore((s) => s.setActiveTab)
  const closeTab = useAppStore((s) => s.closeTab)
  const issues = useAppStore((s) => s.issues)

  return (
    <div className="flex items-center gap-0.5 overflow-x-auto border-b border-border bg-surface-0 px-2 pt-2">
      {tabs.map((tab) => {
        const isQueue = tab === 'queue'
        const inv = isQueue ? null : investigations[tab]
        const running =
          inv?.issueIds.some((id) => issues[id]?.status === 'running') ?? false
        const active = activeTab === tab
        return (
          <button
            key={tab}
            type="button"
            onClick={() => setActiveTab(tab)}
            className={clsx(
              'group flex max-w-[240px] items-center gap-2 rounded-t border border-b-0 px-3 py-1.5 text-sm',
              active
                ? 'border-border bg-surface-1 text-fg'
                : 'border-transparent text-fg-muted hover:bg-surface-1/50 hover:text-fg',
            )}
          >
            {isQueue ? (
              <>
                <LayoutList className="h-3.5 w-3.5" />
                <span>Очередь</span>
              </>
            ) : (
              <>
                {inv && (
                  <span className={clsx('h-2 w-2 rounded-full', severityDot[inv.severity])} />
                )}
                <span className="truncate">{inv?.title ?? tab}</span>
                {running && (
                  <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-proposed" />
                )}
                <span
                  role="button"
                  tabIndex={0}
                  className="ml-1 rounded p-0.5 text-fg-dim opacity-0 hover:bg-surface-3 hover:text-fg group-hover:opacity-100"
                  onClick={(e) => {
                    e.stopPropagation()
                    closeTab(tab)
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.stopPropagation()
                      closeTab(tab)
                    }
                  }}
                >
                  <X className="h-3 w-3" />
                </span>
              </>
            )}
          </button>
        )
      })}
    </div>
  )
}
