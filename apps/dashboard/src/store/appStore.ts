import { create } from 'zustand'
import type {
  ActionResult,
  ContextQueueState,
  FilterChip,
  FilterField,
  Investigation,
  Issue,
  ReviewState,
} from '../types'
import {
  alerts,
  contextEvents,
  correlations,
  enrichmentPayload,
  entities,
  findings,
  graphEdges,
  graphNodes,
  issueTemplates,
  savedViews,
  seedInvestigationEdges,
  seedInvestigationEvents,
  seedInvestigationNodes,
} from '../mocks/scenario'
import { uid } from '../lib/utils'

export type TabId = 'queue' | string

interface AppState {
  // Queue
  chips: FilterChip[]
  timePreset: string
  selectedAlertIds: string[]
  expandedCorrelationIds: string[]

  // Tabs / investigations
  tabs: TabId[]
  activeTab: TabId
  investigations: Record<string, Investigation>
  issues: Record<string, Issue>
  eventReviews: Record<string, ReviewState>
  nodeReviews: Record<string, ReviewState>
  edgeReviews: Record<string, ReviewState>
  findingReviews: Record<string, ReviewState>
  actionResults: Record<string, ActionResult[]>
  contextQueue: Record<string, ContextQueueState>
  agentPanelOpen: boolean
  detailPanelOpen: boolean

  // Actions
  setChips: (chips: FilterChip[]) => void
  addChip: (field: FilterField, value: string) => void
  removeChip: (id: string) => void
  removeChipValue: (id: string, value: string) => void
  setTimePreset: (preset: string) => void
  applySavedView: (viewId: string) => void
  toggleAlertSelect: (id: string) => void
  clearAlertSelection: () => void
  toggleCorrelationExpand: (id: string) => void

  setActiveTab: (tab: TabId) => void
  closeTab: (tab: TabId) => void
  startInvestigation: (alertOrCorrIds: string[]) => string
  createChildInvestigation: (parentId: string, entityIds: string[]) => string
  updateInvestigation: (id: string, patch: Partial<Investigation>) => void

  setReview: (
    kind: 'event' | 'node' | 'edge' | 'finding',
    id: string,
    review: ReviewState,
  ) => void
  setContextQueue: (investigationId: string, patch: Partial<ContextQueueState>) => void
  addContextChip: (investigationId: string, field: FilterField, value: string) => void
  removeContextChip: (investigationId: string, chipId: string) => void
  removeContextChipValue: (investigationId: string, chipId: string, value: string) => void
  clearContextChips: (investigationId: string) => void
  addEventsToContext: (investigationId: string, eventIds: string[]) => void
  setAgentPanelOpen: (open: boolean) => void
  setDetailPanelOpen: (open: boolean) => void

  runEnrichment: (investigationId: string) => void
  createIssue: (
    investigationId: string,
    templateId: string,
    entityIds: string[],
    parentId?: string,
  ) => void
  cancelIssue: (issueId: string) => void
  addIssueComment: (issueId: string, text: string) => void
  runEntityAction: (entityId: string, action: string) => void
  addFindingFromEntity: (investigationId: string, entityId: string) => void
}

export const emptyContextQueue: ContextQueueState = {
  chips: [],
  timePreset: '24h',
  history: [],
  selectedIds: [],
  hideAdded: false,
  originFilter: 'all',
  reviewFilter: 'all',
}

/** Adds a value to the chip of its field, or creates the chip. */
function mergeChipValue(
  chips: FilterChip[],
  field: FilterField,
  value: string,
): FilterChip[] {
  const existing = chips.find((c) => c.field === field)
  if (!existing) return [...chips, { id: uid('chip'), field, values: [value] }]
  if (existing.values.includes(value)) return chips
  return chips.map((c) =>
    c.id === existing.id ? { ...c, values: [...c.values, value] } : c,
  )
}

