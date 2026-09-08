import { create } from 'zustand'
import { errorMessage } from '../api/error'
import {
  addFieldToAst,
  bumpFieldFreq,
  defaultQuery,
  fetchEventFields,
  groupCountColumn,
  loadFieldFreq,
  moveFilterNode,
  parse,
  removeFilterNode,
  removeGroup as removeGroupFromQuery,
  reorderFilterNodes,
  serialize,
  setFilterJoiner,
  setGroupAggregate as setGroupAggregateOnQuery,
  toggleFilterGroupNegated,
  ungroupFilter,
  updateCondition as updateConditionInTree,
  wrapFilterAdjacent,
  type ActiveSection,
  type AggregateFn,
  type Column,
  type Condition,
  type EventFieldDef,
  type LogicalJoiner,
  type ParseError,
  type QueryAst,
} from '../lib/pdql'

function moveItem<T>(items: T[], index: number, delta: number): T[] {
  const next = index + delta
  if (next < 0 || next >= items.length) return items
  const copy = items.slice()
  const [item] = copy.splice(index, 1)
  copy.splice(next, 0, item)
  return copy
}

function commit(query: QueryAst) {
  return { query, pdqlDraft: serialize(query), parseError: null as ParseError | null }
}

interface PdqlState {
  fields: EventFieldDef[]
  fieldsLoading: boolean
  fieldsError: string | null
  fieldFreq: Record<string, number>
  query: QueryAst
  activeSection: ActiveSection
  pdqlDraft: string
  parseError: ParseError | null
  defaultDatetimeIso: string
  loadFields: () => Promise<void>
  setActiveSection: (section: ActiveSection) => void
  addField: (name: string, section?: ActiveSection, parentId?: string | null) => void
  removeCondition: (id: string) => void
  updateCondition: (id: string, patch: Partial<Condition>) => void
  setJoiner: (parentId: string | null, index: number, joiner: LogicalJoiner) => void
  wrapAdjacent: (parentId: string | null, joinerIndex: number) => void
  ungroup: (groupId: string) => void
  toggleGroupNegated: (groupId: string) => void
  removeColumn: (id: string) => void
  setColumnSort: (id: string, sort: Column['sort'] | undefined) => void
  setColumnAggregate: (id: string, aggregate: AggregateFn | undefined) => void
  setGroupAggregate: (aggregate: AggregateFn) => void
  setGroupSort: (sort: Column['sort'] | undefined) => void
  removeGroup: (id: string) => void
  moveCondition: (parentId: string | null, index: number, delta: number) => void
  moveColumn: (index: number, delta: number) => void
  moveGroup: (index: number, delta: number) => void
  reorder: (section: ActiveSection, from: number, to: number, parentId?: string | null) => void
  setPdqlDraft: (text: string) => void
  applyPdql: () => boolean
  initFrom: (pdql: string, defaultDatetimeIso?: string) => void
  resetQuery: () => void
}

