import { create } from 'zustand'
import { errorMessage } from '../api/error'
import {
  addFieldToAst,
  bumpFieldFreq,
  defaultQuery,
  fetchEventFields,
  groupCountColumn,
  loadFieldFreq,
  parse,
  removeGroup as removeGroupFromQuery,
  serialize,
  setGroupAggregate as setGroupAggregateOnQuery,
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
  loadFields: () => Promise<void>
  setActiveSection: (section: ActiveSection) => void
  addField: (name: string, section?: ActiveSection) => void
  removeCondition: (id: string) => void
  updateCondition: (id: string, patch: Partial<Condition>) => void
  setJoiner: (index: number, joiner: LogicalJoiner) => void
  removeColumn: (id: string) => void
  setColumnSort: (id: string, sort: Column['sort'] | undefined) => void
  setColumnAggregate: (id: string, aggregate: AggregateFn | undefined) => void
  setGroupAggregate: (aggregate: AggregateFn) => void
  setGroupSort: (sort: Column['sort'] | undefined) => void
  removeGroup: (id: string) => void
  moveCondition: (index: number, delta: number) => void
  moveColumn: (index: number, delta: number) => void
  moveGroup: (index: number, delta: number) => void
  reorder: (section: ActiveSection, from: number, to: number) => void
  setPdqlDraft: (text: string) => void
  applyPdql: () => boolean
  initFrom: (pdql: string) => void
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

  addField: (name, section) => {
    const target = section ?? get().activeSection
    const { query, fields, fieldFreq } = get()
    const next = addFieldToAst(query, name, target, fields)
    set({
      ...commit(next),
      activeSection: target,
      fieldFreq: bumpFieldFreq(fieldFreq, name),
    })
  },

  removeCondition: (id) => {
    const { query } = get()
    const index = query.filter.findIndex((item) => item.id === id)
    if (index < 0) return
    const filter = query.filter.filter((item) => item.id !== id)
    const joiners = query.joiners.filter((_, joinerIndex) =>
      index === 0 ? joinerIndex !== 0 : joinerIndex !== index - 1,
    )
    set(commit({ ...query, filter, joiners }))
  },

  updateCondition: (id, patch) => {
    set(
      commit({
        ...get().query,
        filter: get().query.filter.map((item) => (item.id === id ? { ...item, ...patch } : item)),
      }),
    )
  },

  setJoiner: (index, joiner) => {
    const joiners = get().query.joiners.slice()
    if (!joiners[index]) return
    joiners[index] = joiner
    set(commit({ ...get().query, joiners }))
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

  moveCondition: (index, delta) => {
    const query = get().query
    const filter = moveItem(query.filter, index, delta)
    if (filter === query.filter) return
    set(commit({ ...query, filter }))
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

  reorder: (section, from, to) => {
    const query = get().query
    if (section === 'filter') {
      const filter = query.filter.slice()
      const [item] = filter.splice(from, 1)
      filter.splice(to, 0, item)
      set(commit({ ...query, filter }))
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

  initFrom: (pdql) => {
    const trimmed = pdql.trim()
    if (!trimmed) {
      set({ ...commit(defaultQuery()), activeSection: 'filter' })
      return
    }
    const result = parse(trimmed)
    if (!result.ok) {
      set({ pdqlDraft: pdql, parseError: result.error })
      return
    }
    set({ ...commit(result.ast), activeSection: 'filter' })
  },

  resetQuery: () => {
    set({ ...commit(defaultQuery()), activeSection: 'filter' })
  },
}))
