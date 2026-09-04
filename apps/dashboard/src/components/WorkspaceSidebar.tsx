import { useState, type ReactNode } from 'react'
import {
  Bot,
  ChevronsLeft,
  ChevronsRight,
  CircleX,
  Inbox,
  Lightbulb,
  Network,
  RefreshCw,
  Table2,
  Undo2,
} from 'lucide-react'
import { useAppStore, type SidebarSectionId } from '../store/appStore'
import { clsx } from '../lib/utils'
import { AgentSection } from './AgentPanel'
import { CloseInvestigationModal } from './CloseInvestigationModal'
import { HypothesesSection } from './HypothesesSection'
import { ResizablePanelFrame } from './ResizablePanelFrame'
import { Panel } from './ui'

const EXPANDED_KEY = 'ir.workspaceSidebar.expanded'

function readExpanded(): boolean {
  try {
    return localStorage.getItem(EXPANDED_KEY) === '1'
  } catch {
    return false
  }
}

function writeExpanded(value: boolean) {
  try {
    localStorage.setItem(EXPANDED_KEY, value ? '1' : '0')
  } catch {
    /* ignore */
  }
}

type InvestigationView = 'table' | 'graph' | 'queue'

const VIEWS: { id: InvestigationView; title: string; icon: typeof Table2 }[] = [
  { id: 'table', title: 'Таблица', icon: Table2 },
  { id: 'graph', title: 'Граф и таймлайн', icon: Network },
  { id: 'queue', title: 'Очередь', icon: Inbox },
]

interface SidebarSectionDef {
  id: SidebarSectionId
  title: string
  icon: typeof Bot
  render: (investigationId: string) => ReactNode
}

const SECTIONS: SidebarSectionDef[] = [
  {
    id: 'agent',
    title: 'ИИ-агент',
    icon: Bot,
    render: (investigationId) => <AgentSection investigationId={investigationId} />,
  },
  {
    id: 'hypotheses',
    title: 'Гипотезы',
    icon: Lightbulb,
    render: (investigationId) => <HypothesesSection investigationId={investigationId} />,
  },
]

