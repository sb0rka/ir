import { getProjectId } from './env'

export interface PersistedWorkspaceTabs {
  openIds: string[]
  activeTab: string
}

function storageKey(projectId: string): string {
  return `ir.${projectId}.workspaceTabs`
}

function isPinned(tab: string): boolean {
  return tab === 'queue' || tab === 'investigations'
}

export function readWorkspaceTabs(): PersistedWorkspaceTabs | null {
  const projectId = getProjectId()
  if (!projectId) return null
  try {
    const raw = localStorage.getItem(storageKey(projectId))
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedWorkspaceTabs>
    if (!Array.isArray(parsed.openIds)) return null
    const openIds = parsed.openIds.filter(
      (id): id is string => typeof id === 'string' && id.length > 0 && !isPinned(id),
    )
    const activeTab =
      typeof parsed.activeTab === 'string' && parsed.activeTab
        ? parsed.activeTab
        : 'investigations'
    return { openIds, activeTab }
  } catch {
    return null
  }
}

export function writeWorkspaceTabs(tabs: string[], activeTab: string): void {
  const projectId = getProjectId()
  if (!projectId) return
  const openIds = tabs.filter((tab) => !isPinned(tab))
  try {
    localStorage.setItem(
      storageKey(projectId),
      JSON.stringify({ openIds, activeTab } satisfies PersistedWorkspaceTabs),
    )
  } catch {
    /* Persistence is best-effort; the in-memory tab bar still works. */
  }
}