function buildInvestigationFromSeed(ids: string[]): Investigation {
  const seedEventIds = [...seedInvestigationEvents]
  const entityIdSet = new Set<string>()
  for (const eid of seedEventIds) {
    contextEvents[eid]?.entityIds.forEach((x) => entityIdSet.add(x))
  }
  // Also pull entities from selected alerts/correlations
  for (const id of ids) {
    if (alerts[id]) alerts[id].entityIds.forEach((x) => entityIdSet.add(x))
    if (correlations[id]) correlations[id].entityIds.forEach((x) => entityIdSet.add(x))
  }

  const title =
    ids.length === 1 && correlations[ids[0]]
      ? correlations[ids[0]].title
      : ids.length === 1 && alerts[ids[0]]
        ? alerts[ids[0]].title
        : `Расследование (${ids.length})`

  const severity =
    ids
      .map((id) => correlations[id]?.severity ?? alerts[id]?.severity)
      .filter(Boolean)
      .sort((a, b) => {
        const order = ['critical', 'high', 'medium', 'low', 'info']
        return order.indexOf(a!) - order.indexOf(b!)
      })[0] ?? 'high'

  return {
    id: uid('inv'),
    title,
    severity: severity as Investigation['severity'],
    status: 'in_progress',
    assignee: 'а.соколов',
    seedEventIds: [...ids],
    eventIds: [...seedEventIds],
    entityIds: [...entityIdSet],
    nodeIds: [...seedInvestigationNodes],
    edgeIds: [...seedInvestigationEdges],
    findingIds: ['find-001', 'find-002'],
    issueIds: [],
    createdAt: new Date().toISOString(),
    view: 'graph',
    selectedEntityIds: [],
  }
}

