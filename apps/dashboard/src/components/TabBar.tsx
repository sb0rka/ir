import { useEffect, useRef } from 'react'
import { X } from 'lucide-react'
import { isPinnedTab, useAppStore } from '../store/appStore'
import { clsx, severityDot } from '../lib/utils'

/** Open investigation case tabs. Hidden when none are open. */
export function TabBar() {
  const tabs = useAppStore((s) => s.tabs)
  const activeTab = useAppStore((s) => s.activeTab)
  const investigations = useAppStore((s) => s.investigations)
  const setActiveTab = useAppStore((s) => s.setActiveTab)
  const closeTab = useAppStore((s) => s.closeTab)
  const issues = useAppStore((s) => s.issues)
  const workspaceTabs = tabs.filter((tab) => !isPinnedTab(tab))
  const scrollerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = scrollerRef.current
    if (!el) return
    const onWheel = (event: WheelEvent) => {
      if (el.scrollWidth <= el.clientWidth) return
      const delta =
        Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY
      if (delta === 0) return
      event.preventDefault()
      el.scrollLeft += delta
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [workspaceTabs.length])

  useEffect(() => {
    if (isPinnedTab(activeTab)) return
    const frame = window.requestAnimationFrame(() => {
      const active = scrollerRef.current?.querySelector<HTMLElement>(
        `[data-tab-id="${CSS.escape(activeTab)}"]`,
      )
      active?.scrollIntoView({ behavior: 'smooth', inline: 'nearest', block: 'nearest' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [activeTab, workspaceTabs.length])

  if (workspaceTabs.length === 0) return null

  return (
    <div
      ref={scrollerRef}
      className="flex items-center gap-0.5 overflow-x-auto border-b border-border bg-surface-0 px-2 pt-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {workspaceTabs.map((tab) => {
        const inv = investigations[tab]
        const running =
          inv?.issueIds.some((id) => issues[id]?.status === 'running') ?? false
        const active = activeTab === tab
        return (
          <button
            key={tab}
            type="button"
            data-tab-id={tab}
            onClick={() => setActiveTab(tab)}
            className={clsx(
              'group flex max-w-[240px] shrink-0 items-center gap-2 rounded-t border border-b-0 px-3 py-1.5 text-sm',
              active
                ? 'border-border bg-surface-1 text-fg'
                : 'border-transparent text-fg-muted hover:bg-surface-1/50 hover:text-fg',
            )}
          >
            {inv && (
              <span className={clsx('h-2 w-2 shrink-0 rounded-full', severityDot[inv.severity])} />
            )}
            <span className="truncate">{inv?.title ?? tab}</span>
            {running && (
              <span className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-proposed" />
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
          </button>
        )
      })}
    </div>
  )
}