export function WorkspaceSidebar({ investigationId }: { investigationId: string }) {
  const sectionId = useAppStore((s) => s.sidebarSection)
  const setSection = useAppStore((s) => s.setSidebarSection)
  const inv = useAppStore((s) => s.investigations[investigationId])
  const view = inv?.view ?? 'graph'
  const update = useAppStore((s) => s.updateInvestigation)
  const persistInvestigation = useAppStore((s) => s.persistInvestigation)
  const loadSomCatalog = useAppStore((s) => s.loadSomCatalog)
  const [expanded, setExpanded] = useState(readExpanded)
  const [closeOpen, setCloseOpen] = useState(false)
  const [closing, setClosing] = useState(false)
  const [somRefreshing, setSomRefreshing] = useState(false)
  const agentBadge = useAppStore((s) => {
    const current = s.investigations[investigationId]
    if (!current) return 0
    return current.edgeIds.filter(
      (id) => (s.edgeReviews[id] ?? s.graphEdges[id]?.review) === 'proposed',
    ).length
  })
  const hypothesesBadge = useAppStore((s) => {
    const current = s.investigations[investigationId]
    if (!current) return 0
    return current.hypothesisIds.filter((id) => {
      const status = s.hypotheses[id]?.status
      return status === 'proposed' || status === 'active'
    }).length
  })
  const badges: Record<SidebarSectionId, number> = {
    agent: agentBadge,
    hypotheses: hypothesesBadge,
  }

  const active = SECTIONS.find((section) => section.id === sectionId)
  const closed = inv?.status === 'closed'

  const toggleExpanded = () => {
    setExpanded((prev) => {
      const next = !prev
      writeExpanded(next)
      return next
    })
  }

  const railBtn = (selected: boolean) =>
    clsx(
      'relative flex h-8 items-center rounded-md transition-colors',
      expanded ? 'w-full gap-2 px-2' : 'w-8 justify-center',
      selected
        ? 'bg-surface-3 text-fg'
        : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
    )

  const railDivider = (key: string) => (
    <div
      key={key}
      className={clsx('border-t border-border', expanded ? 'mx-1.5' : 'mx-2')}
    />
  )

  return (
    <div className="relative z-10 flex h-full shrink-0">
      <nav
        className={clsx(
          'flex flex-col border-r border-border bg-surface-1 py-2 transition-[width]',
          expanded ? 'w-44' : 'w-10',
        )}
      >
        <div
          className={clsx(
            'flex flex-col gap-1',
            expanded ? 'items-stretch px-1.5' : 'items-center',
          )}
        >
          {VIEWS.map((item) => {
            const Icon = item.icon
            const selected = view === item.id
            return (
              <button
                key={item.id}
                type="button"
                title={expanded ? undefined : item.title}
                onClick={() => {
                  if (selected) {
                    if (sectionId) setSection(null)
                    return
                  }
                  update(investigationId, { view: item.id })
                  if (sectionId) setSection(null)
                }}
                className={railBtn(selected)}
              >
                <Icon className="h-4 w-4 shrink-0" />
                {expanded ? (
                  <span className="min-w-0 flex-1 truncate text-left text-xs">{item.title}</span>
                ) : null}
              </button>
            )
          })}
        </div>

        <div className="my-2">{railDivider('views')}</div>

        <div
          className={clsx(
            'flex min-h-0 flex-1 flex-col gap-1',
            expanded ? 'items-stretch px-1.5' : 'items-center',
          )}
        >
          {SECTIONS.map((section) => {
            const Icon = section.icon
            const selected = sectionId === section.id
            const badge = badges[section.id]
            return (
              <button
                key={section.id}
                type="button"
                title={expanded ? undefined : section.title}
                onClick={() => setSection(selected ? null : section.id)}
                className={railBtn(selected)}
              >
                <Icon className="h-4 w-4 shrink-0" />
                {expanded ? (
                  <span className="min-w-0 flex-1 truncate text-left text-xs">{section.title}</span>
                ) : null}
                {badge > 0 && (
                  <span
                    className={clsx(
                      'min-w-3 rounded bg-proposed/20 px-0.5 text-center font-mono text-[9px] leading-3 text-proposed',
                      expanded
                        ? 'shrink-0'
                        : 'absolute -right-0.5 -top-0.5',
                    )}
                  >
                    {badge}
                  </span>
                )}
              </button>
            )
          })}
        </div>

        <div
          className={clsx(
            'mt-auto flex flex-col gap-2 pt-2',
            expanded ? 'px-1.5' : 'items-center',
          )}
        >
          {inv ? (
            closed ? (
              <button
                type="button"
                title={expanded ? undefined : 'Вернуть в работу'}
                disabled={closing}
                onClick={() => {
                  setClosing(true)
                  void persistInvestigation(investigationId, { status: 'open' }).finally(() =>
                    setClosing(false),
                  )
                }}
                className={clsx(
                  'flex h-8 items-center rounded-md text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:opacity-40',
                  expanded ? 'w-full gap-2 px-2' : 'w-8 justify-center',
                )}
              >
                <Undo2 className="h-4 w-4 shrink-0" />
                {expanded ? <span className="truncate text-xs">Вернуть в работу</span> : null}
              </button>
            ) : (
              <button
                type="button"
                title={expanded ? undefined : 'Закрыть'}
                onClick={() => setCloseOpen(true)}
                className={clsx(
                  'flex h-8 items-center rounded-md text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg',
                  expanded ? 'w-full gap-2 px-2' : 'w-8 justify-center',
                )}
              >
                <CircleX className="h-4 w-4 shrink-0" />
                {expanded ? <span className="truncate text-xs">Закрыть</span> : null}
              </button>
            )
          ) : null}

          {railDivider('footer')}

          <button
            type="button"
            title={expanded ? 'Свернуть' : 'Развернуть'}
            aria-label={expanded ? 'Свернуть сайдбар' : 'Развернуть сайдбар'}
            aria-expanded={expanded}
            onClick={toggleExpanded}
            className={clsx(
              'flex h-8 items-center rounded-md text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg',
              expanded ? 'w-full gap-2 px-2' : 'w-8 justify-center',
            )}
          >
            {expanded ? (
              <ChevronsLeft className="h-4 w-4 shrink-0" />
            ) : (
              <ChevronsRight className="h-4 w-4" />
            )}
          </button>
        </div>
      </nav>

      {active && (
        <ResizablePanelFrame storageKey="ir.agentPanel.width" defaultWidth={384} side="left">
          <Panel
            title={active.title}
            side="left"
            className="w-full min-w-0 flex-1"
            actions={
              active.id === 'agent' ? (
                <button
                  type="button"
                  title="Обновить задачи SOM"
                  aria-label="Обновить задачи SOM"
                  disabled={somRefreshing}
                  onClick={() => {
                    setSomRefreshing(true)
                    void loadSomCatalog().finally(() => setSomRefreshing(false))
                  }}
                >
                  <RefreshCw
                    className={clsx(
                      'h-3.5 w-3.5 text-fg-dim',
                      somRefreshing && 'animate-spin',
                    )}
                  />
                </button>
              ) : undefined
            }
          >
            {active.render(investigationId)}
          </Panel>
        </ResizablePanelFrame>
      )}

      {closeOpen && inv && !closed ? (
        <CloseInvestigationModal
          title={inv.title}
          busy={closing}
          onClose={() => {
            if (!closing) setCloseOpen(false)
          }}
          onConfirm={async ({ verdict, reason }) => {
            setClosing(true)
            const ok = await persistInvestigation(investigationId, {
              status: 'closed',
              verdict,
              verdictReason: reason || null,
            })
            setClosing(false)
            if (ok) setCloseOpen(false)
          }}
        />
      ) : null}
    </div>
  )
}