export const useAppStore = create<AppState>((set, get) => ({
  chips: [],
  timePreset: '24h',
  selectedAlertIds: [],
  expandedCorrelationIds: ['corr-001'],

  tabs: ['queue'],
  activeTab: 'queue',
  investigations: {},
  issues: {},
  eventReviews: Object.fromEntries(
    Object.values(contextEvents).map((e) => [e.id, e.review]),
  ),
  nodeReviews: Object.fromEntries(
    Object.values(graphNodes).map((n) => [n.id, n.review]),
  ),
  edgeReviews: Object.fromEntries(
    Object.values(graphEdges).map((e) => [e.id, e.review]),
  ),
  findingReviews: Object.fromEntries(
    Object.values(findings).map((f) => [f.id, f.review]),
  ),
  actionResults: {},
  contextQueue: {},
  agentPanelOpen: false,
  detailPanelOpen: false,

  setChips: (chips) => set({ chips }),
  addChip: (field, value) => {
    set({ chips: mergeChipValue(get().chips, field, value) })
  },
  removeChip: (id) => set({ chips: get().chips.filter((c) => c.id !== id) }),
  removeChipValue: (id, value) => {
    set({
      chips: get()
        .chips.map((c) =>
          c.id === id ? { ...c, values: c.values.filter((v) => v !== value) } : c,
        )
        .filter((c) => c.values.length > 0),
    })
  },
  setTimePreset: (timePreset) => set({ timePreset }),
  applySavedView: (viewId) => {
    const view = savedViews.find((v) => v.id === viewId)
    if (!view) return
    set({
      chips: view.chips.map((c) => ({ ...c, id: uid('chip') })),
      timePreset: view.timePreset,
    })
  },
  toggleAlertSelect: (id) => {
    const sel = get().selectedAlertIds
    set({
      selectedAlertIds: sel.includes(id) ? sel.filter((x) => x !== id) : [...sel, id],
    })
  },
  clearAlertSelection: () => set({ selectedAlertIds: [] }),
  toggleCorrelationExpand: (id) => {
    const cur = get().expandedCorrelationIds
    set({
      expandedCorrelationIds: cur.includes(id)
        ? cur.filter((x) => x !== id)
        : [...cur, id],
    })
  },
  setActiveTab: (activeTab) => set({ activeTab }),
  closeTab: (tab) => {
    if (tab === 'queue') return
    const tabs = get().tabs.filter((t) => t !== tab)
    const activeTab = get().activeTab === tab ? tabs[tabs.length - 1] ?? 'queue' : get().activeTab
    set({ tabs, activeTab })
  },

  startInvestigation: (ids) => {
    const inv = buildInvestigationFromSeed(ids)
    set({
      investigations: { ...get().investigations, [inv.id]: inv },
      tabs: [...get().tabs, inv.id],
      activeTab: inv.id,
      selectedAlertIds: [],
    })
    return inv.id
  },

  createChildInvestigation: (parentId, entityIds) => {
    const parent = get().investigations[parentId]
    if (!parent) return ''
    const child: Investigation = {
      ...parent,
      id: uid('inv'),
      title: `${parent.title} → ветка`,
      parentId,
      entityIds: [...entityIds],
      selectedEntityIds: [],
      selectedNodeId: undefined,
      selectedEventId: undefined,
      // Keep events/nodes that touch selected entities
      eventIds: parent.eventIds.filter((eid) =>
        contextEvents[eid]?.entityIds.some((x) => entityIds.includes(x)),
      ),
      nodeIds: parent.nodeIds.filter((nid) => {
        const n = graphNodes[nid]
        return n && entityIds.includes(n.refId)
      }),
      edgeIds: parent.edgeIds.filter((eid) => {
        const e = graphEdges[eid]
        if (!e) return false
        const s = graphNodes[e.source]
        const t = graphNodes[e.target]
        return (
          (s && entityIds.includes(s.refId)) || (t && entityIds.includes(t.refId))
        )
      }),
      issueIds: [],
      createdAt: new Date().toISOString(),
      status: 'in_progress',
    }
    set({
      investigations: { ...get().investigations, [child.id]: child },
      tabs: [...get().tabs, child.id],
      activeTab: child.id,
    })
    return child.id
  },

  updateInvestigation: (id, patch) => {
    const inv = get().investigations[id]
    if (!inv) return
    set({
      investigations: {
        ...get().investigations,
        [id]: { ...inv, ...patch },
      },
    })
  },

  setReview: (kind, id, review) => {
    if (kind === 'event') {
      set({ eventReviews: { ...get().eventReviews, [id]: review } })
    } else if (kind === 'node') {
      set({ nodeReviews: { ...get().nodeReviews, [id]: review } })
    } else if (kind === 'edge') {
      set({ edgeReviews: { ...get().edgeReviews, [id]: review } })
    } else {
      set({ findingReviews: { ...get().findingReviews, [id]: review } })
    }
  },

  setContextQueue: (investigationId, patch) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, ...patch },
      },
    })
  },

  addContextChip: (investigationId, field, value) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          chips: mergeChipValue(cur.chips, field, value),
          history: [
            { field, value },
            ...cur.history.filter((h) => !(h.field === field && h.value === value)),
          ].slice(0, 8),
        },
      },
    })
  },

  removeContextChip: (investigationId, chipId) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          chips: cur.chips.filter((c) => c.id !== chipId),
        },
      },
    })
  },

  removeContextChipValue: (investigationId, chipId, value) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: {
          ...cur,
          chips: cur.chips
            .map((c) =>
              c.id === chipId ? { ...c, values: c.values.filter((v) => v !== value) } : c,
            )
            .filter((c) => c.values.length > 0),
        },
      },
    })
  },

  clearContextChips: (investigationId) => {
    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, chips: [] },
      },
    })
  },

  addEventsToContext: (investigationId, eventIds) => {
    const inv = get().investigations[investigationId]
    if (!inv) return
    const newIds = eventIds.filter(
      (id) => contextEvents[id] && !inv.eventIds.includes(id),
    )
    if (newIds.length === 0) return

    const entityIdSet = new Set(inv.entityIds)
    const eventReviews = { ...get().eventReviews }
    newIds.forEach((id) => {
      const ev = contextEvents[id]
      ev.entityIds.forEach((x) => entityIdSet.add(x))
      // Manual add: the analyst vouches for the event, no review needed
      contextEvents[id] = { ...ev, origin: 'analyst' }
      eventReviews[id] = 'confirmed'
    })

    const cur = get().contextQueue[investigationId] ?? emptyContextQueue
    set({
      eventReviews,
      investigations: {
        ...get().investigations,
        [investigationId]: {
          ...inv,
          eventIds: [...inv.eventIds, ...newIds],
          entityIds: [...entityIdSet],
        },
      },
      contextQueue: {
        ...get().contextQueue,
        [investigationId]: { ...cur, selectedIds: [] },
      },
    })
  },

  setAgentPanelOpen: (agentPanelOpen) => set({ agentPanelOpen }),
  setDetailPanelOpen: (detailPanelOpen) => set({ detailPanelOpen }),

  runEnrichment: (investigationId) => {
    const inv = get().investigations[investigationId]
    if (!inv) return

    const issue: Issue = {
      id: uid('issue'),
      investigationId,
      template: 'Насыщение контекста',
      title: 'Насыщение контекста',
      description: issueTemplates[0].description,
      entityIds: inv.entityIds.slice(0, 4),
      status: 'running',
      eventsFound: 0,
      edgesFound: 0,
      findingsFound: 0,
      comments: [],
      createdAt: new Date().toISOString(),
    }

    set({
      issues: { ...get().issues, [issue.id]: issue },
      investigations: {
        ...get().investigations,
        [investigationId]: {
          ...inv,
          issueIds: [...inv.issueIds, issue.id],
        },
      },
      agentPanelOpen: true,
    })

    // Simulate async enrichment
    setTimeout(() => {
      const current = get().investigations[investigationId]
      const currentIssue = get().issues[issue.id]
      if (!current || !currentIssue || currentIssue.status === 'cancelled') return

      const newEventIds = enrichmentPayload.eventIds.filter(
        (id) => !current.eventIds.includes(id),
      )
      const newNodeIds = enrichmentPayload.nodeIds.filter(
        (id) => !current.nodeIds.includes(id),
      )
      const newEdgeIds = enrichmentPayload.edgeIds.filter(
        (id) => !current.edgeIds.includes(id),
      )
      const newFindingIds = enrichmentPayload.findingIds.filter(
        (id) => !current.findingIds.includes(id),
      )

      // Ensure proposed reviews
      const eventReviews = { ...get().eventReviews }
      const nodeReviews = { ...get().nodeReviews }
      const edgeReviews = { ...get().edgeReviews }
      const findingReviews = { ...get().findingReviews }
      newEventIds.forEach((id) => {
        eventReviews[id] = 'proposed'
      })
      newNodeIds.forEach((id) => {
        nodeReviews[id] = 'proposed'
      })
      newEdgeIds.forEach((id) => {
        edgeReviews[id] = 'proposed'
      })
      newFindingIds.forEach((id) => {
        findingReviews[id] = 'proposed'
      })

      const newEntityIds = new Set(current.entityIds)
      newNodeIds.forEach((nid) => {
        const n = graphNodes[nid]
        if (n) newEntityIds.add(n.refId)
      })

      set({
        eventReviews,
        nodeReviews,
        edgeReviews,
        findingReviews,
        issues: {
          ...get().issues,
          [issue.id]: {
            ...currentIssue,
            status: 'completed',
            eventsFound: newEventIds.length,
            edgesFound: newEdgeIds.length,
            findingsFound: newFindingIds.length,
            resultSummary: `Найдено: ${newEventIds.length} событий, ${newEdgeIds.length} связей, ${newFindingIds.length} находок. Требуется подтверждение.`,
          },
        },
        investigations: {
          ...get().investigations,
          [investigationId]: {
            ...current,
            eventIds: [...current.eventIds, ...newEventIds],
            nodeIds: [...current.nodeIds, ...newNodeIds],
            edgeIds: [...current.edgeIds, ...newEdgeIds],
            findingIds: [...current.findingIds, ...newFindingIds],
            entityIds: [...newEntityIds],
          },
        },
      })
    }, 2500)
  },

  createIssue: (investigationId, templateId, entityIds, parentId) => {
    const tpl = issueTemplates.find((t) => t.id === templateId) ?? issueTemplates[0]
    const inv = get().investigations[investigationId]
    if (!inv) return

    const issue: Issue = {
      id: uid('issue'),
      investigationId,
      parentId,
      template: tpl.title,
      title: tpl.title,
      description: tpl.description,
      entityIds,
      status: 'running',
      eventsFound: 0,
      edgesFound: 0,
      findingsFound: 0,
      comments: [],
      createdAt: new Date().toISOString(),
    }

    set({
      issues: { ...get().issues, [issue.id]: issue },
      investigations: {
        ...get().investigations,
        [investigationId]: {
          ...inv,
          issueIds: [...inv.issueIds, issue.id],
        },
      },
      agentPanelOpen: true,
    })

    setTimeout(() => {
      const currentIssue = get().issues[issue.id]
      if (!currentIssue || currentIssue.status === 'cancelled') return
      set({
        issues: {
          ...get().issues,
          [issue.id]: {
            ...currentIssue,
            status: 'completed',
            eventsFound: templateId === 'tpl-hash-hunt' ? 2 : 1,
            edgesFound: 1,
            findingsFound: 1,
            resultSummary: 'Проверка завершена. Результаты добавлены как предложения.',
          },
        },
      })
    }, 2000)
  },

  cancelIssue: (issueId) => {
    const issue = get().issues[issueId]
    if (!issue || issue.status !== 'running') return
    set({
      issues: {
        ...get().issues,
        [issueId]: { ...issue, status: 'cancelled', resultSummary: 'Отменено аналитиком' },
      },
    })
  },

  addIssueComment: (issueId, text) => {
    const issue = get().issues[issueId]
    if (!issue) return
    set({
      issues: {
        ...get().issues,
        [issueId]: {
          ...issue,
          comments: [
            ...issue.comments,
            {
              id: uid('cmt'),
              author: 'а.соколов',
              time: new Date().toISOString(),
              text,
            },
          ],
        },
      },
    })
  },

  runEntityAction: (entityId, action) => {
    const entity = entities[entityId]
    if (!entity) return
    const results = get().actionResults[entityId] ?? []
    const bodies: Record<string, string> = {
      enrich: `Обогащение ${entity.label}: найдены 3 связанных события за последние 7 дней, 1 внешняя репутация.`,
      reputation:
        entity.kind === 'ip' || entity.kind === 'domain' || entity.kind === 'file'
          ? `Репутация ${entity.label}: malicious / known_c2 (VirusTotal 42/72, AbuseIPDB confidence 89).`
          : `Репутация для ${entity.kind} недоступна в TI-источниках.`,
      decode: `Декодирование cmdline: Get-Content ... | IEX — загрузка payload с http://185.234.72.19/p.bin`,
      sandbox: `Песочница: файл ${entity.label} — детект Mimikatz-like behavior, network to 185.234.72.19.`,
      related: `Найдено 12 связанных событий по ${entity.label} в окне 24ч.`,
    }
    const titles: Record<string, string> = {
      enrich: 'Обогащение',
      reputation: 'Репутация',
      decode: 'Декодирование',
      sandbox: 'Песочница',
      related: 'Связанные события',
    }
    const result: ActionResult = {
      id: uid('act'),
      action,
      title: titles[action] ?? action,
      body: bodies[action] ?? `Действие ${action} выполнено`,
      time: new Date().toISOString(),
    }
    set({
      actionResults: {
        ...get().actionResults,
        [entityId]: [result, ...results],
      },
      detailPanelOpen: true,
    })
  },

  addFindingFromEntity: (investigationId, entityId) => {
    const inv = get().investigations[investigationId]
    const entity = entities[entityId]
    if (!inv || !entity) return
    const fid = uid('find')
    // Store as dynamic finding via findingReviews only — keep simple
    findings[fid] = {
      id: fid,
      title: `Находка: ${entity.label}`,
      severity: 'high',
      entityIds: [entityId],
      description: `Добавлено аналитиком из карточки ${entity.kind}`,
      review: 'confirmed',
      origin: 'analyst',
    }
    set({
      findingReviews: { ...get().findingReviews, [fid]: 'confirmed' },
      investigations: {
        ...get().investigations,
        [investigationId]: {
          ...inv,
          findingIds: [...inv.findingIds, fid],
        },
      },
    })
  },
}))

// Re-export mock accessors for components
export {
  alerts,
  correlations,
  contextEvents,
  entities,
  findings,
  graphEdges,
  graphNodes,
  queueOrder,
  savedViews,
  filterFieldLabels,
  filterValueOptions,
  issueTemplates,
} from '../mocks/scenario'
