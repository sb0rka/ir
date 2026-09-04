import { FolderKanban, LayoutList } from 'lucide-react'
import { useAppStore } from '../store/appStore'
import { clsx } from '../lib/utils'

const PRIMARY = [
  { id: 'queue' as const, label: 'Очередь', Icon: LayoutList },
  { id: 'investigations' as const, label: 'Расследования', Icon: FolderKanban },
]

/** Pinned queue / investigations switches for the app header. */
export function PrimaryNav() {
  const activeTab = useAppStore((s) => s.activeTab)
  const setActiveTab = useAppStore((s) => s.setActiveTab)

  return (
    <nav className="flex shrink-0 items-center gap-1.5" aria-label="Основные разделы">
      {PRIMARY.map(({ id, label, Icon }) => {
        const active = activeTab === id
        return (
          <button
            key={id}
            type="button"
            onClick={() => setActiveTab(id)}
            className={clsx(
              'inline-flex h-8 items-center gap-1.5 rounded px-2.5 text-xs',
              active
                ? 'bg-surface-3 text-fg'
                : 'text-fg-muted hover:bg-surface-2/60 hover:text-fg',
            )}
          >
            <Icon className="h-3.5 w-3.5 shrink-0" />
            <span className="hidden sm:inline">{label}</span>
          </button>
        )
      })}
    </nav>
  )
}
