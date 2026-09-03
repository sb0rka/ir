import { type ReactNode } from 'react'
import { Bot, Lightbulb, X } from 'lucide-react'
import { useAppStore, type SidebarSectionId } from '../store/appStore'
import { clsx } from '../lib/utils'
import { AgentSection } from './AgentPanel'
import { HypothesesSection } from './HypothesesSection'
import { ResizablePanelFrame } from './ResizablePanelFrame'
import { Panel } from './ui'

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
  const agentBadge = useAppStore((s) => {
    const inv = s.investigations[investigationId]
    if (!inv) return 0
    return inv.edgeIds.filter(
      (id) => (s.edgeReviews[id] ?? s.graphEdges[id]?.review) === 'proposed',
    ).length
  })
  const hypothesesBadge = useAppStore((s) => {
    const inv = s.investigations[investigationId]
    if (!inv) return 0
    return inv.hypothesisIds.filter((id) => {
      const status = s.hypotheses[id]?.status
      return status === 'proposed' || status === 'active'
    }).length
  })
  const badges: Record<SidebarSectionId, number> = {
    agent: agentBadge,
    hypotheses: hypothesesBadge,
  }

  const active = SECTIONS.find((section) => section.id === sectionId)

  return (
    <div className="relative z-10 flex h-full shrink-0">
      <nav className="flex w-10 flex-col items-center gap-1 border-r border-border bg-surface-1 py-2">
        {SECTIONS.map((section) => {
          const Icon = section.icon
          const selected = sectionId === section.id
          const badge = badges[section.id]
          return (
            <button
              key={section.id}
              type="button"
              title={section.title}
              onClick={() => setSection(selected ? null : section.id)}
              className={clsx(
                'relative flex h-8 w-8 items-center justify-center rounded-md transition-colors',
                selected
                  ? 'bg-surface-3 text-fg'
                  : 'text-fg-muted hover:bg-surface-2 hover:text-fg',
              )}
            >
              <Icon className="h-4 w-4" />
              {badge > 0 && (
                <span className="absolute -right-0.5 -top-0.5 min-w-3 rounded bg-proposed/20 px-0.5 text-center font-mono text-[9px] leading-3 text-proposed">
                  {badge}
                </span>
              )}
            </button>
          )
        })}
      </nav>

      {active && (
        <ResizablePanelFrame storageKey="ir.agentPanel.width" defaultWidth={384} side="left">
          <Panel
            title={active.title}
            side="left"
            className="w-full min-w-0 flex-1"
            actions={
              <button type="button" onClick={() => setSection(null)}>
                <X className="h-3.5 w-3.5 text-fg-dim" />
              </button>
            }
          >
            {active.render(investigationId)}
          </Panel>
        </ResizablePanelFrame>
      )}
    </div>
  )
}