export const usePdqlStore = create<PdqlState>((set, get) => ({
  fields: [],
  fieldsLoading: false,
  fieldsError: null,
  fieldFreq: loadFieldFreq(),
  query: defaultQuery(),
  activeSection: 'filter',
  pdqlDraft: serialize(defaultQuery()),
  parseError: null,
  defaultDatetimeIso: new Date(0).toISOString(),

  loadFields: async () => {
    set({ fieldsLoading: true, fieldsError: null })
    try {
      const fields = await fetchEventFields()
      set({ fields, fieldsLoading: false })
    } catch (err) {
      set({ fieldsLoading: false, fieldsError: errorMessage(err) })
    }
  },

  setActiveSection: (activeSection) => set({ activeSection }),

  addField: (name, section, parentId) => {
    const target = section ?? get().activeSection
    const { query, fields, fieldFreq } = get()
    const next = addFieldToAst(query, name, target, fields, parentId ?? null)
    set({
      ...commit(next),
      activeSection: target,
      fieldFreq: bumpFieldFreq(fieldFreq, name),
    })
  },

  removeCondition: (id) => {
    set(commit(removeFilterNode(get().query, id)))
  },

  updateCondition: (id, patch) => {
    set(commit(updateConditionInTree(get().query, id, patch)))
  },

  setJoiner: (parentId, index, joiner) => {
    set(commit(setFilterJoiner(get().query, parentId, index, joiner)))
  },

  wrapAdjacent: (parentId, joinerIndex) => {
    set(commit(wrapFilterAdjacent(get().query, parentId, joinerIndex)))
  },

  ungroup: (groupId) => {
    set(commit(ungroupFilter(get().query, groupId)))
  },

  toggleGroupNegated: (groupId) => {
    set(commit(toggleFilterGroupNegated(get().query, groupId)))
  },

  removeColumn: (id) => {
    set(commit({ ...get().query, columns: get().query.columns.filter((item) => item.id !== id) }))
  },

  setColumnSort: (id, sort) => {
    const columns = get().query.columns.map((item) => {
      if (item.id !== id) return item
      return { ...item, sort }
    })
    if (sort) {
      const used = new Set(
        columns.filter((item) => item.sort && item.id !== id).map((item) => item.sort?.priority ?? 0),
      )
      if (used.has(sort.priority)) {
        let priority = 1
        for (const column of columns) {
          if (!column.sort) continue
          column.sort = { ...column.sort, priority }
          priority += 1
        }
      }
    }
    set(commit({ ...get().query, columns }))
  },

  setColumnAggregate: (id, aggregate) => {
    set(
      commit({
        ...get().query,
        columns: get().query.columns.map((item) => (item.id === id ? { ...item, aggregate } : item)),
      }),
    )
  },

  setGroupAggregate: (aggregate) => {
    set(commit(setGroupAggregateOnQuery(get().query, aggregate)))
  },

  setGroupSort: (sort) => {
    let query = get().query
    if (!groupCountColumn(query)) {
      query = setGroupAggregateOnQuery(query, 'count')
    }
    const target = groupCountColumn(query)
    if (!target) return
    const columns = query.columns.map((item) => {
      if (item.id !== target.id) return item
      return { ...item, sort }
    })
    if (sort) {
      const used = new Set(
        columns.filter((item) => item.sort && item.id !== target.id).map((item) => item.sort?.priority ?? 0),
      )
      if (used.has(sort.priority)) {
        let priority = 1
        for (const column of columns) {
          if (!column.sort) continue
          column.sort = { ...column.sort, priority }
          priority += 1
        }
      }
    }
    set(commit({ ...query, columns }))
  },

  removeGroup: (id) => {
    set(commit(removeGroupFromQuery(get().query, id)))
  },

  moveCondition: (parentId, index, delta) => {
    set(commit(moveFilterNode(get().query, parentId, index, delta)))
  },

  moveColumn: (index, delta) => {
    const query = get().query
    const columns = moveItem(query.columns, index, delta)
    if (columns === query.columns) return
    set(commit({ ...query, columns }))
  },

  moveGroup: (index, delta) => {
    const query = get().query
    const groups = moveItem(query.groups, index, delta)
    if (groups === query.groups) return
    set(commit({ ...query, groups }))
  },

  reorder: (section, from, to, parentId) => {
    const query = get().query
    if (section === 'filter') {
      set(commit(reorderFilterNodes(query, parentId ?? null, from, to)))
      return
    }
    if (section === 'columns') {
      const columns = query.columns.slice()
      const [item] = columns.splice(from, 1)
      columns.splice(to, 0, item)
      set(commit({ ...query, columns }))
      return
    }
    const groups = query.groups.slice()
    const [item] = groups.splice(from, 1)
    groups.splice(to, 0, item)
    set(commit({ ...query, groups }))
  },

  setPdqlDraft: (pdqlDraft) => set({ pdqlDraft }),

  applyPdql: () => {
    const result = parse(get().pdqlDraft)
    if (!result.ok) {
      set({ parseError: result.error })
      return false
    }
    set(commit(result.ast))
    return true
  },

  initFrom: (pdql, defaultDatetimeIso) => {
    const trimmed = pdql.trim()
    const stamp = defaultDatetimeIso ?? get().defaultDatetimeIso
    if (!trimmed) {
      set({ ...commit(defaultQuery()), activeSection: 'filter', defaultDatetimeIso: stamp })
      return
    }
    const result = parse(trimmed)
    if (!result.ok) {
      set({ pdqlDraft: pdql, parseError: result.error, defaultDatetimeIso: stamp })
      return
    }
    set({ ...commit(result.ast), activeSection: 'filter', defaultDatetimeIso: stamp })
  },

  resetQuery: () => {
    set({ ...commit(defaultQuery()), activeSection: 'filter' })
  },
}))
